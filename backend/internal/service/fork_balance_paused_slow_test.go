// FORK: 关调度账号的低频余额刷新（二开新增测试文件）。
package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// 手动关调度的 payg 账号不进常规 runOnce 的余额队列，但要以慢周期刷新快照，
// 否则列表里的余额会长期停在最后一次探测的值。
func TestForkBalanceCheckPausedAccountsRefreshSlowly(t *testing.T) {
	deepseekPaused := Account{
		ID: 11, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false,
		Credentials: map[string]any{"api_key": "sk-deepseek", "base_url": "https://api.deepseek.com"},
	}
	deepseekActive := Account{
		ID: 12, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-deepseek", "base_url": "https://api.deepseek.com"},
	}
	// 非激活账号两条链路都跳过。
	deepseekDisabled := Account{
		ID: 13, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: false,
		Credentials: map[string]any{"api_key": "sk-deepseek"},
	}
	// 聚合中转未开启余额探测：不进入慢速队列。
	agHubPaused := Account{
		ID: 14, Platform: PlatformAggregateHub, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false,
		Credentials: map[string]any{"api_key": "sk-hub", "base_url": "https://relay.example.com/v1"},
	}
	// coding plan 账号没有余额端点（DeepSeek 除外）。
	kimiCodingPaused := Account{
		ID: 15, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
	}

	loadRepo := &recordingCNBalanceLoadRepo{}
	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformDeepseek:     {deepseekPaused, deepseekActive, deepseekDisabled},
		PlatformAggregateHub: {agHubPaused},
		PlatformKimi:         {kimiCodingPaused},
	}}
	svc := &CNProviderBalanceCheckService{
		accountRepo:    repo,
		balanceService: NewCNProviderBalanceService(loadRepo, nil, nil, &config.Config{}),
		cfg:            &config.Config{},
	}

	svc.runOnce()
	require.Equal(t, []int64{12}, loadRepo.getByIDIDs, "常规周期只刷可调度账号")

	svc.runPausedOnce()
	require.Equal(t, []int64{12, 11}, loadRepo.getByIDIDs, "慢周期补刷手动关调度的 payg 账号")
}

// 慢周期配置：<=0 关闭；默认 120 分钟。
func TestForkBalanceCheckPausedIntervalFromConfig(t *testing.T) {
	svc := &CNProviderBalanceCheckService{cfg: &config.Config{}}
	require.Zero(t, svc.pausedBalanceInterval(), "未配置时视为关闭")

	svc.cfg.Gateway.CNProviders.BalanceCheckPausedIntervalMinutes = 120
	require.Equal(t, 2*time.Hour, svc.pausedBalanceInterval())

	svc.cfg.Gateway.CNProviders.BalanceCheckPausedIntervalMinutes = 0
	require.Zero(t, svc.pausedBalanceInterval())

	nilCfg := &CNProviderBalanceCheckService{}
	require.Zero(t, nilCfg.pausedBalanceInterval())
}
