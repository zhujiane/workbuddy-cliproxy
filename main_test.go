package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestAccountIdentity(t *testing.T) {
	a := &storedAuth{Account: storedAccount{UID: "account-a"}}
	b := &storedAuth{Account: storedAccount{UID: "account-b"}}
	first, second := toAuthData(a), toAuthData(b)
	if first.ID == second.ID || first.FileName == second.FileName {
		t.Fatal("different accounts collide")
	}
	again := toAuthData(&storedAuth{Account: storedAccount{UID: "account-a", Nickname: "new name"}})
	if first.ID != again.ID {
		t.Fatal("relogin changed account identity")
	}
	enterprise := toAuthData(&storedAuth{Account: storedAccount{UID: "account-a", EnterpriseID: "enterprise"}})
	if first.ID == enterprise.ID {
		t.Fatal("enterprise accounts collide")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLegacyParseAndRefresh(t *testing.T) {
	storage := []byte(`{"auth":{"accessToken":"old","refreshToken":"old-refresh"},"account":{"uid":"user"}}`)
	request, _ := json.Marshal(pluginapi.AuthParseRequest{FileName: "workbuddy.json", RawJSON: storage})
	raw, err := handleParseAuth(request)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	var parsed pluginapi.AuthParseResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(env.Result, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Handled || parsed.Auth.ID != "workbuddy.json" || parsed.Auth.FileName != "workbuddy.json" {
		t.Fatal("legacy file identity was not preserved")
	}
	client := sharedHTTPClient()
	previous := client.Transport
	t.Cleanup(func() { client.Transport = previous })
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"accessToken":"new","refreshToken":"new-refresh","expiresIn":3600}}`)), Header: make(http.Header)}, nil
	})
	request, _ = json.Marshal(pluginapi.AuthRefreshRequest{AuthID: parsed.Auth.ID, StorageJSON: parsed.Auth.StorageJSON})
	raw, err = handleRefreshAuth(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var refreshed pluginapi.AuthRefreshResponse
	if err := json.Unmarshal(env.Result, &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Auth.ID != parsed.Auth.ID || refreshed.Auth.FileName != "" {
		t.Fatal("refresh changed legacy identity or overrode the host filename")
	}
	sa, err := parseStored(refreshed.Auth.StorageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if sa.Auth.AccessToken != "new" {
		t.Fatal("token was not refreshed")
	}
}

func TestFallbackIdentitySurvivesRotation(t *testing.T) {
	a := &storedAuth{Auth: storedTokens{AccessToken: "access-a", RefreshToken: "refresh-a"}}
	first := toAuthData(a)
	b := &storedAuth{Auth: storedTokens{AccessToken: "access-b", RefreshToken: "refresh-b"}}
	if first.ID == toAuthData(b).ID {
		t.Fatal("accounts without UID collide")
	}
	var reloaded storedAuth
	if err := json.Unmarshal(first.StorageJSON, &reloaded); err != nil {
		t.Fatal(err)
	}
	reloaded.Auth.AccessToken = "rotated-access"
	reloaded.Auth.RefreshToken = "rotated-refresh"
	if toAuthData(&reloaded).ID != first.ID {
		t.Fatal("token rotation changed persisted identity")
	}
}

func TestDeepSeekV41FlashRegistered(t *testing.T) {
	for _, model := range wbModels() {
		if model.ID == "deepseek-v4.1-flash" {
			return
		}
	}
	t.Fatal("DeepSeek V4.1 Flash missing")
}

func TestEditionIsolation(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"provider":"workbuddy","auth":{"accessToken":"old"}}`, "workbuddy-cn"},
		{`{"auth":{"accessToken":"intl","domain":"www.codebuddy.ai"}}`, "workbuddy"},
		{`{"workbuddy_provider":"workbuddy","auth":{"accessToken":"intl"}}`, "workbuddy"},
	} {
		sa, err := parseStored([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		if storedProvider(sa) != tc.want {
			t.Fatalf("edition = %s, want %s", storedProvider(sa), tc.want)
		}
		request, _ := json.Marshal(pluginapi.AuthParseRequest{RawJSON: []byte(tc.raw)})
		raw, err := handleParseAuth(request)
		if err != nil {
			t.Fatal(err)
		}
		var env envelope
		json.Unmarshal(raw, &env)
		var response pluginapi.AuthParseResponse
		json.Unmarshal(env.Result, &response)
		if response.Handled != (tc.want == providerName) {
			t.Fatal("wrong plugin claimed credential")
		}
	}
	cn := toAuthData(&storedAuth{Provider: "workbuddy-cn", Account: storedAccount{UID: "same"}})
	intl := toAuthData(&storedAuth{Provider: "workbuddy", Account: storedAccount{UID: "same"}})
	if cn.ID == intl.ID || cn.Provider == intl.Provider {
		t.Fatal("cross-region identity collision")
	}
}
