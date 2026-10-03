#!/bin/bash
# Package a credential-free development app or a mandatory signed release app.
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 <binary> <version> <archive.tar.gz> dev|release" >&2; exit 64; }
binary="$1"; version="$2"; archive="$3"; mode="$4"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || exit 64
[[ "$mode" == dev || "$mode" == release ]] || exit 64
[[ "$archive" = /* ]] || archive="$PWD/$archive"
workspace="$(mktemp -d)"
trap 'rm -rf "$workspace"' EXIT
root="$workspace/cav-cli-$version"
app="$root/Cav.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$binary" "$app/Contents/MacOS/cav"
chmod 755 "$app/Contents/MacOS/cav"
cp LICENSE NOTICE "$app/Contents/Resources/"
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.sebastiano.cav</string>
<key>CFBundleName</key><string>Cav</string>
<key>CFBundleExecutable</key><string>cav</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>$version</string>
<key>LSBackgroundOnly</key><true/>
</dict></plist>
EOF
if [[ "$mode" == release ]]; then
 [[ "$(uname -s)" == Darwin ]] || { echo 'macOS releases require a macOS signing host' >&2; exit 69; }
 : "${APPLE_DEVELOPER_IDENTITY:?release signing identity required}"
 : "${APPLE_NOTARY_API_KEY_PATH:?notary key path required}"
 : "${APPLE_NOTARY_API_KEY_ID:?notary key id required}"
 : "${APPLE_NOTARY_API_ISSUER:?notary issuer required}"
 [[ "$APPLE_DEVELOPER_IDENTITY" == 'Developer ID Application: '* ]] || { echo 'Developer ID Application identity required' >&2; exit 1; }
 # No JVM/JIT entitlements: this app holds only one native Go executable.
 codesign --force --options runtime --timestamp --sign "$APPLE_DEVELOPER_IDENTITY" "$app/Contents/MacOS/cav"
 codesign --force --options runtime --timestamp --sign "$APPLE_DEVELOPER_IDENTITY" "$app"
 codesign --verify --deep --strict --verbose=4 "$app"
 ditto -c -k --sequesterRsrc --keepParent "$app" "$workspace/notary.zip"
 xcrun notarytool submit "$workspace/notary.zip" --key "$APPLE_NOTARY_API_KEY_PATH" --key-id "$APPLE_NOTARY_API_KEY_ID" --issuer "$APPLE_NOTARY_API_ISSUER" --timeout 30m --wait --output-format json > "$workspace/notary.json"
 python3 - "$workspace/notary.json" <<'CHECK'
import json,sys
r=json.load(open(sys.argv[1]))
if r.get('status')!='Accepted':
    print('Notarization was not accepted. Submission id: '+str(r.get('id')),file=sys.stderr)
    sys.exit(1)
CHECK
 xcrun stapler staple "$app"
 xcrun stapler validate "$app"
 codesign --verify --deep --strict --verbose=4 "$app"
fi
# Native macOS tar preserves the complete app, including ticket/resource metadata.
tar -czf "$archive" -C "$workspace" "cav-cli-$version"
if [[ "$mode" == release ]]; then tools/verify-macos-bundle.sh "$archive" "$version"; fi
