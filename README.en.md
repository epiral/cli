<div align="center">

# Epiral CLI

**Install one tool. Turn any machine into a resource for your Agent.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[中文](README.md) | English

</div>

---

One binary, a few flags, and your machine becomes an extension of [Epiral Agent](https://github.com/epiral/agent). Workstation, VPS, Docker sandbox — the Agent doesn't care, it just sees "available resources."

A single CLI process can register two resource types simultaneously: **Computer** (shell + file operations) and **Browser** (web automation via the [bb-browser](https://github.com/yan5xu/bb-browser) Chrome extension).

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
   │    ↕ SSE         │           └─────────────────┘
   │  Chrome Extension│
   └─────────────────┘
```

## Why

AI agents need to operate real machines — but machines are behind NATs, on different networks, in different locations.

Epiral CLI uses **reverse connection**: the CLI connects outward to the Agent. No port forwarding, no SSH tunnels. The Agent sees all registered machines and can dispatch commands to any of them.

And you can connect multiple machines at once, each for a different purpose:

| Scenario | Machine | Why |
|----------|---------|-----|
| Daily dev | Workstation | Full dev environment, IDE configs |
| Untrusted scripts | Docker sandbox | Run and throw away |
| GPU training | Cloud server | Rent on demand, disconnect when done |
| Deploy testing | VPS | Simulates production |

The Agent routes tasks to the right machine. Dangerous operations go to a sandbox. The Agent itself is always safe.

## Quick Start

### Install

```bash
git clone https://github.com/epiral/cli.git
cd cli && make build
# Binary at ./bin/epiral
```

### Run

```bash
# Computer only (shell + file operations)
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-machine \
  --paths /home/me/projects

# Computer + Browser (full capabilities)
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-pc \
  --browser-id my-chrome \
  --browser-port 19824 \
  --paths /home/me/projects
```

That's it. Your machine is now available to the Agent.

## Usage

```
epiral [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--agent` | **yes** | — | Agent server URL |
| `--computer-id` | no* | hostname | Machine identifier |
| `--computer-desc` | no | same as id | Display name |
| `--browser-id` | no* | — | Browser identifier (enables browser bridge) |
| `--browser-desc` | no | same as id | Browser display name |
| `--browser-port` | no | 19824 | SSE server port for Chrome extension |
| `--paths` | no | unrestricted | Comma-separated allowed paths |
| `--token` | no | — | Authentication token |

> \* At least one of `--computer-id` or `--browser-id` must be specified.

### What gets reported on registration

| Field | Example |
|-------|---------|
| OS / Arch | `darwin/arm64` |
| Shell | `/bin/zsh` |
| Home | `/Users/kl` |
| Installed tools | `go 1.25`, `node v22.13.0`, `git 2.47.1` |
| Allowed paths | `/Users/kl/workspace` |
| Browser (if enabled) | `my-chrome` — online/offline |

### Browser Bridge

When `--browser-id` is specified, the CLI starts an embedded HTTP server bridging the [bb-browser](https://github.com/yan5xu/bb-browser) Chrome extension:

| Endpoint | Description |
|----------|-------------|
| `GET /sse` | Chrome extension connects via SSE to receive commands |
| `POST /result` | Chrome extension posts back execution results |
| `GET /status` | Health check |

Flow: Agent → gRPC → CLI → SSE → Chrome extension → execute → POST /result → CLI → gRPC → Agent

## Two Resource Types

### Computer

| Operation | Description |
|-----------|-------------|
| Shell execution | Streaming stdout/stderr in real-time |
| File read | With line offset and limit |
| File write | Auto-creates parent directories |
| File edit | Find-and-replace, supports replace_all |

All file operations are restricted to the path allowlist (`--paths`).

### Browser

Bridges the [bb-browser](https://github.com/yan5xu/bb-browser) Chrome extension via SSE, letting the Agent control the user's real browser. The extension auto-registers as online when connected, offline when disconnected.

## Connection Resilience

Tested and tuned on unreliable networks (ZeroTier with ~10% packet loss):

```
Heartbeat:    ──ping──ping──ping──ping──
                3s    3s    3s    3s

Pong timeout: 10s without pong → disconnect → reconnect

Reconnect:    1s → 2s → 4s → 8s → 16s → 30s (cap)
              └── resets to 1s after 60s stable
```

| Layer | Mechanism | Timeout |
|-------|-----------|---------|
| Application | Ping/Pong heartbeat | 3s interval, 10s deadline |
| HTTP/2 | ReadIdleTimeout | 30s |
| HTTP/2 | PingTimeout | 10s |
| TCP | Dial timeout | 10s |

Each reconnect creates a fresh HTTP/2 transport to avoid reusing broken connections.

## Internals

```
epiral-cli/
├── cmd/epiral/
│   └── main.go              # Entry: flags, signals, reconnect loop
├── internal/daemon/
│   ├── daemon.go             # Connect, register, heartbeat, dispatch
│   ├── exec.go               # Streaming shell execution
│   ├── fileops.go            # Read / write / edit files
│   └── browser.go            # Browser bridge: SSE server + forwarding
├── proto/epiral/v1/
│   └── epiral.proto          # Protocol definition
├── gen/                      # Generated protobuf + Connect RPC code
├── Makefile                  # build · generate · lint · check
└── .golangci.yml             # 14 linters configured
```

~1100 lines of hand-written Go. The rest is generated.

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

- [x] Computer: shell execution + file operations
- [x] Browser bridge (SSE-based Chrome extension integration)
- [ ] Persistent shell sessions (shell pool)
- [ ] mTLS / token authentication
- [ ] systemd / launchd service files
- [ ] Cross-compilation + GitHub Releases
- [ ] Large file upload/download

## Related

- [Epiral Agent](https://github.com/epiral/agent) — the brain (Node.js)
- [bb-browser](https://github.com/yan5xu/bb-browser) — browser automation Chrome extension

## License

[MIT](LICENSE)
