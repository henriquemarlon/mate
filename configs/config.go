package configs

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Redacted[T any] struct {
	Value T
}

func (r Redacted[T]) String() string {
	return "[REDACTED]"
}

type (
	Duration       = time.Duration
	LogLevel       = slog.Level
	RedactedString = Redacted[string]
)

func ToStringFromString(s string) (string, error) {
	return s, nil
}

func ToDurationFromSeconds(s string) (time.Duration, error) {
	return time.ParseDuration(s + "s")
}

func ToLogLevelFromString(s string) (LogLevel, error) {
	var m = map[string]LogLevel{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	if v, ok := m[s]; ok {
		return v, nil
	}
	var zeroValue LogLevel
	return zeroValue, fmt.Errorf("invalid log level '%s'", s)
}

func ToRedactedStringFromString(s string) (RedactedString, error) {
	return RedactedString{s}, nil
}

var (
	toBool           = strconv.ParseBool
	toString         = ToStringFromString
	toDuration       = ToDurationFromSeconds
	toLogLevel       = ToLogLevelFromString
	toRedactedString = ToRedactedStringFromString
)

// ExpandPath expands ~ to the user's home directory in a file path.
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home dir: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	return path, nil
}

var (
	notDefinedbool           = func() bool { return false }
	notDefinedstring         = func() string { return "" }
	notDefinedDuration       = func() time.Duration { return 0 }
	notDefinedLogLevel       = func() slog.Level { return slog.LevelInfo }
	notDefinedRedactedString = func() RedactedString { return RedactedString{""} }
)
