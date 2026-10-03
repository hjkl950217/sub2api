//go:build unit

// FORK: 二开「API 协议复选 + 兜底转发协议」的端到端转发验证（新增文件，非上游改动）。
// 与 fork_api_protocols_test.go 的单元测试互补：这里真的调用三个入站转发入口
// （Forward / ForwardAsChatCompletions / ForwardAsAnthropic），断言实际发出的
// 上游 URL 与请求体，证明目标里那三条语义确实生效：
//   1. 一个账号可以配置多个受支持的协议；
//   2. 入站协议在支持范围内 → 直接转发到该协议的原生端点（零转换）；
//   3. 入站协议不在支持范围内 → 才转成「兜底协议」转发。

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 用户主场景：一个 DeepSeek 账号同时支持 chat + messages，不支持 responses，兜底选 chat。
func forkMultiProtocolAccount() *Account {
	return &Account{
		ID:          901,
		Name:        "fork-multi-protocol-ds",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":           "sk-test",
			"account_mode":      AccountModePayG,
			"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolAnthropic},
			"fallback_protocol": APIProtocolChatCompletions,
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "http://chat.example",
				APIProtocolAnthropic:       "http://anthropic.example",
			},
		},
	}
}

func forkProtocolUpstream() (*httpUpstreamRecorder, *OpenAIGatewayService) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	return upstream, &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
}

// 语义 2：入站 chat_completions 在支持范围内 → 直接转发原生 CC 端点，零转换。
func TestForkE2EChatInboundDirectToNativeChat(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	_, err := svc.ForwardAsChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), forkMultiProtocolAccount(), body, "", "")
	require.Error(t, err)
	require.Equal(t, "http://chat.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").IsArray(), "CC 请求体应原样直通")
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
}

// 语义 2：入站 messages 在支持范围内 → 直接转发原生 Anthropic 端点，零转换。
func TestForkE2EMessagesInboundDirectToNativeAnthropic(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	body := []byte(`{"model":"deepseek-chat","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`)

	_, err := svc.ForwardAsAnthropic(context.Background(), adaptiveProtocolTestContext("/v1/messages", body), forkMultiProtocolAccount(), body, "", "")
	require.Error(t, err)
	require.Equal(t, "http://anthropic.example/v1/messages", upstream.lastReq.URL.String())
}

// 语义 3：入站 responses 不在支持范围内 → 才转成兜底协议（chat_completions）转发。
func TestForkE2EResponsesInboundFallsBackToChat(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	body := []byte(`{"model":"deepseek-chat","input":"hi","stream":false}`)

	_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), forkMultiProtocolAccount(), body)
	require.Error(t, err)
	require.Equal(t, "http://chat.example/v1/chat/completions", upstream.lastReq.URL.String(), "responses 未勾选 → 落到兜底 CC")
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").IsArray(), "应已转换成 CC 请求体")
}

// 语义 2 的边界：三个协议全勾选时 responses 入站直连原生 Responses 端点，不触发兜底。
func TestForkE2EResponsesInboundDirectWhenSelected(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	account := forkMultiProtocolAccount()
	account.Credentials["api_protocols"] = []any{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}
	account.Credentials["api_base_urls"] = map[string]any{
		APIProtocolChatCompletions: "http://chat.example",
		APIProtocolAnthropic:       "http://anthropic.example",
		APIProtocolResponses:       "http://responses.example",
	}
	body := []byte(`{"model":"deepseek-chat","input":"hi","stream":false}`)

	_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
	require.Error(t, err)
	require.Equal(t, "http://responses.example/responses", upstream.lastReq.URL.String(), "responses 已勾选 → 直连原生端点")
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
}

// 语义 3 变体：兜底选 anthropic 时，responses 入站转成 Anthropic 请求。
func TestForkE2EResponsesInboundFallsBackToAnthropic(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	account := forkMultiProtocolAccount()
	account.Credentials["fallback_protocol"] = APIProtocolAnthropic
	body := []byte(`{"model":"deepseek-chat","input":"hi","stream":false}`)

	_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
	require.Error(t, err)
	require.Equal(t, "http://anthropic.example/v1/messages", upstream.lastReq.URL.String(), "兜底 anthropic → 转成 Anthropic 请求")
}

// 语义 3 变体：兜底选 responses 时，messages 入站转成 Responses 请求。
func TestForkE2EMessagesInboundFallsBackToResponses(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	account := forkMultiProtocolAccount()
	account.Credentials["api_protocols"] = []any{APIProtocolChatCompletions, APIProtocolResponses}
	account.Credentials["fallback_protocol"] = APIProtocolResponses
	account.Credentials["api_base_urls"] = map[string]any{
		APIProtocolChatCompletions: "http://chat.example",
		APIProtocolResponses:       "http://responses.example",
	}
	body := []byte(`{"model":"deepseek-chat","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`)

	_, err := svc.ForwardAsAnthropic(context.Background(), adaptiveProtocolTestContext("/v1/messages", body), account, body, "", "")
	require.Error(t, err)
	require.Equal(t, "http://responses.example/responses", upstream.lastReq.URL.String(), "messages 未勾选 → 落到兜底 Responses")
}

// 语义 1 的负向边界：未配置 api_protocols 的旧账号，仍走原 api_protocol 逻辑。
func TestForkE2ELegacyAccountStillUsesOldProtocol(t *testing.T) {
	upstream, svc := forkProtocolUpstream()
	account := &Account{
		ID:          902,
		Name:        "legacy-ds",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions,
			"base_url":     "http://legacy-chat.example",
		},
	}
	body := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	_, err := svc.ForwardAsChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "", "")
	require.Error(t, err)
	require.Equal(t, "http://legacy-chat.example/v1/chat/completions", upstream.lastReq.URL.String(), "旧账号行为不变")
}
