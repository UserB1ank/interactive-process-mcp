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
    output = s.read_output(timeout=1.0, strip_ansi_flag=True)
    assert "test line" in output
    s.close()


def test_send_and_read_pipe():
    s = Session(command="cat", mode="pipe")
    s.start()
    time.sleep(0.2)
    s.send_input("pipe test\n")
    time.sleep(0.2)
    output = s.read_output(timeout=1.0, strip_ansi_flag=True)
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
