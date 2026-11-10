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
    def __init__(self, executable, args, env):
        self.master, self.slave = pty.openpty()
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack('HHHH', 30, 100, 0, 0))
        self.initial = termios.tcgetattr(self.slave)
        self.screen = pyte.Screen(100, 30)
        self.stream = pyte.Stream(self.screen)
        self.decoder = codecs.getincrementaldecoder('utf8')(errors='replace')
        self.raw = bytearray()
        self.proc = subprocess.Popen([executable, *args], stdin=self.slave, stdout=self.slave,
                                     stderr=self.slave, env=env, start_new_session=True)

    def pump(self):
        if select.select([self.master], [], [], 0.03)[0]:
            data = os.read(self.master, 65536)
            self.raw.extend(data)
            self.stream.feed(self.decoder.decode(data))

    @property
    def text(self):
        return '\n'.join(self.screen.display)

    def wait(self, text, timeout=10):
        def visible():
            self.pump()
            return text in self.text
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
            return self.proc.poll() is not None
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
