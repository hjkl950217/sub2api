// FORK: 账号表单「ping」按钮的后端实现（二开）。
// 只测到目标地址的网络连通性，不校验鉴权与协议内容。

package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	urlPingTimeout        = 10 * time.Second
	urlPingMaxRedirects   = 3
	urlPingUserAgentValue = "sub2api-url-ping"
)

// URLPingResult 是「ping」按钮的返回结构。任何 HTTP 响应都算连通，
// 所以 Success 为 true 时 StatusCode 可能是 401/403/404/500。
type URLPingResult struct {
	Success    bool   `json:"success"`
	LatencyMs  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// PingURL 对 rawURL 发一次 GET，返回从发起到收到响应头的耗时。
// proxyURL 为空表示直连。URL 安全策略与账号保存、余额探测共用一套。
func (s *AccountTestService) PingURL(ctx context.Context, rawURL, proxyURL string) *URLPingResult {
	normalized, err := cnValidateProbeURL(s.cfg, rawURL)
	if err != nil {
		return &URLPingResult{Success: false, Message: err.Error()}
	}

	opts := httpclient.Options{
		ProxyURL: proxyURL,
		Timeout:  urlPingTimeout,
	}
	if s.cfg != nil {
		opts.ValidateResolvedIP = s.cfg.Security.URLAllowlist.Enabled
		opts.AllowPrivateHosts = s.cfg.Security.URLAllowlist.AllowPrivateHosts
	}
	client, err := httpclient.GetClient(opts)
	if err != nil {
		return &URLPingResult{Success: false, Message: err.Error()}
	}

	// GetClient 返回的是缓存共享实例，不能改它的 CheckRedirect。
	// 复制一份单独设重定向策略：每一跳都重新过 URL 安全策略，
	// 否则白名单内主机的 3xx 就能把请求带到白名单外。
	pingClient := *client
	pingClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= urlPingMaxRedirects {
			return errors.New("too many redirects")
		}
		_, err := cnValidateProbeURL(s.cfg, req.URL.String())
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return &URLPingResult{Success: false, Message: err.Error()}
	}
	req.Header.Set("User-Agent", urlPingUserAgentValue)
	req.Header.Set("Accept", "*/*")

	start := time.Now()
	resp, err := pingClient.Do(req)
	if err != nil {
		return &URLPingResult{Success: false, Message: err.Error()}
	}
	latencyMs := time.Since(start).Milliseconds()
	statusCode := resp.StatusCode
	// 只关心响应头：不读 body，避免上游边吐边卡时把请求拖到超时。
	_ = resp.Body.Close()

	return &URLPingResult{Success: true, LatencyMs: latencyMs, StatusCode: statusCode}
}
