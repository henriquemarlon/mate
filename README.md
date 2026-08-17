# Mate

Mate turns locally synced GoodNotes PDFs into auditable study material.

The v1 is intentionally small and opinionated:

```text
GoodNotes PDF folder
        |
        v
pdftoppm -> page PNGs -> visual hashes -> SQLite
                                |
                         new page only
                                |
                                v
                    Codex visual transcription
                                |
                     transcript-only generation
                                |
             transcript.md + feynman.md + cards.json
```

Go owns file discovery, PDF rendering, hashing, state, retries and artifacts. The official Codex CLI is used only where model judgment is needed.

## Requirements

- Go 1.25+
- Official Codex CLI, authenticated with `codex login`
- Poppler's `pdftoppm` on `PATH`
- A local folder containing PDFs synced from GoodNotes/Google Drive

Mate also detects the Codex binary bundled with `ChatGPT.app` on macOS.

## Run

```bash
go run ./cmd/mate run \
  --study-dir ~/GoodNotes \
  --output-dir ~/.mate/output \
  --state-db ~/.mate/state.db
```

The generated [configuration reference](docs/config.md) documents the equivalent environment variables and defaults. Its source of truth is `configs/generate/Config.toml`.

## Run automatically on macOS

The launchd agent under `init/launchd` runs `mate run` on login and every 15 minutes. It assumes the Mate binary is installed at `/usr/local/bin/mate`.

```bash
cp init/launchd/com.henriquemarlon.mate.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.mate.plist
```

Mate scans the configured study directory on each invocation. Page hashes stored in SQLite ensure unchanged pages are ignored.

## Processing rules

- A SHA-256 hash is calculated from each rendered page.
- Unchanged pages never reach Codex again.
- Covers and blank pages are recorded as skipped.
- A processed page that changes later is sent to `needs_review`; it is not silently retranscribed.
- Illegible content is represented as `[?]` and never becomes a flashcard.
- Codex returns validated JSON for both transcription and study-material generation.
- Cards are generated from the transcript, never directly from the image.
- Exercises are outside Mate.

When Codex reports uncertainty, Mate writes a review PNG with red bounding boxes under:

```text
~/.mate/output/<subject>/<note>/review/page-N.png
```

## Outputs

Each note receives:

```text
<output>/<subject>/<note>/
├── transcript.md
├── feynman.md
├── cards.json
└── review/            # only when needed
```

`cards.json` is the deterministic handoff for the upcoming Anki MCP synchronization step. Anki scheduling remains entirely Anki's responsibility.

## Current packages

```text
assets                  embedded runtime schemas
cmd/mate/root/run     CLI command
configs              typed generated configuration and validation
configs/generate     declarative configuration generator
internal/domain/entity persisted entities and typed lifecycle status
internal/infra/repository repository contract and SQLite persistence
internal/infra/codex Codex execution boundary
internal/infra/codex/transcriber visual classification and transcription
internal/infra/codex/paradigm transcript -> Feynman + cards
internal/artifacts   atomic file publication and review annotations
internal/workflow    PDF rendering, hashing, and deterministic orchestration
init/launchd         macOS periodic execution
```

## Verify

```bash
go generate ./configs/...
go test ./...
go vet ./...
```
