# AGENTS.md

## Project overview

- sing-box-ut: builds the newest sing-box tag with a native VLESS + XHTTP client and split upload/download servers (Xray `downloadSettings`), so clients can drop xray-core. Human docs: [README.md](README.md). The user works in Chinese; the full decision log is [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md).
- Type: fork/customization kept as patches, not a fork. Each build is: upstream SagerNet/sing-box at a tag + files copied from Leadaxe/sing-box-lx at the commit in `lx.ref` + `overlay/` + `patches/*.patch`.
- Stack: Go (the version upstream's workflow installs, 1.26.10 for `v1.15.0-alpha.13`), bash, python3 for the interop test, GitHub Actions.
- Outputs: Linux `.deb` (amd64, arm64), Windows `.exe` (amd64, arm64), Android arm64-v8a APK, published as a prerelease `<upstream tag>-ut`.

## Commands

Run from the repository root.

| Task | Command |
|---|---|
| Assemble the source | `scripts/prepare.sh work` (newest tag; `UPSTREAM_REF=v…` and `LX_REF=…` override) |
| Build | `scripts/build.sh work sing-box` (cross: set `GOOS`, `GOARCH`) |
| Vet | in `work/`: `go vet -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS),with_xhttp" ./option/ ./common/badh2/ ./common/lxtrace/ ./common/tls/ ./transport/v2ray/ ./transport/v2rayxhttp/ ./protocol/vless/...` |
| Test | same packages with `go test -count=1 -race -tags …` |
| Single test | in `work/`: `go test -count=1 -tags "with_quic,with_utls,with_xhttp,badlinkname,tfogo_checklinkname0" ./transport/v2rayxhttp/ -run TestDownloadSplit -v` |
| Interop | `test/interop/run.sh ./sing-box xray-bin/xray` (ends with `PASS`; one case: add `stream-up:h3`) |
| Pack a .deb | `scripts/package-deb.sh work sing-box amd64 1.15.0-alpha.13 out.deb` |
| Regenerate the last patch | edit files in `work/`, then `git -C work add -N . && git -C work diff > patches/0002-lx-xhttp-download.patch` |
| CI | push, or Actions → `build` → Run workflow |

## Repository map

- `lx.ref` — full SHA of the sing-box-lx commit files are copied from. `lx-files.txt` — which files (globs, `!` excludes).
- `patches/0001-upstream-seams.patch` — five upstream files: `constant/v2ray.go`, `option/v2ray_transport.go`, `option/vless.go`, `transport/v2ray/transport.go`, `protocol/vless/outbound.go`.
- `patches/0002-lx-xhttp-download.patch` — the download endpoint, on the copied lx files (`transport/v2rayxhttp/`, `option/v2ray_xhttp*.go`). To change download behaviour, start in `transport/v2rayxhttp/download_ut.go`.
- `overlay/cmd/internal/build_libbox/ut_tags.go` — appends `with_xhttp` to libbox's tags.
- `scripts/prepare.sh`, `build.sh`, `package-deb.sh`, `go-version.sh` — used by the workflow and by hand; keep them the single source of build logic.
- `test/interop/` — `run.sh` (cases `MODE:SCENARIO`), `gen.py` (configs: `plain`, `full`, `h3`), `tcp_tap.py` (the "download server", TCP and UDP relay with a log).
- `examples/` — complete sanitized configs. Keep the client in sync with the complete example in both READMEs; validate with `sing-box check` and `xray run -test` after replacing placeholders.
- `.github/workflows/build.yml` — resolve, test, binaries, interop, android, release, notify.

## Conventions

- Lines added to copied lx files carry a `// ut:` comment. New code goes in `*_ut.go` / `*_ut_test.go` files. Do not reformat existing lx lines.
- lx files are copied verbatim; changes to them go into a patch, never into `lx-files.txt` tricks.
- Commits: Conventional Commits in English. Author and committer `10ta <84114181+10ta@users.noreply.github.com>`. Changes reach the repository as one `git format-patch` file per delivery.
- Shell commands given to the user must run in fish as root (no bash-only syntax, no sudo).

## Architecture & invariants

- The download endpoint is a second `*Client` built by `NewClient` from `download` options; only its `newRequest`, xmux pool and `Close` are used.
- Nothing is inherited from the upload side except the dialer and the session id (same as Xray).
- Mode with `download`: `stream-one` is an error; `auto` → `packet-up`, or `stream-up` under REALITY. Resolved before the placement checks.
- `xmuxRelease.next` chains the download connection's release; every release path must stay idempotent and release both.
- `prepare.sh` stages everything before the last patch, so `git diff` in the tree is exactly that patch. It never commits: libbox reads its version from `git describe`.
- Package version `X~alpha.N+ut` sorts above the official package of the same tag and below the next one.

## Do not

- Do not follow a moving lx branch in `lx.ref`. lx's XHTTP keeps gaining imports of lx-only packages (`common/lxdatagram` after the pinned commit); bump the pin deliberately and re-run the interop.
- Do not copy `common/lxtrace/*_lx.go` or `protocol/vless/encryption/lx_vision*`: they need lx-only symbols.
- Do not carry lx's `DialedThroughDetour: tls.DialedThroughDetour(...)` line into the seams patch: the function does not exist upstream.
- Do not edit upstream's `sharedTags` line in `build_libbox/main.go`; it changes often. Use the overlay file.
- Do not switch the interop test to the latest Xray release without running it: server defaults change between releases.
- Do not commit scratch `go.mod` edits or build outputs (`work/`, `sing-box`, `xray-bin/`, `xray.zip`, `dist/`).

## Config & secrets

- Repository secrets, all optional: `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASS`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASS`, `TG_BOT_TOKEN`, `TG_CHAT_ID`. See [README → Update & Maintenance](README.md#-update--maintenance).
- The interop test uses fixed dummy UUIDs and generates VLESS encryption keys and a TLS certificate at run time.
- User-facing options: [README → Configuration](README.md#-configuration).

## Deployment & environments

- Releases are prereleases created by the `build` workflow on GitHub-hosted `ubuntu-latest`: daily for a new upstream tag, on pushes that change build inputs, and on demand.
- Pinned inputs: lx commit (`lx.ref`), Xray `v26.11.9` (`XRAY_VERSION`), gomobile (upstream's `make lib_install`), the Android client commit (upstream's submodule).

## Before you finish

- [ ] `scripts/prepare.sh` succeeds on a clean checkout; vet and tests pass in the tree.
- [ ] `test/interop/run.sh` ends with `PASS`.
- [ ] After editing files in `work/`: the last patch is regenerated and a fresh `prepare.sh` still applies everything.
- [ ] Docs synced: if commands, config keys or behaviour changed, update `README.md`, `README.zh-CN.md` and this file together.

## Gotchas

- Vision (`xtls-rprx-vision`) is not supported for now. Xray 26.11.9 rejects a flow-less client when its account, or a `flow` at the inbound `settings` level, asks for one.
- `finalmask.quicParams` does apply to XHTTP over HTTP/3 in Xray; it is unsupported here because lx hard-codes `quic.Config`, not because QUIC is unused.
- HTTP/3 with a `utls` block needs `common/tls/utls_client_std_lx.go` (in `lx-files.txt`); without it the config fails with "unsupported usage for uTLS".
- Xray 26.11.9: `freedom` blocks loopback targets unless `finalRules` allows them; the server's default `xPaddingBytes` is 100-1000 and a client range outside it gets HTTP 400.
- `curl --socks5-hostname` skips the proxy when `NO_PROXY` lists `127.0.0.1`; the test passes `--noproxy ''`.
- `git ls-remote | head -1` under `set -o pipefail` dies with SIGPIPE; the scripts use `sed -n 1p` forms.
- Android: Java must be 17; the app expects `app/libs/libbox.aar` and `app/release.keystore`, and reads signing values from `local.properties`. The APK keeps upstream's application id.
- Where proxy.golang.org is unreachable, see "踩过的坑" in [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md) for a GitHub-only way to build. There, Windows binaries and the `with_openvpn,with_openconnect` tags cannot be built.

## Current focus

- 2026-11-12: the build pipeline is written. Verified locally: `prepare.sh`, vet and tests, Linux amd64/arm64 binaries, `.deb` packaging, all six interop cases. Not yet run anywhere: the `build` workflow itself, full Windows binaries, the Android job.
- Next: the user pushes and reports the first Actions run; fix what fails, Android first. Then test against the real two-server deployment behind Caddy.
- Decided: no Vision; GPL-3.0-or-later; route "upstream tag + copied lx files + patches".
