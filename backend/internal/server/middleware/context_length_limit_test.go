package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func runContextLengthLimit(t *testing.T, limit, contentLength int64) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/responses", ContextLengthLimit(limit), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.ContentLength = contentLength
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestContextLengthLimitRejectsOversizedDeclaredBody(t *testing.T) {
	rec := runContextLengthLimit(t, 2<<20, 2<<20+1)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"context_length_exceeded"`)
}

func TestContextLengthLimitAllowsBodyAtLimit(t *testing.T) {
	rec := runContextLengthLimit(t, 2<<20, 2<<20)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestContextLengthLimitDisabled(t *testing.T) {
	rec := runContextLengthLimit(t, 0, 1<<30)
	require.Equal(t, http.StatusNoContent, rec.Code)
}
