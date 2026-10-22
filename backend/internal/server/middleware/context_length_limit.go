package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RejectContextLengthExceeded rejects requests whose declared Content-Length
// exceeds maxBytes before the body is read. It is exposed as a plain helper so
// route-level middleware can run it without nesting another gin chain.
// maxBytes <= 0 disables the check. Returns true when the request was aborted.
// FORK-ANCHOR: gateway-context-length-limit
func RejectContextLengthExceeded(c *gin.Context, maxBytes int64) bool {
	if maxBytes <= 0 || c == nil || c.Request == nil || c.Request.ContentLength <= maxBytes {
		return false
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"message": "Request body exceeds the gateway limit. Please compact your session or start a new one.",
			"type":    "invalid_request_error",
			"code":    "context_length_exceeded",
		},
	})
	return true
}

// ContextLengthLimit is the middleware form used on plain routes. Codex CLI
// only auto-compacts on a context_length_exceeded error, so a slow intermediate
// upload that times out otherwise leaves the client retrying the same
// oversized body forever (#7692).
func ContextLengthLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if RejectContextLengthExceeded(c, maxBytes) {
			return
		}
		c.Next()
	}
}
