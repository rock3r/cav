#!/bin/sh
# Install or update the cav CLI on macOS, then run `cav setup`.
#   curl -fsSL https://raw.githubusercontent.com/rock3r/cav/main/install.sh | sh
# Options (environment): CAV_VERSION=v1.0.0 to pin a release, CAV_BIN_DIR to choose the folder,
# CAV_REPO=owner/name for a fork, CAV_BASE_URL to download from a mirror or a local dist/ folder.
set -eu

REPO="${CAV_REPO:-rock3r/cav}"
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
if [ "$os" = darwin ]; then
  app="$(find "$tmp" -type d -name Cav.app -prune)"
  if [ -n "$app" ]; then
    # Verify the complete downloaded bundle; never repair signatures or remove quarantine.
    test -x "$app/Contents/MacOS/cav"
    codesign --verify --deep --strict "$app"
    metadata="$(codesign -d --verbose=4 "$app" 2>&1)"
    printf '%s\n' "$metadata" | grep -q '^Authority=Developer ID Application:'
    printf '%s\n' "$metadata" | grep -q '^Timestamp='
    printf '%s\n' "$metadata" | grep -q 'flags=.*runtime'
    xcrun stapler validate "$app"
    spctl --assess --type execute "$app"
    mkdir -p "$BIN_DIR/.cav-bundles"
    destination="$(mktemp -d "$BIN_DIR/.cav-bundles/install.XXXXXX")"
    # ditto keeps signature/ticket metadata and modes across filesystems.
    ditto "$app" "$destination/Cav.app"
    codesign --verify --deep --strict "$destination/Cav.app"
    xcrun stapler validate "$destination/Cav.app"
    spctl --assess --type execute "$destination/Cav.app"
    destination="$(cd "$destination" && pwd)"
    ln -s "$destination/Cav.app/Contents/MacOS/cav" "$tmp/cav-link"
    # Move on the destination filesystem so replacing an existing symlink/file is atomic.
    link_stage="$destination/path-link"
    mv "$tmp/cav-link" "$link_stage"
    mv -f "$link_stage" "$BIN_DIR/cav"
  elif [ "${CAV_ALLOW_LEGACY_MACOS:-}" = 1 ] && [ "$VERSION" != latest ]; then
    # Explicit opt-in for historical bare-binary releases only.
    if [ -L "$BIN_DIR/cav" ]; then echo "refusing legacy downgrade through an installed bundle link" >&2; exit 1; fi
    install -m 0755 "$tmp/cav" "$BIN_DIR/cav"
  else
    echo 'cav: signed Cav.app missing; historical releases require a pinned CAV_VERSION and CAV_ALLOW_LEGACY_MACOS=1' >&2
    exit 1
  fi
else
  install -m 0755 "$tmp/cav" "$BIN_DIR/cav"
fi
echo "cav: installed $BIN_DIR/cav"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "cav: add $BIN_DIR to your PATH, for example: echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac

"$BIN_DIR/cav" setup || true
echo "cav: next, open Cavalry and click Scripts > cav-bridge (keep its window open), then run: cav doctor"
