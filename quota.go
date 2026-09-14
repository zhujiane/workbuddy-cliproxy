package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

//go:embed quota.html
var quotaPanel []byte

const quotaPath = "/v0/management/plugins/workbuddy/credits"
const panelPath = "/v0/resource/plugins/workbuddy/panel"

type quotaPackage struct {
	Name       string `json:"name"`
	Remain     int64  `json:"remain"`
	Used       int64  `json:"used"`
	Size       int64  `json:"size"`
	CycleStart string `json:"cycle_start"`
	CycleEnd   string `json:"cycle_end"`
}
type quotaSummary struct {
	TotalRemain int64          `json:"total_remain"`
	TotalUsed   int64          `json:"total_used"`
	TotalSize   int64          `json:"total_size"`
	Packages    []quotaPackage `json:"packages"`
	FetchedAt   time.Time      `json:"fetched_at"`
	Scope       string         `json:"scope"`
}
type resourcePackage struct {
	PackageName                                               string
	CapacityRemain, CapacityUsed, CapacitySize                int64
	CycleCapacityRemain, CycleCapacityUsed, CycleCapacitySize *int64
	CycleStartTime, CycleEndTime                              string
}

func summarizePackage(p resourcePackage) quotaPackage {
	remain, used, size := p.CapacityRemain, p.CapacityUsed, p.CapacitySize
	if p.CycleCapacityRemain != nil || p.CycleCapacityUsed != nil || p.CycleCapacitySize != nil {
		remain, used, size = 0, 0, 0
		if p.CycleCapacityRemain != nil {
			remain = *p.CycleCapacityRemain
		}
		if p.CycleCapacityUsed != nil {
			used = *p.CycleCapacityUsed
		}
		if p.CycleCapacitySize != nil {
			size = *p.CycleCapacitySize
		}
		if p.CycleCapacityRemain == nil && size > 0 {
			remain = size - used
		}
	}
	remain, used = max(0, remain), max(0, used)
	size = max(size, remain+used)
	used = max(used, size-remain)
	return quotaPackage{p.PackageName, remain, used, size, p.CycleStartTime, p.CycleEndTime}
}

// Query personal resource packages. This endpoint does not establish enterprise pooled quota.
func fetchQuota(sa *storedAuth) (*quotaSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Isolate cookies and disallow redirects so credentials only reach the billing host.
	client := &http.Client{Transport: sharedHTTPClient().Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	sum := &quotaSummary{Packages: []quotaPackage{}, Scope: "personal_resource_packages"}
	for page := 1; page <= 100; page++ {
		payload, _ := json.Marshal(map[string]any{"PageNumber": page, "PageSize": 100, "ProductCode": "p_tcaca", "Status": []int{0, 3}, "PackageEndTimeRangeBegin": now.Format("2006-01-02 15:04:05"), "PackageEndTimeRangeEnd": now.AddDate(101, 0, 0).Format("2006-01-02 15:04:05")})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, accountOrigin(sa)+"/v2/billing/meter/get-user-resource", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		commonHeaders(req)
		req.Header.Set("Origin", accountOrigin(sa))
		req.Header.Set("Referer", accountOrigin(sa)+"/")
		req.Header.Set("Authorization", "Bearer "+sa.Auth.AccessToken)
		if sa.Account.UID != "" {
			req.Header.Set("X-User-Id", sa.Account.UID)
		}
		if sa.Account.EnterpriseID != "" {
			req.Header.Set("X-Enterprise-Id", sa.Account.EnterpriseID)
			req.Header.Set("X-Tenant-Id", sa.Account.EnterpriseID)
		}
		if sa.Auth.Domain != "" {
			req.Header.Set("X-Domain", sa.Auth.Domain)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("billing request failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("billing HTTP %d", resp.StatusCode)
		}
		if readErr != nil || len(raw) > 4*1024*1024 {
			return nil, fmt.Errorf("invalid billing response body")
		}
		var env struct {
			Code *int `json:"code"`
			Data struct {
				Response struct {
					Data *struct {
						TotalCount *int
						Accounts   []resourcePackage
					}
				}
			} `json:"data"`
		}
		if json.Unmarshal(raw, &env) != nil || env.Code == nil {
			return nil, fmt.Errorf("invalid billing response")
		}
		if *env.Code != 0 {
			return nil, fmt.Errorf("billing code=%d", *env.Code)
		}
		data := env.Data.Response.Data
		if data == nil || data.TotalCount == nil || *data.TotalCount < 0 {
			return nil, fmt.Errorf("missing billing resource data")
		}
		for _, p := range data.Accounts {
			q := summarizePackage(p)
			sum.Packages = append(sum.Packages, q)
			sum.TotalRemain += q.Remain
			sum.TotalUsed += q.Used
			sum.TotalSize += q.Size
		}
		if len(sum.Packages) >= *data.TotalCount {
			sum.FetchedAt = time.Now().UTC()
			return sum, nil
		}
		if len(data.Accounts) == 0 {
			return nil, fmt.Errorf("incomplete billing resource pages")
		}
	}
	return nil, fmt.Errorf("too many billing resource pages")
}

func quotaRegistration() any {
	if providerName == "workbuddy-cn" {
		return struct{}{}
	}
	return struct {
		Routes    []pluginapi.ManagementRoute `json:"routes"`
		Resources []pluginapi.ResourceRoute   `json:"resources"`
	}{
		Routes:    []pluginapi.ManagementRoute{{Method: http.MethodGet, Path: "/plugins/workbuddy/credits", Description: "Query WorkBuddy resource credits"}},
		Resources: []pluginapi.ResourceRoute{{Path: "/panel", Description: "查看账号积分和资源包"}},
	}
}

type quotaAccount struct {
	Provider     string        `json:"provider"`
	Nickname     string        `json:"nickname,omitempty"`
	UID          string        `json:"uid,omitempty"`
	Email        string        `json:"email,omitempty"`
	EnterpriseID string        `json:"enterprise_id,omitempty"`
	AuthIndex    string        `json:"auth_index"`
	Name         string        `json:"name"`
	Disabled     bool          `json:"disabled"`
	Credits      *quotaSummary `json:"credits,omitempty"`
	Error        string        `json:"error,omitempty"`
}

func quotaHostCall(call func(string, []byte) ([]byte, error), method, callbackID, index string, result any) error {
	body, _ := json.Marshal(map[string]string{"host_callback_id": callbackID, "auth_index": index})
	raw, err := call(method, body)
	if err != nil {
		return fmt.Errorf("host credential lookup failed")
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || !env.OK {
		return fmt.Errorf("host credential lookup rejected")
	}
	if err = json.Unmarshal(env.Result, result); err != nil {
		return fmt.Errorf("invalid host credential response")
	}
	return nil
}

func handleQuotaManagement(raw []byte) ([]byte, error) {
	return handleQuotaManagementWithHost(raw, hostCall)
}
func handleQuotaManagementWithHost(raw []byte, call func(string, []byte) ([]byte, error)) ([]byte, error) {
	var req struct {
		pluginapi.ManagementRequest
		HostCallbackID string `json:"host_callback_id"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	respond := func(status int, value any) ([]byte, error) {
		body, _ := json.Marshal(value)
		return okEnvelope(pluginapi.ManagementResponse{StatusCode: status, Headers: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}}, Body: body})
	}
	if req.Method != http.MethodGet {
		return respond(405, map[string]string{"error": "method not allowed"})
	}
	if req.Path == panelPath {
		return okEnvelope(pluginapi.ManagementResponse{StatusCode: 200, Headers: http.Header{"Content-Type": {"text/html; charset=utf-8"}, "Cache-Control": {"no-store"}}, Body: quotaPanel})
	}
	if req.Path != quotaPath {
		return respond(404, map[string]string{"error": "not found"})
	}
	var listing struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if err := quotaHostCall(call, "host.auth.list", req.HostCallbackID, "", &listing); err != nil {
		return respond(502, map[string]string{"error": err.Error()})
	}
	accounts := []quotaAccount{}
	index := strings.TrimSpace(req.Query.Get("auth_index"))
	for _, f := range listing.Files {
		if (firstNonEmpty(f.Provider, f.Type) != "workbuddy" && firstNonEmpty(f.Provider, f.Type) != "workbuddy-cn") || (index != "" && index != f.AuthIndex) {
			continue
		}
		a := quotaAccount{AuthIndex: f.AuthIndex, Name: firstNonEmpty(f.Label, f.Name), Disabled: f.Disabled}
		var credential struct {
			JSON json.RawMessage `json:"json"`
		}
		err := quotaHostCall(call, "host.auth.get", req.HostCallbackID, f.AuthIndex, &credential)
		if err == nil {
			var sa *storedAuth
			sa, err = parseStored(credential.JSON)
			if err != nil {
				err = fmt.Errorf("invalid stored credential")
			} else {
				a.Provider = storedProvider(sa)
				a.Nickname, a.UID, a.Email, a.EnterpriseID = sa.Account.Nickname, sa.Account.UID, sa.Account.Email, sa.Account.EnterpriseID
				a.Name = firstNonEmpty(a.Nickname, a.Email, a.Name, a.UID)
				a.Credits, err = fetchQuota(sa)
			}
		}
		if err != nil {
			a.Error = err.Error()
		}
		accounts = append(accounts, a)
	}
	if index != "" && len(accounts) == 0 {
		return respond(404, map[string]string{"error": "WorkBuddy account not found"})
	}
	return respond(200, map[string]any{"accounts": accounts})
}
