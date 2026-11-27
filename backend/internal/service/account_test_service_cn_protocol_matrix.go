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

// accountTestFirstByteContextKey 挂一个 *time.Time。sendEvent 收到第一条 content
// 事件时打点，协议矩阵据此算出「首字响应」耗时；完整耗时用探针前后的时间差。
// FORK-ANCHOR: protocol-probe-timing-context (二开：逐协议首字/完整耗时打点)
const accountTestFirstByteContextKey = "account_test_first_byte"

// cnProtocolProbeOrder 探测顺序，与前端协议卡片顺序、兜底优先级一致。
var cnProtocolProbeOrder = []string{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}

// FORK-ANCHOR: protocol-matrix-network-ping (二开：协议测试下面加一条网络层 ping 延迟)
// 作为协议列表之外的独立一项，与协议并发探测；不计入 passed/failed，不参与协议回写。
const networkPingProtocol = "ping"

// FORK-ANCHOR: protocol-matrix-network-ping-target (二开：ping 目标取账号主端点)
// 多协议账号优先取 chat 端点本身（不看勾选集合，避免三地址不同时 ping 到别的协议），
// openai API Key 取 openai 基址，最后回退凭据里的 base_url。
// 都没有时返回空，交给 PingURL 报「地址为空」，不会误测 Anthropic 默认域名。
func accountNetworkPingTarget(account *Account) string {
	if account == nil {
		return ""
	}
	for _, candidate := range []string{
		account.GetCNProtocolBaseURLForProbe(APIProtocolChatCompletions),
		account.GetOpenAIBaseURL(),
		account.GetCredential("base_url"),
	} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return ""
}

// FORK-ANCHOR: protocol-probe-endpoint (二开：结果展示用的探测地址)
// protocolProbeEndpoint 返回探测该协议实际使用的上游基址，只用于结果展示。
func protocolProbeEndpoint(account *Account, protocol string) string {
	if protocol == networkPingProtocol {
		return accountNetworkPingTarget(account)
	}
	if baseURL := account.GetCNProtocolBaseURLForProbe(protocol); baseURL != "" {
		return baseURL
	}
	return account.GetOpenAIBaseURL()
}

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

// probeProtocolsConcurrently 并发探测多个协议，按完成顺序把 protocol_probe /
// protocol_result 事件写回外层连接，返回探测通过的协议（按传入顺序）与失败的错误。
//
// 每个协议拿独立的 gin.Context（写入 recorder，事件被抑制）与账号副本，正文、报错、
// 首字打点都收集在各自的上下文里；只有外层 c 由本函数单线程写入，不会并发写 ResponseWriter。
// probe 收到的上下文已设好抑制开关，回调只需决定用哪套探测函数。
func (s *AccountTestService) probeProtocolsConcurrently(
	c *gin.Context,
	protocols []string,
	account *Account,
	probe func(probeContext *gin.Context, probeAccount *Account, protocol string) error,
) (passed []string, failed []error) {
	type result struct {
		protocol string
		err      error
		text     string
		// errText 是探测函数推送到测试上下文 error 事件里的完整报错，探测失败时非空。
		errText string
		// FORK-ANCHOR: protocol-matrix-timing-result (二开：逐协议首字与完整耗时)
		ttfb  time.Duration
		total time.Duration
	}

	results := make(chan result, len(protocols)+1)
	for _, protocol := range protocols {
		s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: protocol})
		go func(protocol string) {
			probeContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			probeContext.Request = c.Request.Clone(c.Request.Context())
			probeContext.Set(accountTestSuppressErrorContextKey, true)
			probeContext.Set(accountTestSuppressCompletionContextKey, true)
			probeContext.Set(accountTestActiveProtocolContextKey, protocol)
			probeContext.Set(accountTestSuppressContentContextKey, true)
			// FORK-ANCHOR: account-test-tool-call-probe-context (二开：工具调用开关与打点随协议上下文隔离)
			if c.GetBool(accountTestWithToolsContextKey) {
				toolSeen := false
				probeContext.Set(accountTestWithToolsContextKey, true)
				probeContext.Set(accountTestToolCallContextKey, &toolSeen)
			}
			contentBuilder := &strings.Builder{}
			probeContext.Set(accountTestContentContextKey, contentBuilder)
			// FORK-ANCHOR: protocol-matrix-collect-error-text (二开：探测失败的完整报错也要收集)
			// 各协议的探测函数把报错塞在测试上下文的 error 事件里，只看返回值拿不到文本。
			errorBuilder := &strings.Builder{}
			probeContext.Set(accountTestErrorContextKey, errorBuilder)
			// FORK-ANCHOR: protocol-matrix-timing-probe (二开：首字打点，探针结束后算首字/完整耗时)
			firstByte := time.Time{}
			probeContext.Set(accountTestFirstByteContextKey, &firstByte)
			start := time.Now()
			err := probe(probeContext, cloneAccountForProtocolProbe(account), protocol)
			total := time.Since(start)
			var ttfb time.Duration
			if !firstByte.IsZero() {
				ttfb = firstByte.Sub(start)
			}
			results <- result{protocol: protocol, err: err, text: contentBuilder.String(), errText: errorBuilder.String(), ttfb: ttfb, total: total}
		}(protocol)
	}

	// FORK-ANCHOR: protocol-matrix-network-ping-probe (二开：网络层 ping 延迟与协议并发)
	// 只测到账号主地址的连通性（GET 响应头），复用一个账号已配置的代理。
	proxyURL := ""
	proxyUnavailable := account.ProxyID != nil && account.Proxy == nil
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	pingTarget := accountNetworkPingTarget(account)
	pingCtx := c.Request.Context()
	s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: networkPingProtocol})
	go func() {
		if proxyUnavailable {
			results <- result{protocol: networkPingProtocol, err: errors.New("account proxy is unavailable")}
			return
		}
		ping := s.PingURL(pingCtx, pingTarget, proxyURL)
		if ping == nil || !ping.Success {
			message := "ping failed"
			if ping != nil && strings.TrimSpace(ping.Message) != "" {
				message = strings.TrimSpace(ping.Message)
			}
			results <- result{protocol: networkPingProtocol, err: errors.New(message)}
			return
		}
		results <- result{protocol: networkPingProtocol, total: time.Duration(ping.LatencyMs) * time.Millisecond}
	}()

	outcomes := make(map[string]result, len(protocols))
	for i := 0; i < len(protocols)+1; i++ {
		result := <-results
		// ping 只上报结果，不并入 passed/failed，避免影响协议回写与整轮成败。
		if result.protocol == networkPingProtocol {
			ok := result.err == nil
			// FORK-ANCHOR: protocol-result-endpoint-ping (二开：ping 结果带上它测的地址)
			event := TestEvent{Type: "protocol_result", Protocol: networkPingProtocol, ProtocolOK: forkBoolPtr(ok), TotalMS: result.total.Milliseconds(), Endpoint: pingTarget}
			if !ok {
				event.Error = result.err.Error()
			}
			s.sendEvent(c, event)
			continue
		}
		outcomes[result.protocol] = result
		ok := result.err == nil
		// FORK-ANCHOR: protocol-result-endpoint-event (二开：结果带上该协议实际探测的地址)
		event := TestEvent{Type: "protocol_result", Protocol: result.protocol, ProtocolOK: forkBoolPtr(ok), Text: result.text, Endpoint: protocolProbeEndpoint(account, result.protocol)}
		// FORK-ANCHOR: protocol-matrix-timing-event (二开：逐协议首字/完整耗时随 protocol_result 回传)
		event.FirstByteMS = result.ttfb.Milliseconds()
		event.TotalMS = result.total.Milliseconds()
		if result.err != nil {
			event.Error = result.err.Error()
		}
		// FORK-ANCHOR: protocol-matrix-error-event (二开：把收集到的完整报错挂到 protocol_result 上)
		if !ok && strings.TrimSpace(result.errText) != "" {
			event.Error = strings.TrimSpace(result.errText)
		}
		s.sendEvent(c, event)
	}

	// 结果按探测顺序回传，调用方的回写与报错选择都不受完成先后影响。
	passed = make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		result := outcomes[protocol]
		if result.err == nil {
			passed = append(passed, protocol)
			continue
		}
		failed = append(failed, result.err)
	}
	return passed, failed
}

// probeCNProviderProtocolsConnection 并发探测三个原生协议端点；仅显式同步时回写通过集合。
func (s *AccountTestService) probeCNProviderProtocolsConnection(c *gin.Context, account *Account, modelID string, prompt string, syncProtocols bool) error {
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}
	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	passed, _ := s.probeProtocolsConcurrently(c, cnProtocolProbeOrder, account, func(probeContext *gin.Context, probeAccount *Account, protocol string) error {
		if protocol == APIProtocolResponses && !account.SupportsNativeCNResponses() {
			return errors.New("provider has no native Responses endpoint")
		}
		//nolint:govet // 只读探测副本：故意浅拷贝以替换 accountRepo，探测期间不并发使用 s 的 mutex
		probeService := *s
		if !syncProtocols {
			probeService.accountRepo = protocolProbeAccountRepository{AccountRepository: s.accountRepo}
		}
		switch protocol {
		case APIProtocolAnthropic:
			return probeService.testCNProviderAdaptiveAnthropicConnection(probeContext, probeAccount, testModelID, authToken, prompt)
		case APIProtocolResponses:
			return probeService.testCNProviderAdaptiveResponsesConnection(probeContext, probeAccount, testModelID, authToken, prompt)
		default:
			return probeService.testCNProviderChatCompletionsConnection(probeContext, probeAccount, modelID, prompt)
		}
	})

	applied := false
	if syncProtocols && len(passed) > 0 {
		applied = s.applyProbedCNProtocols(c, account, passed)
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: len(passed) > 0, ProtocolApplied: applied})
	return nil
}

// isDeepseekDSGroupAccount 报告账号是否直接走三协议探测矩阵（deepseek DS 分组 + 聚合中转）。
// 并发本身已对所有账号生效，这里只决定路由：这两类账号不按已勾选协议裁剪，三协议全测。
// FORK-ANCHOR: ag-hub-protocol-matrix-concurrent (二开：聚合中转与 DS 分组同样并发探测)
func isDeepseekDSGroupAccount(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformAggregateHub {
		return true
	}
	if account.Platform != PlatformDeepseek {
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
	// FORK-ANCHOR: fork-api-protocols-openai-sync-platform-gate (二开：openai API Key / ag_hub 复选账号同样支持协议回写)
	if !account.IsCNProvider() && !account.IsOpenAIApiKey() && account.Platform != PlatformAggregateHub {
		return infraerrors.BadRequest("INVALID_ACCOUNT_PLATFORM", "protocol sync is only supported for CN providers, OpenAI API-key and aggregate-hub accounts")
	}
	if account.IsOpenAIApiKey() {
		return s.updateProbedOpenAIAPIKeyProtocols(ctx, account, passed)
	}
	if !setProbedCNProtocols(account, passed) {
		return infraerrors.BadRequest("NO_PROBED_PROTOCOLS", "at least one passed protocol is required")
	}
	return s.accountRepo.Update(ctx, account)
}

// FORK-ANCHOR: probed-request-zstd-sync (二开：按压缩请求体测试结果更新账号配置)
// UpdateAccountRequestZstd 把账号 extra 里的 upstream_request_zstd 写成测试结论：
// 通过时置 true，失败时删除该键（回到默认不压缩）。只对 API Key 账号生效。
func (s *AccountTestService) UpdateAccountRequestZstd(ctx context.Context, accountID int64, enabled bool) error {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account == nil || account.Type != AccountTypeAPIKey {
		return infraerrors.BadRequest("INVALID_ACCOUNT_TYPE", "request zstd sync is only supported for API-key accounts")
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	if enabled {
		account.Extra[AccountExtraUpstreamRequestZstd] = true
	} else {
		delete(account.Extra, AccountExtraUpstreamRequestZstd)
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
	// FORK-ANCHOR: fork-api-protocols-sync-base-url (二开：只有 chat 通过才回写旧字段 base_url)
	// 原先用 GetCNProtocolBaseURL(chat) 回写，chat 未通过时它会改道兜底协议，
	// 把别的协议地址写进 base_url（三地址不同时会污染模型同步、余额探测等读取方）。
	if stringSliceContains(selected, APIProtocolChatCompletions) {
		if baseURLs, ok := credentials["api_base_urls"].(map[string]any); ok {
			if baseURL, ok := baseURLs[APIProtocolChatCompletions].(string); ok && strings.TrimSpace(baseURL) != "" {
				credentials["base_url"] = strings.TrimSpace(baseURL)
			}
		}
	}
	credentials["api_protocol"] = APIProtocolAdaptive
	if len(selected) == 1 {
		credentials["api_protocol"] = selected[0]
	}
	return true
}
