# macOS signing and distribution

macOS release archives keep the existing names `cav_darwin_arm64.tar.gz` and
`cav_darwin_amd64.tar.gz`. They now contain:

```text
cav-cli-<version>/Cav.app/
  Contents/Info.plist
  Contents/MacOS/cav
  Contents/Resources/{LICENSE,NOTICE}
  Contents/_CodeSignature/...
```

The bundle ID is `dev.sebastiano.cav`. Terminal usage invokes
`Cav.app/Contents/MacOS/cav` directly; no GUI launcher or JVM is included.

## Release path

`tools/release.sh v<version>` requires a macOS signing host and signing/notary configuration
before touching `dist`. `tools/package-macos.sh` Developer ID Application signs the binary
and outer app with Hardened Runtime and secure timestamps, with no JVM/JIT entitlements.
It submits a ditto app archive using App Store Connect API-key authentication, requires
Apple's `Accepted` status, staples the app, and archives the whole tree with native tar.
`tools/verify-macos-bundle.sh` extracts the final distribution without repairing modes,
checks executable permissions, both release signatures/runtime/timestamps, the stapled
ticket and Gatekeeper acceptance. Any failure fails the release. Checksums are computed
only after final notarization, stapling and archive verification.

Development is credential-free: normal `go build` and `tools/release.sh v<current-version>
--dev` need no Apple secrets. Development packaging does not invoke Apple signing tools;
the Go linker may produce its normal local signature. These artifacts have no release
acceptance claim and the macOS installer refuses them. There is no silent release fallback
to ad-hoc signing, no quarantine removal and no private-credential discovery.

## CI secrets

Provision the following repository secrets outside the agent workflow:

| Secret | Purpose |
|---|---|
| `APPLE_DEVELOPER_ID_P12` | Base64 Developer ID Application certificate export. |
| `APPLE_DEVELOPER_ID_P12_PASSWORD` | Password for that export. |
| `APPLE_SIGNING_KEYCHAIN_PASSWORD` | Password for the temporary CI keychain. |
| `APPLE_DEVELOPER_IDENTITY` | Exact Developer ID Application codesign identity. |
| `APPLE_NOTARY_API_KEY` | Base64 App Store Connect .p8 API key. |
| `APPLE_NOTARY_API_KEY_ID` | API key ID. |
| `APPLE_NOTARY_API_ISSUER` | API issuer UUID. |

The reusable `macos-release-artifacts.yml` validates a semver version and resolves a commit
reachable from the default branch before a privileged macOS job checks out that exact SHA.
`tools/ci-macos-release.sh` imports the certificate into a temporary keychain and writes the
API key under the ephemeral runner's temporary folder. The script trap and an always-run
workflow fallback remove the certificate, key and temporary keychain. Do not enable shell
tracing or print credential files. This CI path is restricted to ephemeral GitHub runners.

For a pre-tag smoke, dispatch `notarize-macos.yml` with a trusted default-branch commit and
its manifest version. It calls the same release implementation and uploads archives and
checksums for seven days. It never tags, publishes or deploys. Archives protect bundle
permissions from upload-artifact's mode normalization. Before publishing separately, run
`tools/verify-macos-bundle.sh` on each downloaded macOS archive and test terminal invocation
on matching hardware. Foreign-architecture executables are inspected rather than run in CI.

Without provisioned credentials, real Developer ID signing, Apple's notary acceptance,
stapled-ticket validation and Gatekeeper acceptance remain externally blocked. Mocked
packaging tests verify orchestration and layout only; they are not Apple acceptance.

## Install and update compatibility

The installer verifies the downloaded checksum, signature, stapled ticket and Gatekeeper
acceptance, copies the full app with ditto into a unique `CAV_BIN_DIR/.cav-bundles/install.*`
folder, verifies that copy and atomically switches `CAV_BIN_DIR/cav` to a symlink pointing
at its executable. `cav update` validates archive paths, uses native tar to retain bundle
metadata, verifies the full extracted app, and switches the PATH link. Neither extracts
just the executable. Old bundles are retained for manual rollback/cleanup. Linux and
Windows keep their existing archive layout and replacement behavior.

Historical bare macOS installs can migrate by running the new installer or new cav update.
Already-shipped old updater binaries extract only an executable, so use the **current
installer once** when moving from those versions. A manually installed Cav.app outside the
managed layout uses the installer with `CAV_BIN_DIR` to establish a PATH installation.
New self-update refuses bare-binary downgrades. The installer permits a historical bare
release only with an explicitly pinned `CAV_VERSION` and `CAV_ALLOW_LEGACY_MACOS=1`, and
refuses to install it through a bundle symlink. Use a separate directory for that opt-in.
