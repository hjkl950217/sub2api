//go:build unit

// FORK: 验证 2026-11-17 账号 3351/deepseek-flash 连续空转收尾试点。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekNoopFinalizerForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		accountID      int64
		model          string
		command        string
		stream         bool
		wantNone       bool
		withoutClosing bool
		closingText    string
		newUser        bool
		onlyOneRound   bool
		mixedTool      bool
		resultError    bool
		resultID       string
		resultText     string
	}{
		{name: "two completed echo rounds", wantNone: true},
		{name: "streaming completed echo rounds", stream: true, wantNone: true},
		{name: "quoted Write-Output", command: `Write-Output "final"`, resultText: "final", wantNone: true},
		{name: "single quoted Bash echo", command: `echo 'done'`, resultText: "done", wantNone: true},
		{name: "another account", accountID: 3352},
		{name: "another model", model: "glm-5.3-flash"},
		{name: "new user instruction", newUser: true},
		{name: "one completed echo round", onlyOneRound: true},
		{name: "no completed reply", withoutClosing: true},
		{name: "unfinished task", closingText: "任务尚未完成，先验证输出2。"},
		{name: "task not ended", closingText: "任务还未结束，接着验证输出2。"},
		{name: "future completion", closingText: "需要完成后续诊断，继续验证输出2。"},
		{name: "mixed useful tool", mixedTool: true},
		{name: "tool failed", resultError: true},
		{name: "unmatched result", resultID: "call_unrelated"},
		{name: "result contains new information", resultText: "new diagnostic information"},
		{name: "pipeline", command: "echo OK | Measure-Object"},
		{name: "semicolon", command: "echo OK; Get-Location"},
		{name: "newline", command: "echo\nOK"},
		{name: "redirection", command: "echo OK > marker.txt"},
		{name: "variable", command: "echo $env:USERNAME"},
		{name: "substitution", command: "echo $(Get-Location)"},
		{name: "meaningful output", command: "echo diagnostic-details"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := deepseekNoopHistoryForTest(t)
			if tt.command != "" || tt.withoutClosing || tt.closingText != "" || tt.mixedTool {
				var blocks []apicompat.AnthropicContentBlock
				require.NoError(t, json.Unmarshal(messages[3].Content, &blocks))
				if tt.command != "" {
					blocks[1].Input = deepseekNoopJSONForTest(t, map[string]string{"command": tt.command, "description": "结束"})
					if strings.Contains(tt.command, "'done'") {
						blocks[1].Name = "Bash"
					}
				}
				if tt.withoutClosing {
					blocks[0].Text = "继续获取下一项诊断证据。"
				}
				if tt.closingText != "" {
					blocks[0].Text = tt.closingText
				}
				if tt.mixedTool {
					blocks = append(blocks, apicompat.AnthropicContentBlock{
						Type: "tool_use", ID: "call_read", Name: "Read", Input: json.RawMessage(`{"file_path":"report.md"}`),
					})
				}
				messages[3].Content = deepseekNoopJSONForTest(t, blocks)
			}
			var results []apicompat.AnthropicContentBlock
			require.NoError(t, json.Unmarshal(messages[4].Content, &results))
			results[0].IsError = tt.resultError
			if tt.resultID != "" {
				results[0].ToolUseID = tt.resultID
			}
			if tt.resultText != "" {
				results[0].Content = deepseekNoopJSONForTest(t, tt.resultText)
			}
			if tt.mixedTool {
				results = append(results, apicompat.AnthropicContentBlock{
					Type: "tool_result", ToolUseID: "call_read", Content: json.RawMessage(`"useful report"`),
				})
			}
			if tt.newUser {
				results = append(results, apicompat.AnthropicContentBlock{Type: "text", Text: "继续，检查下一个文件。"})
			}
			messages[4].Content = deepseekNoopJSONForTest(t, results)
			if tt.onlyOneRound {
				messages = messages[:3]
			}

			model := tt.model
			if model == "" {
				model = "deepseek-flash"
			}
			accountID := tt.accountID
			if accountID == 0 {
				accountID = 3351
			}
			body := deepseekNoopJSONForTest(t, map[string]any{
				"model": model, "max_tokens": 128, "stream": tt.stream, "messages": messages,
				"tools": []map[string]any{
					{"name": "PowerShell", "input_schema": map[string]any{"type": "object"}},
					{"name": "Bash", "input_schema": map[string]any{"type": "object"}},
					{"name": "Read", "input_schema": map[string]any{"type": "object"}},
				},
				"tool_choice": map[string]string{"type": "auto"},
			})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			contentType := "application/json"
			responseBody := `{"id":"chatcmpl_noop","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"任务结束。"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`
			if tt.stream {
				contentType = "text/event-stream"
				responseBody = "data: " + `{"id":"chatcmpl_noop","object":"chat.completion.chunk","model":"deepseek-flash","choices":[{"index":0,"delta":{"content":"任务结束。"},"finish_reason":null}]}` + "\n\ndata: " + `{"id":"chatcmpl_noop","object":"chat.completion.chunk","model":"deepseek-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}}
			account := rawChatCompletionsTestAccount()
			account.ID = accountID
			account.Platform = "ag_hub"
			account.Credentials["model_mapping"] = map[string]any{model: "cn:" + model}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, err := svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, body, "")
			require.NoError(t, err)
			wantChoice := "auto"
			if tt.wantNone {
				wantChoice = "none"
			}
			require.Equal(t, wantChoice, gjson.GetBytes(upstream.lastBody, "tool_choice").String())
			require.Equal(t, "cn:"+model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "测试通过后汇报结果并结束。", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
			require.Contains(t, string(upstream.lastBody), "call_echo_1")
			require.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}

func deepseekNoopHistoryForTest(t *testing.T) []apicompat.AnthropicMessage {
	t.Helper()
	messages := []apicompat.AnthropicMessage{{Role: "user", Content: json.RawMessage(`"测试通过后汇报结果并结束。"`)}}
	for n := 1; n <= 2; n++ {
		callID := fmt.Sprintf("call_echo_%d", n)
		messages = append(messages,
			apicompat.AnthropicMessage{Role: "assistant", Content: deepseekNoopJSONForTest(t, []apicompat.AnthropicContentBlock{
				{Type: "text", Text: "测试通过，本轮结束。\n长空，以上是我的回复"},
				{Type: "tool_use", ID: callID, Name: "PowerShell", Input: deepseekNoopJSONForTest(t, map[string]string{
					"command": fmt.Sprintf("echo %d", n), "description": "结束",
				})},
			})},
			apicompat.AnthropicMessage{Role: "user", Content: deepseekNoopJSONForTest(t, []apicompat.AnthropicContentBlock{
				{Type: "tool_result", ToolUseID: callID, Content: deepseekNoopJSONForTest(t, fmt.Sprint(n))},
			})},
		)
	}
	return messages
}

func deepseekNoopJSONForTest(t *testing.T, value any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}
