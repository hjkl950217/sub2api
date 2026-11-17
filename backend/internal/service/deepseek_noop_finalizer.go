// FORK: 2026-11-17 连续空命令收尾试点，仅账号 3351、deepseek-flash 及用户确认的两个别名。
package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const deepseekNoopFinalizerContextKey = "fork.deepseek3351.noopFinalizing"
const deepseekNoopFinalizerNotice = "检测到连续空命令，已停止本轮工具调用。"

var deepseekNoopCommand = regexp.MustCompile(`(?i)^(?:echo|write-output)[\t ]+(?:"([a-z0-9.-]+)"|'([a-z0-9.-]+)'|([a-z0-9.-]+))$`)
var deepseekNoopNumber = regexp.MustCompile(`^[0-9]{1,6}$`)

func shouldFinalizeDeepSeekNoop(accountID int64, model string, messages []apicompat.AnthropicMessage) bool {
	// 2026-11-17：仅账号 3351、仅下列三个同模型名称，目前只是试点。
	// 未来可能扩大处理范围，须先复核误判样本并另行确认，不自动扩展账号或模型。
	if accountID != 3351 || len(messages) < 4 {
		return false
	}
	switch model {
	case "deepseek-flash", "deepseek-v4.1-flash", "deepseek-v4.1-flash-1017":
	default:
		return false
	}
	for i := len(messages) - 4; i < len(messages); i += 2 {
		if !completedDeepSeekNoopRound(messages[i], messages[i+1]) {
			return false
		}
	}
	return true
}

func completedDeepSeekNoopRound(assistant, result apicompat.AnthropicMessage) bool {
	if assistant.Role != "assistant" || result.Role != "user" {
		return false
	}
	var blocks []apicompat.AnthropicContentBlock
	if json.Unmarshal(assistant.Content, &blocks) != nil {
		return false
	}
	var text string
	var call *apicompat.AnthropicContentBlock
	for i := range blocks {
		block := &blocks[i]
		switch block.Type {
		case "text":
			text += block.Text
		case "thinking", "redacted_thinking":
		case "tool_use":
			if call != nil || block.ID == "" || (block.Name != "Bash" && block.Name != "PowerShell") {
				return false
			}
			call = block
		default:
			return false
		}
	}
	if call == nil || !deepseekNoopClosingText(text) {
		return false
	}
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(call.Input, &input) != nil {
		return false
	}
	literal, ok := deepseekNoopLiteral(input.Command)
	if !ok {
		return false
	}

	var results []apicompat.AnthropicContentBlock
	if json.Unmarshal(result.Content, &results) != nil || len(results) != 1 {
		return false
	}
	r := results[0]
	if r.Type != "tool_result" || r.IsError || r.ToolUseID != call.ID {
		return false
	}
	var output string
	if json.Unmarshal(r.Content, &output) != nil {
		var parts []apicompat.AnthropicContentBlock
		if json.Unmarshal(r.Content, &parts) != nil || len(parts) != 1 || parts[0].Type != "text" {
			return false
		}
		output = parts[0].Text
	}
	return strings.TrimSpace(output) == literal
}

func deepseekNoopClosingText(text string) bool {
	clauses := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return strings.ContainsRune("，,。.!！?？；;\n\r", r)
	})
	for _, clause := range clauses {
		clause = strings.Trim(clause, " \t*#：:")
		switch clause {
		case "结束", "完成", "收工", "停止操作", "无待办", "无待执行", "done", "finished":
			return true
		case "本轮结束", "本轮已结束", "本次结束", "本次已结束", "本轮完成", "本轮已完成", "本次完成", "本次已完成":
			return true
		case "任务结束", "任务已结束", "任务完成", "任务已完成", "工作结束", "工作已结束", "工作完成", "工作已完成":
			return true
		case "已完成", "全部完成", "所有收尾完成", "今天签到结束", "今天签到已全部结束":
			return true
		}
	}
	return false
}

// 只由本次请求的试点命中标记启用，不根据响应模型或账号重新推断。
func finalizeDeepSeekNoopResponse(response *apicompat.ChatCompletionsResponse) {
	for i := range response.Choices {
		choice := &response.Choices[i]
		hadTool := len(choice.Message.ToolCalls) > 0 || choice.Message.FunctionCall != nil
		choice.Message.ToolCalls = nil
		choice.Message.FunctionCall = nil
		choice.FinishReason = deepseekNoopFinishReason(choice.FinishReason)
		if hadTool && choice.FinishReason == "stop" && !deepseekNoopContentHasText(choice.Message.Content) {
			choice.Message.Content = json.RawMessage(`"` + deepseekNoopFinalizerNotice + `"`)
		}
	}
}

type deepseekNoopStreamFilter struct {
	sawTool bool
	sawText bool
}

func (f *deepseekNoopStreamFilter) filter(chunk *apicompat.ChatCompletionsChunk) {
	for i := range chunk.Choices {
		choice := &chunk.Choices[i]
		f.sawTool = f.sawTool || len(choice.Delta.ToolCalls) > 0
		choice.Delta.ToolCalls = nil
		if choice.Delta.Content != nil && strings.TrimSpace(*choice.Delta.Content) != "" {
			f.sawText = true
		}
		if choice.FinishReason != nil {
			finish := deepseekNoopFinishReason(*choice.FinishReason)
			choice.FinishReason = &finish
			if finish == "stop" && f.sawTool && !f.sawText {
				notice := deepseekNoopFinalizerNotice
				choice.Delta.Content = &notice
				f.sawText = true
			}
		}
	}
}

func deepseekNoopFinishReason(reason string) string {
	if reason == "tool_calls" || reason == "function_call" {
		return "stop"
	}
	return reason
}

func deepseekNoopContentHasText(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return strings.TrimSpace(text) != ""
	}
	var parts []apicompat.ChatContentPart
	if json.Unmarshal(content, &parts) == nil {
		for _, part := range parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	return false
}

func deepseekNoopLiteral(command string) (string, bool) {
	match := deepseekNoopCommand.FindStringSubmatch(strings.TrimSpace(command))
	if match == nil {
		return "", false
	}
	var literal string
	for _, value := range match[1:] {
		if value != "" {
			literal = value
			break
		}
	}
	switch strings.ToLower(literal) {
	case "ok", "done", "final", "no-op", "stop", "end", "fin", "x", ".":
		return literal, true
	default:
		return literal, deepseekNoopNumber.MatchString(literal)
	}
}
