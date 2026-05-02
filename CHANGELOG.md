# Changelog

## v0.2.0 — 2026-05-02

### New Features

- **Multi-agent session sharing**: Multiple AI agents can now read output from the same process session simultaneously without interfering with each other. Each agent registers as an independent reader with its own cursor — no more output stealing between agents.

- **Reader lifecycle tools**: New MCP tools `register_reader` and `unregister_reader` give agents explicit control over their output stream. The `read_output` and `send_and_read` tools now accept an optional `reader_id` parameter.

- **Session cleanup**: New `delete_session` tool removes exited sessions from memory, preventing resource accumulation over long-running sessions.

- **Automated releases**: GitHub Actions workflow for building and publishing releases automatically.

### Improvements

- **Go rewrite**: The entire project has been rewritten from Python to Go, replacing the pexpect-based process management with an internal SSH server architecture. This provides native PTY support, proper signal delivery, and better concurrency.

- **Robust buffer timeout**: Output buffer reads use a goroutine-based deadline mechanism that guarantees reads return after the specified timeout, even under high contention.

- **Session parameter validation**: Input parameters (command mode, terminal dimensions, timeouts) are validated before session creation.

### Bug Fixes

- **Fixed buffer read deadlock**: The output buffer's read timeout could fail to wake readers in rare race conditions, causing permanent hangs. Replaced with a more robust deadline mechanism.

- **Fixed path traversal vulnerability**: Session and message IDs are now validated against a strict whitelist, preventing directory traversal attacks that could read or write arbitrary files.

- **Fixed zombie processes**: Deleting a running session now returns an error instead of silently removing it while the process continues in the background.

- **Fixed session lock contention**: Sending input to a process no longer holds a read lock during the actual write, preventing deadlocks when the stdin pipe buffer is full.

- **Fixed concurrent termination race**: Multiple termination calls on the same session are now properly serialized, and the exit code is always set by the authoritative exit goroutine.

### Breaking Changes

- `NewReader()` now returns `(int, error)` instead of `int` — callers must handle the error.

## v0.1.0 — 2026-04-27

Initial release. Python implementation with FastMCP server, pexpect-based process management, and 8 MCP tools for interactive process control.
