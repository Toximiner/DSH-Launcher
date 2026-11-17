"""Golden baseline captures for the disconnected + fake-backend TUI states.

Runs the built TUI without Node (PATH stripped), against nothing and against
fake_dsh.py. Every scene is captured at 100x30 and 80x24 into
test/golden/before/offline-*.json for the visual-rebuild baseline.

Usage: python3 capture_golden_offline.py --binary build/tui-go/dsh-launcher-tui-go
"""
import argparse
import json
import os
import select
import socket
import subprocess
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from terminal import Terminal, wait_for  # noqa: E402
from golden import capture_cells  # noqa: E402

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--out', default=str(Path(__file__).resolve().parent / 'golden' / 'before'))
args = parser.parse_args()
BINARY = str(Path(args.binary).resolve())
OUT = Path(args.out)
SIZES = [(100, 30), (80, 24)]


def scene(app, name):
    for columns, rows in SIZES:
        app.resize(columns, rows)
        for _ in range(8):
            app.pump()
        path = OUT / f'offline-{name}-{columns}x{rows}.json'
        capture_cells(app, path, source=f'offline golden: {name}')
        print(f'  {name} {columns}x{rows} -> {path.name}', flush=True)
    app.resize(100, 30)
    for _ in range(4):
        app.pump()


root = Path(tempfile.mkdtemp(prefix='dsh-tui-golden-offline-'))
env = {key: value for key, value in os.environ.items()
       if key not in ('DSH_TUI_URL', 'NO_COLOR', 'DISPLAY', 'WAYLAND_DISPLAY')}
env.update(TERM='xterm-256color', HOME=str(root), XDG_CONFIG_HOME=str(root / 'config'),
           XDG_DATA_HOME=str(root / 'data'), DSH_HOME=str(root / 'dsh'))
print(f'root: {root}', flush=True)

# ---------------------------------------------------------------- no backend
print('== disconnected landing / help / mode ==', flush=True)
app = Terminal(BINARY, [], env)
try:
    app.wait('Новый разговор', timeout=30)
    app.wait('Нет подключения к dsh.')
    scene(app, 'landing-disconnected')
    app.command('/help')
    app.wait('/clear — очистить экран')
    scene(app, 'help')
    app.command('/mode plan')
    app.wait('Режим: планирование')
    scene(app, 'mode-plan')
    app.command('/mode default')
    app.wait('Режим: обычный')
    app.send('черновик сохраняется')
    app.wait('> черновик сохраняется')
    app.send('\x1bOS')  # F4 settings hub
    app.wait('Настройки TUI')
    scene(app, 'settings-hub')
    app.send('\x1bOP')  # F1 hotkeys reference inside the hub
    app.wait('Ctrl+G')
    scene(app, 'hotkeys')
    app.send('\x1b')
    wait_for(lambda: (app.pump(), 'Ctrl+G' not in app.text)[1])
    app.send('\x1bd')  # Alt+D dsh settings page
    app.wait('Для чтения настроек работающего dsh')
    scene(app, 'dsh-settings')
    app.send('\x1bs')  # Alt+S sessions page
    app.wait('Сессии dsh')
    scene(app, 'sessions-page')
    app.send('\x1b')
    wait_for(lambda: (app.pump(), 'Сессии dsh' not in app.text)[1])
    app.wait('> черновик сохраняется')
finally:
    app.close(root / 'offline-disconnected.ansi')

# ------------------------------------------------------------ fake_dsh chat
print('== fake backend: chat, market, settings snapshot ==', flush=True)
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    port = sock.getsockname()[1]
backend_env = {**env, 'DSH_TEST_ROOT': str(root)}
server = subprocess.Popen([sys.executable, str(Path(__file__).with_name('fake_dsh.py')),
                           '--profile', 'web', '--port', str(port)],
                          env=backend_env, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL, text=True)
try:
    assert select.select([server.stdout], [], [], 10)[0], 'fake_dsh did not start'
    login = server.stdout.readline().strip().split('dsh web: ')[1]
    app = Terminal(BINARY, ['--url', login, '--new'], env)
    try:
        app.wait('Введите сообщение.', timeout=30)
        scene(app, 'chat-empty')
        app.command('enter')
        app.wait('Потоковый ', timeout=15)
        app.wait('Потоковый ответ из тестового dsh')
        app.wait('Enter — отправить')
        scene(app, 'chat-done')
        app.command('reconnect test')
        app.wait('соединение с чатом потеряно')
        scene(app, 'chat-reconnecting')
        app.wait('Потоковый ответ из тестового dsh', timeout=20)
        app.wait('Enter — отправить')
        app.send('\x04')
        app.exited()
        app = Terminal(BINARY, ['--url', login, '--new'], env)
        app.wait('Введите сообщение.', timeout=30)
        # History replayed through the snapshot.
        app.wait('Потоковый ответ из тестового dsh', timeout=15)
        scene(app, 'chat-reconnected-history')
        app.command('/market')
        app.wait('Alpha plugin', timeout=15)
        scene(app, 'market')
        app.send('i')
        app.wait('[y Подтвердить]')
        app.send('y')
        app.wait('установлен: 1.0.0', timeout=15)
        scene(app, 'market-installed')
        app.command('/settings')
        try:
            app.wait('Для чтения настроек', timeout=3)
        except Exception:
            pass
        app.send('\x1bd')
        app.wait('model =')
        scene(app, 'settings-snapshot')
        app.send('\x04')
        app.exited()
    finally:
        app.close(root / 'offline-fake.ansi')
finally:
    server.terminate()
    try:
        server.wait(timeout=5)
    except subprocess.TimeoutExpired:
        server.kill()
    server.stdout.close()

print('offline golden captures complete:', flush=True)
for path in sorted(OUT.glob('offline-*.json')):
    print(' ', path.name, flush=True)
