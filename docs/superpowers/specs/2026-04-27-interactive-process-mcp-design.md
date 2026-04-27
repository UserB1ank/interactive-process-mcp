# Interactive Process MCP — Design Spec

## Problem

AI agents (Claude Code) need to interact with long-running interactive processes — SSH sessions, interactive installers, impacket tools, REPLs, etc. Current MCP tooling has no way to start a process, keep it running, send input, and read output over multiple turns.

## Solution

An MCP server that wraps interactive processes with a session-based API. Each process gets a unique session ID. The agent starts a process, sends input, reads output, and terminates it — all through MCP tools. Processes run in background threads with output buffering so the agent can read at its own pace.

## Architecture

**Language:** Python 3.10+
**Concurrency:** Multi-threaded (one reader thread per session)
**Process management:** pexpect (PTY mode) + subprocess (pipe mode)
**Transport:** MCP stdio (JSON-RPC over stdin/stdout)
**Primary client:** Claude Code

### Session Model

Each interactive process is a `Session`:

- `id` — UUID
- `name` — optional human-readable name
- `command` + `args` — what was launched
- `mode` — `"pty"` or `"pipe"`
- `status` — `"running"`, `"exited"`, or `"error"`
- `exit_code` — None until exit
- `output_buffer` — ring buffer storing output history
- `read_cursor` — marks how far the agent has read
- `reader_thread` — background thread continuously reading output

### Output Buffer

- Ring buffer per session (max ~1MB or 10000 lines)
- Background thread appends chunks; MCP tool reads consume from the cursor
- Oldest content dropped on overflow

### Thread Model

```
MCP Server (main thread — handles JSON-RPC)
  ├── Session reader thread → output_buffer
  ├── Session reader thread → output_buffer
  └── Session reader thread → output_buffer
```

Reader threads are daemons. They loop: `read_nonblocking(timeout=0.1)` → append to buffer → repeat until EOF.

## MCP Tools

### `start_process`

Start an interactive process.

| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| command | string | yes | — | Command to execute |
| args | string[] | no | [] | Command arguments |
| mode | "pty" \| "pipe" | no | "pty" | I/O mode |
| name | string | no | auto | Human-readable name |
| cwd | string | no | inherit | Working directory |
| env | object | no | inherit | Environment variables |
| timeout | number | no | 10 | Startup timeout (seconds) |
| rows | number | no | 24 | PTY rows |
| cols | number | no | 80 | PTY columns |

Returns: `{ session_id, pid, initial_output }`

### `send_input`

Send text to a running process.

| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| session_id | string | yes | — | Session ID |
| text | string | yes | — | Text to send |
| press_enter | boolean | no | false | Append newline |

Returns: `{ success: true }`

### `read_output`

Read new output since last read.

| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| session_id | string | yes | — | Session ID |
| strip_ansi | boolean | no | true | Remove ANSI escape codes |
| timeout | number | no | 5 | Wait time for new output (seconds) |
| max_lines | number | no | 100 | Max lines to return |

Returns: `{ output, has_more, lines_returned, bytes_returned }`

- Returns content after `read_cursor`; advances cursor
- If no new output, waits up to `timeout` seconds, then returns empty (not an error)
- `has_more=true` means more buffered content remains

### `send_and_read`

Atomic send + read. Parameters from `send_input` and `read_output` combined.

### `list_sessions`

Returns all sessions: `{ sessions: [{ id, name, command, status, pid, created_at }] }`

### `terminate_process`

| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| session_id | string | yes | — | Session ID |
| force | boolean | no | false | SIGKILL instead of SIGTERM |
| grace_period | number | no | 5 | Seconds before SIGKILL after SIGTERM |

### `resize_pty`

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| session_id | string | yes | Session ID |
| rows | number | yes | New row count |
| cols | number | yes | New column count |

### `get_session_info`

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| session_id | string | yes | Session ID |

Returns full session state including buffer statistics.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Process fails to start | Error response, no session created |
| Reading from exited process | Return remaining buffer + exit status |
| Writing to exited process | Error: "Process has exited" |
| Unknown session_id | Error: "Session not found" |
| Buffer overflow | Drop oldest content, mark truncated |
| Process unresponsive | `read_output` timeout returns empty |
| Server shutdown | SIGTERM all processes, wait, exit |

## ANSI Handling

- `strip_ansi=true` (default): remove color codes, cursor movement, screen clear sequences — return plain text
- `strip_ansi=false`: return raw output
- PTY mode always generates ANSI sequences; default strip is recommended

## Project Structure

```
interactive-process-mcp/
├── pyproject.toml
├── src/
│   └── interactive_process_mcp/
│       ├── __init__.py
│       ├── server.py      # MCP server entry + JSON-RPC
│       ├── session.py     # Session class, process lifecycle
│       ├── buffer.py      # Ring output buffer
│       ├── tools.py       # MCP tool definitions + handlers
│       └── ansi.py        # ANSI escape code stripping
├── tests/
│   ├── test_session.py
│   ├── test_buffer.py
│   ├── test_tools.py
│   └── test_ansi.py
└── README.md
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `mcp` | Official Python MCP SDK |
| `pexpect` | PTY-based process management |
| `ansi` | ANSI escape code stripping |

## Claude Code Integration

```json
{
  "mcpServers": {
    "interactive-process": {
      "command": "python",
      "args": ["-m", "interactive_process_mcp"]
    }
  }
}
```

## Process Lifecycle

```
start_process → create session → spawn process → start reader thread
                                                ↓
                              [running — agent reads/writes freely]
                                                ↓
                    terminate_process (SIGTERM → grace_period → SIGKILL)
                      OR process exits on its own (reader detects EOF)
                                                ↓
                              session.status = "exited", exit_code set
                              session retained for final read
```
