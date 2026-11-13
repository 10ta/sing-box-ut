# 设计记录：sing-box 客户端去掉 xray-core（XHTTP 上下行分离 + VLESS encryption）

这份是调研、决策和踩坑的完整记录（原 README）。怎么用看 [README](../README.zh-CN.md)，给 AI 接手用的命令和约定看 [AGENTS.md](../AGENTS.md)。

## 背景
客户端现在是 sing-box（TUN）→ 本机 socks 127.0.0.1:1234 → xray-core（VLESS + XHTTP + vlessenc）。目标是做一个 sing-box 的定制构建，原生支持这条链路，从而去掉客户端上的 xray。要求：跟进 sing-box 最新 tag（含 alpha，因为上游是半年一个大版本）；用 GitHub Actions 自动产出 Linux `.deb` 和 Android `arm64-v8a` APK；失败时发 TG 通知，用户再手动处理。
**当前状态（2026-11-12）：构建流水线已写好，等 Actions 首跑。** download 补丁和整条"上游 tag + lx 文件 + 补丁"路线本地都验证过（含 H3）。`build` workflow 会出 Linux deb（amd64、arm64）、Windows exe（amd64、arm64）和 Android arm64-v8a APK，并发 prerelease。本地验证到的是：`prepare.sh`、vet 和单元测试、Linux 两个架构的二进制、deb 打包、6 个互通用例。workflow 本身、完整的 Windows 二进制、Android 这三样还没在任何地方跑过。

## 运行环境
- 服务端：Caddy 按 path 反代到 `h2c://127.0.0.1:6501` 的 xray inbound（vless + xhttp `mode:auto`，`security:none`，开了 decryption 和 xPadding 混淆），保持不变。xray 只能做服务端，sing-box 各个移植版本都只实现了客户端。
- 客户端：Debian 旁路由虚拟机上跑 sing-box；Android 端需要自己编 APK（官方 SFA 不带这些功能）；iOS 不做。
- 用户本地没有开发环境。计划用公开 GitHub 仓库（Actions 不限分钟数）加 Codespaces（处理补丁冲突）。

## 必须支持的客户端功能（来自用户的 xray 配置）
- VLESS `encryption`（mlkem768x25519plus），不需要 flow/Vision。
- XHTTP `mode: stream-up`，H2 + TLS，uTLS 指纹 `ios`，`minVersion` 1.3，alpn h2 / http/1.1。
- xPadding：`xPaddingBytes` 92-1254，`xPaddingObfsMode`，`xPaddingMethod: tokenish`，`xPaddingPlacement: queryInHeader`，自定义 `xPaddingKey`，`xPaddingHeader: X-Cache`。
- xmux：`maxConcurrency` 8-16，`cMaxReuseTimes` 0，`hMaxRequestTimes` 600-900，`hMaxReusableSecs` 1800-3000，`hKeepAlivePeriod` 30。
- **`downloadSettings`：下行走另一台服务器，有独立的 address、TLS、serverName 和 xhttp 参数。这项必须支持。**
- 丢弃：`finalmask.quicParams`、`finalmask.udp`、`testpre`、Vision。理由（2026-11-12 更正）：不是"只走 TCP/H2 所以不生效"。Xray 的 XHTTP 在 alpn 为 `h3` 时走 QUIC，`quicParams` 是生效的，还能穿过支持 QUIC 的 CDN。丢弃的真实原因是 lx 的 HTTP/3 传输把 `quic.Config` 写死了（没有拥塞控制和窗口的配置项），也没有 udp noise。H3 本身是支持的，已测。
- **Vision 的代价（2026-11-12 实测）**：Xray 26.11.9 里 `settings` 层的 `"flow": "xtls-rprx-vision"` 是生效的，不带 flow 的客户端会被拒（`rejected since the client flow is empty`）。所以服务端要改：去掉 `settings` 层的 `flow`，给 sing-box-ut 单独一个不带 `flow` 的 client，原有 xray 客户端的 client 项各自写 `flow`。这会影响这个 inbound 上的所有现有客户端，要一起改。用户已确认不要 Vision、接受这个代价。

## 候选实现调研结论
| 实现 | XHTTP | 下载走另一台服务器 | VLESS enc | 维护方式 |
|---|---|---|---|---|
| 上游 sing-box | 无 | — | 无 | — |
| Leadaxe/sing-box-lx（分支 `lx`） | 有，支持 queryInHeader 和 xmux | **不支持**（文档明确说明，代码忽略该字段） | 有，仅客户端 | 手动 merge `upstream/stable`；只跟稳定版；所有改动用 `lx:begin` 标记 |
| justinwoo280/sing-xhttp（Go 库） | 有 | 不支持（只能换 host/path，拨号和 TLS 共用） | 无 | v0.1.x |
| shtorm-7/sing-box-extended（分支 `extended`） | 有 | **支持**：`transport.download` 下可单独设 server、TLS、detour | 有 `encryption` 字段 | 改动分散在各处，不跟上游 tag rebase |

### 2026-11-11 更新：选 (b)，在 lx 上补 download
- **lx 新开了 `lx-1.15` 分支**，已合并上游 `testing`（v1.15.0-alpha.13 + 2 个提交），有预发布 tag `v1.15.0-lx.1-rc.1`。之前"lx 只跟稳定版"的结论过时了；`LX_BRANCH` 应改为 `lx-1.15`。
- lx 的 XHTTP 结构干净（`Client` 自带请求构造、meta/padding、xmux 池），补 download 只需要：下行端点再建一个 `Client`，下行 GET 用它构造并走它的连接池。净改动约 60 行（4 个 lx 文件）+ 3 个新文件，不引入任何新依赖。
- extended 方案的代价没变：要搬 `common/xray/*`、`common/congestion` 等一串包，而且它不跟上游 tag。所以不再考虑 (a)。
- lx 文件对上游 alpha.13 的依赖闭包（按包和导出符号核对过）：比原 `lx-files.txt` 多一项 **`common/lxtrace/`**（只需 `doc.go` 和 `*_stub.go`；`*_lx.go` 带 `with_lx_command` 标签且引用 lx 独有的 `adapter.GroupNow`，不要复制）。`transport/v2rayxhttp/conntest_lx.go` 引用了它。
- 上游文件的接缝改动和原来一致（`git diff v1.15.0-alpha.13 lx-1.15` 核对过，并实际编译验证）：`constant/v2ray.go`（加 `V2RayTransportTypeXHTTP`，原方案放在 overlay 里）、`option/v2ray_transport.go`、`option/vless.go`、`transport/v2ray/transport.go`、`protocol/vless/outbound.go`。lx 在 `protocol/vless/outbound.go` 里另有一行 `DialedThroughDetour: tls.DialedThroughDetour(...)`，上游没有这个函数，接缝里不能带。
- lx 的 `go.mod` 用本地路径 replace 了三个子模块（`submodules/sing-tun`、`wireguard-go`、`utls`），直接编 lx 必须先 `git submodule update --init`。复制到上游的那些文件不依赖这三个 fork（已编译验证）。

#### download 的行为（对照 Xray `splithttp/dialer.go`）
- 配置：`transport.download`，字段为 `server`、`server_port`、`tls`，加上一整套 xhttp 选项（`host`、`path`、`headers`、`x_padding_*`、`xmux` 等）。和 Xray 一样**不从上行继承任何 xhttp 参数**，下行要的都得写全。
- 下行 GET 用 download 的地址、TLS、Host、path、padding 和它自己的 xmux 池；session id 与上行共用；拨号用 outbound 自己的 dialer（detour、bind 等照常生效）。
- `mode`：`stream-one` 不能配 download（报错）；`auto` 在有 download 时解析为 `packet-up`，REALITY 下为 `stream-up`。`download.mode` 留空，取上行的。
- 连接池：conn 关闭/失败时上下行两个池的占用一起释放（幂等）；下行 body 读失败记在下行连接的熔断器上，首次读成功两边都记成功。
- xray `downloadSettings` → 这里的对应：`address/port` → `server/server_port`；`security/tlsSettings` → `tls`；`xhttpSettings.*` → 同名 snake_case 字段。

#### 验证（2026-11-11 实测）
| 项目 | 结果 | 环境 |
|---|---|---|
| lx `lx-1.15`（`20723b7`）+ 补丁：`go vet`、`go test -race ./option/ ./transport/v2rayxhttp/` | 通过 | Go 1.26.6 |
| 新增测试 `TestDownload*`（分流、拒绝非法配置、无 download 时行为不变、下行端点拒绝连接 / 返回 400）`-count=30 -race` | 通过 | 同上 |
| 同一棵树编出的客户端对 Xray 26.11.9 互通，4 个用例 | 通过 | 同上 |
| 上游 `v1.15.0-alpha.13` + 复制的 lx 文件 + 5 处接缝 + 补丁，编出的客户端跑同样 4 个用例 | 通过 | 同上，构建标签去掉了 `with_openvpn,with_openconnect` |
| `sing-box schema`、`sing-box format` 对递归的 download 选项类型 | 正常 | 同上 |

- 互通用例是 `stream-up` / `packet-up` × `plain` / `full` / `h3`（`h3` 是 2026-11-12 加的：`full` 改走 HTTP/3，下行经 UDP 转发，同样通过）。服务端 `clients` 里并排放了一个要求 Vision 的 client 和一个不带 flow 的 client。`full` 对应真实部署的功能集：TLS 1.3 + H2 + uTLS `ios` + vlessenc（mlkem768x25519plus）+ xPadding 混淆（tokenish、queryInHeader、自定义 key 和 header）+ xmux + download。
- 互通的"下行服务器"是 `tcp_tap.py`：监听 6502，转发到 Xray 的 6501，并记录每条连接的首行。`plain` 下断言只见到 `GET /ut/…`；`full` 下 TLS 看不到请求行，只断言有连接走了下行地址。
- 没有覆盖：下行用 REALITY、经 detour 拨号、真实的两台服务器（Caddy → h2c → xray）、Android。
- 本地能编译靠的是变通办法（见"踩过的坑"），CI 上不需要。

### 历史记录：extended 方案需要的东西（已不采用）
要用 extended 的 XHTTP，需要的东西：
- 目录 `transport/v2rayxhttp/`（约 2.2k 行）。
- 上游没有、它自带的包：`common/xray/{,buf,net,pipe,signal/done,utils,uuid}`、`common/congestion`、`common/kmutex`、`common/force_close`。
- 选项类型写在它改过的 `option/v2ray_transport.go` 里：`V2RayXHTTPBaseOptions`、`V2RayXHTTPOptions`、`V2RayXHTTPDownloadOptions`、`V2RayXHTTPXmuxOptions`，以及 Placement 常量。这些需要搬到 overlay 文件里。
- 它的 `transport/v2ray/transport.go` 也有改动。

还没评估过的：extended 的 congestion 和 xray 包还会再引入多少依赖；它的 vlessenc 是否和 lx 的实现等价。

## 历史记录：最初的 init.sh 方案（文件已丢失，2026-11-12 重建为 scripts/，见文末）
- `init.sh` 子命令：
  - `scaffold`：默认命令，生成仓库骨架。
  - `check`：对比上游最新 tag 和 lx 提交与 `state.json`，输出 `changed` 和 `seams_changed`。
  - `prepare [TAG]`：浅克隆上游 → 复制 `lx-files.txt` 列出的文件 → 复制 `overlay/` → 提交为 "xenc base" → `git apply patches/*`。
  - `build`：`DEFAULT_BUILD_TAGS_OTHERS` 加 `with_xhttp`，`CGO_ENABLED=0`，`-mod=mod`。
  - `deb`：使用上游 `.fpm_systemd`，版本号形如 `1.15.0~alpha.13+xenc.<lx7>`。
  - `export`：把相对 xenc base 的全部改动写成一个补丁，lx 文件的修正也包含在内。
  - `save-state`。
  - `keystore`：生成到 `.secrets/`，已加入 gitignore。
  - 退出码：10 拉取失败，11 lx 文件缺失，12 补丁失败，13 编译失败，14 打包失败。
- 仓库结构：`lx-files.txt`、`overlay/`（`constant/v2ray_xenc.go`；`cmd/internal/build_libbox/xenc_tags.go`，用 init() 追加 `with_xhttp`，利用文件名排序晚于 main.go 执行）、`patches/0001-xenc-seams.patch`、`state.json`、`sfa-patches/`、`test/`。
- 补丁只改 4 个上游文件：
  - `option/v2ray_transport.go`：加 xhttp 分支和 enum。
  - `option/vless.go`：加 `Encryption` 字段。
  - `transport/v2ray/transport.go`：switch 的 default 分支改为查 lx 的 registry。
  - `protocol/vless/outbound.go`：接入 encryption 的 Init 和 `wrapEncryption`。
- 从 lx 复制的文件：`transport/v2rayxhttp/`、`transport/v2ray/registry.go`、`include/v2rayxhttp.go`、`option/v2ray_xhttp*.go`、`common/badh2/`、`common/tls/std_config_quic_lx.go`、`protocol/vless/encryption/`（排除 `lx_vision*`）、`protocol/vless/lx_encryption.go`。
- 如果改用 extended，`init.sh` 的整体结构可以保留，需要改的是：`LX_REPO`/`LX_BRANCH`、`lx-files.txt`、overlay 中的选项文件，以及补丁。

## 关键决策
- **用补丁文件模式，不 fork。** 上游自带 `.github/workflows`，rebase 时 `GITHUB_TOKEN` 推不了改动 workflow 的提交；换成 PAT 又会触发上游那一堆 workflow。
- **lx 的文件每次原样复制，只手工维护补丁**：lx 更新能自动跟上。
- **新增文件一律放 overlay**，例如 tag 和常量，避免改动上游文件中频繁变化的行（`DEFAULT_BUILD_TAGS*`、libbox 的 `sharedTags` 那一行）。
- **公开仓库**：Actions 不限分钟。60 天无提交会停用定时任务，用每次提交 `state.json` 解决。
- **Android App 版本不单独跟踪**：上游每个 tag 用 `clients/android` 子模块固定了配套的 SFA 提交。

## 踩过的坑
- **推荐任何实现之前，先对照用户配置逐个字段核对是否支持。** 这次就是只看了功能列表，漏掉了 `downloadSettings`。
- Vision 在 XHTTP 上只有开了 vlessenc 才能用，而且只剩 padding 作用；用户已决定不用。
- lx 的 README 里说"每周自动 rebase"已经过时，它现在手动 merge：`lx` 分支跟稳定版，`lx-1.15` 分支跟 `testing`（alpha）。
- 代理环境里 proxy.golang.org、golang.org、go.dev 都访问不了。变通办法（2026-11-11 用过，能编能测）：Go 工具链用 `pip install go-bin==1.26.6`；`GOPROXY=direct GOSUMDB=off GOFLAGS=-mod=mod`；在**临时副本**的 `go.mod` 里把非 GitHub 模块 replace 到 GitHub 镜像（`golang.org/x/*` → `github.com/golang/*`、`google.golang.org/grpc` → `github.com/grpc/grpc-go` 等）；`go.mod` 里钉的 `sagernet/sing`、`sing-tun` 提交不在任何分支上，要 `git fetch origin '+refs/pull/*:refs/pull/*'` 后用本地路径 replace；`sing-openvpn`、`sing-openconnect` 钉的提交取不到，去掉这两个构建标签。这些改动只留在临时副本里，不进补丁。
- Xray 的服务端默认值会随版本变：26.11.9 的 freedom 出站默认拦截回环地址（要加 `finalRules` 放行），服务端 `xPaddingBytes` 默认 100-1000，客户端范围超出就回 400。所以互通里 Xray 版本是钉死的。
- `curl --socks5-hostname` 在环境变量 `NO_PROXY` 含 `127.0.0.1` 时会直接绕过代理，测试会假通过。互通脚本里用 `--noproxy ''` 关掉。
- `build_libbox` 要求 Java 版本恰好是 openjdk 17；它只会把 aar 复制到 `../sing-box-for-android/app/libs`，需要手动复制到 `clients/android/app/libs`。
- deb 的包名就是 `sing-box`，会替换官方包；如果配置了官方 apt 源会被覆盖回去，需要 `apt-mark hold`。

## 待办
1. 推送后看 `build` workflow 的首次运行，按日志修，Android 优先。
2. 在真实的两台服务器（Caddy → h2c → xray）上实测 H2 和 H3。
3. 配 Secrets：APK 签名 keystore、TG 通知。
4. 以后可做：Android 改 applicationId 以便和官方 SFA 共存；下行端点的日志加标签区分上下行。

## 2026-11-12 更新：构建流水线
- 原来的 `init.sh`、`lx-files.txt`、overlay、接缝补丁都没进过仓库，已经丢了。这次重建为 `scripts/prepare.sh` + `lx-files.txt` + `overlay/` + `patches/0001-upstream-seams.patch`，结构和原方案一致。
- **lx 改为钉死提交（`lx.ref`），不跟分支。** 原因：`lx-1.15` 在 `20723b7` 之后给 XHTTP 加了对 `common/lxdatagram` 和 `adapter.OutboundTypeFromContext` 的依赖（lx 专有），直接跟分支当天就编不过。换提交要人工确认依赖闭包并跑互通。
- `lx-files.txt` 比原方案多两项：`common/lxtrace` 的 stub，和 `common/tls/utls_client_std_lx.go`。后者不带的话，H3 + `utls` 的配置会报 `unsupported usage for uTLS`，lx 自己的三个测试也会失败。带上之后，复制过来的 lx 测试在上游树上全部通过。
- deb 不用 fpm（ruby 依赖，且本地没法测），改用 `dpkg-deb`，文件清单从上游的 `.fpm_systemd` 解析，所以跟着上游走。包名仍是 `sing-box`，版本 `1.15.0~alpha.13+ut`：比同 tag 的官方包新，比下一个 alpha 旧。
- Linux 和 Windows 都用 `DEFAULT_BUILD_TAGS_OTHERS` + `with_xhttp`，纯 Go（`CGO_ENABLED=0`），所以没有 naive 出站。上游的 musl + cronet 构建太重，没有照搬。
- Android：只编 arm64 的 libbox，`with_xhttp` 通过 overlay 里的 `ut_tags.go` 追加；用上游 tag 钉住的 SFA 提交；`assembleOtherRelease` 后取 arm64-v8a 的 APK。没配签名 secrets 时用一次性密钥，文件名带 `-UNSIGNED-KEY`。applicationId 没改，和官方 SFA 不能共存。
- 版本号：二进制是 `<tag>-ut`；libbox 的版本来自 `git describe`，是上游原版本号，所以 `prepare.sh` 只暂存不提交。
- 定时任务每天一次，只在"上游最新 tag 还没有对应 release"时构建。推送改到构建输入时也会构建并覆盖同名 release 的文件。
- **Android 客户端版本（2026-11-12，Actions 实测）**：上游 tag 里 `clients/android` 钉的提交只对稳定版有效。alpha tag 钉的是旧客户端（alpha.13 钉的是 `Bump version 1.14.4`），没有实现新 libbox 接口，Kotlin 编译报 `does not implement abstract members`。上游自己编 alpha 时用的是客户端的 `dev` 分支。`dev` 上每个 sing-box 版本有一个 `Bump version <版本>` 提交，`scripts/android-client.sh` 就取这个提交，找不到才退回钉住的提交。
- 客户端仓库自带上游的 `app/release.keystore`，要先删掉再放我们的，否则 `keytool` 报密码错误。
