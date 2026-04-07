# Installing clawgate on Linux

## Quick Install (recommended)

```bash
curl -fsSL clawgate.org/install.sh | bash
```

This auto-detects your architecture, downloads the right binary,
installs it to `~/.clawgate/bin/`, and adds it to your PATH
automatically.

## Manual Download

```bash
# x86_64 (most common)
curl -L -o clawgate https://github.com/goclawgate/clawgate/releases/latest/download/clawgate-linux-amd64

# ARM64 (Raspberry Pi 4/5, AWS Graviton, etc.)
curl -L -o clawgate https://github.com/goclawgate/clawgate/releases/latest/download/clawgate-linux-arm64
```

## Make it executable

```bash
chmod +x clawgate
```

## Move to a directory in your PATH

Option A -- system-wide (requires sudo):

```bash
sudo mv clawgate /usr/local/bin/
```

Option B -- user-only (no sudo needed):

```bash
mkdir -p ~/.local/bin
mv clawgate ~/.local/bin/
```

If using Option B, make sure `~/.local/bin` is in your PATH. Add this
to your shell config (`~/.bashrc`, `~/.zshrc`, or `~/.profile`):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload:

```bash
source ~/.bashrc   # or ~/.zshrc
```

## Verify

```bash
clawgate --help
```

If you get `command not found`, close and reopen your terminal.

## Login (ChatGPT mode)

```bash
clawgate login
```

Follow the on-screen instructions to authenticate with your ChatGPT
account.

## Start the proxy

```bash
# ChatGPT mode (default)
clawgate

# API key mode
clawgate --mode=api --apiKey=sk-xxx
```

## Connect Claude Code

In a separate terminal:

```bash
ANTHROPIC_BASE_URL=http://localhost:8082 claude
```

## Run on startup (optional)

### systemd (Ubuntu, Debian, Fedora, etc.)

Create `/etc/systemd/system/clawgate.service`:

```ini
[Unit]
Description=clawgate proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/clawgate --mode=api --apiKey=sk-xxx
Restart=on-failure
User=nobody

[Install]
WantedBy=multi-user.target
```

Then enable it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now clawgate
```

## Uninstall

```bash
# Remove the binary
sudo rm /usr/local/bin/clawgate
# or: rm ~/.local/bin/clawgate

# Remove saved credentials
rm -rf ~/.clawgate
```
