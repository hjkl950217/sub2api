//go:build unit

// FORK: 二开「协议探测矩阵 + 一键更新支持协议」的单测。
// 覆盖三条语义：逐协议探测、按通过结果回写勾选集合、全部失败时保持原配置。
package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// protocolSyncTestRepo 记录 Update 收到的账号，用于断言回写结果。
type protocolSyncTestRepo struct {
	openAIAccountTestRepo
	updated *Account
}

func (r *protocolSyncTestRepo) Update(_ context.Context, account *Account) error {
	r.updated = account
	return nil
}

func protocolSyncTestAccount(id int64, platform string, protocols []any) *Account {
	return &Account{
		ID:          id,
		Name:        "protocol-sync-test",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":           "sk-protocol-sync",
			"api_protocol":      APIProtocolAdaptive,
			"api_protocols":     protocols,
			"fallback_protocol": APIProtocolChatCompletions,
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "http://chat.example/v1",
				APIProtocolAnthropic:       "http://anthropic.example",
				APIProtocolResponses:       "http://responses.example",
			},
		},
	}
}

func protocolSyncTestService(account *Account, responses ...*http.Response) (*AccountTestService, *httpUpstreamRecorder, *protocolSyncTestRepo) {
	repo := &protocolSyncTestRepo{
		openAIAccountTestRepo: openAIAccountTestRepo{
			mockAccountRepoForGemini: mockAccountRepoForGemini{
				accountsByID: map[int64]*Account{account.ID: account},
			},
		},
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream, repo
}

// 语义：三个协议各测一次，测通的写回 api_protocols，兜底取 chat_completions 优先。
func TestForkProtocolSyncMatrixAppliesPassedProtocols(t *testing.T) {
	account := protocolSyncTestAccount(901, PlatformKimi, []any{
		APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses,
	})
	svc, upstream, repo := protocolSyncTestService(
		account,
		adaptiveCNChatTestResponse(),
		newJSONResponse(http.StatusBadRequest, `{"error":"no anthropic endpoint"}`),
		adaptiveCNResponsesTestResponse(),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "kimi-k2", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3, "三个协议各探测一次")
	require.Equal(t, "http://chat.example/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, "http://anthropic.example/v1/messages", upstream.requests[1].URL.String())
	require.Equal(t, "http://responses.example/v1/responses", upstream.requests[2].URL.String())

	require.NotNil(t, repo.updated, "探测结果必须回写账号")
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, repo.updated.Credentials["api_protocols"])
	require.Equal(t, APIProtocolChatCompletions, repo.updated.Credentials["fallback_protocol"])

	body := recorder.Body.String()
	require.Equal(t, 3, strings.Count(body, `"type":"protocol_result"`), "每个协议各一条结果事件")
	require.Contains(t, body, `"protocol":"anthropic","protocol_ok":false`)
	require.Contains(t, body, `"protocol":"responses","protocol_ok":true`)
	require.Contains(t, body, `"protocol_applied":true`)
	require.Equal(t, 1, strings.Count(body, `"type":"test_complete"`), "内层探针的 test_complete 不得冒泡")
	// 单个协议失败不再作为终止错误抛出（否则前端会显示测试失败）。
	require.NotContains(t, body, `"type":"error"`)
}

// 语义：三个协议全失败时不改动账号配置，并明确告知未回写。
func TestForkProtocolSyncMatrixAllFailedKeepsConfig(t *testing.T) {
	account := protocolSyncTestAccount(902, PlatformDeepseek, []any{APIProtocolAnthropic})
	svc, upstream, repo := protocolSyncTestService(
		account,
		newJSONResponse(http.StatusBadRequest, `{"error":"chat down"}`),
		newJSONResponse(http.StatusBadRequest, `{"error":"anthropic down"}`),
		newJSONResponse(http.StatusBadRequest, `{"error":"responses down"}`),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-chat", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Nil(t, repo.updated, "全部失败时不得写库")
	require.Equal(t, []any{APIProtocolAnthropic}, account.Credentials["api_protocols"], "原勾选集合保持不变")

	body := recorder.Body.String()
	require.NotContains(t, body, `"protocol_applied":true`, "未回写时不得声称已更新")
	// success 带 omitempty：false 时不出现该字段，前端按"非 true 即失败"处理。
	require.Contains(t, body, `"type":"test_complete"`, "必须发最终完成事件")
	require.NotContains(t, body, `"success":true`, "全失败时不得报告成功")
	require.Equal(t, 3, strings.Count(body, `"type":"protocol_result"`))
	require.Equal(t, 3, strings.Count(body, `"protocol_ok":false`))
	require.NotContains(t, body, `"type":"error"`, "单协议失败不产生终止错误事件")
}

// 语义：平台没有原生 Responses 端点时跳过该协议，不发请求。
func TestForkProtocolSyncMatrixSkipsUnsupportedResponses(t *testing.T) {
	account := protocolSyncTestAccount(903, PlatformZhipu, []any{
		APIProtocolChatCompletions, APIProtocolAnthropic,
	})
	svc, upstream, repo := protocolSyncTestService(
		account,
		adaptiveCNChatTestResponse(),
		adaptiveCNAnthropicTestResponse(),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2, "zhipu 无原生 Responses，不发第三个请求")
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolAnthropic}, repo.updated.Credentials["api_protocols"])
	body := recorder.Body.String()
	require.Contains(t, body, `"protocol":"responses","protocol_ok":false`)
	require.Contains(t, body, `"protocol_applied":true`)
}
