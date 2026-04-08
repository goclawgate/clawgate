# Configuration & Usage

## Modes

clawgate has two modes of operation:

### ChatGPT mode (default)

Uses your existing ChatGPT Plus or Pro subscription. No API key needed.

```bash
# First time: log in
clawgate login

# Then run the proxy
clawgate
```

The login flow opens a browser-based OAuth authorization page. You enter
a code shown in your terminal, approve the connection, and clawgate
saves the token to `~/.clawgate/token.json`. Tokens auto-refresh when
they expire.

### API key mode

Uses a standard OpenAI (or compatible) API key. No login step needed.

```bash
clawgate --mode=api --apiKey=sk-xxx
```

This mode works with any OpenAI-compatible endpoint, including:
- OpenAI API
- Azure OpenAI
- Local inference servers (vLLM, Ollama with OpenAI compat, etc.)
- Any service that speaks the OpenAI Chat Completions format

To use a custom endpoint:

```bash
clawgate --mode=api --apiKey=sk-xxx --baseUrl=https://my.endpoint/v1
```

## CLI flags

All flags are optional. Flags take precedence over environment variables.

| Flag | Default | Description |
|---|---|---|
| `--mode` | `chatgpt` | Auth mode: `chatgpt` or `api` |
| `--apiKey` | -- | OpenAI API key (required when `--mode=api`) |
| `--baseUrl` | `https://api.openai.com/v1` | OpenAI-compatible base URL (api mode only) |
| `--bigModel` | `gpt-5.4` | Model for opus requests |
| `--midModel` | `gpt-5.3-codex` | Model for sonnet requests |
| `--smallModel` | `gpt-5.2-codex` | Model for haiku requests |
| `--fast` | off | Enable fast mode (sends `service_tier: priority`) |
| `--reason` | -- | Reasoning effort: `none`, `minimal`, `low`, `medium`, `high`, `xhigh` (reasoning models only) |
| `--host` | `127.0.0.1` | Bind address (default: localhost only) |
| `--port` | `8082` | Server port |
| `--account` | default | Account to use for this run (ChatGPT mode, one-run override) |
| `--help` | -- | Show help and exit |

Both single-dash and double-dash work: `--mode=api` and `-mode=api` are
equivalent. Both `--flag=value` and `--flag value` forms are accepted.

## Subcommands

| Command | Description |
|---|---|
| `clawgate login [flags]` | Authenticate with ChatGPT (OAuth device flow) |
| `clawgate logout [flags]` | Remove saved credentials |
| `clawgate status [flags]` | Check login status and token validity |
| `clawgate account list` | List saved accounts (the default is marked with `*`) |
| `clawgate account use NAME` | Set the persistent default account |
| `clawgate help` | Show help text |

### `login` flags

| Flag | Description |
|---|---|
| `--name NAME` | Account name to save this login under (default: `default`, or the current default if one exists) |
| `--default` | Make this account the persistent default after login |
| `--replace` | Overwrite an existing account with the same name |

### `logout` flags

| Flag | Description |
|---|---|
| `--account NAME` | Remove a specific account (defaults to the current default) |
| `--all` | Remove every saved account |

### `status` flags

| Flag | Description |
|---|---|
| `--account NAME` | Check a specific account instead of the default |

## Account management

clawgate supports multiple ChatGPT accounts side by side. Each account
keeps its own OAuth token and can be selected per proxy run without
disturbing the persistent default.

```bash
# First login creates an account named "default" and makes it default
clawgate login

# Add a second account
clawgate login --name work

# Add a third and immediately make it default
clawgate login --name personal --default

# List all accounts (* marks the default)
clawgate account list

# Switch the persistent default
clawgate account use work

# Start the proxy using a non-default account for this run only
clawgate --account personal

# Check a specific account's token status
clawgate status --account work

# Remove a single account
clawgate logout --account work

# Remove everything
clawgate logout --all
```

**Account name rules:** lowercase letters, digits, `.`, `_`, or `-`,
starting with a letter or digit, 1–32 characters. The name `all` is
reserved.

**Rules:**
- `clawgate login` with no `--name` re-authenticates the current default
  (or creates one named `default` on first use).
- First account ever saved automatically becomes the default.
- `--account NAME` on a proxy run is a one-run override; it does NOT
  change the persistent default.
- Removing the default account leaves `default_account` empty; the next
  run asks you to pick one with `clawgate account use NAME`.
- In API key mode, `--account` is ignored — no auth store is involved.

## Environment variables

Environment variables work as a fallback when flags are not set. Useful
for CI/CD, Docker, and other automated setups.

| Variable | Equivalent flag |
|---|---|
| `AUTH_MODE` | `--mode` |
| `OPENAI_API_KEY` | `--apiKey` |
| `OPENAI_BASE_URL` | `--baseUrl` |
| `BIG_MODEL` | `--bigModel` |
| `MID_MODEL` | `--midModel` |
| `SMALL_MODEL` | `--smallModel` |
| `FAST_MODE` | `--fast` |
| `REASON` | `--reason` (alias: `REASONING_EFFORT`) |
| `HOST` | `--host` |
| `PORT` | `--port` |
| `DEBUG` | -- (set to `1` to enable debug logging) |

You can also place these in a `.env` file in the working directory.

**Precedence order:** CLI flag > environment variable > `.env` file > default.

Example using environment variables:

```bash
AUTH_MODE=apikey OPENAI_API_KEY=sk-xxx clawgate
```

## Model mapping

clawgate maps Anthropic model names to OpenAI model names:

| Claude Code sends | clawgate routes to |
|---|---|
| Any model containing `haiku` | `--smallModel` (default: `gpt-5.2-codex`) |
| Any model containing `sonnet` | `--midModel` (default: `gpt-5.3-codex`) |
| Any model containing `opus` | `--bigModel` (default: `gpt-5.4`) |
| Any other model | `--bigModel` |

The `anthropic/`, `openai/`, and `gemini/` prefixes are stripped before
matching, so `anthropic/claude-3-5-sonnet` and `claude-3-5-sonnet` both
route to the same model.

Override the defaults:

**ChatGPT mode** — the Codex backend only accepts a fixed set of models:
`gpt-5.1-codex-max`, `gpt-5.1-codex-mini`, `gpt-5.2-codex`, `gpt-5.3-codex`,
`gpt-5.4`. clawgate prints a warning at startup if you pick anything else.

```bash
clawgate --bigModel=gpt-5.4 --midModel=gpt-5.3-codex --smallModel=gpt-5.2-codex
```

**API key mode** — any model your endpoint supports works:

```bash
clawgate --mode=api --apiKey=sk-xxx --bigModel=gpt-4o --midModel=gpt-4o --smallModel=gpt-4o-mini
```

### Reasoning effort

GPT-5 and o-series models accept a `reasoning_effort` knob that trades
quality for cost/latency. clawgate exposes it as `--reason` (env
`REASON`), using Codex CLI's exact vocabulary so values transfer
verbatim:

| Value | Meaning |
|---|---|
| `none` | Skip the reasoning step entirely |
| `minimal` | Smallest amount of reasoning |
| `low` | Light reasoning |
| `medium` | Default for most reasoning models |
| `high` | More thorough reasoning |
| `xhigh` | Maximum reasoning (currently only meaningful for `gpt-5.1-codex-max`) |

```bash
clawgate --reason=high
clawgate --mode=api --apiKey=sk-xxx --reason=minimal
```

**Precedence:** if both `--reason` and per-request `thinking`
(`budget_tokens`) are set, whichever maps to the **higher** effort wins.
If only one is set, that value applies. If neither is set, the proxy
leaves the field unset and upstream uses its model default (typically
`medium`).

The budget-to-effort mapping used for per-request `thinking`:

| Budget range | Effort |
|---|---|
| >= 32000 | `xhigh` |
| >= 10000 | `high` |
| >= 4000  | `medium` |
| >= 1000  | `low` |
| > 0      | `minimal` |
| <= 0     | `none` |

The flag is silently ignored for non-reasoning models such as `gpt-4o`
or `gpt-4`. Unknown values trigger a startup warning but are still
forwarded — the upstream catalog may grow without a proxy release.

## Connecting Claude Code

Once clawgate is running, point Claude Code at it by setting the
`ANTHROPIC_BASE_URL` environment variable:

```bash
ANTHROPIC_BASE_URL=http://localhost:8082 claude
```

On Windows (Command Prompt):

```
set ANTHROPIC_BASE_URL=http://localhost:8082
claude
```

On Windows (PowerShell):

```powershell
$env:ANTHROPIC_BASE_URL="http://localhost:8082"; claude
```

If you changed the port with `--port`, adjust the URL accordingly.

## Debug mode

Set `DEBUG=1` to log all incoming Anthropic requests and outgoing
OpenAI requests to the console:

```bash
DEBUG=1 clawgate
```

This is useful for troubleshooting request translation issues or
verifying that tool calls are mapped correctly.

## Examples

```bash
# ChatGPT mode, default settings
clawgate

# API key mode, default models
clawgate --mode=api --apiKey=sk-xxx

# Custom models and port
clawgate --mode=api --apiKey=sk-xxx --bigModel=gpt-4o --smallModel=gpt-4o-mini --port=9000

# Azure OpenAI
clawgate --mode=api --apiKey=xxx --baseUrl=https://myresource.openai.azure.com/openai/deployments/gpt-4o/v1

# Local vLLM
clawgate --mode=api --apiKey=dummy --baseUrl=http://localhost:8000/v1

# Docker / CI (environment variables)
AUTH_MODE=apikey OPENAI_API_KEY=sk-xxx PORT=8082 clawgate
```

## Files and directories

| Path | Description |
|---|---|
| `~/.clawgate/bin/clawgate` | Binary (installed by install script) |
| `~/.clawgate/token.json` | Account store — holds all saved ChatGPT OAuth accounts and the default selection |
| `.env` | Optional env file in the working directory |

## Troubleshooting

**"command not found" after install**
Close and reopen your terminal. If it still doesn't work, verify that
the directory containing the `clawgate` binary is in your PATH.

**"Not logged in" error**
Run `clawgate login` to authenticate.

**"API key is required" error**
Pass `--apiKey=sk-...` when using `--mode=api`, or set the
`OPENAI_API_KEY` environment variable.

**macOS "unidentified developer" dialog**
Run `xattr -d com.apple.quarantine clawgate` before first use. See
the [macOS install guide](install-macos.html) for details.

**Windows SmartScreen warning**
Click "More info" then "Run anyway". See the
[Windows install guide](install-windows.html) for details.

**429 / rate limit errors**
clawgate retries rate-limited requests automatically (up to 3 times
with backoff). If you see persistent 429s, you're hitting the upstream
provider's rate limit -- wait a moment or reduce request frequency.
