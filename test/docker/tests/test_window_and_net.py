"""The window size between runs, «Restart dsh», the system proxy, the release cache."""
import os
import re
import subprocess

from helpers.env import CONF, FAKE, NO_PROMPTS, NOUPD, PROXY, PROXY_LOG
from helpers.github import github_log
from helpers.procs import as_tester, dsh_tree_alive, dsh_tree_gone, fakedsh_events, fakedsh_starts, wait_for


def test_window_state(start, gui):
    """The saved size is applied; without a saved one — the default, but not
    larger than the screen (Xvfb 1280×800); on close the size is recorded."""
    state = f'{CONF}/window-state.json'
    as_tester(f"mkdir -p {CONF} && echo '{{\"width\":1000,\"height\":640}}' > {state}")

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert gui.outer_width() == 1000, 'saved width 1000 applied'
    gui.close()
    assert launcher.wait_exit(15) is not None, 'launcher exited'
    assert '"width":1000' in open(state).read(), 'size recorded on close'

    os.remove(state)
    launcher = start(FAKE, NOUPD)
    gui.wait()
    width = gui.outer_width() or 0
    assert 760 <= width <= 1280, f'default, but not wider than the screen: {width}'
    gui.close()
    assert launcher.wait_exit(15) is not None, 'launcher exited'
    assert '"width":' in open(state).read(), 'size recorded on close'


def test_restart_dsh(start, gui):
    """«File → Restart dsh»: the old dsh is stopped (SIGTERM), the new one is
    started, the window is back on the GUI — without the «dsh has stopped»
    page."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert dsh_tree_alive(), 'fakedsh tree is alive'

    gui.restart_dsh()

    assert launcher.wait_log('перезапуск dsh по запросу', 15), 'on-request restart'
    assert wait_for(lambda: fakedsh_starts() >= 2, 30), 'dsh started again'
    assert 'SIGTERM' in fakedsh_events(), 'previous dsh received SIGTERM'
    gui.wait()
    assert not re.search(r'dsh has stopped|dsh остановился', launcher.log()), 'without the «dsh has stopped» page'
    assert dsh_tree_alive(), 'new fakedsh tree is alive'
    assert launcher.term() is not None, 'launcher exited cleanly'
    assert dsh_tree_gone(), 'dsh tree stopped'


def test_proxy(start, gui):
    """The update check through the system proxy (session.resolveProxy →
    the Node agent); in the container the proxy is set via environment
    variables."""
    if os.path.exists(PROXY_LOG):
        os.remove(PROXY_LOG)
    subprocess.Popen(f'runuser -u tester -- python3 {PROXY} 3129 >/dev/null 2>&1', shell=True)
    assert wait_for(lambda: os.path.exists(PROXY_LOG) and 'listening' in open(PROXY_LOG).read(), 5, 0.25), 'proxy started'
    url = 'http://127.0.0.1:3129'
    launcher = start(FAKE, NO_PROMPTS, HTTPS_PROXY=url, https_proxy=url, HTTP_PROXY=url, http_proxy=url,
                     DSH_LAUNCHER_GITHUB_API=None)  # the real GitHub: we check the tunnel to it
    gui.wait()  # localhost — bypasses the proxy

    assert launcher.wait_log('последний @deepseek-ai/dsh', 30), 'npm update check passed'
    proxied = open(PROXY_LOG).read()
    assert 'CONNECT registry.npmjs.org:443' in proxied, 'npm request went through the proxy'
    assert 'CONNECT api.github.com:443' in proxied, 'GitHub request went through the proxy'
    launcher.term()


def test_releases_cache(start, gui):
    """The GitHub release list is remembered with an ETag: the next run asks
    with If-None-Match — 304 «not modified» (GitHub's rate limit is not
    consumed, a new release is visible at once); GitHub unavailable — the
    list comes from the cache (if it is not older than an hour). GitHub is
    the fake one: the requests are visible in its log."""
    github = lambda: github_log().count('/releases?per_page=100 ')

    for run in (1, 2):
        launcher = start(FAKE, NO_PROMPTS)
        gui.wait()
        assert launcher.wait_log('последний dsh-launcher', 30), f'run {run}: launcher update check passed'
        assert launcher.wait_log('последний @deepseek-ai/dsh', 30), f'run {run}: dsh check passed'
        if run == 2:
            # 304 from both repositories (the launcher also fetches dsh releases) — by the ETag
            assert github_log().count('/releases?per_page=100 304') >= 2, 'second run — 304 by ETag'
        launcher.term()
    assert github() == 4, f'GitHub queried on every run ({github()})'

    # GitHub is unavailable (nobody on the port) — the lists come from the recent cache
    launcher = start(FAKE, NO_PROMPTS, DSH_LAUNCHER_GITHUB_API='http://127.0.0.1:8799')
    gui.wait()
    assert launcher.wait_log('последний dsh-launcher', 30), 'launcher check — from cache'
    assert launcher.wait_log('последний @deepseek-ai/dsh', 30), 'dsh check — from cache'
    assert launcher.log().count('— список из кэша') >= 2, 'both lists (launcher + dsh) taken from cache'
    launcher.term()
    assert os.path.exists(f'{CONF}/releases-cache.json'), 'release cache written'
    assert os.path.exists(f'{CONF}/dsh-releases-cache.json'), 'dsh release cache written'
