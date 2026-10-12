"""Диагностика: «Справка → Сохранить отчёт…» (архив со сведениями и
журналами, токены скрыты) и плашка «прошлый запуск завершился аварийно»."""
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
    # дамп падения (как от Crashpad) — должен попасть в отчёт
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
    # фейковый dsh печатает «dsh web: …?token=abc»
    assert 'token=***' in files['dsh.log.tail'], 'журнал dsh — в отчёте'
    assert not [n for n, t in files.items() if 'token=abc' in t], 'токен скрыт во всех файлах'
    assert 'crash-dumps/test-crash.dmp' in files, f'дамп падения — в отчёте: {sorted(files)}'
    assert launcher.wait_log('отчёт: дампов падений — 1', 5)
    launcher.term()


def test_crash_notice(start, gui, drv):
    """Лаунчер убит (kill -9) — при следующем запуске плашка «в прошлый раз
    завершился аварийно»; после штатного выхода — плашки нет."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    os.kill(launcher.pid, signal.SIGKILL)
    assert wait_exit(launcher.pid, 10) is not None

    launcher = start(FAKE, NOUPD)  # dsh прежнего ещё работает — лаунчер подключится к нему
    gui.wait()
    toast = Toast(drv).wait_shown('closed unexpectedly last time')
    assert 'завершился аварийно' in launcher.log()
    toast.close()
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'завершился аварийно' not in launcher.log(), 'после штатного выхода — без плашки'
    assert not Toast(drv).is_shown()
    launcher.term()


def test_dsh_clean_fds(start, gui):
    """dsh не получает открытые дескрипторы Chromium (разделяемая память,
    сокеты, порт отладки…): лаунчер запускает его, закрыв всё, кроме 0–2."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    out = subprocess.run(['pgrep', '-u', 'tester', '-f', 'fakes/fakedsh.py'], capture_output=True, text=True).stdout
    pids = [int(p) for p in out.split()]
    assert pids, 'фейковый dsh запущен'
    for pid in pids:
        targets = []
        for fd in os.listdir(f'/proc/{pid}/fd'):
            try:
                targets.append(os.readlink(f'/proc/{pid}/fd/{fd}'))
            except OSError:
                pass
        leaked = [t for t in targets if re.search(r'chromium|icudtl|\.asar|\.pak', t)]
        assert not leaked, f'dsh {pid} унаследовал: {leaked}'
    launcher.term()
