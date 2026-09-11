package server

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/azuradara/bobr/internal/storage"
	"github.com/azuradara/bobr/internal/transform"
)

type Handler struct {
	cache       *cache.Cache
	hosts       map[string]*hostRouter
	driverCache map[string]storage.Driver
	maxAge      int
	mu          sync.RWMutex

	BytesOut    int64
	OriginCalls int64
}

type hostRouter struct {
	config  config.HostConfig
	isRoot  bool
	routes  map[string][]config.OriginConfig
	origins []config.OriginConfig
}

type requestContext struct {
	cacheKey            string
	transformParams     transform.Params
	effectivePath       string
	effectiveTransforms config.TransformsConfig
}

func NewHandler(c *cache.Cache, hosts map[string]config.HostConfig, maxAge int) *Handler {
	h := &Handler{
		cache:       c,
		hosts:       make(map[string]*hostRouter),
		driverCache: make(map[string]storage.Driver),
		maxAge:      maxAge,
	}

	for name, hostCfg := range hosts {
		router := &hostRouter{
			config: hostCfg,
			routes: make(map[string][]config.OriginConfig),
		}

		if len(hostCfg.Origins) > 0 {
			firstPrefix := hostCfg.Origins[0].Prefix
			if firstPrefix == "" || firstPrefix == "/" {
				router.isRoot = true
				router.origins = hostCfg.Origins
			} else {
				for _, o := range hostCfg.Origins {
					router.routes[o.Prefix] = append(router.routes[o.Prefix], o)
				}
			}
		}

		h.hosts[name] = router

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

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)

		return
	}

	router, ok := h.getHostRouter(w, r)
	if !ok {
		return
	}

	reqCtx, err := h.resolveRequest(r, router)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)

		return
	}

	if entry, err := h.cache.Get(reqCtx.cacheKey); err == nil {
		h.serveCacheHit(w, r, entry)

		return
	} else if !errors.Is(err, cache.ErrNotFound) {
		slog.Error("cache error", "err", err)
	}

	h.handleCacheMiss(w, r, router, reqCtx)
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

func (h *Handler) getHostRouter(w http.ResponseWriter, r *http.Request) (*hostRouter, bool) {
	router, ok := h.hosts[r.Host]
	if !ok {
		if def, ok := h.hosts["_default"]; ok {
			return def, true
		}

		http.Error(w, "Host not configured", http.StatusNotFound)

		return nil, false
	}

	return router, true
}

func (h *Handler) resolveRequest(
	r *http.Request,
	router *hostRouter,
) (requestContext, error) {
	path := r.URL.Path
	hostCfg := router.config
	effectiveTransforms := hostCfg.Transforms

	origins := h.selectOrigins(router, path)
	if len(origins) > 0 && origins[0].Transforms != nil {
		effectiveTransforms = *origins[0].Transforms
	}

	ctx := requestContext{
		effectivePath:       path,
		cacheKey:            r.Host + path,
		effectiveTransforms: effectiveTransforms,
	}

	if len(effectiveTransforms.ResizePresets) > 0 {
		//nolint:staticcheck // legacy feature
		if orig, width, ok := transform.ParsePreset(path, effectiveTransforms.ResizePresets); ok {
			ctx.transformParams = transform.Params{Width: width}
			ctx.effectivePath = orig
			ctx.cacheKey = r.Host + ctx.effectivePath + ctx.transformParams.CacheKey()
		} else {
			//nolint:staticcheck // legacy feature
			if transform.IsPresetCandidate(path) {
				return requestContext{}, errors.New("invalid preset")
			}
		}
	} else if effectiveTransforms.Resize {
		ctx.transformParams = transform.ParseParams(r.URL.Query())
		ctx.cacheKey += ctx.transformParams.CacheKey()
	}

	if effectiveTransforms.Optimize {
		ctx.cacheKey += "_opt"
	}

	if hostCfg.Bustable && r.URL.RawQuery != "" && ctx.transformParams.Empty() {
		ctx.cacheKey += "?" + r.URL.RawQuery
	}

	return ctx, nil
}

func (h *Handler) serveCacheHit(w http.ResponseWriter, r *http.Request, entry *cache.Entry) {
	defer func() { _ = entry.Body.Close() }()

	w.Header().Set("X-Cache", "HIT")
	h.setCommonHeaders(w, entry.ContentType, entry.ETag)

	http.ServeContent(w, r, "", entry.StoredAt, entry.Body)
	atomic.AddInt64(&h.BytesOut, entry.Size)
}

func (h *Handler) setCommonHeaders(w http.ResponseWriter, contentType, etag string) {
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(h.maxAge))

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	if etag != "" {
		w.Header().Set("ETag", etag)
	}
}

func (h *Handler) handleCacheMiss(
	w http.ResponseWriter,
	r *http.Request,
	router *hostRouter,
	reqCtx requestContext,
) {
	origins := h.selectOrigins(router, reqCtx.effectivePath)
	if len(origins) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)

		return
	}

	for i, originCfg := range origins {
		last := i == len(origins)-1

		obj, err := h.fetchOrigin(r, originCfg, reqCtx.effectivePath)
		if err != nil {
			if !last {
				continue
			}

			if errors.Is(err, storage.ErrNotFound) {
				http.Error(w, "Not Found", http.StatusNotFound)
			} else {
				http.Error(w, "Origin IO Error", http.StatusBadGateway)
			}

			return
		}

		defer func() { _ = obj.Body.Close() }()

		h.serveOrigin(w, r, reqCtx, obj)

		return
	}
}

func (h *Handler) fetchOrigin(
	r *http.Request,
	originCfg config.OriginConfig,
	path string,
) (*storage.Object, error) {
	originPath := path
	if originCfg.Prefix != "" && originCfg.Prefix != "/" {
		originPath = strings.TrimPrefix(path, originCfg.Prefix)
		if !strings.HasPrefix(originPath, "/") {
			originPath = "/" + originPath
		}
	}

	driver, err := h.getDriver(originCfg)
	if err != nil {
		slog.Error("failed to create driver", "err", err, "origin", originCfg.Name)

		return nil, err
	}

	atomic.AddInt64(&h.OriginCalls, 1)

	obj, err := driver.Fetch(r.Context(), originPath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			slog.Debug("origin object not found", "path", originPath, "origin", originCfg.Name)
		} else {
			slog.Error(
				"origin fetch failed",
				"err", err,
				"path", originPath,
				"origin", originCfg.Name,
			)
		}

		return nil, err
	}

	return obj, nil
}

// Objects that no transform will touch are streamed straight through, so a large
// file never lands in memory. Only transformable images are buffered.
func (h *Handler) serveOrigin(
	w http.ResponseWriter,
	r *http.Request,
	reqCtx requestContext,
	obj *storage.Object,
) {
	if !h.willTransform(reqCtx, obj.ContentType) {
		w.Header().Set("X-Cache", "MISS")
		h.setCommonHeaders(w, obj.ContentType, obj.ETag)

		if h.cache.Cacheable(obj.Size) {
			h.streamAndCache(w, r, reqCtx.cacheKey, obj)

			return
		}

		http.ServeContent(w, r, "", obj.LastModified, obj.Body)
		atomic.AddInt64(&h.BytesOut, obj.Size)

		return
	}

	dataBytes, err := io.ReadAll(obj.Body)
	if err != nil {
		slog.Error("failed to read origin body", "err", err)
		http.Error(w, "Origin IO Error", http.StatusBadGateway)

		return
	}

	dataBytes, contentType, size := h.processContent(
		reqCtx.effectiveTransforms,
		reqCtx.transformParams,
		dataBytes,
		obj.ContentType,
	)

	h.asyncCache(reqCtx.cacheKey, dataBytes, contentType)
	h.serveResponse(w, r, dataBytes, size, contentType)
}

func (h *Handler) streamAndCache(
	w http.ResponseWriter,
	r *http.Request,
	key string,
	obj *storage.Object,
) {
	dataBytes, err := io.ReadAll(obj.Body)
	if err != nil {
		slog.Error("failed to read origin body", "err", err)
		http.Error(w, "Origin IO Error", http.StatusBadGateway)

		return
	}

	h.asyncCache(key, dataBytes, obj.ContentType)

	http.ServeContent(w, r, "", obj.LastModified, bytes.NewReader(dataBytes))
	atomic.AddInt64(&h.BytesOut, int64(len(dataBytes)))
}

func (h *Handler) willTransform(reqCtx requestContext, contentType string) bool {
	if !transform.IsImage(contentType) {
		return false
	}

	t := reqCtx.effectiveTransforms

	return t.Optimize || (t.Resize && !reqCtx.transformParams.Empty())
}

func (h *Handler) selectOrigins(router *hostRouter, path string) []config.OriginConfig {
	if router.isRoot {
		return router.origins
	}

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return nil
	}

	requestPrefix := "/" + parts[1]

	return router.routes[requestPrefix]
}

func (h *Handler) processContent(
	transforms config.TransformsConfig,
	transformParams transform.Params,
	dataBytes []byte,
	contentType string,
) ([]byte, string, int64) {
	shouldTransform := transforms.Resize && !transformParams.Empty() &&
		transform.IsImage(contentType)
	shouldOptimize := transforms.Optimize && transform.IsImage(contentType)

	if shouldTransform || shouldOptimize {
		transformed, newContentType, err := transform.Apply(
			dataBytes,
			transformParams,
			shouldOptimize,
			transforms.Lossless,
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
	r *http.Request,
	data []byte,
	size int64,
	contentType string,
) {
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	w.Header().Set("X-Cache", "MISS")
	h.setCommonHeaders(w, contentType, "")

	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	atomic.AddInt64(&h.BytesOut, size)
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
