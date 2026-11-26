package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

const accountTestSuppressCompletionContextKey = "account_test_suppress_completion"

// testCNProviderAdaptiveConnection verifies every native endpoint used by an
// adaptive account of a provider that routes by inbound protocol. Zhipu uses
// Chat Completions plus Anthropic; DeepSeek and Kimi additionally use their
// native Responses endpoints. Endpoints the provider profile does not offer are
// skipped.
// FORK-ANCHOR: cn-adaptive-prompt-routing (国产供应商自适应协议探测传递弹窗提示词)
// FORK-ANCHOR: cn-adaptive-concurrent-probe (二开：各原生端点并发探测，不再逐个串行)
func (s *AccountTestService) testCNProviderAdaptiveConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	requestedModelID := strings.TrimSpace(modelID)
	if requestedModelID == "" {
		requestedModelID = account.providerDefaultTestModel()
	}
	if requestedModelID == "" {
		requestedModelID = openai.DefaultTestModel
	}
	testModelID := account.GetMappedModel(requestedModelID)

	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}

	// 各端点并发探测，中间 completion 事件全部抑制，由本函数统一收尾。
	protocols := []string{APIProtocolChatCompletions}
	if account.providerSupportsProtocol(APIProtocolAnthropic) {
		protocols = append(protocols, APIProtocolAnthropic)
	}
	if account.SupportsNativeCNResponses() {
		protocols = append(protocols, APIProtocolResponses)
	}
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	_, failed := s.probeProtocolsConcurrently(c, protocols, account, func(probeContext *gin.Context, probeAccount *Account, protocol string) error {
		switch protocol {
		case APIProtocolAnthropic:
			return s.testCNProviderAdaptiveAnthropicConnection(probeContext, probeAccount, testModelID, authToken, prompt)
		case APIProtocolResponses:
			return s.testCNProviderAdaptiveResponsesConnection(probeContext, probeAccount, testModelID, authToken, prompt)
		default:
			return s.testCNProviderChatCompletionsConnection(probeContext, probeAccount, requestedModelID, prompt)
		}
	})
	if len(failed) > 0 {
		// 保持原来的失败语义：任一端点失败即整体失败。内层已抑制 error 事件，这里补发一条。
		return s.sendErrorAndEnd(c, failed[0].Error())
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// FORK-ANCHOR: cn-adaptive-anthropic-probe-base-url (二开：探测用该协议自己的地址，不因未勾选改道)
func (s *AccountTestService) testCNProviderAdaptiveAnthropicConnection(c *gin.Context, account *Account, testModelID string, authToken string, prompt string) error {
	ctx := c.Request.Context()
	baseURL, err := s.validateUpstreamBaseURL(account.GetCNProtocolBaseURLForProbe(APIProtocolAnthropic))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid adaptive Anthropic base URL: %s", err.Error()))
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/v1/messages"

	// FORK-ANCHOR: cn-adaptive-anthropic-prompt (国产供应商 Anthropic 探测使用弹窗提示词)
	payload, err := createTestPayloadWithTools(testModelID, prompt, accountTestToolsEnabled(c))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create adaptive Anthropic test payload")
	}
	payloadBytes, _ := json.Marshal(payload)

	s.sendEvent(c, TestEvent{Type: "status", Text: "正在通过原生 /v1/messages 测试自适应 Anthropic 端点"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create adaptive Anthropic request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	for key, value := range claude.DefaultHeaders() {
		req.Header.Set(key, value)
	}
	req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
	// Ollama Cloud Anthropic 兼容端点按 adaptive 实际选用的 Anthropic
	// base_url 强制 Bearer，其余保持 extra/default 行为。
	setAnthropicAPIKeyAuthHeader(req.Header, account, authToken, account.GetCNProtocolBaseURLForProbe(APIProtocolAnthropic))
	applyOpenCodeUpstreamUserAgent(account, apiURL, req.Header)
	account.ApplyHeaderOverrides(req.Context(), req.Header)
	applyOpenCodeSessionHeader(c, account, apiURL, req.Header, payloadBytes)

	resp, err := s.doCNProviderAdaptiveRequest(req, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Adaptive Anthropic endpoint request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Adaptive Anthropic endpoint returned %d: %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, errMsg)
	}

	if err := s.processCNProviderAdaptiveAnthropicStream(c, resp.Body); err != nil {
		return err
	}
	s.sendEvent(c, TestEvent{Type: "status", Text: "已通过原生 /v1/messages 验证"})
	return nil
}

func (s *AccountTestService) processCNProviderAdaptiveAnthropicStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return s.sendErrorAndEnd(c, "Adaptive Anthropic stream ended before message_stop")
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Adaptive Anthropic stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}
		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			return s.ensureAccountTestToolCall(c)
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		switch eventType, _ := data["type"].(string); eventType {
		case "content_block_start":
			// FORK-ANCHOR: account-test-tool-call-adaptive-stream (二开：adaptive Anthropic 流里的工具调用识别)
			if block, ok := data["content_block"].(map[string]any); ok && block["type"] == "tool_use" {
				markAccountTestToolCall(c)
			}
		case "content_block_delta":
			if delta, ok := data["delta"].(map[string]any); ok {
				if deltaType, _ := delta["type"].(string); deltaType == "input_json_delta" {
					markAccountTestToolCall(c)
				}
				if text, ok := delta["text"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
		case "message_stop":
			return s.ensureAccountTestToolCall(c)
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if message, ok := errData["message"].(string); ok && message != "" {
					errorMsg = message
				}
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Adaptive Anthropic endpoint error: %s", errorMsg))
		}
	}
}

func (s *AccountTestService) testCNProviderAdaptiveResponsesConnection(c *gin.Context, account *Account, testModelID string, authToken string, prompt string) error {
	ctx := c.Request.Context()
	// FORK-ANCHOR: cn-adaptive-responses-probe-base-url (二开：探测用该协议自己的地址，不因未勾选改道)
	baseURL, err := s.validateUpstreamBaseURL(account.GetCNProtocolBaseURLForProbe(APIProtocolResponses))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid adaptive Responses base URL: %s", err.Error()))
	}
	apiURL := buildOpenAIResponsesURLForPlatform(account.Platform, baseURL)

	// FORK-ANCHOR: cn-adaptive-responses-prompt (国产供应商 Responses 探测使用弹窗提示词)
	payload := createOpenAITestPayloadWithPromptTools(testModelID, false, prompt, accountTestToolsEnabled(c))
	// DeepSeek / Kimi native Responses endpoints are stateless and do not need
	// the OpenAI probe's synthetic instructions.
	delete(payload, "instructions")
	payloadBytes, _ := json.Marshal(payload)
	payloadBytes = normalizeDeepSeekResponsesRequestBody(account, payloadBytes)

	s.sendEvent(c, TestEvent{Type: "status", Text: "正在通过原生 /responses 测试自适应 Responses 端点"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create adaptive Responses request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+authToken)
	applyOpenAICodexProbeHeaders(req.Header)
	applyOpenCodeUpstreamUserAgent(account, apiURL, req.Header)
	account.ApplyHeaderOverrides(req.Context(), req.Header)
	applyOpenCodeSessionHeader(c, account, apiURL, req.Header, payloadBytes)

	resp, err := s.doCNProviderAdaptiveRequest(req, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Adaptive Responses endpoint request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Adaptive Responses endpoint returned %d: %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, errMsg)
	}

	if err := s.processOpenAIStream(c, resp.Body); err != nil {
		return err
	}
	s.sendEvent(c, TestEvent{Type: "status", Text: "已通过原生 /responses 验证"})
	return nil
}

func (s *AccountTestService) doCNProviderAdaptiveRequest(req *http.Request, account *Account) (*http.Response, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
}

// testCNProviderAnthropicConnection verifies the native Anthropic endpoint of a
// CN-provider account explicitly configured with api_protocol=anthropic. Before
// this path existed such accounts fell through to the generic Claude tester,// which (a) appended ?beta=true and (b) defaulted a missing base_url to
// https://api.anthropic.com — sending the provider's API key to Anthropic
// instead of the provider's own Anthropic-compatible endpoint. The probe uses
// GetAnthropicProtocolBaseURL (same resolution as real /v1/messages forwarding,
// including per-platform defaults) and the shared API-key auth header.
// FORK-ANCHOR: cn-anthropic-custom-prompt (国产供应商 Anthropic 端点使用弹窗提示词)
func (s *AccountTestService) testCNProviderAnthropicConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = claude.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)

	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetAnthropicProtocolBaseURL())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid Anthropic base URL: %s", err.Error()))
	}
	if hint := cnAnthropicBaseURLMisconfigHint(baseURL, account.routesByModel()); hint != "" {
		return s.sendErrorAndEnd(c, hint)
	}
	apiURL := nativeAnthropicMessagesURL(account, baseURL)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payload, err := createTestPayloadWithTools(testModelID, prompt, accountTestToolsEnabled(c))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Anthropic test payload")
	}
	payloadBytes, _ := json.Marshal(payload)

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Anthropic test request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	for key, value := range claude.DefaultHeaders() {
		req.Header.Set(key, value)
	}
	// Ollama Cloud Anthropic 兼容端点按实际 base_url 强制 Bearer，其余保持
	// extra/default 行为。
	setAnthropicAPIKeyAuthHeader(req.Header, account, authToken, account.GetAnthropicProtocolBaseURL())
	applyOpenCodeUpstreamUserAgent(account, apiURL, req.Header)
	account.ApplyHeaderOverrides(req.Context(), req.Header)
	applyOpenCodeSessionHeader(c, account, apiURL, req.Header, payloadBytes)

	resp, err := s.doCNProviderAdaptiveRequest(req, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Anthropic endpoint request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Anthropic endpoint returned %d: %s", resp.StatusCode, string(body))
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && s.accountRepo != nil {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, errMsg)
	}

	return s.processClaudeStream(c, resp.Body)
}

// cnAnthropicBaseURLMisconfigHint reports an actionable error when an
// anthropic-protocol account's base_url still points at an OpenAI-compatible
// endpoint (paas path, version segment, or chat/completions / responses
// suffix). The naive {base}/v1/messages join would 404 (e.g.
// .../api/paas/v4/v1/messages) with no hint about the actual misconfiguration.
// versionAware is set for providers whose join is version-aware (see
// nativeAnthropicMessagesURL), where a trailing version segment is valid.
func cnAnthropicBaseURLMisconfigHint(baseURL string, versionAware bool) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	path := strings.ToLower(strings.TrimRight(parsed.Path, "/"))
	if path == "" {
		return ""
	}
	openAICompatShaped := strings.Contains(path, "/paas/") ||
		strings.HasSuffix(path, "/chat/completions") ||
		strings.HasSuffix(path, "/responses") ||
		(!versionAware && openAIBaseURLHasVersionSuffix(path))
	if !openAICompatShaped {
		return ""
	}
	return fmt.Sprintf(
		"API protocol is anthropic but base_url (%s) looks like an OpenAI-compatible endpoint; "+
			"requests would hit {base}/v1/messages and 404. Set base_url to the provider's Anthropic endpoint "+
			"(e.g. https://open.bigmodel.cn/api/anthropic) or switch api_protocol to chat_completions/adaptive.",
		baseURL,
	)
}
