# interactive-process-mcp

<p align="center">
  <strong>Give AI Agents Interactive Terminal Capabilities</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8.svg" alt="Go 1.21+">
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux-lightgrey" alt="macOS / Linux">
  <img src="https://img.shields.io/badge/MCP-SSE_Transport-green.svg" alt="MCP SSE">
  <img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License">
</p>

<p align="center">
  <a href="./README.zh.md">中文</a> | <strong>English</strong>
</p>

---

## Introduction

`interactive-process-mcp` is an MCP (Model Context Protocol) server that enables AI Agents (like Claude Code) to start, control, and manage **long-running interactive processes**.

### Why Do You Need It?

AI Agents can natively only execute one-shot commands — they run and immediately return results. But many real-world scenarios require **multi-turn interaction**:

- SSH into a remote server, enter a password first, then run commands
- Debug code line by line in a Python REPL
- Answer `[Y/n]` prompts in interactive installers
- Use terminal-dependent commands like `top`, `htop`
- Run security tools (e.g., impacket) for multi-step operations

In these scenarios, the process keeps running, and the AI Agent needs to **repeatedly read and write** the process's I/O across **multiple conversation turns**. `interactive-process-mcp` is the bridge designed precisely for this purpose.

### Key Features

| Feature | Description |
|---------|-------------|
| **PTY and Pipe dual mode** | PTY mode emulates a real terminal; Pipe mode for simple stdin/stdout interaction |
| **Remote deployment** | SSE over HTTP transport — Agent and Server can run on different machines |
| **Multi-session management** | Manage multiple independent processes simultaneously without interference |
| **Message persistence** | Session records and I/O messages persisted to local JSON files |
| **ANSI escape code stripping** | Optional automatic removal of terminal control sequences for clean text output |
| **Non-blocking reads** | Agent reads output at its own pace; timeout returns empty instead of error |
| **Atomic send-and-read** | `send_and_read` combines sending + reading in one step |
| **Graceful termination** | SIGTERM first, then SIGKILL after a configurable grace period |
| **PTY resize** | Dynamically adjust terminal rows and columns at runtime |

---

## Architecture

```
┌──────┐  SSE/HTTP  ┌──────────────┐  Internal SSH  ┌──────────┐
│Agent │ ──────────> │ Go Server    │ ──────────────> │ PTY/     │
│(MCP) │             │ - MCP API    │  (localhost)    │ Process  │
└──────┘             │ - SSH Server │                 └──────────┘
                     └──────────────┘
                            │
                            ▼
                     ┌──────────────┐
                     │ JSON Storage │
                     │ - sessions   │
                     │ - messages   │
                     └──────────────┘
```

### Project Structure

```
.
├── cmd/server/main.go           # Entry point
├── internal/
│   ├── config/config.go         # Configuration
│   ├── mcp/
│   │   ├── server.go            # MCP SSE server & tool registration
│   │   └── handlers.go          # 10 tool handlers
│   ├── sshserver/server.go      # Internal SSH server (gliderlabs/ssh)
│   ├── sshclient/client.go      # Internal SSH client (crypto/ssh)
│   ├── session/
│   │   ├── session.go           # Session lifecycle
│   │   └── manager.go           # Thread-safe session registry
│   ├── buffer/buffer.go         # Ring buffer (1MB)
│   ├── storage/store.go         # JSON file persistence
│   ├── message/message.go       # Message management
│   └── ansi/strip.go            # ANSI escape code removal
├── pkg/api/types.go             # Public types (Session, Message)
├── go.mod
└── go.sum
```

### Key Design Decisions

1. **Internal SSH Architecture**: The server starts a gliderlabs/SSH server on localhost. Each `start_process` creates an SSH session via crypto/ssh client, leveraging SSH's mature PTY allocation, window resize, and environment variable passing mechanisms.

2. **SSE over HTTP Transport**: Unlike traditional stdio-based MCP servers, this server exposes an HTTP endpoint supporting MCP SSE transport. Agents connect remotely, enabling cross-machine deployment.

3. **JSON File Persistence**: Session metadata and I/O messages are stored as local JSON files, surviving server restarts:
   - `data/sessions.json` — Session list
   - `data/messages/{session_id}/index.json` — Message index
   - `data/messages/{session_id}/messages/{msg_id}.json` — Message content

4. **Ring Buffer**: Each running session maintains a 1MB in-memory ring buffer for real-time I/O, with output simultaneously persisted to storage.

---

## Examples

### Example 1: SSH Remote Operations

```
AI Agent Flow                                   Process Output
─────────────────                              ────────────────

start_process(
  command="ssh",
  args=["deploy@192.168.1.100"],
  mode="pty"
)
                                    ←    "deploy@192.168.1.100's password: "

send_and_read(
  text="my_secret_pass",
  press_enter=true
)
                                    ←    "Welcome to Ubuntu 22.04 LTS
                                          deploy@web-server:~$ "

send_and_read(
  text="df -h",
  press_enter=true
)
                                    ←    "Filesystem      Size  Used Avail Use% Mounted on
                                          /dev/sda1       100G   45G   55G  45% /
                                          deploy@web-server:~$ "

terminate_process(session_id="abc123")
```

### Example 2: Python REPL Debugging

```
start_process(command="python3", mode="pty")
                                    ←    "Python 3.10.12\n>>> "

send_and_read(text="data = [1, 2, 3, 4, 5]", press_enter=true)
                                    ←    ">>> "

send_and_read(text="sum(data)", press_enter=true)
                                    ←    "15\n>>> "
```

### Example 3: Multi-session Parallel Management

```
start_process(command="ping", args=["-c", "5", "google.com"], name="ping-test")
  → session_id: "a1b2c3"

start_process(command="python3", args=["-m", "http.server", "8080"], name="web-server")
  → session_id: "d4e5f6"

list_sessions()
  → [{id: "a1b2c3", status: "running"}, {id: "d4e5f6", status: "running"}]

read_output(session_id="a1b2c3")  → ping statistics

terminate_process(session_id="a1b2c3")
terminate_process(session_id="d4e5f6")
```

---

## Tool Reference

### `start_process`

Start an interactive process.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `command` | string | Yes | — | Command to execute |
| `args` | string[] | No | `[]` | Command arguments |
| `mode` | "pty" \| "pipe" | No | `"pty"` | I/O mode |
| `name` | string | No | Auto-generated | Session name |
| `rows` | integer | No | `24` | PTY row count |
| `cols` | integer | No | `80` | PTY column count |

Returns: `{ session_id, pid, initial_output }`

### `send_input`

Send text to a process.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |
| `text` | string | Yes | — | Text to send |
| `press_enter` | boolean | No | `false` | Whether to append a newline |

### `read_output`

Read new output since the last read.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |
| `strip_ansi` | boolean | No | `true` | Strip ANSI escape codes |
| `timeout` | number | No | `5` | Wait time (seconds) |
| `max_lines` | integer | No | `0` | Max lines (0 = unlimited) |

Returns: `{ output, has_more, lines_returned, bytes_returned }`

### `send_and_read`

Atomic operation: send input + wait + read output. Parameters are the union of `send_input` and `read_output`.

### `list_sessions`

List all sessions. Returns: `{ sessions: [...] }`

### `get_session_info`

Get session details. Returns: `{ id, name, command, args, mode, status, exit_code, pid, created_at }`

### `terminate_process`

Terminate a process.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |
| `force` | boolean | No | `false` | Use SIGKILL directly |
| `grace_period` | number | No | `5` | Seconds to wait after SIGTERM |

### `resize_pty`

Resize PTY dimensions (PTY mode only).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |
| `rows` | integer | No | `24` | Row count |
| `cols` | integer | No | `80` | Column count |

### `list_messages`

List the message index for a session.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |

Returns: `{ messages: [{id, type, created_at, byte_size}, ...] }`

### `get_message`

Get the content of one or more messages.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | Yes | — | Session ID |
| `message_ids` | string[] | No | — | Message IDs to retrieve |

Returns: `{ messages: [{id, session_id, type, content, created_at, byte_size}, ...] }`

---

## Installation

### Build from source

```bash
go build -o server ./cmd/server
```

**Requirements:** Go >= 1.21 / macOS or Linux

### Run

```bash
./server --host 0.0.0.0 --port 8080 --data-dir ./data
```

Options:

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `0.0.0.0` | HTTP server host |
| `--port` | `8080` | HTTP server port |
| `--data-dir` | `./data` | JSON storage directory |
| `--ssh-host` | `127.0.0.1` | Internal SSH server host |
| `--ssh-port` | `0` (random) | Internal SSH server port |

## Configuration

### Claude Code

In `.claude/settings.json` or `.mcp.json`:

```json
{
  "mcpServers": {
    "interactive-process": {
      "url": "http://your-server:8080/sse",
      "transport": "sse"
    }
  }
}
```

### Other MCP Clients

Any MCP client that supports SSE transport can connect to `http://<host>:<port>/sse`.

---

## License

MIT
