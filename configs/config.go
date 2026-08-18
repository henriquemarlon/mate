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
)

type (
	Bool      = bool
	LogLevel  = slog.Level
	Path      = string
	RenderDPI = int
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
	toString    = ToStringFromString
)

var (
	notDefinedBool      = func() Bool { return true }
	notDefinedLogLevel  = func() LogLevel { return slog.LevelInfo }
	notDefinedPath      = func() Path { return "" }
	notDefinedRenderDPI = func() RenderDPI { return 0 }
	notDefinedstring    = func() string { return "" }
)
