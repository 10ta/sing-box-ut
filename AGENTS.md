# AGENTS.md

## Project overview

- sing-box-ut: builds a pinned sing-box tag with a native VLESS + XHTTP client and split upload/download servers (Xray `downloadSettings`), so clients can drop xray-core. Human docs: [README.md](README.md). The user works in Chinese; the full decision log is [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md).
- Type: fork/customization kept as patches, not a fork. Each build is: upstream SagerNet/sing-box at the tag in `upstream.ref` + files copied from Leadaxe/sing-box-lx at the commit in `lx.ref` + `overlay/` + `patches/*.patch`. A daily workflow moves the two pins after testing.
- Stack: Go (the version upstream's workflow installs, 1.26.10 for `v1.15.0-alpha.13`), bash, python3 for the interop test, GitHub Actions.
- Outputs, all named `sing-box_<version>_<os>_<arch>.<ext>` (`_` only between fields; keep it that way, `upsing.sh` and users depend on it): Linux `.deb` and `.tar.gz` (amd64, arm64), Windows `.zip` (amd64, arm64), Android `.apk` (arm64), published as prereleases `<upstream tag>-ut`, `-ut.2`, `-ut.3`, … (one per build).

## Commands

Run from the repository root.

| Task | Command |
|---|---|
| Assemble the source | `scripts/prepare.sh work` (pins from `upstream.ref`, `lx.ref`; `UPSTREAM_REF=v…` and `LX_REF=…` override) |
| Build | `scripts/build.sh work sing-box` (cross: set `GOOS`, `GOARCH`) |
| Vet | in `work/`: `go vet -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS),with_xhttp" ./option/ ./common/badh2/ ./common/lxtrace/ ./common/tls/ ./transport/v2ray/ ./transport/v2rayxhttp/ ./protocol/vless/...` |
| Test | same packages with `go test -count=1 -race -tags …` |
| Single test | in `work/`: `go test -count=1 -tags "with_quic,with_utls,with_xhttp,badlinkname,tfogo_checklinkname0" ./transport/v2rayxhttp/ -run TestDownloadSplit -v` |
| Interop | `test/interop/run.sh ./sing-box xray-bin/xray` (ends with `PASS`; one case: add `stream-up:h3`) |
| Pack a .deb | `scripts/package-deb.sh work sing-box amd64 1.15.0-alpha.13-ut out.deb` |
| Would a new lx commit change the copied files? | `scripts/lx-diff.sh $(cat lx.ref) <new sha>` |
| Preview the Telegram message / failure issue | `DRY_RUN=1 … scripts/notify-telegram.sh`, `DRY_RUN=1 … scripts/report-issue.sh` (variables listed in each script) |
| Regenerate the last patch | edit files in `work/`, then `git -C work add -N . && git -C work diff > patches/0002-lx-xhttp-download.patch` |
| CI | push to main (builds and releases), Actions → `build` → Run workflow, Actions → `update` → Run workflow (the daily update, now) |

## Repository map

- `upstream.ref` — the sing-box tag. `lx.ref` — full SHA of the sing-box-lx commit files are copied from. Both are moved by the daily update. `lx-files.txt` — which lx files are copied; `scripts/lx-files.py` applies it (used by `prepare.sh` and `lx-diff.sh`).
- `patches/0001-upstream-seams.patch` — five upstream files: `constant/v2ray.go`, `option/v2ray_transport.go`, `option/vless.go`, `transport/v2ray/transport.go`, `protocol/vless/outbound.go`.
- `patches/0002-lx-xhttp-download.patch` — the download endpoint, on the copied lx files (`transport/v2rayxhttp/`, `option/v2ray_xhttp*.go`). To change download behaviour, start in `transport/v2rayxhttp/download_ut.go`.
- `overlay/cmd/internal/build_libbox/ut_tags.go` — appends `with_xhttp` to libbox's tags.
- `scripts/prepare.sh`, `build.sh`, `package-deb.sh`, `go-version.sh` — used by the workflow and by hand; keep them the single source of build logic. `fetch-cronet.sh` — prebuilt `libcronet.so` / `.dll` for the naive outbound. `make-keystore.sh` — one-time APK signing key. `notify-telegram.sh` — the build summary message (`DRY_RUN=1` prints it). `android-client.sh` — checks out the sing-box-for-android commit that matches the version being built.
- `test/interop/` — `run.sh` (cases `MODE:SCENARIO`), `gen.py` (configs: `plain`, `full`, `h3`), `tcp_tap.py` (the "download server", TCP and UDP relay with a log).
- `test/naive-start.sh` — checks a build starts a naive outbound (compiled in, library loads).
- `examples/` — complete sanitized configs. Keep the client in sync with the complete example in both READMEs; validate with `sing-box check` and `xray run -test` after replacing placeholders.
- `.github/workflows/build.yml` — modes `release` (push/dispatch), `candidate` and `apk` (called by `update.yml`); jobs resolve, test, binaries, interop, android, promote, release, report, notify.
- `.github/workflows/update.yml` + `scripts/auto-update.sh` — the daily update. `scripts/report-issue.sh` — the failure issue. `scripts/next-release-tag.sh` — `-ut.N` numbering.

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
- Build tags: Linux = upstream `DEFAULT_BUILD_TAGS` + `with_purego`, Windows = `DEFAULT_BUILD_TAGS_WINDOWS`, both + `with_xhttp`, `CGO_ENABLED=0`. The naive outbound is compiled in and loads Cronet at run time: next to the binary, `LD_LIBRARY_PATH`, `/usr/local/lib`, `/usr/lib`. The .deb installs `/usr/lib/libcronet.so`; the Windows zip carries `libcronet.dll`.
- `prepare.sh` stages everything before the last patch, so `git diff` in the tree is exactly that patch. It never commits: libbox reads its version from `git describe`.
- Package version `X~alpha.N+ut[.M]` sorts above the official package of the same tag, above earlier `-ut` builds, and below the next tag.
- Daily update order: (1) newer sing-box tag, (2) newer lx commit; the first that needs a build is the run's candidate; (3) otherwise APK backfill for the newest release. An lx move that changes none of the selected files is committed straight to main without a build.
- A candidate is built from main's scripts with the new pins passed in (no branch is pushed). Only if test, binaries and interop pass does `promote` commit the pins to main, and only if main is still the commit the candidate started from; otherwise it postpones.
- Failed candidates: one open `auto-update` issue per change (title = `自动更新失败：<subject>`), carrying `<!-- ut-base: SHA -->`. `auto-update.sh` skips a change whose issue records the current main; a new main commit or closing the issue re-enables it. A passing candidate closes its issue.
- Pushes made with `GITHUB_TOKEN` do not trigger workflows. That is relied on: the promote commit and silent lx bumps must not start a second build.

## Do not

- Do not hand-edit `upstream.ref` or `lx.ref` to an untested value: a push to main releases whatever they say. Let the daily update move them, or test the new value with `UPSTREAM_REF`/`LX_REF` first.
- Do not try to auto-copy more lx files to satisfy new imports. Tried on 2026-11-13: lx's new code reaches into lx's own config section (`option.Options.LX`), which lives in modified upstream files. Stub the feature instead (an overlay `*_stub.go`).
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

- Releases are prereleases created by the `build` workflow on GitHub-hosted `ubuntu-latest`: on pushes to main that change build inputs, on demand, and when the daily update promotes a candidate.
- Pinned inputs: sing-box tag (`upstream.ref`), lx commit (`lx.ref`), lx branch to follow (`LX_BRANCH` in `update.yml`), Xray `v26.11.9` (`XRAY_VERSION` in `build.yml`), gomobile (upstream's `make lib_install`), the Android client (the client's `dev` commit "Bump version <version>").

## Before you finish

- [ ] `scripts/prepare.sh` succeeds on a clean checkout; vet and tests pass in the tree.
- [ ] `test/interop/run.sh` ends with `PASS`.
- [ ] After editing files in `work/`: the last patch is regenerated and a fresh `prepare.sh` still applies everything.
- [ ] Docs synced: if commands, config keys or behaviour changed, update `README.md`, `README.zh-CN.md` and this file together.

## Gotchas

- `download` takes transport options only. `uuid`, `encryption`, `flow` there are rejected as unknown fields: there is one VLESS handshake per connection, and both halves must reach the same Xray inbound.
- Vision (`xtls-rprx-vision`) is not supported for now. Xray 26.11.9 rejects a flow-less client when its account, or a `flow` at the inbound `settings` level, asks for one.
- `finalmask.quicParams` does apply to XHTTP over HTTP/3 in Xray; it is unsupported here because lx hard-codes `quic.Config`, not because QUIC is unused.
- HTTP/3 with a `utls` block needs `common/tls/utls_client_std_lx.go` (in `lx-files.txt`); without it the config fails with "unsupported usage for uTLS".
- Xray 26.11.9: `freedom` blocks loopback targets unless `finalRules` allows them; the server's default `xPaddingBytes` is 100-1000 and a client range outside it gets HTTP 400.
- `curl --socks5-hostname` skips the proxy when `NO_PROXY` lists `127.0.0.1`; the test passes `--noproxy ''`.
- `git ls-remote | head -1` under `set -o pipefail` dies with SIGPIPE; the scripts use `sed -n 1p` forms.
- Android: the submodule commit upstream pins is only right for stable tags. For an alpha tag it is an older client that does not implement the new libbox interface (Kotlin: "does not implement abstract members"). Upstream builds alphas from the client's `dev` branch; `android-client.sh` picks the dev commit titled `Bump version <version>`.
- Android: the client repository ships upstream's own `app/release.keystore`; delete it before writing ours, or `keytool -genkeypair` fails with "Keystore was tampered with". Java must be 17; the app expects `app/libs/libbox.aar` and `app/release.keystore`, and reads signing values from `local.properties`. The APK keeps upstream's application id.
- Where proxy.golang.org is unreachable, see "踩过的坑" in [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md) for a GitHub-only way to build. There, Windows binaries and the `with_openvpn,with_openconnect` tags cannot be built.

## Current focus

- 2026-11-13: daily automatic updates added (pins, candidate builds, promotion, `-ut.N` releases, failure issues, APK backfill). Locally verified: the selector matches the old copy list exactly, `lx-diff.sh` on real lx history, eight `auto-update.sh` scenarios with a fake `gh`, actionlint and shellcheck clean. Not yet run on Actions.
- Expected first run: lx `lx-1.15` head (df8d8c3) changes `transport/v2rayxhttp/` (SPEC 151 QUIC sniff decoy, needs `common/lxdatagram` and lx's `option.Options.LX`), so the lx candidate fails and opens an issue. Planned fix: stub the decoy (overlay `common/lxdatagram` with a no-op `Decoy`, plus `adapter.OutboundTypeFromContext`).
- The user runs the Linux and Android builds against the real two-server deployment behind Caddy; Windows is untested.
- Decided: no Vision; GPL-3.0-or-later; route "upstream tag + copied lx files + patches"; both pins moved only by tested promotion.
