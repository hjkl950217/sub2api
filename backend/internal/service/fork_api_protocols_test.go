//go:build unit

// FORK: 二开「API 协议复选 + 兜底转发协议」的单元测试（新增文件，非上游改动）。
// 覆盖 account.go 的 fork-api-protocols-parse 解析层：勾选集合解析、兜底协议回落、
// 入站协议 → 出站协议的路由决策，以及未配置复选时旧行为的零回归。

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func forkProtocolAccount(platform string, creds map[string]any) *Account {
	merged := map[string]any{"api_key": "sk-test"}
	for k, v := range creds {
		merged[k] = v
	}
	return &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: merged}
}

// 未配置 api_protocols 时必须完全走旧的 api_protocol 逻辑。
func TestForkProtocolsLegacyFallback(t *testing.T) {
	t.Parallel()

	legacy := forkProtocolAccount(PlatformDeepseek, map[string]any{"api_protocol": APIProtocolAdaptive})
	require.Nil(t, legacy.GetSelectedAPIProtocols(), "未配置复选时勾选集合为 nil")
	require.False(t, legacy.HasExplicitAPIProtocols())
	require.Empty(t, legacy.GetFallbackAPIProtocol())
	require.Equal(t, "", legacy.ResolveAPIProtocolForInbound(APIProtocolChatCompletions), "未配置复选时不接管分流")
	require.True(t, legacy.SupportsAPIProtocol(APIProtocolResponses), "未配置复选时协议不受限")
	require.True(t, legacy.UsesNativeCNResponses(), "adaptive + deepseek 仍用原生 responses")
}

// 勾选集合按固定顺序输出并去重；zhipu 的 responses 被过滤。
func TestForkProtocolsSelectedSet(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols": []any{APIProtocolResponses, APIProtocolChatCompletions, APIProtocolChatCompletions},
	})
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, acc.GetSelectedAPIProtocols())
	require.True(t, acc.HasExplicitAPIProtocols())

	// 逗号串形态兼容
	comma := forkProtocolAccount(PlatformKimi, map[string]any{"api_protocols": "anthropic, chat_completions"})
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolAnthropic}, comma.GetSelectedAPIProtocols())

	// zhipu 不支持 responses：即便勾选也被剔除
	zhipu := forkProtocolAccount(PlatformZhipu, map[string]any{
		"api_protocols": []any{APIProtocolResponses},
	})
	require.Nil(t, zhipu.GetSelectedAPIProtocols(), "全部非法时视为未配置")

	// 全非法值 → 未启用复选
	bogus := forkProtocolAccount(PlatformKimi, map[string]any{"api_protocols": []any{"bogus"}})
	require.Nil(t, bogus.GetSelectedAPIProtocols())
}

// 兜底协议必须在勾选集合内，否则回落集合中第一个。
func TestForkProtocolsFallbackResolution(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols":     []any{APIProtocolAnthropic, APIProtocolChatCompletions},
		"fallback_protocol": APIProtocolAnthropic,
	})
	require.Equal(t, APIProtocolAnthropic, acc.GetFallbackAPIProtocol())

	// 兜底不在集合内 → 回落集合第一个（chat_completions 优先）
	invalid := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols":     []any{APIProtocolAnthropic, APIProtocolChatCompletions},
		"fallback_protocol": APIProtocolResponses,
	})
	require.Equal(t, APIProtocolChatCompletions, invalid.GetFallbackAPIProtocol())

	// 未配置兜底 → 同样回落
	missing := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols": []any{APIProtocolResponses, APIProtocolAnthropic},
	})
	require.Equal(t, APIProtocolAnthropic, missing.GetFallbackAPIProtocol())
}

// 用户主场景：勾选 chat + messages（anthropic），入站 responses 走兜底。
func TestForkProtocolsInboundRouting(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolAnthropic},
		"fallback_protocol": APIProtocolChatCompletions,
	})

	require.Equal(t, APIProtocolChatCompletions, acc.ResolveAPIProtocolForInbound(APIProtocolChatCompletions), "CC 入站零转换直通")
	require.Equal(t, APIProtocolAnthropic, acc.ResolveAPIProtocolForInbound(APIProtocolAnthropic), "messages 入站零转换直通")
	require.Equal(t, APIProtocolChatCompletions, acc.ResolveAPIProtocolForInbound(APIProtocolResponses), "responses 未勾选 → 兜底 CC")

	require.False(t, acc.SupportsAPIProtocol(APIProtocolResponses))
	require.False(t, acc.UsesNativeCNResponses(), "未勾选 responses 时不得走原生 Responses")

	// 三个协议都勾选时 responses 直通
	all := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses},
		"fallback_protocol": APIProtocolChatCompletions,
	})
	require.Equal(t, APIProtocolResponses, all.ResolveAPIProtocolForInbound(APIProtocolResponses))
	require.True(t, all.UsesNativeCNResponses())
}

// 端点地址按勾选集合解析：未勾选协议请求回落到兜底协议地址。
func TestForkProtocolsBaseURLResolution(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformDeepseek, map[string]any{
		"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolAnthropic},
		"fallback_protocol": APIProtocolChatCompletions,
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "https://chat.example.com",
			APIProtocolAnthropic:       "https://anthropic.example.com",
		},
	})
	require.Equal(t, "https://chat.example.com", acc.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://anthropic.example.com", acc.GetCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, "https://chat.example.com", acc.GetCNProtocolBaseURL(APIProtocolResponses), "未勾选 responses → 回落兜底 CC 地址")
	require.Equal(t, "https://anthropic.example.com", acc.GetAnthropicProtocolBaseURL())
	require.Equal(t, "https://chat.example.com", acc.GetOpenAIBaseURL())
}

// 只勾选 anthropic 时，Anthropic 端点可用、OpenAI 格式端点回落平台默认 CC。
func TestForkProtocolsAnthropicOnly(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformKimi, map[string]any{
		"api_protocols":     []any{APIProtocolAnthropic},
		"fallback_protocol": APIProtocolAnthropic,
		"api_base_urls": map[string]any{
			APIProtocolAnthropic: "https://anthropic.example.com",
		},
	})
	require.Equal(t, "https://anthropic.example.com", acc.GetAnthropicProtocolBaseURL())
	require.Equal(t, DefaultKimiPayGBaseURL, acc.GetOpenAIFormatBaseURL(), "未勾选 CC → OpenAI 格式端点用平台默认 CC 地址")
	require.Equal(t, "https://anthropic.example.com", acc.GetOpenAIBaseURL(), "兜底为 anthropic 时 base_url 取 anthropic 端点")
}
