package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 上游可读文案透传（2026-11-06 需求）：换号耗尽后，403 的网络安全策略原文
// 必须到达客户端，不能再被 "Upstream access forbidden..." 覆盖；调度与换号不变。
func TestGatewayFailoverExhaustedPassesUpstreamMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}
	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusForbidden,
		ResponseBody: []byte(`{"type":"error","error":{"type":"permission_error","message":"该会话已被网络安全策略屏蔽，请开启新会话"}}`),
	}, service.PlatformAnthropic, false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "该会话已被网络安全策略屏蔽，请开启新会话")
}

// 401 例外：凭据类原文可能回显 key 片段，保留通用文案。
func TestGatewayFailoverExhaustedKeepsGenericMessageForUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}
	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"message":"Incorrect API key provided: sk-secret"}}`),
	}, service.PlatformAnthropic, false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "sk-secret")
	require.Contains(t, rec.Body.String(), "Upstream authentication failed")
}
