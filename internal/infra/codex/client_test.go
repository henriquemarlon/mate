package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteWritesImageSchemaAndReturnsFinalJSON(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	binaryPath := filepath.Join(dir, "fake-codex")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "` + argsPath + `"
output=""
previous=""
for argument in "$@"; do
  if [ "$previous" = "--output-last-message" ]; then output="$argument"; fi
  previous="$argument"
done
printf '%s' '{"transcription":"ok"}' > "$output"
printf '%s\n' '{"type":"turn.completed"}'
`
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
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

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"exec", "--ephemeral", "--image", "--output-schema", "--output-last-message", "--json"} {
		if !strings.Contains(string(args), expected) {
			t.Errorf("arguments do not contain %q: %s", expected, args)
		}
	}
}

func TestExecuteRejectsInvalidInput(t *testing.T) {
	client := &Client{binary: "unused"}
	if _, err := client.Execute(context.Background(), Request{Schema: []byte(`{}`)}); err == nil {
		t.Fatal("expected empty prompt error")
	}
	if _, err := client.Execute(context.Background(), Request{Prompt: "x", Schema: []byte(`not-json`)}); err == nil {
		t.Fatal("expected invalid schema error")
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
