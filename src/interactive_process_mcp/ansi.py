"""src/interactive_process_mcp/ansi.py"""
import re

_ANSI_RE = re.compile(
    r'\x1b(?:'
    r'[\[(][0-?]*[ -/]*[@-~]'   # CSI sequences: ESC [ ... final_byte
    r'|\].*?(?:\x1b\\|\x07)'    # OSC sequences: ESC ] ... ST/BEL
    r'|[()][Bb0UK]'              # Character set: ESC ( B etc.
    r'|[ -/]*[0-~]'             # 2-byte: ESC + one final byte (covers ESC 7, ESC 8, ESC M, etc.)
    r')'
)


def strip_ansi(text: str) -> str:
    return _ANSI_RE.sub('', text)
