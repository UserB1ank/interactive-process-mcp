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
            if timeout > 0 and remaining <= 0:
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
