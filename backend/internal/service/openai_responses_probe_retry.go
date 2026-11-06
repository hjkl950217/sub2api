package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

const (
	// openAIResponsesProbeRetryInterval 是 unknown 账号的补探测周期。
	// 建号/改号时的首次探测遇网络失败会保持 unknown，网关按「未探测=走 Responses」
	// 路由；若上游实际只支持 Chat Completions，该账号会一直被路由到不存在的端点，
	// 直到管理员再次提交 credentials 才会重探（上游 issue #8356）。
	// 周期取 30 分钟：unknown 极少见，且探测本身有上游调用成本。
	openAIResponsesProbeRetryInterval = 30 * time.Minute

	// openAIResponsesProbeRetryRunTimeout 限制一轮扫描的总时长。
	openAIResponsesProbeRetryRunTimeout = 2 * time.Minute

	// openAIResponsesProbeRetryMaxPerRun 限制单轮补探测数量，避免历史遗留的
	// 大批 unknown 账号在同一时刻齐发上游请求。
	openAIResponsesProbeRetryMaxPerRun = 20
)

// openAIResponsesProbeAccountLister 是补探测所需的最小账号读取能力。
type openAIResponsesProbeAccountLister interface {
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)
}

// OpenAIResponsesProbeRetryService 周期性重探 Responses 能力未知的 OpenAI APIKey 账号。
type OpenAIResponsesProbeRetryService struct {
	accountLister openAIResponsesProbeAccountLister
	probeOne      func(ctx context.Context, accountID int64)
	interval      time.Duration
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// NewOpenAIResponsesProbeRetryService 构造补探测任务。
// probeOne 由调用方传入 AccountTestService.ProbeOpenAIAPIKeyResponsesSupport。
func NewOpenAIResponsesProbeRetryService(
	accountLister openAIResponsesProbeAccountLister,
	probeOne func(ctx context.Context, accountID int64),
) *OpenAIResponsesProbeRetryService {
	return &OpenAIResponsesProbeRetryService{
		accountLister: accountLister,
		probeOne:      probeOne,
		interval:      openAIResponsesProbeRetryInterval,
		stopCh:        make(chan struct{}),
	}
}

// Start 启动后台补探测。interval <= 0 时不启动。
func (s *OpenAIResponsesProbeRetryService) Start() {
	if s == nil || s.accountLister == nil || s.probeOne == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// 启动即补一轮：unknown 属于历史遗留状态，没有理由再等一个周期。
		s.runOnce()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop 停止后台补探测并等待在途扫描结束。
func (s *OpenAIResponsesProbeRetryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// runOnce 扫描 OpenAI 平台账号，对状态仍为 unknown 的 APIKey 账号逐个补探测。
func (s *OpenAIResponsesProbeRetryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), openAIResponsesProbeRetryRunTimeout)
	defer cancel()

	accounts, err := s.accountLister.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		logger.LegacyPrintf("service.openai_probe_retry", "list_accounts_failed: err=%v", err)
		return
	}
	probed := 0
	for i := range accounts {
		if probed >= openAIResponsesProbeRetryMaxPerRun {
			return
		}
		account := &accounts[i]
		if account.Type != AccountTypeAPIKey || !account.IsActive() {
			continue
		}
		if openai_compat.ResolveResponsesSupport(account.Extra) != openai_compat.ResponsesSupportUnknown {
			continue
		}
		s.probeOne(ctx, account.ID)
		probed++
	}
}
