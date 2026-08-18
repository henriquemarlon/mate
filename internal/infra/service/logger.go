package service

import (
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

func NewLogger(level slog.Level, color bool) *slog.Logger {
	opts := &tint.Options{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
		NoColor:    !color,
	}
	handler := tint.NewHandler(os.Stdout, opts)
	return slog.New(handler)
}

func NewServiceLogger(name string, level slog.Level, color bool) *slog.Logger {
	return NewLogger(level, color).With("service", name)
}
