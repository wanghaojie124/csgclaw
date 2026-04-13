# CSGClaw Architecture

## Overview

CSGClaw is a Go-based local multi-agent platform. It runs a single local HTTP server, serves the Web UI, exposes REST/SSE/WebSocket APIs, and manages agents, bots, rooms, users, and messages.

The main runtime concepts are:

- **Agent**: the executable runtime unit, backed by BoxLite.
- **Channel**: a messaging backend, such as the built-in `csgclaw` IM or Feishu.
- **User**: a channel-scoped messaging identity.
- **Bot**: the product-level identity that connects one agent to one channel user.

A bot can be a `manager` or a `worker`.

```text
bot
 ├─ role: manager | worker
 ├─ agent_id  ─────────► agent runtime in BoxLite
 └─ channel + user_id ─► user identity in csgclaw IM or Feishu
```

This keeps execution concerns in `agent`, messaging concerns in `im` / `channel`, and cross-domain bot lifecycle logic in `bot`.

---

## System Diagram

```text
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│ csgclaw CLI │   │ csgclaw-cli │   │ Web UI      │
└──────┬──────┘   └──────┬──────┘   └──────┬──────┘
       └─────────────────┼─────────────────┘
                         │ HTTP
                ┌────────▼─────────┐
                │ Local HTTP API   │
                │ and Web Server   │
                └───┬─────────┬────┘
                    │         │
        ┌───────────▼───┐ ┌───▼─────────────────┐
        │ Bot Service   │ │ IM / Channel APIs   │
        └──────┬────────┘ └─────────┬───────────┘
               │                    │
      ┌────────▼────────┐  ┌────────▼────────────┐
      │ Agent Service   │  │ Channel Backends    │
      └────────┬────────┘  └────────┬────────────┘
               │                    │
           ┌───▼────┐          ┌────▼─────┐
           │BoxLite │          │Storage   │
           └────────┘          └──────────┘
```

The Web UI is served by the local HTTP server and uses the same API surface as the CLIs. At the implementation level, `internal/server` owns server lifecycle and static UI wiring, while `internal/api` owns route registration and request/response handling.

---

## Design Rules

- `cmd/csgclaw` and `cmd/csgclaw-cli` stay thin. They should only start their CLI entrypoints.
- `cli` owns command parsing, HTTP calls, and output formatting.
- `internal/api` owns HTTP request/response handling only.
- `internal/bot` owns bot creation and listing. It coordinates `agent` and channel user creation.
- `internal/agent` owns BoxLite runtime lifecycle and logs.
- `internal/im` owns the built-in `csgclaw` IM.
- `internal/channel` owns external channel integrations such as Feishu.
- Secrets must not be hardcoded or printed. Logs and startup output must keep tokens redacted.

---

## Package Layout

```text
cmd/csgclaw/            CLI entrypoint
cmd/csgclaw-cli/        lite CLI entrypoint
cli/                    command flows and user-facing output
cli/csgclawcli/         csgclaw-cli app wiring and global flag handling
cli/message/            shared message command implementation for csgclaw and csgclaw-cli
internal/server/        local HTTP server and static UI wiring
internal/api/           HTTP handlers and route registration
internal/bot/           bot lifecycle and agent/user binding
internal/agent/         agent runtime, storage, BoxLite wiring
internal/im/            built-in csgclaw IM and PicoClaw bridge
internal/channel/       external channel integrations, including Feishu
internal/config/        config defaults, load/save
web/static/             shipped frontend assets
third_party/boxlite-go/ vendored BoxLite SDK
```

`internal/bot` is the new business boundary for bot behavior. It should not be implemented as extra glue inside API handlers.

---

## Bot Model

The bot record is the stable object exposed to users and higher-level workflows.

Typical fields:

```json
{
  "id": "bot-alice",
  "name": "alice",
  "role": "worker",
  "channel": "csgclaw",
  "agent_id": "agent-alice",
  "user_id": "u-alice"
}
```

Rules:

- `role` must be `manager` or `worker`.
- `channel` defaults to `csgclaw`.
- `channel` may be `csgclaw` or `feishu`.
- each bot maps to exactly one agent.
- each bot maps to exactly one user in the selected channel.
- bot creation should create or bind both underlying identities, then persist the bot mapping.

---

## HTTP API

All new product APIs should live under `/api/v1`.

```text
# Bot
GET    /api/v1/bots                  List bots
POST   /api/v1/bots                  Create a bot

# Agent
GET    /api/v1/agents                List agents
POST   /api/v1/agents                Create an agent
GET    /api/v1/agents/{id}           Get agent status
DELETE /api/v1/agents/{id}           Stop and delete an agent
GET    /api/v1/agents/{id}/logs      Fetch or stream agent logs

# Built-in csgclaw IM
GET    /api/v1/rooms                 List rooms
POST   /api/v1/rooms                 Create a room
DELETE /api/v1/rooms/{id}            Delete a room
GET    /api/v1/users                 List users
DELETE /api/v1/users/{id}            Kick a user
GET    /api/v1/messages              Fetch message history
POST   /api/v1/messages              Send a message

# Feishu channel
GET    /api/v1/channels/feishu/users
POST   /api/v1/channels/feishu/users
GET    /api/v1/channels/feishu/rooms
POST   /api/v1/channels/feishu/rooms
GET    /api/v1/channels/feishu/rooms/{room_id}/members
POST   /api/v1/channels/feishu/rooms/{room_id}/members
POST   /api/v1/channels/feishu/messages
```

`POST /api/v1/bots` should be handled as a bot use case:

```text
API handler
  └─► internal/bot.Create
        ├─► create or bind agent through internal/agent
        ├─► create or bind channel user through internal/im or internal/channel
        └─► persist bot mapping
```

The API layer should not directly duplicate bot orchestration logic.

---

## CLI

Both CLIs are thin HTTP clients. They should not call stores, BoxLite, or channel SDKs directly.

`csgclaw` is the full local management CLI for human operators. It owns onboarding, server lifecycle, agent runtime commands, and the shared bot/room/member/user/message workflows.

`csgclaw-cli` is the lightweight CLI primarily intended for agents and scripts. It exposes only the bot, room, member, and message workflows that agents need for collaboration, and does not manage onboarding, the local server lifecycle, or agent runtime directly.

```text
csgclaw
├── serve
├── stop
├── onboard
├── agent
│   ├── create
│   ├── delete
│   ├── status
│   └── logs
├── bot
│   ├── list
│   └── create
├── room
│   ├── list
│   └── create
├── member
│   ├── list
│   └── create
├── user
│   └── list
└── message

csgclaw-cli
├── bot
│   ├── list
│   └── create
├── room
│   ├── list
│   └── create
├── member
│   ├── list
│   └── create
└── message
```

### Bot Commands

```text
csgclaw bot list   --channel <csgclaw|feishu>
csgclaw bot create --channel <csgclaw|feishu>
csgclaw-cli bot list   --channel <csgclaw|feishu>
csgclaw-cli bot create --channel <csgclaw|feishu>
```

`--channel` defaults to `csgclaw`.

Expected behavior:

- `csgclaw bot list --channel csgclaw` calls `GET /api/v1/bots?channel=csgclaw`
- `csgclaw bot list --channel feishu` calls `GET /api/v1/bots?channel=feishu`
- `csgclaw bot create --channel csgclaw` calls `POST /api/v1/bots`
- `csgclaw bot create --channel feishu` calls `POST /api/v1/bots`
- `csgclaw-cli bot list --channel csgclaw` calls `GET /api/v1/bots?channel=csgclaw`
- `csgclaw-cli bot list --channel feishu` calls `GET /api/v1/bots?channel=feishu`
- `csgclaw-cli bot create --channel csgclaw` calls `POST /api/v1/bots`
- `csgclaw-cli bot create --channel feishu` calls `POST /api/v1/bots`

The selected channel is part of the request payload or query string, not a separate CLI implementation path.

### Message Command

`message` is available in both CLIs as a thin wrapper over the message API.

```text
csgclaw message --channel <csgclaw|feishu> --room-id <id> --sender-id <id> --content <text> [--mention-id <id>]
csgclaw-cli message --channel <csgclaw|feishu> --room-id <id> --sender-id <id> --content <text> [--mention-id <id>]
```

Expected behavior:

- `csgclaw message --room-id room-1 --sender-id u-admin --content hello` calls `POST /api/v1/messages`
- `csgclaw message --channel feishu --room-id oc_alpha --sender-id u-manager --content hello` calls `POST /api/v1/channels/feishu/messages`
- `csgclaw-cli message --room-id room-1 --sender-id u-admin --content hello` calls `POST /api/v1/messages`
- `csgclaw-cli message --channel feishu --room-id oc_alpha --sender-id u-manager --content hello` calls `POST /api/v1/channels/feishu/messages`

Validation rules:

- `--channel` defaults to `csgclaw`.
- `--room-id`, `--sender-id`, and `--content` are required.
- `--mention-id` is optional and is forwarded as part of the create-message request.

---

## Creation Flow

```text
csgclaw bot create --channel feishu
  └─► POST /api/v1/bots
        └─► internal/bot.Create
              ├─► internal/agent creates BoxLite-backed agent
              ├─► internal/channel creates Feishu user
              └─► internal/bot saves:
                    bot_id
                    role
                    channel
                    agent_id
                    user_id
```

For the built-in channel, the same flow uses `internal/im` to create the user identity.

---

## Persistence

Filesystem storage remains the default persistence layer.

Each domain owns its own records:

- `agent`: runtime metadata and BoxLite state references
- `bot`: bot-to-agent-to-channel-user mapping
- `im`: built-in rooms, users, messages, and events
- `channel`: external channel integration state when needed

Do not store channel-specific details directly inside the agent record. The agent should remain the runtime object; channel identity belongs to bot/channel state.

---

## Notes

- Existing compatibility routes, such as PicoClaw-specific bot APIs or older IM aliases, can remain for compatibility, but new bot lifecycle work should use `/api/v1/bots`.
- Feishu support should live behind `internal/channel`, while bot lifecycle decisions stay in `internal/bot`.
- When changing config fields or defaults for bot/channel behavior, update loader, saver, onboard flow, tests, and docs together.
