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
    assert "hello" in result["initial_output"]


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
    assert "test_value" in result["initial_output"]
