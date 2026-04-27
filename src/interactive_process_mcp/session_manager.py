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
