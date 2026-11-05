package middleware

import (
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// RequestBodyLimit 限制传输正文大小，并沿用默认解压上限。
func RequestBodyLimit(maxBytes int64, budgets ...*pkghttputil.BodyMemoryBudget) gin.HandlerFunc {
	return RequestBodyLimitWithDecompression(maxBytes, httputil.DefaultMaxDecompressedBodySize, budgets...)
}

// RequestBodyLimitWithDecompression 分别限制传输正文和解压正文。
// 解压上限不能超过该路由自身的正文限制，防止压缩绕过纯文本接口的较小上限。
// 此处只绑定策略，不提前读体，保留鉴权、白名单和合成路由的原有执行顺序。
func RequestBodyLimitWithDecompression(maxBytes, maxDecompressedBytes int64, budgets ...*pkghttputil.BodyMemoryBudget) gin.HandlerFunc {
	if maxDecompressedBytes <= 0 {
		maxDecompressedBytes = httputil.DefaultMaxDecompressedBodySize
	}
	if maxBytes >= 0 && maxBytes < maxDecompressedBytes {
		maxDecompressedBytes = maxBytes
	}
	return func(c *gin.Context) {
		if len(budgets) > 0 {
			ctx, release := pkghttputil.WithRequestBodyMemoryBudget(c.Request.Context(), budgets[0])
			defer release()
			c.Request = c.Request.WithContext(ctx)
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Request = c.Request.WithContext(httputil.WithMaxDecompressedBodySize(c.Request.Context(), maxDecompressedBytes))
		c.Next()
	}
}
