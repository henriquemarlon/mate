package codex

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecuteUsesAppServerAndReturnsFinalJSON(t *testing.T) {
	dir := t.TempDir()
	messagesPath := filepath.Join(dir, "messages.jsonl")
	binaryPath := filepath.Join(dir, "fake-codex")
	script := `#!/bin/sh
set -eu
messages="` + messagesPath + `"
while IFS= read -r message; do
  printf '%s\n' "$message" >> "$messages"
  case "$message" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"id":1,"result":{"userAgent":"fake"}}'
      ;;
    *'"method":"thread/start"'*)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"}}}'
      ;;
    *'"method":"turn/start"'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1"}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","completedAtMs":1,"item":{"id":"item-1","type":"agentMessage","text":"{\"transcription\":\"ok\"}"}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[],"error":null}}}'
      ;;
  esac
done
`
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	client, err := New(context.Background(), Config{Binary: binaryPath})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	result, err := client.Execute(context.Background(), Request{
		Prompt:    "transcribe",
		ImageData: []byte("png"),
		MediaType: "image/png",
		Schema:    []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"transcription":"ok"}` {
		t.Fatalf("result = %s", result)
	}

	messages, err := os.ReadFile(messagesPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"jsonrpc":"2.0"`, "initialize", "initialized", "thread/start", "turn/start", "localImage", "outputSchema"} {
		if !strings.Contains(string(messages), expected) {
			t.Errorf("messages do not contain %q: %s", expected, messages)
		}
	}
}

func TestExecuteRejectsInvalidInput(t *testing.T) {
	client := &codexImpl{}
	if _, err := client.Execute(context.Background(), Request{Schema: []byte(`{}`)}); err == nil {
		t.Fatal("expected empty prompt error")
	}
	if _, err := client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`not-json`)}); err == nil {
		t.Fatal("expected invalid schema error")
	}
}

func TestNewClosesBackendWhenInitializeFails(t *testing.T) {
	backend := newFakeBackend()
	backend.callErr = errors.New("handshake refused")

	_, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err == nil {
		t.Fatal("expected initialization error")
	}
	if got := backend.closeCount(); got != 1 {
		t.Fatalf("backend closes = %d, want 1", got)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	backend := newFakeBackend()
	client, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if got := backend.closeCount(); got != 1 {
		t.Fatalf("backend closes = %d, want 1", got)
	}
}

func TestExecuteReportsTurnFailure(t *testing.T) {
	backend := newFakeBackend()
	backend.callResults["thread/start"] = `{"thread":{"id":"t1"}}`
	backend.callResults["turn/start"] = `{"turn":{"id":"u1"}}`
	backend.notifications <- Notification{
		Method: "turn/completed",
		Params: json.RawMessage(`{"threadId":"t1","turn":{"id":"u1","status":"failed","error":{"message":"model exploded"}}}`),
	}

	client, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`{}`)})
	if !errors.Is(err, ErrTurnFailed) || !strings.Contains(err.Error(), "model exploded") {
		t.Fatalf("err = %v, want ErrTurnFailed with server message", err)
	}
}

func TestExecuteMapsInterruptedTurn(t *testing.T) {
	backend := newFakeBackend()
	backend.callResults["thread/start"] = `{"thread":{"id":"t1"}}`
	backend.callResults["turn/start"] = `{"turn":{"id":"u1"}}`
	backend.notifications <- Notification{
		Method: "turn/completed",
		Params: json.RawMessage(`{"threadId":"t1","turn":{"id":"u1","status":"interrupted","error":null}}`),
	}

	client, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`{}`)})
	if !errors.Is(err, ErrTurnInterrupted) {
		t.Fatalf("err = %v, want ErrTurnInterrupted", err)
	}
}

func TestExecuteReportsMissingAgentMessage(t *testing.T) {
	backend := newFakeBackend()
	backend.callResults["thread/start"] = `{"thread":{"id":"t1"}}`
	backend.callResults["turn/start"] = `{"turn":{"id":"u1"}}`
	backend.notifications <- Notification{
		Method: "turn/completed",
		Params: json.RawMessage(`{"threadId":"t1","turn":{"id":"u1","status":"completed","error":null}}`),
	}

	client, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`{}`)})
	if !errors.Is(err, ErrNoAgentMessage) {
		t.Fatalf("err = %v, want ErrNoAgentMessage", err)
	}
}

func TestExecuteReportsInvalidFinalJSON(t *testing.T) {
	backend := newFakeBackend()
	backend.callResults["thread/start"] = `{"thread":{"id":"t1"}}`
	backend.callResults["turn/start"] = `{"turn":{"id":"u1"}}`
	backend.notifications <- Notification{
		Method: "item/completed",
		Params: json.RawMessage(`{"threadId":"t1","turnId":"u1","item":{"type":"agentMessage","text":"not json"}}`),
	}
	backend.notifications <- Notification{
		Method: "turn/completed",
		Params: json.RawMessage(`{"threadId":"t1","turn":{"id":"u1","status":"completed","error":null}}`),
	}

	client, err := New(context.Background(), Config{BackendFactoryFn: backend.factory()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`{}`)})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("err = %v, want ErrInvalidResponse", err)
	}
}

func TestExecuteStallsOnInactivity(t *testing.T) {
	backend := newFakeBackend()
	backend.callResults["thread/start"] = `{"thread":{"id":"t1"}}`
	backend.callResults["turn/start"] = `{"turn":{"id":"u1"}}`

	client, err := New(context.Background(), Config{
		BackendFactoryFn:      backend.factory(),
		TurnInactivityTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`{}`)})
	if !errors.Is(err, ErrTurnStalled) {
		t.Fatalf("err = %v, want ErrTurnStalled", err)
	}
	if !backend.sawCall("turn/interrupt") {
		t.Fatal("stalled turn was not interrupted")
	}
}

func TestImageExtension(t *testing.T) {
	if got := imageExtension("image/jpeg"); got != ".jpg" {
		t.Fatalf("jpeg extension = %q", got)
	}
	if got := imageExtension("image/png"); got != ".png" {
		t.Fatalf("png extension = %q", got)
	}
}

// fakeBackend is an in-memory Backend used to exercise lifecycle and
// protocol handling without spawning a subprocess.
type fakeBackend struct {
	mu            sync.Mutex
	callErr       error
	callResults   map[string]string
	calls         []string
	closes        int
	notifications chan Notification
	done          chan struct{}
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		callResults:   make(map[string]string),
		notifications: make(chan Notification, 8),
		done:          make(chan struct{}),
	}
}

func (f *fakeBackend) factory() BackendFactory {
	return func(context.Context, string, *slog.Logger) (Backend, error) { return f, nil }
}

func (f *fakeBackend) Call(_ context.Context, method string, _ any, result any) error {
	f.mu.Lock()
	err := f.callErr
	raw := f.callResults[method]
	f.calls = append(f.calls, method)
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if result != nil && raw != "" {
		return json.Unmarshal([]byte(raw), result)
	}
	return nil
}

func (f *fakeBackend) sawCall(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call == method {
			return true
		}
	}
	return false
}

func (f *fakeBackend) Notify(string, any) error { return nil }

func (f *fakeBackend) Notifications() <-chan Notification { return f.notifications }

func (f *fakeBackend) Done() <-chan struct{} { return f.done }

func (f *fakeBackend) Err() error { return ErrClosed }

func (f *fakeBackend) Close() error {
	f.mu.Lock()
	f.closes++
	f.mu.Unlock()
	return nil
}

func (f *fakeBackend) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}
