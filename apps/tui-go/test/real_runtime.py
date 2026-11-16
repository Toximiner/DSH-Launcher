"""Installation, owned actual dsh, binary rollback and reports in an isolated HOME."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tarfile
import tempfile
import threading
import signal

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
parser.add_argument('--install', action='store_true', help='Also install pinned actual dsh through npm')
parser.add_argument('--capture')
args = parser.parse_args()

with tempfile.TemporaryDirectory(prefix='dsh-tui-runtime-') as directory:
    root = Path(directory)
    binary = root / 'tui'
    shutil.copy2(Path(args.binary).resolve(), binary)
    original = binary.read_bytes()
    candidate = root / 'candidate'
    candidate.write_bytes(original + b'TUI_UPDATE_FIXTURE\n')
    candidate.chmod(0o700)
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    dsh = Path(args.dsh).resolve()
    modules = next(p for p in dsh.parents if p.name == 'node_modules')
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-runtime-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
    env = {k: v for k, v in os.environ.items() if k not in
           ('DSH_TUI_URL', 'GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'NO_COLOR')}
    env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_DATA_HOME=str(root / 'data'),
               XDG_CONFIG_HOME=str(root / 'config'), TERM='xterm-256color', COLORTERM='truecolor')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    runtime = root / 'data/dsh-launcher-tui'
    app = Terminal(str(binary), ['--dsh', str(dsh), '--port', str(port), '--cwd', str(root)], env)
    def drain():
        for _ in range(5):
            app.pump()
    def select(index):
        app.send('\x1b[B' * index)
        drain()
    def confirm():
        app.send('\r')
        app.wait('Подтвердить:')
        app.send('\r')
    def enter_value(value):
        app.send('\r')
        app.wait('Введите значение')
        app.send('\x15' + value + '\r')
        app.wait('Подтвердить:')
        app.send('\r')
    def back():
        app.send('\x1b')
        app.wait('Новый разговор' if not session_id else 'Сессия ·')
    def capture(name):
        if not args.capture:
            return
        for columns, rows in [(100, 30), (80, 24)]:
            app.resize(columns, rows)
            app.wait({'runtime': 'Установка и процесс dsh', 'updates': 'Обновления и откат', 'diagnostics': 'Диагностика и уведомления'}[name])
            drain()
            cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                      'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                     for y in range(rows) for x in range(columns)]
            target = Path(f'{args.capture}-{name}-{columns}x{rows}.json')
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(json.dumps({'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
        app.resize(100, 30)
        drain()
    session_id = None
    try:
        app.wait('Новый разговор')
        app.command('/runtime')
        app.wait('Установка и процесс dsh')
        capture('runtime')
        if args.install:
            select(1)
            enter_value('0.2.2-rc.1')
            app.wait('Готово · Установить', timeout=180)
            state = json.loads((runtime / 'dsh-install.json').read_text())
            assert (runtime / 'versions' / state['Current'] / 'node_modules/.bin/dsh').exists()
            back()
            app.command('/runtime')
            app.wait('Установка и процесс dsh')
        select(2)
        # Cancel the first start confirmation; no process should own the port.
        app.send('\r')
        app.wait('Подтвердить:')
        app.send('\x1b')
        drain()
        with socket.socket() as sock:
            assert sock.connect_ex(('127.0.0.1', port)) != 0, 'Cancelled start launched dsh'
        confirm()
        app.wait('Готово · Запустить', timeout=60)
        back()
        app.wait('Введите сообщение.', timeout=30)
        # No prompt is sent: this scenario never invokes a model.
        session_id = 'selected'
        app.command('/runtime')
        app.wait('Установка и процесс dsh')
        select(4)
        confirm()
        app.wait('Готово · Перезапустить', timeout=60)
        back()
        app.wait('Введите сообщение.', timeout=30)
        app.command('/diagnostics')
        app.wait('Диагностика и уведомления')
        capture('diagnostics')
        select(1)
        enter_value(str(root / 'reports'))
        app.wait('Готово · Сохранить', timeout=15)
        reports = list((root / 'reports').glob('*.tar.gz'))
        assert len(reports) == 1
        with tarfile.open(reports[0]) as report:
            assert set(report.getnames()) == {'info.txt', 'runtime.json', 'dsh-and-operations.log', 'README.txt'}
            text = '\n'.join(report.extractfile(name).read().decode() for name in report.getnames())
            assert '?token=[СКРЫТО]' in text, 'Backend login token was not redacted'
            assert 'token=' not in text.replace('token=[СКРЫТО]', ''), 'Raw login token leaked'
        back()
        app.command('/updates')
        app.wait('Обновления и откат')
        capture('updates')
        select(4)
        enter_value(str(candidate) + ' ' + hashlib.sha256(candidate.read_bytes()).hexdigest())
        app.wait('Готово · Установить TUI', timeout=15)
        assert binary.read_bytes() == candidate.read_bytes()
        assert Path(str(binary) + '.previous').read_bytes() == original
        app.send('\x04')
        app.exited(timeout=15)
        with socket.socket() as sock:
            assert sock.connect_ex(('127.0.0.1', port)) != 0, 'Owned backend survived client exit'
        restored = subprocess.run([str(binary) + '.previous', '--rollback-tui'], env=env, capture_output=True, timeout=15)
        assert restored.returncode == 0, restored.stderr
        assert binary.read_bytes() == original, 'Backup executable did not restore original TUI'
        print('Actual dsh owned start/restart/exit, cancelled start, diagnostics masking, actual TUI update and backup recovery passed', flush=True)
    finally:
        app.close(root / 'terminal.log')

    # A completely empty dsh home must also boot from the shipped web profile.
    fresh_env = {**env, 'DSH_HOME': str(root / 'fresh-dsh')}
    fresh = Terminal(str(binary), ['--start', '--market', 'skip', '--dsh', str(dsh),
                     '--port', str(port), '--cwd', str(root)], fresh_env)
    try:
        fresh.wait('Введите сообщение.', timeout=90)
        fresh.send('\x04')
        fresh.exited(timeout=15)
    finally:
        fresh.close(root / 'fresh.log')

    # An attached client never terminates a separately owned actual backend.
    backend = subprocess.Popen([str(dsh), '--profile', 'web', '--no-open', '--port', str(port)],
        cwd=root, env=fresh_env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        text=True, start_new_session=True)
    login = []
    def read_backend():
        for line in backend.stdout:
            if 'dsh web: ' in line:
                login.append(line.split('dsh web: ')[1].strip())
    threading.Thread(target=read_backend, daemon=True).start()
    attached = None
    try:
        wait_for(lambda: login, timeout=60)
        attached = Terminal(str(binary), ['--url', login[0]], fresh_env)
        attached.wait('Новый разговор')
        attached.command('/runtime')
        attached.wait('Установка и процесс dsh')
        attached.send('\x1b[B' * 3 + '\r')
        attached.wait('Подтвердить:')
        attached.send('\r')
        attached.wait('Нет своего работающего процесса.')
        attached.send('\x04')
        attached.exited(timeout=15)
        assert backend.poll() is None, 'Attached TUI stopped another backend'
        print('Empty dsh home startup and external-process preservation passed', flush=True)
    finally:
        if attached:
            attached.close(root / 'attached.log')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
            backend.wait(timeout=15)
