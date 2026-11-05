"""Shutdown: closing the window, signals to the launcher, an «adopted» dsh,
and the «is closing…» window (while dsh is stopping).

In all cases the launcher must stop the dsh it started, together with its
descendants (the fake dsh has a sleep 7777 in the same process group).
"""
import json

import pytest

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import as_tester, dsh_tree_alive, dsh_tree_gone, fakedsh_events, pgrep, sh, wait_exit, wait_for, wait_port


def test_close(start, gui):
    """Closing the window is a clean exit: dsh receives SIGTERM."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert dsh_tree_alive(), 'the fakedsh tree is alive (dsh + sleep)'

    gui.close()

    assert launcher.wait_exit(15) is not None, 'the launcher exited after the window was closed'
    assert dsh_tree_gone(), 'the dsh tree has stopped'
    assert 'SIGTERM' in fakedsh_events(), 'dsh received SIGTERM (cleanly)'


def test_sigterm(start, gui):
    """SIGTERM to the launcher (pkill, session end) — also a clean exit."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    assert launcher.term(15) is not None, 'the launcher exited on SIGTERM within 15 s'

    assert dsh_tree_gone(), 'the dsh tree has stopped'
    assert 'SIGTERM' in fakedsh_events(), 'dsh received SIGTERM, not an immediate SIGKILL'
    assert 'SIGTERM — ' in launcher.log(), 'the launcher log shows the signal handler'
    assert 'окно «закрывается…» показано' not in launcher.log(), 'fast exit — without the «is closing…» window'


def test_sigterm_ignored(start, gui):
    """dsh ignores SIGTERM — the launcher finishes it off with SIGKILL after
    ~4 s. While waiting, the main window is hidden and the «is closing…»
    window is shown."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()

    sh(f'kill -TERM {launcher.pid}')
    assert gui.drv.wait_text('Останавливаю dsh|Stopping dsh', 5, page='закрывается|is closing'), 'the «is closing…» window is on screen'
    assert launcher.wait_log('окно «закрывается…» показано', 2)
    assert launcher.wait_exit(15) is not None, 'the launcher exited within 15 s'

    assert dsh_tree_gone(), 'the dsh that ignored SIGTERM was finished by SIGKILL'


def test_sigterm_double(start, gui):
    """A second SIGTERM — do not wait, exit immediately (SIGKILL to dsh)."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()

    sh(f'kill -TERM {launcher.pid}')
    assert launcher.wait_log('SIGTERM — ', 5), 'the first SIGTERM was accepted (the launcher is stopping dsh)'
    sh(f'kill -TERM {launcher.pid}')

    assert launcher.wait_exit(5) is not None, 'after a repeated SIGTERM — exit immediately'
    assert wait_for(dsh_tree_gone, 5), 'the dsh tree has stopped'


@pytest.mark.parametrize('detached', [
    pytest.param(False, id='adopt_samegroup'),   # the new dsh — in the same process group
    pytest.param(True, id='adopt_detached'),     # the new dsh — in its own group
])
def test_adopt(start, gui, detached):
    """dsh restarted itself (as after a plugin update): the launcher
    «adopts» the new process and, on close, stops both it and the old
    one's descendants. The systemd service variables from the launcher's
    environment (provided by the GNOME session) do not reach dsh — neither
    the first instance nor the reborn one: otherwise dshmarket forbids
    itself from restarting."""
    extra = {'FAKE_REBORN_DETACHED': '1'} if detached else {}
    launcher = start(FAKE, NOUPD, FAKE_SELF_RESTART_AFTER='5', INVOCATION_ID='0123456789abcdef', JOURNAL_STREAM='8:12345', **extra)
    gui.wait()

    assert launcher.wait_log('перезапустился сам', 30), 'the launcher adopted the relaunched dsh'
    assert wait_port(10), 'the reborn dsh is listening on the port'
    assert pgrep('^sleep 7777'), 'sleep 7777 remains in the old group'
    # as tester (root in the container without CAP_SYS_PTRACE cannot read
    # another process's environ); [f]akedsh — so the pattern does not match
    # the sh process's own command line
    _, envs = as_tester("for p in $(pgrep -u tester -f '[f]akedsh.py'); do tr '\\0' '\\n' < /proc/$p/environ; echo; done")
    assert 'FAKE_ROLE=reborn' in envs, f'the reborn dsh environment was read: {envs[:300]!r}'
    assert 'INVOCATION_ID=' not in envs and 'JOURNAL_STREAM=' not in envs, 'dsh has no systemd service variables'

    gui.close()

    assert launcher.wait_exit(15) is not None, 'the launcher exited after the window was closed'
    assert dsh_tree_gone(), 'the adopted dsh and the old group descendants have stopped'


CLOSING = 'закрывается|is closing'


def closing_shown(gui, title=CLOSING):
    return gui.drv.wait_text('Останавливаю dsh|Stopping dsh', 5, page=title)


@pytest.mark.parametrize('how', ['menu_quit', 'close_window'])
def test_closing_window(start, gui, how):
    """«File → Quit» and the close button (tray off): the main window is
    hidden at once while dsh is stopping — the «is closing…» window, then
    exit."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()

    if how == 'menu_quit':
        gui.quit()
        assert wait_for(gui.hidden, 3, 0.2), 'the main window was hidden immediately'
    else:
        gui.close()
    assert closing_shown(gui), 'the «is closing…» window is on screen'
    assert launcher.wait_exit(15) is not None, 'the launcher exited'
    assert dsh_tree_gone(), 'the dsh tree has stopped'


def test_closing_window_relaunch(start, gui):
    """Relaunching the launcher after a settings change — the title is «is restarting…»."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()
    settings = gui.open_settings()
    settings.set('profile', 'second')
    settings.wait_note('Saved')
    settings.relaunch()

    assert closing_shown(gui, 'перезапускается|is restarting'), 'the «is restarting…» window is on screen'
    assert launcher.wait_exit(20) is not None, 'the previous instance exited'
    new = launcher.successor()
    assert new, 'the launcher relaunched'
    gui.wait()
    sh(f'kill -TERM {new}')
    assert wait_exit(new, 15) is not None, 'the new instance exited normally'


def test_closing_window_keeps_size(start, gui):
    """The window is hidden before dsh stops — the size recorded is the one
    on screen, not the size of the hidden window."""
    state = f'{CONF}/window-state.json'
    as_tester(f"mkdir -p {CONF} && echo '{{\"width\":1000,\"height\":640}}' > {state}")
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()
    assert gui.outer_width() == 1000

    gui.quit()
    assert closing_shown(gui)
    assert launcher.wait_exit(15) is not None, 'the launcher exited'
    saved = json.load(open(state))
    assert saved.get('width') == 1000 and saved.get('maximized') is False, f'the window size was saved: {saved}'
