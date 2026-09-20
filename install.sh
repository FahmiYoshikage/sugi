#!/bin/sh
set -e

REPO="FahmiYoshikage/sugi"

echo "=== Installing Sugi Observability Engine ==="

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
if [ "$OS" != "linux" ]; then
  echo "Error: Sugi currently supports Linux (/proc pseudo-filesystem). Detected OS: $OS"
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)
    TARGET_ARCH="amd64"
    ;;
  aarch64|arm64)
    TARGET_ARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture: $ARCH (supported: amd64, arm64)"
    exit 1
    ;;
esac

BINARY_NAME="sugi-linux-${TARGET_ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}"

echo "Detected Linux (${TARGET_ARCH})."
echo "Downloading Sugi binary from GitHub Releases..."

# Use temporary file to prevent ETXTBSY if an existing sugi process is currently running
TMP_BIN="./sugi.tmp.$$"
trap 'rm -f "$TMP_BIN"' EXIT INT TERM

if command -v curl >/dev/null 2>&1; then
  curl -fL --progress-bar -o "$TMP_BIN" "$DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
  wget --show-progress -O "$TMP_BIN" "$DOWNLOAD_URL"
else
  echo "Error: curl or wget is required to download Sugi."
  exit 1
fi

chmod +x "$TMP_BIN"
# Atomically replace target binary using rename
mv -f "$TMP_BIN" sugi
trap - EXIT INT TERM

echo ""
echo "Successfully installed './sugi'!"
echo ""
echo "To start Sugi:"
echo "  ./sugi"
echo ""
echo "Then open your browser at: http://localhost:8080"
