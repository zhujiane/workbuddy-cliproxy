package main

import "strings"

// CPA exposes label and note on credential rows; email is only set when supplied
// by CodeBuddy, never filled with a nickname or an account ID.
func accountMetadata(sa *storedAuth) map[string]any {
	meta := map[string]any{"type": providerName, "provider": providerName}
	parts := []string{"WorkBuddy"}
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
