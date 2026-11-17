//go:build unit

// FORK: 验证 2026-11-17 账号 3351 的 DeepSeek 三个确认名称共享同一模型映射。
package service

import "testing"

func TestDeepSeek3351ModelAliasesUseCanonicalMapping(t *testing.T) {
	mapping := map[string]any{"deepseek-flash": "cn:deepseek-v4.1-flash"}
	for _, model := range []string{
		"deepseek-flash",
		"deepseek-v4.1-flash",
		"deepseek-v4.1-flash-1017",
	} {
		t.Run(model, func(t *testing.T) {
			account := &Account{
				ID:       3351,
				Platform: "ag_hub",
				Credentials: map[string]any{
					"model_mapping": mapping,
				},
			}
			if !account.IsModelSupported(model) {
				t.Fatalf("IsModelSupported(%q) = false, want true", model)
			}
			if got := account.GetMappedModel(model); got != "cn:deepseek-v4.1-flash" {
				t.Fatalf("GetMappedModel(%q) = %q, want canonical upstream model", model, got)
			}
		})
	}
}

func TestDeepSeek3351ModelAliasesStayScoped(t *testing.T) {
	credentials := map[string]any{
		"model_mapping": map[string]any{"deepseek-flash": "cn:deepseek-v4.1-flash"},
	}
	for _, tc := range []struct {
		name      string
		account   int64
		model     string
		supported bool
	}{
		{name: "other account", account: 3352, model: "deepseek-v4.1-flash", supported: false},
		{name: "unapproved alias", account: 3351, model: "deepseek-v4.1-flash-sg", supported: false},
		{name: "long context suffix", account: 3351, model: "deepseek-v4.1-flash[1M]", supported: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: tc.account, Platform: "ag_hub", Credentials: credentials}
			if got := account.IsModelSupported(tc.model); got != tc.supported {
				t.Fatalf("IsModelSupported(%q) = %v, want %v", tc.model, got, tc.supported)
			}
			if got := account.GetMappedModel(tc.model); got != tc.model {
				t.Fatalf("GetMappedModel(%q) = %q, want original model", tc.model, got)
			}
		})
	}
}
