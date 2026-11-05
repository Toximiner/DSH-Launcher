"""Keyboard under X11 (real key presses via xdotool): a lone Alt and the
Shift+Alt layout switch do not move focus out of the input field into the
menu bar; AltGr (right Alt) still types third-level characters (AltGr+Q → @
on the German layout)."""
import subprocess

from helpers.env import FAKE, NOUPD
from helpers.procs import wait_for

X = {'DISPLAY': ':99'}


def x(*args):
    return subprocess.run(list(args), env={**X, 'PATH': '/usr/bin:/bin'}, capture_output=True, text=True, check=True).stdout


def focus_field(gui):
    """The input field in the (fake) dsh GUI, the launcher window — X-focused."""
    gui.set_body('<input id="t" autofocus>')
    gui.eval("document.getElementById('t').focus(); 1")
    win = x('xdotool', 'search', '--onlyvisible', '--name', 'DeepSeek Harness').split()[0]
    x('xdotool', 'windowfocus', '--sync', win)
    assert wait_for(lambda: gui.eval("document.activeElement && document.activeElement.id === 't'"), 5, 0.1), 'field in focus'


def value(gui):
    return gui.eval("document.getElementById('t').value")


def test_keyboard_alt(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()
    x('setxkbmap', '-layout', 'us,ru', '-option', '', '-option', 'grp:alt_shift_toggle')
    focus_field(gui)

    # a lone Alt (left, right) — then «1» lands in the field, not in the menu
    x('xdotool', 'key', 'Alt_L')
    x('xdotool', 'key', 'Alt_R')
    x('xdotool', 'key', '1')
    assert wait_for(lambda: value(gui) == '1', 5, 0.1), f'after Alt focus is in the field: {value(gui)!r}'

    # Shift+Alt — layout switch; the digit is the same on both layouts
    x('xdotool', 'key', 'shift+alt')
    x('xdotool', 'key', '2')
    assert wait_for(lambda: value(gui) == '12', 5, 0.1), f'after Shift+Alt focus is in the field: {value(gui)!r}'

    # AltGr+Q on the German layout — «@»
    x('setxkbmap', '-layout', 'de', '-option', '')
    x('xdotool', 'keydown', 'ISO_Level3_Shift', 'key', 'q', 'keyup', 'ISO_Level3_Shift')
    assert wait_for(lambda: value(gui) == '12@', 5, 0.1), f'AltGr+Q → @: {value(gui)!r}'
    launcher.term()
