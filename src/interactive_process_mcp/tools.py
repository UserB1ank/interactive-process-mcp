"""src/interactive_process_mcp/tools.py"""
import time

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
