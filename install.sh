#!/bin/sh
# Install or update the cav CLI on macOS, then run `cav setup`.
#   curl -fsSL https://raw.githubusercontent.com/rock3r/cavalry-skill/main/install.sh | sh
# Options (environment): CAV_VERSION=v0.1.0 to pin a release, CAV_BIN_DIR to choose the folder,
# CAV_REPO=owner/name for a fork, CAV_BASE_URL to download from a mirror or a local dist/ folder.
set -eu

REPO="${CAV_REPO:-rock3r/cavalry-skill}"
BIN_DIR="${CAV_BIN_DIR:-$HOME/.local/bin}"
VERSION="${CAV_VERSION:-latest}"

os="$(uname -s)"
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;   # Cavalry has no Linux build; this is for CI and relays only.
  *) echo "cav: unsupported OS $os (use install.ps1 on Windows)" >&2; exit 1 ;;
esac
arch="$(uname -m)"
case "$arch" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) echo "cav: unsupported CPU $arch" >&2; exit 1 ;;
esac

asset="cav_${os}_${arch}.tar.gz"
if [ -n "${CAV_BASE_URL:-}" ]; then
  base="$CAV_BASE_URL"   # a mirror, or file:///path/to/dist for testing
elif [ "$VERSION" = "latest" ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
echo "cav: downloading $asset ($VERSION) from $REPO"
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want="$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
got="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "cav: checksum mismatch for $asset; not installed" >&2
  exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/cav" "$BIN_DIR/cav"
echo "cav: installed $BIN_DIR/cav"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "cav: add $BIN_DIR to your PATH, for example: echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac

"$BIN_DIR/cav" setup || true
echo "cav: next, open Cavalry and click Scripts > cav-bridge (keep its window open), then run: cav doctor"
