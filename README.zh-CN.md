中文 | [English](README.md)

# Epiral CLI

把任何一台电脑变成可远程控制的开发机。

Epiral CLI 是一个用 Go 编写的轻量级守护进程，通过持久双向流（Connect RPC over HTTP/2）连接到 [Epiral Agent](https://github.com/epiral/agent) 服务器。连接建立后，Agent 可以在这台机器上执行 shell 命令、读写编辑文件——使其成为分布式开发环境中的一个计算节点。

## 特性

- **Shell 执行** — 运行命令，流式返回 stdout/stderr
- **文件操作** — 远程读取、写入、编辑（查找替换）文件
- **自动重连** — 指数退避（1s → 30s），连接稳定后自动重置
- **心跳保活** — 3s 发送 Ping，10s 无 Pong 即断连重建
- **工具发现** — 自动检测已安装的工具（Go、Node、Python、Git、Docker 等）并上报 Agent
- **路径白名单** — 限制文件访问范围到指定目录

## 快速开始

### 构建

```bash
make build
```

### 运行

```bash
./bin/epiral \
  --agent http://your-agent-host:8002 \
  --id my-machine \
  --name "My MacBook" \
  --paths /Users/me/workspace,/tmp
```

启动后自动连接、注册，开始接收 Agent 下发的命令。

### 验证

成功启动的输出：

```
2026/02/06 16:02:58 Epiral CLI 启动 (v0.1.2): id=my-machine, agent=http://your-agent-host:8002
2026/02/06 16:02:58 连接 Agent: http://your-agent-host:8002
2026/02/06 16:02:58 已注册: my-machine (darwin/arm64)
```

## 用法

```
epiral [flags]
```

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `--agent` | 是 | — | Agent 服务器地址（如 `http://host:8002`） |
| `--id` | 否 | 主机名 | 机器标识 |
| `--name` | 否 | 同 `--id` | 显示名称 |
| `--paths` | 否 | 不限制 | 允许访问的路径，逗号分隔 |
| `--token` | 否 | — | 认证 token |

## 架构

```
┌─────────────────────────────┐
│  Epiral Agent (Node.js)     │
│  ComputerHub gRPC Server    │
│         :8002               │
└─────────┬───────────────────┘
          │  Connect RPC
          │  双向流 (h2c)
          │
┌─────────┴───────────────────┐     ┌──────────────────────────┐
│  CLI Daemon @ 机器 A        │     │  CLI Daemon @ 机器 B     │
│  exec / read / write / edit │     │  exec / read / write ... │
└─────────────────────────────┘     └──────────────────────────┘
```

CLI 主动发起连接（daemon → Agent），解决 NAT 穿透问题。一条双向 gRPC 流承载所有命令和响应，通过 `request_id` 关联。

### 协议

定义在 [`proto/epiral/v1/epiral.proto`](proto/epiral/v1/epiral.proto)：

| 方向 | 消息 | 用途 |
|------|------|------|
| CLI → Agent | `Registration` | 上报机器信息（OS、架构、工具、路径） |
| CLI → Agent | `ExecOutput` | 流式命令输出 |
| CLI → Agent | `FileContent` | 文件读取结果 |
| CLI → Agent | `OpResult` | 写入/编辑结果 |
| CLI → Agent | `Ping` | 心跳 |
| Agent → CLI | `ExecRequest` | 执行 shell 命令 |
| Agent → CLI | `ReadFileRequest` | 读取文件（支持分页） |
| Agent → CLI | `WriteFileRequest` | 写入文件 |
| Agent → CLI | `EditFileRequest` | 查找替换编辑 |
| Agent → CLI | `Pong` | 心跳回应 |

## 连接稳定性

针对不稳定网络设计（在 ZeroTier ~10% 丢包环境下验证）：

| 机制 | 值 | 说明 |
|------|-----|------|
| 心跳间隔 | 3s | 每 3 秒发送 Ping |
| Pong 超时 | 10s | 无回应则断连重建 |
| 重连退避 | 1s → 30s | 指数退避，稳定 60s 后重置 |
| HTTP/2 ReadIdleTimeout | 30s | 协议层空闲检测 |
| HTTP/2 PingTimeout | 10s | 协议层 PING ACK 超时 |
| TCP 拨号超时 | 10s | 连接建立超时 |

## 项目结构

```
epiral-cli/
├── cmd/epiral/main.go          # 入口：参数解析、信号处理、重连循环
├── internal/daemon/
│   ├── daemon.go               # 核心：连接、注册、心跳、命令分发
│   ├── exec.go                 # Shell 命令执行（流式输出）
│   └── fileops.go              # 文件读取/写入/编辑
├── proto/epiral/v1/
│   └── epiral.proto            # 协议定义
├── gen/                        # 生成的 protobuf 代码
├── Makefile                    # build, generate, lint, check
└── buf.yaml                    # Buf protobuf 工具链配置
```

## 开发

```bash
# 构建
make build

# 格式化 + lint + 构建
make check

# 重新生成 protobuf 代码（需要 buf）
make generate

# 清理
make clean
```

### 依赖

- Go 1.25+
- [buf](https://buf.build/)（protobuf 代码生成）

## 许可证

[MIT](LICENSE)
