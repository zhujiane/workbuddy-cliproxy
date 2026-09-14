package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestQuotaPaginationAndCycleZero(t *testing.T) {
	client := sharedHTTPClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	calls := 0
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "www.codebuddy.cn" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Tenant-Id") != "org" {
			t.Fatal("incorrect billing request")
		}
		var body struct{ PageNumber int }
		json.NewDecoder(r.Body).Decode(&body)
		if body.PageNumber != calls {
			t.Fatal("incorrect pagination")
		}
		pkg := `{"PackageName":"gift","CapacityRemain":100,"CycleCapacitySize":10,"CycleCapacityRemain":0}`
		if calls == 2 {
			pkg = `{"PackageName":"paid","CapacityRemain":20,"CapacityUsed":5,"CapacitySize":25}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"Response":{"Data":{"TotalCount":2,"Accounts":[` + pkg + `]}}}}`))}, nil
	})
	q, err := fetchQuota(&storedAuth{Auth: storedTokens{AccessToken: "secret"}, Account: storedAccount{EnterpriseID: "org"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || q.TotalRemain != 20 || q.TotalUsed != 15 || q.TotalSize != 35 {
		t.Fatalf("incorrect quota: %+v", q)
	}
}

func TestQuotaRejectsUnknownAndErrors(t *testing.T) {
	client := sharedHTTPClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	for _, body := range []string{`{}`, `{"code":0,"data":{}}`, `{"code":42,"msg":"secret"}`, `{"code":0,"data":{"Response":{"Data":{"TotalCount":1,"Accounts":[]}}}}`} {
		client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		q, err := fetchQuota(&storedAuth{})
		if err == nil || q != nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unexpected result %v %v", q, err)
		}
	}
}

func TestQuotaManagementFiltersAccountsAndHidesCredentials(t *testing.T) {
	client := sharedHTTPClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("secret"))}, nil
	})
	call := func(method string, raw []byte) ([]byte, error) {
		var request map[string]string
		json.Unmarshal(raw, &request)
		if request["host_callback_id"] != "callback" {
			t.Fatal("missing callback context")
		}
		if method == "host.auth.list" {
			return okEnvelope(map[string]any{"files": []map[string]string{{"provider": "workbuddy", "auth_index": "wb", "name": "file"}, {"provider": "codex", "auth_index": "other"}}})
		}
		if request["auth_index"] != "wb" {
			t.Fatal("read unrelated credential")
		}
		return okEnvelope(map[string]any{"json": json.RawMessage(`{"auth":{"accessToken":"secret"},"account":{"uid":"uid","nickname":"Alice"}}`)})
	}
	raw, err := handleQuotaManagementWithHost([]byte(`{"Method":"GET","Path":"/v0/management/plugins/workbuddy/credits","host_callback_id":"callback"}`), call)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	json.Unmarshal(raw, &env)
	var response pluginapi.ManagementResponse
	json.Unmarshal(env.Result, &response)
	if strings.Contains(string(response.Body), "secret") || !strings.Contains(string(response.Body), "Alice") || !strings.Contains(string(response.Body), "billing HTTP 401") {
		t.Fatalf("bad response %s", response.Body)
	}
	var result struct{ Accounts []quotaAccount }
	json.Unmarshal(response.Body, &result)
	if len(result.Accounts) != 1 || result.Accounts[0].Credits != nil {
		t.Fatal("invalid account results")
	}
}

func TestAccountDisplayMetadata(t *testing.T) {
	sa := &storedAuth{Auth: storedTokens{AccessToken: "secret"}, Account: storedAccount{UID: "123", Nickname: "昵称", Email: "user@example.com", EnterpriseID: "org"}}
	auth := toAuthData(sa)
	if auth.Label != "workbuddy-cn 昵称" || auth.Metadata["email"] != "user@example.com" || !strings.Contains(auth.Metadata["note"].(string), "昵称") {
		t.Fatalf("missing account display: %+v", auth.Metadata)
	}
	delete(auth.Metadata, "email")
	sa.Account.Email = ""
	if _, ok := toAuthData(sa).Metadata["email"]; ok {
		t.Fatal("invented email")
	}
}

func TestLoginWaitsForAccountAndPreservesProfile(t *testing.T) {
	const state = "test-account-login"
	accountReady := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"accessToken":"secret","expiresIn":3600}}`
		if strings.Contains(r.URL.Path, "/login/account") {
			body = `{"code":0,"data":{"uid":"123","nickname":"Alice","email":"alice@example.com"}}`
			if !accountReady {
				body = `{"code":12}`
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	loginStates.Store(state, &loginCtx{client: client, expires: time.Now().Add(time.Minute)})
	t.Cleanup(func() { loginStates.Delete(state) })
	for _, ready := range []bool{false, true} {
		accountReady = ready
		raw, err := handlePollLogin([]byte(`{"state":"test-account-login"}`))
		if err != nil {
			t.Fatal(err)
		}
		var env envelope
		json.Unmarshal(raw, &env)
		var response pluginapi.AuthLoginPollResponse
		json.Unmarshal(env.Result, &response)
		if !ready {
			if response.Status != pluginapi.AuthLoginStatusPending || len(response.Auth.StorageJSON) > 0 {
				t.Fatal("saved incomplete account")
			}
		} else {
			if response.Status != pluginapi.AuthLoginStatusSuccess || response.Auth.Label != providerName+" Alice" {
				t.Fatal("missing login profile")
			}
			saved, err := parseStored(response.Auth.StorageJSON)
			if err != nil || saved.Account.Email != "alice@example.com" {
				t.Fatal("profile not persisted")
			}
		}
	}
}
