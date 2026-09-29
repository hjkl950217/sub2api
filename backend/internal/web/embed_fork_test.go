//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FORK: 本文件只覆盖 fork 对 embed_on.go 的改动，与上游文件分开，减少合并冲突。
// 需 -tags=embed 且有 dist 才能编译运行：
//   cd backend && go test -tags=embed -run TestFork ./internal/web/

// data/public 覆盖目录优先于内嵌资源，且允许投放内嵌 dist 中不存在的新文件。
func TestForkOverridePrecedesEmbeddedLookup(t *testing.T) {
	t.Parallel()

	provider := &mockSettingsProvider{settings: map[string]string{"test": "value"}}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)

	overrideDir := t.TempDir()
	server.overrideDir = overrideDir
	require.NoError(t, os.WriteFile(filepath.Join(overrideDir, "fork-probe.js"), []byte("OVERRIDE-NEW-FILE"), 0o644))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.CSPNonceKey, "test-nonce")
		c.Next()
	})
	router.Use(server.Middleware())

	// 该路径不在内嵌 dist 里：改动前会被当成 SPA 路由回落成 index.html。
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fork-probe.js", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OVERRIDE-NEW-FILE", w.Body.String())
	assert.Empty(t, w.Header().Get("Cache-Control"), "覆盖文件不应带 immutable 缓存头")
}

// data/public/index.html 可替换内嵌首页，并在 mtime/size 变化后自动失效缓存。
func TestForkIndexHTMLOverrideAndCacheInvalidation(t *testing.T) {
	t.Parallel()

	provider := &mockSettingsProvider{settings: map[string]string{"test": "value"}}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)

	overrideDir := t.TempDir()
	server.overrideDir = overrideDir
	indexPath := filepath.Join(overrideDir, "index.html")

	const firstPage = "<html><head><title>OVERRIDE-1</title></head><body>one</body></html>"
	require.NoError(t, os.WriteFile(indexPath, []byte(firstPage), 0o644))

	serve := func() string {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.CSPNonceKey, "test-nonce")
		server.serveIndexHTML(c)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.String()
	}

	first := serve()
	assert.Contains(t, first, "OVERRIDE-1")
	assert.NotContains(t, first, "Sub2API - AI API Gateway")

	// 内容变更 + mtime 前移，应重新加载并失效已渲染缓存。
	const secondPage = "<html><head><title>OVERRIDE-2</title></head><body>two</body></html>"
	require.NoError(t, os.WriteFile(indexPath, []byte(secondPage), 0o644))
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(indexPath, future, future))

	second := serve()
	assert.Contains(t, second, "OVERRIDE-2")
	assert.NotContains(t, second, "OVERRIDE-1")

	// 覆盖文件被移除后应回退到内嵌首页。
	require.NoError(t, os.Remove(indexPath))
	third := serve()
	assert.Contains(t, third, "Sub2API - AI API Gateway")
	assert.NotContains(t, third, "OVERRIDE-2")
}
