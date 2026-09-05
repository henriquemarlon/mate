# Mate

Turn locally synced GoodNotes manuscripts into auditable study material.

## Table of Contents

- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Configuration](#configuration)
  - [Running](#running)
  - [Launch at Login (macOS only)](#launch-at-login-macos-only)

## Getting Started

### Prerequisites

For a native Apple Silicon macOS installation:

1. [Install mise](https://mise.jdx.dev/getting-started.html) 2026.8.11 or later;
2. Create an [OpenAI API key](https://platform.openai.com/api-keys) (or a key for any OpenAI-compatible endpoint). Supply it as `MATE_LLM_API_KEY`, or point `MATE_LLM_API_KEY_FILE` at a file containing it (Docker Compose secrets convention);
3. Install Poppler globally with mise:

   ```sh
   mise use -g conda:poppler@latest
   pdftoppm -v
   ```

   The mise Conda backend downloads Poppler and its runtime dependencies directly from conda-forge; Conda itself is not required.

4. [Install Anki Desktop](https://apps.ankiweb.net/) and the [AnkiConnect add-on](https://ankiweb.net/shared/info/2055492159);
5. Sync the GoodNotes or Google Drive PDFs to a local directory.

Mate talks to the model through the [eino](https://github.com/cloudwego/eino) framework. The endpoint and model are configurable with `--llm-base-url`/`MATE_LLM_BASE_URL` and `--llm-model`/`MATE_LLM_MODEL`, so any OpenAI-compatible provider works.

Go 1.25 or later is required only when building Mate from source.

### Installation

Install the latest Mate release globally with mise:

```sh
mise use -g github:henriquemarlon/mate@latest
mate --version
```

Re-run the `mise use` command to resolve and install the latest Mate release.

### Configuration

Mate works without an environment file. Every setting can be supplied through an environment variable or an equivalent command-line flag.

The generated [configuration reference](docs/config.md) contains the available variables and defaults. Its source of truth is [`configs/generate/Config.toml`](configs/generate/Config.toml).

### Running

Start Mate with the directories used for synchronized PDFs, generated artifacts, and persistent SQLite state:

```sh
export MATE_LLM_API_KEY=sk-...
mate run \
  --study-dir "$HOME/GoodNotes" \
  --output-dir "$HOME/.mate/output" \
  --state-db "$HOME/.mate/state.db" \
  --anki-endpoint http://127.0.0.1:8765
```

Anki Desktop must be running so Mate can reach AnkiConnect. The default polling interval is 900 seconds and can be changed with `--poll-interval` or `MATE_POLL_INTERVAL_SECONDS`.

### Launch at Login (macOS only)

This setup is available only on macOS. The launchd agents under [`init/launchd`](init/launchd) automate Mate and Anki:

- `com.henriquemarlon.mate.plist` starts Mate at login, resolves Mate and Poppler through the mise shims, and restarts Mate if it exits;
- `com.henriquemarlon.anki.plist` opens Anki Desktop hidden at login and re-opens it after a manual quit.

The agents require absolute paths. Review the bundled plists and adapt their user-specific paths before installing them. launchd does not inherit the login shell environment, so the Mate agent reads the API key through `MATE_LLM_API_KEY_FILE`; create that file first:

```sh
printf '%s' "sk-..." > "$HOME/.mate/llm_api_key"
chmod 600 "$HOME/.mate/llm_api_key"
```

```sh
mkdir -p "$HOME/.mate/output"
cp init/launchd/com.henriquemarlon.mate.plist ~/Library/LaunchAgents/
cp init/launchd/com.henriquemarlon.anki.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.mate.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.anki.plist
```

The agents store their logs under `$HOME/.mate`.
