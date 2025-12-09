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

func TestHandler_CORS(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-handler-test")

	defer func() { _ = os.RemoveAll(tmpDir) }()

	cacheCfg := config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	}

	c, err := cache.New(cacheCfg)
	assert.NoError(t, err)

	defer func() { _ = c.Close() }()

	hosts := map[string]config.HostConfig{}
	h := NewHandler(c, hosts)

	req := httptest.NewRequest(http.MethodOptions, "http://example.com/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
}
