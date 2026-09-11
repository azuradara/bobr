package server

import (
	"context"
	"net/http"
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
	proxyHandler := NewHandler(cache, hosts, cfg.MaxAgeSeconds())

	srv := &Server{
		middleware: []Middleware{},
		mux:        mux,
		handler:    proxyHandler,
		startTime:  time.Now(),
	}

	mux.HandleFunc("/favicon.ico", NewFaviconHandler())
	mux.HandleFunc("/robots.txt", NewRobotsHandler())

	statsHandler := guard(NewAllowlist(cfg.Metrics.Allow), NewStatsHandler(srv, cache))
	mux.HandleFunc("/metrics", statsHandler)
	mux.HandleFunc("/_stats", statsHandler)
	mux.HandleFunc("/healthz", NewHealthHandler())

	mux.Handle("/", proxyHandler)

	srv.httpServer = &http.Server{
		Addr:              cfg.Listen,
		ReadHeaderTimeout: cfg.Timeouts.ReadTimeout(),
		ReadTimeout:       cfg.Timeouts.ReadTimeout(),
		WriteTimeout:      cfg.Timeouts.WriteTimeout(),
		IdleTimeout:       cfg.Timeouts.IdleTimeout(),
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

func guard(allow *Allowlist, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allow.Allows(r) {
			http.Error(w, "Not Found", http.StatusNotFound)

			return
		}

		next(w, r)
	}
}
