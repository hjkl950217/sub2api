//go:build unit

// FORK: 二开「测试时带上工具调用」的单测。
// 覆盖：请求体确实附了 echo 工具并强制调用；响应里没有工具调用时该协议判失败。
package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func toolCallChatTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"message\":\"测试\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`)),
	}
}

func toolCallResponsesTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"response.output_item.added","item":{"type":"function_call","name":"echo"}}

data: {"type":"response.completed"}

`)),
	}
}

// 带工具调用的测试：请求里必须附 tool_choice，响应里有工具调用才算通过。
func TestForkAccountTestWithToolsPassesWhenToolCallSeen(t *testing.T) {
	account := forkOpenAIMatrixTestAccount(920)
	svc, upstream, _ := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return toolCallChatTestResponse() },
		nil,
		func() *http.Response { return toolCallResponsesTestResponse() },
	), true)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-4o", "hello", AccountTestModeDefault, AccountTestOptions{WithTools: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	for _, request := range upstream.requests {
		body := protocolMatrixRequestBody(t, request)
		require.Contains(t, body, `"tool_choice"`, "带工具测试必须强制工具调用")
		require.Contains(t, body, accountTestToolName)
	}
	body := recorder.Body.String()
	require.Equal(t, 3, strings.Count(body, `"type":"protocol_result"`))
	require.NotContains(t, body, `"protocol_ok":false`, "两个协议都返回了工具调用")
}

// 上游返回纯文本、没有工具调用时必须判失败，并把原因带给前端。
func TestForkAccountTestWithToolsFailsWithoutToolCall(t *testing.T) {
	account := forkOpenAIMatrixTestAccount(921)
	svc, _, _ := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"纯文本"},"finish_reason":"stop"}]}

data: [DONE]

`)),
			}
		},
		nil,
		func() *http.Response { return toolCallResponsesTestResponse() },
	), true)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "gpt-4o", "hello", AccountTestModeDefault, AccountTestOptions{WithTools: true})

	require.NoError(t, err)
	body := recorder.Body.String()
	require.Contains(t, body, `"protocol":"chat_completions","protocol_ok":false`)
	require.Contains(t, body, "工具调用")
	require.Contains(t, body, `"protocol":"responses","protocol_ok":true`)
}

// 工具调用的默认测试消息与普通默认消息不同。
func TestForkAccountToolTestPayloadDefaults(t *testing.T) {
	chatPayload := createOpenAIChatCompletionsTestPayloadTools("gpt-4o", "", true)
	messages, _ := chatPayload["messages"].([]map[string]any)
	require.Equal(t, defaultAccountToolTestPrompt, messages[0]["content"])

	anthropicPayload, err := createTestPayloadWithTools("claude-test", "", true)
	require.NoError(t, err)
	require.Contains(t, anthropicPayload, "tools")
	require.Contains(t, anthropicPayload, "tool_choice")

	responsesPayload := createOpenAITestPayloadWithPromptTools("gpt-4o", false, "", true)
	require.Contains(t, responsesPayload, "tools")
	require.Contains(t, responsesPayload, "tool_choice")
}
