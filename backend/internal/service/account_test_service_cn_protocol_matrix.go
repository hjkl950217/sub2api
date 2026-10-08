// FORK: 国产供应商「协议探测矩阵 + 一键更新支持协议」（二开新增文件）
//
// 背景：新接入一个站点时运维方往往不知道它到底支持哪几个上游协议，站点也常不
// 说明不同协议是否走不同端点。常见做法是先把三个协议都开放，逐个测试，再手工
// 取消测不通的协议。本文件把「三个协议各测一次 + 按结果回写账号勾选集合」做成
// 一次探测，配合前端测试弹窗的「更新支持协议」按钮使用。
//
// 语义约定（与 account.go 的 fork-api-protocols 解析层、前端字段契约一致）：
//   - 探测时三个协议各用账号当前的协议端点（api_base_urls），互不干扰；
//   - 某协议通过 → 进入 api_protocols 勾选集合；
//   - 全部失败 → 视为无法判断，保留账号原有勾选集合，不改动任何配置；
//   - fallback_protocol 取探测通过的协议，优先级 chat_completions > anthropic > responses。
package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

// accountTestSuppressErrorContextKey 抑制探测过程中的 error 事件。
// 协议矩阵要把每个协议的失败当作"该协议不支持"的正常结论继续往下测，
// 不能再把第一个失败当作整个测试的终止错误推给前端。
const accountTestSuppressErrorContextKey = "account_test_suppress_error"

// accountTestActiveProtocolContextKey 标记当前上下文正在探测哪个协议。
// 三个协议并发探测时 content 事件本身不带协议名，sendEvent 靠这个键补上来源，
// 前端才能把各协议返回的正文分开显示。只影响事件标注，不参与解析与通过判定。
const accountTestActiveProtocolContextKey = "account_test_active_protocol"

// accountTestContentContextKey 挂一个 *strings.Builder，sendEvent 把 content 事件的
// 正文累积进去；协议跑完后由协议矩阵取出，随 protocol_result 一次性带给前端。
const accountTestContentContextKey = "account_test_content"

// accountTestSuppressContentContextKey 抑制发往前端的 content 事件。
// 协议矩阵下三个协议的正文要按协议分开显示，不能各自流式灌进同一个输出框。
const accountTestSuppressContentContextKey = "account_test_suppress_content"

// cnProtocolProbeOrder 探测顺序，与前端协议卡片顺序、兜底优先级一致。
var cnProtocolProbeOrder = []string{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}

// forkBoolPtr 取 bool 地址（本包 ops_metrics_collector.go 已有 boolPtr，避免重名）。
func forkBoolPtr(v bool) *bool { return &v }

type protocolProbeAccountRepository struct {
	AccountRepository
}

func (protocolProbeAccountRepository) SetError(context.Context, int64, string) error { return nil }
func (protocolProbeAccountRepository) ClearError(context.Context, int64) error       { return nil }
func (protocolProbeAccountRepository) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}

// probeCNProviderProtocolsConnection 并发探测三个原生协议端点；仅显式同步时回写通过集合。
func (s *AccountTestService) probeCNProviderProtocolsConnection(c *gin.Context, account *Account, modelID string, prompt string, syncProtocols bool) error {
	if !isDeepseekDSGroupAccount(account) {
		return s.probeCNProviderProtocolsConnectionSequential(c, account, modelID, prompt)
	}
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}
	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)

	type result struct {
		protocol string
		err      error
		text     string
		// errText 是探测函数推送到测试上下文 error 事件里的完整报错，探测失败时非空。
		errText string
	}
	results := make(chan result, len(cnProtocolProbeOrder))
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	for _, protocol := range cnProtocolProbeOrder {
		s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: protocol})
		go func(protocol string) {
			if protocol == APIProtocolResponses && !account.SupportsNativeCNResponses() {
				results <- result{protocol: protocol, err: errors.New("provider has no native Responses endpoint")}
				return
			}
			// FORK-ANCHOR: protocol-matrix-collect-error-text (二开：探测失败的完整报错也要收集)
			// 各协议的探测函数把报错塞在测试上下文的 error 事件里，只看返回值拿不到文本。
			// 收一份到 errorBuilder，随 protocol_result 一起给前端，失败原因才看得见。
			probeContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			probeContext.Request = c.Request.Clone(c.Request.Context())
			probeContext.Set(accountTestSuppressErrorContextKey, true)
			probeContext.Set(accountTestSuppressCompletionContextKey, true)
			probeContext.Set(accountTestActiveProtocolContextKey, protocol)
			probeContext.Set(accountTestSuppressContentContextKey, true)
			contentBuilder := &strings.Builder{}
			probeContext.Set(accountTestContentContextKey, contentBuilder)
			errorBuilder := &strings.Builder{}
			probeContext.Set(accountTestErrorContextKey, errorBuilder)
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
			probeService := *s
			if !syncProtocols {
				probeService.accountRepo = protocolProbeAccountRepository{AccountRepository: s.accountRepo}
			}
			var err error
			switch protocol {
			case APIProtocolAnthropic:
				err = probeService.testCNProviderAdaptiveAnthropicConnection(probeContext, &probeAccount, testModelID, authToken, prompt)
			case APIProtocolResponses:
				err = probeService.testCNProviderAdaptiveResponsesConnection(probeContext, &probeAccount, testModelID, authToken, prompt)
			default:
				err = probeService.testCNProviderChatCompletionsConnection(probeContext, &probeAccount, modelID, prompt)
			}
			results <- result{protocol: protocol, err: err, text: contentBuilder.String(), errText: errorBuilder.String()}
		}(protocol)
	}

	passed := make([]string, 0, len(cnProtocolProbeOrder))
	for range cnProtocolProbeOrder {
		result := <-results
		ok := result.err == nil
		if ok {
			passed = append(passed, result.protocol)
		}
		event := TestEvent{Type: "protocol_result", Protocol: result.protocol, ProtocolOK: forkBoolPtr(ok), Text: result.text}
		if result.err != nil {
			event.Error = result.err.Error()
		}
		// FORK-ANCHOR: protocol-matrix-error-event (二开：把收集到的完整报错挂到 protocol_result 上)
		if !ok && strings.TrimSpace(result.errText) != "" {
			event.Error = strings.TrimSpace(result.errText)
		}
		s.sendEvent(c, event)
	}

	applied := false
	if syncProtocols && len(passed) > 0 {
		applied = s.applyProbedCNProtocols(c, account, passed)
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: len(passed) > 0, ProtocolApplied: applied})
	return nil
}

func (s *AccountTestService) probeCNProviderProtocolsConnectionSequential(c *gin.Context, account *Account, modelID string, prompt string) error {
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}
	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)
	c.Set(accountTestSuppressErrorContextKey, true)
	c.Set(accountTestSuppressCompletionContextKey, true)
	defer func() {
		c.Set(accountTestSuppressErrorContextKey, false)
		c.Set(accountTestSuppressCompletionContextKey, false)
	}()
	passed := make([]string, 0, len(cnProtocolProbeOrder))
	for _, protocol := range cnProtocolProbeOrder {
		if protocol == APIProtocolResponses && !account.SupportsNativeCNResponses() {
			s.sendEvent(c, TestEvent{Type: "protocol_result", Protocol: protocol, ProtocolOK: forkBoolPtr(false), Error: "provider has no native Responses endpoint"})
			continue
		}
		c.Set(accountTestActiveProtocolContextKey, protocol)
		c.Set(accountTestSuppressContentContextKey, true)
		contentBuilder := &strings.Builder{}
		c.Set(accountTestContentContextKey, contentBuilder)
		// FORK-ANCHOR: protocol-matrix-collect-error-text-seq (二开：串行路径同样收集失败报错)
		errorBuilder := &strings.Builder{}
		c.Set(accountTestErrorContextKey, errorBuilder)
		s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: protocol})
		var err error
		switch protocol {
		case APIProtocolAnthropic:
			err = s.testCNProviderAdaptiveAnthropicConnection(c, account, testModelID, authToken, prompt)
		case APIProtocolResponses:
			err = s.testCNProviderAdaptiveResponsesConnection(c, account, testModelID, authToken, prompt)
		default:
			err = s.testCNProviderChatCompletionsConnection(c, account, modelID, prompt)
		}
		if err == nil {
			passed = append(passed, protocol)
		}
		// FORK-ANCHOR: protocol-matrix-error-event-seq (二开：串行路径也把完整报错带回去)
		seqEvent := TestEvent{Type: "protocol_result", Protocol: protocol, ProtocolOK: forkBoolPtr(err == nil), Text: contentBuilder.String()}
		if err != nil {
			seqEvent.Error = err.Error()
		}
		if err != nil && strings.TrimSpace(errorBuilder.String()) != "" {
			seqEvent.Error = strings.TrimSpace(errorBuilder.String())
		}
		s.sendEvent(c, seqEvent)
	}
	applied := len(passed) > 0 && s.applyProbedCNProtocols(c, account, passed)
	c.Set(accountTestSuppressErrorContextKey, false)
	c.Set(accountTestSuppressCompletionContextKey, false)
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: len(passed) > 0, ProtocolApplied: applied})
	return nil
}

func isDeepseekDSGroupAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformDeepseek {
		return false
	}
	for _, group := range account.Groups {
		if group != nil && group.Name == "赛博羊毛-DS" {
			return true
		}
	}
	return false
}

// applyProbedCNProtocols 把探测通过的协议写回账号复选配置。
func (s *AccountTestService) applyProbedCNProtocols(c *gin.Context, account *Account, passed []string) bool {
	if !setProbedCNProtocols(account, passed) || s.accountRepo == nil {
		return false
	}
	if err := s.accountRepo.Update(c.Request.Context(), account); err != nil {
		s.sendEvent(c, TestEvent{Type: "status", Text: "Failed to save probed protocols: " + err.Error()})
		return false
	}
	return true
}

func (s *AccountTestService) UpdateProbedCNProtocols(ctx context.Context, accountID int64, passed []string) error {
	if len(passed) == 0 {
		return infraerrors.BadRequest("NO_PROBED_PROTOCOLS", "at least one passed protocol is required")
	}
	for _, protocol := range passed {
		if !stringSliceContains(cnProtocolProbeOrder, protocol) {
			return infraerrors.BadRequest("INVALID_PROBED_PROTOCOL", "unsupported protocol: "+protocol)
		}
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	// FORK-ANCHOR: fork-api-protocols-openai-sync-platform-gate (二开：openai API Key 复选账号同样支持协议回写)
	if !account.IsCNProvider() && !account.IsOpenAIApiKey() {
		return infraerrors.BadRequest("INVALID_ACCOUNT_PLATFORM", "protocol sync is only supported for CN providers and OpenAI API-key accounts")
	}
	if account.IsOpenAIApiKey() {
		return s.updateProbedOpenAIAPIKeyProtocols(ctx, account, passed)
	}
	if !setProbedCNProtocols(account, passed) {
		return infraerrors.BadRequest("NO_PROBED_PROTOCOLS", "at least one passed protocol is required")
	}
	return s.accountRepo.Update(ctx, account)
}

func setProbedCNProtocols(account *Account, passed []string) bool {
	// 旧字段继续映射到 Chat 端点与当前兜底协议，兼容既有账号读取路径。
	selected := make([]string, 0, len(cnProtocolProbeOrder))
	for _, protocol := range cnProtocolProbeOrder {
		if stringSliceContains(passed, protocol) {
			selected = append(selected, protocol)
		}
	}
	if len(selected) == 0 {
		return false
	}
	credentials := account.Credentials
	if credentials == nil {
		credentials = map[string]any{}
	}
	credentials[apiProtocolsCredentialKey] = selected
	credentials[fallbackProtocolCredentialKey] = selected[0]
	account.Credentials = credentials
	if baseURL := account.GetCNProtocolBaseURL(APIProtocolChatCompletions); baseURL != "" {
		credentials["base_url"] = baseURL
	}
	credentials["api_protocol"] = APIProtocolAdaptive
	if len(selected) == 1 {
		credentials["api_protocol"] = selected[0]
	}
	return true
}
