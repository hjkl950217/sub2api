package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAggregateHubBalanceProbeAccount() *Account {
	return &Account{
		ID:       77,
		Platform: PlatformAggregateHub,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"account_mode":                     AccountModePayG,
			"api_key":                          "sk-relay",
			"base_url":                         "https://relay.example.com",
			aggregateHubBalanceProbeEnabledKey: true,
		},
	}
}

func TestAggregateHubBalanceProbe_OneAPIQuota(t *testing.T) {
	repo := &cnBalanceProbeRepo{account: newAggregateHubBalanceProbeAccount()}
	upstream := &cnBalanceResponseUpstream{
		statusCode: http.StatusOK,
		body:       `{"success":true,"data":{"quota":1250000}}`,
	}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "USD", result.Currency)
	require.Equal(t, 2.5, result.Balance)
	require.Len(t, repo.extraWrites, 1)
	require.Equal(t, 2.5, repo.extraWrites[0]["ag_hub_balance"])
}

func TestAggregateHubBalanceProbe_CustomPathAndDivisor(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	account.Credentials[aggregateHubBalancePathKey] = "data.remaining"
	account.Credentials[aggregateHubBalanceCurrencyKey] = "cny"
	account.Credentials[aggregateHubBalanceDivisorKey] = float64(100)
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

func TestAggregateHubBalanceProbe_DisabledByDefault(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	delete(account.Credentials, aggregateHubBalanceProbeEnabledKey)
	repo := &cnBalanceProbeRepo{account: account}
	svc := NewCNProviderBalanceService(repo, nil, &cnBalanceResponseUpstream{statusCode: http.StatusOK}, nil)

	_, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
	require.Empty(t, repo.extraWrites)
}

func TestAggregateHubBalanceProbe_URLDefaultsToBase(t *testing.T) {
	account := newAggregateHubBalanceProbeAccount()
	require.Equal(t, "https://relay.example.com/api/user/self", aggregateHubBalanceURL(account))

	account.Credentials[aggregateHubBalanceURLKey] = "https://relay.example.com/custom/balance"
	require.Equal(t, "https://relay.example.com/custom/balance", aggregateHubBalanceURL(account))
}

func TestCaBalance_DeepSeekIgnoresStaleCodingMode(t *testing.T) {
	account := newDeepSeekBalanceProbeAccount()
	account.Credentials["account_mode"] = AccountModeCoding

	require.NoError(t, validatePayGAccount(account))
}
