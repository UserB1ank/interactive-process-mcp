"""tests/test_ansi.py"""
from interactive_process_mcp.ansi import strip_ansi


def test_strips_color_codes():
    text = "\x1b[31mred text\x1b[0m normal"
    assert strip_ansi(text) == "red text normal"


def test_strips_cursor_movement():
    text = "hello\x1b[2J\x1b[Hworld"
    assert strip_ansi(text) == "helloworld"


def test_strips_osc_title():
    text = "\x1b]0;window title\x07content"
    assert strip_ansi(text) == "content"


def test_strips_osc_title_bel_terminator():
    text = "\x1b]2;title\x1b\\data"
    assert strip_ansi(text) == "data"


def test_plain_text_unchanged():
    text = "hello world\nline 2\n"
    assert strip_ansi(text) == text


def test_empty_string():
    assert strip_ansi("") == ""


def test_mixed_sequences():
    text = "\x1b[1;32m\x1b[K\x1b[?25lhello\x1b[0m\x1b[?25h"
    assert strip_ansi(text) == "hello"


def test_two_byte_esc():
    text = "\x1b7saved\x1b8restored"
    assert strip_ansi(text) == "savedrestored"
