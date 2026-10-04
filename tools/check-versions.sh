#!/bin/sh
# Check (or with --set, write) the release version in every manifest, the bridge script and
# the helper library and the development CLI default.
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
  sed -i.bak -E "s/^\/\/ cav-bridge VERSION .*/\/\/ cav-bridge VERSION $v/; s/^var BRIDGE_VERSION = '[^']+'/var BRIDGE_VERSION = '$v'/" assets/bridge/cav-bridge.js && rm assets/bridge/cav-bridge.js.bak
  sed -i.bak -E "s/cav\.VERSION = '[^']+'/cav.VERSION = '$v'/" assets/helpers/cav-helpers.js && rm assets/helpers/cav-helpers.js.bak
  sed -i.bak -E "s/^var version = \"[^\"]+\"/var version = \"$v-dev\"/" cmd/cav/main.go && rm cmd/cav/main.go.bak
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
got="$(sed -nE "s/^var BRIDGE_VERSION = '([^']+)'/\1/p" assets/bridge/cav-bridge.js)"
[ "$got" = "$want" ] || { echo "cav-bridge.js has version $got, want $want" >&2; bad=1; }
got="$(sed -nE "s/.*cav\.VERSION = '([^']+)'.*/\1/p" assets/helpers/cav-helpers.js)"
[ "$got" = "$want" ] || { echo "cav-helpers.js has version $got, want $want" >&2; bad=1; }
got="$(sed -nE 's/^var version = "([^"]+)-dev"/\1/p' cmd/cav/main.go)"
[ "$got" = "$want" ] || { echo "cmd/cav/main.go has development version $got, want $want-dev" >&2; bad=1; }
exit $bad
