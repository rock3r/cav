#!/bin/sh
# Check (or with --set, write) the plugin version in every manifest.
#   tools/check-versions.sh 1.0.0          fail if any manifest differs
#   tools/check-versions.sh --set 1.1.0    write the version everywhere
set -eu
cd "$(dirname "$0")/.."
files="plugins/cavalry/plugin.json plugins/cavalry/.claude-plugin/plugin.json plugins/cavalry/.codex-plugin/plugin.json"
if [ "${1:-}" = "--set" ]; then
  v="${2:?version}"
  for f in $files; do
    sed -i.bak -E "s/\"version\": \"[^\"]+\"/\"version\": \"$v\"/" "$f" && rm "$f.bak"
  done
  sed -i.bak -E "s/^  version: \"[^\"]+\"/  version: \"$v\"/" plugins/cavalry/skills/cavalry/SKILL.md && rm plugins/cavalry/skills/cavalry/SKILL.md.bak
  echo "set version $v"
  exit 0
fi
want="${1:?version}"
bad=0
for f in $files; do
  got="$(sed -nE 's/.*"version": "([^"]+)".*/\1/p' "$f" | head -1)"
  [ "$got" = "$want" ] || { echo "$f has version $got, want $want" >&2; bad=1; }
done
got="$(sed -nE 's/^  version: "([^"]+)"/\1/p' plugins/cavalry/skills/cavalry/SKILL.md)"
[ "$got" = "$want" ] || { echo "SKILL.md has version $got, want $want" >&2; bad=1; }
exit $bad
