// FORK: URL 连通性测试接口的自检（二开）。

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandler_PingURL_DirectConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer target.Close()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	testSvc := service.NewAccountTestService(nil, nil, nil, nil, nil, nil, cfg, nil)
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, testSvc, nil, nil, nil, nil, nil)

	router := gin.New()
	router.POST("/api/v1/admin/accounts/ping-url", handler.PingURL)

	raw, err := json.Marshal(map[string]any{"url": target.URL, "proxy_id": nil})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/ping-url", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data service.URLPingResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Data.Success)
	require.Equal(t, http.StatusNotFound, body.Data.StatusCode)
}
