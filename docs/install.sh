#!/bin/sh
# clawgate installer — https://clawgate.org
# Usage: curl -fsSL clawgate.org/install.sh | bash
#
# Detects OS and architecture, downloads the latest binary from GitHub
# Releases, installs to ~/.clawgate/bin, and adds it to PATH.

set -e

REPO="goclawgate/clawgate"
INSTALL_DIR="$HOME/.clawgate/bin"
BINARY="clawgate"

# ── Helpers ──────────────────────────────────────────────────────────

download() {
    local url="$1" output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL ${output:+-o "$output"} "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "${output:--}" "$url"
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi
}

# ── Detect OS ────────────────────────────────────────────────────────

OS="$(uname -s)"
case "$OS" in
    Linux*)  OS="linux" ;;
    Darwin*) OS="darwin" ;;
    MINGW*|MSYS*|CYGWIN*)
        echo "Error: This script doesn't support Windows."
        echo "Run this in PowerShell instead:"
        echo "  irm clawgate.org/install.ps1 | iex"
        exit 1
        ;;
    *)
        echo "Error: Unsupported OS: $OS" >&2
        exit 1
        ;;
esac

# ── Detect architecture ─────────────────────────────────────────────

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Error: Unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

# Detect Rosetta 2: if running x64 under Rosetta on an ARM Mac,
# download the native arm64 binary instead.
if [ "$OS" = "darwin" ] && [ "$ARCH" = "amd64" ]; then
    if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
        ARCH="arm64"
    fi
fi

# ── Download ─────────────────────────────────────────────────────────

ASSET="clawgate-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo ""
echo "  clawgate installer"
echo "  ───────────────────"
echo "  OS:   $OS"
echo "  Arch: $ARCH"
echo ""

TMPFILE="$(mktemp)"
trap 'rm -f "$TMPFILE"' EXIT

echo "  Downloading from GitHub..."
if ! download "$URL" "$TMPFILE"; then
    echo ""
    echo "  Error: Download failed." >&2
    echo "  Check https://github.com/${REPO}/releases for available assets." >&2
    exit 1
fi

# ── Install ──────────────────────────────────────────────────────────

chmod +x "$TMPFILE"

# Remove macOS quarantine flag
if [ "$OS" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "$TMPFILE" 2>/dev/null || true
fi

mkdir -p "$INSTALL_DIR"
mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
trap - EXIT

echo "  Installed to ${INSTALL_DIR}/${BINARY}"

# ── Add to PATH ──────────────────────────────────────────────────────

add_to_path() {
    local rc_file="$1"
    local line="export PATH=\"${INSTALL_DIR}:\$PATH\""

    # Don't add if already present
    if [ -f "$rc_file" ] && grep -qF "$INSTALL_DIR" "$rc_file" 2>/dev/null; then
        return 0
    fi

    echo "" >> "$rc_file"
    echo "# clawgate" >> "$rc_file"
    echo "$line" >> "$rc_file"
    return 0
}

PATH_ADDED=false
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    SHELL_NAME="$(basename "${SHELL:-sh}" 2>/dev/null)"
    case "$SHELL_NAME" in
        zsh)
            add_to_path "$HOME/.zshrc"
            PATH_ADDED=true
            ;;
        bash)
            if [ "$OS" = "darwin" ]; then
                add_to_path "$HOME/.bash_profile"
            else
                add_to_path "$HOME/.bashrc"
            fi
            PATH_ADDED=true
            ;;
        fish)
            FISH_CONF="$HOME/.config/fish/config.fish"
            if [ ! -f "$FISH_CONF" ] || ! grep -qF "$INSTALL_DIR" "$FISH_CONF" 2>/dev/null; then
                mkdir -p "$(dirname "$FISH_CONF")"
                echo "" >> "$FISH_CONF"
                echo "# clawgate" >> "$FISH_CONF"
                echo "fish_add_path ${INSTALL_DIR}" >> "$FISH_CONF"
            fi
            PATH_ADDED=true
            ;;
        *)
            add_to_path "$HOME/.profile"
            PATH_ADDED=true
            ;;
    esac
fi

# ── Done ─────────────────────────────────────────────────────────────

echo ""
if [ "$PATH_ADDED" = true ]; then
    echo "  Added ${INSTALL_DIR} to PATH."
    echo "  Restart your terminal or run:"
    case "$SHELL_NAME" in
        zsh)  echo "    source ~/.zshrc" ;;
        bash)
            if [ "$OS" = "darwin" ]; then
                echo "    source ~/.bash_profile"
            else
                echo "    source ~/.bashrc"
            fi
            ;;
        fish) echo "    source ~/.config/fish/config.fish" ;;
        *)    echo "    source ~/.profile" ;;
    esac
    echo ""
fi

echo "  Installation complete!"
echo ""
echo "  Get started:"
echo "    clawgate login          # authenticate with ChatGPT"
echo "    clawgate                # start the proxy"
echo "    ANTHROPIC_BASE_URL=http://localhost:8082 claude"
echo ""
