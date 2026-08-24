// Package codex runs the official Codex CLI headless (`codex exec`) and
// exposes a narrow execution contract on top of it. Every Execute call is
// one short-lived subprocess: prompt, optional image, and JSON schema in,
// validated JSON out. There is no persistent server, so a failed call is
// scoped to that call and never poisons subsequent ones.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// DefaultExecTimeout bounds one full `codex exec` run. It is a total cap,
// not an inactivity window: with one process per call there is no stream to
// watch, only a subprocess to reap.
const DefaultExecTimeout = 10 * time.Minute

// Request is one isolated unit of model work: a prompt, an optional local
// image, and the JSON schema the final answer must match.
type Request struct {
	Prompt    string
	ImageData []byte
	MediaType string
	Schema    []byte
}

// Codex is the public contract for executing isolated requests against the
// Codex CLI.
type Codex interface {
	// Execute runs one `codex exec` invocation and returns the validated
	// JSON produced by the model.
	Execute(ctx context.Context, request Request) ([]byte, error)
}

// Config describes how to construct a Client.
type Config struct {
	// Binary is the Codex CLI path or executable name. Empty means "codex".
	Binary string
	// ExecTimeout is the total cap on one exec run. Zero means
	// DefaultExecTimeout.
	ExecTimeout time.Duration
	// Logger receives exec lifecycle records. Nil means discard.
	Logger *slog.Logger
}

type Client struct {
	binary      string
	execTimeout time.Duration
	logger      *slog.Logger
}

var _ Codex = (*Client)(nil)

// New resolves the Codex binary and returns a ready Client. Nothing is
// spawned until Execute is called.
func New(config Config) (*Client, error) {
	binary := strings.TrimSpace(config.Binary)
	if binary == "" {
		binary = "codex"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("codex: binary %q not found in PATH: %w", binary, err)
	}
	timeout := config.ExecTimeout
	if timeout <= 0 {
		timeout = DefaultExecTimeout
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Client{
		binary:      resolved,
		execTimeout: timeout,
		logger:      logger,
	}, nil
}

func (c *Client) Execute(ctx context.Context, request Request) ([]byte, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("codex: prompt is required")
	}
	if !json.Valid(request.Schema) {
		return nil, errors.New("codex: output schema is not valid JSON")
	}

	dir, err := os.MkdirTemp("", "mate-codex-")
	if err != nil {
		return nil, fmt.Errorf("codex: create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)

	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, request.Schema, 0o600); err != nil {
		return nil, fmt.Errorf("codex: write output schema: %w", err)
	}
	outputPath := filepath.Join(dir, "output.json")

	args := []string{
		"exec",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--output-schema", schemaPath,
		"--output-last-message", outputPath,
	}
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
		args = append(args, "--image", imagePath)
	}
	// --image accepts multiple values, so terminate option parsing before the
	// positional prompt or the CLI will interpret the prompt as another image.
	args = append(args, "--", request.Prompt)

	execCtx, cancel := context.WithTimeout(ctx, c.execTimeout)
	defer cancel()

	// The whole process group is killed on timeout or cancellation so
	// helpers spawned by the CLI never outlive the call.
	var stderr bytes.Buffer
	command := exec.CommandContext(execCtx, c.binary, args...)
	command.Dir = dir
	command.Stdout = io.Discard
	command.Stderr = &stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = 5 * time.Second

	started := time.Now()
	c.logger.Debug("codex exec started")
	if err := command.Run(); err != nil {
		if execCtx.Err() != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("codex: exec exceeded %s: %w", c.execTimeout, execCtx.Err())
		}
		return nil, fmt.Errorf("codex: exec failed: %w (stderr: %s)", err, truncate(stderr.String(), 4096))
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("codex: read final message: %w", err)
	}
	output = bytes.TrimSpace(output)
	if !json.Valid(output) {
		return nil, fmt.Errorf("codex: final response is not valid JSON: %s", truncate(string(output), 4096))
	}
	c.logger.Debug("codex exec completed", "duration", time.Since(started))
	return output, nil
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
