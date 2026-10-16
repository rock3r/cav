# macOS signing and distribution

macOS release archives use the names `cav_darwin_arm64.tar.gz` and
`cav_darwin_amd64.tar.gz`. Each contains this app bundle:

```text
cav-cli-<version>/Cav.app/
  Contents/Info.plist
  Contents/MacOS/cav
  Contents/Resources/{LICENSE,NOTICE}
  Contents/_CodeSignature/...
```

The bundle ID is `dev.sebastiano.cav`. Terminal users run
`Cav.app/Contents/MacOS/cav` directly. The bundle has no GUI launcher or JVM.

## Release path

`tools/release.sh v<version>` requires a macOS signing host and notary configuration
before it changes `dist`. `tools/package-macos.sh` signs the binary and outer app with
a Developer ID Application certificate, Hardened Runtime and secure timestamps. It
adds no JVM/JIT entitlements. It submits a ditto app archive with App Store Connect
API-key authentication. The script requires Apple's `Accepted` status, staples the app
and archives the full tree with native tar. `tools/verify-macos-bundle.sh` extracts the
final archive without changing file modes. It checks executable permissions, both
signatures, runtime settings, timestamps, the stapled ticket and Gatekeeper acceptance.
Any failed check stops the release. Checksums follow notarization, stapling and verification.

Normal `go build` and `tools/release.sh v<current-version> --dev` require no Apple
credentials. Development packaging does not call Apple signing tools, although the Go
linker can add a local signature. These builds have no release acceptance claim, and
the macOS installer refuses them. Release packaging never falls back to ad-hoc signing,
removes quarantine or searches for private credentials.

## CI secrets

The maintainer provisions these repository secrets outside the agent workflow:

| Secret | Purpose |
|---|---|
| `APPLE_DEVELOPER_ID_P12` | Base64 Developer ID Application certificate export. |
| `APPLE_DEVELOPER_ID_P12_PASSWORD` | Password for that export. |
| `APPLE_SIGNING_KEYCHAIN_PASSWORD` | Password for the temporary CI keychain. |
| `APPLE_DEVELOPER_IDENTITY` | Exact Developer ID Application codesign identity. |
| `APPLE_NOTARY_API_KEY` | Base64 App Store Connect .p8 API key. |
| `APPLE_NOTARY_API_KEY_ID` | API key ID. |
| `APPLE_NOTARY_API_ISSUER` | API issuer UUID. |

The reusable `macos-release-artifacts.yml` validates the version and checks that the
commit is reachable from the default branch. A privileged macOS job checks out that
exact SHA. `tools/ci-macos-release.sh` imports the certificate into a temporary keychain
and writes the API key in the ephemeral runner's temporary folder. A script trap and
an always-run workflow step remove the certificate, key and keychain. Keep shell tracing
off and do not print credential files. Use this CI path only on ephemeral GitHub runners.

For a pre-tag check, run `notarize-macos.yml` with a trusted commit on the default branch
and its manifest version. It uses the release code and keeps archives and checksums for
seven days. It does not tag, publish or deploy. The archives preserve bundle permissions
through artifact upload. Before publishing, run `tools/verify-macos-bundle.sh` on each
downloaded macOS archive. Test terminal execution on matching hardware. CI inspects
executables for other architectures without running them.

Without the required credentials, the release cannot obtain Developer ID signatures,
Apple notary acceptance, a validated staple or Gatekeeper acceptance. Mocked packaging
tests check orchestration and layout only. They do not establish Apple acceptance.

## Install and update compatibility

The installer checks the download checksum, signature, stapled ticket and Gatekeeper
acceptance. It copies the full app with ditto into a unique
`CAV_BIN_DIR/.cav-bundles/install.*` folder, verifies the copy and switches
`CAV_BIN_DIR/cav` atomically to its executable. `cav update` checks archive paths,
extracts with native tar to keep bundle metadata, verifies the app and switches the
PATH link. Both keep the full bundle. Old bundles remain for manual rollback or cleanup.
Linux and Windows retain their archive layouts and replacement behavior.

Bare macOS installs can migrate with the current installer or updater. Older shipped
updaters extract only an executable, so users of those versions must run the **current
installer once**. For a Cav.app outside the managed layout, set `CAV_BIN_DIR` when running
the installer to create a PATH entry. Current self-update refuses bare-binary downgrades.
Installing a historical bare release requires a pinned `CAV_VERSION` and
`CAV_ALLOW_LEGACY_MACOS=1`. The installer refuses that release through a bundle symlink.
Use a separate directory for this explicit opt-in.
