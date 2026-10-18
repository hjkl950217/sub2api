package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func headerValueFold(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// 上游按客户端身份拦截时，指纹请求必须带 Claude Code 身份头（账号 3304 实测：
// 只带协议头会 401）。这里锁住 anthropic 分支复用的是单账号测试那套头。
func TestFingerprintExtraHeadersAnthropicUsesClaudeIdentity(t *testing.T) {
	account := &Account{Platform: PlatformAggregateHub, Type: AccountTypeAPIKey}

	headers := fingerprintExtraHeaders(account, "message")

	if ua := headerValueFold(headers, "User-Agent"); !strings.HasPrefix(ua, "claude-cli/") {
		t.Fatalf("User-Agent = %q, want claude-cli/ prefix", ua)
	}
	if got := headerValueFold(headers, "X-App"); got != "cli" {
		t.Fatalf("X-App = %q, want cli", got)
	}
	if got := headerValueFold(headers, "anthropic-beta"); got != claude.APIKeyBetaHeader {
		t.Fatalf("anthropic-beta = %q, want %q", got, claude.APIKeyBetaHeader)
	}
}

// responses 分支对齐单账号测试的 Codex 探测身份头。
func TestFingerprintExtraHeadersResponsesUsesCodexIdentity(t *testing.T) {
	account := &Account{Platform: PlatformAggregateHub, Type: AccountTypeAPIKey}

	headers := fingerprintExtraHeaders(account, "responses")

	if got := headerValueFold(headers, "originator"); got == "" {
		t.Fatal("originator header missing")
	}
	if got := headerValueFold(headers, "OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q, want responses=experimental", got)
	}
}

// 账号的请求头覆写最后生效，与真实转发优先级一致。
func TestFingerprintExtraHeadersAccountOverridesWin(t *testing.T) {
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"user-agent": "relay-check"},
		},
	}

	headers := fingerprintExtraHeaders(account, "message")

	if got := headerValueFold(headers, "User-Agent"); got != "relay-check" {
		t.Fatalf("User-Agent = %q, want account override relay-check", got)
	}
}
