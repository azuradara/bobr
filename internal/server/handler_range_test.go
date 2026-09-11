package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/azuradara/bobr/internal/storage"
	"github.com/stretchr/testify/assert"
)

type staticDriver struct {
	body        []byte
	contentType string
}

func (d *staticDriver) Fetch(_ context.Context, _ string) (*storage.Object, error) {
	return &storage.Object{
		Body:        nopSeekCloser{bytes.NewReader(d.body)},
		Size:        int64(len(d.body)),
		ContentType: d.contentType,
	}, nil
}

func rangeHandler(t *testing.T, body []byte, contentType string) *Handler {
	t.Helper()

	tmpDir, _ := os.MkdirTemp("", "bobr-range-test")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	c, err := cache.New(config.CacheConfig{
		Dir:     tmpDir + "/blob",
		DbDir:   tmpDir + "/db",
		MaxSize: "10MB",
	})
	assert.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	hosts := map[string]config.HostConfig{
		"example.com": {
			Origins: []config.OriginConfig{{Name: "vid", Type: "s3"}},
		},
	}

	h := NewHandler(c, hosts, 14400)
	h.driverCache["vid"] = &staticDriver{body: body, contentType: contentType}

	return h
}

func TestHandler_RangeRequestOnMiss(t *testing.T) {
	body := []byte("0123456789abcdefghij")
	h := rangeHandler(t, body, "video/mp4")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/clip.mp4", nil)
	req.Header.Set("Range", "bytes=5-9")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	res := w.Result()
	assert.Equal(t, http.StatusPartialContent, res.StatusCode)
	assert.Equal(t, "bytes 5-9/20", res.Header.Get("Content-Range"))
	assert.Equal(t, "bytes", res.Header.Get("Accept-Ranges"))

	got, _ := io.ReadAll(res.Body)
	assert.Equal(t, "56789", string(got))
}

func TestHandler_HeadSendsNoBody(t *testing.T) {
	body := []byte("0123456789abcdefghij")
	h := rangeHandler(t, body, "video/mp4")

	req := httptest.NewRequest(http.MethodHead, "http://example.com/clip.mp4", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	res := w.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	got, _ := io.ReadAll(res.Body)
	assert.Empty(t, got)
}

func TestHandler_RejectsWriteMethods(t *testing.T) {
	h := rangeHandler(t, []byte("x"), "video/mp4")

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "http://example.com/clip.mp4", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode, m)
	}
}

func TestHandler_CacheControlAndConditional(t *testing.T) {
	body := []byte("hello world")
	h := rangeHandler(t, body, "text/plain")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/a.txt", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, "public, max-age=14400", w.Header().Get("Cache-Control"))

	etag := waitForCachedETag(t, h, "example.com/a.txt")

	req = httptest.NewRequest(http.MethodGet, "http://example.com/a.txt", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotModified, w.Result().StatusCode)
}

func waitForCachedETag(t *testing.T, h *Handler, key string) string {
	t.Helper()

	for range 100 {
		if entry, err := h.cache.Get(key); err == nil {
			_ = entry.Body.Close()

			if entry.ETag != "" {
				return entry.ETag
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("object never reached the cache under %s", key)

	return ""
}
