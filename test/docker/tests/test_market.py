"""Плагин маркета (dshmarket) перед запуском dsh: вопрос «Установить / Не
сейчас / Не спрашивать больше», установка с живым логом (и pnpm, если его
нет), ошибка установки. Плагин ставит подставной dsh (`plugin add`)."""
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
    """Вопрос включён (conftest по умолчанию его отключает), плагина нет."""
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
    """«Установить»: pnpm нет — лаунчер ставит его через npm (nvm
    пользователя), затем `dsh plugin add dshmarket` с живым логом; плагин в
    профиле, dsh запускается."""
    launcher = start(FAKE, NOUPD)
    market.wait(r'Install dshmarket\?.*plugin --profile web add dshmarket', 60)

    market.install()

    assert launcher.wait_log('pnpm нет — ставлю через npm', 10), launcher.log()[-1500:]
    assert launcher.wait_log('[launcher] dshmarket установлен', 300), launcher.log()[-1500:]
    assert json.load(open(PROFILE_PKG))['dependencies']['dshmarket'], 'плагин в профиле'
    plugin_add = [l for l in open(FAKEDSH_LOG) if 'plugin add dshmarket' in l]
    assert plugin_add and 'pnpm=None' not in plugin_add[0], f'pnpm в PATH команды: {plugin_add}'
    assert wait_port(60), 'dsh запущен'
    gui.wait()
    assert not os.path.exists(PREF), 'отказа не было — ничего не запоминается'
    launcher.term()


def test_market_fail(start, gui, market):
    """Установка не удалась — код и вывод команды на странице, вопрос снова;
    «Не сейчас» — dsh запускается без плагина, отказ не запоминается."""
    fake_pnpm()
    launcher = start(FAKE, NOUPD, FAKE_PLUGIN_FAIL='1')
    market.wait(r'Install dshmarket\?', 60)

    market.install()

    market.wait(r'Installation failed \(code 1\).*ERR_PNPM_FETCH_404', 60)
    assert 'установка dshmarket не удалась (код 1)' in launcher.log()
    market.skip()
    assert launcher.wait_log('установка маркета отложена (не сейчас)', 10)
    gui.wait()
    assert not os.path.exists(PROFILE_PKG) and not os.path.exists(PREF), 'плагина нет, отказ не запомнен'
    launcher.term()


def test_market_never(start, gui, market):
    """«Не спрашивать больше» — запоминается; следующий запуск без вопроса."""
    launcher = start(FAKE, NOUPD)
    market.wait(r'Install dshmarket\?', 60)

    market.never()

    assert launcher.wait_log('пользователь отказался от маркета навсегда', 10)
    gui.wait()
    assert json.load(open(PREF)) == {'dontAsk': True}, 'отказ записан'
    launcher.term()

    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert 'dshmarket не установлен, вопрос отключён' in launcher.log(), launcher.log()[-1500:]
    launcher.term()
