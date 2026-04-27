# interactive-process-mcp

MCP server for managing interactive processes. Allows AI agents (Claude Code, etc.) to start long-running interactive programs — SSH sessions, REPLs, installers, impacket tools — and interact with them over multiple turns via read/write operations.

## Features

- **PTY & Pipe modes** — PTY mode emulates a real terminal (programs like SSH, top, vim work correctly); pipe mode for simpler stdin/stdout interaction
- **Multi-session** — manage multiple interactive processes simultaneously
- **ANSI stripping** — optional removal of terminal escape codes for clean output
- **Non-blocking reads** — agent reads output at its own pace with configurable timeouts
- **Graceful shutdown** — SIGTERM with configurable grace period before SIGKILL

## Requirements

- Python >= 3.10
- Linux (uses `pty` module)

## Installation

```bash
pip install -e .
```

Or with dev dependencies (for running tests):

```bash
pip install -e ".[dev]"
```

## Configuration

### Claude Code

command

```
claude mcp add --scope user interactive-process -- interactive-process-mcp
```

Or

Add to your Claude Code MCP settings (`.claude/settings.json` or project-level `.mcp.json`):

```json
{
  "mcpServers": {
    "interactive-process": {
      "command": "interactive-process-mcp"
    }
  }
}
```

Or if installed from source:

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

### Other MCP Clients

Any MCP client that supports stdio transport can use this server. The entry point is:

```bash
interactive-process-mcp
# or
python -m interactive_process_mcp
```

## Tools

### `start_process`

Start an interactive process and return its session info.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `command` | string | yes | — | Command to execute |
| `args` | string[] | no | `[]` | Command arguments |
| `mode` | "pty" \| "pipe" | no | `"pty"` | I/O mode |
| `name` | string | no | auto | Human-readable session name |
| `cwd` | string | no | inherit | Working directory |
| `env` | object | no | inherit | Environment variables |
| `timeout` | number | no | `10` | Startup timeout (seconds) |
| `rows` | integer | no | `24` | PTY rows (pty mode) |
| `cols` | integer | no | `80` | PTY columns (pty mode) |

Returns: `{ session_id, pid, initial_output }`

### `send_input`

Send text to a running process.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | yes | — | Session ID |
| `text` | string | yes | — | Text to send |
| `press_enter` | boolean | no | `false` | Append newline |

Returns: `{ success: true }` or `{ error: "..." }`

### `read_output`

Read new output since the last read. If no new output is available, waits up to `timeout` seconds. Returns empty on timeout (not an error).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | yes | — | Session ID |
| `strip_ansi` | boolean | no | `true` | Remove ANSI escape codes |
| `timeout` | number | no | `5` | Wait time (seconds) |
| `max_lines` | integer | no | `0` | Max lines (0 = unlimited) |

Returns: `{ output, has_more, lines_returned, bytes_returned }`

### `send_and_read`

Atomic send + read. Sends input, waits briefly, then returns new output.

Combines parameters from `send_input` and `read_output`.

### `list_sessions`

List all active sessions.

Returns: `{ sessions: [{ id, name, command, status, pid, created_at }] }`

### `terminate_process`

Terminate a running process.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | yes | — | Session ID |
| `force` | boolean | no | `false` | SIGKILL instead of SIGTERM |
| `grace_period` | number | no | `5` | Seconds before SIGKILL |

### `resize_pty`

Resize the PTY dimensions (pty mode only).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `session_id` | string | yes | — | Session ID |
| `rows` | integer | no | `24` | Row count |
| `cols` | integer | no | `80` | Column count |

### `get_session_info`

Get detailed info about a session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string | yes | Session ID |

Returns: `{ id, name, command, args, mode, status, exit_code, pid, created_at }`

## Usage Examples

### SSH Session

```
1. start_process(command="ssh", args=["user@host"], mode="pty")
   → { session_id: "abc123", initial_output: "user@host's password: " }

2. send_input(session_id="abc123", text="mypassword", press_enter=true)
   → { success: true }

3. read_output(session_id="abc123", timeout=5)
   → { output: "Welcome to Ubuntu...\n$ " }

4. send_and_read(session_id="abc123", text="ls -la", press_enter=true, timeout=3)
   → { output: "total 32\ndrwxr-xr-x ...\n$ " }

5. terminate_process(session_id="abc123")
   → { success: true }
```

### Python REPL

```
1. start_process(command="python3", mode="pty")
   → { session_id: "def456", initial_output: ">>> " }

2. send_and_read(session_id="def456", text="print(2 + 2)", press_enter=true)
   → { output: "4\n>>> " }
```

### Interactive Installer

```
1. start_process(command="sudo", args=["apt", "install", "some-package"], mode="pty")
   → { session_id: "ghi789", initial_output: "Do you want to continue? [Y/n] " }

2. send_input(session_id="ghi789", text="Y", press_enter=true)
   → { success: true }

3. read_output(session_id="ghi789", timeout=30)
   → { output: "Setting up some-package ...\n" }
```

## Architecture

```
MCP Server (main thread — JSON-RPC over stdio)
  ├── Session reader thread → ring buffer → agent reads
  ├── Session reader thread → ring buffer → agent reads
  └── Session reader thread → ring buffer → agent reads
```

Each session runs its own background reader thread that continuously reads process output into a ring buffer (max ~1MB). The agent consumes output at its own pace via `read_output` / `send_and_read`.

## Testing

```bash
pip install -e ".[dev]"
pytest tests/ -v
```

## License

MIT
