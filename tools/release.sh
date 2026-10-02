#!/bin/sh
# Build release archives for every supported platform into dist/.
#   tools/release.sh v1.0.0          build only
# Publishing is a separate, manual step (see RELEASING.md). This script never pushes.
set -eu
VERSION="${1:?usage: tools/release.sh vX.Y.Z}"
case "$VERSION" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "version must look like v1.2.3" >&2; exit 1 ;; esac
cd "$(dirname "$0")/.."

tools/check-versions.sh "${VERSION#v}"
go test ./...
node --test assets/helpers/test/*.test.js assets/bridge/test/*.test.js

rm -rf dist && mkdir -p dist
for target in darwin/arm64 darwin/amd64 windows/amd64 windows/arm64 linux/amd64; do
  os="${target%/*}"; arch="${target#*/}"
  out="dist/cav_${os}_${arch}"
  mkdir -p "$out"
  bin="cav"; [ "$os" = windows ] && bin="cav.exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=${VERSION#v}" -o "$out/$bin" ./cmd/cav
  cp LICENSE NOTICE README.md "$out/"
  if [ "$os" = windows ]; then
    (cd "$out" && zip -q -r "../cav_${os}_${arch}.zip" .)
  else
    tar -C "$out" -czf "dist/cav_${os}_${arch}.tar.gz" .
  fi
  rm -rf "$out"
done
(cd dist && shasum -a 256 cav_* > checksums.txt)
cp install.sh install.ps1 dist/
echo "built $(ls dist | wc -l | tr -d ' ') files in dist/ for $VERSION"
