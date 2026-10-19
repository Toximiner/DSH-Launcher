"""Установка и обновление dsh через npm при запуске.

В образе стоит настоящий старый dsh 0.1.7-rc.2: в nvm пользователя (prefix
доступен на запись), в ~/.local («другой prefix») и в /opt/noderoot (prefix
принадлежит root). «Последний» на npm подменён на 0.2.0-rc.2 (DSH_LATEST в
helpers/env.py): живой реестр живёт своей жизнью — вышедший 0.2.1-rc.2
уронил эти сценарии, хотя код не менялся.
"""
import json
import os
import shutil

from helpers.env import CONF, HOME
from helpers.procs import real_dsh_gone, sh, wait_for, wait_port

NVM_NODE = f'{HOME}/.nvm/versions/node'
NVM_BIN = f'{NVM_NODE}/{sorted(os.listdir(NVM_NODE))[0]}/bin' if os.path.isdir(NVM_NODE) else ''
OLD_LAUNCHER = {'DSH_LAUNCHER_FAKE_INSTALLED': '9.9.9-1'}  # вопрос об обновлении лаунчера не нужен


def test_dsh_update_auto(start, prompt):
    """prefix доступен на запись — лаунчер обновляет dsh сам и запускает новый."""
    launcher = start(OLD_LAUNCHER)
    prompt.wait(r'Update DeepSeek Harness.*0\.2\.0-rc\.2.*installed: 0\.1\.7-rc\.2')

    prompt.install()

    assert launcher.wait_log('обновлён → 0.2.0-rc.2', 300), 'npm install -g прошёл, dsh обновлён'
    _, out = sh(f'runuser -u tester -- env PATH={NVM_BIN}:/usr/bin:/bin {NVM_BIN}/dsh --version')
    assert out.splitlines()[-1:] == ['0.2.0-rc.2'], f'в nvm теперь dsh 0.2.0-rc.2: {out}'
    assert wait_port(120), 'лаунчер перешёл к запуску dsh (порт поднят)'
    assert launcher.term() is not None, 'лаунчер вышел по SIGTERM'
    assert real_dsh_gone(), 'настоящий dsh остановлен'


def test_dsh_update_other_prefix(start, prompt):
    """npm ставит в один prefix, а запускается dsh из другого — команда для
    терминала (без sudo)."""
    launcher = start(OLD_LAUNCHER, DSH_BIN=f'{HOME}/.local/bin/dsh', PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    prompt.wait(r'npm installed the package into /home/tester/\.nvm.*'
                r'runs /home/tester/\.local/bin/dsh \(version 0\.1\.7-rc\.2\)', 300)
    prompt.wait(r'Run in a terminal: npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2', 5)
    prompt.later()
    assert wait_port(120), '«Не сейчас» — dsh запускается'
    launcher.term()


def test_dsh_update_root_prefix(start, prompt):
    """prefix принадлежит root, npm системный — установка идёт через pkexec:
    polkit спрашивает пароль системным диалогом (в контейнере правило
    разрешает), dsh обновляется и запускается."""
    launcher = start(OLD_LAUNCHER, DSH_BIN='/opt/noderoot/bin/dsh')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    assert launcher.wait_log('dsh обновлён → 0.2.0-rc.2', 300), launcher.log()[-2000:]
    # runuser сбрасывает окружение, а dsh — «#!/usr/bin/env node»: node лежит
    # в /opt/noderoot/bin, поэтому PATH задаём явно (как в dsh_update_auto).
    _, out = sh('runuser -u tester -- env PATH=/opt/noderoot/bin:/usr/bin:/bin /opt/noderoot/bin/dsh --version')
    assert out.splitlines()[-1:] == ['0.2.0-rc.2'], f'в /opt/noderoot теперь dsh 0.2.0-rc.2: {out}'
    assert wait_port(120), 'лаунчер перешёл к запуску dsh (порт поднят)'
    assert launcher.term() is not None, 'лаунчер вышел по SIGTERM'
    assert real_dsh_gone(), 'настоящий dsh остановлен'


def test_dsh_update_root_prefix_nopkexec(start, prompt):
    """root-prefix, но pkexec нет — сразу команда с sudo; «Не спрашивать
    больше» запоминается на версию."""
    shutil.move('/usr/bin/pkexec', '/usr/bin/pkexec.off')
    launcher = start(OLD_LAUNCHER, DSH_BIN='/opt/noderoot/bin/dsh')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    prompt.wait(r'needs sudo.*sudo npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2')
    prompt.never()
    assert wait_port(120), '«Не спрашивать больше» — dsh запускается'
    prefs = json.load(open(f'{CONF}/update-prefs.json'))
    assert prefs.get('dsh') == '0.2.0-rc.2', f'отказ запомнен на версию: {prefs}'
    launcher.term()


def test_dsh_install(start, prompt):
    """dsh нет нигде — страница установки; «Установить» ставит его через npm
    (prefix nvm доступен на запись), лаунчер запускает установленный dsh."""
    for link in (f'{NVM_BIN}/dsh', f'{HOME}/.local/bin/dsh'):  # оставляем только npm
        sh(f'rm -f {link}')
    launcher = start(OLD_LAUNCHER, PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    prompt.wait(r'could not find dsh.*Install', 60)

    prompt.drv.go('dshlauncher://install/run/')

    assert launcher.wait_log('dsh установлен — продолжаю запуск', 300), 'npm install -g прошёл'
    assert wait_port(120), 'установленный dsh запущен (порт поднят)'
    assert launcher.term() is not None, 'лаунчер вышел по SIGTERM'
    assert wait_for(real_dsh_gone, 5), 'настоящий dsh остановлен'
