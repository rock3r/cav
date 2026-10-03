#!/bin/sh
# Build release archives for every supported platform into dist/.
#   tools/release.sh v1.0.0          build only
# Publishing is a separate, manual step (see RELEASING.md). This script never pushes.
set -eu
VERSION="${1:?usage: tools/release.sh vX.Y.Z [--dev]}"
MODE=release
if [ "${2:-}" = --dev ]; then MODE=dev; elif [ -n "${2:-}" ]; then exit 64; fi
case "$VERSION" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "version must look like v1.2.3" >&2; exit 1 ;; esac
cd "$(dirname "$0")/.."

# Fail before building or touching dist when release signing is unavailable.
if [ "$MODE" = release ]; then
 [ "$(uname -s)" = Darwin ] || { echo 'release artifacts require a macOS signing host; use --dev for local fixtures' >&2; exit 69; }
 : "${APPLE_DEVELOPER_IDENTITY:?release signing required}"
 : "${APPLE_NOTARY_API_KEY_PATH:?notary API key path required}"
 : "${APPLE_NOTARY_API_KEY_ID:?notary API key id required}"
 : "${APPLE_NOTARY_API_ISSUER:?notary issuer required}"
fi
tools/check-versions.sh "${VERSION#v}"
go test ./...
node --test assets/helpers/test/*.test.js assets/bridge/test/*.test.js assets/diagnostics/test/*.test.js

rm -rf dist && mkdir -p dist
for target in darwin/arm64 darwin/amd64 windows/amd64 windows/arm64 linux/amd64; do
  os="${target%/*}"; arch="${target#*/}"
  out="dist/cav_${os}_${arch}"
  mkdir -p "$out"
  bin="cav"; [ "$os" = windows ] && bin="cav.exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=${VERSION#v}" -o "$out/$bin" ./cmd/cav
  cp LICENSE NOTICE README.md "$out/"
  if [ "$os" = darwin ]; then
    tools/package-macos.sh "$out/$bin" "${VERSION#v}" "dist/cav_${os}_${arch}.tar.gz" "$MODE"
  elif [ "$os" = windows ]; then
    (cd "$out" && zip -q -r "../cav_${os}_${arch}.zip" .)
  else
    tar -C "$out" -czf "dist/cav_${os}_${arch}.tar.gz" .
  fi
  rm -rf "$out"
done
(cd dist && shasum -a 256 cav_* > checksums.txt)
cp install.sh install.ps1 dist/
echo "$MODE: built $(ls dist | wc -l | tr -d ' ') files in dist/ for $VERSION"
