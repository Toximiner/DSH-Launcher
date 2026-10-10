"""Проверка обновлений, пока лаунчер работает (раз в 12 часов; в тесте — раз
в 2 с): новая версия → уведомление и пункт «Справка → Обновить … до X…»; его
окно — что нового и «Обновить сейчас» (перезапуск лаунчера, обновление без
вопроса) / «Позже»."""
import os

from helpers.env import FAKE
from helpers.procs import fakedsh_starts, wait_exit

# «последняя» версия dsh на npm — из файла: тест «выпускает» новую на ходу
DSH_LATEST = '/tmp/dsh-latest'


def test_update_poll(start, gui):
    with open(DSH_LATEST, 'w') as f:
        f.write('0.0.1')          # при запуске новой версии нет — вопроса нет
    os.chmod(DSH_LATEST, 0o644)
    launcher = start(FAKE, DSH_LAUNCHER_FAKE_LAUNCHER_LATEST='1.0.0', DSH_LAUNCHER_FAKE_DSH_LATEST=DSH_LATEST,
                     DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert launcher.wait_log('периодическая проверка обновлений: раз в 2 с', 5), launcher.log()[-800:]
    assert 'доступно обновление' not in launcher.log(), 'при запуске обновлений нет'

    with open(DSH_LATEST, 'w') as f:
        f.write('99.0.0')         # вышла новая версия dsh

    assert launcher.wait_log('уведомление: доступна dsh 99.0.0', 30), 'системное уведомление'
    assert launcher.wait_log('доступно обновление dsh 99.0.0 — пункт в меню «Справка»', 5), 'пункт меню'
    gui.mark()

    # «Позже» — ничего не происходит: dsh работает, GUI не перезагружался
    upd = gui.open_update_now('dsh')
    upd.wait_text(r'Version 99\.0\.0 is available \(installed: 0\.2\.0-rc\.2\)', 5)
    upd.wait_text('The launcher will restart and dsh will be stopped', 5)
    upd.later()
    assert launcher.alive and gui.not_reloaded(), '«Позже» — лаунчер и GUI dsh как были'

    # «Обновить сейчас» — перезапуск; новый экземпляр сразу к установке dsh
    gui.open_update_now('dsh').update_now()
    assert launcher.wait_log('перезапуск лаунчера, чтобы обновить dsh до 99.0.0', 10)
    assert launcher.wait_exit(20) is not None, 'прежний экземпляр завершился'
    new = launcher.successor()
    assert new, 'лаунчер перезапустился'
    # вывод нового экземпляра — в тот же лог
    assert launcher.wait_log('обновление dsh — по просьбе пользователя, без вопроса', 30), launcher.log()[-1500:]
    assert launcher.wait_log('новый @deepseek-ai/dsh: 99.0.0', 30), launcher.log()[-1500:]
    # npm в PATH нет — установка остановилась на подсказке; dsh новый экземпляр
    # до обновления не запускал
    assert fakedsh_starts() == 1, 'dsh запускал только прежний экземпляр'
    os.kill(new, 15)
    assert wait_exit(new, 15) is not None, 'новый экземпляр штатно завершился'
