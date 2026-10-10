"""Процессы, порт и ожидания."""
import os
import subprocess
import time
import urllib.request

from .env import FAKEDSH_LOG


def sh(cmd, check=False):
    """Команда оболочки → (код, вывод)."""
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and r.returncode:
        raise AssertionError(f'{cmd}: код {r.returncode}\n{r.stdout}{r.stderr}')
    return r.returncode, (r.stdout + r.stderr).strip()


def quote(s):
    return "'" + s.replace("'", "'\\''") + "'"


def as_tester(cmd):
    """Команда оболочки от пользователя tester."""
    return sh(f'runuser -u tester -- sh -c {quote(cmd)}')


def wait_for(cond, timeout, step=0.5):
    """Ждать, пока cond() не станет истинным; вернуть итоговое значение."""
    end = time.time() + timeout
    while time.time() < end:
        if cond():
            return True
        time.sleep(step)
    return bool(cond())


def pgrep(pattern, user=None):
    """Есть ли процесс с такой командной строкой. Без оболочки: у `sh -c "pgrep -f X"`
    в командной строке тоже есть X — pgrep нашёл бы саму оболочку."""
    args = ['pgrep'] + (['-u', user] if user else []) + ['-f', pattern]
    return subprocess.run(args, capture_output=True).returncode == 0


def alive(pid):
    """Процесс жив. Завершившийся, но не подобранный родителем («зомби», Z) —
    уже не жив: лаунчер — прямой потомок pytest, и до waitpid он остаётся
    зомби, а os.kill(pid, 0) для зомби проходит."""
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    try:
        with open(f'/proc/{pid}/stat') as f:
            state = f.read().rsplit(')', 1)[1].split()[0]
        return state not in ('Z', 'X')
    except (OSError, IndexError):
        return False


def wait_exit(pid, timeout):
    """Секунды до выхода процесса или None, если не вышел."""
    t0 = time.time()
    if wait_for(lambda: not alive(pid), timeout, 0.1):
        return round(time.time() - t0, 1)
    return None


# ---- фейковый dsh: сам процесс и его потомок sleep 7777 в той же группе ----

def dsh_tree_alive():
    return pgrep('fakedsh.py') and pgrep('^sleep 7777')


def dsh_tree_gone():
    return not pgrep('fakedsh.py') and not pgrep('^sleep 7777')


def fakedsh_events():
    try:
        with open(FAKEDSH_LOG) as f:
            return f.read()
    except OSError:
        return ''


def fakedsh_starts():
    return fakedsh_events().count('start pgid')


def real_dsh_gone():
    """Настоящий dsh (npm) не запущен."""
    return not pgrep('@deepseek-ai/dsh|bin/dsh', user='tester')


# ---- порт GUI dsh, пакет ----

def port_up():
    try:
        urllib.request.urlopen('http://127.0.0.1:3080/', timeout=2).close()
        return True
    except Exception as e:  # HTTP-ошибка — тоже «порт отвечает»
        return hasattr(e, 'code')


def wait_port(timeout):
    return wait_for(port_up, timeout)


def dpkg_version():
    return sh("dpkg-query -Wf '${Version}' dsh-launcher")[1]
