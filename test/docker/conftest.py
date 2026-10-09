"""Фикстуры Docker-сценариев. Каждый сценарий — в свежем контейнере (run.sh
запускает `pytest test_scenarios.py::test_<имя>` от root): system dbus,
polkitd (правило «tester — разрешить/запретить» по POLKIT=yes|no), Xvfb;
лаунчер — от tester с --remote-debugging-port=9222."""
import os
import subprocess
import time

import pytest

import lib
from cdp import Driver


@pytest.fixture(scope='session', autouse=True)
def container_env():
    subprocess.run('mkdir -p /run/dbus && dbus-daemon --system --fork', shell=True, check=True)
    os.makedirs('/etc/polkit-1/rules.d', exist_ok=True)
    verdict = 'YES' if os.environ.get('POLKIT', 'yes') == 'yes' else 'NO'
    with open('/etc/polkit-1/rules.d/00-test.rules', 'w') as f:
        f.write(f'polkit.addRule(function(a, s) {{ if (s.user == "tester") return polkit.Result.{verdict}; }});\n')
    polkitd = next(p for p in ('/usr/lib/polkit-1/polkitd', '/usr/libexec/polkitd') if os.path.exists(p))
    subprocess.Popen(f'{polkitd} --no-debug >/tmp/polkitd.log 2>&1', shell=True)
    subprocess.Popen('Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >/dev/null 2>&1', shell=True)
    time.sleep(1.5)
    # Вопрос про маркет в тестах не нужен.
    lib.as_tester('mkdir -p ~/.config/dsh-launcher && echo \'{"dontAsk":true}\' > ~/.config/dsh-launcher/market-prompt.json')
    if os.path.exists(lib.FAKEDSH_LOG):
        os.remove(lib.FAKEDSH_LOG)
    yield


@pytest.fixture
def drv():
    return Driver()


@pytest.fixture
def start():
    """start(**env) → Launcher; в конце теста оставшиеся лаунчеры — SIGKILL."""
    def _start(*envs, **kw):
        env = {}
        for e in envs:
            env.update(e)
        env.update(kw)
        return lib.Launcher(env)
    yield _start
    for pid in lib.launcher_pids():
        lib.sh(f'kill -KILL {pid}')


@pytest.hookimpl(tryfirst=True)
def pytest_runtest_makereport(item, call):
    """В отчёт об ошибке — хвост лога лаунчера (как раньше в scenario.sh)."""
    if call.when == 'call' and call.excinfo is not None:
        tail = '\n'.join(lib.log().splitlines()[-25:])
        item.add_report_section('call', 'хвост лога лаунчера', tail)
