"""src/interactive_process_mcp/session.py"""
import enum
import os
import signal
import subprocess
import threading
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
        import select
        proc = self._process
        stdout_fd = proc.stdout.fileno()
        try:
            while proc.poll() is None or select.select([stdout_fd], [], [], 0.0)[0]:
                ready, _, _ = select.select([stdout_fd], [], [], 0.1)
                if ready:
                    chunk = os.read(stdout_fd, 4096)
                    if not chunk:
                        break
                    self._buffer.write(chunk.decode("utf-8", errors="replace"))
        except (OSError, ValueError):
            pass
        finally:
            remaining = proc.stdout.read()
            if remaining:
                self._buffer.write(remaining.decode("utf-8", errors="replace"))
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
            self._process.send(text)
        else:
            self._process.stdin.write(text.encode("utf-8"))
            self._process.stdin.flush()

    def read_output(self, timeout: float = 5.0, strip_ansi_flag: bool = True, max_lines: int = 0) -> str:
        output = self._buffer.read_new(timeout=timeout)
        if strip_ansi_flag:
            output = strip_ansi(output)
        if max_lines > 0:
            lines = output.split("\n")
            output = "\n".join(lines[:max_lines])
        return output

    def terminate(self, force: bool = False, grace_period: float = 5.0) -> None:
        if self.status != SessionStatus.RUNNING:
            return
        if self.mode == "pty":
            if force:
                self._process.terminate(force=True)
            else:
                self._process.terminate(force=False)
                import time
                time.sleep(grace_period)
                if self._process.isalive():
                    self._process.terminate(force=True)
        else:
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
