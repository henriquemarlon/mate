package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	shutdownGracePeriod = 2 * time.Second
	// maxLineBytes caps one newline-delimited JSON-RPC message. A larger
	// line is a protocol violation and stops the backend.
	maxLineBytes = 10 << 20
)

// appServerBackend is the concrete Backend that spawns `codex app-server`
// and speaks newline-delimited JSON-RPC 2.0 over its standard input and
// output. It is the only layer that knows about exec.Cmd, pipes,
// request/response correlation, process groups, signals, and stderr capture.
type appServerBackend struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stderr  lockedBuffer
	logger  *slog.Logger

	writeMu sync.Mutex
	encoder *json.Encoder

	pendingMu sync.Mutex
	pending   map[uint64]chan rpcMessage
	nextID    uint64

	notifications chan Notification
	done          chan struct{}
	closeOnce     sync.Once
	waitMu        sync.Mutex
	waitErr       error
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewAppServerBackend spawns the Codex App Server subprocess in its own
// process group and starts the read loop. The context only gates spawning;
// the subprocess lifetime is owned by Close.
func NewAppServerBackend(ctx context.Context, binary string, logger *slog.Logger) (Backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("codex: spawn app server: %w", err)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if strings.TrimSpace(binary) == "" {
		binary = "codex"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("codex: binary %q not found in PATH: %w", binary, err)
	}

	command := exec.Command(resolved, "app-server", "--stdio")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex: open stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codex: open stdout: %w", err)
	}

	backend := &appServerBackend{
		command:       command,
		stdin:         stdin,
		logger:        logger,
		encoder:       json.NewEncoder(stdin),
		pending:       make(map[uint64]chan rpcMessage),
		notifications: make(chan Notification, 256),
		done:          make(chan struct{}),
	}
	command.Stderr = &backend.stderr
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codex: start app server: %w", err)
	}
	go backend.read(stdout)
	return backend, nil
}

func (b *appServerBackend) Call(ctx context.Context, method string, params any, result any) error {
	b.pendingMu.Lock()
	b.nextID++
	id := b.nextID
	response := make(chan rpcMessage, 1)
	b.pending[id] = response
	b.pendingMu.Unlock()

	if err := b.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		b.removePending(id)
		return err
	}

	select {
	case message := <-response:
		if message.Error != nil {
			return fmt.Errorf("app server error %d: %s", message.Error.Code, message.Error.Message)
		}
		if result == nil || len(message.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(message.Result, result); err != nil {
			return fmt.Errorf("decode %s response: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		b.removePending(id)
		return ctx.Err()
	case <-b.done:
		b.removePending(id)
		return b.Err()
	}
}

func (b *appServerBackend) Notify(method string, params any) error {
	return b.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (b *appServerBackend) Notifications() <-chan Notification {
	return b.notifications
}

func (b *appServerBackend) Done() <-chan struct{} {
	return b.done
}

// Err reports why the backend stopped serving. It returns ErrClosed after a
// clean shutdown and a stderr-decorated error wrapping ErrStopped otherwise.
func (b *appServerBackend) Err() error {
	b.waitMu.Lock()
	defer b.waitMu.Unlock()
	if b.waitErr == nil {
		return ErrClosed
	}
	return fmt.Errorf("%w: %v (stderr: %s)", ErrStopped, b.waitErr, truncate(b.stderr.String(), 4096))
}

// Close asks the app server to exit by closing stdin, then escalates to
// SIGTERM and SIGKILL on the whole process group. It is idempotent and
// always waits for the process to be reaped.
func (b *appServerBackend) Close() error {
	b.closeOnce.Do(func() {
		b.writeMu.Lock()
		_ = b.stdin.Close()
		b.writeMu.Unlock()
		select {
		case <-b.done:
		case <-time.After(shutdownGracePeriod):
			b.logger.Warn("codex app server did not exit after stdin close; sending SIGTERM")
			_ = syscall.Kill(-b.command.Process.Pid, syscall.SIGTERM)
			select {
			case <-b.done:
			case <-time.After(shutdownGracePeriod):
				b.logger.Warn("codex app server ignored SIGTERM; sending SIGKILL")
				_ = syscall.Kill(-b.command.Process.Pid, syscall.SIGKILL)
				<-b.done
			}
		}
	})
	return nil
}

func (b *appServerBackend) write(message any) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	select {
	case <-b.done:
		return b.Err()
	default:
	}
	if err := b.encoder.Encode(message); err != nil {
		return fmt.Errorf("codex: write app server message: %w", err)
	}
	return nil
}

func (b *appServerBackend) read(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var message rpcMessage
		if err := json.Unmarshal(line, &message); err != nil {
			b.setWaitError(fmt.Errorf("codex: decode app server message: %w", err))
			break
		}
		// Server-initiated requests (approvals, user input) are rejected:
		// Mate always runs with approvalPolicy "never".
		if len(message.ID) > 0 && message.Method != "" {
			_ = b.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      message.ID,
				"error": map[string]any{
					"code":    -32601,
					"message": "Mate does not handle server requests",
				},
			})
			continue
		}
		if len(message.ID) > 0 {
			var id uint64
			if json.Unmarshal(message.ID, &id) == nil {
				b.pendingMu.Lock()
				response := b.pending[id]
				delete(b.pending, id)
				b.pendingMu.Unlock()
				if response != nil {
					response <- message
				}
			}
			continue
		}
		if message.Method != "" {
			b.publish(Notification{Method: message.Method, Params: message.Params})
		}
	}
	if err := scanner.Err(); err != nil {
		b.setWaitError(fmt.Errorf("codex: read app server message: %w", err))
	}
	if err := b.command.Wait(); err != nil {
		b.setWaitError(err)
	}
	close(b.done)
}

// publish never blocks the read loop: a stalled or absent consumer must not
// stop response correlation. When the buffer is full the oldest buffered
// notification is dropped so the newest events (turn completion) survive.
func (b *appServerBackend) publish(notification Notification) {
	select {
	case b.notifications <- notification:
		return
	default:
	}
	select {
	case dropped := <-b.notifications:
		b.logger.Debug("codex notification buffer full; dropped oldest", "method", dropped.Method)
	default:
	}
	select {
	case b.notifications <- notification:
	default:
	}
}

func (b *appServerBackend) removePending(id uint64) {
	b.pendingMu.Lock()
	delete(b.pending, id)
	b.pendingMu.Unlock()
}

func (b *appServerBackend) setWaitError(err error) {
	b.waitMu.Lock()
	if b.waitErr == nil {
		b.waitErr = err
	}
	b.waitMu.Unlock()
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}
