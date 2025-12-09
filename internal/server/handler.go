package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/azuradara/bobr/internal/storage"
	"github.com/azuradara/bobr/internal/transform"
)

type Handler struct {
	cache       *cache.Cache
	hosts       map[string]config.HostConfig
	driverCache map[string]storage.Driver
	mu          sync.RWMutex

	BytesOut    int64
	OriginCalls int64
}

func NewHandler(c *cache.Cache, hosts map[string]config.HostConfig) *Handler {
	h := &Handler{
		cache:       c,
		hosts:       hosts,
		driverCache: make(map[string]storage.Driver),
	}

	for _, hostCfg := range hosts {
		for _, originCfg := range hostCfg.Origins {
			if _, exists := h.driverCache[originCfg.Name]; !exists {
				if f, err := storage.NewDriver(originCfg); err == nil {
					h.driverCache[originCfg.Name] = f
				}
			}
		}
	}

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.handleOptions(w, r) {
		return
	}

	hostCfg, ok := h.getHostConfig(w, r)
	if !ok {
		return
	}

	cacheKey, transformParams := h.getCacheKey(r, hostCfg)

	if data, size, contentType, err := h.cache.Get(cacheKey); err == nil {
		h.serveCacheHit(w, data, size, contentType)

		return
	} else if !errors.Is(err, cache.ErrNotFound) {
		slog.Error("cache error", "err", err)
	}

	h.handleCacheMiss(w, r, hostCfg, cacheKey, transformParams)
}

func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)

		return true
	}

	return false
}

func (h *Handler) getHostConfig(w http.ResponseWriter, r *http.Request) (config.HostConfig, bool) {
	hostCfg, ok := h.hosts[r.Host]
	if !ok {
		if def, ok := h.hosts["_default"]; ok {
			return def, true
		}

		http.Error(w, "Host not configured", http.StatusNotFound)

		return config.HostConfig{}, false
	}

	return hostCfg, true
}

func (h *Handler) getCacheKey(
	r *http.Request,
	hostCfg config.HostConfig,
) (string, transform.Params) {
	path := r.URL.Path
	cacheKey := r.Host + path

	var transformParams transform.Params

	if hostCfg.Transform {
		transformParams = transform.ParseParams(r.URL.Query())
		cacheKey += transformParams.CacheKey()
	}

	if hostCfg.Optimize {
		cacheKey += "_opt"
	}

	if hostCfg.Bustable && r.URL.RawQuery != "" && transformParams.Empty() {
		cacheKey += "?" + r.URL.RawQuery
	}

	return cacheKey, transformParams
}

func (h *Handler) serveCacheHit(
	w http.ResponseWriter,
	data io.ReadCloser,
	size int64,
	contentType string,
) {
	defer func() { _ = data.Close() }()

	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	n, _ := io.Copy(w, data)
	atomic.AddInt64(&h.BytesOut, n)
}

func (h *Handler) handleCacheMiss(
	w http.ResponseWriter,
	r *http.Request,
	hostCfg config.HostConfig,
	cacheKey string,
	transformParams transform.Params,
) {
	if len(hostCfg.Origins) == 0 {
		http.Error(w, "No origins configured", http.StatusServiceUnavailable)

		return
	}

	originCfg := hostCfg.Origins[0]

	driver, err := h.getDriver(originCfg)
	if err != nil {
		slog.Error("failed to create driver", "err", err)
		http.Error(w, "Origin Error", http.StatusBadGateway)

		return
	}

	atomic.AddInt64(&h.OriginCalls, 1)

	path := r.URL.Path

	body, _, contentType, err := driver.Fetch(r.Context(), path)
	if err != nil {
		slog.Error("origin fetch failed", "err", err, "path", path)
		http.Error(w, "Not Found", http.StatusNotFound)

		return
	}

	defer func() { _ = body.Close() }()

	dataBytes, err := io.ReadAll(body)
	if err != nil {
		slog.Error("failed to read origin body", "err", err)
		http.Error(w, "Origin IO Error", http.StatusBadGateway)

		return
	}

	dataBytes, contentType, size := h.processContent(
		hostCfg,
		transformParams,
		dataBytes,
		contentType,
	)

	h.asyncCache(cacheKey, dataBytes, contentType)
	h.serveResponse(w, dataBytes, size, contentType)
}

func (h *Handler) processContent(
	hostCfg config.HostConfig,
	transformParams transform.Params,
	dataBytes []byte,
	contentType string,
) ([]byte, string, int64) {
	shouldTransform := hostCfg.Transform && !transformParams.Empty() &&
		transform.IsImage(contentType)
	shouldOptimize := hostCfg.Optimize && transform.IsImage(contentType)

	if shouldTransform || shouldOptimize {
		transformed, newContentType, err := transform.Apply(
			dataBytes,
			transformParams,
			shouldOptimize,
		)
		if err != nil {
			slog.Error("transform failed", "err", err)
		} else {
			return transformed, newContentType, int64(len(transformed))
		}
	}

	return dataBytes, contentType, int64(len(dataBytes))
}

func (h *Handler) asyncCache(key string, data []byte, contentType string) {
	finalContentType := contentType
	if finalContentType == "" {
		finalContentType = http.DetectContentType(data)
	}

	go func() {
		err := h.cache.Set(key, data, finalContentType)
		if err != nil {
			slog.Warn("failed to cache", "key", key, "err", err)
		}
	}()
}

func (h *Handler) serveResponse(
	w http.ResponseWriter,
	data []byte,
	size int64,
	contentType string,
) {
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", http.DetectContentType(data))
	}

	n, _ := w.Write(data)
	atomic.AddInt64(&h.BytesOut, int64(n))
}

func (h *Handler) getDriver(cfg config.OriginConfig) (storage.Driver, error) {
	key := cfg.Name

	h.mu.RLock()
	f, ok := h.driverCache[key]
	h.mu.RUnlock()

	if ok {
		return f, nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if f, ok := h.driverCache[key]; ok {
		return f, nil
	}

	f, err := storage.NewDriver(cfg)
	if err != nil {
		return nil, err
	}

	h.driverCache[key] = f

	return f, nil
}

func (h *Handler) Fetch(
	ctx context.Context,
	driver storage.Driver,
	path string,
) (io.ReadCloser, int64, string, error) {
	return driver.Fetch(ctx, path)
}
