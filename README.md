# clawgate

Use **Claude Code** with **ChatGPT** or any OpenAI-compatible backend.

Claude Code speaks the Anthropic API. clawgate sits in the middle, translates every request to OpenAI format on the fly, and sends it upstream. You get to use your ChatGPT subscription or any OpenAI-compatible backend — no code changes, no config hacks.

---

## Features

| | |
|---|---|
| **Two auth modes** | ChatGPT Codex OAuth (use your subscription, no API key) or standard OpenAI API key |
| **Single binary** | ~6 MB, zero dependencies, pure Go stdlib |
| **Streaming** | Full SSE support with error handling and auto-retry on 429s |
| **Tool calls** | Complete `tool_use` round-trip translation |
| **Reasoning** | Maps Anthropic `thinking` to OpenAI `reasoning_effort` automatically |
| **Cross-platform** | Linux, macOS, Windows — amd64 and arm64 |

---

## Install

### Linux / macOS

```bash
curl -fsSL clawgate.org/install.sh | bash
```

### Windows

**PowerShell:**
```powershell
irm clawgate.org/install.ps1 | iex
```

**CMD:**
```cmd
curl -fsSL clawgate.org/install.cmd -o install.cmd && install.cmd && del install.cmd
```

Installs to `~/.clawgate/bin/` and adds it to PATH automatically.

> Platform-specific guides (manual download, PATH setup, startup config):
> [Linux](https://clawgate.org/install-linux.html) ·
> [macOS](https://clawgate.org/install-macos.html) ·
> [Windows](https://clawgate.org/install-windows.html)

Or grab the binary from [Releases](https://github.com/goclawgate/clawgate/releases):

| Platform | File |
|---|---|
| Linux x86_64 | `clawgate-linux-amd64` |
| Linux ARM | `clawgate-linux-arm64` |
| macOS Intel | `clawgate-darwin-amd64` |
| macOS Apple Silicon | `clawgate-darwin-arm64` |
| Windows | `clawgate-windows-amd64.exe` |

---

## Quick Start

### ChatGPT mode (default)

Use your existing ChatGPT Plus/Pro subscription. No API key needed.

```bash
# One-time login
clawgate login

# Start the proxy
clawgate

# In another terminal — that's it
ANTHROPIC_BASE_URL=http://localhost:8082 claude
```

### API key mode

Use a standard OpenAI (or compatible) API key instead:

```bash
clawgate --mode=api --apiKey=sk-...
```

Point at any OpenAI-compatible endpoint (Azure, vLLM, Ollama, etc.):

```bash
clawgate --mode=api --apiKey=sk-xxx --baseUrl=https://my.endpoint/v1
```

---

## Model Mapping

clawgate maps Anthropic model names to OpenAI models automatically:

| Claude Code requests | clawgate sends to | Default model |
|---|---|---|
| anything with **haiku** | `--smallModel` | `gpt-5.2-codex` |
| anything with **sonnet** | `--midModel` | `gpt-5.3-codex` |
| anything with **opus** | `--bigModel` | `gpt-5.4` |

Prefixes like `anthropic/`, `openai/`, `gemini/` are stripped before matching.

---

## Configuration

All settings are optional. **Precedence: flag > env > .env > default.**

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--mode` | `AUTH_MODE` | `chatgpt` | `chatgpt` or `api` |
| `--apiKey` | `OPENAI_API_KEY` | — | Required for `--mode=api` |
| `--baseUrl` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Custom endpoint (api mode) |
| `--bigModel` | `BIG_MODEL` | `gpt-5.4` | Model for opus |
| `--midModel` | `MID_MODEL` | `gpt-5.3-codex` | Model for sonnet |
| `--smallModel` | `SMALL_MODEL` | `gpt-5.2-codex` | Model for haiku |
| `--fast` | `FAST_MODE` | off | `service_tier: priority` |
| `--reason` | `REASON` | — | Reasoning effort: `none` / `minimal` / `low` / `medium` / `high` / `xhigh` |
| `--host` | `HOST` | `127.0.0.1` | Bind address |
| `--port` | `PORT` | `8082` | Server port |

Environment variables and `.env` files work as fallback (useful for CI/Docker):

```bash
AUTH_MODE=apikey OPENAI_API_KEY=sk-... clawgate
```

See the full [Configuration & Usage guide](https://clawgate.org/usage.html) for reasoning effort details, troubleshooting, and more examples.

---

## Build from Source

```bash
go build -ldflags="-s -w" -o clawgate .
```

Cross-compile all platforms:

```bash
make release
# Output: builds/clawgate-{linux,darwin}-{amd64,arm64} and builds/clawgate-windows-amd64.exe
```

> **Windows:** the Makefile uses Unix shell commands — run `make release` from Git Bash or WSL.

---

## Disclaimer

Independent open-source project, not affiliated with or endorsed by OpenAI or Anthropic. "Claude" and "Claude Code" are trademarks of Anthropic PBC. "ChatGPT" and "OpenAI" are trademarks of OpenAI, Inc.

ChatGPT OAuth mode uses the same Codex Responses API endpoint as OpenAI's official Codex CLI. This is an undocumented endpoint whose behavior may change at any time. Use at your own risk.

API key mode uses only the official, documented OpenAI Chat Completions API.
