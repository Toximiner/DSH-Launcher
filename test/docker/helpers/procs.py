"""Processes, the port, and waiting."""
import os
import subprocess
import time
import urllib.request

from .env import FAKEDSH_LOG


def sh(cmd, check=False):
    """Shell command → (code, output)."""
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and r.returncode:
        raise AssertionError(f'{cmd}: exit code {r.returncode}\n{r.stdout}{r.stderr}')
    return r.returncode, (r.stdout + r.stderr).strip()


def quote(s):
    return "'" + s.replace("'", "'\\''") + "'"


def as_tester(cmd):
    """Shell command as user tester."""
    return sh(f'runuser -u tester -- sh -c {quote(cmd)}')


def wait_for(cond, timeout, step=0.5):
    """Wait until cond() becomes true; return the final value."""
    end = time.time() + timeout
    while time.time() < end:
        if cond():
            return True
        time.sleep(step)
    return bool(cond())


def pgrep(pattern, user=None):
    """Whether there is a process with such a command line. No shell: `sh -c "pgrep -f X"`
    also has X in its command line — pgrep would find the shell itself."""
    args = ['pgrep'] + (['-u', user] if user else []) + ['-f', pattern]
    return subprocess.run(args, capture_output=True).returncode == 0


def alive(pid):
    """The process is alive. A finished one not yet reaped by its parent (“zombie”, Z) —
    is already not alive: the launcher is a direct child of pytest, and until waitpid it
    remains a zombie, while os.kill(pid, 0) succeeds for zombies."""
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
    """Seconds until the process exits, or None if it did not."""
    t0 = time.time()
    if wait_for(lambda: not alive(pid), timeout, 0.1):
        return round(time.time() - t0, 1)
    return None


# ---- fake dsh: the process itself and its child sleep 7777 in the same group ----

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
    """The real dsh (npm) is not running."""
    return not pgrep('@deepseek-ai/dsh|bin/dsh', user='tester')


# ---- the dsh GUI port, the package ----

def port_up():
    try:
        urllib.request.urlopen('http://127.0.0.1:3080/', timeout=2).close()
        return True
    except Exception as e:  # an HTTP error is also “the port answers”
        return hasattr(e, 'code')


def wait_port(timeout):
    return wait_for(port_up, timeout)


def dpkg_version():
    return sh("dpkg-query -Wf '${Version}' dsh-launcher")[1]
