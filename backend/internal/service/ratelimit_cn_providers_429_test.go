//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// 上游临时过载（overloaded_error / server is busy）与配额窗口无关：不得进
// opencode_go / Coding Plan 的快照选窗冷却（否则一次过载会被停调到数天后的
// 窗口重置点），必须返回 false 落默认 429 短冷却。
func TestApplyCNProviderReactive429_OverloadedFallsBackToDefault(t *testing.T) {
	t.Parallel()
	futureWeekly := time.Now().Add(3 * 24 * time.Hour).Format(time.RFC3339)
	bodies := []string{
		`{"type":"error","error":{"type":"overloaded_error","message":"server is busy, please retry later"}}`,
		`{"error":{"message":"Server is busy, please retry later"}}`,
		`{"error":{"message":"The engine is currently overloaded. Please try again later."}}`,
	}
	accounts := []*Account{
		{
			Platform:    PlatformMiniMax,
			Credentials: map[string]any{"account_mode": AccountModeCoding},
			Extra:       map[string]any{"minimax_weekly_reset_at": futureWeekly},
		},
		{
			Platform: PlatformOpenCodeGo,
			Extra:    map[string]any{"opencode_go_weekly_reset_at": futureWeekly},
		},
	}
	for i, account := range accounts {
		for j, body := range bodies {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			handled := svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, []byte(body))
			require.False(t, handled, "account#%d body#%d", i, j)
			require.Zero(t, repo.rateLimitedCalls, "account#%d body#%d", i, j)
			require.Zero(t, repo.tempCalls, "account#%d body#%d", i, j)
		}
	}
}

// 非 overloaded 语义的 429 仍按快照选窗冷却：修复不得把真配额限流放行。
func TestApplyCNProviderReactive429_Quota429StillCoolsToSnapshotReset(t *testing.T) {
	t.Parallel()
	resetAt := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		Platform:    PlatformMiniMax,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       map[string]any{"minimax_5h_reset_at": resetAt.Format(time.RFC3339)},
	}
	body := []byte(`{"error":{"type":"rate_limit_error","message":"You have reached the 5-hour usage limit; quota will reset at the window boundary"}}`)

	require.True(t, svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, body))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, resetAt, repo.lastRateLimitedAt, time.Minute)
}

// 智谱等中文额度耗尽文案过去不在英文清单里，会被当成动态限流只写 5 秒兜底冷却，
// 账号随后立刻被重新选中并继续 429（上游 issue #8399）。Coding Plan 账号应冷却到
// 快照里的窗口重置点。
func TestApplyCNProviderReactive429_ChineseQuotaExhaustionCoolsToSnapshotReset(t *testing.T) {
	t.Parallel()
	resetAt := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       map[string]any{cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset): resetAt.Format(time.RFC3339)},
	}
	body := []byte(`{"error":{"code":"1302","message":"您的5小时额度已用尽，请稍后再试"}}`)

	require.True(t, svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, body))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, resetAt, repo.lastRateLimitedAt, time.Minute)
	require.Zero(t, repo.tempCalls)
}

// payg 账号没有窗口快照可依：额度耗尽 429 按余额不足同口径临时停调，
// 不能再落到 5 秒兜底冷却。
func TestApplyCNProviderReactive429_PaygQuotaExhaustionTempUnschedules(t *testing.T) {
	t.Parallel()
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{Platform: PlatformZhipu}
	body := []byte(`{"error":{"code":"1113","message":"当前账户额度已用尽，请充值后重试"}}`)

	require.True(t, svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, body))
	require.Equal(t, 1, repo.tempCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.WithinDuration(t, time.Now().Add(20*time.Minute), repo.lastTempUntil, time.Minute)
}

// 纯频率型 429（无额度文案）仍走默认秒级冷却，不被上面的新分支误伤。
func TestApplyCNProviderReactive429_Frequency429StillFallsBack(t *testing.T) {
	t.Parallel()
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
	}
	body := []byte(`{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`)

	require.False(t, svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, body))
	require.Zero(t, repo.tempCalls)
	require.Zero(t, repo.rateLimitedCalls)
}

// 过载识别签名收紧：确定性过载信号才命中，真配额限流/余额文案不得误判。
func TestCNProviderResponseIndicatesOverloaded(t *testing.T) {
	t.Parallel()
	require.True(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)))
	require.True(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"message":"server is busy, please retry later"}}`)))
	require.True(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"message":"The engine is currently overloaded. Please try again later."}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"type":"rate_limit_error","message":"usage limit reached"}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"message":"insufficient balance"}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(nil))
}
