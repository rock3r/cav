#!/bin/bash
set -euo pipefail
[[ $# == 2 ]] || { echo "usage: $0 <archive.tar.gz> <version>" >&2; exit 64; }
[[ "$(uname -s)" == Darwin ]] || exit 69
[[ "$2" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || exit 64
workspace="$(mktemp -d)"
trap 'rm -rf "$workspace"' EXIT
tar -xzf "$1" -C "$workspace"
app="$workspace/cav-cli-$2/Cav.app"
test -x "$app/Contents/MacOS/cav"
codesign --verify --deep --strict --verbose=4 "$app"
# Verification alone accepts ad-hoc signatures. Require the release identity,
# secure timestamp and hardened runtime in both the executable and bundle.
for code in "$app/Contents/MacOS/cav" "$app"; do
 metadata="$(codesign -d --verbose=4 "$code" 2>&1)"
 grep -q '^Authority=Developer ID Application:' <<< "$metadata"
 grep -q '^TeamIdentifier=.' <<< "$metadata"
 grep -q '^Timestamp=' <<< "$metadata"
 grep -q 'flags=.*runtime' <<< "$metadata"
done
xcrun stapler validate "$app"
spctl --assess --type execute --verbose=4 "$app"
