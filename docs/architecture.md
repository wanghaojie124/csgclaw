# CSGClaw — System Design Document

## Overview

CSGClaw is a Go-based system with two core capabilities:

1. **Agent Manager** — sandbox-based agent lifecycle management (create, delete, status, logs) backed by Boxlite.
2. **IM System** — real-time messaging with room and user management, a WebSocket push layer, and a web UI.

Both capabilities are exposed through a unified HTTP server and a single CLI binary (`csgclaw`) that also serves as the server entry point.

---

## Architecture

```
                        ┌──────────────┐
                        │  csgclaw CLI │
                        └──────┬───────┘
                               │ HTTP
                               │
              ┌────────────────▼─────────────────┐
              │            HTTP Server            │
              │         (REST + WebSocket)        │
              └──────┬──────────────────┬─────────┘
                     │                  │
          ┌──────────▼──────┐  ┌────────▼──────────┐
          │  Agent Manager  │  │     IM System      │
          │                 │  │                    │
          │  serve / stop   │  │  rooms / users     │
          │  status / logs  │  │  messaging / push  │
          └──────┬──────────┘  └────────┬───────────┘
                 │                      │
                 ▼                      ▼
          ┌─────────────┐       ┌───────────────┐
          │   Boxlite   │       │  Filesystem   │
          │  (sandbox)  │       │   (storage)   │
          └─────────────┘       └───────────────┘

              ┌─────────────────────────────┐
              │           Web UI            │
              │   (served by HTTP Server)   │
              └─────────────────────────────┘
```

### Key design decisions

- **Single binary** — `csgclaw` is both the CLI tool and the server entry point. `csgclaw serve` starts the HTTP server; all other commands communicate with it over HTTP.
- **Agent Manager** and **IM System** are fully independent modules with no cross-imports. Cross-domain coordination, if ever needed, is handled in the API layer.
- **CLI communicates exclusively over HTTP** — it never connects directly to the filesystem or Boxlite. This means it can operate remotely against any deployed instance.
- **No `/im` prefix on HTTP routes** — IM resource names (`rooms`, `users`, `messages`) do not conflict with agent resources, so a prefix would be redundant.
- **Versioned API routes** — all REST endpoints are prefixed with `/api/v1/` to allow non-breaking evolution of the API surface.
- **Boxlite** is the fixed sandbox implementation, called directly from the Agent Manager with no abstraction layer.
- **Filesystem** is the fixed storage backend. Each domain owns its own subdirectory.
- **Web UI** is served as static assets by the HTTP server, keeping the deployment footprint to a single binary.

---

## Directory Structure

```
CSGClaw/
│
├── cmd/
│   └── csgclaw/
│       └── main.go              # Single entry point for both CLI and server
│
├── internal/
│   ├── agent/
│   │   ├── agent.go             # Core logic: create, delete, status — calls Boxlite directly
│   │   ├── log.go               # Log collection: stream from sandbox stdout/stderr, paginated query
│   │   └── store.go             # Agent metadata persistence on the filesystem
│   │
│   ├── im/
│   │   ├── hub.go               # WebSocket connection pool: register, broadcast, unregister
│   │   ├── room.go              # Room CRUD and membership management
│   │   ├── user.go              # User registration, query, kick
│   │   └── message.go           # Message persistence and delivery via hub.Broadcast()
│   │
│   ├── api/
│   │   ├── router.go            # Route registration and middleware (auth, logging)
│   │   ├── agent.go             # Handlers for /api/v1/agents/*
│   │   └── im.go                # Handlers for /api/v1/rooms/*, /api/v1/users/*, /api/v1/messages/*, /api/v1/ws
│   │
│   └── config/
│       └── config.go            # Loads config.yaml, exposes a global Config struct
│
├── cli/                         # CLI subcommand implementations (separate from cmd/ for testability)
│   ├── root.go                  # Root command: registers global flags and subcommands
│   ├── serve.go                 # csgclaw serve: starts the HTTP server (foreground or daemon)
│   ├── stop.go                  # csgclaw stop: reads pid file, sends SIGTERM
│   ├── agent.go                 # csgclaw agent: create/delete/status/logs
│   ├── room.go                  # csgclaw room: list/create/delete
│   ├── user.go                  # csgclaw user: list/kick
│   └── message.go               # csgclaw message: send/history
│
├── web/                         # Frontend UI for IM
│   ├── src/
│   └── package.json
│
├── migrations/                  # Filesystem layout init scripts, numbered sequentially (e.g. 001_init.sh).
│                                # Run once on first deploy to create required directory structure.
├── config.yaml                  # Default config: ports, storage paths, Boxlite settings
├── Makefile                     # Shortcuts: build, run, test, lint
└── go.mod
```

### Directory rationale

| Directory | Purpose |
|---|---|
| `cmd/csgclaw/` | Single entry point. No business logic. |
| `internal/agent/` | All agent lifecycle logic. Calls Boxlite directly. |
| `internal/im/` | All IM logic. `hub.go` owns the WebSocket connection pool. |
| `internal/api/` | HTTP layer only: parse request, call domain package, write response. |
| `internal/config/` | Single source of truth for runtime configuration. |
| `cli/` | One file per resource. Each subcommand does: parse flags → call HTTP → format output. |
| `web/` | Frontend source. Build output is served as static files by the HTTP server. |
| `migrations/` | Filesystem directory setup scripts, run once on first deploy. |

---

## HTTP API

All endpoints are versioned under `/api/v1/`. Resource names (`rooms`, `users`, `messages`) do not conflict with agent resources, so no additional domain prefix is needed. The WebSocket endpoint lives under `/api/v1/ws` to keep it consistent with the rest of the API surface.

A health check endpoint is provided for load balancers and orchestration systems.

```
# Health
GET    /api/v1/health            Liveness check — returns 200 OK when the server is up

# Agent
POST   /api/v1/agents            Create and start an agent
DELETE /api/v1/agents/:id        Stop and delete an agent
GET    /api/v1/agents            List all agents
GET    /api/v1/agents/:id        Get agent status
GET    /api/v1/agents/:id/logs   Stream or fetch agent logs

# IM
GET    /api/v1/rooms             List rooms
POST   /api/v1/rooms             Create a room
DELETE /api/v1/rooms/:id         Delete a room
GET    /api/v1/users             List users
DELETE /api/v1/users/:id         Kick a user
POST   /api/v1/messages          Send a message
GET    /api/v1/messages          Fetch message history

# WebSocket
WS     /api/v1/ws                Real-time message push
```

---

## CLI Design

### Command tree

```
csgclaw
├── serve                        # Start the HTTP server
├── stop                         # Stop the HTTP server

├── agent
│   ├── create                   # Create and start an agent
│   ├── delete <id>              # Stop and delete an agent
│   ├── status [id]              # List all agents, or inspect one
│   └── logs <id>                # View or stream agent logs

├── room
│   ├── list
│   ├── create
│   └── delete <id>

├── user
│   ├── list
│   └── kick <id>

└── message
    ├── send
    └── history <id>
```

### Global flags

All subcommands inherit these flags via `PersistentFlags()`. They can also be set via environment variables using the `CSGCLAW_` prefix (e.g. `CSGCLAW_TOKEN`).

| Flag | Default | Description |
|---|---|---|
| `--endpoint` | `http://localhost:8080` | HTTP server address |
| `--token` | — | API authentication token |
| `--output` | `table` | Output format: `table`, `json`, or `yaml` |
| `--config` | — | Path to a config file |

### Command reference

#### `csgclaw serve`

Start the HTTP server. Without `-d`, runs in the foreground with logs printed to stdout — the default for local development. With `-d`, daemonises the process and writes logs to a file.

```
-d, --daemon        Run in the background as a daemon
    --log string    Log file path, daemon mode only (default: ./csgclaw.log)
    --pid string    PID file path, daemon mode only (default: ./csgclaw.pid)
```

```bash
csgclaw serve                              # Foreground, logs to terminal
csgclaw serve -d                           # Background, logs to ./csgclaw.log
csgclaw serve -d --log /var/log/csgclaw.log --pid /var/run/csgclaw.pid
```

#### `csgclaw stop`

Read the PID file and send SIGTERM to the server process for a graceful shutdown.

```
--pid string    PID file path (default: ./csgclaw.pid)
```

#### `csgclaw agent create`

Create and start an agent in a Boxlite sandbox.

```
--timeout duration   Startup timeout (default: 30s)
```

#### `csgclaw agent delete <id>`

Stop and permanently delete an agent.

```
--force   Send SIGKILL instead of SIGTERM
```

#### `csgclaw agent status [id]`

List all agents, or inspect a single one. Omitting `id` returns all agents.

```
--filter string   Filter by state: running | stopped | error
```

#### `csgclaw agent logs <id>`

Fetch or stream agent logs from Boxlite stdout/stderr.

```
--follow         Stream logs in real time (like tail -f)
--tail int       Number of recent lines to show (default: 100)
--since string   Show logs from this duration ago, e.g. 1h, 30m
--level string   Filter by log level: debug | info | error
```

#### `csgclaw room create`

```
--name string         Room display name
--max-members int     Maximum member capacity
```

#### `csgclaw user list`

```
--room string   Filter by room ID
```

#### `csgclaw message send`

```
--room string      Target room ID
--from string      Sender user ID
--content string   Message body
```

#### `csgclaw message history <id>`

```
--limit int        Number of messages to return (default: 50)
--cursor string    Pagination cursor: return messages before this message ID
```

### Usage examples

```bash
# Start the server in the foreground (development)
csgclaw serve

# Start the server as a daemon (production)
csgclaw serve -d --log /var/log/csgclaw.log

# Stop the server
csgclaw stop

# Create an agent with a custom timeout
csgclaw agent create --timeout 60s

# Force-delete an agent
csgclaw agent delete agent-42 --force

# List running agents as JSON (for scripts)
csgclaw agent status --filter running --output json

# Stream error logs from an agent
csgclaw agent logs agent-42 --follow --level error

# Create an IM room
csgclaw room create --name "dev-general" --max-members 200

# Send a message
csgclaw message send --room room-1 --from user-99 --content "Hello"

# Paginate message history
csgclaw message history room-1 --limit 50 --cursor msg-500

# Target a remote server using environment variables
export CSGCLAW_ENDPOINT=http://prod-host:8080
export CSGCLAW_TOKEN=secret
csgclaw agent status --output json
```

### Implementation notes

Built with [cobra](https://github.com/spf13/cobra) and [viper](https://github.com/spf13/viper).

Each leaf command follows a strict three-step pattern:

```
parse flags → call HTTP API client → format and print output
```

`csgclaw serve` is the only exception — it does not call the HTTP API. It initialises config, storage, and the router, then starts listening.

Output formatting is centralised in a single `printer.Print(data, format)` function shared across all commands, so `--output table|json|yaml` works consistently everywhere.

```go
// cli/root.go
var rootCmd = &cobra.Command{Use: "csgclaw"}

func init() {
    f := rootCmd.PersistentFlags()
    f.String("endpoint", "http://localhost:8080", "server address")
    f.String("token",    "",                      "api token")
    f.StringP("output", "o", "table",             "table|json|yaml")
    f.String("config",  "",                       "config file path")

    viper.AutomaticEnv()
    viper.SetEnvPrefix("CSGCLAW")

    rootCmd.AddCommand(newServeCmd())
    rootCmd.AddCommand(newStopCmd())
    rootCmd.AddCommand(newAgentCmd())
    rootCmd.AddCommand(newRoomCmd())
    rootCmd.AddCommand(newUserCmd())
    rootCmd.AddCommand(newMessageCmd())
}
```

---

## Data Flow

### Agent log streaming

```
csgclaw agent logs <id> --follow
  └─► GET /api/v1/agents/{id}/logs?follow=true
        └─► agent.log.Stream(id)
              └─► Boxlite stdout/stderr (real-time)
```

### IM message delivery

```
User A sends message (WebSocket)
  └─► internal/api im.go (WS handler)
        └─► im.message.Send(roomID, msg)
              ├─► Filesystem (persist)
              └─► hub.Broadcast(roomID, msg)
                    └─► Push to all online connections in room
```

---

## Technology Choices

| Concern | Choice | Reason |
|---|---|---|
| Language | Go | Concurrency model suits both agent management and WebSocket hub |
| CLI framework | cobra + viper | De facto standard; persistent flags, env var binding, subcommand trees |
| Sandbox | Boxlite | Fixed choice; called directly, no abstraction layer needed |
| Storage | Filesystem | Simple, zero dependencies, sufficient for project scale |
| WebSocket | `gorilla/websocket` or stdlib | Managed in `hub.go`; one goroutine per connection |
| Frontend | Framework of choice | Served as static assets by the HTTP server |
