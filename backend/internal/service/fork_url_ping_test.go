//go:build unit

// FORK: URL 连通性测试的自检（二开）。

package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newPingTestService() *AccountTestService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	return NewAccountTestService(nil, nil, nil, nil, nil, nil, cfg, nil)
}

// 4xx/5xx 也算连通：只要能拿到 HTTP 响应就返回 Success。
func TestPingURL_ErrorStatusStillReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	res := newPingTestService().PingURL(context.Background(), server.URL, "")

	require.True(t, res.Success)
	require.Equal(t, http.StatusInternalServerError, res.StatusCode)
	require.GreaterOrEqual(t, res.LatencyMs, int64(0))
}

func TestPingURL_ConnectionRefusedFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	res := newPingTestService().PingURL(context.Background(), "http://"+addr, "")

	require.False(t, res.Success)
	require.NotEmpty(t, res.Message)
}

func TestPingURL_RejectsNonHTTPSchemes(t *testing.T) {
	res := newPingTestService().PingURL(context.Background(), "ftp://example.com", "")

	require.False(t, res.Success)
	require.Contains(t, res.Message, "url")
}
