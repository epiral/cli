<div align="center">

# Epiral CLI

**一个二进制，任何机器变成 Agent 的延伸**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-BSL%201.1-orange.svg)](LICENSE)

中文 | [English](README.en.md)

</div>

---

一个二进制，你的机器就成了 [Epiral Agent](https://github.com/epiral/agent) 的延伸。工作站、VPS、Docker 沙箱——Agent 不关心是什么，只看到可用资源。

一个进程同时注册两种资源：**Computer**（shell + 文件）和 **Browser**（网页自动化，通过 [bb-browser](https://github.com/yan5xu/bb-browser) Chrome 扩展）。内置 Web 管理面板，配置、日志、状态一目了然。

```
                      Epiral Agent
                 ┌──────────────────────┐
                 │     ComputerHub      │
                 │  ┌────────────────┐  │
                 │  │  computers [ ] │  │
                 │  │  browsers  [ ] │  │
                 │  └────────────────┘  │
                 └──┬─────────────┬─────┘
                    │             │
          ┌─────────┘             └─────────┐
          │                                 │
   ┌──────┴──────────┐           ┌──────────┴──────┐
   │  Epiral CLI      │           │  Epiral CLI      │
   │  my-pc         │           │  homelab         │
   │                  │           │                  │
   │  Computer ✓      │           │  Computer ✓      │
   │  Browser  ✓      │           │                  │
   │  Web UI :19800   │           │  Web UI :19800   │
   │    ↕ SSE         │           └─────────────────┘
   │  Chrome 扩展     │
   └─────────────────┘
```

## 为什么

Agent 需要操作真实机器。但机器在 NAT 后面，不同网络，不同地方。

**反向连接**：CLI 主动连 Agent，无需端口转发，无需 SSH。Agent 看到所有注册的机器，命令派发到任何一台。

同时连多台，不同机器做不同事：

| 场景 | 机器 | 说明 |
|------|------|------|
| 日常开发 | 工作站 | 有完整开发环境、IDE 配置 |
| 不信任的脚本 | Docker 沙箱 | 跑完就扔，不影响真机 |
| GPU 训练 | 云服务器 | 按需租用，用完断开 |
| 部署验证 | VPS | 模拟生产环境 |

Agent 路由任务到对的机器。危险操作丢沙箱，Agent 永远安全。

## 快速开始

### 安装

```bash
git clone https://github.com/epiral/cli.git
cd cli && make build
# 二进制文件在 ./bin/epiral
```

### 运行（推荐：Web 管理面板）

```bash
# 启动 Web 管理面板，在浏览器中完成配置
./bin/epiral start

# 指定配置文件和端口（多实例场景）
./bin/epiral start --config ~/.epiral/dev.yaml --port 19802
```

打开 `http://localhost:19800`，在 Config 页面填写 Agent 地址和 Computer/Browser ID，点击 Save & Restart 即可。

### 运行（直连模式）

```bash
# 只注册电脑（shell + 文件操作）
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-machine \
  --paths /home/me/projects

# 同时注册电脑 + 浏览器
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-pc \
  --browser-id my-chrome \
  --browser-port 19824 \
  --paths /home/me/projects
```

## Web 管理面板

`epiral start` 启动内嵌的 Web 管理面板（默认端口 19800），提供：

| 页面 | 功能 |
|------|------|
| **Dashboard** | 连接状态、Computer/Browser 信息、在线时长、重连次数 |
| **Config** | 可视化配置 Agent/Computer/Browser，Save & Restart 一键生效 |
| **Logs** | 实时日志流（SSE），分级显示，支持滚动和暂停 |

配置持久化在 `~/.epiral/config.yaml`，修改后自动重启 Daemon，无需手动操作。

### 多实例

同一台机器可以运行多个 CLI 实例（如同时连 dev 和 prod Agent）：

```bash
# Dev 实例
./bin/epiral start --config ~/.epiral/dev.yaml --port 19800

# Prod 实例
./bin/epiral start --config ~/.epiral/prod.yaml --port 19801
```

每个实例有独立的配置文件、Web 端口和 Browser SSE 端口。

## 用法

### `epiral start`（推荐）

```
epiral start [flags]
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--config` | `~/.epiral/config.yaml` | 配置文件路径 |
| `--port` | 19800 | Web 管理面板端口 |

### `epiral`（直连模式）

```
epiral [flags]
```

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `--agent` | **是** | — | Agent 服务地址 |
| `--computer-id` | 否* | hostname | 电脑标识符 |
| `--computer-desc` | 否 | 同 id | 电脑显示名 |
| `--browser-id` | 否* | — | 浏览器标识符（启用浏览器桥接） |
| `--browser-desc` | 否 | 同 id | 浏览器显示名 |
| `--browser-port` | 否 | 19824 | Chrome 扩展 SSE 服务端口 |
| `--paths` | 否 | 不限制 | 允许 Agent 访问的路径（逗号分隔） |
| `--token` | 否 | — | 认证 token |

> \* `--computer-id` 和 `--browser-id` 至少指定一个。

### 注册时上报的信息

| 字段 | 示例 |
|------|------|
| OS / Arch | `darwin/arm64` |
| Shell | `/bin/zsh` |
| Home | `/Users/kl` |
| 已安装工具 | `go 1.25`, `node v22.13.0`, `git 2.47.1`, `docker 27.5.1` |
| 允许路径 | `/Users/kl/workspace` |
| 浏览器（如启用） | `my-chrome` — online/offline |

### Browser Bridge

指定 `--browser-id`（或在 Web 面板中配置 Browser ID）后，CLI 会启动一个内嵌 HTTP 服务，桥接 Chrome 扩展（[bb-browser](https://github.com/yan5xu/bb-browser)）：

| 端点 | 说明 |
|------|------|
| `GET /sse` | Chrome 扩展通过 SSE 连接，接收命令 |
| `POST /result` | Chrome 扩展回传执行结果 |
| `GET /status` | 健康检查 |

命令流转：Agent → gRPC → CLI → SSE → Chrome 扩展 → 执行 → POST /result → CLI → gRPC → Agent

## 两种资源类型

### Computer

Agent 可以在远程电脑上执行的操作：

| 操作 | 说明 |
|------|------|
| Shell 执行 | 流式 stdout/stderr，实时返回 |
| 文件读取 | 支持行偏移和行数限制 |
| 文件写入 | 自动创建父目录 |
| 文件编辑 | 查找替换，支持 replace_all |

所有文件操作受路径白名单（`--paths`）限制。

### Browser

通过内嵌的 SSE 服务桥接 [bb-browser](https://github.com/yan5xu/bb-browser) Chrome 扩展，让 Agent 操控用户的真实浏览器。扩展连上后自动注册为 online，断开自动标记 offline。

## 连接韧性

在不稳定网络（如 ZeroTier ~10% 丢包）下实测调优：

```
心跳:     ──ping──ping──ping──ping──
            3s    3s    3s    3s

Pong 超时:  10s 未收到 → 断开 → 重连

重连退避:   1s → 2s → 4s → 8s → 16s → 30s (上限)
            └── 稳定 60s 后重置为 1s
```

| 层 | 机制 | 超时 |
|----|------|------|
| 应用层 | Ping/Pong 心跳 | 3s 间隔，10s 超时 |
| HTTP/2 | ReadIdleTimeout | 30s |
| HTTP/2 | PingTimeout | 10s |
| TCP | Dial timeout | 10s |

每次重连都创建全新的 HTTP/2 transport，避免复用损坏的连接。

## 内部结构

```
epiral-cli/
├── cmd/epiral/
│   └── main.go              # 入口：子命令分发、信号处理
├── internal/
│   ├── config/
│   │   └── config.go         # YAML 配置加载/保存/Store
│   ├── daemon/
│   │   ├── daemon.go          # 连接、注册、心跳、消息分发
│   │   ├── manager.go         # Daemon 生命周期管理（启停重启）
│   │   ├── exec.go            # Shell 流式执行
│   │   ├── fileops.go         # 文件读/写/编辑
│   │   └── browser.go         # Browser Bridge: SSE 服务 + 命令转发
│   ├── logger/
│   │   └── logger.go          # Ring buffer 日志 + SSE 订阅
│   └── webserver/
│       └── server.go          # Web 管理面板 (REST API + embed SPA)
├── web/                       # React + Vite + Tailwind 前端源码
├── proto/epiral/v1/
│   └── epiral.proto           # 协议定义
├── gen/                       # 生成的 protobuf + Connect RPC 代码
├── Makefile                   # build · web · generate · lint · check
└── .golangci.yml              # 14 个 linter
```

~2000 行手写 Go 代码，其余是生成的。

## 开发

```bash
make build      # 完整构建（前端 + Go）
make build-go   # 仅构建 Go（使用已有的 dist）
make web        # 仅构建前端
make dev        # 前端开发模式（vite dev server）
make check      # 格式化 + lint + 编译（提交前必跑）
make generate   # 重新生成 protobuf 代码（需要 buf）
make clean      # 清理构建产物
```

### 依赖

- Go 1.25+
- Node.js 22+ / pnpm — 前端构建
- [buf](https://buf.build/) — protobuf 代码生成
- [golangci-lint](https://golangci-lint.run/) — lint

## Roadmap

- [x] Computer：shell 执行 + 文件操作
- [x] Browser Bridge（SSE 桥接 Chrome 扩展）
- [x] Web 管理面板（Dashboard / Config / Logs）
- [x] YAML 配置持久化
- [x] 多实例支持（`--config` + `--port`）
- [ ] 持久化 Shell 会话 (shell pool)
- [ ] mTLS / token 认证
- [ ] systemd / launchd 服务文件
- [ ] 交叉编译 + GitHub Releases
- [ ] 大文件上传/下载

## 相关项目

- [Epiral Agent](https://github.com/epiral/agent) — 大脑（Node.js）
- [bb-browser](https://github.com/yan5xu/bb-browser) — 浏览器自动化 Chrome 扩展

## 许可证

[BSL 1.1](LICENSE)
