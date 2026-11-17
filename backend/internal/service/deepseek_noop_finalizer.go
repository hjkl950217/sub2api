// FORK: 2026-11-17 连续空命令收尾试点，仅账号 3351、请求模型 deepseek-flash。
package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

var deepseekNoopCommand = regexp.MustCompile(`(?i)^(?:echo|write-output)[\t ]+(?:"([a-z0-9.-]+)"|'([a-z0-9.-]+)'|([a-z0-9.-]+))$`)
var deepseekNoopNumber = regexp.MustCompile(`^[0-9]{1,6}$`)

func shouldFinalizeDeepSeekNoop(accountID int64, model string, messages []apicompat.AnthropicMessage) bool {
	// 2026-11-17：仅账号 3351、仅 deepseek-flash，目前只是试点。
	// 未来可能扩大处理范围，须先复核误判样本并另行确认，不自动扩展账号或模型。
	if accountID != 3351 || model != "deepseek-flash" || len(messages) < 4 {
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
	var text strings.Builder
	var call *apicompat.AnthropicContentBlock
	for i := range blocks {
		block := &blocks[i]
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
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
	if call == nil || !deepseekNoopClosingText(text.String()) {
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
	text = strings.ToLower(text)
	for _, marker := range []string{"结束", "完成", "收工", "无待办", "无待执行", "done", "finished"} {
		if strings.Contains(text, marker) {
			return true
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
