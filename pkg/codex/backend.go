package codex

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Notification is a JSON-RPC notification pushed by the app server.
type Notification struct {
	Method string
	Params json.RawMessage
}

// Backend is the narrow process/protocol boundary the Codex implementation
// needs from an app-server transport. It abstracts request/response
// correlation and process lifetime so the implementation can be exercised
// without a real subprocess.
type Backend interface {
	// Call sends a JSON-RPC request and decodes the matching response into
	// result. A nil result discards the response payload.
	Call(ctx context.Context, method string, params any, result any) error
	// Notify sends a JSON-RPC notification.
	Notify(method string, params any) error
	// Notifications streams server-initiated notifications in arrival order.
	// When no consumer keeps up, the transport may drop the oldest buffered
	// notification to keep the protocol stream alive.
	Notifications() <-chan Notification
	// Done is closed once the backend stops serving, whether the process
	// exited on its own or Close reclaimed it.
	Done() <-chan struct{}
	// Err reports why the backend stopped. It returns ErrClosed after a
	// clean shutdown and an error wrapping ErrStopped otherwise.
	Err() error
	// Close terminates the backend and reclaims the subprocess. It is
	// idempotent.
	Close() error
}

// BackendFactory creates the process backend used by New. It is injectable
// so tests can substitute a fake transport.
type BackendFactory func(ctx context.Context, binary string, logger *slog.Logger) (Backend, error)
