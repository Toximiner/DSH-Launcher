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
import time
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


def close_hotkeys(app):
    # One Esc dismisses the hotkeys overlay and returns to the TUI-settings
    # hub (mirrors terminal_preview.py: Esc closes the hotkeys list while the
    # hub stays open until the second Esc).
    app.send('\x1b')
    # Wait on the dialog-only counter marker and the hub title: the hub's
    # right pane lists a 'Горячие клавиши [Открыть]' field, so the dialog
    # title alone is not a close signal.
    app.wait('Настройки TUI', absent=('Найдено: 67',))

root = Path(tempfile.mkdtemp(prefix='dsh-tui-golden-offline-'))
env = {key: value for key, value in os.environ.items()
       if key not in ('DSH_TUI_URL', 'NO_COLOR', 'DISPLAY', 'WAYLAND_DISPLAY')}
env.update(TERM='xterm-256color', HOME=str(root), XDG_CONFIG_HOME=str(root / 'config'),
           XDG_DATA_HOME=str(root / 'data'), DSH_HOME=str(root / 'dsh'))
print(f'root: {root}', flush=True)

# ---------------------------------------------------------------- no backend
print('== disconnected: landing / help / settings hub / hotkeys / dsh settings / sessions ==', flush=True)
app = Terminal(BINARY, [], env)
try:
    app.wait('Новый разговор', timeout=30)
    app.wait('Нет подключения к dsh.')
    scene(app, 'landing-disconnected')
    app.command('/help')
    app.wait('/clear — очистить экран')
    scene(app, 'help')
    app.send('\x1bOS')  # F4 settings hub
    app.wait('Настройки TUI')
    scene(app, 'settings-hub')
    app.send('\x1bOP')  # F1 hotkeys reference inside the hub
    app.wait('Найдено: 67 · строки 1–16/148')
    scene(app, 'hotkeys')
    close_hotkeys(app)
    # second Esc closes the hub itself
    app.send('\x1b')
    wait_for(lambda: (app.pump(), 'Настройки TUI' not in app.text)[1])
    app.send('\x1bd')  # Alt+D dsh settings page
    app.wait('Для чтения настроек работающего dsh')
    scene(app, 'dsh-settings')
    app.send('\x1bs')  # Alt+S sessions page
    app.wait('Сессии dsh недоступны без подключения')
    scene(app, 'sessions-page')
    app.send('\x1b')
    wait_for(lambda: (app.pump(), 'Enter вход · F3 имя' not in app.text)[1])
    app.wait('Новый разговор')
finally:
    app.close(root / 'offline-disconnected.ansi')

# ------------------------------------------------------------ fake_dsh chat
print('== fake backend: chat, reconnect, history, market, mode, snapshot, sessions ==', flush=True)
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
    # Reconnect without --new: the hub shows the disconnected landing until
    # /new re-joins session-pty and replays its history (probe-verified).
    app = Terminal(BINARY, ['--url', login], env)
    app.wait('Новый разговор', timeout=30)
    app.command('/new')
    app.wait('Сессия · session-pty', timeout=15)
    app.wait('Потоковый ответ из тестового dsh', timeout=15)
    app.wait('Enter — отправить')
    scene(app, 'chat-reconnected-history')
    app.command('/market')
    app.wait('Alpha plugin', timeout=15)
    scene(app, 'market')
    app.send('/Beta\r')
    app.wait('Beta plugin', timeout=15)
    app.send('i')
    app.wait('[y Подтвердить]')
    app.send('y')
    app.wait('установлен: 1.0.0', timeout=15)
    scene(app, 'market-installed')
    # Market is a section: Esc leaves it at the main chat (probe-verified).
    # The sidebar permanently shows '▦ Маркет плагинов', so wait on the
    # market header marker instead of the section title.
    app.send('\x1b')
    app.wait('Сессия · session-pty', absent=('[r Обновить]',))
    app.command('/mode plan')
    app.wait('Режим: планирование', timeout=15)
    scene(app, 'mode-plan')
    app.command('/mode default')
    # After a message has been sent the status line is the bare 'Режим: …'
    # (the '· F2' hint only renders in the empty-chat state).
    app.wait('Режим: обычный', timeout=15)
    app.send('\x1bd')  # Alt+D settings snapshot of the running (fake) dsh
    app.wait('dsh не вернул разделов настроек', timeout=15)
    scene(app, 'settings-snapshot')
    app.send('\x1b')
    wait_for(lambda: (app.pump(), 'Настройки dsh' not in app.text)[1])
    app.command('/sessions')
    app.wait('› session-pty', timeout=15)
    scene(app, 'sessions-list')
    app.send('\x04')
    app.exited()
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