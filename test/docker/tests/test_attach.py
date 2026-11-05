"""Window and dsh started outside the launcher, or one that comes up slowly:
the «Token required» page (dsh from a terminal, no cookie), login without it
(dsh restarted by the market button, the link — in its output file) and
«Failed to start dsh» (the port never opened) — with the switch to the GUI once dsh is ready."""
import os
import subprocess

from helpers.env import FAKE, FAKEDSH, FAKEDSH_LOG, NOUPD
from helpers.procs import pgrep, wait_for, wait_port
from helpers.windows import PastePage


def test_paste_token(start, gui, drv):
    """dsh is already running (from a terminal) and asks for a token, the window has no cookie —
    the paste page; the user pastes the token — the window shows dsh's GUI. Closing the launcher
    does not stop a foreign dsh."""
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_REQUIRE_TOKEN=1', 'FAKE_ROLE=terminal', FAKEDSH,
                      '--profile', 'web'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'the dsh from the "terminal" is up'
    launcher = start(FAKE, NOUPD)
    page = PastePage(drv)
    page.wait(r'Token required.*paste the full URL here', 30)
    assert 'подключение: валидного куки и ссылки входа нет — страница вставки токена' in launcher.log()

    page.paste('abc')  # token only — the launcher fills in the address itself

    gui.wait()
    assert 'token accepted' in open(FAKEDSH_LOG).read(), 'the token was sent to dsh'
    assert 'FAKE DSH terminal' in gui.eval('document.body.innerText'), 'the window shows the GUI of that very dsh'
    launcher.term()
    assert pgrep('fakedsh.py') and wait_port(1), 'the external dsh keeps running'


def test_market_restart_token(start, gui):
    """dsh was restarted by the market button: the new process is not a child of
    the launcher, has its own token (the old cookie is no good) and prints the
    login link to its own file. The launcher picks it up from the output of the
    process on the port — the window is in the GUI again, no «Token required» page."""
    out = '/tmp/fakedsh-reborn.out.log'
    launcher = start(FAKE, NOUPD, FAKE_REQUIRE_TOKEN='1', FAKE_SELF_RESTART_AFTER='5',
                     FAKE_REBORN_TOKEN='xyz', FAKE_REBORN_OUT=out)
    gui.wait()

    assert launcher.wait_log('ссылка входа — из вывода процесса dsh на порту', 40), 'the login link was taken from the new dsh output'

    def reborn_gui():
        try:
            return 'FAKE DSH reborn' in (gui.eval('document.body.innerText') or '')
        except Exception:  # noqa: BLE001 — window in transition
            return False
    assert wait_for(reborn_gui, 20), 'the window shows the GUI of the restarted dsh'
    assert 'token=xyz' not in launcher.log(), 'the token did not leak into the log'
    assert 'страница вставки токена' not in launcher.log()


def test_dsh_no_port(start, gui):
    """dsh is running, but did not bring up the port within the allotted time — the «Failed to
    start dsh» page with the log; brought it up later — the window switches to the GUI on its own."""
    listen = '/tmp/fakedsh-listen'
    if os.path.exists(listen):
        os.remove(listen)
    launcher = start(FAKE, NOUPD, FAKE_LISTEN_FILE=listen, DSH_START_TIMEOUT_MS='3000')
    page = PastePage(gui.drv)
    page.wait(r'Failed to start dsh.*did not bring up the service', 30)

    open(listen, 'w').close()  # dsh finally brought the service up

    assert wait_port(15), 'the port opened'
    gui.wait(30)
    launcher.term()
