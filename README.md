# Mate

A CLI toolkit for processing handwritten notes — transcribes, clusters, generates flashcards, and exports to Obsidian.

## Architecture

```
Google Drive (PDFs from Goodnotes)
       |
       v
  drive download --> split into page images
       |
       |----------------------------.
       v                            v
  vision transcribe            embed image
  (Claude sonnet)              (Gemini Embedding 2)
  text for flashcards          vector for clustering
       |                            |
       |                            v
       |                     qdrant upsert
       |                            |
       |                            v
       |                     cluster run (DBSCAN)
       |                            |
       |                            v
       |                     cluster diff (dirty/stale)
       |                            |
       v                            v
  flashcard generate          obsidian export
  (Claude + transcriptions    (KNN links, graph view)
   + bridge pages)
       |
       v
  anki push
```

Two parallel paths from each page image:
1. **Embedding** (Gemini Embedding 2) — embeds the image directly for clustering and KNN similarity
2. **Transcription** (Claude vision) — extracts text for flashcard generation

`mate sync` runs the full pipeline deterministically. No LLM orchestration — Claude is only used where intelligence is needed.

## Stack

| Component | Technology |
|---|---|
| Language | Go |
| CLI framework | Cobra + Viper |
| Vision / Transcription | Claude `claude-sonnet-4-6` (Anthropic SDK) |
| Flashcard generation | Claude `claude-sonnet-4-6` (Anthropic SDK) |
| Embeddings | Gemini Embedding 2 (multimodal — images + text in one vector space) |
| Vector store | Qdrant (gRPC, cosine distance) |
| Clustering | DBSCAN (client-side, cosine distance) |
| Similarity search | KNN (client-side, cosine similarity) |
| State tracking | BoltDB (`bbolt`) |
| Flashcard host | Anki via AnkiConnect |
| Knowledge graph | Obsidian vault with wikilinks |
| Scheduler | macOS launchd |
| Config | TOML + code generation (`go generate`) |

## CLI Subcommands

### `mate sync` — Full pipeline

Runs the deterministic pipeline: drive → embed image → transcribe → qdrant → cluster → flashcards → anki → obsidian.

```bash
mate sync \
  --skills-dir ~/.mate/skills \
  --folder-id YOUR_DRIVE_FOLDER_ID \
  --credentials-file ~/.mate/secrets/drive_credentials.json \
  --anthropic-api-key-file ~/.mate/secrets/anthropic_api_key \
  --google-api-key-file ~/.mate/secrets/google_api_key \
  --obsidian-vault ~/Obsidian/Mate
```

### `mate drive` — Google Drive operations

| Command | Description |
|---|---|
| `mate drive list --folder-id ID --credentials-file FILE` | List all PDFs in a Drive folder |
| `mate drive download --file-id ID --credentials-file FILE --output DIR` | Download a PDF and split into per-page images |

### `mate vision` — Transcription

| Command | Description |
|---|---|
| `mate vision transcribe --image PATH --prompt-file FILE` | Transcribe a page image using Claude vision. The prompt file (skill) defines how to handle text, formulas, and diagrams |

### `mate embed` — Embeddings (Gemini Embedding 2)

| Command | Description |
|---|---|
| `mate embed image --input PATH` | Embed an image using Gemini Embedding 2 (multimodal) |
| `mate embed text --input TEXT` | Embed text using Gemini Embedding 2 |
| `mate embed file --input PATH` | Embed text from a file |

### `mate qdrant` — Vector store

| Command | Description |
|---|---|
| `mate qdrant upsert --id ID --vector FILE --payload FILE` | Upsert a point (vector + metadata) |
| `mate qdrant search --vector FILE --limit N --exclude-notebook ID` | KNN search, with optional same-notebook exclusion |
| `mate qdrant scroll` | Scroll all points in the collection |

### `mate cluster` — Clustering

| Command | Description |
|---|---|
| `mate cluster run` | Run DBSCAN on all vectors in Qdrant, output cluster assignments |
| `mate cluster diff` | Compare current clusters against stored state, output dirty/stale lists |

### `mate flashcard` — Flashcard generation

| Command | Description |
|---|---|
| `mate flashcard generate --cluster-id ID --prompt-file FILE` | Generate flashcards for a cluster using Claude. The prompt file (skill) defines generation rules |

### `mate anki` — Anki operations

| Command | Description |
|---|---|
| `mate anki push --deck NAME --cards FILE` | Push flashcards to Anki via AnkiConnect |
| `mate anki delete --card-ids IDS` | Delete cards by ID |
| `mate anki ping` | Check if AnkiConnect is reachable |

### `mate notebook` — Notebook operations

| Command | Description |
|---|---|
| `mate notebook describe --image PATH --prompt-file FILE` | Extract subject + references from a notebook's first page image using Claude |

### `mate obsidian` — Obsidian vault export

| Command | Description |
|---|---|
| `mate obsidian export --vault PATH` | Export all pages into an Obsidian vault with semantic links |

### `mate service` — Scheduler (macOS launchd)

| Command | Description |
|---|---|
| `mate service install` | Install and load a launchd plist to run `mate sync` periodically |
| `mate service uninstall` | Unload and remove the plist |
| `mate service status` | Show if the service is loaded and running |
| `mate service logs` | Tail service logs |

## Skills

Skills are markdown files with YAML frontmatter that define **prompts for Claude**. They control how specific LLM tasks behave — transcription rules, flashcard generation rules, notebook description rules. Skills live in `--skills-dir` and are loaded at runtime.

### How skills are used

Skills are **not** an agent orchestration mechanism. They are prompt templates consumed by specific subcommands:

```
~/.mate/skills/
├── transcribe.md           --> mate vision transcribe --prompt-file
├── flashcard-general.md    --> mate flashcard generate --prompt-file
├── flashcard-math.md       --> mate flashcard generate --prompt-file
└── notebook-describe.md    --> mate notebook describe --prompt-file
```

Each subcommand that calls Claude accepts a `--prompt-file` flag pointing to a skill. The skill file defines:
- **What model to use** (frontmatter `model` field)
- **How many tokens** (frontmatter `max_tokens` field)
- **The system prompt** (markdown body)

In `mate sync`, skills are loaded from `--skills-dir` by name. The sync pipeline knows which skill to use for each step — no routing needed:

| Pipeline step | Skill used | How selected |
|---|---|---|
| Transcription | `transcribe` | Loaded by name: `store.Get("transcribe")` |
| Flashcard generation | `flashcard-general` | Loaded by name: `store.Get("flashcard-general")` |
| Notebook description | `notebook-describe` | Loaded by name: `store.Get("notebook-describe")` |

To change how transcription works, edit `~/.mate/skills/transcribe.md`. Zero recompile.

### Skill file format

```markdown
---
name: transcribe
description: Transcribes handwritten text, formulas, and diagrams from page images
model: claude-sonnet-4-6
max_tokens: 4096
---

Transcribe all handwritten content in this image exactly as written.
Preserve structure, formatting, headings, bullet points, and numbering.
- Convert mathematical expressions and formulas to LaTeX notation using $...$ for inline and $$...$$ for display math.
- For diagrams, flowcharts, or visual structures: describe them in text, noting components, labels, and connections.
If you cannot read a word, use [illegible]. Output only the transcribed content, nothing else.
```

**Frontmatter fields:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique identifier used by `store.Get(name)` |
| `description` | string | yes | What this skill does |
| `model` | string | no | Claude model override |
| `max_tokens` | int | no | Max tokens override |

### Provided skills

| Skill | Description | Used by |
|---|---|---|
| `transcribe` | Transcribes handwritten text, formulas, and diagrams | `mate vision transcribe` |
| `flashcard-general` | General-purpose flashcard generation | `mate flashcard generate` |
| `flashcard-math` | Flashcard generation for math and proofs | `mate flashcard generate` |
| `notebook-describe` | Extracts subject + references from title page | `mate notebook describe` |

## Project Structure

```
mate/
├── cmd/mate/
│   ├── main.go
│   └── root/
│       ├── root.go                      # Root cobra command, persistent flags
│       ├── sync/sync.go                 # sync — deterministic pipeline
│       ├── drive/drive.go               # drive list, drive download
│       ├── vision/vision.go             # vision transcribe
│       ├── embed/embed.go               # embed text, embed file, embed image
│       ├── qdrant/qdrant.go             # qdrant upsert, search, scroll
│       ├── cluster/cluster.go           # cluster run, cluster diff
│       ├── flashcard/flashcard.go       # flashcard generate
│       ├── anki/anki.go                 # anki push, delete, ping
│       ├── notebook/notebook.go         # notebook describe
│       ├── obsidian/obsidian.go         # obsidian export
│       └── service/service.go           # service install, uninstall, status, logs
├── configs/
│   ├── config.go                        # Base types (Redacted[T], ExpandPath)
│   ├── generated.go                     # AUTO-GENERATED (go generate ./...)
│   └── generate/
│       ├── Config.toml                  # Source of truth for all MATE_* variables
│       ├── main.go, code.go, docs.go    # Code generator
│       └── env.go, helpers.go           # TOML parsing
├── internal/
│   ├── domain/
│   │   ├── entity/
│   │   │   ├── page.go                  # Page entity
│   │   │   └── flashcard.go             # Flashcard + ClusterResult entities
│   │   └── skill/
│   │       ├── skill.go                 # Skill type, Parse function
│   │       └── store.go                 # Store: loads skills from --skills-dir
│   ├── infra/
│   │   ├── drive/client.go              # Google Drive API client
│   │   ├── vision/client.go             # Claude vision API client
│   │   ├── embeddings/client.go         # Gemini Embedding 2 (text + image)
│   │   ├── store/client.go              # Qdrant vector store
│   │   ├── anki/client.go               # AnkiConnect HTTP client
│   │   ├── flashcardgen/generator.go    # Claude flashcard generation
│   │   ├── obsidian/exporter.go         # Obsidian vault export
│   │   ├── launchd/plist.go             # macOS launchd plist management
│   │   ├── statedb/statedb.go           # BoltDB cluster state tracking
│   │   └── version/version.go           # Build version
│   └── usecase/
│       ├── sync_notes.go                # Full pipeline orchestration
│       ├── cluster_notes.go             # DBSCAN + dirty cluster detection
│       ├── generate_flashcards.go       # Flashcard gen + Anki push
│       └── export_obsidian.go           # Obsidian vault export with semantic links
├── pkg/
│   ├── dbscan/dbscan.go                 # DBSCAN algorithm (cosine distance)
│   └── knn/knn.go                       # KNN search (cosine similarity)
└── docs/
    └── config.md                        # AUTO-GENERATED config reference
```

## Configuration

| Variable | Description | Default |
|---|---|---|
| `MATE_ANTHROPIC_API_KEY` | Anthropic API key. File: `_FILE` | required |
| `MATE_GOOGLE_API_KEY` | Google API key (embeddings). File: `_FILE` | required |
| `MATE_GOOGLE_EMBEDDING_MODEL` | Google embedding model name | `gemini-embedding-2` |
| `MATE_DRIVE_CREDENTIALS` | Google service account credentials JSON. File: `_FILE` | required |
| `MATE_DRIVE_FOLDER_ID` | Google Drive folder ID | required |
| `MATE_QDRANT_ADDRESS` | Qdrant gRPC address | `localhost:6334` |
| `MATE_QDRANT_COLLECTION` | Qdrant collection name | `notes` |
| `MATE_DBSCAN_EPSILON` | DBSCAN max cosine distance (0.0-1.0) | `0.3` |
| `MATE_DBSCAN_MIN_POINTS` | DBSCAN minimum cluster size | `2` |
| `MATE_ANKI_CONNECT_URL` | AnkiConnect API URL | `http://localhost:8765` |
| `MATE_ANKI_DECK_PREFIX` | Anki deck prefix (`{prefix}::{topic}`) | `Mate` |
| `MATE_ANKI_MODEL_NAME` | Anki note model | `Basic` |
| `MATE_OBSIDIAN_VAULT` | Obsidian vault path | (disabled) |
| `MATE_OBSIDIAN_K_NEIGHBORS` | Cross-notebook similarity links per page | `5` |
| `MATE_SKILLS_DIR` | Skills directory | `~/.mate/skills` |
| `MATE_STATE_DB_PATH` | BoltDB state file path | `~/.mate/state.db` |
| `MATE_SERVICE_INTERVAL` | Seconds between sync runs (launchd) | `900` |
| `MATE_LOG_LEVEL` | Log level: debug, info, warn, error | `info` |
| `MATE_LOG_COLOR` | Colored log output | `true` |

## Key Design Decisions

| Concern | Choice | Reason |
|---|---|---|
| Pipeline | Deterministic Go code | No LLM orchestration overhead, predictable, debuggable |
| LLM usage | Only for intelligence tasks | Transcription, flashcard gen, notebook describe — nothing else |
| Skills | Markdown files on disk | Behavior defined outside the binary, zero recompile |
| Skill selection | By name in pipeline code | Deterministic — `store.Get("transcribe")`, no routing needed |
| Embeddings | Gemini Embedding 2 (multimodal) | Embed images directly — no transcription needed for clustering |
| Vision model | Claude `claude-sonnet-4-6` | Best quality for handwriting, formulas, and diagrams |
| Vector store | Qdrant | Easy setup, great Go client, cosine distance built-in |
| Clustering | DBSCAN (client-side) | Auto-discovers clusters, no K needed, handles noise |
| Flashcard host | Anki via AnkiConnect | Spaced repetition handled by Anki |
| Knowledge graph | Obsidian vault with wikilinks | Graph view for free, portable markdown |
| State tracking | BoltDB (`bbolt`) | Embedded, zero-CGO, single file |
| Scheduler | macOS launchd | Native, no extra dependencies, runs on login |
| Drive auth | Service Account | Simpler for personal use |
| CLI framework | Cobra + Viper | Flag binding, env var support, config generation |

## Obsidian Export

Exports all pages into an Obsidian vault with semantic links for graph-view exploration.

### Vault structure

```
~/Obsidian/Mate/
├── Notebooks/
│   ├── Notebook A.md                   <- Subject + references + links to all pages
│   ├── Notebook B.md
│   └── Notebook C.md
├── Clusters/
│   ├── Linear Algebra/
│   │   ├── _Linear Algebra.md          <- MOC (Map of Content)
│   │   ├── Notebook A - Page 3.md
│   │   └── Notebook B - Page 12.md
│   ├── Thermodynamics/
│   │   ├── _Thermodynamics.md
│   │   └── Notebook C - Page 1.md
│   └── ...
└── _Index.md                           <- Top-level MOC
```

### Link hierarchy

1. **Notebook <-> Notebook** — subject descriptions embedded with Gemini Embedding 2, KNN across notebooks
2. **Notebook -> Pages** — parent links to all pages in order
3. **Page -> Notebook** — every page links back to parent
4. **Page <-> Pages (cross-notebook only)** — KNN excludes same-notebook pages
5. **Cluster MOC** — same-cluster pages grouped in folders

### Page note

```markdown
---
notebook: "Notebook A"
page: 3
cluster: "Linear Algebra"
drive_file_id: "1ABC123"
---

# Notebook A - Page 3

![[Notebook A - Page 3.png]]

## Notebook

- [[Notebook A]] -- page 3 of 15

## Related Pages (cross-notebook)

- [[Notebook B - Page 7]] -- cosine: 0.84
- [[Notebook C - Page 1]] -- cosine: 0.79
- [[Notebook D - Page 12]] -- cosine: 0.76
```

Just the original image plus links. No transcription — transcription is internal only.

### Notebook note

Generated from the first page (title page) via `mate notebook describe`:

```markdown
---
notebook: "Notebook A"
total_pages: 15
---

# Notebook A

## Subject

Introductory linear algebra following MIT 18.06. Vector spaces,
eigenvalues, matrix decompositions.

## References

- Gilbert Strang, *Introduction to Linear Algebra* (5th ed.)
- MIT OpenCourseWare 18.06

## Related Notebooks

- [[Notebook B]] -- Differential Equations (cosine: 0.87)
- [[Notebook D]] -- Numerical Methods (cosine: 0.81)

## Pages

| Page | Cluster | Link |
|------|---------|------|
| 1 | Linear Algebra | [[Notebook A - Page 1]] |
| 2 | Linear Algebra | [[Notebook A - Page 2]] |
| 3 | Linear Algebra | [[Notebook A - Page 3]] |
| ... | ... | ... |
```

## How Clustering Works

1. **Embed**: Each page image embedded directly with Gemini Embedding 2 (multimodal)
2. **DBSCAN**: Client-side clustering with cosine distance
3. **Bridge context**: K nearest pages from other clusters included as context for flashcard generation
4. **Regeneration**: Changed clusters get full flashcard regeneration
5. **Noise**: Unclustered pages skipped

## Page Storage

Each page in Qdrant carries:
- **Vector**: image embedding for clustering and similarity search (Gemini Embedding 2)
- **Transcription**: text, LaTeX formulas, diagram descriptions (for flashcard generation)
- **Drive reference**: file ID + page number for on-demand image fetch

Original images stay on Drive — no duplication.
