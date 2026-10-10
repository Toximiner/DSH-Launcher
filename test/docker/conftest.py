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
from helpers.env import FAKEDSH_LOG
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
    yield


# ---------------- фикстуры тестов ----------------

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
        item.add_report_section('call', 'процессы', sh('ps -eo pid,ppid,user,stat,etime,args --width 220')[1])
