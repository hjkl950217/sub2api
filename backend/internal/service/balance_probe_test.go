// FORK: 余额探测脚本登记表单测（二开新增文件）。
package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func newAggregateHubBalanceProbeAccount() *Account {
	return &Account{
		ID:       77,
		Platform: PlatformAggregateHub,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"account_mode":         AccountModePayG,
			"api_key":              "sk-relay",
			"base_url":             "https://relay.example.com",
			balanceProbeEnabledKey: true,
		},
	}
}

func newOpenAIBalanceProbeAccount() *Account {
	account := newAggregateHubBalanceProbeAccount()
	account.Platform = PlatformOpenAI
	return account
}

// cnBalanceRouteUpstream 按 URL 子串返回不同响应，供多端点脚本（New API）测试。
type cnBalanceRouteUpstream struct {
	routes []cnBalanceRoute
}

type cnBalanceRoute struct {
	match  string
	status int
	body   string
}

func (u *cnBalanceRouteUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	status, body := http.StatusNotFound, "not found"
	for _, route := range u.routes {
		if strings.Contains(req.URL.String(), route.match) {
			status, body = route.status, route.body
			break
		}
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func (u *cnBalanceRouteUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestBalanceProbe_NewAPIFiniteQuota(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptNewAPI
	repo := &cnBalanceProbeRepo{account: account}
	upstream := &cnBalanceRouteUpstream{routes: []cnBalanceRoute{
		{match: "/v1/dashboard/billing/subscription", status: http.StatusOK, body: `{"hard_limit_usd":10}`},
		{match: "/v1/dashboard/billing/usage", status: http.StatusOK, body: `{"total_usage":2.5}`},
	}}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "USD", result.Currency)
	require.Equal(t, 7.5, result.Balance)
}

func TestBalanceProbe_NewAPIUnlimited(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptNewAPI
	repo := &cnBalanceProbeRepo{account: account}
	upstream := &cnBalanceRouteUpstream{routes: []cnBalanceRoute{
		{match: "/v1/dashboard/billing/subscription", status: http.StatusOK, body: `{"hard_limit_usd":100000000}`},
	}}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.Balances[0].Unlimited)
	require.Equal(t, true, repo.extraWrites[0]["ag_hub_balance_unlimited"])
}

func TestBalanceProbe_Sub2APIUsage(t *testing.T) {
	account := newOpenAIBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptSub2API
	repo := &cnBalanceProbeRepo{account: account}
	upstream := &cnBalanceRouteUpstream{routes: []cnBalanceRoute{
		{match: "/v1/usage", status: http.StatusOK, body: `{"balance":12.5,"isValid":true}`},
	}}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 12.5, result.Balance)
	require.Equal(t, "USD", result.Currency)
}

func TestBalanceProbe_AutoPersistsHitScript(t *testing.T) {
	account := newOpenAIBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptAuto
	repo := &cnBalanceProbeRepo{account: account}
	// New API 端点 404，Sub2API 的 /v1/usage 命中。
	upstream := &cnBalanceRouteUpstream{routes: []cnBalanceRoute{
		{match: "/v1/dashboard/billing/subscription", status: http.StatusNotFound, body: `404 page not found`},
		{match: "/v1/usage", status: http.StatusOK, body: `{"balance":5}`},
	}}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 5.0, result.Balance)
	require.Len(t, repo.credentialWrites, 1)
	require.Equal(t, balanceProbeScriptSub2API, repo.credentialWrites[0][balanceProbeScriptKey])
}

func TestBalanceProbe_CustomPathAndDivisor(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	account.Credentials[balanceProbePathKey] = "data.remaining"
	account.Credentials[balanceProbeCurrencyKey] = "cny"
	account.Credentials[balanceProbeDivisorKey] = float64(100)
	repo := &cnBalanceProbeRepo{account: account}
	upstream := &cnBalanceResponseUpstream{
		statusCode: http.StatusOK,
		body:       `{"data":{"remaining":1234}}`,
	}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "CNY", result.Currency)
	require.Equal(t, 12.34, result.Balance)
}

func TestBalanceProbe_DisabledByDefault(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptSub2API
	account.Credentials[balanceProbeEnabledKey] = false
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderBalanceService(repo, nil, &cnBalanceResponseUpstream{statusCode: http.StatusOK}, nil)

	_, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
	require.Empty(t, repo.extraWrites)
}

func TestBalanceProbe_FailurePersistsError(t *testing.T) {
	account := newOpenAIBalanceProbeAccount()
	account.Credentials[balanceProbeScriptKey] = balanceProbeScriptSub2API
	repo := &cnBalanceProbeRepo{account: account}
	upstream := &cnBalanceResponseUpstream{statusCode: http.StatusUnauthorized, body: `{"message":"bad key"}`}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.NoError(t, err)
	require.False(t, result.Success)
	require.Len(t, repo.extraWrites, 1)
	require.Contains(t, repo.extraWrites[0]["openai_balance_error"], "Authentication failed")
	require.NotEmpty(t, repo.extraWrites[0]["openai_balance_error_at"])
}

func TestBalanceProbe_URLDefaults(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	require.Equal(t, "https://relay.example.com/api/user/self", customBalanceURL(account))
	require.Equal(t, "https://relay.example.com/v1/dashboard/billing/subscription", newAPIBalanceURL(account))

	// 中转站的 OpenAI 端点通常带 /v1，余额接口仍在站点根路径。
	account.Credentials["base_url"] = "https://relay.example.com/v1/"
	require.Equal(t, "https://relay.example.com/api/user/self", customBalanceURL(account))

	// 手填 URL 的存量账号自动落到自定义脚本。
	account.Credentials[balanceProbeURLKey] = "https://relay.example.com/custom/balance"
	require.Equal(t, balanceProbeScriptCustom, resolveBalanceProbeScript(account))
	require.Equal(t, "https://relay.example.com/custom/balance", customBalanceURL(account))
}

func TestCaBalance_DeepSeekIgnoresStaleCodingMode(t *testing.T) {
	account := newDeepSeekBalanceProbeAccount()
	account.Credentials["account_mode"] = AccountModeCoding

	require.NoError(t, validatePayGAccount(account))
}
