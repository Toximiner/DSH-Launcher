"""The market plugin (dshmarket) before dsh starts: the prompt «Install / Not
now / Don't ask again», installation with a live log (and pnpm, if there is
none), install failure. The plugin installs a stub dsh (`plugin add`)."""
import json
import os

import pytest

from helpers.env import CONF, FAKE, FAKEDSH_LOG, HOME, NOUPD
from helpers.procs import as_tester, wait_port
from helpers.windows import MarketPrompt

PREF = f'{CONF}/market-prompt.json'
PROFILE_PKG = f'{HOME}/.dsh/profiles/web/package.json'
FAKE_PNPM = '/usr/local/bin/pnpm'


@pytest.fixture
def market(drv):
    """The prompt is enabled (conftest disables it by default), the plugin is absent."""
    for f in (PREF, PROFILE_PKG):
        if os.path.exists(f):
            os.remove(f)
    yield MarketPrompt(drv)
    if os.path.exists(FAKE_PNPM):
        os.remove(FAKE_PNPM)


def fake_pnpm():
    with open(FAKE_PNPM, 'w') as f:
        f.write('#!/bin/sh\nexit 0\n')
    os.chmod(FAKE_PNPM, 0o755)


def test_market_install(start, gui, market):
    """«Install»: no pnpm — the launcher installs it via npm (the user's
    nvm), then `dsh plugin add dshmarket` with a live log; the plugin is in
    the profile, dsh starts."""
    launcher = start(FAKE, NOUPD)
    market.wait(r'Install dshmarket\?.*plugin --profile web add dshmarket', 60)

    market.install()

    assert launcher.wait_log('pnpm нет — ставлю через npm', 10), launcher.log()[-1500:]
    assert launcher.wait_log('[launcher] dshmarket установлен', 300), launcher.log()[-1500:]
    assert json.load(open(PROFILE_PKG))['dependencies']['dshmarket'], 'the plugin is in the profile'
    plugin_add = [l for l in open(FAKEDSH_LOG) if 'plugin add dshmarket' in l]
    assert plugin_add and 'pnpm=None' not in plugin_add[0], f'pnpm is in the command PATH: {plugin_add}'
    assert wait_port(60), 'dsh started'
    gui.wait()
    assert not os.path.exists(PREF), 'no refusal — nothing is remembered'
    launcher.term()


def test_market_fail(start, gui, market):
    """The installation failed — the code and the command output on the page, the prompt
    again; «Not now» — dsh starts without the plugin, the refusal is not remembered."""
    fake_pnpm()
    launcher = start(FAKE, NOUPD, FAKE_PLUGIN_FAIL='1')
    market.wait(r'Install dshmarket\?', 60)

    market.install()

    market.wait(r'Installation failed \(code 1\).*ERR_PNPM_FETCH_404', 60)
    assert 'установка dshmarket не удалась (код 1)' in launcher.log()
    market.skip()
    assert launcher.wait_log('установка маркета отложена (не сейчас)', 10)
    gui.wait()
    assert not os.path.exists(PROFILE_PKG) and not os.path.exists(PREF), 'no plugin, the refusal is not remembered'
    launcher.term()


def test_market_never(start, gui, market):
    """«Don't ask again» — remembered; the next start without the prompt."""
    launcher = start(FAKE, NOUPD)
    market.wait(r'Install dshmarket\?', 60)

    market.never()

    assert launcher.wait_log('пользователь отказался от маркета навсегда', 10)
    gui.wait()
    assert json.load(open(PREF)) == {'dontAsk': True}, 'the refusal was written'
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'dshmarket не установлен, вопрос отключён' in launcher.log(), launcher.log()[-1500:]
    launcher.term()
