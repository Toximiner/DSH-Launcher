"""dsh crashed on its own (not stopped by the launcher): the launcher restarts it
and shows a «dsh restarted after a crash» toast; crashes again and again — after
three restarts within 10 minutes it stops and shows the page with the log."""
import os

from helpers.env import FAKE, NOUPD
from helpers.procs import fakedsh_starts, wait_for
from helpers.windows import Toast

ONCE = '/tmp/crash-once'


def test_dsh_crash_restart(start, gui, drv):
    if os.path.exists(ONCE):
        os.remove(ONCE)
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2', FAKE_CRASH_ONCE=ONCE)
    gui.wait()
    assert launcher.wait_log('dsh упал (code=3, signal=null) — перезапускаю (1 за 10 минут)', 30), launcher.log()[-1500:]
    assert wait_for(lambda: fakedsh_starts() == 2, 30), 'dsh was started again'
    gui.wait()
    toast = Toast(drv).wait_shown('restarted after a crash', 30)
    assert 'exit code 3' in toast.text(), toast.text()
    launcher.term()


def test_dsh_crash_loop(start, gui):
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    assert fakedsh_starts() == 4, f'first start and three restarts: {fakedsh_starts()}'
    assert launcher.log().count('— перезапускаю (') == 3
    assert launcher.alive, 'the launcher is alive — showing the log page'
    launcher.term()
