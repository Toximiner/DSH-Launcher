"""The rollback suggestion: the previous run terminated abnormally, and it was the
just-updated version (last-update.json, within 30 minutes) — in the toast a
«Roll back to the previous version…» button, it opens the rollback dialog. The
update is old (hours ago) — no button (the crash is no longer linked to the update)."""
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
    """Kill the launcher (kill -9) and substitute the markers: the previous
    crash run — version LATEST_PKG, updated updated_ago (recently by
    default), started started_ago. updated_ago — when the update happened,
    started_ago — when the crashed run started (usually after the update)."""
    os.kill(launcher.pid, signal.SIGKILL)
    assert wait_exit(launcher.pid, 10) is not None
    with open(f'{CONF}/running.json', 'w') as f:
        json.dump({'pid': launcher.pid, 'startedAt': _iso(started_ago), 'version': LATEST_PKG}, f)
    with open(f'{CONF}/last-update.json', 'w') as f:
        json.dump({'from': '1.1.0-1', 'to': LATEST_PKG, 'at': _iso(updated_ago)}, f)


def test_rollback_suggested_after_update(start, gui, drv):
    """An update 10 minutes ago, the crashed run with it 5 minutes ago — the
    «closed unexpectedly» toast with a «Roll back to the previous version…»
    button; it opens the rollback dialog."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _crash_and_set_update(launcher, updated_ago=10 * MIN, started_ago=5 * MIN)

    launcher = start(FAKE, NOUPD)  # the previous dsh is still running — the launcher will attach to it
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    log = launcher.log()
    assert 'завершился аварийно' in log
    assert 'предлагаю откат' in log and LATEST_PKG in log, 'log: the crash is linked to the update'
    assert 'Roll back to the previous version' in toast.text(), 'the «Roll back to the previous version…» button'

    toast.drv.go('dshlauncher://toast/rollback/', page=Toast.TITLE)
    RollbackDialog(drv).wait_text(rf'Installed: {INSTALLED}\. Choose a version: .*{ROLLBACK} ', 30)
    launcher.term()


def test_no_rollback_stale_update(start, gui, drv):
    """An update 3 hours ago, the version with it crashed — no «Roll back to
    the previous version…» button (outside the 30-minute window); a clean
    exit — and no toast anymore."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    _crash_and_set_update(launcher, updated_ago=timedelta(hours=3), started_ago=timedelta(hours=3, minutes=-10))

    launcher = start(FAKE, NOUPD)
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    log = launcher.log()
    assert 'завершился аварийно' in log
    assert 'предлагаю откат' not in log, 'outside the 30-minute window — not linked to the update'
    assert 'Roll back to the previous version' not in toast.text(), 'there is no rollback button'
    toast.close()
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'завершился аварийно' not in launcher.log(), 'after a clean exit — no toast'
    assert not Toast(drv).is_shown()
    launcher.term()
