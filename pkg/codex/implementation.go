package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const interruptTimeout = 2 * time.Second

// App Server protocol payloads. Only the fields Mate relies on are typed;
// this is deliberately not a full protocol SDK.

type clientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type initializeParams struct {
	ClientInfo clientInfo `json:"clientInfo"`
}

type threadStartParams struct {
	ApprovalPolicy string `json:"approvalPolicy"`
	Cwd            string `json:"cwd"`
	Ephemeral      bool   `json:"ephemeral"`
	Sandbox        string `json:"sandbox"`
	ServiceName    string `json:"serviceName"`
}

type inputItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Path string `json:"path,omitempty"`
}

type sandboxPolicy struct {
	Type          string `json:"type"`
	NetworkAccess bool   `json:"networkAccess"`
}

type turnStartParams struct {
	ThreadID       string          `json:"threadId"`
	Input          []inputItem     `json:"input"`
	Cwd            string          `json:"cwd"`
	ApprovalPolicy string          `json:"approvalPolicy"`
	SandboxPolicy  sandboxPolicy   `json:"sandboxPolicy"`
	OutputSchema   json.RawMessage `json:"outputSchema"`
}

type turnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type threadStartResponse struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
}

type turnStartResponse struct {
	Turn struct {
		ID string `json:"id"`
	} `json:"turn"`
}

type itemCompletedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Item     struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

type turnCompletedParams struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"turn"`
}

// codexImpl implements Codex on top of a Backend. It owns the App Server
// semantics: the initialize handshake, one ephemeral thread with a single
// turn per Execute, completion tracking, and JSON validation. Threads are
// deliberately not reused across Execute calls so page jobs stay isolated.
type codexImpl struct {
	backend           Backend
	logger            *slog.Logger
	inactivityTimeout time.Duration

	executeMu sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (c *codexImpl) log() *slog.Logger {
	if c.logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.logger
}

func (c *codexImpl) initialize(ctx context.Context) error {
	err := c.backend.Call(ctx, "initialize", initializeParams{
		ClientInfo: clientInfo{Name: "mate", Title: "Mate", Version: "1"},
	}, nil)
	if err != nil {
		return err
	}
	return c.backend.Notify("initialized", struct{}{})
}

func (c *codexImpl) Execute(ctx context.Context, request Request) ([]byte, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("codex: prompt is required")
	}
	if !json.Valid(request.Schema) {
		return nil, errors.New("codex: output schema is not valid JSON")
	}

	c.executeMu.Lock()
	defer c.executeMu.Unlock()

	dir, err := os.MkdirTemp("", "mate-codex-")
	if err != nil {
		return nil, fmt.Errorf("codex: create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)

	input := []inputItem{{Type: "text", Text: request.Prompt}}
	if len(request.ImageData) > 0 {
		extension := ".png"
		switch strings.ToLower(strings.TrimSpace(request.MediaType)) {
		case "image/jpeg", "image/jpg":
			extension = ".jpg"
		case "image/webp":
			extension = ".webp"
		case "image/gif":
			extension = ".gif"
		}
		imagePath := filepath.Join(dir, "page"+extension)
		if err := os.WriteFile(imagePath, request.ImageData, 0o600); err != nil {
			return nil, fmt.Errorf("codex: write image: %w", err)
		}
		input = append(input, inputItem{Type: "localImage", Path: imagePath})
	}

	var thread threadStartResponse
	if err := c.backend.Call(ctx, "thread/start", threadStartParams{
		ApprovalPolicy: "never",
		Cwd:            dir,
		Ephemeral:      true,
		Sandbox:        "read-only",
		ServiceName:    "mate",
	}, &thread); err != nil {
		return nil, fmt.Errorf("codex: start thread: %w", err)
	}
	if thread.Thread.ID == "" {
		return nil, errors.New("codex: app server returned an empty thread id")
	}

	var turn turnStartResponse
	if err := c.backend.Call(ctx, "turn/start", turnStartParams{
		ThreadID:       thread.Thread.ID,
		Input:          input,
		Cwd:            dir,
		ApprovalPolicy: "never",
		SandboxPolicy:  sandboxPolicy{Type: "readOnly", NetworkAccess: false},
		OutputSchema:   json.RawMessage(request.Schema),
	}, &turn); err != nil {
		return nil, fmt.Errorf("codex: start turn: %w", err)
	}
	if turn.Turn.ID == "" {
		return nil, errors.New("codex: app server returned an empty turn id")
	}

	started := time.Now()
	c.log().Debug("codex turn started", "thread", thread.Thread.ID, "turn", turn.Turn.ID)

	result, err := c.waitForTurn(ctx, thread.Thread.ID, turn.Turn.ID)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, ErrTurnStalled) {
			c.interrupt(thread.Thread.ID, turn.Turn.ID)
		}
		c.log().Debug("codex turn failed",
			"thread", thread.Thread.ID, "turn", turn.Turn.ID,
			"duration", time.Since(started), "error", err)
		return nil, err
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, truncate(string(result), 4096))
	}
	c.log().Debug("codex turn completed",
		"thread", thread.Thread.ID, "turn", turn.Turn.ID,
		"duration", time.Since(started))
	return result, nil
}

// waitForTurn consumes app-server notifications until the turn completes.
// The inactivity timer is a silence window in the Symphony sense: it resets
// on every received notification and never caps total turn duration.
func (c *codexImpl) waitForTurn(ctx context.Context, threadID, turnID string) ([]byte, error) {
	timer := time.NewTimer(c.inactivityWindow())
	defer timer.Stop()

	var finalMessage string
	for {
		select {
		case notification := <-c.backend.Notifications():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(c.inactivityWindow())
			switch notification.Method {
			case "item/completed":
				var params itemCompletedParams
				if json.Unmarshal(notification.Params, &params) == nil && params.ThreadID == threadID && params.TurnID == turnID && params.Item.Type == "agentMessage" {
					finalMessage = params.Item.Text
				}
			case "turn/completed":
				var params turnCompletedParams
				if json.Unmarshal(notification.Params, &params) != nil || params.ThreadID != threadID || params.Turn.ID != turnID {
					continue
				}
				if params.Turn.Status != "completed" {
					// Map the status onto the package sentinels so callers
					// can route failures without parsing messages.
					message := params.Turn.Status
					if params.Turn.Error != nil && params.Turn.Error.Message != "" {
						message = params.Turn.Error.Message
					}
					sentinel := ErrTurnFailed
					switch params.Turn.Status {
					case "interrupted", "canceled", "cancelled":
						sentinel = ErrTurnInterrupted
					}
					return nil, fmt.Errorf("%w: %s: %s", sentinel, params.Turn.Status, message)
				}
				if strings.TrimSpace(finalMessage) == "" {
					return nil, ErrNoAgentMessage
				}
				return []byte(finalMessage), nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("%w: no app server activity for %s", ErrTurnStalled, c.inactivityWindow())
		case <-ctx.Done():
			return nil, fmt.Errorf("codex: wait for turn: %w", ctx.Err())
		case <-c.backend.Done():
			return nil, c.backend.Err()
		}
	}
}

func (c *codexImpl) inactivityWindow() time.Duration {
	if c.inactivityTimeout <= 0 {
		return DefaultTurnInactivityTimeout
	}
	return c.inactivityTimeout
}

func (c *codexImpl) interrupt(threadID, turnID string) {
	ctx, cancel := context.WithTimeout(context.Background(), interruptTimeout)
	defer cancel()
	_ = c.backend.Call(ctx, "turn/interrupt", turnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil)
}

func (c *codexImpl) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.backend.Close()
	})
	return c.closeErr
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
