// Package codex controls one persistent Codex App Server subprocess and
// exposes a narrow execution contract on top of it. The package mirrors the
// machine-control layering of Cartesi Rollups Node: codex.go holds the public
// contract and lifecycle, backend.go the narrow process/protocol boundary,
// implementation.go the private App Server semantics, and appserver.go the
// concrete stdio transport.
package codex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// DefaultStartupTimeout bounds the initialize handshake when the caller's
// context has no earlier deadline.
const DefaultStartupTimeout = 15 * time.Second

// DefaultTurnInactivityTimeout bounds the silence interval between app-server
// notifications during a turn. It is an inactivity window reset on every
// received event, not a cap on total turn duration.
const DefaultTurnInactivityTimeout = 5 * time.Minute

// Sentinel errors let callers map failures to policy (retry, review, abort)
// instead of parsing messages.
var (
	// ErrClosed reports a clean shutdown initiated by Close.
	ErrClosed = errors.New("codex: client is closed")
	// ErrStopped reports that the app-server process stopped on its own.
	ErrStopped = errors.New("codex: app server stopped")
	// ErrTurnFailed reports a turn the app server completed with a failure.
	ErrTurnFailed = errors.New("codex: turn failed")
	// ErrTurnInterrupted reports a turn cancelled or interrupted server-side.
	ErrTurnInterrupted = errors.New("codex: turn interrupted")
	// ErrTurnStalled reports no app-server activity within the inactivity window.
	ErrTurnStalled = errors.New("codex: turn stalled")
	// ErrNoAgentMessage reports a completed turn without a final agent message.
	ErrNoAgentMessage = errors.New("codex: completed turn returned no agent message")
	// ErrInvalidResponse reports a final agent message that is not valid JSON.
	ErrInvalidResponse = errors.New("codex: final response is not valid JSON")
)

// Request is one isolated unit of model work: a prompt, an optional local
// image, and the JSON schema the final answer must match.
type Request struct {
	Prompt    string
	ImageData []byte
	MediaType string
	Schema    []byte
}

// Codex is the public contract for executing isolated requests against a
// persistent Codex App Server process.
type Codex interface {
	// Execute runs one ephemeral thread with a single turn and returns the
	// validated JSON produced by the model.
	Execute(ctx context.Context, request Request) ([]byte, error)
	// Close terminates the underlying app-server process. It is idempotent.
	Close() error
}

// Config describes how to construct a Codex instance.
type Config struct {
	// Binary is the Codex CLI path or executable name. Empty means "codex".
	Binary string
	// StartupTimeout bounds the initialize handshake. Zero means
	// DefaultStartupTimeout.
	StartupTimeout time.Duration
	// TurnInactivityTimeout bounds the silence interval between app-server
	// events during a turn. Zero means DefaultTurnInactivityTimeout.
	TurnInactivityTimeout time.Duration
	// Logger receives thread/turn lifecycle records. Nil means discard.
	Logger *slog.Logger
	// BackendFactoryFn creates the process backend. Nil means
	// DefaultBackendFactory.
	BackendFactoryFn BackendFactory
}

// DefaultBackendFactory spawns the real `codex app-server` stdio backend.
func DefaultBackendFactory(ctx context.Context, binary string, logger *slog.Logger) (Backend, error) {
	return NewAppServerBackend(ctx, binary, logger)
}

// New acquires a backend, performs the initialize handshake, and returns a
// ready Codex. The backend is closed on every failed initialization path.
func New(ctx context.Context, config Config) (Codex, error) {
	factory := config.BackendFactoryFn
	if factory == nil {
		factory = DefaultBackendFactory
	}
	startupTimeout := config.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = DefaultStartupTimeout
	}
	inactivityTimeout := config.TurnInactivityTimeout
	if inactivityTimeout <= 0 {
		inactivityTimeout = DefaultTurnInactivityTimeout
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	backend, err := factory(ctx, config.Binary, logger)
	if err != nil {
		return nil, err
	}
	client := &codexImpl{
		backend:           backend,
		logger:            logger,
		inactivityTimeout: inactivityTimeout,
	}

	initCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := client.initialize(initCtx); err != nil {
		closeErr := client.Close()
		return nil, errors.Join(fmt.Errorf("codex: initialize app server: %w", err), closeErr)
	}
	return client, nil
}
