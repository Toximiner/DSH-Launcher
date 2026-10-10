"""Установленный лаунчер (/usr/bin/dsh-launcher) от пользователя tester с CDP
на порту 9222."""
import subprocess
import time
import urllib.request

from .env import HOME, LAUNCHER_OUT
from .procs import alive, quote, sh, wait_exit, wait_for


def launcher_pids():
    """Главные процессы лаунчера (без --type= — это не служебные процессы Chromium)."""
    out = subprocess.run(['pgrep', '-u', 'tester', '-f', 'electron/electron /opt/dsh-launcher'],
                         capture_output=True, text=True).stdout
    pids = []
    for p in out.split():
        try:
            with open(f'/proc/{p}/cmdline', 'rb') as f:
                if b'--type=' not in f.read():
                    pids.append(int(p))
        except OSError:
            pass
    return pids


def launcher_log():
    try:
        with open(LAUNCHER_OUT, encoding='utf-8', errors='replace') as f:
            return f.read()
    except OSError:
        return ''


class Launcher:
    def __init__(self, env):
        base = {'HOME': HOME, 'USER': 'tester', 'LANG': 'C.UTF-8', 'DISPLAY': ':99',
                'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'}
        base.update(env)
        args = ' '.join(f'{k}={quote(v)}' for k, v in base.items())
        subprocess.Popen(f'runuser -u tester -- env -i {args} /usr/bin/dsh-launcher '
                         f'--remote-debugging-port=9222 >{LAUNCHER_OUT} 2>&1', shell=True)
        self.pid = None

        def ready():
            pids = launcher_pids()
            if not pids:
                return False
            try:
                urllib.request.urlopen('http://127.0.0.1:9222/json/list', timeout=2).close()
            except Exception:
                return False
            self.pid = pids[0]
            return True

        assert wait_for(ready, 30), 'лаунчер не стартовал:\n' + '\n'.join(launcher_log().splitlines()[-20:])

    @property
    def alive(self):
        return alive(self.pid)

    def log(self):
        return launcher_log()

    def wait_log(self, text, timeout):
        """Дождаться строки в логе лаунчера."""
        return wait_for(lambda: text in launcher_log(), timeout)

    def term(self, timeout=15):
        """SIGTERM → секунды до выхода (None — не вышел)."""
        sh(f'kill -TERM {self.pid}')
        return wait_exit(self.pid, timeout)

    def wait_exit(self, timeout=15):
        return wait_exit(self.pid, timeout)

    def successor(self, wait=3):
        """Новый экземпляр после самообновления (другой pid) или None."""
        time.sleep(wait)
        pids = [p for p in launcher_pids() if p != self.pid]
        return pids[0] if pids else None

    def last_context_menu(self):
        """Последняя строка «контекстное меню: …» из лога (что показали по правому клику)."""
        lines = [s for s in launcher_log().splitlines() if 'контекстное меню:' in s]
        return lines[-1].split('контекстное меню: ', 1)[1] if lines else None

    def context_menus_shown(self):
        return launcher_log().count('контекстное меню:')
