<div align="center">

# Epiral CLI

**Turn any computer into a remotely controllable development machine.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Connect RPC](https://img.shields.io/badge/RPC-Connect_RPC-6C47FF)](https://connectrpc.com)

[中文](README.zh-CN.md) | English

</div>

---

Install the Epiral CLI daemon on any machine, point it at your [Agent](https://github.com/epiral/agent) server, and that machine becomes a compute node you can control remotely — run shell commands, read/write files, all through a single persistent connection.

```
┌──────────────────────────────────────────────────────────────┐
│                    Epiral Agent (Server)                      │
│                  ComputerHub gRPC Server                      │
└──────────┬──────────────────────────┬────────────────────────┘
           │                          │
     Connect RPC                Connect RPC
     Bidi Stream (h2c)          Bidi Stream (h2c)
           │                          │
┌──────────┴──────────┐   ┌──────────┴──────────┐
│  my-pc            │   │  homelab            │
│  MacBook Pro M2     │   │  Mac Mini M4        │
│  darwin/arm64       │   │  darwin/arm64       │
│  python3, git       │   │  go, node, docker   │
└─────────────────────┘   └─────────────────────┘
```

## Why

AI coding agents need to operate real machines, not just sandboxed containers. But machines are behind NATs, on different networks, sometimes connected through flaky VPNs.

Epiral CLI solves this by **reversing the connection** — the daemon connects outward to the Agent server, establishing a persistent bidirectional stream. No port forwarding, no SSH tunnels. The Agent sees all registered machines and can dispatch commands to any of them.

## Features

- **Single binary, zero config** — one binary, a few flags, done
- **Shell execution** — run commands with streaming stdout/stderr in real-time
- **File operations** — read, write, and edit (find-and-replace) files remotely
- **Auto-reconnect** — exponential backoff, resets after stable connection
- **Tool discovery** — auto-detects installed tools (Go, Node, Python, Docker, etc.) and reports capabilities
- **Path allowlist** — restrict access to specific directories
- **Battle-tested resilience** — survives 10% packet loss on ZeroTier networks

## Quick Start

### Install

```bash
# From source
git clone https://github.com/epiral/cli.git
cd cli && make build

# Binary is at ./bin/epiral
```

### Run

```bash
./bin/epiral \
  --agent http://your-agent:8002 \
  --id my-machine \
  --paths /home/me/projects
```

That's it. Your machine is now available to the Agent.

```
$ ./bin/epiral --agent http://192.168.1.100:8002 --id my-pc
2026/02/06 16:02:58 Epiral CLI v0.1.2: id=my-pc, agent=http://192.168.1.100:8002
2026/02/06 16:02:58 连接 Agent: http://192.168.1.100:8002
2026/02/06 16:02:58 已注册: my-pc (darwin/arm64)
█  ← stays connected, waiting for commands
```

## Usage

```
epiral [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--agent` | **yes** | — | Agent server URL |
| `--id` | no | hostname | Machine identifier |
| `--name` | no | same as `--id` | Human-readable display name |
| `--paths` | no | unrestricted | Comma-separated paths the Agent can access |
| `--token` | no | — | Authentication token |

### What gets reported on registration

When the daemon connects, it sends:

| Field | Example |
|-------|---------|
| OS / Arch | `darwin/arm64` |
| Shell | `/bin/zsh` |
| Home directory | `/Users/kl` |
| Installed tools | `go 1.25`, `node v22.13.0`, `git 2.47.1`, `docker 27.5.1` |
| Allowed paths | `/Users/kl/workspace` |

## Protocol

Built on [Connect RPC](https://connectrpc.com) (HTTP/2 bidirectional streaming). A single `Connect` RPC carries all traffic:

```protobuf
service ComputerHubService {
  rpc Connect(stream ConnectRequest) returns (stream ConnectResponse);
}
```

### Messages

| Direction | Message | Description |
|-----------|---------|-------------|
| `CLI → Agent` | `Registration` | Machine identity and capabilities |
| `CLI → Agent` | `Ping` | Heartbeat (every 3s) |
| `CLI → Agent` | `ExecOutput` | Streaming command output (stdout + stderr + exit code) |
| `CLI → Agent` | `FileContent` | File read result |
| `CLI → Agent` | `OpResult` | Write/edit success or failure |
| `Agent → CLI` | `ExecRequest` | Execute a shell command |
| `Agent → CLI` | `ReadFileRequest` | Read a file (with offset/limit) |
| `Agent → CLI` | `WriteFileRequest` | Write a file (auto-creates parent dirs) |
| `Agent → CLI` | `EditFileRequest` | Find-and-replace in a file |
| `Agent → CLI` | `Pong` | Heartbeat response |

Full definition: [`proto/epiral/v1/epiral.proto`](proto/epiral/v1/epiral.proto)

## Connection Resilience

Designed for unreliable networks. Tested and tuned on ZeroTier with ~10% packet loss and 17–180ms latency jitter.

```
Heartbeat:    ──ping──ping──ping──ping──ping──ping──
                3s    3s    3s    3s    3s    3s

Pong check:   If no pong received for 10s → disconnect → reconnect

Reconnect:    1s → 2s → 4s → 8s → 16s → 30s (cap)
              └── resets to 1s after 60s stable connection
```

| Layer | Mechanism | Timeout |
|-------|-----------|---------|
| Application | Ping/Pong heartbeat | 3s interval, 10s deadline |
| HTTP/2 | `ReadIdleTimeout` | 30s |
| HTTP/2 | `PingTimeout` | 10s |
| TCP | Dial timeout | 10s |

> [!NOTE]
> Each reconnect creates a fresh HTTP/2 transport to avoid reusing broken connections.

## Internals

```
epiral-cli/
├── cmd/epiral/
│   └── main.go              # Entry point: flags, signal handling, reconnect loop
├── internal/daemon/
│   ├── daemon.go             # Connect, register, heartbeat, message dispatch
│   ├── exec.go               # Shell execution with streaming output
│   └── fileops.go            # Read / write / edit file operations
├── proto/epiral/v1/
│   └── epiral.proto          # Protocol definition (111 lines)
├── gen/                      # Generated protobuf + Connect RPC code
├── Makefile                  # build · generate · lint · check · clean
├── buf.yaml                  # Buf protobuf toolchain
└── .golangci.yml             # 13 linters configured
```

~750 lines of hand-written Go. The rest is generated.

### Key design decisions

- **Reverse connection** — CLI connects to Agent (not the other way around), solving NAT
- **Single bidi stream** — all commands and responses multiplexed on one stream, correlated by `request_id`
- **h2c (HTTP/2 cleartext)** — no TLS overhead for internal networks; add a reverse proxy for public exposure
- **Mutex-protected sends** — `stream.Send()` is not concurrent-safe in Connect RPC; a `sync.Mutex` serializes all outbound messages
- **Async command handling** — each incoming command dispatches to a goroutine, so long-running `exec` doesn't block file operations

## Development

```bash
make build      # Compile to ./bin/epiral
make check      # Format + lint + build (pre-commit)
make generate   # Regenerate protobuf code (requires buf)
make clean      # Remove build artifacts
```

### Requirements

- Go 1.25+
- [buf](https://buf.build/) for protobuf code generation
- [golangci-lint](https://golangci-lint.run/) for linting

## Roadmap

- [ ] Persistent shell sessions (shell pool)
- [ ] Environment snapshot auto-detection
- [ ] mTLS / token authentication
- [ ] Systemd / launchd service files
- [ ] Cross-compilation + GitHub Releases
- [ ] File upload/download (large files)

## Related

- [Epiral Agent](https://github.com/epiral/agent) — the server side (Node.js)

## License

[MIT](LICENSE)
