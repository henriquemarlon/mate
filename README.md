# Mate

A Go CLI that reads handwritten notes from Google Drive (exported by Goodnotes), transcribes them using Claude vision, clusters related pages via DBSCAN on Google embeddings, and auto-generates Anki flashcards per cluster.

## Overview

```
Google Drive (Goodnotes PDFs)
    ↓  [Drive API — list & download]
Raw images per page
    ↓  [Claude claude-sonnet-4-6 — vision]
Transcribed text per page
    ↓  [Google text-embedding-004, task_type=CLUSTERING]
Embedding vectors
    ↓  [Qdrant — vector storage]
All vectors
    ↓  [DBSCAN — client-side clustering]
Topic clusters
    ↓  [Claude — flashcard generation + topic labeling]
Q&A flashcards per cluster
    ↓  [AnkiConnect API]
Anki decks: Mate::{topic}
```

## Data Flow (`mate sync`)

```
mate sync
  ├─ [1] Drive: list folder → download PDFs
  ├─ [2] Split PDFs into per-page images
  ├─ [3] For each page:
  │      ├─ Vision (Claude): image → transcribed text
  │      ├─ Embed (Google, task_type=CLUSTERING): text → vector
  │      └─ Upsert into Qdrant (vector + metadata)
  ├─ [4] Cluster: DBSCAN on all vectors (client-side)
  ├─ [5] Detect changed clusters (compare content hashes vs stored state)
  ├─ [6] For each changed cluster:
  │      ├─ Claude: generate topic label + flashcards (with bridge context)
  │      ├─ Anki: delete old cards → create deck → push new cards
  │      └─ StateDB: save cluster state
  └─ [7] Log summary
```

## Architecture

```
mate/
├── cmd/mate/
│   ├── main.go                          # Entrypoint
│   └── root/
│       ├── root.go                      # Root cobra command, persistent flags
│       ├── sync/sync.go                 # sync subcommand — full pipeline
│       └── serve/serve.go               # serve subcommand — HTTP API (planned)
├── configs/
│   ├── config.go                        # Base types (Redacted[T], ExpandPath)
│   ├── generated.go                     # AUTO-GENERATED (go generate ./...)
│   └── generate/
│       ├── Config.toml                  # Source of truth for all MATE_* variables
│       ├── main.go, code.go, docs.go    # Code generator
│       ├── env.go, helpers.go           # TOML parsing
├── internal/
│   ├── domain/
│   │   ├── entity/
│   │   │   ├── page.go                  # Page entity
│   │   │   └── flashcard.go             # Flashcard + ClusterResult entities
│   │   └── cluster/
│   │       └── dbscan.go                # DBSCAN algorithm (cosine distance)
│   ├── infra/
│   │   ├── drive/client.go              # Google Drive API client
│   │   ├── vision/client.go             # Claude vision transcription
│   │   ├── embeddings/client.go         # Google embeddings (task_type=CLUSTERING)
│   │   ├── store/client.go              # Qdrant vector store
│   │   ├── anki/client.go               # AnkiConnect HTTP client
│   │   ├── flashcardgen/generator.go    # Claude flashcard + topic generation
│   │   ├── statedb/statedb.go           # BoltDB cluster state tracking
│   │   └── version/version.go           # Build version
│   └── usecase/
│       ├── sync_notes.go                # Drive → Vision → Embed → Qdrant
│       ├── cluster_notes.go             # DBSCAN → dirty cluster detection
│       └── generate_flashcards.go       # Flashcard gen → Anki push
└── docs/
    └── config.md                        # AUTO-GENERATED config reference
```

## Key Design Decisions

| Concern | Choice | Reason |
|---|---|---|
| Vision model | Claude (`claude-sonnet-4-6`) | Best handwriting transcription quality |
| Embeddings | Google `text-embedding-004` with `task_type=CLUSTERING` | Optimizes vectors for clustering |
| Vector store | Qdrant | Easy setup, great Go client, cosine distance built-in |
| Clustering | DBSCAN (client-side) | Auto-discovers clusters, no K needed, handles noise |
| Flashcard scope | Per-cluster (not per-page) | Richer context produces better cards |
| Cluster overlap | Hard clusters + bridge context at prompt time | Avoids duplicates, cascades, complex state |
| Flashcard host | Anki via AnkiConnect | Spaced repetition handled by Anki, no custom UI |
| State tracking | BoltDB (`bbolt`) | Embedded, zero-CGO, single file, CNCF-maintained |
| Drive auth | Service Account | Simpler for personal use |
| CLI | Cobra + Viper | Flag binding, env var support, config generation |

## Quick Start

### 1. Prerequisites

- Go 1.25+
- Docker (for Qdrant)
- Anki with [AnkiConnect](https://ankiweb.net/shared/info/2055492159) plugin
- Google Cloud service account with Drive API enabled
- Anthropic API key
- Google API key (for embeddings)

### 2. Start Qdrant

```bash
docker run -d -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

### 3. Configure secrets

```bash
mkdir -p ~/.mate/secrets
cp /path/to/service-account.json ~/.mate/secrets/drive_credentials.json
echo "your-anthropic-api-key" > ~/.mate/secrets/anthropic_api_key
echo "your-google-api-key" > ~/.mate/secrets/google_api_key
chmod 600 ~/.mate/secrets/*
```

### 4. Sync and generate flashcards

```bash
mate sync \
  --folder-id YOUR_DRIVE_FOLDER_ID \
  --credentials-file ~/.mate/secrets/drive_credentials.json \
  --anthropic-api-key-file ~/.mate/secrets/anthropic_api_key \
  --google-api-key-file ~/.mate/secrets/google_api_key
```

### 5. Skip Anki (transcribe + embed only)

```bash
mate sync --no-anki --folder-id ... --credentials-file ...
```

## Configuration

| Variable | Description | Default |
|---|---|---|
| `MATE_ANTHROPIC_API_KEY` | Anthropic API key (vision + flashcard gen). File: `_FILE` | required |
| `MATE_GOOGLE_API_KEY` | Google API key (embeddings). File: `_FILE` | required |
| `MATE_GOOGLE_EMBEDDING_MODEL` | Google embedding model name | `text-embedding-004` |
| `MATE_DRIVE_CREDENTIALS` | Google service account credentials JSON. File: `_FILE` | required |
| `MATE_DRIVE_FOLDER_ID` | Google Drive folder ID to watch | required |
| `MATE_QDRANT_ADDRESS` | Qdrant gRPC address | `localhost:6334` |
| `MATE_QDRANT_COLLECTION` | Qdrant collection name | `notes` |
| `MATE_DBSCAN_EPSILON` | DBSCAN max cosine distance (0.0-1.0) | `0.3` |
| `MATE_DBSCAN_MIN_POINTS` | DBSCAN minimum cluster size | `2` |
| `MATE_ANKI_CONNECT_URL` | AnkiConnect API URL | `http://localhost:8765` |
| `MATE_ANKI_DECK_PREFIX` | Anki deck prefix (decks: `{prefix}::{topic}`) | `Mate` |
| `MATE_ANKI_MODEL_NAME` | Anki note model | `Basic` |
| `MATE_STATE_DB_PATH` | BoltDB state file path | `~/.mate/state.db` |
| `MATE_HTTP_ADDRESS` | HTTP API address (serve subcommand) | `:8081` |
| `MATE_API_KEY` | HTTP API auth key. File: `_FILE` | required (serve) |
| `MATE_LOG_LEVEL` | Log level: debug, info, warn, error | `info` |
| `MATE_LOG_COLOR` | Colored log output | `true` |

## How Clustering Works

1. **Embed**: Each page is embedded using Google's `text-embedding-004` with `task_type=CLUSTERING`, which optimizes vectors for grouping similar content
2. **DBSCAN**: All vectors are clustered client-side using DBSCAN with cosine distance. Pages about the same topic naturally cluster together
3. **Bridge context**: When generating flashcards for a cluster, the K nearest pages from other clusters are included as context, enabling cross-topic cards (e.g., "How does modular arithmetic apply to RSA?")
4. **Regeneration**: When a cluster changes (new pages added), the entire flashcard set is regenerated with full context, producing a better deck each time
5. **Noise handling**: Pages that don't fit any cluster (DBSCAN noise) are skipped — they lack enough context for quality flashcards
