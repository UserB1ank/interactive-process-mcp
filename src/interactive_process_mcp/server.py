"""src/interactive_process_mcp/server.py"""
import time

from mcp.server.fastmcp import FastMCP

from interactive_process_mcp.session_manager import SessionManager

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
