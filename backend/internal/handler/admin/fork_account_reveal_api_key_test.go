package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// FORK-ANCHOR: account-reveal-api-key-test (二开：编辑弹窗「眼睛」查看 API Key 明文，
// 覆盖「新接口给明文」与「老接口仍然脱敏」两条不变量)

func newRevealAPIKeyTestRouter(account *service.Account) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	svc.getAccountResult = account
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/accounts/:id", handler.GetByID)
	router.GET("/accounts/:id/credentials/reveal", handler.RevealAPIKey)
	return router
}

func revealAPIKeyResponse(t *testing.T, router *gin.Engine, path string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code, recorder.Body.String()
}

func TestForkRevealAPIKeyReturnsPlaintext(t *testing.T) {
	account := &service.Account{
		ID: 7, Name: "cn", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive,
		Credentials: map[string]any{
			"base_url":      "https://example.test/v1",
			"api_key":       "sk-fork-reveal",
			"refresh_token": "must-not-leak",
		},
	}
	router := newRevealAPIKeyTestRouter(account)

	status, body := revealAPIKeyResponse(t, router, "/accounts/7/credentials/reveal")
	require.Equal(t, http.StatusOK, status)

	var payload struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	require.Equal(t, "sk-fork-reveal", payload.Data["api_key"])
	// 只回 api_key：其余敏感键（token / 私钥等）不得随响应外带
	require.Len(t, payload.Data, 1)
	require.NotContains(t, body, "must-not-leak")
}

func TestForkRevealAPIKeyEmptyWhenAbsent(t *testing.T) {
	account := &service.Account{
		ID: 8, Name: "no-key", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Credentials: map[string]any{"base_url": "https://example.test/v1"},
	}
	router := newRevealAPIKeyTestRouter(account)

	status, body := revealAPIKeyResponse(t, router, "/accounts/8/credentials/reveal")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, `"api_key":""`)
}

func TestForkRevealAPIKeyInvalidID(t *testing.T) {
	router := newRevealAPIKeyTestRouter(&service.Account{ID: 9, Status: service.StatusActive})
	status, _ := revealAPIKeyResponse(t, router, "/accounts/abc/credentials/reveal")
	require.Equal(t, http.StatusBadRequest, status)
}

// 老接口行为不变：GET /accounts/:id 仍然经 dto.RedactCredentials 剥掉 api_key。
func TestForkGetAccountStillRedactsAPIKey(t *testing.T) {
	account := &service.Account{
		ID: 10, Name: "cn", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Credentials: map[string]any{"base_url": "https://example.test/v1", "api_key": "sk-must-not-appear"},
	}
	router := newRevealAPIKeyTestRouter(account)

	status, body := revealAPIKeyResponse(t, router, "/accounts/10")
	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, body, "sk-must-not-appear")
	require.Contains(t, body, `"has_api_key":true`)
}
