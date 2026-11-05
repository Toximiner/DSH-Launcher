"""Launcher log (launcher.log next to dsh.log): launcher messages with
timestamps, no dsh lines and no tokens; rotation after 2 MB."""
import os
import re

from helpers.env import FAKE, NOUPD, STATE
from helpers.procs import as_tester, wait_for

LOG = f'{STATE}/launcher.log'


def read(path):
    with open(path, encoding='utf-8', errors='replace') as f:
        return f.read()


def test_launcher_log(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()
    launcher.term()

    text = read(LOG)
    assert re.search(r'===== DeepSeek Harness Launcher \S+, pid \d+, ', text), 'launch header'
    assert re.search(r'^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3} \[launcher\] окно показано$', text, re.M), 'lines with timestamps'
    assert '[launcher] starting: /opt/t/fakes/fakedsh.py' in text
    assert '[launcher] SIGTERM' in text, 'and the shutdown'
    # the fake dsh prints «dsh web: …?token=abc»: dsh lines — only in dsh.log
    assert '[dsh]' not in text, 'no dsh lines in the launcher log'
    assert 'token=abc' in read(f'{STATE}/dsh.log'), 'they are in dsh.log'
    assert 'token=abc' not in text, 'the token is hidden'


def test_launcher_log_rotate(start, gui):
    as_tester(f"mkdir -p {STATE} && head -c 2200000 /dev/zero | tr '\\0' x > {LOG}")
    launcher = start(FAKE, NOUPD)
    gui.wait()
    launcher.term()

    assert os.path.getsize(f'{LOG}.1') >= 2200000, 'the big log was moved to launcher.log.1'
    text = read(LOG)
    assert 'лог прокручен: launcher.log (2200000 Б) -> launcher.log.1' in text, text[:500]
    assert os.path.getsize(LOG) < 100000, 'the new log starts fresh'


def test_log_rotate_while_running(start, gui):
    """Rotation while running, not only at start: for each log — no more than
    two files (*.log and *.log.1), each no bigger than the threshold."""
    max_bytes = 800  # over the session the launcher writes ~2 KB — its log rotates too
    launcher = start(FAKE, NOUPD, DSH_LAUNCHER_LOG_MAX_BYTES=str(max_bytes), FAKE_SPAM_LINES='500')
    gui.wait()
    assert wait_for(lambda: 'spam line 00499' in read(f'{STATE}/dsh.log'), 30), 'dsh finished printing'
    launcher.term()

    logs = sorted(n for n in os.listdir(STATE) if '.log' in n)  # excluding, e.g., the GPU marker
    assert logs == ['dsh.log', 'dsh.log.1', 'launcher.log', 'launcher.log.1'], logs
    for name in logs:
        # rotation happens before the write: no file exceeds the threshold
        assert os.path.getsize(f'{STATE}/{name}') <= max_bytes, f'{name}: {os.path.getsize(f"{STATE}/{name}")} B'
    assert 'spam line 00499' in read(f'{STATE}/dsh.log'), 'the last dsh lines are in the new file'
