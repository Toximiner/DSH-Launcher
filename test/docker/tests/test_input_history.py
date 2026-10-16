"""История введённого (↑/↓ в поле dsh): после отправки сообщение можно поднять
стрелками (свежие — сверху, повторы подряд не дублируются), Esc возвращает
недописанный черновик, история переживает перезагрузку страницы dsh (хранится
в main-процессе лаунчера и на диске — input-history.json).

Клавиши шлём реальными X-событиями (xdotool + windowfocus, как в test_keyboard):
CDP-доставка (Input.dispatchKeyEvent) в прогоне №2 непредсказуемо не доходила до
страницы — на двух дистрибутивах первый Enter не сработал."""
import json
import os
import subprocess

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import wait_for

X = {'DISPLAY': ':99'}


def x(*args):
    return subprocess.run(list(args), env={**X, 'PATH': '/usr/bin:/bin'},
                          capture_output=True, text=True, check=True).stdout


def focus_window():
    """Активировать окно лаунчера: без фокуса X-клавиши уходят «в никуда»."""
    win = x('xdotool', 'search', '--onlyvisible', '--name', 'DeepSeek Harness').split()[0]
    x('xdotool', 'windowfocus', '--sync', win)


def key(name):
    x('xdotool', 'key', name)


def field(gui):
    return gui.eval('window.__field()')


def type_text(gui, t):
    gui.eval(f'window.__type({json.dumps(t)}); 1')
    assert wait_for(lambda: field(gui) == t, 5, 0.1), f'текст набран: {field(gui)!r}'


def focus_composer(gui):
    """Фокус в поле ввода (после перезагрузки страницы он теряется)."""
    ok = gui.eval("(() => { const c = document.querySelector('[data-composer-input=\"true\"]');"
                  " if (!c) return false; c.focus(); return document.activeElement === c; })()")
    assert ok, 'композер получил фокус'


def caret_start(gui):
    """Каретка в начале поля (вход в режим истории при непустом черновике)."""
    ok = gui.eval("(() => { const c = document.querySelector('[data-composer-input=\"true\"]');"
                  " c.focus(); const r = document.createRange();"
                  " r.setStart(c.firstChild || c, 0); r.collapse(true);"
                  " const s = getSelection(); s.removeAllRanges(); s.addRange(r); return true; })()")
    assert ok, 'каретка в начале'


def history_file():
    path = os.path.join(CONF, 'input-history.json')
    return json.load(open(path))['items'] if os.path.exists(path) else None


def test_input_history(start, gui):
    launcher = start(FAKE, NOUPD, FAKE_DSH_GUI='1')
    gui.wait()
    focus_window()

    # Enter «отправляет» (поле очищается, как в dsh), текст попадает в историю
    type_text(gui, 'первое сообщение')
    key('Return')
    assert wait_for(lambda: field(gui) == '', 5, 0.1), f'после Enter поле пустое: {field(gui)!r}'
    assert wait_for(lambda: history_file() == ['первое сообщение'], 10, 0.2), f'история записана: {history_file()!r}'

    type_text(gui, 'второе сообщение')
    key('Return')
    assert wait_for(lambda: history_file() == ['второе сообщение', 'первое сообщение'], 10, 0.2), \
        f'свежие сверху: {history_file()!r}'

    # повтор подряд не дублируется
    type_text(gui, 'второе сообщение')
    key('Return')
    assert wait_for(lambda: history_file() == ['второе сообщение', 'первое сообщение'], 10, 0.2), \
        f'повтор не дублируется: {history_file()!r}'

    # ↑ — свежее, ↑↑ — старше, дальше некуда; ↓ — назад, ↓ ниже нового — пустое
    key('Up')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↑ даёт последнее: {field(gui)!r}'
    key('Up')
    assert wait_for(lambda: field(gui) == 'первое сообщение', 5, 0.1), f'↑↑ даёт предыдущее: {field(gui)!r}'
    key('Up')
    assert wait_for(lambda: field(gui) == 'первое сообщение', 5, 0.1), 'дальше истории нет'
    key('Down')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↓ возвращает к свежему: {field(gui)!r}'
    key('Down')
    assert wait_for(lambda: field(gui) == '', 5, 0.1), f'↓ ниже нового — пустое поле: {field(gui)!r}'

    # Esc: недописанный черновик возвращается, в историю не попадает
    type_text(gui, 'черновик')
    caret_start(gui)
    key('Up')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↑ из черновика: {field(gui)!r}'
    key('Escape')
    assert wait_for(lambda: field(gui) == 'черновик', 5, 0.1), f'Esc вернул черновик: {field(gui)!r}'
    assert history_file() == ['второе сообщение', 'первое сообщение'], f'черновик не записан: {history_file()!r}'

    # перезагрузка страницы dsh: история поднимается снова (впрыск при загрузке).
    # __mark со старой страницы пропадает только у нового документа: иначе
    # wait() мог поймать ещё не начавшуюся перезагрузку (флэк на debian-13).
    gui.mark()
    gui.eval('location.reload(); 1')
    assert wait_for(lambda: gui.eval("document.readyState === 'complete' && window.__mark === undefined") is True, 15, 0.2), \
        'страница перезагрузилась'
    assert wait_for(lambda: gui.eval('!!window.__dshInputHistory'), 10, 0.2), 'скрипт истории впрыснут'
    focus_window()
    focus_composer(gui)
    key('Up')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'после перезагрузки ↑ работает: {field(gui)!r}'

    launcher.term()
