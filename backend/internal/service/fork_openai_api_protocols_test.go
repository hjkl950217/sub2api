//go:build unit

// FORK: 二开「openai API Key 协议复选」的单元测试（新增文件，非上游改动）。
// 覆盖 openai 平台的协议复选门控、入站分流决定、端点回落，以及未配置复选时
// 继续走 extra.openai_responses_mode 的零回归。

package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

// forkOpenAIMatrixTestAccount 造一个未配置协议复选的 openai API Key 账号，
// 用于验证普通测试也会走两协议矩阵。
func forkOpenAIMatrixTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "fork-openai-matrix",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-openai-matrix",
			"base_url": "http://openai.example",
		},
	}
}

// 未配置 api_protocols 的 openai 账号必须完全走旧逻辑（extra.openai_responses_mode）。
func TestForkOpenAIProtocolsLegacyFallback(t *testing.T) {
	t.Parallel()

	legacy := forkProtocolAccount(PlatformOpenAI, map[string]any{"base_url": "https://relay.example.com"})
	require.False(t, legacy.IsMultiProtocolAPIKey(), "未配置复选时 openai 不进入多协议路径")
	require.Nil(t, legacy.GetSelectedAPIProtocols())
	require.False(t, legacy.HasExplicitAPIProtocols())
	require.Equal(t, "", legacy.ResolveAPIProtocolForInbound(APIProtocolChatCompletions))
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(legacy), "未探测标记 → 保持旧行为（不转 raw chat）")

	// 旧字段仍生效：强制 chat 时转 raw chat
	forcedChat := forkProtocolAccount(PlatformOpenAI, map[string]any{"base_url": "https://relay.example.com"})
	forcedChat.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(forcedChat))
}

// 配置复选后 openai 按勾选集合决定 responses 是否转 raw chat。
func TestForkOpenAIProtocolsSelection(t *testing.T) {
	t.Parallel()

	both := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"base_url":          "https://relay.example.com/v1",
		"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolResponses},
		"fallback_protocol": APIProtocolChatCompletions,
	})
	require.True(t, both.IsMultiProtocolAPIKey())
	require.True(t, both.HasExplicitAPIProtocols())
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, both.GetSelectedAPIProtocols())
	require.Equal(t, APIProtocolChatCompletions, both.GetFallbackAPIProtocol())
	require.True(t, both.SupportsNativeCNResponses(), "openai API Key 视为具备原生 responses 端点")
	require.True(t, both.UsesNativeCNResponses())
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(both), "两个协议都支持 → responses 保留原生")

	// 只勾 chat → 入站 responses 转 chat 直转
	chatOnly := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"base_url":          "https://relay.example.com/v1",
		"api_protocols":     []any{APIProtocolChatCompletions},
		"fallback_protocol": APIProtocolChatCompletions,
	})
	require.Equal(t, APIProtocolChatCompletions, chatOnly.ResolveAPIProtocolForInbound(APIProtocolResponses))
	require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(chatOnly))

	// 只勾 responses → 保留原生 responses
	responsesOnly := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"base_url":          "https://relay.example.com/v1",
		"api_protocols":     []any{APIProtocolResponses},
		"fallback_protocol": APIProtocolResponses,
	})
	require.Equal(t, []string{APIProtocolResponses}, responsesOnly.GetSelectedAPIProtocols())
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(responsesOnly))

	// 勾选集合只认两个协议：anthropic 被剔除，全非法视为未配置
	withAnthropic := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"base_url":      "https://relay.example.com/v1",
		"api_protocols": []any{APIProtocolAnthropic},
	})
	require.Nil(t, withAnthropic.GetSelectedAPIProtocols(), "openai 不支持 anthropic 协议")
}

// 未写 api_base_urls 时，两个协议都回落账号自己的 base_url。
func TestForkOpenAIProtocolsBaseURL(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"base_url":          "https://relay.example.com/v1",
		"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolResponses},
		"fallback_protocol": APIProtocolChatCompletions,
	})
	require.Equal(t, "https://relay.example.com/v1", acc.GetOpenAIBaseURL())
	require.Equal(t, "https://relay.example.com/v1", acc.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://relay.example.com/v1", acc.GetCNProtocolBaseURL(APIProtocolResponses))
}

// 门控边界：OAuth 不进入复选路径；openai 复选账号不参与 CN 余额/额度与分组级豁免。
func TestForkOpenAIProtocolsGating(t *testing.T) {
	t.Parallel()

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"api_protocols": []any{APIProtocolChatCompletions, APIProtocolResponses},
	}}
	require.False(t, oauth.IsMultiProtocolAPIKey())
	require.False(t, oauth.HasExplicitAPIProtocols())
	require.False(t, oauth.SupportsNativeCNResponses(), "openai OAuth 不视为 CN 原生 responses 账号")

	relay := forkProtocolAccount(PlatformOpenAI, map[string]any{
		"api_key":       "sk-relay",
		"base_url":      "https://relay.example.com/v1",
		"api_protocols": []any{APIProtocolChatCompletions, APIProtocolResponses},
	})
	require.Empty(t, relay.GetCNAPIKey(), "openai 不参与 CN 余额/额度探测")
	require.False(t, relay.IsCNProvider())
	require.Equal(t, "sk-relay", relay.GetOpenAIProtocolAPIKey())
	require.False(t, IsMultiProtocolAPIKeyProvider(PlatformOpenAI), "分组级判定保持不含 openai")
}

// 探测结论到 extra.openai_responses_mode 的映射。
func TestForkOpenAIResponsesModeFromProbedProtocols(t *testing.T) {
	t.Parallel()

	require.Equal(t, openai_compat.ResponsesSupportModeForceResponses,
		openAIResponsesModeFromProbedProtocols([]string{APIProtocolChatCompletions, APIProtocolResponses}))
	require.Equal(t, openai_compat.ResponsesSupportModeForceResponses,
		openAIResponsesModeFromProbedProtocols([]string{APIProtocolResponses}))
	require.Equal(t, openai_compat.ResponsesSupportModeForceChatCompletions,
		openAIResponsesModeFromProbedProtocols([]string{APIProtocolChatCompletions}))
}

// 「更新支持协议」的写回：协议集合与兜底写入 credentials，base_url 保持账号原值。
func TestForkOpenAIProbedProtocolsWriteback(t *testing.T) {
	t.Parallel()

	acc := forkProtocolAccount(PlatformOpenAI, map[string]any{"base_url": "https://relay.example.com/v1"})
	require.True(t, setProbedCNProtocols(acc, []string{APIProtocolChatCompletions, APIProtocolResponses}))
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, acc.Credentials["api_protocols"])
	require.Equal(t, APIProtocolChatCompletions, acc.Credentials["fallback_protocol"])
	require.Equal(t, APIProtocolAdaptive, acc.Credentials["api_protocol"])
	require.Equal(t, "https://relay.example.com/v1", acc.Credentials["base_url"])
	require.True(t, acc.HasExplicitAPIProtocols())
}

// 普通连接测试（非「更新支持协议」）在 openai API Key 账号上也跑两协议矩阵：
// 逐协议上报结果，但不改动账号配置 —— 与 DeepSeek 分组账号的行为一致。
func TestForkOpenAIProtocolsPlainTestProbesBothProtocols(t *testing.T) {
	account := forkOpenAIMatrixTestAccount(910)
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return adaptiveCNChatTestResponse() },
		nil,
		func() *http.Response { return adaptiveCNResponsesTestResponse() },
	))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-4o", "hello", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2, "chat 与 responses 各探测一次")
	require.Nil(t, repo.updated, "普通测试不得回写协议配置")
	require.Zero(t, repo.setErrorCalls, "单协议失败不得把账号状态改成 error")

	body := recorder.Body.String()
	require.Equal(t, 2, strings.Count(body, `"type":"protocol_result"`), "每个协议各一条结果事件")
	require.Contains(t, body, `"protocol":"chat_completions","protocol_ok":true`)
	require.Contains(t, body, `"protocol":"responses","protocol_ok":true`)
	require.NotContains(t, body, `"protocol_applied":true`)
}

// 「更新支持协议」在 openai 账号上按探测结论回写：只写通的协议。
func TestForkOpenAIProtocolsSyncTestWritesBackPassedOnly(t *testing.T) {
	account := forkOpenAIMatrixTestAccount(911)
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return adaptiveCNChatTestResponse() },
		nil,
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"no responses endpoint"}`) },
	))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-4o", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.NotNil(t, repo.updated, "显式同步时必须回写")
	require.Equal(t, []string{APIProtocolChatCompletions}, repo.updated.Credentials["api_protocols"])
	require.Equal(t, APIProtocolChatCompletions, repo.updated.Credentials["fallback_protocol"])

	body := recorder.Body.String()
	require.Contains(t, body, `"protocol":"responses","protocol_ok":false`)
	require.Contains(t, body, `"protocol_applied":true`)
	require.NotContains(t, body, `"type":"error"`, "单协议失败不产生终止错误事件")
}
