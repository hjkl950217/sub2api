//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func compatSessionTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, rec
}

func TestApplyCompatAnthropicSessionIDIsStableAndIsolated(t *testing.T) {
	account := &Account{ID: 71, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "k"}}
	c, _ := compatSessionTestContext(t)
	body := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hello"}]}`)

	updated, sessionID := applyCompatAnthropicSessionID(body, account, c, "")
	require.NotEmpty(t, sessionID)
	require.Contains(t, gjson.GetBytes(updated, "metadata.user_id").String(), "_session_"+sessionID)

	_, again := applyCompatAnthropicSessionID(body, account, c, "")
	require.Equal(t, sessionID, again, "same conversation must reuse the session id")

	other := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"different"}]}`)
	_, otherSession := applyCompatAnthropicSessionID(other, account, c, "")
	require.NotEqual(t, sessionID, otherSession, "different conversations must not collide")
}

func TestApplyCompatAnthropicSessionIDKeepsExplicitSignal(t *testing.T) {
	account := &Account{ID: 72, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "k"}}
	c, _ := compatSessionTestContext(t)
	c.Request.Header.Set("X-Claude-Code-Session-Id", "explicit-session")
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)

	updated, sessionID := applyCompatAnthropicSessionID(body, account, c, "")
	require.Empty(t, sessionID)
	require.JSONEq(t, string(body), string(updated))
}

func TestApplyCompatAnthropicSessionIDSkipsOAuthAndOtherPlatforms(t *testing.T) {
	c, _ := compatSessionTestContext(t)
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	for _, account := range []*Account{
		{ID: 73, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 74, Platform: PlatformKimi, Type: AccountTypeAPIKey},
	} {
		updated, sessionID := applyCompatAnthropicSessionID(body, account, c, "")
		require.Empty(t, sessionID)
		require.JSONEq(t, string(body), string(updated))
	}
}

func TestForwardAsChatCompletionsInjectsCompatSession(t *testing.T) {
	body := `{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hi"}]}`
	c, _ := compatSessionTestContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(namespaceToolAnthropicStream())),
	}}
	account := &Account{ID: 75, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture-key"}}
	svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)

	sessionID := upstream.lastReq.Header.Get("X-Claude-Code-Session-Id")
	require.NotEmpty(t, sessionID, "outbound /v1/messages must carry a session signal")
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "metadata.user_id").String(), "_session_"+sessionID)
}
