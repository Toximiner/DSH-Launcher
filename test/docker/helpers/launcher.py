"""The installed launcher (/usr/bin/dsh-launcher) as user tester with CDP
on port 9222."""
import subprocess
import urllib.request

from .env import DSH_LATEST, GITHUB_API, GITHUB_TOKEN, HOME, LAUNCHER_OUT, ROUTE_KEY, V8_COVERAGE
from .procs import alive, sh, wait_exit, wait_for


def launcher_pids():
    """The launcher's main processes (no --type= — those are not Chromium helper processes)."""
    # by the program path: the argument order of a restarted instance is different
    # (“electron --remote-debugging-port=… /opt/dsh-launcher”)
    out = subprocess.run(['pgrep', '-u', 'tester', '-f', '^/opt/dsh-launcher/electron/electron '],
                         capture_output=True, text=True).stdout
    pids = []
    for p in out.split():
        try:
            with open(f'/proc/{p}/cmdline', 'rb') as f:
                if b'--type=' not in f.read():
                    pids.append(int(p))
        except OSError:
            pass
    # The launcher spawns child processes via fork → exec: before the exec the
    # child has the same command line as the launcher. The main one is the one
    # whose parent is not itself a launcher process.
    return [p for p in pids if _ppid(p) not in pids]


def _ppid(pid):
    try:
        with open(f'/proc/{pid}/stat') as f:
            return int(f.read().rsplit(')', 1)[1].split()[1])
    except (OSError, ValueError, IndexError):
        return None


def launcher_log():
    try:
        with open(LAUNCHER_OUT, encoding='utf-8', errors='replace') as f:
            return f.read()
    except OSError:
        return ''


class Launcher:
    def __init__(self, env):
        base = {'HOME': HOME, 'USER': 'tester', 'LANG': 'C.UTF-8', 'DISPLAY': ':99',
                'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
                'DSH_LAUNCHER_GITHUB_API': GITHUB_API,  # None — the real GitHub
                'DSH_LAUNCHER_ROUTE_KEY': ROUTE_KEY,
                # The “latest” dsh — a pin from env.py: we do not trust the live
                # npm registry in tests (0.2.1-rc.2 broke the scenarios without code changes).
                'DSH_LAUNCHER_FAKE_DSH_LATEST': DSH_LATEST,
                'NODE_V8_COVERAGE': V8_COVERAGE}
        if GITHUB_TOKEN:
            base['DSH_LAUNCHER_GITHUB_TOKEN'] = GITHUB_TOKEN
        base.update(env)
        base = {k: v for k, v in base.items() if v is not None}  # None — the variable is absent (e.g. DISPLAY)
        # The environment — fully our own (env=base), not `env -i A=B` arguments:
        # any process's command line is visible in /proc/*/cmdline.
        # As tester via Python (pytest in the container is root): the process
        # environment is exactly base, with no runuser/PAM additions.
        with open(LAUNCHER_OUT, 'w') as out:
            subprocess.Popen(['/usr/bin/dsh-launcher', '--remote-debugging-port=9222'], cwd=HOME,
                             user='tester', group='tester', extra_groups=[],
                             env=base, stdout=out, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)
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

        assert wait_for(ready, 30), 'the launcher did not start:\n' + '\n'.join(launcher_log().splitlines()[-20:])

    @property
    def alive(self):
        return alive(self.pid)

    def log(self):
        return launcher_log()

    def wait_log(self, text, timeout):
        """Wait for a line in the launcher log."""
        return wait_for(lambda: text in launcher_log(), timeout)

    def wait_log_count(self, text, count, timeout):
        """Wait for the line to appear in the log at least count times."""
        return wait_for(lambda: launcher_log().count(text) >= count, timeout)

    def term(self, timeout=15):
        """SIGTERM → seconds until exit (None — did not exit)."""
        sh(f'kill -TERM {self.pid}')
        return wait_exit(self.pid, timeout)

    def wait_exit(self, timeout=15):
        return wait_exit(self.pid, timeout)

    def successor(self, timeout=20):
        """The new instance after a restart (a different pid), or None. Each poll —
        fresh and only live ones: the old instance, while it is finishing, is still
        visible in the process list. The set must match two polls in a row; of
        several — the newest (the largest pid)."""
        last = []
        found = []

        def stable():
            now = sorted(p for p in launcher_pids() if p != self.pid and alive(p))
            ok = bool(now) and now == last
            last[:] = now
            if ok:
                found[:] = now
            return ok

        wait_for(stable, timeout)
        return found[-1] if found else None

    def last_context_menu(self):
        """The last “контекстное меню: …” line from the log (what was shown on a right click)."""
        lines = [s for s in launcher_log().splitlines() if 'контекстное меню:' in s]
        return lines[-1].split('контекстное меню: ', 1)[1] if lines else None

    def context_menus_shown(self):
        return launcher_log().count('контекстное меню:')

    def context_menus_closed(self):
        return launcher_log().count('контекстное меню закрыто')
