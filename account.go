package main

import "strings"

// CPA exposes label and note on credential rows; email is only set when supplied
// by CodeBuddy, never filled with a nickname or an account ID.
func accountMetadata(sa *storedAuth) map[string]any {
	meta := map[string]any{"type": storedProvider(sa), "provider": storedProvider(sa)}
	parts := []string{storedProvider(sa)}
	for _, field := range []struct{ key, value, prefix string }{
		{"nickname", sa.Account.Nickname, ""},
		{"email", sa.Account.Email, ""},
		{"uid", sa.Account.UID, "UID: "},
		{"enterprise_id", sa.Account.EnterpriseID, "企业: "},
	} {
		if value := strings.TrimSpace(field.value); value != "" {
			meta[field.key] = value
			parts = append(parts, field.prefix+value)
		}
	}
	meta["note"] = strings.Join(parts, " · ")
	return meta
}

func storedProvider(sa *storedAuth) string {
	if sa == nil {
		return ""
	}
	if sa.Provider == "workbuddy-cn" || sa.Provider == "workbuddy" {
		return sa.Provider
	}
	if strings.Contains(sa.Auth.Domain, "codebuddy.ai") {
		return "workbuddy"
	}
	return "workbuddy-cn"
}
func accountOrigin(sa *storedAuth) string {
	if storedProvider(sa) == "workbuddy" {
		return "https://www.codebuddy.ai"
	}
	return "https://www.codebuddy.cn"
}
