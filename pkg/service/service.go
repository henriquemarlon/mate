// Package service provides the daemon building blocks Mate uses to run
// long-lived components: a base template with structured logging and a
// tick-based template that re-runs an implementation on a fixed interval.
//
// Adapted from the Cartesi Rollups Node service framework
// (github.com/cartesi/rollups-node, pkg/service, Apache-2.0).
package service

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

var (
	ErrInvalid = errors.New("invalid argument")
	// ErrServiceStopped aborts Serve when a tick reports that the service
	// cannot make progress anymore (for example, a dead subprocess).
	ErrServiceStopped = errors.New("service stopped unexpectedly")
)

// SupervisedService is the minimal lifecycle a runnable service exposes.
type SupervisedService interface {
	String() string
	Serve(context.Context) error
	Ready() bool
	Teardown()
}

// BaseTemplate is embedded by concrete services to inherit identity,
// logging, and default lifecycle implementations.
type BaseTemplate struct {
	Name   string
	Logger *slog.Logger
}

// BaseConfigs stores configuration for InitServiceTemplate.
type BaseConfigs struct {
	Name     string
	Logger   *slog.Logger
	LogLevel slog.Level
	LogColor bool
}

// InitServiceTemplate initializes a BaseTemplate, building a logger when the
// configuration does not provide one.
func InitServiceTemplate(s *BaseTemplate, c *BaseConfigs) {
	s.Name = c.Name
	s.Logger = c.Logger
	if s.Logger == nil {
		s.Logger = NewLogger(c.Name, c.LogLevel, c.LogColor)
	}
}

// Default implementations of the SupervisedService methods except Serve.
func (s *BaseTemplate) String() string { return s.Name }
func (s *BaseTemplate) Ready() bool    { return true }
func (s *BaseTemplate) Teardown()      {}

func NewLogger(name string, level slog.Level, color bool) *slog.Logger {
	opts := &tint.Options{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
		NoColor:    !color,
	}
	return slog.New(tint.NewHandler(os.Stdout, opts)).With("service", name)
}
