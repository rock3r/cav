# Releasing

The CLI, plugin, bridge script and helper library share one semantic version:
`vMAJOR.MINOR.PATCH`. `tools/check-versions.sh` sets and checks it across files.
A packaging test fails when versions differ.

The bridge protocol has a separate number: `Protocol` in `internal/bridge/client.go`
and `PROTOCOL` in `cav-bridge.js`. Change both when request or result formats change.
A bridge refuses another protocol, and `cav doctor` reports the mismatch as a failure.
An older bridge with the same protocol keeps working; `cav doctor` suggests restarting it.

| What changed | Bump |
|---|---|
| Bug fix, docs, skill wording | PATCH |
| New command, flag, helper or recipe | MINOR |
| Removed or renamed command, flag or helper; bridge protocol change | MAJOR |
| Bridge request or result format | MAJOR, and the protocol number in both places |

## Steps

GitHub Actions uses repository secrets to build and notarize release archives. The local
machine needs no Apple credentials. The maintainer or a person they authorize publishes
the release. These examples use version `1.4.0`.

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

Start the notarize workflow at the merge commit. Pass the full 40-character SHA from
`git rev-parse`. A mistyped or shortened SHA can fail with `not our ref`.

```bash
gh workflow run notarize-macos.yml -f version=1.4.0 -f ref=<full SHA>
gh run watch <run id> --exit-status
```

The workflow runs `tools/ci-macos-release.sh`, then `tools/release.sh`. It uploads
`dist/` as the `release-artifacts` artifact for seven days:

- one archive per platform: `cav_<os>_<arch>.tar.gz` or `.zip`;
- `checksums.txt`;
- `install.sh` and `install.ps1`.

macOS archives contain the full signed, notarized and stapled `Cav.app`. The workflow
does not create a tag or publish a release.

On a configured macOS host, `tools/release.sh v1.4.0` can build the same `dist/` locally.
Without credentials, it stops before making changes. Use
`tools/release.sh v<current-version> --dev` for development archives without credentials.
Do not publish a `--dev` build.

### 3. Check the archives

Download the artifact into an empty folder and check it:

```bash
gh run download <run id> -n release-artifacts -D <empty folder>
(cd <empty folder> && shasum -a 256 -c checksums.txt)
tools/verify-macos-bundle.sh <empty folder>/cav_darwin_arm64.tar.gz 1.4.0
tools/verify-macos-bundle.sh <empty folder>/cav_darwin_amd64.tar.gz 1.4.0
```

For both macOS archives, `verify-macos-bundle.sh` must report `accepted` and
`source=Notarized Developer ID`. Extract the archive for the current machine and run
`cav version`. Confirm that it prints the new version.

### 4. Run the live tests in Cavalry

Use a cav build from the release commit. Open Cavalry and start the bridge; `cav doctor`
must report `ready`. Before `cav scene new --force`, use `cav scene info` to check that
the open scene contains no work that needs preserving.

```bash
cav setup                                   # installs the new bridge script
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3
cav run assets/helpers/test/live.js         # expect "fail": 0
cav scene new --force --width 1280 --height 720 --fps 30 --seconds 3
cav run assets/helpers/test/live-native.js  # expect "fail": 0
```

`live.js` warns once that the font "Definitely Not A Font" is not installed. That warning
is expected: a check uses a missing font on purpose.

Run the command smoke with that cav. It runs commands in a scratch scene and temporary
folder. It fails if `cav help` lists a command that the smoke did not run:

```bash
tools/release-smoke.sh <path to cav>        # expect "0 failed"
```

The smoke starts its scene without `--force`, so save or close the open scene first. Install
potrace (`brew install potrace`) so it also covers `cav gen vector`; without a tracer that
step is skipped. With uv installed it also runs `cav gen cutout` with rembg; its first run
downloads the BiRefNet model (about 1 GB). It reads no
key (`CAV_NO_KEYS=1`) and uses only free services, so it costs nothing and never asks
1Password. A `warn` line is a step that needs the internet. With the maintainer at the
keyboard, `--keyed` adds one call to each keyed service after it lists them and asks.
`--keyed=GROUP[,GROUP]` runs only those keyed groups, without Cavalry, to check one
service again (for example `--keyed=freesound`).

### 5. Tag and publish

Use the version's `CHANGELOG.md` section as the source for release notes. Remove its
heading and shorten the text for users. See [Release notes](#release-notes).
Then tag the merge commit and publish the archives from step 3:

```bash
git tag v1.4.0 <full SHA> && git push origin v1.4.0
gh release create v1.4.0 --title v1.4.0 --notes-file <notes file> --verify-tag \
  cav_darwin_amd64.tar.gz cav_darwin_arm64.tar.gz cav_linux_amd64.tar.gz \
  cav_windows_amd64.zip cav_windows_arm64.zip checksums.txt install.ps1 install.sh
```

After publication, `codex-plugin-zip.yml` attaches
`cav-codex-plugin-1.4.0.zip` to it. Check that the release then lists the zip.

### 6. Plugin marketplaces

The Claude Code plugin reads its update version from
`plugins/cavalry/.claude-plugin/plugin.json`.

For the OpenAI plugin marketplace, download `cav-codex-plugin-1.4.0.zip` from the
release and upload it. Wait for an earlier submission's review to finish first. A new
upload loses its place in the review queue.

`tools/package-codex-plugin.sh v1.4.0` builds the zip locally from the plugin folder
at that tag. It writes `build/cav-codex-plugin-1.4.0.zip`. The workflow and script fail
if the tag and plugin version differ.

### 7. Update your own install

Run `cav update` for a managed install. For an old bare executable or local build,
use the release installer with `CAV_VERSION=v1.4.0 sh install.sh`. The installer checks
the archive and installs the full `Cav.app`.

`cav update` and installers download the latest GitHub release and check archives against
`checksums.txt`. Plugin marketplaces read `version` in
`plugins/cavalry/.claude-plugin/plugin.json`. Update that field for every release so
users receive the new plugin.

See [macOS notarization](docs/NOTARIZATION.md) for the seven secrets, temporary key and
keychain cleanup, extracted-archive checks and the artifact-only workflow. The reusable
workflow checks that the commit is reachable from the default branch and the manifest
version matches before privileged work. It does not publish. Compute checksums after
signing, stapling and archiving. Development archives are not signed release artifacts.
Older shipped macOS updaters need the current installer to migrate to full bundles.

The optional bridgeSession field and completed-ID deduplication keep protocol-1 request
and result compatibility. New bridges gain those features. The operation ledger also
works with older running protocol-1 bridges. Builds do not create tags or releases.

## Release notes

Write release notes for cav users.

- Start with one line about what users can do with the release.
- Then add short `## What's new` and `## Fixes` sections. Leave out a section that is empty.
  A small patch release can be one line and two or three bullets.
- State a new user action or fixed problem in each bullet. Leave implementation detail
  in the changelog.
- Use short sentences and common English words. Avoid idioms and slang.
- Keep detailed changes in `CHANGELOG.md`.
- Keep upgrade steps (for example `cav setup`, reopen **Scripts > cav-bridge**) when the
  user must do something.
- Add images for visual changes when they help users understand the release. Use output
  from real runs and upload the images as release assets.
- Never put spaces around a slash. Write `a/b`; reject `a / b`.
- Describe shipped behavior. Leave out history, plans and unrelated scope notes.
