package cli

import (
	"context"
	"errors"
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
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "bobr",
	Short: "bobr cde",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().
		StringVarP(&cfgFile, "config", "c", "config/config.yaml", "config file path")
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(purgeCmd)
	rootCmd.AddCommand(versionCmd)
}

func runServer() error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	if err := logger.Init(cfg.Logger); err != nil {
		return err
	}

	c, err := cache.New(cfg.Cache)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	srv := server.New(cfg, c, cfg.Hosts)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

		return err
	}

	slog.Info("stopped")

	return nil
}
