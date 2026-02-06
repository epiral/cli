[中文](README.zh-CN.md) | English

# Epiral CLI

Turn any computer into a remotely controllable development machine.

Epiral CLI is a lightweight daemon written in Go that connects to an [Epiral Agent](https://github.com/epiral/agent) server via a persistent bidirectional stream (Connect RPC over HTTP/2). Once connected, the Agent can execute shell commands, read/write/edit files on the machine — making it a first-class compute node in a distributed development environment.

## Features

- **Shell execution** — Run commands with streaming stdout/stderr
- **File operations** — Read, write, and edit (find-and-replace) files remotely
- **Auto-reconnect** — Exponential backoff (1s → 30s), resets after stable connection
- **Heartbeat** — 3s ping interval, 10s pong timeout for fast dead-connection detection
- **Tool discovery** — Auto-detects installed tools (Go, Node, Python, Git, Docker, etc.) and reports to Agent
- **Path allowlist** — Restrict file access to specific directories

## Quick Start

### Build

```bash
make build
```

### Run

```bash
./bin/epiral \
  --agent http://your-agent-host:8002 \
  --id my-machine \
  --name "My MacBook" \
  --paths /Users/me/workspace,/tmp
```

The daemon will connect, register, and start accepting commands from the Agent.

### Verify

A successful startup looks like:

```
2026/02/06 16:02:58 Epiral CLI 启动 (v0.1.2): id=my-machine, agent=http://your-agent-host:8002
2026/02/06 16:02:58 连接 Agent: http://your-agent-host:8002
2026/02/06 16:02:58 已注册: my-machine (darwin/arm64)
```

## Usage

```
epiral [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--agent` | Yes | — | Agent server address (e.g. `http://host:8002`) |
| `--id` | No | hostname | Machine identifier |
| `--name` | No | same as `--id` | Display name shown in Agent |
| `--paths` | No | unrestricted | Comma-separated allowed paths |
| `--token` | No | — | Authentication token |

## Architecture

```
┌─────────────────────────────┐
│  Epiral Agent (Node.js)     │
│  ComputerHub gRPC Server    │
│         :8002               │
└─────────┬───────────────────┘
          │  Connect RPC
          │  Bidi Stream (h2c)
          │
┌─────────┴───────────────────┐     ┌──────────────────────────┐
│  CLI Daemon @ machine-a     │     │  CLI Daemon @ machine-b  │
│  exec / read / write / edit │     │  exec / read / write ... │
└─────────────────────────────┘     └──────────────────────────┘
```

The CLI initiates the connection (daemon → Agent), solving NAT traversal. A single bidirectional gRPC stream carries all commands and responses, correlated by `request_id`.

### Protocol

Defined in [`proto/epiral/v1/epiral.proto`](proto/epiral/v1/epiral.proto):

| Direction | Message | Purpose |
|-----------|---------|---------|
| CLI → Agent | `Registration` | Identify machine (OS, arch, tools, paths) |
| CLI → Agent | `ExecOutput` | Streaming command output |
| CLI → Agent | `FileContent` | File read result |
| CLI → Agent | `OpResult` | Write/edit result |
| CLI → Agent | `Ping` | Heartbeat |
| Agent → CLI | `ExecRequest` | Execute shell command |
| Agent → CLI | `ReadFileRequest` | Read file (with pagination) |
| Agent → CLI | `WriteFileRequest` | Write file |
| Agent → CLI | `EditFileRequest` | Find-and-replace edit |
| Agent → CLI | `Pong` | Heartbeat response |

## Connection Resilience

Designed for unreliable networks (tested on ZeroTier with ~10% packet loss):

| Mechanism | Value | Description |
|-----------|-------|-------------|
| Heartbeat interval | 3s | Ping sent every 3 seconds |
| Pong timeout | 10s | No pong → disconnect and reconnect |
| Reconnect backoff | 1s → 30s | Exponential, resets after 60s stable connection |
| HTTP/2 ReadIdleTimeout | 30s | Protocol-level idle detection |
| HTTP/2 PingTimeout | 10s | Protocol-level ping ACK timeout |
| TCP dial timeout | 10s | Connection establishment timeout |

## Project Structure

```
epiral-cli/
├── cmd/epiral/main.go          # Entry point, arg parsing, reconnect loop
├── internal/daemon/
│   ├── daemon.go               # Core: connect, register, heartbeat, dispatch
│   ├── exec.go                 # Shell command execution (streaming)
│   └── fileops.go              # File read/write/edit operations
├── proto/epiral/v1/
│   └── epiral.proto            # Protocol definition
├── gen/                        # Generated protobuf code
├── Makefile                    # build, generate, lint, check
└── buf.yaml                    # Buf protobuf toolchain config
```

## Development

```bash
# Build
make build

# Format + lint + build
make check

# Regenerate protobuf code (requires buf)
make generate

# Clean
make clean
```

### Requirements

- Go 1.25+
- [buf](https://buf.build/) (for protobuf code generation)

## License

[MIT](LICENSE)
