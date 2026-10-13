"""Проверка обновлений, пока лаунчер работает (раз в 12 часов; в тесте — раз
в 2 с): новая версия → уведомление и пункт «Справка → Обновить … до X…»; его
окно — что нового и «Обновить сейчас» (перезапуск лаунчера, обновление без
вопроса) / «Позже»."""
import json
import os
import re

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_PKG, LATEST_RE, NO_PROMPTS, STATE
from helpers.launcher import launcher_pids
from helpers.procs import dpkg_version, fakedsh_starts, wait_exit, wait_for, wait_port
from helpers.windows import UpdateToast

# «последняя» версия dsh на npm — из файла: тест «выпускает» новую на ходу
DSH_LATEST = '/tmp/dsh-latest'
# «последняя» версия лаунчера на GitHub — тоже из файла (описания и .deb —
# последнего релиза в подставном GitHub)
LAUNCHER_LATEST = '/tmp/launcher-latest'


def test_update_poll(start, gui, drv):
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

    assert launcher.wait_log('уведомление: доступна dsh 99.0.0', 30), 'уведомление'
    assert launcher.wait_log('доступно обновление dsh 99.0.0 — пункт в меню «Справка»', 5), 'пункт меню'
    gui.mark()

    # плашка в окне: «Подробнее…» — окно обновления, плашка закрывается
    toast = UpdateToast(drv).wait_shown()
    assert 'dsh 99.0.0 is available' in toast.text() and 'installed: 0.2.0-rc.2' in toast.text(), toast.text()
    upd = toast.details('dsh')
    assert wait_for(lambda: not toast.is_shown(), 10), 'плашка закрылась'

    # «Позже» — ничего не происходит: dsh работает, GUI не перезагружался
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


def test_update_poll_setting(start, gui):
    """Галочка «Проверять обновления раз в 12 часов» действует сразу, без
    перезапуска; проверка при запуске — отдельная галочка."""
    launcher = start(FAKE, NO_PROMPTS, DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert launcher.wait_log('периодическая проверка обновлений: раз в 2 с', 5)
    settings = gui.open_settings()

    settings.set('pollUpdates', 'false')
    settings.wait_note('Saved')
    assert launcher.wait_log('периодическая проверка обновлений выключена', 5)
    assert json.load(open(f'{CONF}/settings.json')) == {'pollUpdates': False}
    assert not settings.restart_banner(), 'без перезапуска'

    settings.set('pollUpdates', 'true')
    settings.wait_note('Saved')
    assert launcher.wait_log_count('периодическая проверка обновлений: раз в 2 с', 2, 5), 'снова включена'
    assert json.load(open(f'{CONF}/settings.json')) == {}, 'по умолчанию — ключа нет'
    settings.close()
    launcher.term()


def test_update_now_launcher(start, gui, drv):
    """«Обновить сейчас» для самого лаунчера целиком: проверка по таймеру
    нашла новую версию → плашка → окно с «Что нового» → «Обновить сейчас» —
    перезапуск, новый экземпляр без вопроса скачивает последний (подставной)
    релиз, сверяет sha256, ставит через pkexec и перезапускается в новую
    версию; она поднимает dsh."""
    with open(LAUNCHER_LATEST, 'w') as f:
        f.write('1.0.0')          # при запуске новой версии нет — вопроса нет
    os.chmod(LAUNCHER_LATEST, 0o644)
    launcher = start(FAKE, DSH_LAUNCHER_FAKE_LAUNCHER_LATEST=LAUNCHER_LATEST, DSH_LAUNCHER_FAKE_DSH_LATEST='0.0.1',
                     DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert 'доступно обновление' not in launcher.log(), 'при запуске обновлений нет'

    with open(LAUNCHER_LATEST, 'w') as f:
        f.write(LATEST)           # «вышел» последний релиз (подставной GitHub)

    toast = UpdateToast(drv).wait_shown()
    assert f'DeepSeek Harness Launcher {LATEST} is available' in toast.text(), toast.text()
    upd = toast.details('launcher')
    upd.wait_text(rf'Version {LATEST_RE} is available \(installed: {re.escape(INSTALLED)}\).*What.s new: {LATEST_RE} ', 10)

    upd.update_now()
    assert launcher.wait_log(f'перезапуск лаунчера, чтобы обновить DeepSeek Harness Launcher до {LATEST}', 10)
    assert launcher.wait_exit(20) is not None, 'прежний экземпляр завершился'
    # вывод нового экземпляра — в тот же лог: сразу к установке, без вопроса
    assert launcher.wait_log('обновление launcher — по просьбе пользователя, без вопроса', 30), launcher.log()[-1500:]
    assert launcher.wait_log(f'dsh-launcher {LATEST} установлен', 400), launcher.log()[-1500:]
    assert 'совпадает с релизом' in launcher.log(), 'sha256 совпал с digest релиза'
    assert dpkg_version() == LATEST_PKG, f'dpkg: установлен {LATEST_PKG}'

    # затем — перезапуск в новую версию (её вывод — в launcher.log), она поднимает dsh
    assert wait_for(lambda: f'DeepSeek Harness Launcher {LATEST},' in _launcher_log() or
                    f'DSH Launcher {LATEST},' in _launcher_log(), 60), 'запущена новая версия'
    assert wait_port(60), 'новая версия подняла dsh'
    assert fakedsh_starts() == 2, 'dsh запускали первый экземпляр и новая версия; промежуточный — нет'
    for pid in launcher_pids():
        os.kill(pid, 15)
        wait_exit(pid, 15)


def _launcher_log():
    try:
        with open(f'{STATE}/launcher.log', encoding='utf-8', errors='replace') as f:
            return f.read()
    except OSError:
        return ''
