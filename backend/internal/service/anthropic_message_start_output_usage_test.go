//go:build unit

package service

import "testing"

// 上游可能在 message_start 已上报 output_tokens，若流在 message_delta 前中断，
// 两条解析路径都必须保留这部分已观测输出，否则会少计费。
func TestMessageStartOutputUsagePreserved(t *testing.T) {
	const start = `{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":8,"cache_read_input_tokens":1000}}}`
	const delta = `{"type":"message_delta","usage":{"output_tokens":12}}`

	t.Run("standard", func(t *testing.T) {
		usage := &ClaudeUsage{}
		svc := &GatewayService{}
		svc.parseSSEUsage(start, usage)
		if usage.OutputTokens != 8 {
			t.Fatalf("message_start output tokens = %d, want 8", usage.OutputTokens)
		}
		svc.parseSSEUsage(delta, usage)
		if usage.OutputTokens != 12 {
			t.Fatalf("message_delta output tokens = %d, want 12", usage.OutputTokens)
		}
	})

	t.Run("passthrough", func(t *testing.T) {
		usage := &ClaudeUsage{}
		parseSSEUsagePassthrough(start, usage)
		if usage.OutputTokens != 8 {
			t.Fatalf("message_start output tokens = %d, want 8", usage.OutputTokens)
		}
		parseSSEUsagePassthrough(delta, usage)
		if usage.OutputTokens != 12 {
			t.Fatalf("message_delta output tokens = %d, want 12", usage.OutputTokens)
		}
	})
}
