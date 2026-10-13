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

A release needs no Apple credentials on your machine. GitHub Actions builds and notarizes
the archives with the repository secrets. Only the maintainer, or someone the maintainer
asks, publishes a release. The examples use version `1.4.0`.

### 1. Merge a release pull request

On a branch from `main`:

1. Set the version in every file: `tools/check-versions.sh --set 1.4.0`.
2. In `CHANGELOG.md`, rename `## Unreleased` to `## 1.4.0`.
3. Check the versions and run the local gate:

   ```bash
   tools/check-versions.sh 1.4.0
   gofmt -l . && go vet ./... && go test ./...
   ```

4. Commit as `Release 1.4.0`, open a pull request, and merge it when CI is green. Note the
   full SHA of the merge commit on `main`: `git rev-parse origin/main`.

### 2. Build the signed archives in CI

Start the notarize workflow on that commit. Pass the full 40-character SHA from
`git rev-parse`. Do not type or shorten it: a wrong SHA fails with `not our ref`.

```bash
gh workflow run notarize-macos.yml -f version=1.4.0 -f ref=<full SHA>
gh run watch <run id> --exit-status
```

The workflow runs `tools/ci-macos-release.sh`, which runs `tools/release.sh`. It uploads
`dist/` as the `release-artifacts` artifact for seven days:

- one archive per platform: `cav_<os>_<arch>.tar.gz` or `.zip`;
- `checksums.txt`;
- `install.sh` and `install.ps1`.

The macOS archives hold the full signed, notarized and stapled `Cav.app`. The workflow
never tags or publishes.

On a macOS host that has the Apple configuration, `tools/release.sh v1.4.0` builds the same
`dist/` locally. Without credentials it fails before it changes anything. For test builds
without credentials, use `tools/release.sh v<current-version> --dev`. Never publish a
`--dev` build.

### 3. Check the archives

Download the artifact into an empty folder and check it:

```bash
gh run download <run id> -n release-artifacts -D <empty folder>
(cd <empty folder> && shasum -a 256 -c checksums.txt)
tools/verify-macos-bundle.sh <empty folder>/cav_darwin_arm64.tar.gz 1.4.0
tools/verify-macos-bundle.sh <empty folder>/cav_darwin_amd64.tar.gz 1.4.0
```

`verify-macos-bundle.sh` must end with `accepted` and `source=Notarized Developer ID` for
both macOS archives. Also extract the archive for your machine and run `cav version`. It
must print the new version.

### 4. Run the live tests in Cavalry

Use a cav built from the release commit, with Cavalry open and the bridge running
(`cav doctor` says `ready`). `cav scene new --force` replaces the open scene, so first
check with `cav scene info` that it holds nothing you need.

```bash
cav setup                                   # installs the new bridge script
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3
cav run assets/helpers/test/live.js         # expect "fail": 0
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3
cav run assets/helpers/test/live-native.js  # expect "fail": 0
```

`live.js` warns once that the font "Definitely Not A Font" is not installed. That warning
is expected: a check uses a missing font on purpose.

Then run the command smoke with the same cav. It runs every command once in a new scratch
scene and a temporary folder, and fails when `cav help` lists a command it does not run:

```bash
tools/release-smoke.sh <path to cav>        # expect "0 failed"
```

It starts its scene without `--force`, so save or close the open scene first. Install
potrace (`brew install potrace`) so it also covers `cav gen vector`; without a tracer that
step is skipped. It reads no
key (`CAV_NO_KEYS=1`) and uses only free services, so it costs nothing and never asks
1Password. A `warn` line is a step that needs the internet. With the maintainer at the
keyboard, `--keyed` adds one call to each keyed service after it lists them and asks.

### 5. Tag and publish

Write the release notes from the version's section of `CHANGELOG.md`, without the heading.
Then tag the merge commit and publish the archives from step 3:

```bash
git tag v1.4.0 <full SHA> && git push origin v1.4.0
gh release create v1.4.0 --title v1.4.0 --notes-file <notes file> --verify-tag \
  cav_darwin_amd64.tar.gz cav_darwin_arm64.tar.gz cav_linux_amd64.tar.gz \
  cav_windows_amd64.zip cav_windows_arm64.zip checksums.txt install.ps1 install.sh
```

When the release is published, the `codex-plugin-zip.yml` workflow attaches
`cav-codex-plugin-1.4.0.zip` to it. Check that the release then lists the zip.

### 6. Plugin marketplaces

The Claude Code plugin updates by itself from the version in
`plugins/cavalry/.claude-plugin/plugin.json`.

For the OpenAI plugin marketplace, download `cav-codex-plugin-1.4.0.zip` from the release
page and upload it. Do not upload while an earlier version is still waiting for review:
a new upload loses its place in the review queue. Upload the newest version when that
review is done.

To build the zip locally, `tools/package-codex-plugin.sh v1.4.0` zips the plugin folder at
that tag into `build/cav-codex-plugin-1.4.0.zip`. The workflow and the script both fail
when the tag and the plugin version differ.

### 7. Update your own install

Run `cav update`. An install that `cav update` cannot handle can use the release's
installer instead: `CAV_VERSION=v1.4.0 sh install.sh`. An example is a bare executable
from an old version or a local build. The installer checks the archive and installs the
full `Cav.app`.

`cav update` and the install scripts download from the latest GitHub release and check each
archive against `checksums.txt`. The plugin updates through the marketplace, which reads the
`version` in `plugins/cavalry/.claude-plugin/plugin.json`: bump it on every release, or users
never see the update.

See [macOS notarization](docs/NOTARIZATION.md) for the seven repository secrets, temporary
keychain/API-key cleanup, final extracted-artifact verification and the artifact-only
`notarize-macos.yml` smoke. The reusable workflow requires a commit reachable from the
default branch and matching manifest version before privileged work. It never publishes.
Checksums follow every signing/stapling/archive mutation. Development archives are not
signed release artifacts. Already-shipped old macOS updater binaries require migration
through the current installer so that the full bundle survives.

The optional bridgeSession field and completed-ID deduplication retain protocol-1 request
and result compatibility. They add capability for new bridges while the operation ledger
also works with older running protocol-1 bridges. No tag or release is created by builds.
