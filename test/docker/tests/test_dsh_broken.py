"""dsh найден, но не запускается (битый бинарник): страница установки с
причиной «не запускается»; «Скопировать команду» и «Проверить ещё раз»."""
from helpers.env import NOUPD
from helpers.procs import as_tester

# /tmp — общая папка контейнера: битый бинарник создаём от root.
BROKEN = '/tmp/dsh-broken'
PAGE = 'DeepSeek Harness is required'


def make_broken():
    as_tester("echo '#!/bin/sh' > /tmp/dsh-broken")
    as_tester('echo \'echo "dsh: error: boom" >&2\' >> /tmp/dsh-broken')
    as_tester('echo "exit 7" >> /tmp/dsh-broken')
    as_tester('chmod 755 /tmp/dsh-broken')


def test_dsh_broken(start, drv):
    make_broken()
    launcher = start(NOUPD, DSH_BIN=BROKEN)

    # Стартовая страница: битый бинарник — «не запускается», не «не найден».
    drv.wait_text(PAGE + r'.*the /tmp/dsh-broken binary fails to run', 30)
    drv.wait_text('dsh: error: boom', 10)

    # «Скопировать команду» — команда в буфер, сообщение на странице.
    drv.go('dshlauncher://install/copy/')
    assert launcher.wait_log('команда установки скопирована в буфер обмена', 30), launcher.log()[-2000:]
    drv.wait_text('Command copied to the clipboard — paste it into a terminal.', 10)

    # «Проверить ещё раз» — бинарник всё ещё битый: причина та же.
    drv.go('dshlauncher://install/recheck/')
    assert launcher.wait_log('повторная проверка: dsh найден, но не запускается', 30), launcher.log()[-2000:]
    drv.wait_text('dsh was found but does not run — see the reason below.', 10)
    drv.wait_text('the /tmp/dsh-broken binary fails to run', 10)

    assert launcher.term() is not None, 'лаунчер штатно завершился'
