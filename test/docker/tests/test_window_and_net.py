"""Размер окна между запусками, «Перезапустить dsh», системный прокси, кэш релизов."""
import os
import re
import subprocess

from helpers.env import CONF, FAKE, NO_PROMPTS, NOUPD, PROXY, PROXY_LOG
from helpers.procs import as_tester, dsh_tree_alive, dsh_tree_gone, fakedsh_events, fakedsh_starts, wait_for


def test_window_state(start, gui):
    """Сохранённый размер применяется; без сохранённого — по умолчанию, но не
    больше экрана (Xvfb 1280×800); при закрытии размер записывается."""
    state = f'{CONF}/window-state.json'
    as_tester(f"mkdir -p {CONF} && echo '{{\"width\":1000,\"height\":640}}' > {state}")

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert gui.outer_width() == 1000, 'сохранённая ширина 1000 применена'
    gui.close()
    assert launcher.wait_exit(15) is not None, 'лаунчер завершился'
    assert '"width":1000' in open(state).read(), 'при закрытии размер записан'

    os.remove(state)
    launcher = start(FAKE, NOUPD)
    gui.wait()
    width = gui.outer_width() or 0
    assert 760 <= width <= 1280, f'по умолчанию, но не шире экрана: {width}'
    gui.close()
    assert launcher.wait_exit(15) is not None, 'лаунчер завершился'
    assert '"width":' in open(state).read(), 'размер записан при закрытии'


def test_restart_dsh(start, gui):
    """«Файл → Перезапустить dsh»: прежний dsh остановлен (SIGTERM), новый
    запущен, окно снова на GUI — без страницы «dsh остановился»."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert dsh_tree_alive(), 'дерево fakedsh живо'

    gui.restart_dsh()

    assert launcher.wait_log('перезапуск dsh по запросу', 15), 'перезапуск по запросу'
    assert wait_for(lambda: fakedsh_starts() >= 2, 30), 'dsh запущен заново'
    assert 'SIGTERM' in fakedsh_events(), 'прежний dsh получил SIGTERM'
    gui.wait()
    assert not re.search(r'dsh has stopped|dsh остановился', launcher.log()), 'без страницы «dsh остановился»'
    assert dsh_tree_alive(), 'новое дерево fakedsh живо'
    assert launcher.term() is not None, 'лаунчер штатно завершился'
    assert dsh_tree_gone(), 'дерево dsh остановлено'


def test_proxy(start, gui):
    """Проверка обновлений через системный прокси (session.resolveProxy →
    агент Node); в контейнере прокси задан переменными окружения."""
    if os.path.exists(PROXY_LOG):
        os.remove(PROXY_LOG)
    subprocess.Popen(f'runuser -u tester -- python3 {PROXY} 3129 >/dev/null 2>&1', shell=True)
    assert wait_for(lambda: os.path.exists(PROXY_LOG) and 'listening' in open(PROXY_LOG).read(), 5, 0.25), 'прокси запущен'
    url = 'http://127.0.0.1:3129'
    launcher = start(FAKE, NO_PROMPTS, HTTPS_PROXY=url, https_proxy=url, HTTP_PROXY=url, http_proxy=url)
    gui.wait()  # localhost — мимо прокси

    assert launcher.wait_log('последний @deepseek-ai/dsh', 30), 'проверка обновлений npm прошла'
    proxied = open(PROXY_LOG).read()
    assert 'CONNECT registry.npmjs.org:443' in proxied, 'запрос к npm шёл через прокси'
    assert 'CONNECT api.github.com:443' in proxied, 'запрос к GitHub шёл через прокси'
    launcher.term()


def test_releases_cache(start, gui):
    """Список релизов GitHub запоминается: второй запуск в течение часа в
    GitHub не ходит (по журналу прокси), npm — спрашивается как обычно."""
    if os.path.exists(PROXY_LOG):
        os.remove(PROXY_LOG)
    subprocess.Popen(f'runuser -u tester -- python3 {PROXY} 3129 >/dev/null 2>&1', shell=True)
    assert wait_for(lambda: os.path.exists(PROXY_LOG) and 'listening' in open(PROXY_LOG).read(), 5, 0.25), 'прокси запущен'
    url = 'http://127.0.0.1:3129'
    via_proxy = {'HTTPS_PROXY': url, 'https_proxy': url, 'HTTP_PROXY': url, 'http_proxy': url}
    github = lambda: open(PROXY_LOG).read().count('CONNECT api.github.com:443')
    npm = lambda: open(PROXY_LOG).read().count('CONNECT registry.npmjs.org:443')

    for run in (1, 2):
        launcher = start(FAKE, NO_PROMPTS, via_proxy)
        gui.wait()
        assert launcher.wait_log('последний dsh-launcher', 30), f'запуск {run}: проверка обновлений лаунчера прошла'
        assert launcher.wait_log('последний @deepseek-ai/dsh', 30), f'запуск {run}: проверка dsh прошла'
        launcher.term()

    assert github() == 1, f'GitHub спрошен один раз за два запуска ({github()})'
    assert npm() == 2, f'npm — при каждом запуске ({npm()})'
    assert os.path.exists(f'{CONF}/releases-cache.json'), 'кэш релизов записан'
