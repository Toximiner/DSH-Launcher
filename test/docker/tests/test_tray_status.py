"""Статус dsh у значка трея: подсказка отражает состояние — «dsh запущен»,
когда порт открыт; «dsh не запущен», когда dsh (запущенный не лаунчером)
погас; «dsh остановлен (авто-перезапуск остановлен)», когда собственный dsh
упал и лаунчер больше не перезапускает. Сигналом служит строка лога
«[launcher] трей: …» (нативный tooltip Electron CDP не видит)."""
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
    """dsh запущен лаунчером — включили трей: подсказка «dsh запущен»."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('трей: dsh запущен', 10), launcher.log()[-1500:]
    launcher.term()


def test_tray_status_not_running(start, gui):
    """dsh запущен не лаунчером (в терминале), лаунчер подключился: трей
    «dsh запущен»; погас — «dsh не запущен»."""
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_ROLE=terminal', FAKEDSH],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'dsh из «терминала» запущен'
    launcher = start(FAKE, NOUPD)  # порт открыт — подключается
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('трей: dsh запущен', 10), launcher.log()[-1500:]

    subprocess.run(['pkill', '-f', 'fakes/fakedsh.py'], capture_output=True)  # dsh погас
    assert launcher.wait_log('трей: dsh не запущен', 15), launcher.log()[-1500:]
    launcher.term()


def test_tray_status_stopped(start, gui):
    """Собственный dsh падает снова и снова — авто-перезапуск остановлен:
    подсказка «dsh остановлен (авто-перезапуск остановлен)»."""
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()
    _enable_tray(gui, launcher)
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    assert fakedsh_starts() == 4, 'первый запуск и три перезапуска'
    assert launcher.wait_log('трей: dsh остановлен (авто-перезапуск остановлен)', 15), launcher.log()[-1500:]
    launcher.term()
