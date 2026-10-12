"""Клавиатура под X11 (настоящие нажатия через xdotool): одиночный Alt и смена
раскладки Shift+Alt не уводят фокус из поля ввода в строку меню; AltGr
(правый Alt) по-прежнему набирает символы третьего уровня (AltGr+Q → @ на
немецкой раскладке)."""
import subprocess

from helpers.env import FAKE, NOUPD
from helpers.procs import wait_for

X = {'DISPLAY': ':99'}


def x(*args):
    return subprocess.run(list(args), env={**X, 'PATH': '/usr/bin:/bin'}, capture_output=True, text=True, check=True).stdout


def focus_field(gui):
    """Поле ввода в GUI (фейкового) dsh, окно лаунчера — в фокусе X."""
    gui.set_body('<input id="t" autofocus>')
    gui.eval("document.getElementById('t').focus(); 1")
    win = x('xdotool', 'search', '--onlyvisible', '--name', 'DeepSeek Harness').split()[0]
    x('xdotool', 'windowfocus', '--sync', win)
    assert wait_for(lambda: gui.eval("document.activeElement && document.activeElement.id === 't'"), 5, 0.1), 'поле в фокусе'


def value(gui):
    return gui.eval("document.getElementById('t').value")


def test_keyboard_alt(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()
    x('setxkbmap', '-layout', 'us,ru', '-option', '', '-option', 'grp:alt_shift_toggle')
    focus_field(gui)

    # одиночный Alt (левый, правый) — затем «1» попадает в поле, а не в меню
    x('xdotool', 'key', 'Alt_L')
    x('xdotool', 'key', 'Alt_R')
    x('xdotool', 'key', '1')
    assert wait_for(lambda: value(gui) == '1', 5, 0.1), f'после Alt фокус в поле: {value(gui)!r}'

    # Shift+Alt — смена раскладки; цифра на обеих раскладках та же
    x('xdotool', 'key', 'shift+alt')
    x('xdotool', 'key', '2')
    assert wait_for(lambda: value(gui) == '12', 5, 0.1), f'после Shift+Alt фокус в поле: {value(gui)!r}'

    # AltGr+Q на немецкой раскладке — «@»
    x('setxkbmap', '-layout', 'de', '-option', '')
    x('xdotool', 'keydown', 'ISO_Level3_Shift', 'key', 'q', 'keyup', 'ISO_Level3_Shift')
    assert wait_for(lambda: value(gui) == '12@', 5, 0.1), f'AltGr+Q → @: {value(gui)!r}'
    launcher.term()
