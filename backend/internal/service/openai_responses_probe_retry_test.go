package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

type stubResponsesProbeLister struct {
	accounts []Account
	err      error
}

func (s *stubResponsesProbeLister) ListByPlatform(context.Context, string) ([]Account, error) {
	return s.accounts, s.err
}

func TestOpenAIResponsesProbeRetryRunOnceProbesOnlyUnknownAPIKeys(t *testing.T) {
	lister := &stubResponsesProbeLister{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		}},
		{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusDisabled},
	}}
	var probed []int64
	svc := NewOpenAIResponsesProbeRetryService(lister, func(_ context.Context, accountID int64) {
		probed = append(probed, accountID)
	})

	svc.runOnce()

	require.Equal(t, []int64{1}, probed)
}

func TestOpenAIResponsesProbeRetryRunOnceSkipsOnListError(t *testing.T) {
	lister := &stubResponsesProbeLister{err: errors.New("db down")}
	probed := 0
	svc := NewOpenAIResponsesProbeRetryService(lister, func(context.Context, int64) { probed++ })

	svc.runOnce()

	require.Zero(t, probed)
}
