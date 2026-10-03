#!/bin/bash
# CI only: ephemeral keychain and API-key authentication, never credential output.
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true ]] || { echo 'run this credential-import path only on an ephemeral GitHub macOS runner' >&2; exit 69; }
: "${RUNNER_TEMP:?}"
: "${RELEASE_VERSION:?}"
: "${APPLE_DEVELOPER_ID_P12:?}"
: "${APPLE_DEVELOPER_ID_P12_PASSWORD:?}"
: "${APPLE_SIGNING_KEYCHAIN_PASSWORD:?}"
: "${APPLE_NOTARY_API_KEY:?}"
: "${APPLE_NOTARY_API_KEY_ID:?}"
: "${APPLE_NOTARY_API_ISSUER:?}"
: "${APPLE_DEVELOPER_IDENTITY:?}"
umask 077
certificate="$RUNNER_TEMP/cav-developer-id.p12"
keychain="$RUNNER_TEMP/cav-signing.keychain-db"
key="$RUNNER_TEMP/cav-notary.p8"
cleanup(){
 security delete-keychain "$keychain" >/dev/null 2>&1 || true
 rm -f "$certificate" "$key"
}
trap cleanup EXIT INT TERM
printf '%s' "$APPLE_DEVELOPER_ID_P12" | base64 --decode > "$certificate"
printf '%s' "$APPLE_NOTARY_API_KEY" | base64 --decode > "$key"
security create-keychain -p "$APPLE_SIGNING_KEYCHAIN_PASSWORD" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$APPLE_SIGNING_KEYCHAIN_PASSWORD" "$keychain"
# Trust the signing tool explicitly so headless runners never wait for a key-access prompt.
security import "$certificate" -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -T /usr/bin/codesign -t cert -f pkcs12 -k "$keychain" >/dev/null
security list-keychains -d user -s "$keychain"
security default-keychain -d user -s "$keychain"
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$APPLE_SIGNING_KEYCHAIN_PASSWORD" "$keychain" >/dev/null
APPLE_NOTARY_API_KEY_PATH="$key" tools/release.sh "v$RELEASE_VERSION"
