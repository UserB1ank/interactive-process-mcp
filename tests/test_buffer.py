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
    buf.write("abcdefghij")  # 10 bytes, total 20 at limit
    buf.write("XYZ")         # 3 more, triggers overflow
    content = buf.read_new(timeout=0.5)
    assert "XYZ" in content


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
