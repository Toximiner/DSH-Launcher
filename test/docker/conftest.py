"""Фикстуры Docker-сценариев.

Каждый сценарий — в свежем контейнере: run.sh вызывает
`pytest /opt/t/tests --scenario <имя>`. Имя сценария — имя теста без «test_»;
у параметризованного теста — id параметра (test_adopt[adopt_detached] →
adopt_detached). `--list-scenarios` печатает все имена (run.sh берёт из них
список по умолчанию).

Окружение контейнера: system dbus, polkitd (правило «tester — разрешить /
запретить» по POLKIT=yes|no), Xvfb; лаунчер — от tester с CDP на 9222.
"""
import os
import subprocess

import pytest

from helpers.cdp import Driver
from helpers.env import FAKEDSH_LOG, STATE
from helpers.github import DEBS, PORT, RELEASES, SERVER, github_log, releases, write_releases
from helpers.launcher import Launcher, launcher_log, launcher_pids
from helpers.procs import as_tester, fakedsh_events, pgrep, sh, wait_for
from helpers.windows import Gui, UpdatePrompt


# ---------------- выбор сценария ----------------

def pytest_addoption(parser):
    parser.addoption('--scenario', help='запустить один сценарий по имени')
    parser.addoption('--list-scenarios', action='store_true', help='напечатать имена сценариев')


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
            raise pytest.UsageError(f'нет сценария «{name}»')
        items[:] = selected


# ---------------- окружение контейнера ----------------

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
    assert wait_for(lambda: os.path.exists('/tmp/.X11-unix/X99'), 15, 0.1), 'Xvfb не поднял дисплей :99'
    assert wait_for(lambda: os.path.exists('/run/dbus/system_bus_socket'), 15, 0.1), 'нет системной шины D-Bus'
    assert wait_for(lambda: pgrep('polkitd'), 15, 0.1), 'polkitd не запустился'
    # Вопрос про маркет в тестах не нужен.
    as_tester('mkdir -p ~/.config/dsh-launcher && echo \'{"dontAsk":true}\' > ~/.config/dsh-launcher/market-prompt.json')
    if os.path.exists(FAKEDSH_LOG):
        os.remove(FAKEDSH_LOG)
    # Подставной GitHub: список релизов и .deb (сценарий может подменить список).
    write_releases(releases())
    subprocess.Popen(f'runuser -u tester -- python3 {SERVER} {PORT} {RELEASES} {DEBS} >/tmp/github.out 2>&1', shell=True)
    assert wait_for(lambda: 'listening' in github_log(), 5, 0.1), 'подставной GitHub не запустился'
    yield


# ---------------- фикстуры тестов ----------------

WESTON_LOG = '/tmp/weston.log'


def os_version():
    with open('/etc/os-release') as f:
        return next((l.split('=', 1)[1].strip().strip('"') for l in f if l.startswith('VERSION_ID=')), '')


@pytest.fixture
def wayland():
    """Wayland-сеанс: безголовый weston от tester. Окружение для лаунчера —
    WAYLAND_DISPLAY, XDG_RUNTIME_DIR, XDG_SESSION_TYPE=wayland. Только на Ubuntu
    26.04: остальные сценарии и системы — под X11 (Xvfb)."""
    if os_version() != '26.04':
        pytest.skip('Wayland-сценарии — только на Ubuntu 26.04')
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
    assert wait_for(lambda: os.path.exists(sock), 15, 0.1), 'weston не поднял Wayland-сокет:\n' + open(WESTON_LOG).read()[-1500:]
    yield {'WAYLAND_DISPLAY': 'wayland-1', 'XDG_RUNTIME_DIR': run_dir, 'XDG_SESSION_TYPE': 'wayland'}
    weston.terminate()
    try:
        weston.wait(5)
    except subprocess.TimeoutExpired:
        weston.kill()


@pytest.fixture
def drv():
    """Драйвер окон (CDP)."""
    return Driver()


@pytest.fixture
def start():
    """start(*наборы_env, **env) → Launcher. В конце теста оставшиеся
    экземпляры лаунчера — SIGKILL."""
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
    """Главное окно с GUI dsh."""
    return Gui(drv)


@pytest.fixture
def prompt(drv):
    """Вопрос об обновлении в главном окне."""
    return UpdatePrompt(drv)


@pytest.hookimpl(tryfirst=True)
def pytest_runtest_makereport(item, call):
    """В отчёт об ошибке — хвост лога лаунчера."""
    if call.when == 'call' and call.excinfo is not None:
        item.add_report_section('call', 'хвост лога лаунчера', '\n'.join(launcher_log().splitlines()[-25:]))
        item.add_report_section('call', 'журнал фейкового dsh', fakedsh_events() or '(пусто)')
        # launcher.log — со временем и pid, общий для прежнего и перезапущенного экземпляров
        try:
            with open(f'{STATE}/launcher.log', encoding='utf-8', errors='replace') as f:
                tail = '\n'.join(f.read().splitlines()[-40:])
        except OSError:
            tail = '(нет)'
        item.add_report_section('call', 'launcher.log', tail)
        if os.path.exists(WESTON_LOG):
            item.add_report_section('call', 'weston', '\n'.join(open(WESTON_LOG, errors='replace').read().splitlines()[-20:]))
        item.add_report_section('call', 'процессы', sh('ps -eo pid,ppid,user,stat,etime,args --width 220')[1])
