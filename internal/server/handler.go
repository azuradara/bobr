package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	hostCfg, ok := h.hosts[r.Host]

	if !ok {
		if def, ok := h.hosts["_default"]; ok {
			hostCfg = def
		} else {
			http.Error(w, "Host not configured", http.StatusNotFound)
			return
		}
	}

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

	data, size, contentType, err := h.cache.Get(cacheKey)

	if err == nil {
		defer func() { _ = data.Close() }()

		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}

		n, _ := io.Copy(w, data)
		atomic.AddInt64(&h.BytesOut, n)

		return
	}

	if err != cache.ErrNotFound {
		slog.Error("cache error", "err", err)
	}

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

	body, size, contentType, err := driver.Fetch(r.Context(), path)
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

	shouldTransform := hostCfg.Transform && !transformParams.Empty() && transform.IsImage(contentType)
	shouldOptimize := hostCfg.Optimize && transform.IsImage(contentType)

	if shouldTransform || shouldOptimize {
		transformed, newContentType, err := transform.Apply(dataBytes, transformParams, shouldOptimize)

		if err != nil {
			slog.Error("transform failed", "err", err)
		} else {
			dataBytes = transformed
			size = int64(len(dataBytes))
			contentType = newContentType
		}
	}

	finalContentType := contentType
	if finalContentType == "" {
		finalContentType = http.DetectContentType(dataBytes)
	}

	go func() {
		if err := h.cache.Set(cacheKey, dataBytes, finalContentType); err != nil {
			slog.Warn("failed to cache", "key", cacheKey, "err", err)
		}
	}()

	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", http.DetectContentType(dataBytes))
	}

	n, _ := w.Write(dataBytes)
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
