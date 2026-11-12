#!/usr/bin/env bash
# Interop: sing-box (XHTTP + download) against a real Xray server.
#   upload   -> 127.0.0.1:6501 (Xray inbound)
#   download -> 127.0.0.1:6502 (tcp_tap.py -> 6501)
# A case passes when a 5 MB file fetched through the proxy is intact AND the
# tap saw the download request. In the cleartext scenario the tap can read the
# request line, so it also asserts that only GETs took the download route.
# usage: run.sh SING_BOX_BIN XRAY_BIN [MODE:SCENARIO...]
#   default cases: both modes x plain, full, h3
#   scenarios are described in gen.py
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
singbox=$(realpath "$1"); xray=$(realpath "$2"); shift 2
cases=("$@"); [ ${#cases[@]} -eq 0 ] && cases=(stream-up:plain packet-up:plain stream-up:full packet-up:full stream-up:h3 packet-up:h3)
work=$(mktemp -d); pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; rm -rf "$work"; }
trap cleanup EXIT

stop() { kill "$1" 2>/dev/null || true; wait "$1" 2>/dev/null || true; }
wait_port() { # PORT: up to 10 s for a listener
  for _ in $(seq 100); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then return 0; fi
    sleep 0.1
  done
  echo "FAIL: nothing listening on 127.0.0.1:$1"; return 1
}
fail() {
  echo "FAIL: $*"
  echo "--- sing-box"; tail -n 60 "$work/client.log" || true
  echo "--- xray"; tail -n 40 "$work/xray.log" || true
  echo "--- tap"; cat "$work/tap.log" || true
  exit 1
}

head -c 5000000 /dev/urandom > "$work/blob"
want=$(sha256sum < "$work/blob")
(cd "$work" && exec python3 -m http.server 8000 --bind 127.0.0.1) >"$work/web.log" 2>&1 & pids+=($!)
python3 "$here/tcp_tap.py" 6502 6501 "$work/tap.log" & pids+=($!)
wait_port 8000; wait_port 6502

for c in "${cases[@]}"; do
  mode=${c%%:*}; scenario=${c##*:}
  echo "== $mode / $scenario"
  python3 "$here/gen.py" "$work" "$mode" "$scenario" "$xray"
  : > "$work/tap.log"
  "$xray" run -c "$work/server.json" >"$work/xray.log" 2>&1 & server=$!; pids+=($server)
  "$singbox" run -c "$work/client.json" >"$work/client.log" 2>&1 & client=$!; pids+=($client)
  if [ "$scenario" = h3 ]; then sleep 1; else wait_port 6501 || fail "xray did not start"; fi  # h3: UDP only
  wait_port 1080 || fail "sing-box did not start"
  # --noproxy '': a NO_PROXY that lists 127.0.0.1 would make curl skip the SOCKS proxy.
  got=$(curl -sS --max-time 60 --noproxy '' --socks5-hostname 127.0.0.1:1080 http://127.0.0.1:8000/blob | sha256sum) || fail "curl"
  [ "$got" = "$want" ] || fail "payload mismatch"
  [ -s "$work/tap.log" ] || fail "nothing took the download route"
  if [ "$scenario" = plain ]; then
    grep -q '^GET /ut/' "$work/tap.log" || fail "no download GET on the download route"
    if grep -qv '^GET ' "$work/tap.log"; then fail "an upload request took the download route"; fi
  fi
  if [ "$scenario" = h3 ]; then grep -q '^QUIC$' "$work/tap.log" || fail "download did not use QUIC"; fi
  echo "ok: payload intact, $(wc -l < "$work/tap.log") connection(s) via the download server"
  stop "$client"; stop "$server"
done
echo "PASS"
