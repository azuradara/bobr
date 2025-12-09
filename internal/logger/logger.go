package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/azuradara/bobr/internal/config"
)

var logLevelMap = map[string]slog.Level{
	"DEBUG":   slog.LevelDebug,
	"INFO":    slog.LevelInfo,
	"WARN":    slog.LevelWarn,
	"WARNING": slog.LevelWarn,
	"ERROR":   slog.LevelError,
}

func Init(cfg config.LoggerConfig) error {
	slevel, err := parseLogLevel(cfg.Level)
	if err != nil {
		return err
	}

	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				AddSource: true,
				Level:     slevel,
			},
		),
	)

	slog.SetDefault(logger)

	return nil
}

func parseLogLevel(level string) (slog.Level, error) {
	normalized := strings.ToUpper(strings.TrimSpace(level))

	if l, ok := logLevelMap[normalized]; ok {
		return l, nil
	}

	return slog.LevelInfo, fmt.Errorf("unknown log level: %s", level)
}
