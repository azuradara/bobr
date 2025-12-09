package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/azuradara/bobr/internal/logger"
	"github.com/azuradara/bobr/internal/server"
)

func main() {
	cfgpath := flag.String("config", "config/config.yaml", "a config file")
	flag.StringVar(cfgpath, "c", *cfgpath, "alias for -config")
	flag.Parse()

	cfg, err := config.Load(*cfgpath)
	if err != nil {
		log.Fatalf("could not parse config file: %v", err)
	}

	if err := logger.Init(cfg.Logger); err != nil {
		log.Fatalf("could not init logger: %v", err)
	}

	c, err := cache.New(cfg.Cache)
	if err != nil {
		log.Fatalf("could not init cache: %v", err)
	}

	defer func() { _ = c.Close() }()

	srv := server.New(cfg, c, cfg.Hosts)

	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	slog.Info("server listening", slog.String("address", cfg.Listen))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("stopped")
}
