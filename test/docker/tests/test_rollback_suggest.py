"""Предложение отката: прошлый запуск завершился аварийно, и это была только
что обновлённая версия (last-update.json, в пределах 30 минут) — в плашке
кнопка «Вернуть предыдущую версию…», она открывает диалог отката. Обновление
старое (часы назад) — кнопки нет (сбой уже не связывают с обновлением)."""
import json
import os
import signal
from datetime import datetime, timedelta, timezone

from helpers.env import CONF, FAKE, INSTALLED, LATEST_PKG, NOUPD, ROLLBACK
from helpers.procs import wait_exit
from helpers.windows import RollbackDialog, Toast

MIN = timedelta(minutes=1)


def _iso(delta):
    return (datetime.now(timezone.utc) - delta).isoformat(timespec='seconds').replace('+00:00', 'Z')


def _crash_and_set_update(launcher, updated_ago, started_ago):
    """Убить лаунчер (kill -9) и подставить метки: прошлый аварийный запуск —
    версия LATEST_PKG, обновлена updated_ago (по умолчанию недавно), стартовал
    started_ago. updated_ago — когда было обновление, started_ago — когда
    начался упавший запуск (обычно позже обновления)."""
    os.kill(launcher.pid, signal.SIGKILL)
    assert wait_exit(launcher.pid, 10) is not None
    with open(f'{CONF}/running.json', 'w') as f:
        json.dump({'pid': launcher.pid, 'startedAt': _iso(started_ago), 'version': LATEST_PKG}, f)
    with open(f'{CONF}/last-update.json', 'w') as f:
        json.dump({'from': '1.1.0-1', 'to': LATEST_PKG, 'at': _iso(updated_ago)}, f)


def test_rollback_suggested_after_update(start, gui, drv):
    """Обновление 10 минут назад, упавший запуск с ним 5 минут назад — плашка
    «завершился аварийно» с кнопкой «Вернуть предыдущую версию…»; она
    открывает диалог отката."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _crash_and_set_update(launcher, updated_ago=10 * MIN, started_ago=5 * MIN)

    launcher = start(FAKE, NOUPD)  # dsh прежнего ещё работает — лаунчер подключится к нему
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    log = launcher.log()
    assert 'завершился аварийно' in log
    assert 'предлагаю откат' in log and LATEST_PKG in log, 'лог: связали сбой с обновлением'
    assert 'Roll back to the previous version' in toast.text(), 'кнопка «Вернуть предыдущую версию…»'

    toast.drv.go('dshlauncher://toast/rollback/', page=Toast.TITLE)
    RollbackDialog(drv).wait_text(rf'Installed: {INSTALLED}\. Choose a version: .*{ROLLBACK} ', 30)
    launcher.term()


def test_no_rollback_stale_update(start, gui, drv):
    """Обновление 3 часа назад, упала версия с ним — кнопки «Вернуть
    предыдущую версию…» нет (вне окна 30 минут); штатный выход — и плашки
    больше нет."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _crash_and_set_update(launcher, updated_ago=timedelta(hours=3), started_ago=timedelta(hours=3, minutes=-10))

    launcher = start(FAKE, NOUPD)
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    log = launcher.log()
    assert 'завершился аварийно' in log
    assert 'предлагаю откат' not in log, 'вне окна 30 минут — не связываем с обновлением'
    assert 'Roll back to the previous version' not in toast.text(), 'кнопки отката нет'
    toast.close()
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'завершился аварийно' not in launcher.log(), 'после штатного выхода — без плашки'
    assert not Toast(drv).is_shown()
    launcher.term()
