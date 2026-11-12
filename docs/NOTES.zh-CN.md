# 设计记录：sing-box 客户端去掉 xray-core（XHTTP 上下行分离 + VLESS encryption）

这份是调研、决策和踩坑的完整记录（原 README）。怎么用看 [README](../README.zh-CN.md)，给 AI 接手用的命令和约定看 [AGENTS.md](../AGENTS.md)。

## 背景
客户端现在是 sing-box（TUN）→ 本机 socks 127.0.0.1:1234 → xray-core（VLESS + XHTTP + vlessenc）。目标是做一个 sing-box 的定制构建，原生支持这条链路，从而去掉客户端上的 xray。要求：跟进 sing-box 最新 tag（含 alpha，因为上游是半年一个大版本）；用 GitHub Actions 自动产出 Linux `.deb` 和 Android `arm64-v8a` APK；失败时发 TG 通知，用户再手动处理。
**当前状态（2026-11-11）：恢复推进，走方案 (b)，download 补丁已实测通过。** 在 lx 的 XHTTP 上补出了上下行分离（`patches/0002-lx-xhttp-download.patch`），本地编译、单元测试、对真实 Xray 26.11.9 的互通都过了，包括"上游 alpha.13 + lx 文件 + 接缝 + 本补丁"这条最终构建路线。还没做的是自动构建流水线（deb / APK）和在真实服务器上的实测。

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
- 可以丢弃：`finalmask.quicParams` 和 `finalmask.udp`（走 TCP/H2，本来就不生效）、`testpre`、Vision。

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

- 互通用例是 `stream-up` / `packet-up` × `plain` / `full`。`full` 对应真实部署的功能集：TLS 1.3 + H2 + uTLS `ios` + vlessenc（mlkem768x25519plus）+ xPadding 混淆（tokenish、queryInHeader、自定义 key 和 header）+ xmux + download。
- 互通的"下行服务器"是 `tcp_tap.py`：监听 6502，转发到 Xray 的 6501，并记录每条连接的首行。`plain` 下断言只见到 `GET /ut/…`；`full` 下 TLS 看不到请求行，只断言有连接走了下行地址。
- 没有覆盖：下行走 HTTP/3、下行用 REALITY、经 detour 拨号、真实的两台服务器（Caddy → h2c → xray）、Android。
- 本地能编译靠的是变通办法（见"踩过的坑"），CI 上不需要。

### 历史记录：extended 方案需要的东西（已不采用）
要用 extended 的 XHTTP，需要的东西：
- 目录 `transport/v2rayxhttp/`（约 2.2k 行）。
- 上游没有、它自带的包：`common/xray/{,buf,net,pipe,signal/done,utils,uuid}`、`common/congestion`、`common/kmutex`、`common/force_close`。
- 选项类型写在它改过的 `option/v2ray_transport.go` 里：`V2RayXHTTPBaseOptions`、`V2RayXHTTPOptions`、`V2RayXHTTPDownloadOptions`、`V2RayXHTTPXmuxOptions`，以及 Placement 常量。这些需要搬到 overlay 文件里。
- 它的 `transport/v2ray/transport.go` 也有改动。

还没评估过的：extended 的 congestion 和 xray 包还会再引入多少依赖；它的 vlessenc 是否和 lx 的实现等价。

## 已有产出（在用户手上：init.sh 和 README.md，基于 lx 方案）
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
0. （当前）补丁已实测通过。接下来：
   - 推送后确认 `verify-download` 在 Actions 上也是绿的。
   - 把 `init.sh` 等已有产出提交进仓库（现在还在用户手上）。改三处：`LX_BRANCH=lx-1.15`；`lx-files.txt` 加 `common/lxtrace/doc.go` 和 `common/lxtrace/*_stub.go`；`patches/0002-lx-xhttp-download.patch` 会在 `prepare` 的 `git apply patches/*` 里自动打上（它改的全是从 lx 复制来的文件，路径一致）。
   - 把用户的 xray 客户端配置翻成 sing-box 配置，在真实的两台服务器上实测。
   - 再接着做第 2 步的完整 workflow（interop 一步可以直接复用 `test/interop/run.sh`）。
1. ~~决定 XHTTP 的来源~~（已选 b，以下保留作记录）：
   - (a) 从 extended 抽取 XHTTP 加 download，enc 用 lx 或 extended 的。先评估依赖闭包，以及它对上游 alpha 的适配程度。
   - (b) 在 lx 的 XHTTP 上自己实现 downloadSettings，可参考 extended `transport/v2rayxhttp/client.go` 里的 `Download` 分支。
   - (c) 维持现状，客户端继续用 xray。
2. 选定后：先用户在 Codespaces 里 `./init.sh build` 验证首次编译，再写 workflow：check → linux（prepare/build/deb）→ interop（runner 里起 xray 服务端，自签证书，两个 inbound 模拟分离，curl 验证）→ android → release（prerelease，加 `save-state` 并提交，失败发 TG）。

## 运行
下面是 `init.sh` 的用法。`init.sh` 还没进仓库；仓库里现在能跑的命令见 [README](../README.zh-CN.md)。
```bash
./init.sh && ./init.sh prepare && ./init.sh build    # Codespaces 里运行，需要 Go
./init.sh check                                       # CI 判断是否需要构建
# 补丁失败：照 .rej 修改 work/sing-box → 删掉 .rej → ./init.sh build → ./init.sh export → 提交 patches/
```
