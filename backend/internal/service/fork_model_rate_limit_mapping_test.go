//go:build unit

// FORK: 模型级冷却按「请求模型名 / 映射后模型名」双向匹配的单元测试（新增文件，非上游改动）。
// 起因：temp 规则写入冷却 key 用请求模型名，调度判断读映射后的名字，
// 配了 model_mapping 的账号两边对不上，模型级冷却等于没生效。

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func forkModelRateLimitAccount(mapping map[string]any, limitedKeys ...string) *Account {
	resetAt := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	limits := make(map[string]any, len(limitedKeys))
	for _, key := range limitedKeys {
		limits[key] = map[string]any{"rate_limit_reset_at": resetAt}
	}
	account := &Account{
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{modelRateLimitsKey: limits},
	}
	if mapping != nil {
		account.Credentials["model_mapping"] = mapping
	}
	return account
}

func TestForkModelRateLimitMatchesRequestedAndMappedModel(t *testing.T) {
	ctx := context.Background()
	mapping := map[string]any{
		"glm-5.3":        "cn:glm-5.3",
		"glm-5.3-latest": "cn:glm-5.3",
	}

	t.Run("冷却写在请求模型名", func(t *testing.T) {
		account := forkModelRateLimitAccount(mapping, "glm-5.3")

		require.True(t, account.isModelRateLimitedWithContext(ctx, "glm-5.3"))
		require.Greater(t, account.GetModelRateLimitRemainingTimeWithContext(ctx, "glm-5.3"), time.Duration(0))
	})

	t.Run("冷却写在映射后模型名", func(t *testing.T) {
		account := forkModelRateLimitAccount(mapping, "cn:glm-5.3")

		require.True(t, account.isModelRateLimitedWithContext(ctx, "glm-5.3"))
	})

	t.Run("同一上游模型的其他别名同样命中", func(t *testing.T) {
		account := forkModelRateLimitAccount(mapping, "glm-5.3")

		require.True(t, account.isModelRateLimitedWithContext(ctx, "glm-5.3-latest"))
	})

	t.Run("其他模型不受影响", func(t *testing.T) {
		account := forkModelRateLimitAccount(mapping, "glm-5.3")

		require.False(t, account.isModelRateLimitedWithContext(ctx, "deepseek-flash"))
	})

	t.Run("调度入口排除被冷却的模型", func(t *testing.T) {
		account := forkModelRateLimitAccount(mapping, "glm-5.3")

		require.False(t, account.IsSchedulableForModel("glm-5.3"))
		require.True(t, account.IsSchedulableForModel("deepseek-flash"))
	})

	t.Run("通配符 pattern 不作为别名反查", func(t *testing.T) {
		account := forkModelRateLimitAccount(map[string]any{"glm-*": "cn:glm-5.3"}, "glm-*")

		require.False(t, account.isModelRateLimitedWithContext(ctx, "glm-5.3"))
	})
}
