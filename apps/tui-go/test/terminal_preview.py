"""First migration checkpoint: exercise the Go shell without Node in PATH."""
import os
from pathlib import Path
import signal
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / 'test/docker-tui'))
from terminal import Terminal  # noqa: E402

BINARY = str(Path(sys.argv[1]).resolve())


class TerminalPreview(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='tui-go-pty-')
        env = {**os.environ, 'PATH': '/nonexistent', 'TERM': 'xterm-256color', 'HOME': self.temp.name}
        env.pop('DISPLAY', None)
        env.pop('WAYLAND_DISPLAY', None)
        self.app = Terminal(BINARY, [], env)
        self.app.wait('Новый разговор')

    def tearDown(self):
        self.app.close(Path(self.temp.name) / 'terminal.log')
        self.temp.cleanup()

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
        self.app.resize(60, 20)
        self.app.command('/help')
        self.app.wait('/clear — очистить экран')
        self.app.send('\x04')
        self.app.exited()

    def test_signal_restores_terminal(self):
        self.app.proc.send_signal(signal.SIGTERM)
        self.app.exited()

    def test_navigation_preserves_draft(self):
        self.app.send('Несохранённый черновик')
        self.app.send('\t\x1b[B')
        self.app.wait('Сессии dsh недоступны')
        self.app.send('\x1b[B')
        self.app.wait('Данные этого окна')
        self.app.send('\x1b')
        self.app.wait('> Несохранённый черновик')


if __name__ == '__main__':
    unittest.main(argv=[sys.argv[0]], verbosity=2)
