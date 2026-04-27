# Interactive Process MCP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Python MCP server that wraps interactive processes with session-based read/write access for Claude Code agents.

**Architecture:** FastMCP server with multi-threaded session management. Each interactive process runs in a pexpect spawn (PTY mode) or subprocess.Popen (pipe mode), with a dedicated reader thread buffering output into a ring buffer per session. Agent interacts via 8 MCP tools: start, send, read, send_and_read, list, terminate, resize, info.

**Tech Stack:** Python 3.10+, `mcp` SDK (FastMCP), `pexpect`, `threading`, `collections.deque`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `pyproject.toml` | Project config, dependencies, entry point |
| `src/interactive_process_mcp/__init__.py` | Package init |
| `src/interactive_process_mcp/ansi.py` | ANSI escape code stripping |
| `src/interactive_process_mcp/buffer.py` | Thread-safe ring output buffer |
| `src/interactive_process_mcp/session.py` | Session class — process lifecycle, I/O |
| `src/interactive_process_mcp/session_manager.py` | Session registry — create, get, list, terminate |
| `src/interactive_process_mcp/tools.py` | MCP tool definitions and handlers |
| `src/interactive_process_mcp/server.py` | FastMCP server entry point, __main__ |
| `tests/test_ansi.py` | Tests for ANSI stripping |
| `tests/test_buffer.py` | Tests for ring buffer |
| `tests/test_session.py` | Tests for session lifecycle |
| `tests/test_session_manager.py` | Tests for session registry |
| `tests/test_tools.py` | Integration tests for MCP tools |

---

### Task 1: Project Scaffold

**Files:**
- Create: `pyproject.toml`
- Create: `src/interactive_process_mcp/__init__.py`

- [ ] **Step 1: Create pyproject.toml**

```toml
[build-system]
requires = ["setuptools>=68.0"]
build-backend = "setuptools.backends._legacy:_Backend"

[project]
name = "interactive-process-mcp"
version = "0.1.0"
description = "MCP server for managing interactive processes"
requires-python = ">=3.10"
dependencies = [
    "mcp>=1.0.0",
    "pexpect>=4.8",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
]

[project.scripts]
interactive-process-mcp = "interactive_process_mcp.server:main"

[tool.setuptools.packages.find]
where = ["src"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

- [ ] **Step 2: Create __init__.py**

```python
"""Interactive Process MCP Server."""
```

- [ ] **Step 3: Create directories**

```bash
mkdir -p src/interactive_process_mcp tests
```

- [ ] **Step 4: Install in dev mode**

```bash
pip install -e ".[dev]" --break-system-packages
```

- [ ] **Step 5: Commit**

```bash
git add pyproject.toml src/ tests/
git commit -m "chore: scaffold project structure"
```

---

### Task 2: ANSI Escape Code Stripping

**Files:**
- Create: `src/interactive_process_mcp/ansi.py`
- Create: `tests/test_ansi.py`

- [ ] **Step 1: Write tests for ANSI stripping**

```python
"""tests/test_ansi.py"""
from interactive_process_mcp.ansi import strip_ansi


def test_strips_color_codes():
    text = "\x1b[31mred text\x1b[0m normal"
    assert strip_ansi(text) == "red text normal"


def test_strips_cursor_movement():
    text = "hello\x1b[2J\x1b[Hworld"
    assert strip_ansi(text) == "helloworld"


def test_strips_osc_title():
    text = "\x1b]0;window title\x07content"
    assert strip_ansi(text) == "content"


def test_strips_osc_title_bel_terminator():
    text = "\x1b]2;title\x1b\\data"
    assert strip_ansi(text) == "data"


def test_plain_text_unchanged():
    text = "hello world\nline 2\n"
    assert strip_ansi(text) == text


def test_empty_string():
    assert strip_ansi("") == ""


def test_mixed_sequences():
    text = "\x1b[1;32m\x1b[K\x1b[?25lhello\x1b[0m\x1b[?25h"
    assert strip_ansi(text) == "hello"


def test_two_byte_esc():
    text = "\x1b7saved\x1b8restored"
    assert strip_ansi(text) == "savedrestored"
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest tests/test_ansi.py -v
```

Expected: FAIL (module not found)

- [ ] **Step 3: Implement ANSI stripping**

```python
"""src/interactive_process_mcp/ansi.py"""
import re

_ANSI_RE = re.compile(
    r'\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\].*?(?:\x1b\\|\x07))'
)


def strip_ansi(text: str) -> str:
    return _ANSI_RE.sub('', text)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest tests/test_ansi.py -v
```

Expected: all 8 tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/interactive_process_mcp/ansi.py tests/test_ansi.py
git commit -m "feat: add ANSI escape code stripping"
```

---

### Task 3: Ring Output Buffer

**Files:**
- Create: `src/interactive_process_mcp/buffer.py`
- Create: `tests/test_buffer.py`

- [ ] **Step 1: Write tests for ring buffer**

```python
"""tests/test_buffer.py"""
import time
import threading
from interactive_process_mcp.buffer import OutputBuffer


def test_write_and_read():
    buf = OutputBuffer(max_bytes=1024)
    buf.write("hello\n")
    assert buf.read_new() == "hello\n"


def test_read_returns_new_content_only():
    buf = OutputBuffer(max_bytes=1024)
    buf.write("chunk1")
    assert buf.read_new() == "chunk1"
    buf.write("chunk2")
    assert buf.read_new() == "chunk2"


def test_read_empty_when_no_new_content():
    buf = OutputBuffer(max_bytes=1024)
    assert buf.read_new() == ""


def test_overflow_drops_oldest():
    buf = OutputBuffer(max_bytes=20)
    buf.write("0123456789")  # 10 bytes
    buf.write("abcdefghij")  # 10 bytes, total 20 — at limit
    buf.write("XYZ")         # 3 more, triggers overflow
    content = buf.read_new()
    # oldest content dropped, should contain the later content
    assert "XYZ" in content
    assert len(content) <= 23  # total written


def test_wait_for_content():
    buf = OutputBuffer(max_bytes=1024)

    def delayed_write():
        time.sleep(0.1)
        buf.write("delayed")

    t = threading.Thread(target=delayed_write)
    t.start()
    result = buf.read_new(timeout=1.0)
    t.join()
    assert result == "delayed"


def test_wait_timeout_returns_empty():
    buf = OutputBuffer(max_bytes=1024)
    start = time.monotonic()
    result = buf.read_new(timeout=0.1)
    elapsed = time.monotonic() - start
    assert result == ""
    assert elapsed >= 0.1


def test_has_more():
    buf = OutputBuffer(max_bytes=1024)
    buf.write("short")
    has_more = buf.read_new()
    assert has_more == "short"


def test_close_unblocks_waiting_reader():
    buf = OutputBuffer(max_bytes=1024)

    def delayed_close():
        time.sleep(0.1)
        buf.close()

    t = threading.Thread(target=delayed_close)
    t.start()
    result = buf.read_new(timeout=10.0)
    t.join()
    assert result == ""


def test_stats():
    buf = OutputBuffer(max_bytes=1024)
    buf.write("hello\nworld\n")
    buf.read_new()
    stats = buf.stats()
    assert stats["total_bytes_written"] == 12
    assert stats["total_bytes_read"] == 12
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest tests/test_buffer.py -v
```

Expected: FAIL (module not found)

- [ ] **Step 3: Implement ring buffer**

```python
"""src/interactive_process_mcp/buffer.py"""
import threading
import time
from collections import deque


class OutputBuffer:
    def __init__(self, max_bytes: int = 1024 * 1024):
        self._max_bytes = max_bytes
        self._chunks: deque[str] = deque()
        self._total_bytes = 0
        self._lock = threading.Lock()
        self._new_data = threading.Event()
        self._closed = False
        self._read_pos = 0
        self._write_pos = 0
        self._stats = {"total_bytes_written": 0, "total_bytes_read": 0}

    def write(self, data: str) -> None:
        if not data:
            return
        with self._lock:
            self._chunks.append(data)
            self._total_bytes += len(data)
            self._write_pos += 1
            self._stats["total_bytes_written"] += len(data)
            while self._total_bytes > self._max_bytes and len(self._chunks) > 1:
                oldest = self._chunks.popleft()
                self._total_bytes -= len(oldest)
            self._new_data.set()

    def read_new(self, timeout: float = 0) -> str:
        deadline = time.monotonic() + timeout if timeout > 0 else 0
        while True:
            with self._lock:
                if self._read_pos < self._write_pos:
                    break
                if self._closed:
                    return ""
            remaining = deadline - time.monotonic() if deadline else 0
            if remaining <= 0 and timeout > 0:
                return ""
            if timeout > 0:
                self._new_data.wait(timeout=min(remaining, 0.1))
            else:
                return ""
        with self._lock:
            parts = []
            while self._chunks and self._read_pos < self._write_pos:
                parts.append(self._chunks.popleft())
                self._total_bytes -= len(parts[-1])
                self._read_pos += 1
            result = "".join(parts)
            self._stats["total_bytes_read"] += len(result)
            self._new_data.clear()
            return result

    def close(self) -> None:
        with self._lock:
            self._closed = True
        self._new_data.set()

    def stats(self) -> dict:
        with self._lock:
            return dict(self._stats)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest tests/test_buffer.py -v
```

Expected: all 9 tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/interactive_process_mcp/buffer.py tests/test_buffer.py
git commit -m "feat: add thread-safe ring output buffer"
```

---

### Task 4: Session — Process Lifecycle

**Files:**
- Create: `src/interactive_process_mcp/session.py`
- Create: `tests/test_session.py`

- [ ] **Step 1: Write tests for session lifecycle**

```python
"""tests/test_session.py"""
import time
import pytest
from interactive_process_mcp.session import Session, SessionStatus


def test_start_pty_session():
    s = Session(command="echo", args=["hello"], mode="pty")
    s.start()
    assert s.status == SessionStatus.RUNNING
    assert s.pid is not None
    time.sleep(0.3)
    assert s.status == SessionStatus.EXITED
    s.close()


def test_start_pipe_session():
    s = Session(command="echo", args=["world"], mode="pipe")
    s.start()
    assert s.status == SessionStatus.RUNNING
    time.sleep(0.3)
    assert s.status == SessionStatus.EXITED
    s.close()


def test_send_and_read_pty():
    s = Session(command="cat", mode="pty")
    s.start()
    time.sleep(0.2)
    s.send_input("test line\n")
    time.sleep(0.2)
    output = s.read_output(timeout=1.0, strip_ansi=True)
    assert "test line" in output
    s.close()


def test_send_and_read_pipe():
    s = Session(command="cat", mode="pipe")
    s.start()
    time.sleep(0.2)
    s.send_input("pipe test\n")
    time.sleep(0.2)
    output = s.read_output(timeout=1.0, strip_ansi=True)
    assert "pipe test" in output
    s.close()


def test_send_with_press_enter():
    s = Session(command="cat", mode="pipe")
    s.start()
    time.sleep(0.2)
    s.send_input("hello", press_enter=True)
    time.sleep(0.2)
    output = s.read_output(timeout=1.0)
    assert "hello" in output
    s.close()


def test_send_to_exited_process():
    s = Session(command="echo", args=["done"], mode="pipe")
    s.start()
    time.sleep(0.5)
    assert s.status == SessionStatus.EXITED
    with pytest.raises(RuntimeError, match="exited"):
        s.send_input("nope")
    s.close()


def test_terminate_process():
    s = Session(command="sleep", args=["60"], mode="pipe")
    s.start()
    assert s.status == SessionStatus.RUNNING
    s.terminate(grace_period=1.0)
    assert s.status == SessionStatus.EXITED
    s.close()


def test_terminate_force():
    s = Session(command="sleep", args=["60"], mode="pipe")
    s.start()
    s.terminate(force=True)
    assert s.status == SessionStatus.EXITED
    s.close()


def test_resize_pty():
    s = Session(command="cat", mode="pty", rows=24, cols=80)
    s.start()
    s.resize_pty(rows=50, cols=120)
    s.close()


def test_resize_pipe_raises():
    s = Session(command="cat", mode="pipe")
    s.start()
    with pytest.raises(RuntimeError, match="PTY"):
        s.resize_pty(rows=50, cols=120)
    s.close()


def test_session_info():
    s = Session(command="echo", args=["hi"], mode="pty", name="test-echo")
    s.start()
    info = s.info()
    assert info["name"] == "test-echo"
    assert info["command"] == "echo"
    assert info["mode"] == "pty"
    assert info["status"] in ("running", "exited")
    s.close()


def test_read_from_exited_process_returns_buffered():
    s = Session(command="echo", args=["buffered output"], mode="pipe")
    s.start()
    time.sleep(0.5)
    assert s.status == SessionStatus.EXITED
    output = s.read_output(timeout=0.5)
    assert "buffered output" in output
    s.close()


def test_start_fails_on_bad_command():
    s = Session(command="/nonexistent/command", mode="pipe")
    with pytest.raises(RuntimeError, match="start"):
        s.start()
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest tests/test_session.py -v
```

Expected: FAIL (module not found)

- [ ] **Step 3: Implement Session class**

```python
"""src/interactive_process_mcp/session.py"""
import enum
import os
import signal
import subprocess
import threading
import time
import uuid
from datetime import datetime, timezone

import pexpect

from interactive_process_mcp.ansi import strip_ansi
from interactive_process_mcp.buffer import OutputBuffer


class SessionStatus(str, enum.Enum):
    RUNNING = "running"
    EXITED = "exited"
    ERROR = "error"


class Session:
    def __init__(
        self,
        command: str,
        args: list[str] | None = None,
        mode: str = "pty",
        name: str | None = None,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
        timeout: float = 10.0,
        rows: int = 24,
        cols: int = 80,
    ):
        self.id = uuid.uuid4().hex[:12]
        self.name = name or f"session-{self.id}"
        self.command = command
        self.args = args or []
        self.mode = mode
        self.cwd = cwd
        self.env = env
        self._timeout = timeout
        self._rows = rows
        self._cols = cols
        self.status = SessionStatus.RUNNING
        self.exit_code: int | None = None
        self.pid: int | None = None
        self.created_at = datetime.now(timezone.utc).isoformat()
        self._process: pexpect.spawn | subprocess.Popen | None = None
        self._buffer = OutputBuffer(max_bytes=1024 * 1024)
        self._reader_thread: threading.Thread | None = None

    def start(self) -> None:
        try:
            if self.mode == "pty":
                self._start_pty()
            else:
                self._start_pipe()
        except Exception as e:
            self.status = SessionStatus.ERROR
            raise RuntimeError(f"Failed to start process: {e}") from e

    def _start_pty(self) -> None:
        cmd = " ".join([self.command] + self.args) if self.args else self.command
        self._process = pexpect.spawn(
            cmd,
            cwd=self.cwd,
            env=self.env or os.environ,
            timeout=self._timeout,
            encoding="utf-8",
            codec_errors="replace",
            dimensions=(self._rows, self._cols),
        )
        self.pid = self._process.pid
        self._start_reader_thread(self._read_loop_pty)

    def _start_pipe(self) -> None:
        self._process = subprocess.Popen(
            [self.command] + self.args,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            cwd=self.cwd,
            env=self.env or os.environ,
        )
        self.pid = self._process.pid
        self._start_reader_thread(self._read_loop_pipe)

    def _start_reader_thread(self, target) -> None:
        self._reader_thread = threading.Thread(target=target, daemon=True)
        self._reader_thread.start()

    def _read_loop_pty(self) -> None:
        proc = self._process
        assert isinstance(proc, pexpect.spawn)
        while proc.isalive():
            try:
                chunk = proc.read_nonblocking(size=4096, timeout=0.1)
                if chunk:
                    self._buffer.write(chunk)
            except pexpect.TIMEOUT:
                continue
            except pexpect.EOF:
                break
        remaining = proc.before or ""
        if remaining:
            self._buffer.write(remaining)
        proc.close()
        self.exit_code = proc.exitstatus
        self.status = SessionStatus.EXITED
        self._buffer.close()

    def _read_loop_pipe(self) -> None:
        proc = self._process
        assert isinstance(proc, subprocess.Popen)
        assert proc.stdout is not None
        try:
            while True:
                chunk = proc.stdout.read(4096)
                if not chunk:
                    break
                self._buffer.write(chunk.decode("utf-8", errors="replace"))
        except Exception:
            pass
        finally:
            proc.wait()
            self.exit_code = proc.returncode
            self.status = SessionStatus.EXITED
            self._buffer.close()

    def send_input(self, text: str, press_enter: bool = False) -> None:
        if self.status != SessionStatus.RUNNING:
            raise RuntimeError(f"Process has {self.status.value}, cannot send input")
        if press_enter:
            text += os.linesep
        if self.mode == "pty":
            assert isinstance(self._process, pexpect.spawn)
            self._process.send(text)
        else:
            assert isinstance(self._process, subprocess.Popen)
            assert self._process.stdin is not None
            self._process.stdin.write(text.encode("utf-8"))
            self._process.stdin.flush()

    def read_output(self, timeout: float = 5.0, strip_ansi_flag: bool = True, max_lines: int = 0) -> str:
        output = self._buffer.read_new(timeout=timeout)
        if strip_ansi_flag:
            from interactive_process_mcp.ansi import strip_ansi as _strip
            output = _strip(output)
        if max_lines > 0:
            lines = output.split("\n")
            output = "\n".join(lines[:max_lines])
        return output

    def terminate(self, force: bool = False, grace_period: float = 5.0) -> None:
        if self.status != SessionStatus.RUNNING:
            return
        if self.mode == "pty":
            assert isinstance(self._process, pexpect.spawn)
            if force:
                self._process.terminate(force=True)
            else:
                self._process.terminate(force=False)
                time.sleep(grace_period)
                if self._process.isalive():
                    self._process.terminate(force=True)
        else:
            assert isinstance(self._process, subprocess.Popen)
            if force:
                self._process.kill()
            else:
                self._process.send_signal(signal.SIGTERM)
                try:
                    self._process.wait(timeout=grace_period)
                except subprocess.TimeoutExpired:
                    self._process.kill()
        if self._reader_thread:
            self._reader_thread.join(timeout=2.0)
        self.status = SessionStatus.EXITED

    def resize_pty(self, rows: int, cols: int) -> None:
        if self.mode != "pty":
            raise RuntimeError("PTY resize only available in pty mode")
        if self.status != SessionStatus.RUNNING:
            raise RuntimeError("Process not running")
        assert isinstance(self._process, pexpect.spawn)
        self._process.setwinsize(rows, cols)
        self._rows = rows
        self._cols = cols

    def info(self) -> dict:
        return {
            "id": self.id,
            "name": self.name,
            "command": self.command,
            "args": self.args,
            "mode": self.mode,
            "status": self.status.value,
            "exit_code": self.exit_code,
            "pid": self.pid,
            "created_at": self.created_at,
        }

    def close(self) -> None:
        if self.status == SessionStatus.RUNNING:
            self.terminate(force=True)
        self._buffer.close()
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest tests/test_session.py -v
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/interactive_process_mcp/session.py tests/test_session.py
git commit -m "feat: add Session class with PTY and pipe process lifecycle"
```

---

### Task 5: Session Manager

**Files:**
- Create: `src/interactive_process_mcp/session_manager.py`
- Create: `tests/test_session_manager.py`

- [ ] **Step 1: Write tests for session manager**

```python
"""tests/test_session_manager.py"""
import time
import pytest
from interactive_process_mcp.session_manager import SessionManager


def test_create_session():
    mgr = SessionManager()
    session_id = mgr.create(command="echo", args=["hello"], mode="pty")
    assert session_id is not None
    time.sleep(0.3)
    session = mgr.get(session_id)
    assert session is not None
    assert session.command == "echo"


def test_get_nonexistent():
    mgr = SessionManager()
    assert mgr.get("nope") is None


def test_list_sessions():
    mgr = SessionManager()
    id1 = mgr.create(command="sleep", args=["60"], mode="pipe", name="s1")
    id2 = mgr.create(command="sleep", args=["60"], mode="pipe", name="s2")
    sessions = mgr.list_all()
    assert len(sessions) == 2
    names = {s["name"] for s in sessions}
    assert names == {"s1", "s2"}
    mgr.terminate(id1, force=True)
    mgr.terminate(id2, force=True)


def test_terminate_session():
    mgr = SessionManager()
    sid = mgr.create(command="sleep", args=["60"], mode="pipe")
    assert mgr.get(sid).status.value in ("running", "exited")
    mgr.terminate(sid, force=True)
    assert mgr.get(sid).status.value == "exited"


def test_terminate_nonexistent():
    mgr = SessionManager()
    mgr.terminate("nope", force=True)


def test_cleanup_all():
    mgr = SessionManager()
    mgr.create(command="sleep", args=["60"], mode="pipe")
    mgr.create(command="sleep", args=["60"], mode="pipe")
    assert len(mgr.list_all()) == 2
    mgr.cleanup_all(force=True)
    for s in mgr.list_all():
        assert s["status"] == "exited"
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest tests/test_session_manager.py -v
```

Expected: FAIL (module not found)

- [ ] **Step 3: Implement session manager**

```python
"""src/interactive_process_mcp/session_manager.py"""
import threading

from interactive_process_mcp.session import Session


class SessionManager:
    def __init__(self):
        self._sessions: dict[str, Session] = {}
        self._lock = threading.Lock()

    def create(self, command: str, **kwargs) -> str:
        session = Session(command=command, **kwargs)
        session.start()
        with self._lock:
            self._sessions[session.id] = session
        return session.id

    def get(self, session_id: str) -> Session | None:
        with self._lock:
            return self._sessions.get(session_id)

    def list_all(self) -> list[dict]:
        with self._lock:
            return [s.info() for s in self._sessions.values()]

    def terminate(self, session_id: str, force: bool = False, grace_period: float = 5.0) -> None:
        with self._lock:
            session = self._sessions.get(session_id)
        if session:
            session.terminate(force=force, grace_period=grace_period)

    def cleanup_all(self, force: bool = True) -> None:
        with self._lock:
            sessions = list(self._sessions.values())
        for s in sessions:
            s.terminate(force=force)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest tests/test_session_manager.py -v
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/interactive_process_mcp/session_manager.py tests/test_session_manager.py
git commit -m "feat: add SessionManager registry"
```

---

### Task 6: MCP Tools

**Files:**
- Create: `src/interactive_process_mcp/tools.py`
- Create: `tests/test_tools.py`

- [ ] **Step 1: Write integration tests for MCP tools**

```python
"""tests/test_tools.py"""
import time
import pytest
from interactive_process_mcp.tools import create_tools


@pytest.fixture
def tools():
    return create_tools()


def _call(tools, name: str, arguments: dict | None = None):
    for tool_name, handler in tools:
        if tool_name == name:
            return handler(arguments or {})
    raise ValueError(f"Tool {name} not found")


def test_start_and_read(tools):
    result = _call(tools, "start_process", {"command": "echo", "args": ["hello"], "mode": "pipe"})
    assert "session_id" in result
    assert "pid" in result
    time.sleep(0.3)
    output = _call(tools, "read_output", {"session_id": result["session_id"]})
    assert "hello" in output["output"]
    _call(tools, "terminate_process", {"session_id": result["session_id"]})


def test_send_and_read(tools):
    result = _call(tools, "start_process", {"command": "cat", "mode": "pipe"})
    sid = result["session_id"]
    time.sleep(0.2)
    output = _call(tools, "send_and_read", {
        "session_id": sid,
        "text": "ping\n",
        "timeout": 1.0,
    })
    assert "ping" in output["output"]
    _call(tools, "terminate_process", {"session_id": sid, "force": True})


def test_list_sessions(tools):
    r1 = _call(tools, "start_process", {"command": "sleep", "args": ["60"], "mode": "pipe"})
    r2 = _call(tools, "start_process", {"command": "sleep", "args": ["60"], "mode": "pipe"})
    result = _call(tools, "list_sessions", {})
    assert len(result["sessions"]) >= 2
    _call(tools, "terminate_process", {"session_id": r1["session_id"], "force": True})
    _call(tools, "terminate_process", {"session_id": r2["session_id"], "force": True})


def test_get_session_info(tools):
    r = _call(tools, "start_process", {"command": "cat", "mode": "pty", "name": "my-cat"})
    info = _call(tools, "get_session_info", {"session_id": r["session_id"]})
    assert info["name"] == "my-cat"
    assert info["mode"] == "pty"
    _call(tools, "terminate_process", {"session_id": r["session_id"], "force": True})


def test_terminate(tools):
    r = _call(tools, "start_process", {"command": "sleep", "args": ["60"], "mode": "pipe"})
    result = _call(tools, "terminate_process", {"session_id": r["session_id"], "force": True})
    assert result["success"] is True


def test_session_not_found(tools):
    with pytest.raises(ValueError, match="not found"):
        _call(tools, "read_output", {"session_id": "nonexistent"})


def test_resize_pty(tools):
    r = _call(tools, "start_process", {"command": "cat", "mode": "pty"})
    result = _call(tools, "resize_pty", {
        "session_id": r["session_id"],
        "rows": 50,
        "cols": 200,
    })
    assert result["success"] is True
    _call(tools, "terminate_process", {"session_id": r["session_id"], "force": True})


def test_start_with_env(tools):
    result = _call(tools, "start_process", {
        "command": "env",
        "mode": "pipe",
        "env": {"MY_VAR": "test_value"},
    })
    time.sleep(0.3)
    output = _call(tools, "read_output", {"session_id": result["session_id"], "timeout": 1.0})
    assert "test_value" in output["output"]
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest tests/test_tools.py -v
```

Expected: FAIL (module not found)

- [ ] **Step 3: Implement MCP tools**

```python
"""src/interactive_process_mcp/tools.py"""
from interactive_process_mcp.session_manager import SessionManager


def create_tools() -> list[tuple[str, callable]]:
    mgr = SessionManager()

    def _get_session(session_id: str):
        s = mgr.get(session_id)
        if not s:
            raise ValueError(f"Session '{session_id}' not found")
        return s

    def start_process(args: dict) -> dict:
        session_id = mgr.create(
            command=args["command"],
            args=args.get("args", []),
            mode=args.get("mode", "pty"),
            name=args.get("name"),
            cwd=args.get("cwd"),
            env=args.get("env"),
            timeout=args.get("timeout", 10),
            rows=args.get("rows", 24),
            cols=args.get("cols", 80),
        )
        session = mgr.get(session_id)
        import time
        time.sleep(0.1)
        initial = session.read_output(timeout=0.5, strip_ansi_flag=True)
        return {
            "session_id": session_id,
            "pid": session.pid,
            "initial_output": initial,
        }

    def send_input(args: dict) -> dict:
        session = _get_session(args["session_id"])
        session.send_input(args["text"], press_enter=args.get("press_enter", False))
        return {"success": True}

    def read_output(args: dict) -> dict:
        session = _get_session(args["session_id"])
        output = session.read_output(
            timeout=args.get("timeout", 5.0),
            strip_ansi_flag=args.get("strip_ansi", True),
            max_lines=args.get("max_lines", 0),
        )
        return {
            "output": output,
            "has_more": False,
            "lines_returned": output.count("\n"),
            "bytes_returned": len(output.encode("utf-8")),
        }

    def send_and_read(args: dict) -> dict:
        session = _get_session(args["session_id"])
        session.send_input(args["text"], press_enter=args.get("press_enter", False))
        import time
        time.sleep(0.1)
        output = session.read_output(
            timeout=args.get("timeout", 5.0),
            strip_ansi_flag=args.get("strip_ansi", True),
            max_lines=args.get("max_lines", 0),
        )
        return {
            "output": output,
            "has_more": False,
            "lines_returned": output.count("\n"),
            "bytes_returned": len(output.encode("utf-8")),
        }

    def list_sessions(args: dict) -> dict:
        return {"sessions": mgr.list_all()}

    def terminate_process(args: dict) -> dict:
        mgr.terminate(
            args["session_id"],
            force=args.get("force", False),
            grace_period=args.get("grace_period", 5.0),
        )
        return {"success": True}

    def resize_pty(args: dict) -> dict:
        session = _get_session(args["session_id"])
        session.resize_pty(rows=args["rows"], cols=args["cols"])
        return {"success": True}

    def get_session_info(args: dict) -> dict:
        session = _get_session(args["session_id"])
        return session.info()

    return [
        ("start_process", start_process),
        ("send_input", send_input),
        ("read_output", read_output),
        ("send_and_read", send_and_read),
        ("list_sessions", list_sessions),
        ("terminate_process", terminate_process),
        ("resize_pty", resize_pty),
        ("get_session_info", get_session_info),
    ]
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest tests/test_tools.py -v
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/interactive_process_mcp/tools.py tests/test_tools.py
git commit -m "feat: add MCP tool handlers for all 8 tools"
```

---

### Task 7: FastMCP Server Entry Point

**Files:**
- Create: `src/interactive_process_mcp/server.py`

- [ ] **Step 1: Implement FastMCP server**

```python
"""src/interactive_process_mcp/server.py"""
from mcp.server.fastmcp import FastMCP

from interactive_process_mcp.session_manager import SessionManager
from interactive_process_mcp.session import Session

mcp = FastMCP("interactive-process")
_mgr = SessionManager()


@mcp.tool()
def start_process(
    command: str,
    args: list[str] | None = None,
    mode: str = "pty",
    name: str | None = None,
    cwd: str | None = None,
    env: dict[str, str] | None = None,
    timeout: float = 10.0,
    rows: int = 24,
    cols: int = 80,
) -> dict:
    """Start an interactive process and return its session info.

    Args:
        command: The command to execute.
        args: Command arguments.
        mode: I/O mode — "pty" (pseudo-terminal) or "pipe". Default "pty".
        name: Optional human-readable session name.
        cwd: Working directory for the process.
        env: Environment variables (dict of string key-value pairs).
        timeout: Process startup timeout in seconds.
        rows: PTY row count (pty mode only).
        cols: PTY column count (pty mode only).
    """
    session_id = _mgr.create(
        command=command,
        args=args or [],
        mode=mode,
        name=name,
        cwd=cwd,
        env=env,
        timeout=timeout,
        rows=rows,
        cols=cols,
    )
    import time
    time.sleep(0.1)
    session = _mgr.get(session_id)
    initial = session.read_output(timeout=0.5, strip_ansi_flag=True)
    return {
        "session_id": session_id,
        "pid": session.pid,
        "initial_output": initial,
    }


@mcp.tool()
def send_input(
    session_id: str,
    text: str,
    press_enter: bool = False,
) -> dict:
    """Send text input to a running interactive process.

    Args:
        session_id: The session ID returned by start_process.
        text: Text to send to the process stdin.
        press_enter: Whether to append a newline after the text.
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    try:
        session.send_input(text, press_enter=press_enter)
        return {"success": True}
    except RuntimeError as e:
        return {"error": str(e)}


@mcp.tool()
def read_output(
    session_id: str,
    strip_ansi: bool = True,
    timeout: float = 5.0,
    max_lines: int = 0,
) -> dict:
    """Read new output from an interactive process since last read.

    If no new output is available, waits up to timeout seconds.
    Returns empty output on timeout (not an error).

    Args:
        session_id: The session ID returned by start_process.
        strip_ansi: Remove ANSI escape codes from output. Default True.
        timeout: Seconds to wait for new output. Default 5.
        max_lines: Max lines to return (0 = unlimited).
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    output = session.read_output(
        timeout=timeout,
        strip_ansi_flag=strip_ansi,
        max_lines=max_lines,
    )
    return {
        "output": output,
        "has_more": False,
        "lines_returned": output.count("\n"),
        "bytes_returned": len(output.encode("utf-8")),
    }


@mcp.tool()
def send_and_read(
    session_id: str,
    text: str,
    press_enter: bool = False,
    strip_ansi: bool = True,
    timeout: float = 5.0,
    max_lines: int = 0,
) -> dict:
    """Send input to a process and immediately read its response.

    Atomic operation: sends text, waits briefly, then reads new output.

    Args:
        session_id: The session ID returned by start_process.
        text: Text to send.
        press_enter: Append newline after text.
        strip_ansi: Remove ANSI escape codes. Default True.
        timeout: Seconds to wait for response. Default 5.
        max_lines: Max lines to return (0 = unlimited).
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    try:
        session.send_input(text, press_enter=press_enter)
    except RuntimeError as e:
        return {"error": str(e)}
    import time
    time.sleep(0.1)
    output = session.read_output(
        timeout=timeout,
        strip_ansi_flag=strip_ansi,
        max_lines=max_lines,
    )
    return {
        "output": output,
        "has_more": False,
        "lines_returned": output.count("\n"),
        "bytes_returned": len(output.encode("utf-8")),
    }


@mcp.tool()
def list_sessions() -> dict:
    """List all interactive process sessions."""
    return {"sessions": _mgr.list_all()}


@mcp.tool()
def terminate_process(
    session_id: str,
    force: bool = False,
    grace_period: float = 5.0,
) -> dict:
    """Terminate an interactive process.

    Args:
        session_id: The session ID to terminate.
        force: Use SIGKILL instead of SIGTERM. Default False.
        grace_period: Seconds to wait after SIGTERM before SIGKILL. Default 5.
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    session.terminate(force=force, grace_period=grace_period)
    return {"success": True}


@mcp.tool()
def resize_pty(
    session_id: str,
    rows: int = 24,
    cols: int = 80,
) -> dict:
    """Resize the PTY terminal dimensions for a session.

    Only works in pty mode.

    Args:
        session_id: The session ID.
        rows: New row count.
        cols: New column count.
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    try:
        session.resize_pty(rows=rows, cols=cols)
        return {"success": True}
    except RuntimeError as e:
        return {"error": str(e)}


@mcp.tool()
def get_session_info(session_id: str) -> dict:
    """Get detailed information about a session.

    Args:
        session_id: The session ID to query.
    """
    session = _mgr.get(session_id)
    if not session:
        return {"error": f"Session '{session_id}' not found"}
    return session.info()


def main():
    mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Smoke test the server starts**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}' | timeout 3 python -m interactive_process_mcp 2>/dev/null | head -1
```

Expected: JSON response with server info (may timeout, that's OK — confirms it starts and reads stdin)

- [ ] **Step 3: Commit**

```bash
git add src/interactive_process_mcp/server.py
git commit -m "feat: add FastMCP server with all 8 tools"
```

---

### Task 8: Final Validation

**Files:**
- All test files

- [ ] **Step 1: Run full test suite**

```bash
pytest tests/ -v
```

Expected: all tests PASS

- [ ] **Step 2: Run a real interactive test with bash**

Manually test with the MCP server:

```bash
# Start the server, then test via JSON-RPC:
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"start_process","arguments":{"command":"bash","mode":"pty","name":"test-bash"}}}' | timeout 5 python -m interactive_process_mcp 2>/dev/null
```

Expected: returns session_id and pid

- [ ] **Step 3: Verify Claude Code integration config**

Confirm `pyproject.toml` entry point `interactive-process-mcp = "interactive_process_mcp.server:main"` works:

```bash
pip install -e . --break-system-packages && which interactive-process-mcp
```

Expected: command is available

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: final validation and cleanup"
```
