package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestServer_Routes(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-server-route-test")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cacheCfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}
	c, err := cache.New(cacheCfg)
	assert.NoError(t, err)
	defer func() { _ = c.Close() }()

	cfg := &config.Config{
		Listen: ":0",
	}
	hosts := map[string]config.HostConfig{}

	srv := New(cfg, c, hosts)
	handler := srv.httpServer.Handler

	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Result().StatusCode)

	req = httptest.NewRequest("GET", "/robots.txt", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "Disallow:")

	req = httptest.NewRequest("GET", "/some/path", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	req = httptest.NewRequest("GET", "/metrics", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "bobr_cache_hits_total")
}

func TestServer_Middleware(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-server-mw-test")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cacheCfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}
	c, err := cache.New(cacheCfg)
	assert.NoError(t, err)
	defer func() { _ = c.Close() }()

	cfg := &config.Config{Listen: ":0"}
	hosts := map[string]config.HostConfig{}

	srv := New(cfg, c, hosts)

	var mwCalled bool
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwCalled = true
			next.ServeHTTP(w, r)
		})
	})

	handler := srv.httpServer.Handler

	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, mwCalled, "Middleware should be called")
	assert.Equal(t, http.StatusNoContent, w.Result().StatusCode)
}
