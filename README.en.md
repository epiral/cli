<div align="center">

# Epiral CLI

**One binary. Any machine becomes your Agent's extension.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-BSL%201.1-orange.svg)](LICENSE)

[中文](README.md) | English

</div>

---

One binary, and your machine becomes an extension of [Epiral Agent](https://github.com/epiral/agent). Workstation, VPS, Docker sandbox — the Agent doesn't care what it is, just that it's available.

One process registers two resource types: **Computer** (shell + files) and **Browser** (web automation via the [bb-browser](https://github.com/yan5xu/bb-browser) Chrome extension). Built-in web management panel for configuration, logs, and status at a glance.

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
   │  Web UI :19800   │           │  Web UI :19800   │
   │    ↕ SSE         │           └─────────────────┘
   │  Chrome Extension│
   └─────────────────┘
```

## Why

Agents need real machines. But machines are behind NATs, on different networks, in different places.

**Reverse connection**: the CLI connects outward to the Agent. No port forwarding. No SSH. The Agent sees all registered machines and dispatches to any of them.

Multiple machines at once, each for a different purpose:

| Scenario | Machine | Why |
|----------|---------|-----|
| Daily dev | Workstation | Full dev environment, IDE configs |
| Untrusted scripts | Docker sandbox | Run and throw away |
| GPU training | Cloud server | Rent on demand, disconnect when done |
| Deploy testing | VPS | Simulates production |

The Agent routes tasks to the right machine. Dangerous ops go to a sandbox. The Agent is always safe.

## Quick Start

### Install

```bash
git clone https://github.com/epiral/cli.git
cd cli && make build
# Binary at ./bin/epiral
```

### Run (Recommended: Web Panel)

```bash
# Start with the web management panel
./bin/epiral start

# With custom config and port (multi-instance)
./bin/epiral start --config ~/.epiral/dev.yaml --port 19802
```

Open `http://localhost:19800`, fill in Agent address and Computer/Browser IDs on the Config page, click Save & Restart.

### Run (Direct Mode)

```bash
# Computer only (shell + file operations)
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id my-machine \
  --paths /home/me/projects

# Computer + Browser (full capabilities)
./bin/epiral \
  --agent http://your-agent:8002 \
  --computer-id skywork \
  --browser-id skywork-chrome \
  --browser-port 19824 \
  --paths /home/me/projects
```

## Web Management Panel

`epiral start` launches an embedded web panel (default port 19800):

| Page | Features |
|------|----------|
| **Dashboard** | Connection status, Computer/Browser info, uptime, reconnect count |
| **Config** | Visual configuration for Agent/Computer/Browser, Save & Restart |
| **Logs** | Real-time log stream (SSE), level filtering, scroll and pause |

Configuration is persisted to `~/.epiral/config.yaml`. Changes automatically restart the daemon — no manual intervention needed.

### Multi-Instance

Run multiple CLI instances on the same machine (e.g., connecting to both dev and prod Agents):

```bash
# Dev instance
./bin/epiral start --config ~/.epiral/dev.yaml --port 19800

# Prod instance
./bin/epiral start --config ~/.epiral/prod.yaml --port 19801
```

Each instance has its own config file, web port, and Browser SSE port.

## Usage

### `epiral start` (Recommended)

```
epiral start [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `~/.epiral/config.yaml` | Config file path |
| `--port` | 19800 | Web panel port |

### `epiral` (Direct Mode)

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
| Browser (if enabled) | `skywork-chrome` — online/offline |

### Browser Bridge

When `--browser-id` is specified (or configured in the web panel), the CLI starts an embedded HTTP server bridging the [bb-browser](https://github.com/yan5xu/bb-browser) Chrome extension:

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
│   └── main.go              # Entry: subcommand dispatch, signals
├── internal/
│   ├── config/
│   │   └── config.go         # YAML config load/save/Store
│   ├── daemon/
│   │   ├── daemon.go          # Connect, register, heartbeat, dispatch
│   │   ├── manager.go         # Daemon lifecycle (start/stop/restart)
│   │   ├── exec.go            # Streaming shell execution
│   │   ├── fileops.go         # Read / write / edit files
│   │   └── browser.go         # Browser bridge: SSE server + forwarding
│   ├── logger/
│   │   └── logger.go          # Ring buffer logging + SSE subscriptions
│   └── webserver/
│       └── server.go          # Web panel (REST API + embedded SPA)
├── web/                       # React + Vite + Tailwind frontend source
├── proto/epiral/v1/
│   └── epiral.proto           # Protocol definition
├── gen/                       # Generated protobuf + Connect RPC code
├── Makefile                   # build · web · generate · lint · check
└── .golangci.yml              # 14 linters configured
```

~2000 lines of hand-written Go. The rest is generated.

## Development

```bash
make build      # Full build (frontend + Go)
make build-go   # Go only (uses existing dist)
make web        # Frontend only
make dev        # Frontend dev mode (vite dev server)
make check      # Format + lint + build (pre-commit)
make generate   # Regenerate protobuf code (requires buf)
make clean      # Remove build artifacts
```

### Requirements

- Go 1.25+
- Node.js 22+ / pnpm — frontend build
- [buf](https://buf.build/) for protobuf code generation
- [golangci-lint](https://golangci-lint.run/) for linting

## Roadmap

- [x] Computer: shell execution + file operations
- [x] Browser bridge (SSE-based Chrome extension integration)
- [x] Web management panel (Dashboard / Config / Logs)
- [x] YAML config persistence
- [x] Multi-instance support (`--config` + `--port`)
- [ ] Persistent shell sessions (shell pool)
- [ ] mTLS / token authentication
- [ ] systemd / launchd service files
- [ ] Cross-compilation + GitHub Releases
- [ ] Large file upload/download

## Related

- [Epiral Agent](https://github.com/epiral/agent) — the brain (Node.js)
- [bb-browser](https://github.com/yan5xu/bb-browser) — browser automation Chrome extension

## License

[BSL 1.1](LICENSE)
