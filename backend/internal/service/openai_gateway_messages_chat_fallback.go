package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// forwardAnthropicViaRawChatCompletions serves /v1/messages clients through
// an OpenAI-compatible upstream that only supports /v1/chat/completions.
//
// Conversion chain (direct, no Responses intermediary):
//
//	Request:  Anthropic Messages → Chat Completions (AnthropicToChatCompletionsRequest)
//	Response: CC chunk/response → Anthropic events/response (direct bridge)
//
// This is the /v1/messages counterpart of forwardResponsesViaRawChatCompletions
// (which serves /v1/responses clients). Unlike the Responses path, the direct
// bridge skips the Responses API intermediate representation entirely — every
// streaming token runs through a single state machine instead of two.
func (s *OpenAIGatewayService) forwardAnthropicViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 1. Parse Anthropic request
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	originalModel := anthropicReq.Model
	if strings.TrimSpace(originalModel) == "" {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	applyOpenAICompatModelNormalization(&anthropicReq)
	clientStream := anthropicReq.Stream

	// 2. Anthropic → Chat Completions (direct, no Responses intermediary)
	chatReq, err := apicompat.AnthropicToChatCompletionsRequest(&anthropicReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("convert anthropic to chat completions: %w", err)
	}

	billingModel := resolveOpenAIForwardModel(account, anthropicReq.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if err := validateGPT61SolCompatRequest(body, upstreamModel); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	if openai.IsGPT61SolModelSpelling(upstreamModel) && len(anthropicReq.Tools) > 0 {
		err := fmt.Errorf("gpt-6.1-sol requires Responses for tool calls; this account only supports Chat Completions")
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatReq.Model = upstreamModel
	chatReq.ReasoningEffort = openAICompatAnthropicReasoningEffort(&anthropicReq, upstreamModel, chatReq.ReasoningEffort)
	chatReq.Stream = clientStream
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	serviceTier := extractOpenAIServiceTierFromBody(body)

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	if normalizedBody, normalized := NormalizeGLMOpenAIReasoningEffort(chatBody, upstreamModel); normalized {
		chatBody = normalizedBody
	}
	if account.Platform == PlatformOpenAI {
		policyBody, changed, policyErr := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, chatBody)
		if policyErr != nil {
			if IsReasoningEffortPolicyDenied(policyErr) {
				MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
				writeAnthropicError(c, http.StatusForbidden, "forbidden_error", policyErr.Error())
			}
			return nil, policyErr
		}
		if changed {
			chatBody = policyBody
		}
	}
	// Provider normalization and policy caps can both change the converted effort.
	// Use the final outbound value for usage logs and billing.
	reasoningEffort := extractOpenAIReasoningEffortFromBody(chatBody, upstreamModel, billingModel, originalModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, chatBody, upstreamModel)
	// Unlike forwardResponsesViaRawChatCompletions, applyOpenAIFastPolicyToBody
	// is intentionally skipped: Anthropic Messages bodies carry no service_tier,
	// so the converted Chat Completions body never contains one and the policy
	// would always be a no-op on this path.

	logger.L().Debug("openai messages: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	// 3. Build and send upstream request via the shared CC pipeline
	apiKey, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		return nil, err
	}
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, chatBody, clientStream, apiKey, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 4. Handle error responses
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		// Non-failover error: return Anthropic-formatted error to client via the
		// shared compat handler (passthrough rules, ops recording, cyber_policy).
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
	}

	// 5. Convert response
	if clientStream {
		return s.streamChatCompletionsAsAnthropic(c, resp, account, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	return s.bufferChatCompletionsAsAnthropic(c, resp, account, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

// FORK-ANCHOR: messages-cc-empty-completion-guard（二开：/v1/messages→CC 回退链路的上游空正文判定）
//
// 上游返回 200 但整条流没有任何正文 / 工具调用 / 推理内容时，按上游异常处理：本次请求记为
// 错误请求并换号重试。2026-10-10 长空报的 sharellm站-免费（账号 3310）就是这种情况 ——
// 用量表里 input_tokens 有值、output_tokens=0，客户端只见 message_start 后直接结束。
//
// 不复用 openAIChatSilentRefusalDetector.IsSilentRefusal()：那个判据要求整条流里没有 usage，
// 而中转站通常仍会回一个 completion_tokens=0 的 usage 块，会被判成"有 usage"而漏掉。
func newChatStreamEmptyBodyDetector() *openAIChatSilentRefusalDetector {
	// 强制开启：本链路已固定 stream_options.include_usage=true，正常流必有 usage 块，
	// 空正文只可能是上游异常，不需要 detector 自带的 64KB 请求体门控。
	detector := newOpenAIChatSilentRefusalDetector(openAISilentRefusalMinRequestBodyBytes)
	detector.enabled = true
	return detector
}

// chatStreamProducedAssistantOutput 报告上游是否已经产出过可交付给客户端的内容。
// 判据里刻意不含 usage：只有 usage 的流对客户端来说同样是空回复。
func chatStreamProducedAssistantOutput(d *openAIChatSilentRefusalDetector) bool {
	if d == nil {
		return true
	}
	return d.sawContent || d.sawToolCall || d.sawFunctionCall || d.sawReasoning || d.sawError
}

func (s *OpenAIGatewayService) bufferChatCompletionsAsAnthropic(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, resp, writeAnthropicError)
	if err != nil {
		return nil, err
	}
	anthropicResp := apicompat.ChatCompletionsResponseToAnthropic(ccResp, originalModel)
	// FORK-ANCHOR: messages-cc-empty-completion-nonstream（二开：非流式分支，上游 200 但正文为空
	// 同样按上游异常处理并换号；判据在 newChatStreamEmptyBodyDetector 上方注释里）
	if len(anthropicResp.Content) == 0 {
		return nil, newOpenAISilentRefusalFailoverError(c, account, requestID)
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, anthropicResp)

	return &OpenAIForwardResult{
		RequestID:                   requestID,
		UpstreamHeaders:             resp.Header,
		Usage:                       usage,
		Model:                       originalModel,
		BillingModel:                billingModel,
		UpstreamModel:               upstreamModel,
		ReasoningEffort:             reasoningEffort,
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
		ServiceTier:                 resolvedOpenAIUpstreamServiceTier(c, serviceTier),
		Stream:                      false,
		Duration:                    time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) streamChatCompletionsAsAnthropic(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	writeStreamHeaders := s.newStreamHeaderWriter(c, resp.Header)

	anthropicState := apicompat.NewChatCompletionsToAnthropicStreamState(originalModel)
	clientDisconnected := false

	// FORK-ANCHOR: messages-cc-empty-completion-prebuffer（二开：先确认上游有正文再放行客户端输出，
	// 否则空响应一旦写出响应头就无法换号，客户端只会看到半截空流。见上方 helper 注释）
	emptyBodyDetector := newChatStreamEmptyBodyDetector()
	pendingSSE := make([]string, 0, 16)
	clientOutputStarted := false

	// 与 responses 兄弟不同：客户端断开后仍继续做事件转换（喂 anthropicState），
	// 仅跳过写出，保证 finalize 阶段的 usage 汇总不受断开影响。
	emitChunk := func(chunk *apicompat.ChatCompletionsChunk) {
		emptyBodyDetector.ObserveChatChunk(*chunk)
		// CC chunk → Anthropic events (direct, single state machine)
		anthropicEvents := apicompat.ChatCompletionsChunkToAnthropicEvents(chunk, anthropicState)
		if clientDisconnected {
			return
		}
		var batch []string
		for _, aEvt := range anthropicEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(aEvt)
			if err != nil {
				continue
			}
			batch = append(batch, sse)
		}
		if len(batch) == 0 {
			return
		}
		if !clientOutputStarted && !chatStreamProducedAssistantOutput(emptyBodyDetector) {
			pendingSSE = append(pendingSSE, batch...)
			return
		}
		writeStreamHeaders()
		for _, sse := range pendingSSE {
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				return
			}
		}
		pendingSSE = pendingSSE[:0]
		for _, sse := range batch {
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				return
			}
		}
		clientOutputStarted = true
		c.Writer.Flush()
	}

	scan := s.scanCCStream(c, resp, "openai messages chat fallback", requestID, startTime, emitChunk)
	usage := scan.Usage

	if scan.Err != nil {
		// Broken upstream read: skip finalization so no synthetic message_stop
		// masks the truncation, and surface the error to flag usage incomplete
		// (mirrors forwardResponsesViaRawChatCompletions).
		return &OpenAIForwardResult{
			RequestID:                   requestID,
			UpstreamHeaders:             resp.Header,
			Usage:                       usage,
			Model:                       originalModel,
			BillingModel:                billingModel,
			UpstreamModel:               upstreamModel,
			ReasoningEffort:             reasoningEffort,
			UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
			ServiceTier:                 resolvedOpenAIUpstreamServiceTier(c, serviceTier),
			Stream:                      true,
			Duration:                    time.Since(startTime),
			FirstTokenMs:                scan.FirstTokenMs,
			ClientDisconnect:            clientDisconnected,
		}, fmt.Errorf("stream usage incomplete: %w", scan.Err)
	}

	// Finalize: close open blocks + emit message_delta/message_stop.
	finalEvents := apicompat.FinalizeChatCompletionsAnthropicStream(anthropicState)
	// FORK-ANCHOR: messages-cc-empty-completion-finalize（二开：整条流没有任何正文 → 记错误请求 + 换号）
	if !clientDisconnected && !clientOutputStarted && !chatStreamProducedAssistantOutput(emptyBodyDetector) {
		return nil, newOpenAISilentRefusalFailoverError(c, account, requestID)
	}
	if !clientDisconnected {
		if !clientOutputStarted && len(pendingSSE) > 0 {
			writeStreamHeaders()
			for _, sse := range pendingSSE {
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					break
				}
			}
			pendingSSE = pendingSSE[:0]
			clientOutputStarted = !clientDisconnected
		}
		for _, aEvt := range finalEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(aEvt)
			if err != nil {
				continue
			}
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
			}
		}
		c.Writer.Flush()
	}
	if !scan.SawDone {
		logCCStreamMissingDoneSentinel("openai messages chat fallback", requestID)
	}

	return &OpenAIForwardResult{
		RequestID:                   requestID,
		UpstreamHeaders:             resp.Header,
		Usage:                       usage,
		Model:                       originalModel,
		BillingModel:                billingModel,
		UpstreamModel:               upstreamModel,
		ReasoningEffort:             reasoningEffort,
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
		ServiceTier:                 resolvedOpenAIUpstreamServiceTier(c, serviceTier),
		Stream:                      true,
		Duration:                    time.Since(startTime),
		FirstTokenMs:                scan.FirstTokenMs,
		ClientDisconnect:            clientDisconnected,
	}, nil
}
