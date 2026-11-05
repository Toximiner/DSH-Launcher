"""Fixtures for the Docker scenarios.

Each scenario runs in a fresh container: run.sh invokes
`pytest /opt/t/tests --scenario <name>`. The scenario name is the test name
without the «test_» prefix; for a parametrized test it is the parameter id
(test_adopt[adopt_detached] → adopt_detached). `--list-scenarios` prints all
names (run.sh builds its default list from them).

Container environment: system dbus, polkitd (a “tester — allow / deny” rule
per POLKIT=yes|no), Xvfb; the launcher runs as tester with CDP on 9222.
"""
import json
import os
import subprocess
import uuid

import pytest

from helpers.cdp import Driver
from helpers.env import FAKEDSH_LOG, STATE, V8_COVERAGE
from helpers.github import DSH_RELEASES, DEBS, PORT, RELEASES, SERVER, github_log, releases, write_releases
from helpers.launcher import Launcher, launcher_log, launcher_pids
from helpers.procs import as_tester, fakedsh_events, pgrep, sh, wait_for
from helpers.windows import Gui, UpdatePrompt


# ---------------- scenario selection ----------------

def pytest_addoption(parser):
    parser.addoption('--scenario', help='run a single scenario by name')
    parser.addoption('--list-scenarios', action='store_true', help='print the scenario names')


def scenario_name(item):
    callspec = getattr(item, 'callspec', None)
    return callspec.id if callspec else item.originalname.removeprefix('test_')


def pytest_collection_modifyitems(config, items):
    if config.getoption('list_scenarios'):
        for item in items:
            print(f'SCENARIO {scenario_name(item)}')
        items[:] = []
        return
    name = config.getoption('scenario')
    if name:
        selected = [i for i in items if scenario_name(i) == name]
        if not selected:
            raise pytest.UsageError(f'no scenario "{name}"')
        items[:] = selected


# ---------------- container environment ----------------

@pytest.fixture(scope='session', autouse=True)
def container_env(request):
    if request.config.getoption('list_scenarios'):
        yield
        return
    subprocess.run('mkdir -p /run/dbus && dbus-daemon --system --fork', shell=True, check=True)
    os.makedirs('/etc/polkit-1/rules.d', exist_ok=True)
    verdict = 'YES' if os.environ.get('POLKIT', 'yes') == 'yes' else 'NO'
    with open('/etc/polkit-1/rules.d/00-test.rules', 'w') as f:
        f.write(f'polkit.addRule(function(a, s) {{ if (s.user == "tester") return polkit.Result.{verdict}; }});\n')
    polkitd = next(p for p in ('/usr/lib/polkit-1/polkitd', '/usr/libexec/polkitd') if os.path.exists(p))
    subprocess.Popen(f'{polkitd} --no-debug >/tmp/polkitd.log 2>&1', shell=True)
    subprocess.Popen('Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >/dev/null 2>&1', shell=True)
    assert wait_for(lambda: os.path.exists('/tmp/.X11-unix/X99'), 15, 0.1), 'Xvfb did not bring up display :99'
    assert wait_for(lambda: os.path.exists('/run/dbus/system_bus_socket'), 15, 0.1), 'no system D-Bus bus'
    assert wait_for(lambda: pgrep('polkitd'), 15, 0.1), 'polkitd did not start'
    # The market prompt is not needed in tests.
    as_tester('mkdir -p ~/.config/dsh-launcher && echo \'{"dontAsk":true}\' > ~/.config/dsh-launcher/market-prompt.json')
    if os.path.exists(FAKEDSH_LOG):
        os.remove(FAKEDSH_LOG)
    # Fake GitHub: the list of releases and .deb files (a scenario may swap the list).
    write_releases(releases())
    subprocess.Popen(f'runuser -u tester -- python3 {SERVER} {PORT} {RELEASES} {DEBS} {DSH_RELEASES} >/tmp/github.out 2>&1', shell=True)
    assert wait_for(lambda: 'listening' in github_log(), 5, 0.1), 'the fake GitHub did not start'
    as_tester(f'mkdir -p {V8_COVERAGE}')
    yield
    collect_coverage()


LAUNCHER_SRC = 'file:///opt/dsh-launcher/'


def collect_coverage():
    """Coverage from the launcher processes (NODE_V8_COVERAGE) — into /results:
    only its own files (main.js, src/), no Electron and no other Node processes
    (npm, dsh). run.sh assembles the report across all scenarios."""
    if not os.path.isdir(V8_COVERAGE) or not os.path.isdir('/results'):
        return
    for name in os.listdir(V8_COVERAGE):
        try:
            with open(os.path.join(V8_COVERAGE, name)) as f:
                result = json.load(f).get('result', [])
        except (OSError, ValueError):
            continue  # the process was killed mid-write
        own = [r for r in result if r.get('url', '').startswith(LAUNCHER_SRC)
               and not r['url'].startswith(LAUNCHER_SRC + 'electron/')]
        if own:
            with open(f'/results/coverage-{uuid.uuid4().hex}.json', 'w') as f:
                json.dump({'result': own}, f)


# ---------------- test fixtures ----------------

WESTON_LOG = '/tmp/weston.log'


def os_version():
    with open('/etc/os-release') as f:
        return next((l.split('=', 1)[1].strip().strip('"') for l in f if l.startswith('VERSION_ID=')), '')


@pytest.fixture
def wayland():
    """Wayland session: headless weston from tester. Launcher environment —
    WAYLAND_DISPLAY, XDG_RUNTIME_DIR, XDG_SESSION_TYPE=wayland. Ubuntu 26.04
    only: the other scenarios and systems run under X11 (Xvfb)."""
    if os_version() != '26.04':
        pytest.skip('Wayland scenarios — only on Ubuntu 26.04')
    import pwd
    pw = pwd.getpwnam('tester')
    run_dir = f'/run/user/{pw.pw_uid}'
    os.makedirs(run_dir, exist_ok=True)
    os.chown(run_dir, pw.pw_uid, pw.pw_gid)
    os.chmod(run_dir, 0o700)
    env = {'HOME': pw.pw_dir, 'XDG_RUNTIME_DIR': run_dir, 'PATH': '/usr/sbin:/usr/bin:/sbin:/bin'}
    with open(WESTON_LOG, 'w') as log:
        weston = subprocess.Popen(['weston', '--backend=headless', '--socket=wayland-1', '--width=1280', '--height=800',
                                   '--idle-time=0'], user='tester', group='tester', extra_groups=[], env=env,
                                  stdout=log, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)
    sock = f'{run_dir}/wayland-1'
    assert wait_for(lambda: os.path.exists(sock), 15, 0.1), 'weston did not bring up the Wayland socket:\n' + open(WESTON_LOG).read()[-1500:]
    yield {'WAYLAND_DISPLAY': 'wayland-1', 'XDG_RUNTIME_DIR': run_dir, 'XDG_SESSION_TYPE': 'wayland'}
    weston.terminate()
    try:
        weston.wait(5)
    except subprocess.TimeoutExpired:
        weston.kill()


@pytest.fixture
def drv():
    """Window driver (CDP)."""
    return Driver()


@pytest.fixture(autouse=True)
def _dsh_releases():
    """dsh releases (the “What’s new” descriptions) — visible only in the
    scenario that created them: after the test the file is removed, so the
    other scenarios do not see it."""
    yield
    try:
        os.remove(DSH_RELEASES)
    except OSError:
        pass


@pytest.fixture
def start():
    """start(*env_sets, **env) → Launcher. At the end of the test the
    remaining launcher instances are SIGKILLed."""
    def _start(*envs, **kw):
        env = {}
        for e in envs:
            env.update(e)
        env.update(kw)
        return Launcher(env)
    yield _start
    for pid in launcher_pids():
        sh(f'kill -KILL {pid}')


@pytest.fixture
def gui(drv):
    """Main window with the dsh GUI."""
    return Gui(drv)


@pytest.fixture
def prompt(drv):
    """Update prompt in the main window."""
    return UpdatePrompt(drv)


@pytest.hookimpl(tryfirst=True)
def pytest_runtest_makereport(item, call):
    """Adds the tail of the launcher log to the error report."""
    if call.when == 'call' and call.excinfo is not None:
        item.add_report_section('call', 'launcher log tail', '\n'.join(launcher_log().splitlines()[-25:]))
        item.add_report_section('call', 'fake dsh log', fakedsh_events() or '(empty)')
        # launcher.log — with timestamp and pid, shared by the previous and restarted instances
        try:
            with open(f'{STATE}/launcher.log', encoding='utf-8', errors='replace') as f:
                tail = '\n'.join(f.read().splitlines()[-40:])
        except OSError:
            tail = '(none)'
        item.add_report_section('call', 'launcher.log', tail)
        if os.path.exists(WESTON_LOG):
            item.add_report_section('call', 'weston', '\n'.join(open(WESTON_LOG, errors='replace').read().splitlines()[-20:]))
        item.add_report_section('call', 'processes', sh('ps -eo pid,ppid,user,stat,etime,args --width 220')[1])
