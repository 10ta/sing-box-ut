#!/usr/bin/env bash
# Check that a build can start a naive outbound, i.e. that it was compiled with
# it and can load libcronet.so. No naive server is involved.
# usage: test/naive-start.sh SING_BOX_BIN LIBCRONET_SO
set -euo pipefail
singbox=$(realpath "$1"); lib=$(realpath "$2")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cat > "$work/config.json" <<'JSON'
{
  "log": { "level": "info" },
  "outbounds": [
    {
      "type": "naive",
      "server": "127.0.0.1",
      "server_port": 9,
      "username": "user",
      "password": "pass",
      "tls": { "enabled": true, "server_name": "example.com" }
    }
  ]
}
JSON
# The library is looked up next to the binary, in LD_LIBRARY_PATH, then /usr/lib.
LD_LIBRARY_PATH=$(dirname "$lib") timeout 5 "$singbox" run -c "$work/config.json" >"$work/log" 2>&1 || true
if grep -q "NaiveProxy started" "$work/log"; then
  echo "ok: $(grep -o 'NaiveProxy started.*' "$work/log")"
else
  echo "FAIL: naive outbound did not start"; cat "$work/log"; exit 1
fi
