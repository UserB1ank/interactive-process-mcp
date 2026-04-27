"""tests/test_session_manager.py"""
import time
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
