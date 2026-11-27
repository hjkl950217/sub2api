package repository

import (
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// FORK: 上游请求体压缩的 host 级策略（二开新增文件，非上游改动）。
// 两件事：
//  1. 本机/内网上游不压 —— 传输在局域网内，压缩只花 CPU、省不到网络时间；
//  2. 按 host 熔断 —— 压缩请求被上游按编码问题拒掉后，十分钟内不再对同一 host 压缩。
//     判错的代价只是少省一次压缩，不会让请求失败，所以宁可保守。
const (
	upstreamZstdHostBlockDuration = 10 * time.Minute
	upstreamZstdHostBlockMaxSize  = 256
)

type upstreamZstdHostPolicy struct {
	mu           sync.Mutex
	blockedUntil map[string]time.Time
	now          func() time.Time
}

func newUpstreamZstdHostPolicy() *upstreamZstdHostPolicy {
	return &upstreamZstdHostPolicy{blockedUntil: make(map[string]time.Time), now: time.Now}
}

func normalizeZstdHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

// blocked 报告该 host 是否还在熔断窗口内；窗口过期随即清掉这一条。
func (p *upstreamZstdHostPolicy) blocked(host string) bool {
	if p == nil {
		return false
	}
	host = normalizeZstdHost(host)
	if host == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.blockedUntil[host]
	if !ok {
		return false
	}
	if p.now().Before(until) {
		return true
	}
	delete(p.blockedUntil, host)
	return false
}

// record 只在「本次确实压缩过」的请求上调用：2xx 解除熔断，编码类 4xx 进入熔断窗口。
// 其他状态码（限流、鉴权、服务端错误）与压缩无关，不记。
func (p *upstreamZstdHostPolicy) record(host string, statusCode int) {
	if p == nil {
		return
	}
	host = normalizeZstdHost(host)
	if host == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case statusCode >= 200 && statusCode < 300:
		delete(p.blockedUntil, host)
	case statusCode == 400, statusCode == 413, statusCode == 415, statusCode == 422:
		_, alreadyBlocked := p.blockedUntil[host]
		p.evictForCapacityLocked()
		p.blockedUntil[host] = p.now().Add(upstreamZstdHostBlockDuration)
		if !alreadyBlocked {
			slog.Warn("upstream request compression paused for host after rejection",
				"host", host, "status_code", statusCode, "seconds", int(upstreamZstdHostBlockDuration.Seconds()))
		}
	}
}

func (p *upstreamZstdHostPolicy) evictForCapacityLocked() {
	if len(p.blockedUntil) < upstreamZstdHostBlockMaxSize {
		return
	}
	now := p.now()
	for host, until := range p.blockedUntil {
		if !now.Before(until) {
			delete(p.blockedUntil, host)
		}
	}
	if len(p.blockedUntil) < upstreamZstdHostBlockMaxSize {
		return
	}
	var oldestHost string
	var oldestUntil time.Time
	for host, until := range p.blockedUntil {
		if oldestHost == "" || until.Before(oldestUntil) {
			oldestHost, oldestUntil = host, until
		}
	}
	delete(p.blockedUntil, oldestHost)
}

// isLocalCompressionHost 判断上游是否就在本机或局域网内。
// 只认字面 IP 和 localhost：域名解析到内网地址的情况识别不到，按可压缩处理。
// shortcut: 不做 DNS 解析，解析结果随环境变化且要额外延迟。
func isLocalCompressionHost(host string) bool {
	host = strings.TrimSuffix(normalizeZstdHost(host), ".")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
