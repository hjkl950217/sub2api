// FORK: 二开用例——DeepSeek 主机上的 platform=openai 账号强制走 chat，避免 #7285 的
// Responses-Lite 工具退化为 DSML 文本。与上游 openai_apikey_responses_probe_test.go 分开放，
// 减少合并冲突。
package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestForkIsDeepSeekUnsafeResponsesHost(t *testing.T) {
	unsafe := []string{
		"https://api.deepseek.com",
		"https://api.deepseek.com/v1",
		"https://API.DEEPSEEK.COM:443/v1/responses",
		"  https://api.deepseek.com  ",
	}
	for _, u := range unsafe {
		require.True(t, isDeepSeekUnsafeResponsesHost(u), u)
	}

	safe := []string{
		"https://api.openai.com",
		"https://relay.example.com/v1",
		// 后缀伪装：不能只看 strings.Contains
		"https://api.deepseek.com.example.test/v1",
		"https://notapi.deepseek.com/v1",
		"",
		"not-a-url",
	}
	for _, u := range safe {
		require.False(t, isDeepSeekUnsafeResponsesHost(u), u)
	}
}

func TestForkProbeDeepSeekHostSkipsNetworkAndForcesChat(t *testing.T) {
	for _, baseURL := range []string{"https://api.deepseek.com", "https://api.deepseek.com/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			updateCalls := make(chan map[string]any, 1)
			account := Account{
				ID:          7401,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL},
			}
			repo := &snapshotUpdateAccountRepo{
				stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
				updateExtraCalls:      updateCalls,
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"function_call","name":"probe_ping"}]}`)),
			}}
			svc := &AccountTestService{
				accountRepo:  repo,
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}

			svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

			// 不打上游
			require.Nil(t, upstream.lastReq)
			updates := <-updateCalls
			require.Equal(t, false, updates[openai_compat.ExtraKeyResponsesSupported])
			require.Equal(t, string(openai_compat.ResponsesSupportModeAuto), updates[openai_compat.ExtraKeyResponsesMode])
		})
	}
}

func TestForkProbeNonDeepSeekHostStillProbes(t *testing.T) {
	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          7402,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://compat-upstream.example/v1"},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"function_call","name":"probe_ping"}]}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

	// 非 DeepSeek 主机照旧走网络探测并据结果落标
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://compat-upstream.example/v1/responses", upstream.lastReq.URL.String())
	updates := <-updateCalls
	require.Equal(t, true, updates[openai_compat.ExtraKeyResponsesSupported])
}
