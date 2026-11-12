# sing-box-ut

[English](README.md) · **简体中文**

一组补丁，让 sing-box 原生支持 VLESS + XHTTP 客户端和上下行分离，客户端不再需要 xray-core。

[![verify-download](https://github.com/10ta/sing-box-ut/actions/workflows/verify-download.yml/badge.svg)](https://github.com/10ta/sing-box-ut/actions/workflows/verify-download.yml)

```text
$ test/interop/run.sh ./sing-box xray-bin/xray
== stream-up / plain
ok: payload intact, 1 connection(s) via the download server
== packet-up / plain
ok: payload intact, 1 connection(s) via the download server
== stream-up / full
ok: payload intact, 1 connection(s) via the download server
== packet-up / full
ok: payload intact, 1 connection(s) via the download server
PASS
```
*打过补丁的 sing-box 客户端对接真实的 Xray 26.11.9 服务端，上行和下行走不同地址。*

## 目录

- [为什么是 sing-box-ut？](#-为什么是-sing-box-ut)
- [快速开始](#-快速开始)
- [使用](#-使用)
- [配置](#-配置)
- [更新与维护](#-更新与维护)
- [工作原理](#-工作原理)
- [项目结构](#-项目结构)
- [开发](#-开发)
- [局限与路线](#-局限与路线)
- [上游与差异](#-上游与差异)
- [文档地图](#-文档地图)
- [许可](#-许可)

## 💡 为什么是 sing-box-ut？

Xray 的 XHTTP 可以把上行发到一台服务器、下行发到另一台（`downloadSettings`）。没有哪个 sing-box 构建支持这一点，所以需要它的 sing-box 客户端只能在旁边再跑一个 xray-core。本仓库把缺的这一块做成补丁，打在 [sing-box-lx](https://github.com/Leadaxe/sing-box-lx) 上，它已经有 XHTTP 客户端和 VLESS encryption。

- ✅ 适合：sing-box 客户端通过 VLESS + XHTTP 连接 Xray 服务端，并且用了 `downloadSettings`。
- ❌ 不适合：想要现成的二进制（目前不发布，见[局限与路线](#-局限与路线)）；需要 XHTTP 服务端（只有客户端，服务端继续用 Xray）；想在 `stream-one` 模式下分离下行。

## 🚀 快速开始

需要 Linux 上的 git 和 Go 1.26 或更新版本。

```bash
git clone https://github.com/10ta/sing-box-ut
cd sing-box-ut
git clone --depth 1 -b lx-1.15 --recurse-submodules --shallow-submodules https://github.com/Leadaxe/sing-box-lx lx
git -C lx apply ../patches/0002-lx-xhttp-download.patch
cd lx
env CGO_ENABLED=0 go build -trimpath -tags "with_quic,with_utls,with_clash_api,with_xhttp,badlinkname,tfogo_checklinkname0" -ldflags "-s -w -checklinkname=0" -o ../sing-box ./cmd/sing-box
cd ..
./sing-box version
```

最后一条命令会打印构建标签，其中必须有 `with_xhttp`：

```text
sing-box version unknown

Environment: go1.26.6 linux/amd64
Tags: with_quic,with_utls,with_clash_api,with_xhttp,badlinkname,tfogo_checklinkname0
```

## 🧭 使用

### 对接真实的 Xray 服务端做验证

需要 bash、python3、curl 和 unzip，并且 127.0.0.1 上的 1080、6501、6502、8000 端口空闲。

```bash
curl -fsSL -o xray.zip https://github.com/XTLS/Xray-core/releases/download/v26.11.9/Xray-linux-64.zip
unzip -q xray.zip xray -d xray-bin
test/interop/run.sh ./sing-box xray-bin/xray
```

最后一行是 `PASS`，和页面顶部的输出一样。每个用例通过代理取回一个 5 MB 的文件，并检查下行请求确实走了第二个地址。`full` 用例同时启用 TLS 1.3、HTTP/2、uTLS、VLESS encryption、X-Padding 混淆和 XMUX。

### 在 GitHub Actions 里跑同样的检查

打开 **Actions → verify-download → Run workflow**。推送改到补丁、互通测试或 workflow 时也会自动运行。这个任务把补丁打到 sing-box-lx 上，运行 `gofmt`、`go vet` 和单元测试，编译客户端，再跑互通测试。

## 🔧 配置

补丁只给 VLESS 出站的 XHTTP transport 加了一个对象 `download`，其余都是 sing-box-lx 原有的 XHTTP 配置。

| 名称 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `download.server` | ✅ | — | 下行服务器地址 |
| `download.server_port` | ✅ | — | 下行服务器端口 |
| `download.tls` | ❌ | 明文 | 下行服务器的出站 TLS 选项 |
| `download.path`、`host`、`headers`、`x_padding_*`、`xmux` 等 | ❌ | transport 默认值 | 下行请求的 XHTTP 选项，不从上行继承任何一项 |
| `download.mode` | ❌ | 上行的 mode | 留空。填了不同的值会报错 |

规则与 Xray 一致：

- 下行请求和上行共用 session id，拨号用出站自己的 dialer（detour、bind）。
- `stream-one` 配 `download` 会报错。`auto` 配 `download` 解析为 `packet-up`，REALITY 下为 `stream-up`。
- `download` 里不能再嵌套 `download`。

最小示例（结构与 [`test/interop/gen.py`](test/interop/gen.py) 的 `full` 用例相同）：

```json
{
  "type": "vless",
  "server": "up.example.com",
  "server_port": 443,
  "uuid": "<YOUR_UUID>",
  "encryption": "<YOUR_VLESS_ENCRYPTION>",
  "tls": { "enabled": true, "server_name": "up.example.com", "alpn": ["h2", "http/1.1"] },
  "transport": {
    "type": "xhttp",
    "mode": "stream-up",
    "path": "/<YOUR_PATH>/",
    "x_padding_bytes": "92-1254",
    "download": {
      "server": "down.example.com",
      "server_port": 443,
      "tls": { "enabled": true, "server_name": "down.example.com", "alpn": ["h2", "http/1.1"] },
      "path": "/<YOUR_PATH>/",
      "x_padding_bytes": "92-1254"
    }
  }
}
```

| Xray `downloadSettings` | 这里 |
|---|---|
| `address`、`port` | `download.server`、`download.server_port` |
| `security`、`tlsSettings` | `download.tls` |
| `xhttpSettings.*` | `download.*`，改为 snake_case，例如 `xPaddingBytes` → `x_padding_bytes` |

## 🔄 更新与维护

### sing-box-lx 更新时

workflow 跟的是 `lx-1.15` 分支的最新提交。如果 lx 改到了补丁涉及的行，“Apply the download patch” 这一步会失败。在一个新的 lx clone 里重做补丁：

```bash
git -C lx apply --reject ../patches/0002-lx-xhttp-download.patch
# fix every *.rej by hand, delete them, then:
git -C lx add -A
git -C lx diff --cached > patches/0002-lx-xhttp-download.patch
```

### Xray 发新版本时

互通测试钉死了 Xray 版本（workflow 里的 `XRAY_VERSION`），因为服务端默认值会随版本变化。要换版本，先手动运行 workflow 并在 `xray_version` 输入框填新 tag，通过后再改默认值。

## 🧠 工作原理

```mermaid
flowchart LR
  C[sing-box client] -- "upload: POST, session id" --> U[upload server]
  C -- "download: GET, same session id" --> D[download server]
  U --> X[Xray XHTTP inbound]
  D --> X
```

- **下行端点就是第二个 XHTTP 客户端** → 请求构造的所有规则（padding、placement、headers、HTTP 版本）直接复用，连接池各用各的，sing-box-lx 原有文件只改了约 70 行。
- **两条池化连接一起释放** → 连接关闭或失败时，两个池都不会漏掉占用。
- **用补丁，不 fork** → sing-box-lx 一直在更新。只动它四个文件的补丁维护成本低，打不上时会立刻报错。

## 📁 项目结构

```text
.
├── patches/
│   └── 0002-lx-xhttp-download.patch   # the change: XHTTP download endpoint + unit tests
├── test/interop/
│   ├── run.sh                         # runs every case against a real Xray
│   ├── gen.py                         # writes the Xray and sing-box configs for one case
│   └── tcp_tap.py                     # stands in for the download server, logs what reaches it
├── .github/workflows/
│   └── verify-download.yml            # apply, vet, test, build, interop
├── docs/NOTES.zh-CN.md                # research, decisions and pitfalls (Chinese)
└── AGENTS.md                          # commands and conventions for AI coding agents
```

补丁在 sing-box-lx 里改动的文件：

| 文件 | 改动 |
|---|---|
| `option/v2ray_xhttp.go` | 加 `download` 字段 |
| `option/v2ray_xhttp_download_ut.go` | 新增：`download` 的选项类型 |
| `transport/v2rayxhttp/client.go` | 构建下行端点，确定 mode |
| `transport/v2rayxhttp/conn.go` | 下行 GET 改走下行端点 |
| `transport/v2rayxhttp/xmux.go` | 下行连接随上行连接一起释放 |
| `transport/v2rayxhttp/download_ut.go` | 新增：下行端点的逻辑 |
| `transport/v2rayxhttp/download_ut_test.go` | 新增：单元测试 |

## 🔨 开发

在[快速开始](#-快速开始)里打好补丁的 `lx` 目录中运行：

```bash
gofmt -l option/v2ray_xhttp*.go transport/v2rayxhttp/
go vet -tags "with_quic,with_utls,with_clash_api,with_xhttp,badlinkname,tfogo_checklinkname0" ./option/ ./transport/v2rayxhttp/
go test -count=1 -race -tags "with_quic,with_utls,with_clash_api,with_xhttp,badlinkname,tfogo_checklinkname0" ./option/ ./transport/v2rayxhttp/
```

`gofmt` 没有输出，两个测试包都显示 `ok`。补丁加进 lx 文件的行都带 `// ut:` 标记。新代码尽量放在 `*_ut.go` 文件里，让补丁碰到的 lx 原有行越少越好。

## 🚧 局限与路线

- 不发布二进制。跟随 sing-box 最新 alpha 自动构建 Linux `.deb` 和 Android APK 的流水线，设计写在 [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md) 里，还没有放进本仓库。
- 只在 Linux amd64 上测过。没测过：下行端点走 HTTP/3 或 REALITY、经 detour 拨号、Android。
- 只有客户端。sing-box-lx 没有 XHTTP 服务端。

## 🔀 上游与差异

基于 [Leadaxe/sing-box-lx](https://github.com/Leadaxe/sing-box-lx) 的 `lx-1.15` 分支（`20723b7`），它合并了 [SagerNet/sing-box](https://github.com/SagerNet/sing-box) 的 `v1.15.0-alpha.13`。

| 方面 | sing-box | sing-box-lx | 本仓库 |
|---|---|---|---|
| XHTTP 客户端 | ❌ | ✅ | ✅ |
| VLESS encryption | ❌ | ✅ | ✅ |
| 下行走另一台服务器 | ❌ | ❌ | ✅ |

同步方式是手动：补丁打不上时 workflow 会报出来，见[更新与维护](#-更新与维护)。

## 📚 文档地图

- [AGENTS.md](AGENTS.md)：给 AI 编码代理的命令、约定和注意事项。
- [docs/NOTES.zh-CN.md](docs/NOTES.zh-CN.md)：候选实现的调研、设计决策、测试记录和踩过的坑（中文）。

## 📄 许可

<!-- TODO: add a LICENSE file. -->
还没有许可证文件。补丁修改的是 sing-box 和 sing-box-lx 的源码，它们的许可证是 GPL-3.0-or-later。
