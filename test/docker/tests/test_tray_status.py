"""The dsh status by the tray icon: the tooltip reflects the state — «dsh
running» when the port is open; «dsh not running» when dsh (started, not by
the launcher) goes down; «dsh stopped (auto-restart stopped)» when the
launcher's own dsh has crashed and the launcher no longer restarts it. The
signal is the log line «[launcher] tray: …» (CDP cannot see Electron's
native tooltip)."""
import subprocess

from helpers.env import FAKE, FAKEDSH, NOUPD
from helpers.procs import fakedsh_starts, wait_port


def _enable_tray(gui, launcher):
    settings = gui.open_settings()
    settings.set('closeToTray', 'true')
    settings.wait_note('Saved')
    assert launcher.wait_log('трей включён', 5)
    settings.close()


def test_tray_status_running(start, gui):
    """dsh is started by the launcher — the tray is enabled: the tooltip is «dsh running»."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('трей: dsh запущен', 10), launcher.log()[-1500:]
    launcher.term()


def test_tray_status_not_running(start, gui):
    """dsh is started not by the launcher (in a terminal), the launcher
    attached: the tray is «dsh running»; it goes down — «dsh not running»."""
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_ROLE=terminal', FAKEDSH],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'the «terminal» dsh is up'
    launcher = start(FAKE, NOUPD)  # the port is open — attaches
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('трей: dsh запущен', 10), launcher.log()[-1500:]

    subprocess.run(['pkill', '-f', 'fakes/fakedsh.py'], capture_output=True)  # dsh went down
    assert launcher.wait_log('трей: dsh не запущен', 15), launcher.log()[-1500:]
    launcher.term()


def test_tray_status_stopped(start, gui):
    """The launcher's own dsh keeps crashing — auto-restart is stopped: the
    tooltip is «dsh stopped (auto-restart stopped)»."""
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    assert fakedsh_starts() == 4, 'the first start and three restarts'
    assert launcher.wait_log('трей: dsh остановлен (авто-перезапуск остановлен)', 15), launcher.log()[-1500:]
    launcher.term()
