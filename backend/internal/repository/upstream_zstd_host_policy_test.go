package repository

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func zstdPolicyRequest(t *testing.T, rawURL string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestIsLocalCompressionHost(t *testing.T) {
	t.Parallel()

	local := []string{
		"localhost", "LOCALHOST", "127.0.0.1", "::1", "10.1.2.3", "172.16.0.9", "172.31.255.254",
		"192.168.50.191", "169.254.10.1", "0.0.0.0", "localhost.",
	}
	for _, host := range local {
		require.True(t, isLocalCompressionHost(host), "%s 应判为内网", host)
	}

	remote := []string{"relay.example", "api.cssmapi.fun", "8.8.8.8", "172.32.0.1", "11.0.0.1", ""}
	for _, host := range remote {
		require.False(t, isLocalCompressionHost(host), "%s 不应判为内网", host)
	}
}

func TestUpstreamZstdHostPolicyBlocksAfterRejection(t *testing.T) {
	t.Parallel()

	p := newUpstreamZstdHostPolicy()
	now := time.Now()
	p.now = func() time.Time { return now }

	require.False(t, p.blocked("relay.example"))

	p.record("relay.example", http.StatusBadRequest)
	require.True(t, p.blocked("RELAY.example"), "host 比较不分大小写")

	now = now.Add(upstreamZstdHostBlockDuration - time.Second)
	require.True(t, p.blocked("relay.example"))
	now = now.Add(2 * time.Second)
	require.False(t, p.blocked("relay.example"), "窗口过期后自动恢复")

	p.record("relay.example", http.StatusTooManyRequests)
	require.False(t, p.blocked("relay.example"), "限流与压缩无关，不该熔断")

	p.record("relay.example", http.StatusUnsupportedMediaType)
	require.True(t, p.blocked("relay.example"))
	p.record("relay.example", http.StatusOK)
	require.False(t, p.blocked("relay.example"), "成功一次就解除")
}

func TestUpstreamZstdHostPolicyNilSafe(t *testing.T) {
	t.Parallel()

	var p *upstreamZstdHostPolicy
	require.False(t, p.blocked("relay.example"))
	p.record("relay.example", http.StatusBadRequest)
}

// 内网 host 即使用账号开关打开也不压；白名单是显式决定，压过这条规则。
func TestCompressUpstreamRequestBodySkipsLocalHost(t *testing.T) {
	t.Parallel()

	body := compressibleBody(64 << 10)
	const localURL = "http://192.168.50.191:34080/v1/chat/completions"

	s := zstdTestService(t, nil)
	s.zstdAccounts = zstdAccountsWith(7)
	local := zstdPolicyRequest(t, localURL, body)
	require.Same(t, local, s.compressUpstreamRequestBody(local, 7))

	allowlisted := zstdTestService(t, []string{"192.168.50.191"})
	force := zstdPolicyRequest(t, localURL, body)
	require.NotSame(t, force, allowlisted.compressUpstreamRequestBody(force, 0))
}

// 熔断窗口内不再压缩这个 host。
func TestCompressUpstreamRequestBodySkipsBlockedHost(t *testing.T) {
	t.Parallel()

	s := zstdTestService(t, []string{"relay.example"})
	s.zstdHosts.record("relay.example", http.StatusUnsupportedMediaType)

	req := zstdPolicyRequest(t, "https://relay.example/v1/messages", compressibleBody(64<<10))
	require.Same(t, req, s.compressUpstreamRequestBody(req, 0))
}

// 端到端：压缩请求被上游按 400 拒掉后，同一个 host 的下一次请求不再带压缩。
func TestUpstreamZstdTransportBlocksHostAfterRejection(t *testing.T) {
	var mu sync.Mutex
	var encodings []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		encodings = append(encodings, r.Header.Get("Content-Encoding"))
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	s := zstdTestService(t, []string{"127.0.0.1"})
	body := compressibleBody(64 << 10)
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.Do(req, "", 0, 1)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"zstd", ""}, encodings, "第一次压缩被拒后，第二次不再压这个 host")
}
