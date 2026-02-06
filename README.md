<div align="center">

# Epiral CLI

**装一个工具，把任何机器变成 Agent 的资源**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

中文 | [English](README.en.md)

</div>

---

一个二进制文件，几个参数，你的机器就成了 [Epiral Agent](https://github.com/epiral/agent) 的延伸。可以是工作站、VPS、Docker 沙箱——Agent 不关心，它只看到"可用资源"。

一个 CLI 进程可以同时注册两种资源：**Computer**（shell + 文件操作）和 **Browser**（网页自动化，通过 [bb-browser](https://github.com/yan5xu/bb-browser) Chrome 扩展）。

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
   │  skywork         │           │  homelab         │
   │                  │           │                  │
   │  Computer ✓      │           │  Computer ✓      │
   │  Browser  ✓      │           │                  │
   │    ↕ SSE         │           └─────────────────┘
   │  Chrome 扩展     │
   └─────────────────┘
```

## 为什么

AI Agent 需要操作真实机器——但机器在 NAT 后面、不同网络、不同地方。

Epiral CLI 用**反向连接**解决：CLI 主动连 Agent，不需要端口转发、不需要 SSH 隧道。Agent 看到所有注册的机器，可以把命令派发到任何一台。

而且可以同时连多台。不同的机器做不同的事：

| 场景 | 机器 | 说明 |
|------|------|------|
| 日常开发 | 工作站 | 有完整开发环境、IDE 配置 |
| 不信任的脚本 | Docker 沙箱 | 跑完就扔，不影响真机 |
| GPU 训练 | 云服务器 | 按需租用，用完断开 |
| 部署验证 | VPS | 模拟生产环境 |

Agent 把任务路由到对的机器。危险操作丢给沙箱，Agent 自己永远安全。

## 快速开始

### 安装

```bash
git clone https://github.com/epiral/cli.git
cd cli && make build
# 二进制文件在 ./bin/epiral
```

### 运行

```bash
# 只注册电脑（shell + 文件操作）
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-machine \
  --paths /home/me/projects

# 同时注册电脑 + 浏览器
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id skywork \
  --browser-id skywork-chrome \
  --browser-port 19824 \
  --paths /home/me/projects
```

连上就能用：

```
$ ./bin/epiral --agent http://192.168.1.100:8002 --computer-id skywork \
    --browser-id skywork-chrome --browser-port 19824
2026/02/06 19:40:54 [系统] Epiral CLI 启动 (v0.2.0): computer=skywork, browser=skywork-chrome (port 19824)
2026/02/06 19:40:55 [连接] 已注册电脑: skywork (darwin/arm64)
2026/02/06 19:40:55 [浏览器] SSE 服务已启动: port=19824, id=skywork-chrome
2026/02/06 19:40:55 [连接] 等待 Agent 下发命令...
```

## 用法

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
| 浏览器（如启用） | `skywork-chrome` — online/offline |

### Browser Bridge

指定 `--browser-id` 后，CLI 会启动一个内嵌 HTTP 服务，桥接 Chrome 扩展（[bb-browser](https://github.com/yan5xu/bb-browser)）：

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
│   └── main.go              # 入口：参数、信号处理、重连循环
├── internal/daemon/
│   ├── daemon.go             # 连接、注册、心跳、消息分发
│   ├── exec.go               # Shell 流式执行
│   ├── fileops.go            # 文件读/写/编辑
│   └── browser.go            # Browser Bridge: SSE 服务 + 命令转发
├── proto/epiral/v1/
│   └── epiral.proto          # 协议定义
├── gen/                      # 生成的 protobuf + Connect RPC 代码
├── Makefile                  # build · generate · lint · check
└── .golangci.yml             # 14 个 linter
```

~1100 行手写 Go 代码，其余是生成的。

## 开发

```bash
make build      # 编译到 ./bin/epiral
make check      # 格式化 + lint + 编译（提交前必跑）
make generate   # 重新生成 protobuf 代码（需要 buf）
make clean      # 清理构建产物
```

### 依赖

- Go 1.25+
- [buf](https://buf.build/) — protobuf 代码生成
- [golangci-lint](https://golangci-lint.run/) — lint

## Roadmap

- [x] Computer：shell 执行 + 文件操作
- [x] Browser Bridge（SSE 桥接 Chrome 扩展）
- [ ] 持久化 Shell 会话 (shell pool)
- [ ] mTLS / token 认证
- [ ] systemd / launchd 服务文件
- [ ] 交叉编译 + GitHub Releases
- [ ] 大文件上传/下载

## 相关项目

- [Epiral Agent](https://github.com/epiral/agent) — 大脑（Node.js）
- [bb-browser](https://github.com/yan5xu/bb-browser) — 浏览器自动化 Chrome 扩展

## 许可证

[MIT](LICENSE)
