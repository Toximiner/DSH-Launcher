"""Input history (↑/↓ in the dsh field): after sending, the message can be recalled
with the arrows (newest first, consecutive repeats are not duplicated), Esc restores
the unfinished draft, the history survives a reload of the dsh page (stored in the
launcher's main process and on disk — input-history.json).

Keys are sent as real X events (xdotool + windowfocus, as in test_keyboard):
CDP delivery (Input.dispatchKeyEvent) in run №2 unpredictably did not reach the
page — on two distros the first Enter did not work."""
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
    """Activate the launcher window: without focus X keys go «nowhere»."""
    wins = x('xdotool', 'search', '--onlyvisible', '--name', 'DeepSeek Harness|fake dsh gui').split()
    # print all matches: in a failing test's report it is visible how many
    # windows there were and which one kept focus (run №4 diagnostics)
    print('X windows:', [(w, x('xdotool', 'getwindowname', w).strip()) for w in wins], flush=True)
    x('xdotool', 'windowfocus', '--sync', wins[0])


def key(name):
    x('xdotool', 'key', name)


def ensure_key_counter(gui):
    """A keydown counter on the page: proof that the X key arrived."""
    gui.eval('window.__keys_n = window.__keys_n || 0;'
             ' if (!window.__kch) { window.__kch = 1;'
             ' document.addEventListener("keydown", () => { window.__keys_n++; }, true); } 1')


def press_key(gui, name):
    """An X key with delivery confirmation.

    With an unfocused window keys are lost (runs №4–5: both CDP and xdotool).
    We resend only if the page did not see the event — a lost key has no
    side effects.
    """
    ensure_key_counter(gui)
    for _ in range(5):
        n = gui.eval('window.__keys_n')
        if n is None:
            ensure_key_counter(gui)
            continue
        key(name)
        if wait_for(lambda: gui.eval('window.__keys_n') != n, 2, 0.05):
            return
        focus_window()
    raise AssertionError(f'key {name} did not reach the page')


def field(gui):
    return gui.eval('window.__field()')


def type_text(gui, t):
    gui.eval(f'window.__type({json.dumps(t)}); 1')
    assert wait_for(lambda: field(gui) == t, 5, 0.1), f'text typed: {field(gui)!r}'


def focus_composer(gui):
    """Focus in the input field (lost after a page reload)."""
    ok = gui.eval("(() => { const c = document.querySelector('[data-composer-input=\"true\"]');"
                  " if (!c) return false; c.focus(); return document.activeElement === c; })()")
    assert ok, 'composer got focus'


def caret_start(gui):
    """The caret at the start of the field (entering history mode with a non-empty draft)."""
    ok = gui.eval("(() => { const c = document.querySelector('[data-composer-input=\"true\"]');"
                  " c.focus(); const r = document.createRange();"
                  " r.setStart(c.firstChild || c, 0); r.collapse(true);"
                  " const s = getSelection(); s.removeAllRanges(); s.addRange(r); return true; })()")
    assert ok, 'caret at the start'


def history_file():
    path = os.path.join(CONF, 'input-history.json')
    return json.load(open(path))['items'] if os.path.exists(path) else None


def test_input_history(start, gui):
    launcher = start(FAKE, NOUPD, FAKE_DSH_GUI='1')
    gui.wait()
    focus_window()

    # Enter «sends» (the field is cleared, as in dsh), the text goes into the history
    type_text(gui, 'первое сообщение')
    press_key(gui, 'Return')
    assert wait_for(lambda: field(gui) == '', 5, 0.1), f'after Enter the field is empty: {field(gui)!r}'
    assert wait_for(lambda: history_file() == ['первое сообщение'], 10, 0.2), f'history written: {history_file()!r}'

    type_text(gui, 'второе сообщение')
    press_key(gui, 'Return')
    assert wait_for(lambda: history_file() == ['второе сообщение', 'первое сообщение'], 10, 0.2), \
        f'newest on top: {history_file()!r}'

    # a consecutive repeat is not duplicated
    type_text(gui, 'второе сообщение')
    press_key(gui, 'Return')
    assert wait_for(lambda: history_file() == ['второе сообщение', 'первое сообщение'], 10, 0.2), \
        f'repeat not duplicated: {history_file()!r}'

    # ↑ — newest, ↑↑ — older, no further to go; ↓ — back, ↓ below the newest — empty
    press_key(gui, 'Up')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↑ gives the last one: {field(gui)!r}'
    press_key(gui, 'Up')
    assert wait_for(lambda: field(gui) == 'первое сообщение', 5, 0.1), f'↑↑ gives the previous one: {field(gui)!r}'
    press_key(gui, 'Up')
    assert wait_for(lambda: field(gui) == 'первое сообщение', 5, 0.1), 'no older history'
    press_key(gui, 'Down')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↓ returns to the newer one: {field(gui)!r}'
    press_key(gui, 'Down')
    assert wait_for(lambda: field(gui) == '', 5, 0.1), f'↓ below the newest — empty field: {field(gui)!r}'

    # Esc: the unfinished draft is restored, it does not enter the history
    type_text(gui, 'черновик')
    caret_start(gui)
    press_key(gui, 'Up')
    assert wait_for(lambda: field(gui) == 'второе сообщение', 5, 0.1), f'↑ from the draft: {field(gui)!r}'
    press_key(gui, 'Escape')
    assert wait_for(lambda: field(gui) == 'черновик', 5, 0.1), f'Esc restored the draft: {field(gui)!r}'
    assert history_file() == ['второе сообщение', 'первое сообщение'], f'draft not written: {history_file()!r}'

    # reload of the dsh page: the history is recalled again (injection at load).
    # __mark from the old page disappears only in the new document: otherwise
    # wait() could catch a reload that has not started yet (flaky on debian-13).
    gui.mark()
    gui.eval('location.reload(); 1')
    assert wait_for(lambda: gui.eval("document.readyState === 'complete' && window.__mark === undefined") is True, 15, 0.2), \
        'page reloaded'
    assert wait_for(lambda: gui.eval('!!window.__dshInputHistory'), 10, 0.2), 'history script injected'
    assert wait_for(lambda: gui.eval("!!document.querySelector('[data-composer-input=\"true\"]')") is True, 10, 0.2), \
        'composer on the reloaded page'
    focus_window()
    focus_composer(gui)
    # diagnostics (run №4: ↑ after a reload did not work): listen for keydown/beforeinput
    # and snapshot the injection state — from the report it is visible whether the key
    # reached the page and whether the history handler saw it
    gui.eval("window.__diag = [];"
             " document.addEventListener('keydown', (e) => { window.__diag.push('keydown ' + e.key"
             " + ' target=' + (e.target.id || e.target.tagName)"
             " + ' active=' + (document.activeElement.id || document.activeElement.tagName)"
             " + ' prevented=' + e.defaultPrevented); }, true);"
             " document.addEventListener('beforeinput', (e) => { window.__diag.push('beforeinput '"
             " + e.inputType + ' data=' + JSON.stringify(e.data) + ' prevented=' + e.defaultPrevented); }, true);"
             " window.__state0 = JSON.stringify({ items: (window.__dshInputHistory || {}).items,"
             " enabled: (window.__dshInputHistory || {}).enabled, installed: (window.__dshInputHistory || {}).installed }); 1")
    focus_window()
    focus_composer(gui)
    press_key(gui, 'Up')
    if not wait_for(lambda: field(gui) == 'второе сообщение', 8, 0.1):
        diag = gui.eval('JSON.stringify({ diag: window.__diag, state0: window.__state0,'
                        ' field: window.__field(), items: (window.__dshInputHistory || {}).items })')
        raise AssertionError(f'after reload ↑ works: {field(gui)!r}; diagnostics: {diag}')

    launcher.term()
