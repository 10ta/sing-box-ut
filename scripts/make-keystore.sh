#!/usr/bin/env bash
# Create the keystore that signs the Android APK, once. Needs only openssl.
# Writes three files to a directory outside the repository and prints what to
# put into the repository secrets. Keep the directory: losing the keystore
# means later APKs can no longer update installed ones.
#
# usage: scripts/make-keystore.sh [OUT_DIR]     (default: ~/sing-box-ut-keystore)
set -euo pipefail
out=${1:-$HOME/sing-box-ut-keystore}
[ ! -e "$out/release.p12" ] || { echo "make-keystore: $out/release.p12 already exists, not overwriting" >&2; exit 1; }
umask 077
mkdir -p "$out"
pass=$(openssl rand -hex 16)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/key.pem" -out "$tmp/cert.pem" \
  -days 36500 -subj "/CN=sing-box-ut" 2>/dev/null
openssl pkcs12 -export -inkey "$tmp/key.pem" -in "$tmp/cert.pem" -name sing-box-ut \
  -out "$out/release.p12" -passout "pass:$pass"
base64 -w0 "$out/release.p12" > "$out/ANDROID_KEYSTORE_BASE64.txt"
printf '%s' "$pass" > "$out/ANDROID_KEYSTORE_PASS.txt"
cat <<DONE
Keystore written to $out

Repository secrets (Settings -> Secrets and variables -> Actions -> New repository secret):
  ANDROID_KEYSTORE_BASE64   contents of $out/ANDROID_KEYSTORE_BASE64.txt
  ANDROID_KEYSTORE_PASS     contents of $out/ANDROID_KEYSTORE_PASS.txt
DONE
