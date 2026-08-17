package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const bundledCodexPath = "/Applications/ChatGPT.app/Contents/Resources/codex"

type Request struct {
	Prompt    string
	ImageData []byte
	MediaType string
	Schema    []byte
}

type Client struct {
	binary string
}

func NewClient(binary string) (*Client, error) {
	resolved, err := resolveBinary(binary)
	if err != nil {
		return nil, err
	}
	return &Client{binary: resolved}, nil
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

	schemaPath := filepath.Join(dir, "output.schema.json")
	if err := os.WriteFile(schemaPath, request.Schema, 0o600); err != nil {
		return nil, fmt.Errorf("codex: write output schema: %w", err)
	}

	outputPath := filepath.Join(dir, "output.json")
	args := []string{
		"exec",
		"--ephemeral",
		"--ignore-user-config",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--cd", dir,
		"--output-schema", schemaPath,
		"--output-last-message", outputPath,
		"--json",
		"--color", "never",
	}

	if len(request.ImageData) > 0 {
		imagePath := filepath.Join(dir, "page"+imageExtension(request.MediaType))
		if err := os.WriteFile(imagePath, request.ImageData, 0o600); err != nil {
			return nil, fmt.Errorf("codex: write image: %w", err)
		}
		args = append(args, "--image", imagePath)
	}
	args = append(args, "-")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(c.binary, args...)
	cmd.Stdin = strings.NewReader(request.Prompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := runCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("codex: execution failed: %w (stderr: %s)", err, truncate(stderr.String(), 4096))
	}

	result, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("codex: read final response: %w (events: %s)", err, truncate(stdout.String(), 4096))
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("codex: final response is not valid JSON: %s", truncate(string(result), 4096))
	}
	return result, nil
}

func resolveBinary(binary string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "codex"
	}
	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}
	if binary == "codex" {
		if info, err := os.Stat(bundledCodexPath); err == nil && !info.IsDir() {
			return bundledCodexPath, nil
		}
	}
	return "", fmt.Errorf("codex: binary %q not found; install Codex or set MATE_CODEX_BIN", binary)
}

func runCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return ctx.Err()
	}
}

func imageExtension(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
