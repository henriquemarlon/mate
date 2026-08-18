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
                                |
                                v
                 AnkiConnect -> Mate::<note> deck
```

Go owns file discovery, PDF rendering, hashing, state, retries and artifacts. One persistent official Codex App Server process is used only where model judgment is needed.

## Requirements

- Go 1.25+
- Official Codex CLI, authenticated with `codex login`
- Poppler's `pdftoppm` on `PATH`
- Anki Desktop running with the [AnkiConnect add-on](https://ankiweb.net/shared/info/2055492159) installed
- A local folder containing PDFs synced from GoodNotes/Google Drive

## Run

```bash
go run ./cmd/mate run \
  --study-dir ~/GoodNotes \
  --output-dir ~/.mate/output \
  --state-db ~/.mate/state.db \
  --anki-endpoint http://127.0.0.1:8765
```

The generated [configuration reference](docs/config.md) documents the equivalent environment variables and defaults. Its source of truth is `configs/generate/Config.toml`.

## Run once with Docker

Create the persistent output directory. Docker downloads the public image automatically when it is not available locally:

```bash
mkdir -p "$HOME/.mate/output"
```

Run one scan. The GoodNotes directory is read-only; the SQLite database and generated artifacts persist under `~/.mate`; and the Codex authentication already created by `codex login` is shared with the container:

```bash
docker run --rm --init \
  --pull=missing \
  -v "$HOME/GoodNotes:/notes:ro" \
  -v "$HOME/.mate:/home/mate/.mate" \
  -v "$HOME/.codex:/home/mate/.codex" \
  ghcr.io/henriquemarlon/mate:latest run \
  --study-dir=/notes \
  --output-dir=/home/mate/.mate/output \
  --state-db=/home/mate/.mate/state.db \
  --anki-endpoint=http://host.docker.internal:8765
```

The container exits when the scan finishes. `host.docker.internal` lets the container reach AnkiConnect in Anki Desktop on the Mac. Schedule repeated executions with the host operating system; unchanged page hashes are skipped before reaching Codex. Run `docker pull ghcr.io/henriquemarlon/mate:latest` when you want to update an image that is already cached locally.

## Run automatically on macOS

The launchd agent under `init/launchd` runs the public Docker image once on login and every 15 minutes. It uses `/usr/local/bin/docker` and the fixed `/Users/henriquemarlon` paths, so Docker Desktop and Anki Desktop must be running and Codex must already be authenticated under `/Users/henriquemarlon/.codex`.

```bash
mkdir -p /Users/henriquemarlon/.mate/output
cp init/launchd/com.henriquemarlon.mate.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.mate.plist
```

Each launchd invocation creates a disposable container, runs one scan, and exits. Page hashes stored in SQLite ensure unchanged pages are ignored. Logs are written to `/Users/henriquemarlon/.mate/launchd.stdout.log` and `/Users/henriquemarlon/.mate/launchd.stderr.log`.

## Processing rules

- A SHA-256 hash is calculated from each rendered page.
- Unchanged pages never reach Codex again.
- Covers and blank pages are recorded as skipped.
- A processed page that changes later is sent to `needs_review`; it is not silently retranscribed.
- Illegible content is represented as `[?]` and never becomes a flashcard.
- Codex returns validated JSON for both transcription and study-material generation.
- Cards are generated from the transcript, never directly from the image.
- Generated material is persisted before synchronization. If Anki is closed, a later run retries AnkiConnect without another Codex generation.
- Mate creates `Mate Basic`, `Mate Reversed`, and `Mate Cloze` note types and uses a stable hidden ID to update existing notes without resetting their scheduling.
- Each PDF is synchronized to a child deck below the configured `Mate` root deck.
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

`feynman.md` remains a readable study script. `cards.json` is both an auditable artifact and the content synchronized directly into the local Anki collection through AnkiConnect. Mate does not invoke a model or MCP for this deterministic step. Scheduling and AnkiWeb synchronization remain Anki's responsibility.

## Current packages

```text
assets                            embedded runtime schemas
cmd/mate/root/run                 CLI command
configs                          typed generated configuration and validation
configs/generate                 declarative configuration generator
internal/domain/entity           persisted entities and typed lifecycle status
internal/infra/repository        repository contract and SQLite persistence
internal/infra/anki              deterministic AnkiConnect client and card upsert
internal/infra/service           workflow, rendering, hashing, and artifact publication
pkg/codex                        reusable Codex App Server client and stdio backend
internal/infra/codex/transcriber Mate-specific visual classification and transcription
internal/infra/codex/paradigm    Mate-specific transcript -> Feynman + cards
init/launchd                     macOS periodic execution
```

## Verify

```bash
go generate ./configs/...
go test ./...
go vet ./...
```
