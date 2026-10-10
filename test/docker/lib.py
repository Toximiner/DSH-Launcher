"""Общие помощники Docker-сценариев: запуск лаунчера от tester, ожидания,
состояние процессов fakedsh / настоящего dsh."""
import os
import re
import subprocess
import time
import urllib.request

T = '/opt/t'
OUT = '/tmp/launcher.out'
FAKEDSH_LOG = '/tmp/fakedsh.log'
HOME = '/home/tester'
CONF = f'{HOME}/.config/dsh-launcher'
UPD = f'{CONF}/updates'
GUI = r'^http://127\.0\.0\.1:3080/'  # адрес GUI dsh (регулярное выражение)

FAKE = {'DSH_BIN': f'{T}/fakedsh.py'}
NOUPD = {'DSH_LAUNCHER_NO_UPDATE_CHECK': '1'}

# Последний релиз лаунчера на GitHub (передаёт run.sh): тег vX.Y.Z → пакет X.Y.Z-1.
LATEST = os.environ.get('LATEST', '')
LATEST_PKG = LATEST if '-' in LATEST else f'{LATEST}-1'
LATEST_RE = re.escape(LATEST)
DEB_RE = re.escape(f'dsh-launcher_{LATEST_PKG}_amd64.deb')


def sh(cmd, check=False):
    """Команда оболочки → (код, вывод)."""
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and r.returncode:
        raise AssertionError(f'{cmd}: код {r.returncode}\n{r.stdout}{r.stderr}')
    return r.returncode, (r.stdout + r.stderr).strip()


def as_tester(cmd):
    return sh(f"runuser -u tester -- sh -c {sh_quote(cmd)}")


def sh_quote(s):
    return "'" + s.replace("'", "'\\''") + "'"


def wait_for(cond, timeout, step=0.5):
    end = time.time() + timeout
    while time.time() < end:
        if cond():
            return True
        time.sleep(step)
    return cond()


def pgrep(pattern, user=None):
    """Есть ли процесс с такой командной строкой. Без оболочки: у `sh -c "pgrep -f X"`
    в командной строке тоже есть X — pgrep нашёл бы саму оболочку."""
    args = ['pgrep'] + (['-u', user] if user else []) + ['-f', pattern]
    return subprocess.run(args, capture_output=True).returncode == 0


def tree_alive():
    return pgrep('fakedsh.py') and pgrep('^sleep 7777')


def tree_gone():
    return not pgrep('fakedsh.py') and not pgrep('^sleep 7777')


def real_dsh_gone():
    return not pgrep('@deepseek-ai/dsh|bin/dsh', user='tester')


def port_up():
    try:
        urllib.request.urlopen('http://127.0.0.1:3080/', timeout=2).close()
        return True
    except Exception as e:  # HTTP-ошибка — тоже «порт отвечает»
        return hasattr(e, 'code')


def wait_port(timeout):
    return wait_for(port_up, timeout)


def log():
    try:
        with open(OUT, encoding='utf-8', errors='replace') as f:
            return f.read()
    except OSError:
        return ''


def wait_log(text, timeout):
    return wait_for(lambda: text in log(), timeout)


def fakedsh_starts():
    try:
        with open(FAKEDSH_LOG) as f:
            return f.read().count('start pgid')
    except OSError:
        return 0


def launcher_pids():
    """Главные процессы лаунчера (без --type=)."""
    pids = []
    out = subprocess.run(['pgrep', '-u', 'tester', '-f', 'electron/electron /opt/dsh-launcher'],
                         capture_output=True, text=True).stdout
    for p in out.split():
        try:
            with open(f'/proc/{p}/cmdline', 'rb') as f:
                if b'--type=' not in f.read():
                    pids.append(int(p))
        except OSError:
            pass
    return pids


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def wait_exit(pid, timeout):
    """Секунды до выхода процесса или None, если не вышел."""
    t0 = time.time()
    if wait_for(lambda: not alive(pid), timeout, 0.1):
        return round(time.time() - t0, 1)
    return None


class Launcher:
    """Установленный лаунчер (/usr/bin/dsh-launcher) от tester с CDP на 9222."""

    def __init__(self, env):
        base = {'HOME': HOME, 'USER': 'tester', 'LANG': 'C.UTF-8', 'DISPLAY': ':99',
                'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'}
        base.update(env)
        args = ' '.join(f'{k}={sh_quote(v)}' for k, v in base.items())
        subprocess.Popen(f'runuser -u tester -- env -i {args} /usr/bin/dsh-launcher --remote-debugging-port=9222 >{OUT} 2>&1',
                         shell=True)
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

        assert wait_for(ready, 30), 'лаунчер не стартовал:\n' + '\n'.join(log().splitlines()[-20:])

    def term(self, timeout=15):
        """SIGTERM → секунды до выхода (None — не вышел)."""
        sh(f'kill -TERM {self.pid}')
        return wait_exit(self.pid, timeout)
