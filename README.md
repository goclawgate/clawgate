# clawgate

Ultra-fast, single-binary Go proxy that lets Anthropic-format clients use OpenAI-compatible backends.

```
Anthropic client → (Anthropic format) → clawgate → (OpenAI format) → OpenAI / ChatGPT / compatible endpoint
```

## Features

- **Dual Mode** — ChatGPT Codex OAuth (no API key needed) or standard OpenAI API key.
- **Zero dependencies** — pure Go stdlib.
- **Single binary** — ~6MB, no runtime needed.
- **Streaming** — full SSE streaming support with error handling.
- **Tool calls** — complete tool_use round-trip translation.
- **Cross-platform** — Linux, macOS, Windows (amd64 & arm64).

## Install

**Linux / macOS:**

```bash
curl -fsSL clawgate.org/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm clawgate.org/install.ps1 | iex
```

**Windows (CMD):**

```cmd
curl -fsSL clawgate.org/install.cmd -o install.cmd && install.cmd && del install.cmd
```

Downloads the binary, installs to `~/.clawgate/bin/`, and adds it to
PATH automatically.

**Platform guides** with manual download, PATH setup, and startup config:

- **[Linux](https://clawgate.org/install-linux.html)**
- **[macOS](https://clawgate.org/install-macos.html)**
- **[Windows](https://clawgate.org/install-windows.html)**

Or grab the binary directly from [Releases](https://github.com/goclawgate/clawgate/releases):

| Platform | File |
|---|---|
| Linux (x86_64) | `clawgate-linux-amd64` |
| Linux (ARM) | `clawgate-linux-arm64` |
| macOS (Intel) | `clawgate-darwin-amd64` |
| macOS (Apple Silicon) | `clawgate-darwin-arm64` |
| Windows | `clawgate-windows-amd64.exe` |

## Quick Start

```bash
# 1. Login (ChatGPT mode — one time only)
clawgate login

# 2. Start the proxy
clawgate

# 3. Connect Claude Code (in another terminal)
ANTHROPIC_BASE_URL=http://localhost:8082 claude
```

That's it. Claude Code now uses ChatGPT.

## API Key Mode

If you prefer using an OpenAI API key instead of your ChatGPT subscription:

```bash
./clawgate --mode=api --apiKey=sk-...
```

You can also point at any OpenAI-compatible endpoint (Azure, local vLLM, ...):

```bash
./clawgate --mode=api --apiKey=sk-xxx --baseUrl=https://my.endpoint/v1 --port=9000
```

## Configuration

See the full [Configuration & Usage guide](https://clawgate.org/usage.html) for all
options, examples, model mapping, troubleshooting, and more.

All settings are optional. The preferred way is CLI flags; environment
variables and a `.env` file are still honoured as a fallback (useful for
CI/containers). Precedence: **flag > env > .env > default**.

| Flag | Env fallback | Default | Description |
|---|---|---|---|
| `--mode` | `AUTH_MODE` | `chatgpt` | `chatgpt` (OAuth) or `api` (OpenAI API key) |
| `--apiKey` | `OPENAI_API_KEY` | — | Required when `--mode=api` |
| `--baseUrl` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Custom endpoint for `api` mode |
| `--bigModel` | `BIG_MODEL` | `gpt-5.4` | Model for opus requests |
| `--midModel` | `MID_MODEL` | `gpt-5.3-codex` | Model for sonnet requests |
| `--smallModel` | `SMALL_MODEL` | `gpt-5.2-codex` | Model for haiku requests |
| `--fast` | `FAST_MODE` | off | Send `service_tier: priority` in API requests |
| `--reason` | `REASON` | — | Reasoning effort for reasoning models: `none\|minimal\|low\|medium\|high\|xhigh` |
| `--port` | `PORT` | `8082` | Server port |

Environment-variable form still works unchanged:

```bash
AUTH_MODE=apikey OPENAI_API_KEY=sk-... ./clawgate
```

## Model Mapping

| Claude Code sends | Proxy routes to |
|---|---|
| `*haiku*` | `SMALL_MODEL` (default: gpt-5.2-codex) |
| `*sonnet*` | `MID_MODEL` (default: gpt-5.3-codex) |
| `*opus*` | `BIG_MODEL` (default: gpt-5.4) |

## Build from Source

```bash
go build -ldflags="-s -w" -o clawgate .
```

Cross-compile all platforms:

> **Note for Windows users:** the `Makefile` uses Unix shell commands
> (`rm`, `mkdir -p`), so run `make release` from Git Bash or WSL rather
> than `cmd.exe` / PowerShell.

```bash
make release
# Output: builds/clawgate-{linux,darwin,windows}-{amd64,arm64}
```

## Disclaimer

This is an independent open-source project, not affiliated with or
endorsed by OpenAI, Anthropic, or the OpenCode project. "Claude" and
"Claude Code" are trademarks of Anthropic PBC. "ChatGPT" and "OpenAI"
are trademarks of OpenAI, Inc.

The ChatGPT OAuth mode relies on the same Codex Responses API endpoint
used by OpenAI's official Codex CLI. This is an undocumented endpoint
whose behavior may change at any time. Use is at your own risk.

The API key mode uses only the official, documented OpenAI Chat
Completions API.
