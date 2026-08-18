// Package configs manages Mate configuration sourced from environment variables.
// The generate sub-package declares those variables and produces generated.go.
package configs

//go:generate go run ./generate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type (
	Bool      = bool
	LogLevel  = slog.Level
	Path      = string
	RenderDPI = int
	Seconds   = time.Duration
)

func ToStringFromString(value string) (string, error) {
	return value, nil
}

func ToBoolFromString(value string) (Bool, error) {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q", value)
	}
	return parsed, nil
}

func ToSecondsFromString(value string) (Seconds, error) {
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if seconds < 0 {
		return 0, fmt.Errorf("must not be negative, got %d", seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

func ToRenderDPIFromString(value string) (RenderDPI, error) {
	dpi, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if dpi < 72 || dpi > 600 {
		return 0, fmt.Errorf("must be between 72 and 600, got %d", dpi)
	}
	return dpi, nil
}

func ToLogLevelFromString(value string) (LogLevel, error) {
	levels := map[string]LogLevel{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	if level, ok := levels[strings.ToLower(value)]; ok {
		return level, nil
	}
	return slog.LevelInfo, fmt.Errorf("invalid log level %q", value)
}

func ToPathFromString(value string) (Path, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if value == "~" {
			return home, nil
		}
		return filepath.Join(home, value[2:]), nil
	}
	return filepath.Clean(value), nil
}

var (
	toBool      = ToBoolFromString
	toLogLevel  = ToLogLevelFromString
	toPath      = ToPathFromString
	toRenderDPI = ToRenderDPIFromString
	toSeconds   = ToSecondsFromString
	toString    = ToStringFromString
)

var (
	notDefinedBool      = func() Bool { return true }
	notDefinedLogLevel  = func() LogLevel { return slog.LevelInfo }
	notDefinedPath      = func() Path { return "" }
	notDefinedRenderDPI = func() RenderDPI { return 0 }
	notDefinedSeconds   = func() Seconds { return 0 }
	notDefinedstring    = func() string { return "" }
)
