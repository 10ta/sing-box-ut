# 交接：sing-box 客户端去掉 xray-core（XHTTP 上下行分离 + VLESS encryption）

## 背景
客户端现在是 sing-box（TUN）→ 本机 socks 127.0.0.1:1234 → xray-core（VLESS + XHTTP + vlessenc）。目标是做一个 sing-box 的定制构建，原生支持这条链路，从而去掉客户端上的 xray。要求：跟进 sing-box 最新 tag（含 alpha，因为上游是半年一个大版本）；用 GitHub Actions 自动产出 Linux `.deb` 和 Android `arm64-v8a` APK；失败时发 TG 通知，用户再手动处理。
**当前状态：暂停。** 原因是选定的 XHTTP 实现不支持上下行分离，这是硬性要求。

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
- lx 的 README 里说"每周自动 rebase"已经过时，它现在只手动 merge 稳定版。
- 代理环境里 proxy.golang.org、golang.org、go.dev 都访问不了，无法本地编译，只能在 Actions 或 Codespaces 里编。
- `build_libbox` 要求 Java 版本恰好是 openjdk 17；它只会把 aar 复制到 `../sing-box-for-android/app/libs`，需要手动复制到 `clients/android/app/libs`。
- deb 的包名就是 `sing-box`，会替换官方包；如果配置了官方 apt 源会被覆盖回去，需要 `apt-mark hold`。

## 待办
1. 决定 XHTTP 的来源：
   - (a) 从 extended 抽取 XHTTP 加 download，enc 用 lx 或 extended 的。先评估依赖闭包，以及它对上游 alpha 的适配程度。
   - (b) 在 lx 的 XHTTP 上自己实现 downloadSettings，可参考 extended `transport/v2rayxhttp/client.go` 里的 `Download` 分支。
   - (c) 维持现状，客户端继续用 xray。
2. 选定后：先用户在 Codespaces 里 `./init.sh build` 验证首次编译，再写 workflow：check → linux（prepare/build/deb）→ interop（runner 里起 xray 服务端，自签证书，两个 inbound 模拟分离，curl 验证）→ android → release（prerelease，加 `save-state` 并提交，失败发 TG）。

## 运行
```bash
./init.sh && ./init.sh prepare && ./init.sh build    # Codespaces 里运行，需要 Go
./init.sh check                                       # CI 判断是否需要构建
# 补丁失败：照 .rej 修改 work/sing-box → 删掉 .rej → ./init.sh build → ./init.sh export → 提交 patches/
```
