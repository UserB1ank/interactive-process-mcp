# Changelog

## v0.3.1 — 2026-05-03

### Improvements

- **Compact performance**: Replaced O(n²) blank-line folding loop with single-pass regex. Added early-return fast path for already-clean input — avoids all allocations on typical incremental reads.

- **Changelog corrected**: Merged never-released v0.2.0 into v0.2.1, aligning CHANGELOG with actual tags.

## v0.3.0 — 2026-05-03

### New Features

- **Output noise reduction**: `read_output` and `send_and_read` now automatically compact PTY output when `strip_ansi=true`. Progress bars (e.g. `git clone`, `wget`, `docker pull`), carriage-return overwrites, control characters, trailing whitespace, and excess blank lines are stripped — only the final visible content reaches the LLM. A typical `git clone` drops from ~11,500 bytes to ~200 bytes (98% reduction).

### Improvements

- **Multi-session agent guidelines**: CLAUDE.md now includes multi-session parallel work rules for agents — one session per task, short timeouts (≤3s), poll in rotation, clean up when done.

## v0.2.1 — 2026-05-02

### New Features

- **Go rewrite**: The entire project has been rewritten from Python to Go, replacing the pexpect-based process management with an internal SSH server architecture. This provides native PTY support, proper signal delivery, and better concurrency.

- **Multi-agent session sharing**: Multiple AI agents can now read output from the same process session simultaneously without interfering with each other. Each agent registers as an independent reader with its own cursor — no more output stealing between agents.

- **Reader lifecycle tools**: New MCP tools `register_reader` and `unregister_reader` give agents explicit control over their output stream. The `read_output` and `send_and_read` tools now accept an optional `reader_id` parameter.

- **Session cleanup**: New `delete_session` tool removes exited sessions from memory, preventing resource accumulation over long-running sessions.

- **Automated releases**: GitHub Actions workflow for building and publishing releases automatically.

### Improvements

- **Robust buffer timeout**: Output buffer reads use a goroutine-based deadline mechanism that guarantees reads return after the specified timeout, even under high contention.

- **Session parameter validation**: Input parameters (command mode, terminal dimensions, timeouts) are validated before session creation.

- **Config validation**: `Validate()` checks port range (1–65535) and non-empty `DataDir`. Default host changed from `0.0.0.0` to `127.0.0.1`.

- **Type safety**: New `SessionMode` type with `ModePTY`/`ModePipe` constants replaces raw string comparisons.

### Bug Fixes

- **Fixed buffer read deadlock**: The output buffer's read timeout could fail to wake readers in rare race conditions, causing permanent hangs. Replaced with a more robust deadline mechanism.

- **Fixed path traversal vulnerability**: Session and message IDs are now validated against a strict whitelist, preventing directory traversal attacks that could read or write arbitrary files.

- **Fixed zombie processes**: Deleting a running session now returns an error instead of silently removing it while the process continues in the background.

- **Fixed session lock contention**: Sending input to a process no longer holds a read lock during the actual write, preventing deadlocks when the stdin pipe buffer is full.

- **Fixed concurrent termination race**: Multiple termination calls on the same session are now properly serialized, and the exit code is always set by the authoritative exit goroutine.

- **Fixed session lifecycle races**: `Info()` now returns a deep copy of `ExitCode` to prevent data races. `CleanupAll` waits for all sessions to reach exited status before persisting. Reader goroutines properly clean up on terminate via a `done` channel.

- **Fixed error handling gaps**: MCP handlers now validate `mode`, `rows`, `cols`, `timeout`, and `grace_period` parameters. Shared `jsonResult`, `successResult`, and `requireSession` helpers eliminate boilerplate duplication.

- **Fixed message append race**: `Manager.Append` now uses a per-session mutex to prevent concurrent appends from corrupting the message index.

- **Fixed storage atomicity**: All JSON writes use temp-file + fsync + rename to prevent half-written files on crash.

- **Fixed SSH server robustness**: `ProcessState` nil check prevents panic on command-not-found. `started` flag uses `atomic.Bool`. Signal forwarding from client to local process now works in pipe mode. Serve and Setsize errors are logged.

- **Fixed SSH client safety**: `shellQuote` now escapes the command itself. `Close()` closes `Stdin` and returns the first non-nil error. Type assertions use comma-ok pattern.

### Breaking Changes

- `NewReader()` now returns `(int, error)` instead of `int` — callers must handle the error.

## v0.1.0 — 2026-04-27

Initial release. Python implementation with FastMCP server, pexpect-based process management, and 8 MCP tools for interactive process control.
