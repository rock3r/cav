# Releasing

One version number covers the CLI and the plugin: `vMAJOR.MINOR.PATCH` (semantic versioning).
The bridge script and the helper library carry their own version strings, because Cavalry
reloads them separately; bump them when their files change.

| What changed | Bump |
|---|---|
| Bug fix, docs, skill wording | PATCH |
| New command, flag, helper or recipe | MINOR |
| Removed or renamed command, flag or helper; bridge protocol change | MAJOR (MINOR while below 1.0) |
| `assets/bridge/cav-bridge.js` | also `BRIDGE_VERSION` in that file |
| `assets/helpers/cav-helpers.js` | also `cav.VERSION` in that file |

## Steps

1. Set the plugin version everywhere: `tools/check-versions.sh --set 0.2.0`.
2. Commit: `Release 0.2.0`.
3. Build: `tools/release.sh v0.2.0`. It runs the tests, then writes `dist/` with one archive per
   platform (`cav_<os>_<arch>.tar.gz` or `.zip`), `checksums.txt` and the install scripts.
4. Run the live helper tests in a throwaway Cavalry scene (see README, Development).
5. **Only with the maintainer's approval**: tag and publish.

   ```bash
   git tag v0.2.0 && git push origin main v0.2.0
   gh release create v0.2.0 dist/* --title v0.2.0 --notes-file <notes.md>
   ```

`cav update` and the install scripts download from the latest GitHub release and check each
archive against `checksums.txt`. The plugin updates through the marketplace, which reads the
`version` in `plugins/cavalry/.claude-plugin/plugin.json`: bump it on every release, or users
never see the update.
