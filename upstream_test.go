package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUpstreamMessageAndThinkingDefaults(t *testing.T) {
	for _, role := range []string{"user", "developer", "system"} {
		input := `{"model":"deepseek-v4.1-flash","messages":[{"role":"` + role + `","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`
		var body map[string]any
		json.Unmarshal(rewriteSystemForUpstream([]byte(input)), &body)
		messages := body["messages"].([]any)
		if messages[0].(map[string]any)["role"] != "system" || body["reasoning_effort"] != "high" {
			t.Fatal("missing system prompt or default thinking", body)
		}
		last := messages[len(messages)-1].(map[string]any)
		parts := last["content"].([]any)
		if parts[1].(map[string]any)["image_url"].(map[string]any)["url"] != "data:image/png;base64,abc" {
			t.Fatal("image lost")
		}
	}
	for _, effort := range []string{"low", "high", "max"} {
		body := map[string]any{"model": "deepseek-v4.1-flash", "reasoning_effort": effort}
		if forceMaxThinking(body) || body["reasoning_effort"] != effort {
			t.Fatal("explicit effort overridden")
		}
	}
}

func TestUpstreamRetryAndStatus(t *testing.T) {
	client := sharedHTTPClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	for _, status := range []int{400, 401, 429, 503} {
		calls := 0
		client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			body, _ := io.ReadAll(r.Body)
			if string(body) != "payload" {
				t.Fatal("retry body changed")
			}
			code := status
			if status == 503 && calls == 2 {
				code = 200
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(`{"code":11128,"msg":"first message is not system prompt"}`)), Header: make(http.Header)}, nil
		})
		req, _ := http.NewRequest("POST", "https://example.test", strings.NewReader("payload"))
		resp, err := openUpstream(req)
		if status == 503 {
			if err != nil || calls != 2 {
				t.Fatalf("transient retry failed: %v", err)
			}
			resp.Body.Close()
		} else {
			var upstream *upstreamError
			if !errors.As(err, &upstream) || upstream.status != status || calls != 1 {
				t.Fatalf("status lost or permanent error retried: %v", err)
			}
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestStreamReadFailureIsNotSuccess(t *testing.T) {
	if _, err := aggregateSSEChecked(failingReader{}, false); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if _, err := aggregateCompletion(failingReader{}, "deepseek-v4.1-flash"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
}

func TestDeepSeekCapabilities(t *testing.T) {
	for _, m := range wbModels() {
		if m.ID != "deepseek-v4.1-flash" {
			continue
		}
		if m.Thinking == nil || len(m.Thinking.Levels) != 3 || strings.Join(m.SupportedInputModalities, ",") != "text,image" {
			t.Fatal(m)
		}
		return
	}
	t.Fatal("model missing")
}

func TestGLM53InternationalCapabilities(t *testing.T) {
	previous := providerName
	t.Cleanup(func() { providerName = previous })
	for _, provider := range []string{"workbuddy", "workbuddy-cn"} {
		providerName = provider
		found := false
		for _, m := range wbModels() {
			if m.ID != "glm-5.3" {
				continue
			}
			found = true
			if m.ContextLength != 300000 || m.OwnedBy != provider || m.Thinking == nil || strings.Join(m.Thinking.Levels, ",") != "low,high,max" || strings.Join(m.SupportedParameters, ",") != "reasoning_effort" {
				t.Fatalf("unexpected GLM-5.3 capabilities: %+v", m)
			}
		}
		if found != (provider == "workbuddy") {
			t.Fatalf("GLM-5.3 registration for %s = %v", provider, found)
		}
	}
}

func TestGLM53UpstreamThinking(t *testing.T) {
	for _, control := range []string{``, `,"reasoning_effort":"low"`, `,"reasoning_effort":"high"`, `,"reasoning_effort":"max"`, `,"thinking":{"type":"disabled"}`} {
		input := `{"model":"glm-5.3","messages":[{"role":"user","content":"hello"}]` + control + `}`
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rewriteSystemForUpstream([]byte(input)), &body); err != nil {
			t.Fatal(err)
		}
		if string(body["model"]) != `"glm-5.3"` {
			t.Fatal("model changed")
		}
		if control == "" {
			if string(body["reasoning_effort"]) != `"high"` {
				t.Fatal("missing default high")
			}
		} else {
			var original map[string]json.RawMessage
			if err := json.Unmarshal([]byte(input), &original); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"reasoning_effort", "thinking"} {
				if string(body[key]) != string(original[key]) {
					t.Fatalf("%s changed: %s", key, body[key])
				}
			}
		}
	}
}

func TestVisionModelsPreserveImageParts(t *testing.T) {
	expected := map[string]bool{
		"glm-5.2": true, "glm-5v-turbo": true, "kimi-k2.7": true,
		"minimax-m3-pay": true, "hy3": true, "deepseek-v4-pro": true,
		"deepseek-v4-flash": true, "deepseek-v4.1-flash": true,
	}
	previous := providerName
	t.Cleanup(func() { providerName = previous })
	for _, provider := range []string{"workbuddy", "workbuddy-cn"} {
		providerName = provider
		seen := 0
		for _, model := range wbModels() {
			wantInputs := "text"
			if expected[model.ID] {
				wantInputs = "text,image"
				seen++
			}
			if strings.Join(model.SupportedInputModalities, ",") != wantInputs || strings.Join(model.SupportedOutputModalities, ",") != "text" {
				t.Fatalf("%s/%s: unexpected modalities", provider, model.ID)
			}
			if !expected[model.ID] {
				continue
			}
			t.Run(provider+"/"+model.ID, func(t *testing.T) {
				for _, url := range []string{"https://example.test/photo.png", "data:image/png;base64,abc"} {
					original := map[string]any{"model": model.ID, "messages": []any{map[string]any{
						"role": "user", "content": []any{
							map[string]any{"type": "text", "text": "Describe this image."},
							map[string]any{"type": "image_url", "image_url": map[string]any{"url": url, "detail": "high"}},
						},
					}}}
					input, err := json.Marshal(original)
					if err != nil {
						t.Fatal(err)
					}
					var result map[string]any
					if err := json.Unmarshal(rewriteSystemForUpstream(input), &result); err != nil {
						t.Fatal(err)
					}
					messages := result["messages"].([]any)
					last, _ := json.Marshal(messages[len(messages)-1])
					want, _ := json.Marshal(original["messages"].([]any)[0])
					if string(last) != string(want) || result["model"] != model.ID {
						t.Fatalf("multimodal request changed: %s", last)
					}
				}
			})
		}
		if seen != len(expected) {
			t.Fatalf("%s: missing vision models", provider)
		}
	}
}
