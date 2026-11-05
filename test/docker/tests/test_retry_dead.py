"""«Check again» from a fatal page: dsh is dead (the port is closed) — the «dsh is not
running» page with a terminal command; dsh started by hand — «Check again» again
brings the window to the GUI (a foreign dsh is not killed on close).

All CDP operations are done with page= (by title): the first entry in /json/list
may be a toast notification (launcher-toast — a separate CDP page)."""
import subprocess

from helpers.env import FAKE, FAKEDSH, NOUPD
from helpers.procs import pgrep, wait_port
from helpers.windows import Toast

FATAL = 'dsh has stopped'
NOTRUN = 'dsh is not running'


def test_retry_dead(start, gui, drv):
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()

    # dsh crashes and crashes again — the launcher stops restarting it and
    # shows the fatal page with a «Check again» button.
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    drv.wait_text(FATAL + r'.*Check again', 30, page=FATAL)

    # The port is closed (dsh is dead) → «Check again» does not go to the GUI,
    # but to the «dsh is not running» page with the start command for a terminal.
    drv.go('dshlauncher://retry/', page=FATAL)
    drv.wait_text(NOTRUN + r'.*Port 3080 is closed.*Start it in a terminal', 30, page=NOTRUN)

    # Start dsh «from a terminal» (by hand) — «Check again» connects to it:
    # the GUI, and the launcher does not count this dsh as its own.
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_ROLE=terminal', FAKEDSH],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'the manual dsh brought up the port'
    drv.go('dshlauncher://retry/', page=NOTRUN)
    assert launcher.wait_log('retry: dsh запущен: куки валидны — открываю GUI', 60), launcher.log()[-2000:]
    gui.wait()
    assert 'FAKE DSH terminal' in (gui.eval('document.body.innerText') or ''), 'the window shows the GUI of that dsh'
    assert pgrep('fakedsh.py'), 'the manual dsh is alive'

    # closing: a foreign dsh is not killed
    assert launcher.term() is not None, 'the launcher exited normally'
    assert pgrep('fakedsh.py') and wait_port(1), 'a dsh not started by the launcher kept running'
