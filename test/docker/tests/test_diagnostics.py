"""Diagnostics: «Help → Save report…» (an archive with info and logs, tokens
masked) and the «the previous run terminated abnormally» toast."""
import os
import re
import signal
import subprocess
import tarfile

from helpers.env import CONF, FAKE, INSTALLED, NOUPD
from helpers.procs import as_tester, wait_exit
from helpers.windows import ReportDialog, Toast

REPORT = '/tmp/report.tar.gz'


def test_report(start, gui, drv):
    # a crash dump (as from Crashpad) — must end up in the report
    as_tester(f'mkdir -p {CONF}/Crashpad/completed && head -c 4096 /dev/urandom > {CONF}/Crashpad/completed/test-crash.dmp')
    launcher = start(FAKE, NOUPD, DSH_LAUNCHER_REPORT_PATH=REPORT)
    gui.wait()
    gui.go('dshlauncher://menu/report/')
    ReportDialog(drv).wait_text(f'Report saved: {REPORT}', 30)

    with tarfile.open(REPORT) as tar:
        files = {m.name.split('/', 1)[1]: tar.extractfile(m).read().decode('utf-8', 'replace')
                 for m in tar.getmembers() if m.isfile()}
    assert {'info.txt', 'launcher.log', 'dsh.log.tail', 'settings.json'} <= set(files), sorted(files)
    assert f'DeepSeek Harness Launcher: {INSTALLED}' in files['info.txt'], files['info.txt']
    assert 'dsh backend: 0.2.0-rc.2' in files['info.txt'], files['info.txt']
    # the fake dsh prints «dsh web: …?token=abc»
    assert 'token=***' in files['dsh.log.tail'], 'the dsh log — in the report'
    assert not [n for n, t in files.items() if 'token=abc' in t], 'the token is masked in every file'
    assert 'crash-dumps/test-crash.dmp' in files, f'the crash dump — in the report: {sorted(files)}'
    assert launcher.wait_log('отчёт: дампов падений — 1', 5)
    launcher.term()


def test_crash_notice(start, gui, drv):
    """The launcher was killed (kill -9) — on the next start the «closed
    unexpectedly last time» toast; after a clean exit — no toast."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    os.kill(launcher.pid, signal.SIGKILL)
    assert wait_exit(launcher.pid, 10) is not None

    launcher = start(FAKE, NOUPD)  # the previous dsh is still running — the launcher will attach to it
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    assert 'завершился аварийно' in launcher.log()
    toast.close()
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'завершился аварийно' not in launcher.log(), 'after a clean exit — no toast'
    assert not Toast(drv).is_shown()
    launcher.term()


def test_dsh_clean_fds(start, gui):
    """dsh receives no open file descriptors from Chromium (shared memory, sockets,
    the debug port…): the launcher starts it with everything closed except 0–2."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    out = subprocess.run(['pgrep', '-u', 'tester', '-f', 'fakes/fakedsh.py'], capture_output=True, text=True).stdout
    pids = [int(p) for p in out.split()]
    assert pids, 'the fake dsh is up'
    for pid in pids:
        targets = []
        for fd in os.listdir(f'/proc/{pid}/fd'):
            try:
                targets.append(os.readlink(f'/proc/{pid}/fd/{fd}'))
            except OSError:
                pass
        leaked = [t for t in targets if re.search(r'chromium|icudtl|\.asar|\.pak', t)]
        assert not leaked, f'dsh {pid} inherited: {leaked}'
    launcher.term()
