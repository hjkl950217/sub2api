// FORK: openai API Key「协议探测矩阵 + 一键更新支持协议」（二开新增文件）
//
// 与国产供应商的协议矩阵同语义，只是协议集合收窄为 chat_completions 与
// responses 两个：openai 平台的 API Key 账号（多为中转站）其 chat 与 responses
// 通常同域不同路径，因此两个协议的探测共用账号配置的 base_url，responses 一侧靠
// 临时的 openai_responses_mode=force_responses 强制走原生端点。
//
// 语义约定（与 account_test_service_cn_protocol_matrix.go 一致）：
//   - 两协议各测一次，互不干扰；
//   - 某协议通过 → 进入 api_protocols 勾选集合；
//   - 全部失败 → 保留账号原有勾选集合，不改动任何配置；
//   - fallback_protocol 取探测通过的协议，优先级 chat_completions > responses。
package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
)

// openaiAPIKeyProtocolProbeOrder 探测顺序，与前端协议卡片顺序、兜底优先级一致。
var openaiAPIKeyProtocolProbeOrder = []string{APIProtocolChatCompletions, APIProtocolResponses}

// cloneAccountForProtocolProbe 复制账号并清空派生缓存，供协议探针独立使用。
// 与 CN 矩阵内联的处理保持一致，避免副本继承原账号的映射与请求头覆写缓存。
func cloneAccountForProtocolProbe(account *Account) *Account {
	probeAccount := *account
	probeAccount.modelMappingCache = nil
	probeAccount.modelMappingCacheReady = false
	probeAccount.modelMappingCacheCredentialsPtr = 0
	probeAccount.modelMappingCacheRawPtr = 0
	probeAccount.modelMappingCacheRawLen = 0
	probeAccount.modelMappingCacheRawSig = 0
	probeAccount.modelMappingCacheRuntimeVersion = 0
	probeAccount.headerOverrideCache = nil
	probeAccount.headerOverrideCacheReady = false
	probeAccount.headerOverrideCacheCredentialsPtr = 0
	probeAccount.headerOverrideCacheRawPtr = 0
	probeAccount.headerOverrideCacheRawLen = 0
	probeAccount.headerOverrideCacheRawSig = 0
	return &probeAccount
}

// withOpenAIResponsesMode 返回带指定 responses 模式的 extra 副本，避免并发探测改动共享 map。
func withOpenAIResponsesMode(extra map[string]any, mode openai_compat.ResponsesSupportMode) map[string]any {
	out := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		out[key] = value
	}
	out[openai_compat.ExtraKeyResponsesMode] = string(mode)
	return out
}

// openAIResponsesModeFromProbedProtocols 把两协议探测结论映射到 openai_responses_mode，
// 让未走复选路径的读取方（extra.openai_responses_mode）也拿到一致结论。
func openAIResponsesModeFromProbedProtocols(passed []string) openai_compat.ResponsesSupportMode {
	if stringSliceContains(passed, APIProtocolResponses) {
		return openai_compat.ResponsesSupportModeForceResponses
	}
	return openai_compat.ResponsesSupportModeForceChatCompletions
}

// probeOpenAIAPIKeyProtocolsConnection 依次探测 chat_completions 与 responses 两个原生端点；
// 仅显式同步时回写通过集合，普通连接测试只上报逐协议结果（对齐 CN 协议矩阵语义）。
func (s *AccountTestService) probeOpenAIAPIKeyProtocolsConnection(c *gin.Context, account *Account, modelID string, prompt string, protocols []string, syncProtocols bool) error {
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}
	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)

	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}

	// 普通测试不改动账号，探测过程中的 SetError / SetRateLimited 也不该落库。
	probeService := *s
	if !syncProtocols && s.accountRepo != nil {
		probeService.accountRepo = protocolProbeAccountRepository{AccountRepository: s.accountRepo}
	}

	c.Set(accountTestSuppressErrorContextKey, true)
	c.Set(accountTestSuppressCompletionContextKey, true)
	defer func() {
		c.Set(accountTestSuppressErrorContextKey, false)
		c.Set(accountTestSuppressCompletionContextKey, false)
	}()

	passed := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		c.Set(accountTestActiveProtocolContextKey, protocol)
		c.Set(accountTestSuppressContentContextKey, true)
		contentBuilder := &strings.Builder{}
		c.Set(accountTestContentContextKey, contentBuilder)
		s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: protocol})

		probeAccount := cloneAccountForProtocolProbe(account)
		var probeErr error
		switch protocol {
		case APIProtocolResponses:
			probeAccount.Extra = withOpenAIResponsesMode(account.Extra, openai_compat.ResponsesSupportModeForceResponses)
			probeErr = probeService.testOpenAIAccountConnection(c, probeAccount, testModelID, prompt, AccountTestModeDefault)
		default:
			probeErr = probeService.testOpenAIChatCompletionsConnection(c, probeAccount, testModelID, prompt, normalizedBaseURL, authToken)
		}
		if probeErr == nil {
			passed = append(passed, protocol)
		}
		event := TestEvent{Type: "protocol_result", Protocol: protocol, ProtocolOK: forkBoolPtr(probeErr == nil), Text: contentBuilder.String()}
		if probeErr != nil {
			event.Error = probeErr.Error()
		}
		s.sendEvent(c, event)
	}

	applied := false
	if syncProtocols && len(passed) > 0 {
		applied = s.applyProbedOpenAIAPIKeyProtocols(c, account, passed)
	}
	c.Set(accountTestSuppressErrorContextKey, false)
	c.Set(accountTestSuppressCompletionContextKey, false)
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: len(passed) > 0, ProtocolApplied: applied})
	return nil
}

// applyProbedOpenAIAPIKeyProtocols 把探测通过的协议写回账号复选配置，并同步
// extra.openai_responses_mode。全部失败时不改动任何配置。
func (s *AccountTestService) applyProbedOpenAIAPIKeyProtocols(c *gin.Context, account *Account, passed []string) bool {
	if !setProbedCNProtocols(account, passed) || s.accountRepo == nil {
		return false
	}
	setOpenAIResponsesModeExtra(account, openAIResponsesModeFromProbedProtocols(passed))
	if err := s.accountRepo.Update(c.Request.Context(), account); err != nil {
		s.sendEvent(c, TestEvent{Type: "status", Text: "Failed to save probed protocols: " + err.Error()})
		return false
	}
	return true
}

// updateProbedOpenAIAPIKeyProtocols 是「直接保存已有探测结果」路径的 openai 版本，
// 不发起任何上游请求。
func (s *AccountTestService) updateProbedOpenAIAPIKeyProtocols(ctx context.Context, account *Account, passed []string) error {
	for _, protocol := range passed {
		if !stringSliceContains(openaiAPIKeyProtocolProbeOrder, protocol) {
			return infraerrors.BadRequest("INVALID_PROBED_PROTOCOL", "unsupported protocol: "+protocol)
		}
	}
	if !setProbedCNProtocols(account, passed) {
		return infraerrors.BadRequest("NO_PROBED_PROTOCOLS", "at least one passed protocol is required")
	}
	setOpenAIResponsesModeExtra(account, openAIResponsesModeFromProbedProtocols(passed))
	return s.accountRepo.Update(ctx, account)
}

// setOpenAIResponsesModeExtra 就地更新账号 extra 里的 responses 模式。
func setOpenAIResponsesModeExtra(account *Account, mode openai_compat.ResponsesSupportMode) {
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[openai_compat.ExtraKeyResponsesMode] = string(mode)
}
