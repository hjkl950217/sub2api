// FORK: 批量账号测试（二开新增文件，不改上游）。
//
// 给账号列表页的「批量测试」用：
//   - BuildBatchTestPlan 按平台分组，算出每组的共有模型（来源是账号凭据里的
//     model_mapping 键，不请求上游），并标记哪些账号能参与。
//   - RunAccountBatchTest 并发跑完一批账号的连通性测试，把每个账号的 SSE 事件
//     打上 account_id 后转发给前端；单账号超时不影响其它账号。
//
// 语义与单账号普通测试一致：只探测，不写账号配置。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// defaultBatchTestConcurrency 是批量测试同时进行的账号数上限。并发不暴露给
	// 前端：一次几十个账号同时打上游容易触发站点限流，也会把本地连接池打满。
	defaultBatchTestConcurrency = 3
	// defaultBatchTestTimeout 是单个账号的测试超时。整批靠它兜底，避免一个卡死
	// 的账号拖住后面所有账号。
	defaultBatchTestTimeout = 90 * time.Second

	batchTestIneligiblePlatform = "该平台不支持协议探测"
	batchTestNoModelMapping     = "账号未配置 model_mapping，无法确定测试模型"
	// batchTestNoCompletion 表示测试正常返回却没有发出 test_complete，无法判定结果。
	// 判失败而不是判通过：宁可多报一次错，也不要给一个没结论的账号标「通过」。
	batchTestNoCompletion = "测试未返回完成事件，无法判定结果"
)

// BatchTestTarget 是批量测试里单个账号的测试参数。
type BatchTestTarget struct {
	AccountID int64  `json:"account_id"`
	ModelID   string `json:"model_id"`
	Prompt    string `json:"prompt"`
}

// BatchTestAccountInfo 是候选账号里的一项。
type BatchTestAccountInfo struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Models   []string `json:"models"`
	Eligible bool     `json:"eligible"`
	Reason   string   `json:"reason,omitempty"`
	// Schedulable 是账号当前的调度开关，弹窗里逐行改完后直接回写。
	Schedulable bool `json:"schedulable"`
	// Protocols 是账号会被探测的协议清单（与协议矩阵同序）。前端拿它先渲染
	// 灰色占位标签，测试推进时逐个变色，不用等结果回来才知道要测几个协议。
	Protocols []string `json:"protocols,omitempty"`
}

// BatchTestAccountGroup 是同一平台的一组账号，附带该组的共有模型。
type BatchTestAccountGroup struct {
	Platform     string                 `json:"platform"`
	AccountIDs   []int64                `json:"account_ids"`
	CommonModels []string               `json:"common_models"`
	Accounts     []BatchTestAccountInfo `json:"accounts"`
}

// BatchTestPlan 是批量测试弹窗打开时需要的全部前置数据。
type BatchTestPlan struct {
	Groups       []BatchTestAccountGroup `json:"groups"`
	SkippedCount int                     `json:"skipped_count"`
}

// BatchTestRunOptions 控制一次批量测试执行。
type BatchTestRunOptions struct {
	Targets     []BatchTestTarget
	Concurrency int
	Timeout     time.Duration
}

// batchTestEligible 报告账号能否参与批量测试。范围与测试弹窗的「更新支持协议」
// 按钮一致：国产四家 + openai API Key。
func batchTestEligible(account *Account) (bool, string) {
	if account == nil {
		return false, batchTestIneligiblePlatform
	}
	if !account.IsCNProvider() && !account.IsOpenAIApiKey() {
		return false, batchTestIneligiblePlatform
	}
	if len(account.GetModelMapping()) == 0 {
		return false, batchTestNoModelMapping
	}
	return true, ""
}

// batchTestProtocols 报告账号会被探测哪些协议，顺序与协议矩阵的探测顺序一致。
func batchTestProtocols(account *Account) []string {
	if account == nil {
		return nil
	}
	if account.IsCNProvider() {
		return cnProtocolProbeOrder
	}
	if account.IsOpenAIApiKey() {
		return openaiAPIKeyProtocolProbeOrder
	}
	return nil
}

// BuildBatchTestPlan 把选中账号按平台分组，并算出每组的共有模型。
func (s *AccountTestService) BuildBatchTestPlan(ctx context.Context, accountIDs []int64) (*BatchTestPlan, error) {
	plan := &BatchTestPlan{Groups: []BatchTestAccountGroup{}}
	if s == nil || s.accountRepo == nil {
		return plan, nil
	}

	byPlatform := map[string]*BatchTestAccountGroup{}
	order := []string{}
	for _, id := range accountIDs {
		account, err := s.accountRepo.GetByID(ctx, id)
		if err != nil || account == nil {
			plan.SkippedCount++
			continue
		}
		group, ok := byPlatform[account.Platform]
		if !ok {
			group = &BatchTestAccountGroup{Platform: account.Platform}
			byPlatform[account.Platform] = group
			order = append(order, account.Platform)
		}
		info := BatchTestAccountInfo{ID: account.ID, Name: account.Name, Schedulable: account.Schedulable}
		if eligible, reason := batchTestEligible(account); eligible {
			info.Eligible = true
			info.Models = sortedModelKeys(account.GetModelMapping())
			info.Protocols = batchTestProtocols(account)
		} else {
			info.Reason = reason
			plan.SkippedCount++
		}
		group.Accounts = append(group.Accounts, info)
		group.AccountIDs = append(group.AccountIDs, account.ID)
	}

	sort.Strings(order)
	for _, platform := range order {
		group := byPlatform[platform]
		group.CommonModels = commonModelKeys(group.Accounts)
		plan.Groups = append(plan.Groups, *group)
	}
	return plan, nil
}

func sortedModelKeys(mapping map[string]string) []string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// commonModelKeys 取该组所有可测账号模型键的交集，保持字典序。
func commonModelKeys(accounts []BatchTestAccountInfo) []string {
	common := []string{}
	for _, account := range accounts {
		if !account.Eligible || len(account.Models) == 0 {
			continue
		}
		if len(common) == 0 {
			common = append(common, account.Models...)
			continue
		}
		keep := make(map[string]bool, len(account.Models))
		for _, model := range account.Models {
			keep[model] = true
		}
		next := make([]string, 0, len(common))
		for _, model := range common {
			if keep[model] {
				next = append(next, model)
			}
		}
		common = next
	}
	return common
}

// batchTestEvent 是转发给前端的事件：内层的测试事件 + 所属账号。
type batchTestEvent struct {
	AccountID int64 `json:"account_id"`
	TestEvent
}

// batchTestSink 把各账号的事件串行写回外层 SSE 连接。
type batchTestSink struct {
	mu       sync.Mutex
	outer    *gin.Context
	writeErr error
}

func (s *batchTestSink) forwardEvent(accountID int64, event TestEvent) {
	encoded, err := json.Marshal(batchTestEvent{AccountID: accountID, TestEvent: event})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return
	}
	if _, err := fmt.Fprintf(s.outer.Writer, "data: %s\n\n", encoded); err != nil {
		s.writeErr = err
		return
	}
	s.outer.Writer.Flush()
}

// batchTestWriter 是喂给内层 gin.Context 的 ResponseWriter：按 SSE 事件边界切分
// 后逐条转发，并打上账号标记。每个账号一个实例，因此 buffer 不需要加锁。
type batchTestWriter struct {
	accountID int64
	sink      *batchTestSink
	buffer    []byte
	// completion / completionOK 记录 test_complete 的结论。整体成败只能看它：
	// 协议矩阵在「三个协议全失败」时仍然返回 nil，靠返回值判断会把失败标成通过。
	completion   bool
	completionOK bool
}

func (w *batchTestWriter) Header() http.Header    { return http.Header{} }
func (w *batchTestWriter) WriteHeader(int)         {}
func (w *batchTestWriter) Flush()                  {}

func (w *batchTestWriter) Write(p []byte) (int, error) {
	w.buffer = append(w.buffer, p...)
	for {
		index := bytes.Index(w.buffer, []byte("\n\n"))
		if index < 0 {
			break
		}
		chunk := w.buffer[:index]
		w.buffer = w.buffer[index+2:]
		w.forwardChunk(chunk)
	}
	return len(p), nil
}

func (w *batchTestWriter) forwardChunk(chunk []byte) {
	line := bytes.TrimSpace(chunk)
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	var event TestEvent
	if err := json.Unmarshal(bytes.TrimSpace(line[len("data:"):]), &event); err != nil {
		return
	}
	if event.Type == "test_complete" {
		w.completion = true
		w.completionOK = event.Success
	}
	w.sink.forwardEvent(w.accountID, event)
}

// RunAccountBatchTest 并发跑完一批账号连通性测试，逐账号把 SSE 事件转发到 c。
// 每个账号的事件带 account_id；单账号超时或客户端断开都不会拖住整批。
func (s *AccountTestService) RunAccountBatchTest(c *gin.Context, opts BatchTestRunOptions) error {
	if s == nil || c == nil {
		return fmt.Errorf("batch test service is unavailable")
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultBatchTestConcurrency
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultBatchTestTimeout
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	ctx := c.Request.Context()
	sink := &batchTestSink{outer: c}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, target := range opts.Targets {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(target BatchTestTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runSingleBatchTarget(ctx, sink, target, timeout)
		}(target)
	}
	wg.Wait()
	return nil
}

func (s *AccountTestService) runSingleBatchTarget(ctx context.Context, sink *batchTestSink, target BatchTestTarget, timeout time.Duration) {
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	writer := &batchTestWriter{accountID: target.AccountID, sink: sink}
	ginCtx, _ := gin.CreateTestContext(writer)
	ginCtx.Request = (&http.Request{}).WithContext(testCtx)

	err := s.TestAccountConnection(ginCtx, target.AccountID, target.ModelID, target.Prompt, AccountTestModeDefault)
	// 收尾事件不能只看 TestAccountConnection 的返回值：协议矩阵路径在探测完全失败时
	// 仍返回 nil，只把结论放在 test_complete 的 success 里。
	switch {
	case err != nil:
		// 子测试自己的 error 事件已经转发过了。这里补一条收尾事件，避免前端
		// 该账号一直停在「测试中」。
		sink.forwardEvent(target.AccountID, TestEvent{Type: "batch_test_complete", Error: err.Error()})
	case !writer.completion:
		sink.forwardEvent(target.AccountID, TestEvent{Type: "batch_test_complete", Error: batchTestNoCompletion})
	case !writer.completionOK:
		// 具体哪几个协议没过已经在 protocol_result 里逐条给出，这里不重复带错误文本。
		sink.forwardEvent(target.AccountID, TestEvent{Type: "batch_test_complete"})
	default:
		sink.forwardEvent(target.AccountID, TestEvent{Type: "batch_test_complete", Success: true})
	}
}
