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
2. Install Codex and authenticate it:

   ```sh
   curl -fsSL https://chatgpt.com/codex/install.sh | sh
   codex login
   ```

3. Install the local OpenAI-compatible Codex proxy globally with mise:

   ```sh
   mise use -g github:hotchpotch/openai-api-server-via-codex@latest
   openai-api-server-via-codex --version
   ```

4. Install Poppler globally with mise:

   ```sh
   mise use -g conda:poppler@latest
   pdftoppm -v
   ```

   The mise Conda backend downloads Poppler and its runtime dependencies directly from conda-forge; Conda itself is not required.

5. [Install Anki Desktop](https://apps.ankiweb.net/) and the [AnkiConnect add-on](https://ankiweb.net/shared/info/2055492159);
6. Sync the GoodNotes or Google Drive PDFs to a local directory.

Mate talks to the model through the [eino](https://github.com/cloudwego/eino) framework and the local [`openai-api-server-via-codex`](https://github.com/hotchpotch/openai-api-server-via-codex) bridge. The bridge is an unofficial project and uses the Codex authentication stored under `~/.codex`.

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
openai-api-server-via-codex \
  --host 127.0.0.1 \
  --port 18080 \
  --default-model gpt-5.6-luna
```

In another terminal, start Mate:

```sh
export MATE_LLM_API_KEY=codex-local
mate run \
  --study-dir "$HOME/GoodNotes" \
  --output-dir "$HOME/.mate/output" \
  --state-db "$HOME/.mate/state.db" \
  --anki-endpoint http://127.0.0.1:8765 \
  --llm-base-url http://127.0.0.1:18080/v1 \
  --llm-model gpt-5.6-luna
```

Anki Desktop must be running so Mate can reach AnkiConnect. The default polling interval is 900 seconds and can be changed with `--poll-interval` or `MATE_POLL_INTERVAL_SECONDS`.

### Launch at Login (macOS only)

This setup is available only on macOS. The launchd agents under [`init/launchd`](init/launchd) automate every runtime process:

- `com.henriquemarlon.codex-proxy.plist` keeps the loopback Codex proxy running;
- `com.henriquemarlon.mate.plist` starts Mate with the proxy endpoint, resolves Mate and Poppler through the mise shims, and restarts Mate if it exits;
- `com.henriquemarlon.anki.plist` opens Anki Desktop hidden at login and re-opens it after a manual quit.

The agents resolve user-specific paths from `$HOME` at runtime. No path changes are required when using the default `~/GoodNotes` and `~/.mate` directories. Authenticate Codex and install every prerequisite listed above first. Eino requires a non-empty API key, although the local proxy ignores its value, so store a placeholder in the file referenced by the Mate agent:

```sh
mkdir -p "$HOME/.mate/output"
printf '%s' "codex-local" > "$HOME/.mate/llm_api_key"
chmod 600 "$HOME/.mate/llm_api_key"
cp init/launchd/com.henriquemarlon.codex-proxy.plist ~/Library/LaunchAgents/
cp init/launchd/com.henriquemarlon.mate.plist ~/Library/LaunchAgents/
cp init/launchd/com.henriquemarlon.anki.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.codex-proxy.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.mate.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.henriquemarlon.anki.plist
```

The agents store their logs under `$HOME/.mate`.
