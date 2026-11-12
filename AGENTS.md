# AGENTS.md

## Project overview

- sing-box-ut: patches that give sing-box a native VLESS + XHTTP client with split upload/download servers (Xray `downloadSettings`), so clients can drop xray-core. Human docs: [README.md](README.md). The user works in Chinese; the full decision log is [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md).
- Type: fork/customization kept as patches, not a fork. Base: Leadaxe/sing-box-lx branch `lx-1.15` (verified at `20723b7`, which merges SagerNet/sing-box `v1.15.0-alpha.13`).
- Stack: Go 1.26 (lx pins the exact version in `go.version`), bash + python3 for the interop test, GitHub Actions.
- End goal, not built yet: on every new sing-box alpha, Actions builds a Linux amd64 `.deb` and an Android arm64-v8a APK from upstream sing-box + files copied from lx + patches, and notifies on failure.

## Commands

Run from the repository root unless noted. `T` stands for the tag list `with_quic,with_utls,with_clash_api,with_xhttp,badlinkname,tfogo_checklinkname0`.

| Task | Command |
|---|---|
| Get a patched tree | `git clone --depth 1 -b lx-1.15 --recurse-submodules --shallow-submodules https://github.com/Leadaxe/sing-box-lx lx` then `git -C lx apply ../patches/0002-lx-xhttp-download.patch` |
| Format check | in `lx/`: `gofmt -l option/v2ray_xhttp*.go transport/v2rayxhttp/` (no output) |
| Vet | in `lx/`: `go vet -tags "$T" ./option/ ./transport/v2rayxhttp/` |
| Test | in `lx/`: `go test -count=1 -race -tags "$T" ./option/ ./transport/v2rayxhttp/` |
| Single test | in `lx/`: `go test -count=1 -tags "$T" ./transport/v2rayxhttp/ -run TestDownloadSplit -v` |
| Build | in `lx/`: `CGO_ENABLED=0 go build -trimpath -tags "$T" -ldflags "-s -w -checklinkname=0" -o ../sing-box ./cmd/sing-box` |
| Interop | `test/interop/run.sh ./sing-box xray-bin/xray` (ends with `PASS`; one case: add `stream-up:full`) |
| Regenerate the patch | in `lx/`: `git add -A && git diff --cached > ../patches/0002-lx-xhttp-download.patch` |
| CI | push, or Actions → `verify-download` → Run workflow |

## Repository map

- `patches/0002-lx-xhttp-download.patch` — the only code change. Applies to an lx tree, and equally to an upstream sing-box tree that has the lx files copied in (same paths).
- `test/interop/run.sh` — starts Xray, the tap and sing-box per case, fetches 5 MB through the proxy. Cases are `MODE:SCENARIO`.
- `test/interop/gen.py` — writes both configs. `plain` = cleartext HTTP/1.1; `full` = TLS 1.3 + H2 + uTLS + VLESS encryption + X-Padding obfs + XMUX; `h3` = `full` over HTTP/3.
- `test/interop/tcp_tap.py` — the "download server": forwards 6502 → 6501 (TCP and UDP) and logs the first line of each TCP connection, `QUIC` per UDP client.
- `.github/workflows/verify-download.yml` — apply, gofmt, vet, test, build, interop.
- `examples/` — complete sanitized configs (sing-box client, Xray server, Caddyfile). Keep them in sync with the complete example in both READMEs; validate with `sing-box check` and `xray run -test` after replacing placeholders.
- To change download behaviour, start in `transport/v2rayxhttp/download_ut.go` inside the patched lx tree.

## Conventions

- Lines added to existing lx files carry a `// ut:` comment. New code goes in `*_ut.go` / `*_ut_test.go` files.
- Do not reformat or realign existing lx lines; every touched lx line is a future conflict.
- Commits: Conventional Commits in English. Author and committer `10ta <84114181+10ta@users.noreply.github.com>`.
- Shell commands given to the user must run in fish as root (no bash-only syntax, no sudo).

## Architecture & invariants

- The download endpoint is a second `*Client` built by `NewClient` from `download` options; only its `newRequest`, xmux pool and `Close` are used.
- Nothing is inherited from the upload side except the dialer and the session id (same as Xray).
- Mode with `download`: `stream-one` is an error; `auto` → `packet-up`, or `stream-up` under REALITY. Resolved before the placement checks.
- `xmuxRelease.next` chains the download connection's release; every release path must stay idempotent and release both.
- Download body failures are charged to the download connection's breaker; the first successful download read credits both.

## Do not

- Do not copy `common/lxtrace/*_lx.go` or `protocol/vless/encryption/lx_vision*` into an upstream tree: they need lx-only symbols.
- Do not carry lx's `DialedThroughDetour: tls.DialedThroughDetour(...)` line into `protocol/vless/outbound.go` on upstream: the function does not exist there.
- Do not switch the interop test to the latest Xray release without running it: server defaults change between releases.
- Do not commit scratch `go.mod` edits (mirror replaces, local paths) or build outputs (`lx/`, `sing-box`, `xray-bin/`, `xray.zip`).
- `patches/0001-*` is reserved for the seams patch (upstream files), still to be added.

## Config & secrets

- No secrets. The interop test uses a fixed dummy UUID and generates VLESS encryption keys and a TLS certificate at run time.
- User-facing options: [README → Configuration](README.md#-configuration).

## Deployment & environments

- Nothing is deployed or released yet. CI is the `verify-download` workflow on GitHub-hosted `ubuntu-latest`.
- Pinned inputs: lx branch `lx-1.15` (moving), Xray `v26.11.9` (fixed).

## Before you finish

- [ ] gofmt, vet and tests pass in the patched lx tree.
- [ ] `test/interop/run.sh` ends with `PASS`.
- [ ] The patch applies to a fresh lx clone: `git -C lx apply --check ../patches/0002-lx-xhttp-download.patch`.
- [ ] Docs synced: if commands, config keys or behaviour changed, update `README.md`, `README.zh-CN.md` and this file together.

## Gotchas

- lx's `go.mod` replaces `sing-tun`, `wireguard-go` and `utls` with git submodules; a clone without submodules does not build.
- Xray 26.11.9: `freedom` blocks loopback targets unless `finalRules` allows them; the server's default `xPaddingBytes` is 100-1000 and a client range outside it gets HTTP 400.
- No Vision: Xray rejects a flow-less client when its account, or a `flow` at the inbound `settings` level, asks for one. The server needs a separate flow-less client entry.
- `finalmask.quicParams` does apply to XHTTP over HTTP/3 in Xray; it is unsupported here because lx hard-codes `quic.Config`, not because QUIC is unused.
- `curl --socks5-hostname` skips the proxy when `NO_PROXY` lists `127.0.0.1`; the test passes `--noproxy ''`.
- Where proxy.golang.org is unreachable, see "踩过的坑" in [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md) for a GitHub-only way to build.
- The route "upstream `v1.15.0-alpha.13` + copied lx files + seams + this patch" was built and passed the interop test on 2026-11-11. Copy list and seams: "已有产出" and "2026-11-11 更新" in the notes.

## Current focus

- 2026-11-12: the download patch is verified locally, including HTTP/3. The user's earlier `init.sh` and seams patch are lost; rebuild them. Decided: keep the route "upstream alpha tag + copied lx files + seams + patches"; no Vision; GPL-3.0-or-later.
- Next, all in GitHub Actions: Linux `.deb` (amd64, arm64) and Windows `.exe` (amd64, arm64), then the Android arm64-v8a APK, then scheduled upstream checks, prereleases and Telegram failure notices.
- Not yet tested: Caddy in front of Xray (the user's real topology), a CDN.
