// FORK: 账号表单「ping」按钮的接口（二开）。
// POST /api/v1/admin/accounts/ping-url

package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PingURLRequest 是 URL 连通性测试的请求体。ProxyID 为空或 0 表示直连。
type PingURLRequest struct {
	URL     string `json:"url"`
	ProxyID *int64 `json:"proxy_id"`
}

// PingURL 测一个 URL 的连通性，可选走账号当前选中的代理。
// 任何 HTTP 响应都算连通；网络、DNS、TLS、代理失败才算失败。
func (h *AccountHandler) PingURL(c *gin.Context) {
	var req PingURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.accountTestService == nil {
		response.InternalError(c, "Account test service is not configured")
		return
	}

	proxyURL := ""
	if req.ProxyID != nil && *req.ProxyID > 0 {
		// 选了代理就必须用上：解析不出来直接报失败，绝不静默直连。
		if h.adminService == nil {
			response.Success(c, &service.URLPingResult{Success: false, Message: "proxy is not available"})
			return
		}
		proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID)
		if err != nil || proxy == nil {
			response.Success(c, &service.URLPingResult{Success: false, Message: "proxy not found"})
			return
		}
		proxyURL = proxy.URL()
	}

	response.Success(c, h.accountTestService.PingURL(c.Request.Context(), req.URL, proxyURL))
}
