package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// issue #7747：claude-opus-4.8 在 >200K 输入时的输出与缓存读价格必须按
// Anthropic 1M 长上下文档计价（输入 2x、输出 1.5x、缓存读随输入 2x）。
// 4.6/4.7/4.8 继承 4.5 的价卡，因此只需在 4.5 上配置一次阶梯。
func TestCalculateCost_OpusLongContextTier(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 250000, CacheReadTokens: 10000, OutputTokens: 1000}

	for _, model := range []string{"claude-opus-4-5", "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8"} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1.0)
			require.NoError(t, err)
			require.True(t, cost.LongContextBillingApplied)
			require.InDelta(t, 250000*5e-6*2, cost.InputCost, 1e-9)
			require.InDelta(t, 1000*25e-6*1.5, cost.OutputCost, 1e-9)
			require.InDelta(t, 10000*0.5e-6*2, cost.CacheReadCost, 1e-9)
		})
	}
}

func TestCalculateCost_OpusBelowLongContextThresholdUsesBaseRates(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 200000, OutputTokens: 1000}

	cost, err := svc.CalculateCost("claude-opus-4-8", tokens, 1.0)
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied, "200K is not above the exclusive Anthropic boundary")
	require.InDelta(t, 200000*5e-6, cost.InputCost, 1e-9)
	require.InDelta(t, 1000*25e-6, cost.OutputCost, 1e-9)
}
