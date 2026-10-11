#!/bin/sh
# Zip the plugin folder for upload to the OpenAI plugin marketplace.
#   tools/package-codex-plugin.sh            zip the plugin at HEAD
#   tools/package-codex-plugin.sh v1.3.0     zip the plugin at that tag (or any commit)
# Writes build/cav-codex-plugin-<version>.zip, with .codex-plugin/plugin.json at the top
# level. It zips the committed files of the ref, so local changes are never included.
set -eu
REF="${1:-HEAD}"
cd "$(dirname "$0")/.."

git rev-parse -q --verify "$REF^{commit}" >/dev/null || { echo "unknown ref: $REF" >&2; exit 1; }
manifest="$(git show "$REF:plugins/cavalry/.codex-plugin/plugin.json")"
version="$(printf '%s\n' "$manifest" | sed -nE 's/.*"version": "([^"]+)".*/\1/p' | head -1)"
[ -n "$version" ] || { echo "no version in .codex-plugin/plugin.json at $REF" >&2; exit 1; }
case "$REF" in
  v[0-9]*) [ "${REF#v}" = "$version" ] || { echo "$REF has plugin version $version" >&2; exit 1; } ;;
esac

out="build/cav-codex-plugin-$version.zip"
mkdir -p build
rm -f "$out"
git archive --format=zip -o "$out" "$REF:plugins/cavalry"
echo "$out"
