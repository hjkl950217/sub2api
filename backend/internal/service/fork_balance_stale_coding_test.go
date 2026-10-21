// FORK: DeepSeek 残留 coding 标记的余额探测覆盖（二开新增文件）。
package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// DeepSeek 没有 coding 套餐：存量账号上残留的 coding 标记不应把它挡在余额探测之外，
// 额度照常探测的同时也要进 payg 余额检查队列。
func TestForkBalanceCheckDeepSeekStaleCodingStillProbesBalance(t *testing.T) {
	deepseekCoding := Account{
		ID: 7, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"api_key":      "sk-deepseek",
			"base_url":     "https://api.deepseek.com",
		},
	}

	loadRepo := &recordingCNBalanceLoadRepo{}
	repo := &fakeCNCheckRepo{byPlatform: map[string][]Account{
		PlatformDeepseek: {deepseekCoding},
	}}
	prober := &fakeCNQuotaProber{}
	svc := &CNProviderBalanceCheckService{
		accountRepo:    repo,
		balanceService: NewCNProviderBalanceService(loadRepo, nil, nil, &config.Config{}),
		quotaService:   prober,
		cfg:            &config.Config{},
	}

	svc.runOnce()

	require.Equal(t, []int64{7}, prober.probed, "额度探测不能丢")
	require.Equal(t, []int64{7}, loadRepo.getByIDIDs, "余额探测不能因为残留 coding 标记被跳过")
}
