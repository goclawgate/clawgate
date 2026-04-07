# Installing clawgate on macOS

## Quick Install (recommended)

```bash
curl -fsSL clawgate.org/install.sh | bash
```

This auto-detects your chip (Intel vs Apple Silicon), installs to
`~/.clawgate/bin/`, adds it to your PATH, and removes the macOS
quarantine flag automatically.

## Manual Download

```bash
# Apple Silicon (M1/M2/M3/M4 — most modern Macs)
curl -L -o clawgate https://github.com/goclawgate/clawgate/releases/latest/download/clawgate-darwin-arm64

# Intel Mac
curl -L -o clawgate https://github.com/goclawgate/clawgate/releases/latest/download/clawgate-darwin-amd64
```

Not sure which chip you have? Click the Apple menu > **About This Mac**.
If it says "Apple M1/M2/M3/M4", use `darwin-arm64`. If it says "Intel",
use `darwin-amd64`.

## Make it executable

```bash
chmod +x clawgate
```

## Remove the quarantine flag

macOS blocks unsigned binaries downloaded from the internet. Remove the
quarantine attribute so it can run:

```bash
xattr -d com.apple.quarantine clawgate
```

If you skip this step, macOS will show a dialog saying the app "can't be
opened because it is from an unidentified developer."

## Move to a directory in your PATH

Option A -- `/usr/local/bin` (most common):

```bash
sudo mv clawgate /usr/local/bin/
```

Option B -- user-only (no sudo needed):

```bash
mkdir -p ~/.local/bin
mv clawgate ~/.local/bin/
```

If using Option B, add `~/.local/bin` to your PATH. Add this line to
your shell config:

For **zsh** (default on modern macOS) -- edit `~/.zshrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

For **bash** -- edit `~/.bash_profile`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload:

```bash
source ~/.zshrc   # or ~/.bash_profile
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

### launchd

Create `~/Library/LaunchAgents/org.clawgate.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>org.clawgate</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/clawgate</string>
        <string>--mode=api</string>
        <string>--apiKey=sk-xxx</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Then load it:

```bash
launchctl load ~/Library/LaunchAgents/org.clawgate.plist
```

## Uninstall

```bash
# Remove the binary
sudo rm /usr/local/bin/clawgate
# or: rm ~/.local/bin/clawgate

# Remove saved credentials
rm -rf ~/.clawgate

# Remove launchd service (if installed)
launchctl unload ~/Library/LaunchAgents/org.clawgate.plist
rm ~/Library/LaunchAgents/org.clawgate.plist
```
