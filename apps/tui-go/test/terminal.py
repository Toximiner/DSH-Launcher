"""PTY driver for the installed application; pyte interprets the visible screen."""
import codecs
import fcntl
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import termios
import time

import pyte


def wait_for(predicate, timeout=10):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(0.02)
    raise AssertionError('Condition did not settle before timeout')


def alive(pid):
    try:
        # A dead child can remain as a zombie until the container init reaps it.
        return Path(f'/proc/{pid}/stat').read_text().split(') ', 1)[1][0] != 'Z'
    except FileNotFoundError:
        return False


class Terminal:
    def __init__(self, executable, args, env, columns=100, rows=30):
        self.master, self.slave = pty.openpty()
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack('HHHH', rows, columns, 0, 0))
        self.initial = termios.tcgetattr(self.slave)
        self.screen = pyte.Screen(columns, rows)
        self.stream = pyte.Stream(self.screen)
        self.decoder = codecs.getincrementaldecoder('utf8')(errors='replace')
        self.raw = bytearray()
        # This child is an interactive terminal, even when its driver runs in CI.
        # termenv treats CI=true as redirected output and disables terminal colors.
        terminal_env = dict(env)
        terminal_env.pop('CI', None)
        self.proc = subprocess.Popen([executable, *args], stdin=self.slave, stdout=self.slave,
                                     stderr=self.slave, env=terminal_env, start_new_session=True)

    def pump(self):
        # A redraw may span several PTY reads. Drain the currently arriving
        # output before inspecting pyte, rather than mixing a new heading with
        # the previous frame's ready status. Bound draining for live streams.
        deadline = time.monotonic() + 0.1
        pause = 0.03
        while select.select([self.master], [], [], pause)[0]:
            data = os.read(self.master, 65536)
            if not data:
                break
            self.raw.extend(data)
            self.stream.feed(self.decoder.decode(data))
            if time.monotonic() >= deadline:
                break
            pause = 0.01

    @property
    def text(self):
        return '\n'.join(self.screen.display)

    def wait(self, text, timeout=10, absent=()):
        expected = (text,) if isinstance(text, str) else tuple(text)
        def visible():
            self.pump()
            screen = self.text
            return all(value in screen for value in expected) and not any(value in screen for value in absent)
        try:
            wait_for(visible, timeout)
        except AssertionError:
            raise AssertionError(f'Terminal did not display {text!r}; exit={self.proc.poll()}\n{self.text}') from None

    def send(self, text):
        os.write(self.master, text.encode('utf8'))

    def command(self, text):
        self.send(text + '\r')

    def resize(self, columns, rows):
        self.screen.resize(rows, columns)
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack('HHHH', rows, columns, 0, 0))
        self.proc.send_signal(signal.SIGWINCH)

    def exited(self, code=0, timeout=10):
        def finished():
            self.pump()
            status = self.proc.poll()
            # A reaped child can still have unread terminal frames in the PTY.
            # Wait for the final restore frame, not just the process exit.
            return status is not None and (status != code or b'\x1b[?1049l' in self.raw)
        wait_for(finished, timeout)
        self.pump()
        assert self.proc.returncode == code, self.text
        assert b'\x1b[?1049l' in self.raw, 'Alternate screen was not restored'
        mask = termios.ICANON | termios.ECHO | termios.ISIG
        assert termios.tcgetattr(self.slave)[3] & mask == self.initial[3] & mask, 'Terminal flags were not restored'

    def close(self, transcript):
        try:
            if self.proc.poll() is None:
                self.send('\x04')
                try:
                    self.exited(timeout=8)
                except AssertionError:
                    self.proc.send_signal(signal.SIGTERM)
                    try:
                        self.proc.wait(timeout=6)
                    except subprocess.TimeoutExpired:
                        self.proc.kill()
                        self.proc.wait(timeout=3)
            self.pump()
            Path(transcript).write_bytes(self.raw)
        finally:
            os.close(self.master)
            os.close(self.slave)
