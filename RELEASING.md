# Releasing

One version number covers the CLI, the plugin, the bridge script and the helper library:
`vMAJOR.MINOR.PATCH` (semantic versioning). `tools/check-versions.sh` sets and checks it in
every file, and a packaging test fails when the files disagree.

Compatibility between `cav` and a running bridge is a separate number: the protocol
(`Protocol` in `internal/bridge/client.go`, `PROTOCOL` in `cav-bridge.js`). Change both only
when the request or result format changes. A bridge refuses requests with another protocol,
and `cav doctor` fails on a mismatch. When only the version differs, an older bridge keeps
working, and `cav doctor` suggests a restart.

| What changed | Bump |
|---|---|
| Bug fix, docs, skill wording | PATCH |
| New command, flag, helper or recipe | MINOR |
| Removed or renamed command, flag or helper; bridge protocol change | MAJOR |
| Bridge request or result format | MAJOR, and the protocol number in both places |

## Steps

1. Set the plugin version everywhere: `tools/check-versions.sh --set 1.1.0`.
2. Add a section for the version to `CHANGELOG.md`, then commit: `Release 1.1.0`.
3. Build: `tools/release.sh v1.1.0`. It runs the tests, then writes `dist/` with one archive per
   platform (`cav_<os>_<arch>.tar.gz` or `.zip`), `checksums.txt` and the install scripts.
4. Run the live helper tests in a throwaway Cavalry scene (see README, Development).
5. **Only with the maintainer's approval**: tag and publish.

   ```bash
   git tag v1.1.0 && git push origin main v1.1.0
   gh release create v1.1.0 dist/* --title v1.1.0 --notes-file <that CHANGELOG section>
   ```

`cav update` and the install scripts download from the latest GitHub release and check each
archive against `checksums.txt`. The plugin updates through the marketplace, which reads the
`version` in `plugins/cavalry/.claude-plugin/plugin.json`: bump it on every release, or users
never see the update.
