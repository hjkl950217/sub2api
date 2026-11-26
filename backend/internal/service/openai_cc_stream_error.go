package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ccStreamUpstreamError preserves an error carried inside HTTP 200 SSE.
// Comments, role-only chunks and DONE are transport metadata, not model output.
type ccStreamUpstreamError struct {
	Payload []byte
	Message string
}

func (e *ccStreamUpstreamError) Error() string { return e.Message }

func parseCCStreamUpstreamError(payload string) *ccStreamUpstreamError {
	if !gjson.Valid(payload) {
		return nil
	}
	root := gjson.Parse(payload)
	typ := root.Get("type").String()
	err := root.Get("error")
	if (!err.Exists() || err.Type == gjson.Null) && typ != "response.failed" && typ != "error" {
		return nil
	}
	message := extractOpenAISSEErrorMessage([]byte(payload))
	if strings.TrimSpace(message) == "" {
		message = "Upstream returned an error inside HTTP 200"
	}
	return &ccStreamUpstreamError{Payload: []byte(payload), Message: message}
}

// ccStreamErrorIsDeterministic 判断流内错误是不是「换个账号也一样」的那种：
// 请求形状错误、上下文超限、内容策略拒绝。这类错误换号只会把同一个请求
// 在整池账号上重放一遍，所以即使还没给客户端任何内容也不换号。
func ccStreamErrorIsDeterministic(payload []byte, message string) bool {
	if hit, _, _ := detectOpenAICyberPolicy(payload); hit {
		return true
	}
	if isOpenAIContextWindowError(message, payload) {
		return true
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	if code == "invalid_encrypted_content" || code == "context_length_exceeded" {
		return true
	}
	typ := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	return typ == "invalid_request_error" || typ == "invalid_request"
}

func ccPayloadHasModelOutput(payload string) bool {
	for _, choice := range gjson.Get(payload, "choices").Array() {
		delta := choice.Get("delta")
		for _, field := range []string{"content", "reasoning_content", "reasoning", "refusal"} {
			if delta.Get(field).String() != "" {
				return true
			}
		}
		if len(delta.Get("tool_calls").Array()) > 0 || delta.Get("function_call").Exists() {
			return true
		}
	}
	return false
}
