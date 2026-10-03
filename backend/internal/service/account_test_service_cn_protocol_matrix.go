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
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

// accountTestSuppressErrorContextKey 抑制探测过程中的 error 事件。
// 协议矩阵要把每个协议的失败当作"该协议不支持"的正常结论继续往下测，
// 不能再把第一个失败当作整个测试的终止错误推给前端。
const accountTestSuppressErrorContextKey = "account_test_suppress_error"

// cnProtocolProbeOrder 探测顺序，与前端协议卡片顺序、兜底优先级一致。
var cnProtocolProbeOrder = []string{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}

// forkBoolPtr 取 bool 地址（本包 ops_metrics_collector.go 已有 boolPtr，避免重名）。
func forkBoolPtr(v bool) *bool { return &v }

// probeCNProviderProtocolsConnection 逐个探测三个原生协议端点并回写勾选集合。
//
// 与 testCNProviderSelectedProtocolsConnection 的区别：后者按账号已勾选集合裁剪、
// 首个失败即终止；这里三个协议各测一次，每个协议的成功/失败单独成事件，
// 全部测完再按结果更新账号配置。
func (s *AccountTestService) probeCNProviderProtocolsConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	authToken := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}
	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}
	testModelID = account.GetMappedModel(testModelID)

	// 两层抑制：单个协议失败是"该协议不支持"的结论而非终止错误；内层探针的
	// test_complete 也不能提前冒泡，否则前端会在矩阵跑完前就判定测试结束。
	c.Set(accountTestSuppressErrorContextKey, true)
	c.Set(accountTestSuppressCompletionContextKey, true)
	defer func() {
		c.Set(accountTestSuppressErrorContextKey, false)
		c.Set(accountTestSuppressCompletionContextKey, false)
	}()

	passed := make([]string, 0, len(cnProtocolProbeOrder))
	for _, protocol := range cnProtocolProbeOrder {
		// 平台本身没有原生 Responses 端点（如 zhipu）：直接判定为不支持，不发请求。
		if protocol == APIProtocolResponses && !account.SupportsNativeCNResponses() {
			s.sendEvent(c, TestEvent{
				Type:       "protocol_result",
				Protocol:   protocol,
				ProtocolOK: forkBoolPtr(false),
				Error:      "provider has no native Responses endpoint",
			})
			continue
		}
		s.sendEvent(c, TestEvent{Type: "protocol_probe", Protocol: protocol})
		var err error
		switch protocol {
		case APIProtocolAnthropic:
			err = s.testCNProviderAdaptiveAnthropicConnection(c, account, testModelID, authToken)
		case APIProtocolResponses:
			err = s.testCNProviderAdaptiveResponsesConnection(c, account, testModelID, authToken)
		default:
			err = s.testCNProviderChatCompletionsConnection(c, account, modelID, prompt)
		}
		if err == nil {
			passed = append(passed, protocol)
		}
		s.sendEvent(c, TestEvent{Type: "protocol_result", Protocol: protocol, ProtocolOK: forkBoolPtr(err == nil)})
	}

	applied := false
	if len(passed) > 0 {
		applied = s.applyProbedCNProtocols(c, account, passed)
	}

	// 先解除两层抑制再发最终事件，否则这条 test_complete 会被自己吞掉
	// （defer 里的解除要等函数返回才执行，来不及）。
	c.Set(accountTestSuppressErrorContextKey, false)
	c.Set(accountTestSuppressCompletionContextKey, false)
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: len(passed) > 0, ProtocolApplied: applied})
	return nil
}

// applyProbedCNProtocols 把探测通过的协议写回账号复选配置。
// 返回是否真的落库成功；未落库时前端不应提示"已更新"。
func (s *AccountTestService) applyProbedCNProtocols(c *gin.Context, account *Account, passed []string) bool {
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
	// 旧字段与新字段保持同一契约：base_url 指向 chat_completions 端点（未勾选时
	// 指向兜底协议端点），api_protocol 只在"仅勾选一个且它就是兜底"时写协议名。
	if baseURL := account.GetCNProtocolBaseURL(APIProtocolChatCompletions); baseURL != "" {
		credentials["base_url"] = baseURL
	}
	credentials["api_protocol"] = APIProtocolAdaptive
	if len(selected) == 1 {
		credentials["api_protocol"] = selected[0]
	}
	if s.accountRepo == nil {
		return false
	}
	if err := s.accountRepo.Update(c.Request.Context(), account); err != nil {
		s.sendEvent(c, TestEvent{Type: "status", Text: "Failed to save probed protocols: " + err.Error()})
		return false
	}
	return true
}
