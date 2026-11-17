//go:build unit

// FORK: 验证 2026-11-17 账号 3351/deepseek-flash 连续空转收尾试点。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		{name: "DeepSeek flash alias", model: "deepseek-v4.1-flash", wantNone: true},
		{name: "DeepSeek flash dated alias", model: "deepseek-v4.1-flash-1017", stream: true, wantNone: true},
		{name: "alias on another account", accountID: 3352, model: "deepseek-v4.1-flash-1017"},
		{name: "unapproved model alias", model: "deepseek-v4.1-flash-sg"},
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

func TestDeepSeekNoopFinalizerResponseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		stream     bool
		onlyTool   bool
		newUser    bool
		otherID    bool
		legacy     bool
		readError  bool
		finish     string
		wantTool   bool
		wantNotice bool
	}{
		{name: "buffered ignores upstream tool", finish: "tool_calls"},
		{name: "stream ignores fragmented tool", stream: true, finish: "tool_calls"},
		{name: "buffered tool only has notice", onlyTool: true, finish: "tool_calls", wantNotice: true},
		{name: "stream tool only has notice", stream: true, onlyTool: true, finish: "tool_calls", wantNotice: true},
		{name: "buffered legacy function", legacy: true, onlyTool: true, finish: "function_call", wantNotice: true},
		{name: "stop cannot carry tool", stream: true, finish: "stop"},
		{name: "buffered length preserved", finish: "length"},
		{name: "stream length preserved", stream: true, finish: "length"},
		{name: "buffered filter preserved", finish: "content_filter"},
		{name: "stream filter preserved", stream: true, finish: "content_filter"},
		{name: "broken stream not completed", stream: true, onlyTool: true, readError: true},
		{name: "new user restores buffered tool", newUser: true, finish: "tool_calls", wantTool: true},
		{name: "new user restores stream tool", stream: true, newUser: true, finish: "tool_calls", wantTool: true},
		{name: "other account retains tool", otherID: true, finish: "tool_calls", wantTool: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := deepseekNoopHistoryForTest(t)
			if tt.newUser {
				messages = append(messages, apicompat.AnthropicMessage{Role: "user", Content: json.RawMessage(`"新任务：请检查当前目录。"`)})
			}
			body := deepseekNoopJSONForTest(t, map[string]any{
				"model": "deepseek-flash", "max_tokens": 128, "stream": tt.stream, "messages": messages,
				"tools":       []map[string]any{{"name": "PowerShell", "input_schema": map[string]any{"type": "object"}}},
				"tool_choice": map[string]string{"type": "auto"},
			})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			text := "测试通过，本轮结束。"
			if tt.onlyTool {
				text = ""
			}
			const tool = `{"index":0,"id":"call_unwanted","type":"function","function":{"name":"PowerShell","arguments":"{\"command\":\"echo 5\"}"}}`
			message := map[string]any{"role": "assistant", "content": text, "reasoning_content": "核验已结束。"}
			if tt.legacy {
				message["function_call"] = map[string]string{"name": "PowerShell", "arguments": `{"command":"echo 5"}`}
			} else {
				message["tool_calls"] = json.RawMessage("[" + tool + "]")
			}
			responseBody := string(deepseekNoopJSONForTest(t, map[string]any{
				"id": "chatcmpl_none_contract", "object": "chat.completion", "model": "deepseek-flash",
				"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": tt.finish}},
				"usage":   map[string]int{"prompt_tokens": 8, "completion_tokens": 7, "total_tokens": 15},
			}))
			contentType := "application/json"
			if tt.stream {
				contentType = "text/event-stream"
				responseBody = "data: " + string(deepseekNoopJSONForTest(t, map[string]any{
					"id": "chatcmpl_none_contract", "object": "chat.completion.chunk", "model": "deepseek-flash",
					"choices": []map[string]any{{"index": 0, "delta": map[string]any{
						"role": "assistant", "content": text, "reasoning_content": "核验已结束。",
						"tool_calls": json.RawMessage("[" + tool + "]"),
					}}},
				})) + "\n\n"
				if !tt.readError {
					responseBody += "data: " + `{"id":"chatcmpl_none_contract","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" "}}]},"finish_reason":null}]}` + "\n\n"
					responseBody += "data: " + string(deepseekNoopJSONForTest(t, map[string]any{
						"id": "chatcmpl_none_contract", "choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": tt.finish}},
					})) + "\n\ndata: " + `{"id":"chatcmpl_none_contract","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":7,"total_tokens":15}}` + "\n\ndata: [DONE]\n\n"
				}
			}
			var responseReader io.ReadCloser = io.NopCloser(strings.NewReader(responseBody))
			if tt.readError {
				responseReader = &errTailReader{data: []byte(responseBody), err: errors.New("diagnostic upstream disconnect")}
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: responseReader,
			}}
			account := rawChatCompletionsTestAccount()
			account.ID = 3351
			account.Platform = "ag_hub"
			if tt.otherID {
				account.ID = 3352
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, body, "")
			output := recorder.Body.String()
			if tt.readError {
				require.ErrorContains(t, err, "diagnostic upstream disconnect")
				require.NotContains(t, output, "message_stop")
				require.NotContains(t, output, `"type":"tool_use"`)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 7, result.Usage.OutputTokens)
			if tt.wantTool {
				require.Contains(t, output, `"type":"tool_use"`)
				require.Contains(t, output, `"stop_reason":"tool_use"`)
				require.Equal(t, "auto", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
				return
			}
			require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
			require.NotContains(t, output, `"type":"tool_use"`)
			require.NotContains(t, output, "echo 5")
			require.Contains(t, output, "核验已结束。")
			wantStop := "end_turn"
			if tt.finish == "length" {
				wantStop = "max_tokens"
			}
			require.Contains(t, output, `"stop_reason":"`+wantStop+`"`)
			if tt.wantNotice {
				require.Contains(t, output, "检测到连续空命令，已停止本轮工具调用。")
			} else {
				require.Contains(t, output, text)
				require.NotContains(t, output, "检测到连续空命令")
			}
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
