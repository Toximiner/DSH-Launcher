"""Exercise the Go terminal client without Node in PATH."""
import os
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import threading
from pathlib import Path
import signal
import socket
import select
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from terminal import Terminal, wait_for  # noqa: E402

BINARY = str(Path(sys.argv[1]).resolve())


class TerminalPreview(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='tui-go-pty-')
        env = {**os.environ, 'PATH': '/nonexistent', 'TERM': 'xterm-256color', 'HOME': self.temp.name, 'XDG_CONFIG_HOME': self.temp.name + '/config'}
        env.pop('DSH_TUI_URL', None)
        env.pop('DISPLAY', None)
        env.pop('WAYLAND_DISPLAY', None)
        env.pop('NO_COLOR', None)
        env['COLORTERM'] = 'truecolor'
        self.env = env
        self.app = Terminal(BINARY, [], env)
        self.app.wait('DeepSeek Harness')
        self.capture('landing')

    def tearDown(self):
        self.app.close(Path(self.temp.name) / 'terminal.log')
        self.temp.cleanup()

    def mouse(self, button, x, y, release=False):
        suffix = 'm' if release else 'M'
        self.app.send(f'\x1b[<{button};{x + 1};{y + 1}{suffix}')

    def capture(self, name):
        """Optionally save the actual PTY cells for visual review, never a mockup."""
        directory = os.environ.get('DSH_TUI_CAPTURE_DIR')
        if not directory:
            return
        self.app.pump()
        screen = self.app.screen
        cells = []
        for y in range(screen.lines):
            for x in range(screen.columns):
                cell = screen.buffer[y][x]
                cells.append({'x': x, 'y': y, 'text': cell.data,
                              'fg': cell.fg, 'bg': cell.bg,
                              'bold': cell.bold, 'reverse': cell.reverse})
        output = Path(directory)
        output.mkdir(parents=True, exist_ok=True)
        path = output / f'{self._testMethodName}-{name}-{screen.columns}x{screen.lines}.json'
        path.write_text(json.dumps({'columns': screen.columns, 'rows': screen.lines, 'cells': cells}))

    def assert_composer_surface(self, context_hidden=False):
        self.app.pump()
        screen = self.app.screen
        width = screen.columns - 1
        if screen.columns >= 120 and not context_hidden:
            width -= 32  # 30-cell context panel and 2-cell gap.
        x = 0
        y = screen.lines - 5
        for row in range(y, y + 4):
            self.assertEqual(screen.buffer[row][x].data, '┃', f'Composer edge moved at row {row}')
            for col in range(x, x + width):
                self.assertEqual(screen.buffer[row][col].bg, '242424',
                                 f'Composer surface gap at {col},{row}')
        self.assertIn('Агент', screen.display[y + 3])
        self.assertNotIn('[Модель]', screen.display[y + 3])

    def palette(self, query, execute=True):
        self.app.send('\x10')
        self.app.wait('Найти команду…')
        self.app.send(query)
        self.app.wait(query)
        self.capture('palette')
        if execute:
            self.app.send('\r')

    def click_text(self, text):
        self.app.wait(text)
        for y, line in reversed(list(enumerate(self.app.screen.display))):
            if text in line:
                self.mouse(0, line.index(text), y)
                return
        self.fail(f'No visible click target: {text!r}')

    def close_preferences(self):
        self.app.send('\x1b')

        def closed():
            self.app.pump()
            return 'Настройки TUI' not in self.app.text

        wait_for(closed)

    def test_language_theme_and_shortcut_reference(self):
        self.app.send('preserved draft')
        self.app.send('\x1bOS')  # F4
        self.app.wait('Настройки TUI')
        self.app.send('\x1b[C' + '\x1b[B' * 2 + '\r')
        self.app.wait('[светлая]')
        self.app.send('\x1b[A\r')
        self.app.wait('TUI Settings')
        self.app.send('\x1bOP')  # F1
        self.app.wait('Hotkeys')
        self.app.send('Schedules')
        self.app.wait('Ctrl+G')
        self.assertNotIn('Ctrl+]', self.app.text)
        self.app.send('\x1b')
        self.app.wait('TUI Settings')
        self.app.send('\x1b')
        self.app.wait('preserved draft')
        self.app.send('\x04')
        self.app.exited()
        self.app = Terminal(BINARY, [], self.env)
        self.app.wait('DeepSeek Harness')
        self.app.send('\x1bOS')
        self.app.wait('TUI Settings')
        self.app.wait('[light]')

    def test_mouse_navigation_and_scroll(self):
        self.assertIn(b'\x1b[?1002h', self.app.raw)
        self.assertIn(b'\x1b[?1006h', self.app.raw)
        self.app.send('Черновик мыши')
        self.palette('Настройки dsh', execute=False)
        self.click_text('Настройки dsh')
        self.app.wait('Для чтения настроек работающего dsh')
        self.palette('Настройки TUI', execute=False)
        self.click_text('Настройки TUI')
        self.app.wait('Настройки TUI')
        self.close_preferences()
        self.app.wait('Для чтения настроек работающего dsh')
        self.app.send('\x1b')
        self.app.wait('Черновик мыши')
        self.click_text('Черновик мыши')
        self.app.command(' — продолжение')
        self.app.wait('Черновик мыши — продолжение')
        for i in range(20):
            self.app.command(f'message-{i:02d}')
        self.app.wait('message-19')
        for _ in range(30):
            self.mouse(64, 40, 10)
            self.app.pump()
        self.app.wait('Черновик мыши — продолжение')
        for _ in range(30):
            self.mouse(65, 40, 10)
            self.app.pump()
        self.app.wait('message-19')
        self.app.resize(80, 24)
        self.app.wait('message-19')
        self.capture('chat-resized')
        self.click_text('Сообщение')
        self.app.command('narrow-click')
        self.app.wait('narrow-click')
        self.app.send('\x04')
        self.app.exited()
        self.assertIn(b'\x1b[?1002l', self.app.raw)
        self.assertIn(b'\x1b[?1006l', self.app.raw)

    def test_native_text_selection_releases_mouse_and_preserves_dialog(self):
        self.app.send('selection draft')
        self.app.wait('selection draft')
        self.app.send('\x1bOS')  # F4 settings dialog.
        self.app.wait('Настройки TUI')
        offset = len(self.app.raw)
        self.app.send('\x1b[21~')  # F10: terminal-native selection.
        self.app.wait('Ctrl+Shift+C')
        wait_for(lambda: (self.app.pump(), b'\x1b[?1002l' in self.app.raw[offset:])[1])
        frozen = self.app.text
        self.app.send('\x1b[200~ignored paste\x1b[201~\r')
        for _ in range(4):
            self.app.pump()
        self.assertEqual(self.app.text, frozen)
        self.capture('selection-over-settings')
        offset = len(self.app.raw)
        self.app.send('\x1b')
        self.app.wait('Настройки TUI', absent=('Ctrl+Shift+C',))
        wait_for(lambda: (self.app.pump(), b'\x1b[?1002h' in self.app.raw[offset:])[1])
        self.app.send('\x1b')
        self.app.wait('selection draft')
        self.assertNotIn('ignored paste', self.app.text)

    def test_no_mouse_option(self):
        self.app.close(Path(self.temp.name) / 'first.log')
        env = {**os.environ, 'PATH': '/nonexistent', 'TERM': 'xterm-256color', 'HOME': self.temp.name, 'XDG_CONFIG_HOME': self.temp.name + '/config'}
        env.pop('DSH_TUI_URL', None)
        self.app = Terminal(BINARY, ['--no-mouse'], env)
        self.app.wait('DeepSeek Harness')
        self.assertNotIn(b'\x1b[?1002h', self.app.raw)
        self.app.command('keyboard-only')
        self.app.wait('keyboard-only')

    def test_preferences_survive_restart_and_mouse_can_be_restored(self):
        self.app.command('history entry')
        self.app.wait('history entry')
        self.app.send('unsent draft')
        self.palette('Настройки TUI')
        self.app.wait('Настройки TUI')
        for key in ('\x1b[C', '\x1b[F', '\r', '\x1b[A', '\r'):
            self.app.send(key)
            self.app.pump()
        path = Path(self.env['XDG_CONFIG_HOME']) / 'dsh-launcher-tui/preferences.json'

        def saved(mouse, history):
            self.app.pump()
            return path.exists() and json.loads(path.read_text()) == {'mouse': mouse, 'inputHistory': history}

        wait_for(lambda: saved(False, False))
        self.app.send('\x1b')
        self.app.wait('> unsent draft')
        self.app.send('\x1b[A')
        self.app.wait('> unsent draft')
        wait_for(lambda: (self.app.pump(), b'\x1b[?1002l' in self.app.raw)[1])
        self.assertIn(b'\x1b[?1002l', self.app.raw)
        self.app.send('\x04')
        self.app.exited()
        self.app.close(Path(self.temp.name) / 'before-restart.log')
        self.app = Terminal(BINARY, [], self.env)
        self.app.wait('DeepSeek Harness')
        self.assertNotIn(b'\x1b[?1002h', self.app.raw)
        self.app.send('\x1bOS')
        self.app.wait('Настройки TUI')
        for key in ('\x1b[C', '\x1b[F', '\x1b[A', '\r'):
            self.app.send(key)
            self.app.pump()
        wait_for(lambda: saved(True, False))
        # Persisting the preference and emitting the terminal command are
        # separate events; the file write alone does not acknowledge mouse mode.
        wait_for(lambda: (self.app.pump(), b'\x1b[?1002h' in self.app.raw)[1])
        self.assertIn(b'\x1b[?1002h', self.app.raw)
        self.app.send('\x1b[B\r')
        wait_for(lambda: saved(True, True))

    def test_chat_stream_cancel_and_reconnect_without_resubmission(self):
        self.app.close(Path(self.temp.name) / 'offline.log')
        root = Path(self.temp.name)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        env = {**os.environ, 'DSH_TEST_ROOT': str(root), 'DSH_HOME': str(root / 'dsh')}
        server = subprocess.Popen(['/usr/bin/python3', str(Path(__file__).with_name('fake_dsh.py')),
                                   '--profile', 'web', '--port', str(port)], env=env,
                                  stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
        try:
            self.assertTrue(select.select([server.stdout], [], [], 5)[0], 'Fixture did not start')
            login = server.stdout.readline().strip().split('dsh web: ')[1]
            self.app = Terminal(BINARY, ['--url', login, '--new'], self.env)
            self.app.wait('DeepSeek Harness')
            self.app.command('enter')
            self.app.wait('Потоковый ')
            self.assertNotIn('из тестового dsh', self.app.text)
            self.capture('streaming')
            self.app.wait('Потоковый ответ из тестового dsh')
            self.capture('dialog')
            self.app.wait('Потоковый ответ из тестового dsh',
                          absent=('[F8 Стоп]', '◌ Ожидание ответа…', '◌ Отправка…'))
            self.app.send('Черновик для проверки')
            self.app.wait('Черновик для проверки')
            self.app.send('\x1bs')  # Alt+S opens the searchable quick picker.
            self.app.wait('Сессии и рабочие области')
            # This fixture intentionally lacks workspace/follow: sessions must
            # remain selectable after a partial catalog failure.
            self.app.wait('Рабочие области недоступны')
            self.capture('session-picker-partial')
            self.app.send('\x1b[18~')  # F7 stays inside this modal.
            self.app.wait('Сессии и рабочие области')
            self.app.send('\r')  # Select the current session without reattaching.
            self.app.wait('Черновик для проверки', absent=('Сессии и рабочие области',))
            for columns, rows in ((80, 24), (100, 30), (160, 40)):
                self.app.resize(columns, rows)
                self.app.wait('Черновик для проверки')
                self.assert_composer_surface()
                self.capture('conversation-surface')
            self.app.resize(100, 30)
            self.app.wait('Черновик для проверки')
            self.app.send('\x15')
            self.app.command('cancel test')
            self.app.wait('Частичный ответ до отмены')
            self.app.send('\x1b[19~')  # F8
            self.app.wait('Выполнение остановлено.')
            self.app.command('reconnect test')
            self.app.wait('соединение с чатом потеряно')
            self.app.wait('Потоковый ответ из тестового dsh')
            self.app.send('\x04')
            self.app.exited()
            calls = [json.loads(line) for line in (root / 'requests.jsonl').read_text().splitlines()]
            prompts = [call['args']['request'] for call in calls if call['method'] == 'session/prompt']
            self.assertEqual([p['content'][0]['text'] for p in prompts], ['enter', 'cancel test', 'reconnect test'])
            self.assertEqual(len({p['requestId'] for p in prompts}), 3)
            self.assertTrue(any(call['method'] == 'session/cancel' for call in calls))
        finally:
            server.terminate()
            server.wait(timeout=5)
            server.stdout.close()

    def test_settings_snapshot_navigation_and_refresh(self):
        state = {'reads': 0, 'fail': False, 'paths': []}

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                state['paths'].append(self.path.split('?')[0])
                self.send_response(302)
                self.send_header('Set-Cookie', 'dsh=test; Path=/')
                self.send_header('Location', '/')
                self.end_headers()

            def do_POST(self):
                state['paths'].append(self.path)
                request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                if request['method'] != 'settings/describe':
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({'type': 'server-response', 'rpcId': request['rpcId'],
                        'result': {'ok': True, 'value': {'items': []}}}).encode())
                    return
                state['reads'] += 1
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({
                    'type': 'server-response', 'rpcId': request['rpcId'],
                    'result': {'ok': not state['fail'], 'value': {'namespaces': [
                        {'ns': 'model', 'revision': state['reads'],
                         'value': {'model': f"loaded-model-{state['reads']}"}, 'secrets': []},
                        {'ns': 'tools', 'revision': 0, 'value': {'limit': 42}, 'secrets': []}
                    ]}}
                }).encode())

        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            self.app.close(Path(self.temp.name) / 'first.log')
            env = {**os.environ, 'PATH': '/nonexistent', 'TERM': 'xterm-256color',
                   'HOME': self.temp.name, 'XDG_CONFIG_HOME': self.temp.name + '/config',
                   'DSH_TUI_URL': f'http://127.0.0.1:{server.server_port}/?token=not-for-display'}
            self.app = Terminal(BINARY, [], env)
            self.app.wait('DeepSeek Harness')
            # Init preloads the settings snapshot. Let it settle before an
            # explicit navigation action, whose contract includes a refresh.
            wait_for(lambda: state['reads'] == 1)
            for _ in range(5):
                self.app.pump()
            self.app.send('\x1bd')
            self.app.wait('loaded-model-2')
            self.app.send('\x1bOS')
            self.app.wait('Настройки TUI')
            self.app.send('r')
            self.close_preferences()
            # Closing preferences reveals the existing settings page. Alt+D
            # explicitly reopens/refreshes it through the shared action registry.
            self.app.wait('loaded-model-2')
            self.app.send('\x1b[C')
            self.app.wait('limit = 42')
            self.app.send('\x1b[D')
            self.app.wait('loaded-model-2')
            self.app.send('r')
            self.app.wait('loaded-model-3')
            state['fail'] = True
            self.app.send('r')
            # The previous snapshot is also shown while loading; wait for the
            # failed response before inspecting the server request count.
            self.app.wait('Ошибка:')
            self.app.wait('Показан предыдущий снимок.')
            self.assertIn('loaded-model-3', self.app.text)
            self.assertNotIn('not-for-display', self.app.text)
            self.assertEqual(state['reads'], 4)
            self.assertEqual(set(state['paths']), {'/connect', '/api/settings/describe', '/api/session/list', '/api/remote.mux'})
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=2)

    def test_owned_backend_market_and_agent_modes(self):
        self.app.close(Path(self.temp.name) / 'first.log')
        root = Path(self.temp.name)
        bin_dir = root / 'bin'
        bin_dir.mkdir()
        for name, content in [('node', '#!/bin/sh\necho v24.22.1\n'), ('pnpm', '#!/bin/sh\nexit 0\n')]:
            executable = bin_dir / name
            executable.write_text(content)
            executable.chmod(0o700)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        env = {**os.environ, 'PATH': str(bin_dir), 'HOME': self.temp.name,
               'DSH_HOME': str(root / 'dsh'), 'XDG_CONFIG_HOME': str(root / 'config'),
               'XDG_DATA_HOME': str(root / 'data'), 'DSH_TEST_ROOT': self.temp.name,
               'TERM': 'xterm-256color'}
        env.pop('DSH_TUI_URL', None)
        self.app = Terminal(BINARY, ['--start', '--dsh', str(Path(__file__).with_name('fake_dsh.py')),
                                    '--port', str(port), '--cwd', str(root)], env)
        self.app.wait('Не спрашивать больше')
        manifest = root / 'dsh/profiles/web/package.json'
        self.assertFalse(manifest.exists())
        self.app.command('i')
        self.app.wait('Режим: обычный')
        self.assertIn('dshmarket', manifest.read_text())
        self.app.command('/mode plan')
        self.app.wait('Режим: планирование')
        self.app.send('\x1bOQ')  # F2 toggles planning off
        self.app.wait('Режим: обычный')
        self.app.command('/mode goal')
        self.app.wait('> /goal ')
        self.app.command('Test objective')
        self.app.wait('Режим: цель')
        self.app.wait('Test objective')
        self.app.command('/mode default')
        self.app.wait('Режим: обычный')
        self.app.command('/goal clear')
        self.app.command('/sessions-manager')
        self.app.wait('› session-pty')
        self.app.send('\r')
        self.app.wait('DeepSeek Harness')
        self.assertIn('session-pty', self.app.screen.display[0])
        self.app.command('/market')
        self.app.wait('Alpha plugin')
        self.app.send('/Beta\r')
        self.app.wait('Beta plugin')
        self.app.send('i')
        self.app.wait('[y Подтвердить]')
        writes = root / 'market-writes.jsonl'
        self.assertFalse(writes.exists())
        self.app.send('n')
        self.app.wait('Операции требуют подтверждения.')
        self.app.send('i')
        self.app.wait('[y Подтвердить]')
        self.app.send('y')
        self.app.wait('установлен: 1.0.0')
        self.app.send('u')
        self.app.wait('[y Подтвердить]')
        self.app.send('y')
        self.app.wait('установлен: 2.0.0')
        self.app.send('d')
        self.app.wait('[y Подтвердить]')
        self.app.send('y')
        self.app.wait('не установлен')
        self.assertEqual([json.loads(line)['operation'] for line in writes.read_text().splitlines()],
                         ['install', 'update', 'uninstall'])
        self.app.send('\x1b')
        self.app.wait('DeepSeek Harness')
        self.assertIn('session-pty', self.app.screen.display[0])
        self.app.command('/exit')
        self.app.exited()
        with socket.socket() as sock:
            self.assertNotEqual(sock.connect_ex(('127.0.0.1', port)), 0)
        calls = [json.loads(line)['method'] for line in (root / 'requests.jsonl').read_text().splitlines()]
        self.assertIn('goals/create', calls)
        self.assertIn('goals/pause', calls)
        self.assertNotIn('session/prompt', calls)

    def test_input_history_and_local_commands(self):
        self.app.command('Привет 世界')
        self.app.wait('ВЫ · ЛОКАЛЬНЫЙ ЧЕРНОВИК')
        self.app.send('\x1b[A')
        self.app.wait('> Привет 世界')
        self.app.send('\x15')
        self.app.command('/help')
        self.app.wait('/clear — очистить экран')
        self.app.command('/clear')
        self.app.command('После очистки')
        self.app.wait('После очистки')
        self.assertNotIn('Привет 世界', self.app.text)
        self.app.command('/exit')
        self.app.exited()

    def test_paste_resize_and_ctrl_d(self):
        self.app.send('\x1b[200~/exit\r\x1b[201~')
        self.app.wait('> /exit')
        self.assertIsNone(self.app.proc.poll())
        self.app.send('\x15')
        self.app.resize(80, 24)
        def resized():
            self.app.pump()
            return self.app.screen.columns == 80 and 'Ctrl+P команды' in self.app.text
        wait_for(resized)
        self.app.command('/help')
        self.app.wait('Ctrl+Tab / Shift+Tab — сессии')
        self.app.send('\x04')
        self.app.exited()

    def test_minimum_size_warning_preserves_dialog_and_draft(self):
        self.app.send('MINIMUM SIZE DRAFT')
        self.app.send('\x1bOS')
        self.app.wait('Настройки TUI')
        self.app.resize(79, 24)
        self.app.wait('Увеличьте терминал: минимум 80×24.')
        self.app.wait('Сейчас: 79×24.')
        self.app.send('\rignored text')
        self.app.resize(80, 23)
        self.app.wait('Сейчас: 80×23.')
        self.app.resize(80, 24)
        self.app.wait('Настройки TUI')
        self.close_preferences()
        self.app.wait('MINIMUM SIZE DRAFT')
        self.assertNotIn('ignored text', self.app.text)
        self.app.resize(79, 24)
        self.app.wait('Увеличьте терминал')
        self.app.send('\x04')
        self.app.exited()

    def test_signal_restores_terminal(self):
        self.app.proc.send_signal(signal.SIGTERM)
        self.app.exited()
        self.assertIn(b'\x1b[?1002l', self.app.raw)
        self.assertIn(b'\x1b[?1006l', self.app.raw)

    def test_landing_composer_has_contiguous_background(self):
        for columns, rows in [(100, 30), (80, 24)]:
            self.app.resize(columns, rows)
            self.app.wait('DeepSeek Harness')
            for _ in range(5):
                self.app.pump()
            input_y = next(y for y, line in enumerate(self.app.screen.display) if 'Что будем делать?' in line)
            y = input_y - 1
            x = self.app.screen.display[input_y].index('┃')
            self.assertEqual(self.app.screen.display[y][x:].strip(), '┃')
            self.assertEqual(self.app.screen.display[y + 4][x:].strip(), '┃')
            self.assertIn('Агент', self.app.screen.display[y + 3])
            background = self.app.screen.buffer[y][x + 1].bg
            self.assertNotEqual(background, 'default')
            for yy in range(y, y + 5):
                for xx in range(x, x + min(76, columns - 1)):
                    self.assertEqual(self.app.screen.buffer[yy][xx].bg, background,
                                     f'Composer surface gap at {xx},{yy} in {columns}x{rows}')
            outer_background = self.app.screen.buffer[1][0].bg
            for yy in range(y, y + 5):
                self.assertEqual(self.app.screen.buffer[yy][x - 1].bg, outer_background)
                self.assertEqual(self.app.screen.buffer[yy][x + min(76, columns - 1)].bg, outer_background)
            self.capture('contiguous-surface')

    def test_navigation_preserves_draft(self):
        self.app.send('Несохранённый черновик')
        self.palette('Настройки dsh')
        self.app.wait('Для чтения настроек работающего dsh')
        self.app.send('\x1bOS')
        self.app.wait('Настройки TUI')
        self.close_preferences()
        self.app.wait('Для чтения настроек работающего dsh')
        self.app.send('\x1b')
        self.app.wait('> Несохранённый черновик')


if __name__ == '__main__':
    unittest.main(argv=[sys.argv[0]], verbosity=2)
