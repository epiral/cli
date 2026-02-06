<div align="center">

# Epiral CLI

**把任何一台电脑变成可远程控制的开发机。**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Connect RPC](https://img.shields.io/badge/RPC-Connect_RPC-6C47FF)](https://connectrpc.com)

中文 | [English](README.md)

</div>

---

在任意一台机器上安装 Epiral CLI daemon，指向你的 [Agent](https://github.com/epiral/agent) 服务器，这台机器就变成了一个可远程控制的计算节点——执行 shell 命令、读写文件，全部通过一条持久连接完成。

```
┌──────────────────────────────────────────────────────────────┐
│                    Epiral Agent (服务端)                       │
│                  ComputerHub gRPC Server                      │
└──────────┬──────────────────────────┬────────────────────────┘
           │                          │
     Connect RPC                Connect RPC
     双向流 (h2c)                双向流 (h2c)
           │                          │
┌──────────┴──────────┐   ┌──────────┴──────────┐
│  my-pc            │   │  homelab            │
│  MacBook Pro M2     │   │  Mac Mini M4        │
│  darwin/arm64       │   │  darwin/arm64       │
│  python3, git       │   │  go, node, docker   │
└─────────────────────┘   └─────────────────────┘
```

## 为什么需要它

AI 编程 Agent 需要操控真实的机器，而不仅仅是沙箱容器。但机器在 NAT 后面，在不同网络上，有时还通过不稳定的 VPN 连接。

Epiral CLI 通过**反向连接**解决这个问题——daemon 主动向外连接 Agent 服务器，建立一条持久的双向流。不需要端口转发，不需要 SSH 隧道。Agent 能看到所有注册的机器，可以向任意一台下发命令。

## 特性

- **单二进制，零配置** — 一个二进制文件，几个参数，搞定
- **Shell 执行** — 运行命令，实时流式返回 stdout/stderr
- **文件操作** — 远程读取、写入、编辑（查找替换）文件
- **自动重连** — 指数退避，连接稳定后自动重置
- **工具发现** — 自动检测已安装工具（Go、Node、Python、Docker 等）并上报能力
- **路径白名单** — 限制访问范围到指定目录
- **实战验证的稳定性** — 在 ZeroTier 10% 丢包网络下正常运行

## 快速开始

### 安装

```bash
# 从源码构建
git clone https://github.com/epiral/cli.git
cd cli && make build

# 二进制在 ./bin/epiral
```

### 运行

```bash
./bin/epiral \
  --agent http://your-agent:8002 \
  --id my-machine \
  --paths /home/me/projects
```

完事。你的机器现在对 Agent 可用了。

```
$ ./bin/epiral --agent http://192.168.1.100:8002 --id my-pc
2026/02/06 16:02:58 Epiral CLI v0.1.2: id=my-pc, agent=http://192.168.1.100:8002
2026/02/06 16:02:58 连接 Agent: http://192.168.1.100:8002
2026/02/06 16:02:58 已注册: my-pc (darwin/arm64)
█  ← 保持连接，等待命令
```

## 用法

```
epiral [flags]
```

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `--agent` | **是** | — | Agent 服务器地址 |
| `--id` | 否 | 主机名 | 机器标识 |
| `--name` | 否 | 同 `--id` | 显示名称 |
| `--paths` | 否 | 不限制 | Agent 可访问的路径，逗号分隔 |
| `--token` | 否 | — | 认证 token |

### 注册时上报的信息

daemon 连接时会自动发送：

| 字段 | 示例 |
|------|------|
| OS / 架构 | `darwin/arm64` |
| Shell | `/bin/zsh` |
| Home 目录 | `/Users/kl` |
| 已安装工具 | `go 1.25`、`node v22.13.0`、`git 2.47.1`、`docker 27.5.1` |
| 允许路径 | `/Users/kl/workspace` |

## 协议

基于 [Connect RPC](https://connectrpc.com)（HTTP/2 双向流）。一个 `Connect` RPC 承载所有流量：

```protobuf
service ComputerHubService {
  rpc Connect(stream ConnectRequest) returns (stream ConnectResponse);
}
```

### 消息类型

| 方向 | 消息 | 说明 |
|------|------|------|
| `CLI → Agent` | `Registration` | 上报机器身份和能力 |
| `CLI → Agent` | `Ping` | 心跳（每 3 秒） |
| `CLI → Agent` | `ExecOutput` | 流式命令输出（stdout + stderr + exit code） |
| `CLI → Agent` | `FileContent` | 文件读取结果 |
| `CLI → Agent` | `OpResult` | 写入/编辑成功或失败 |
| `Agent → CLI` | `ExecRequest` | 执行 shell 命令 |
| `Agent → CLI` | `ReadFileRequest` | 读取文件（支持 offset/limit 分页） |
| `Agent → CLI` | `WriteFileRequest` | 写入文件（自动创建父目录） |
| `Agent → CLI` | `EditFileRequest` | 文件查找替换 |
| `Agent → CLI` | `Pong` | 心跳回应 |

完整定义：[`proto/epiral/v1/epiral.proto`](proto/epiral/v1/epiral.proto)

## 连接稳定性

针对不稳定网络设计。在 ZeroTier ~10% 丢包、17–180ms 延迟抖动的环境下调优和验证。

```
心跳:         ──ping──ping──ping──ping──ping──ping──
                3s    3s    3s    3s    3s    3s

Pong 检测:    10 秒内没有收到 Pong → 断连 → 重连

重连退避:     1s → 2s → 4s → 8s → 16s → 30s (上限)
              └── 连接稳定 60s 后重置为 1s
```

| 层级 | 机制 | 超时 |
|------|------|------|
| 应用层 | Ping/Pong 心跳 | 3s 间隔，10s 截止 |
| HTTP/2 | `ReadIdleTimeout` | 30s |
| HTTP/2 | `PingTimeout` | 10s |
| TCP | 拨号超时 | 10s |

> [!NOTE]
> 每次重连都会创建全新的 HTTP/2 transport，避免复用已损坏的连接。

## 内部结构

```
epiral-cli/
├── cmd/epiral/
│   └── main.go              # 入口：参数解析、信号处理、重连循环
├── internal/daemon/
│   ├── daemon.go             # 连接、注册、心跳、消息分发
│   ├── exec.go               # Shell 命令流式执行
│   └── fileops.go            # 文件读取 / 写入 / 编辑
├── proto/epiral/v1/
│   └── epiral.proto          # 协议定义（111 行）
├── gen/                      # 生成的 protobuf + Connect RPC 代码
├── Makefile                  # build · generate · lint · check · clean
├── buf.yaml                  # Buf protobuf 工具链
└── .golangci.yml             # 配置了 13 个 linter
```

~750 行手写 Go 代码，其余是生成的。

### 关键设计决策

- **反向连接** — CLI 连接 Agent（而非反过来），解决 NAT 穿透
- **单条双向流** — 所有命令和响应复用一条流，通过 `request_id` 关联
- **h2c (HTTP/2 明文)** — 内网无 TLS 开销；公网暴露时加反向代理
- **Mutex 保护发送** — Connect RPC 的 `stream.Send()` 非并发安全，用 `sync.Mutex` 串行化所有出站消息
- **异步命令处理** — 每个传入命令在 goroutine 中处理，长时间运行的 `exec` 不会阻塞文件操作

## 开发

```bash
make build      # 编译到 ./bin/epiral
make check      # 格式化 + lint + 构建（提交前检查）
make generate   # 重新生成 protobuf 代码（需要 buf）
make clean      # 清理构建产物
```

### 依赖

- Go 1.25+
- [buf](https://buf.build/) — protobuf 代码生成
- [golangci-lint](https://golangci-lint.run/) — 代码检查

## 路线图

- [ ] 持久化 Shell 会话（Shell Pool）
- [ ] 环境快照自动检测
- [ ] mTLS / Token 认证
- [ ] Systemd / launchd 服务文件
- [ ] 交叉编译 + GitHub Releases
- [ ] 大文件上传下载

## 相关项目

- [Epiral Agent](https://github.com/epiral/agent) — 服务端（Node.js）

## 许可证

[MIT](LICENSE)
