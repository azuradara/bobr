package server

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
)

type Middleware func(http.Handler) http.Handler

type Server struct {
	httpServer *http.Server
	middleware []Middleware
	mux        *http.ServeMux
	handler    *Handler
	startTime  time.Time
}

func New(cfg *config.Config, cache *cache.Cache, hosts map[string]config.HostConfig) *Server {
	mux := http.NewServeMux()
	proxyHandler := NewHandler(cache, hosts)

	srv := &Server{
		middleware: []Middleware{},
		mux:        mux,
		handler:    proxyHandler,
		startTime:  time.Now(),
	}

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow:\n"))
	})

	statsHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		uptime := time.Since(srv.startTime).Seconds()
		c := cache
		baseStats := c.Stats()

		extraStats := fmt.Sprintf(`# HELP bobr_uptime_seconds Uptime in seconds
# TYPE bobr_uptime_seconds gauge
bobr_uptime_seconds %.2f
# HELP bobr_bytes_out_total Total bytes written to clients
# TYPE bobr_bytes_out_total counter
bobr_bytes_out_total %d
# HELP bobr_s3_calls_total Total origin fetch calls
# TYPE bobr_s3_calls_total counter
bobr_s3_calls_total %d
# HELP bobr_cache_flushed_total Total number of evicted items
# TYPE bobr_cache_flushed_total counter
bobr_cache_flushed_total %d
`,
			uptime,
			atomic.LoadInt64(&srv.handler.BytesOut),
			atomic.LoadInt64(&srv.handler.OriginCalls),
			atomic.LoadInt64(&c.Flushed),
		)

		_, _ = w.Write([]byte(baseStats + extraStats))
	}

	mux.HandleFunc("/metrics", statsHandler)
	mux.HandleFunc("/_stats", statsHandler)

	mux.Handle("/", proxyHandler)

	srv.httpServer = &http.Server{
		Addr: cfg.Listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var final http.Handler = srv.mux
			for i := len(srv.middleware) - 1; i >= 0; i-- {
				final = srv.middleware[i](final)
			}
			final.ServeHTTP(w, r)
		}),
	}

	return srv
}

func (s *Server) Use(mw Middleware) {
	s.middleware = append(s.middleware, mw)
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
