package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type upstreamError struct {
	status  int
	message string
}

func (e *upstreamError) Error() string { return e.message }

func upstreamEventError(data string) error {
	var event map[string]json.RawMessage
	if json.Unmarshal([]byte(data), &event) != nil {
		return nil
	}
	if value, ok := event["error"]; ok && string(value) != "null" {
		return &upstreamError{502, "upstream stream error: " + data}
	}
	var code int
	if json.Unmarshal(event["code"], &code) == nil && code != 0 {
		return &upstreamError{502, "upstream stream error: " + data}
	}
	return nil
}

// Retry only before receiving a successful response. Replaying a partially
// delivered completion can duplicate tool calls, so stream reads never retry.
func openUpstream(req *http.Request) (*http.Response, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req = req.Clone(req.Context())
			req.Body = body
		}
		// The shared client also serves bounded login/management requests. Chat
		// completions must not inherit its whole-response 120 second deadline.
		client := *sharedHTTPClient()
		client.Timeout = 0
		resp, err := client.Do(req)
		if err != nil {
			last = &upstreamError{http.StatusBadGateway, fmt.Sprintf("http_error: %v", err)}
			if req.GetBody == nil || req.Context().Err() != nil {
				return nil, last
			}
			continue
		}
		if resp.StatusCode < 400 {
			return resp, nil
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		resp.Body.Close()
		message := fmt.Sprintf("upstream %d: %s", resp.StatusCode, string(body))
		if readErr != nil {
			message += fmt.Sprintf(" (read error: %v)", readErr)
		}
		last = &upstreamError{resp.StatusCode, message}
		if req.GetBody == nil || (resp.StatusCode != 502 && resp.StatusCode != 503 && resp.StatusCode != 504) {
			return nil, last
		}
	}
	return nil, last
}
