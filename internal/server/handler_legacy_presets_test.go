package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/azuradara/bobr/internal/storage"
	"github.com/stretchr/testify/assert"
)

type MockDriver struct {
	files map[string][]byte
}

func (m *MockDriver) Fetch(ctx context.Context, path string) (io.ReadCloser, int64, string, error) {
	if content, ok := m.files[path]; ok {
		return io.NopCloser(
				strings.NewReader(string(content)),
			), int64(
				len(content),
			), "image/png", nil
	}

	if !strings.HasPrefix(path, "/") {
		if content, ok := m.files["/"+path]; ok {
			return io.NopCloser(
					strings.NewReader(string(content)),
				), int64(
					len(content),
				), "image/png", nil
		}
	}

	return nil, 0, "", storage.ErrNotFound
}

func TestLegacyPresets(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "bobr-legacy-test")
	defer func() { _ = os.RemoveAll(tmpDir) }()
	c, err := cache.New(
		config.CacheConfig{Dir: tmpDir + "/blob", DbDir: tmpDir + "/db", MaxSize: "10MB"},
	)
	assert.NoError(t, err)
	defer func() { _ = c.Close() }()

	hosts := map[string]config.HostConfig{
		"legacy.test": {
			Transforms: config.TransformsConfig{
				ResizePresets: map[string]int{
					"md": 100,
				},
			},
			Origins: []config.OriginConfig{
				{Name: "mock", Prefix: "/"},
			},
		},
		"standard.test": {
			Transforms: config.TransformsConfig{
				Resize: true,
			},
			Origins: []config.OriginConfig{
				{Name: "mock", Prefix: "/"},
			},
		},
	}

	h := NewHandler(c, hosts)

	driver := &MockDriver{
		files: map[string][]byte{
			"/image.png": []byte("fake_png_data"),
		},
	}
	h.driverCache["mock"] = driver

	tests := []struct {
		name          string
		host          string
		path          string
		query         string
		expectStatus  int
		expectContent string
	}{
		{
			name:          "Preset Match",
			host:          "legacy.test",
			path:          "/image_md.png",
			expectStatus:  http.StatusOK,
			expectContent: "fake_png_data",
		},
		{
			name:         "Invalid 2-char suffix (Strict 404)",
			host:         "legacy.test",
			path:         "/image_xy.png",
			expectStatus: http.StatusNotFound,
		},
		{
			name:          "No Preset Suffix (Original)",
			host:          "legacy.test",
			path:          "/image.png",
			expectStatus:  http.StatusOK,
			expectContent: "fake_png_data",
		},
		{
			name:          "Free Params Ignored when Presets Enabled",
			host:          "legacy.test",
			path:          "/image.png",
			query:         "?width=50",
			expectStatus:  http.StatusOK,
			expectContent: "fake_png_data",
		},
		{
			name:         "Standard Free Transform (Control Group)",
			host:         "standard.test",
			path:         "/image.png",
			query:        "?width=50",
			expectStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://" + tt.host + tt.path + tt.query
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode)
			if tt.expectContent != "" && w.Result().StatusCode == http.StatusOK {
				assert.Equal(t, tt.expectContent, w.Body.String())
			}
		})
	}
}
