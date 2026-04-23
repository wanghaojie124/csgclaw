# CSGClaw CLI Reference

This document supplements the CLI section in [architecture.md](./architecture.md). The architecture doc describes the role split between `csgclaw` and `csgclaw-cli`, while this document records the currently implemented commands, flags, defaults, and behavior.

## CLI Positioning

`csgclaw` is the full local operator CLI. It manages onboarding, local server lifecycle, agent runtime operations, and the shared collaboration workflows.

`csgclaw-cli` is the lightweight HTTP client intended for bots, agents, and scripts. It exposes only collaboration-oriented workflows and does not manage onboarding, config files, or server lifecycle.

Both CLIs are thin HTTP clients over the local API. They do not talk to BoxLite, stores, or channel SDKs directly.

## Shared Conventions

### Output formats

- `--output table` prints human-readable tables or plain text.
- `--output json` prints structured JSON.
- If `--output` is omitted:
  - terminal output defaults to `table`
  - piped or redirected output defaults to `json`
- Special cases:
  - `csgclaw serve`, `csgclaw stop`, and `csgclaw agent logs` default to `table`
  - `csgclaw-cli --version` defaults to `table`

### Environment variables

Both CLIs support:

- `CSGCLAW_BASE_URL`: default API endpoint
- `CSGCLAW_ACCESS_TOKEN`: default API token

If `--endpoint` or `--token` is passed, the flag overrides the environment variable.

### Channels

Most collaboration commands accept `--channel` with:

- `csgclaw`
- `feishu`

Defaults to `csgclaw` unless stated otherwise.

### Config and local paths

`csgclaw` additionally supports `--config` and reads `~/.csgclaw/config.toml` by default.

Common local defaults:

- config: `~/.csgclaw/config.toml`
- daemon log: `~/.csgclaw/server.log`
- daemon PID: `~/.csgclaw/server.pid`
- agents state: `~/.csgclaw/agents/state.json`
- built-in IM state: `~/.csgclaw/im/state.json`

## `csgclaw`

### Global flags

Usage:

```bash
csgclaw [global-flags] <command> [args]
```

Global flags:

- `--endpoint string`: HTTP server endpoint. Default from `CSGCLAW_BASE_URL`.
- `--token string`: API authentication token. Default from `CSGCLAW_ACCESS_TOKEN`.
- `--output string`: `table` or `json`.
- `--config string`: path to config file.
- `--version`, `-V`: print version and exit.

Top-level commands:

- `onboard`
- `serve`
- `stop`
- `agent`
- `user`
- `bot`
- `room`
- `member`
- `message`

### `csgclaw onboard`

Initializes local config and bootstrap state.

Usage:

```bash
csgclaw onboard [flags]
```

Flags:

- `--provider string`: LLM provider preset. Supported values: `csghub-lite`, `custom`.
- `--base-url string`: LLM provider base URL.
- `--api-key string`: LLM provider API key.
- `--models string`: comma-separated model IDs.
- `--reasoning-effort string`: optional upstream `reasoning_effort` default.
- `--manager-image string`: bootstrap manager image.
- `--force-recreate-manager`: remove and recreate the bootstrap manager box.

Behavior:

- If no config exists and no explicit model flags are provided, `onboard` can prompt interactively.
- In non-interactive usage, pass model settings explicitly.
- It writes config, ensures bootstrap IM state, and ensures the bootstrap manager bot.
- If model config is incomplete, later `serve` commands fail with a remediation hint.

Examples:

```bash
csgclaw onboard
csgclaw onboard --provider csghub-lite --models Qwen/Qwen3-0.6B-GGUF
csgclaw onboard --base-url https://api.openai.com/v1 --api-key "$OPENAI_API_KEY" --models gpt-5.4-mini
csgclaw onboard --manager-image ghcr.io/example/manager:latest --force-recreate-manager
```

### `csgclaw serve`

Starts the local HTTP server.

Usage:

```bash
csgclaw serve [-d|--daemon] [flags]
```

Flags:

- `--daemon`, `-d`: run in background.
- `--log string`: daemon log path. Daemon mode only. Default `~/.csgclaw/server.log`.
- `--pid string`: daemon PID path. Daemon mode only. Default `~/.csgclaw/server.pid`.

Behavior:

- Loads config from `--config` or `~/.csgclaw/config.toml`.
- Validates effective model configuration before startup.
- For `csghub-lite`, it performs a provider reachability preflight.
- In foreground mode it prints the effective config and IM URL.
- In daemon mode it launches the hidden internal `_serve` entrypoint and waits for `/healthz`.

Examples:

```bash
csgclaw serve
csgclaw serve --daemon
csgclaw serve --config /path/to/config.toml
csgclaw --endpoint http://127.0.0.1:18080 serve
```

### `csgclaw stop`

Stops the daemonized local server.

Usage:

```bash
csgclaw stop [flags]
```

Flags:

- `--pid string`: PID file path. Default `~/.csgclaw/server.pid`.

Behavior:

- Sends `SIGTERM` to the PID stored in the PID file.
- If the process is already gone, it removes the stale PID file and reports that state.

### `csgclaw agent`

Manages runtime agents.

Usage:

```bash
csgclaw agent <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`
- `logs`
- `status`

#### `csgclaw agent list`

Usage:

```bash
csgclaw agent list [flags]
```

Flags:

- `--filter string`: filter by agent state after listing.

#### `csgclaw agent create`

Usage:

```bash
csgclaw agent create [flags]
```

Flags:

- `--id string`: agent ID.
- `--name string`: agent name.
- `--description string`: agent description.
- `--profile string`: agent LLM profile.

#### `csgclaw agent delete`

Usage:

```bash
csgclaw agent delete <id>
```

#### `csgclaw agent logs`

Usage:

```bash
csgclaw agent logs <id> [-f|--follow] [-n lines]
```

Flags:

- `-f`, `--follow`: stream logs continuously.
- `-n int`: number of lines to fetch. Default `20`.

Behavior:

- `-n` must be greater than `0`.
- `--output json` is supported only for non-follow mode.
- `--output json --follow` returns an error.

#### `csgclaw agent status`

Usage:

```bash
csgclaw agent status [id]
```

Behavior:

- With an ID, returns one agent.
- Without an ID, behaves like `agent list`.

Examples:

```bash
csgclaw agent list
csgclaw agent list --filter running
csgclaw agent create --name alice --description "frontend worker" --profile openai.gpt-5.4-mini
csgclaw agent status
csgclaw agent status agent-alice
csgclaw agent logs agent-alice -n 50
csgclaw agent logs agent-alice --follow
csgclaw agent delete agent-alice
```

### `csgclaw user`

Manages channel users.

Usage:

```bash
csgclaw user <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

#### `csgclaw user list`

Usage:

```bash
csgclaw user list [flags]
```

Flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `csgclaw user create`

Usage:

```bash
csgclaw user create [flags]
```

Flags:

- `--channel string`: must be `feishu`.
- `--id string`: user ID.
- `--name string`: user name.
- `--handle string`: user handle.
- `--role string`: user role.
- `--avatar string`: avatar initials.

Behavior:

- This command currently supports only `--channel feishu`.

#### `csgclaw user delete`

Usage:

```bash
csgclaw user delete <id> [flags]
```

Flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

Examples:

```bash
csgclaw user list
csgclaw user list --channel feishu
csgclaw user create --channel feishu --name Alice --handle alice --role manager --avatar AL
csgclaw user delete u-alice
```

### Shared collaboration groups in `csgclaw`

The following command groups are shared with `csgclaw-cli` and use the same flags and semantics.

#### `bot`

Usage:

```bash
csgclaw bot <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

`bot list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--role string`: filter by `manager` or `worker`.

`bot create` flags:

- `--id string`: bot ID.
- `--name string`: required.
- `--description string`: bot description.
- `--role string`: required. `manager` or `worker`.
- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--model-id string`: agent model ID.

`bot delete` usage and flags:

```bash
csgclaw bot delete <id> [flags]
```

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `room`

Usage:

```bash
csgclaw room <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

`room list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

`room create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--title string`: room title.
- `--description string`: room description.
- `--creator-id string`: creator user ID.
- `--participant-ids string`: comma-separated participant IDs.
- `--locale string`: room locale.

`room delete` usage and flags:

```bash
csgclaw room delete <id> [flags]
```

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `member`

Usage:

```bash
csgclaw member <subcommand> [flags]
```

Subcommands:

- `list`
- `create`

`member list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: target room ID.

`member create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: target room ID.
- `--user-id string`: required. User to add.
- `--inviter-id string`: inviter user ID.
- `--locale string`: room locale.

#### `message`

Usage:

```bash
csgclaw message <subcommand> [flags]
```

Subcommands:

- `list`
- `create`

`message list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: required.

`message create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: required.
- `--sender-id string`: required.
- `--content string`: required.
- `--mention-id string`: optional mentioned user ID.

Examples:

```bash
csgclaw bot list
csgclaw bot create --name alice --role worker --model-id gpt-5.4-mini
csgclaw room create --title "release-room" --creator-id admin --participant-ids admin,manager
csgclaw member create --room-id room-1 --user-id u-alice --inviter-id admin
csgclaw message list --room-id room-1
csgclaw message create --channel csgclaw --room-id room-1 --sender-id admin --content hello
```

## `csgclaw-cli`

### Global flags

Usage:

```bash
csgclaw-cli [global-flags] <command> [args]
```

Global flags:

- `--endpoint string`: HTTP server endpoint. Default from `CSGCLAW_BASE_URL`.
- `--token string`: API authentication token. Default from `CSGCLAW_ACCESS_TOKEN`.
- `--output string`: `table` or `json`.
- `--version`, `-V`: print version and exit.

Top-level commands:

- `bot`
- `room`
- `member`
- `message`

### Command groups

`csgclaw-cli` reuses the same implementations as `csgclaw` for:

- `bot list`
- `bot create`
- `bot delete`
- `room list`
- `room create`
- `room delete`
- `member list`
- `member create`
- `message list`
- `message create`

That means flags, defaults, validations, and JSON shapes are identical between the two CLIs for those groups.

Examples:

```bash
csgclaw-cli bot list --channel feishu
csgclaw-cli bot create --name manager --role manager --channel feishu
csgclaw-cli room create --channel feishu --title "ops-room"
csgclaw-cli member list --channel feishu --room-id oc_x
csgclaw-cli message create --channel feishu --room-id oc_x --sender-id u-manager --content hello
```
