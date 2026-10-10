"""Самообновление лаунчера: настоящий последний релиз с GitHub ставится поверх
пакета 1.1.0-1 из образа (через pkexec), и «Что нового».
"""
import json
import os
import re
import shutil

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_DEB, LATEST_PKG, LATEST_RE, NO_PROMPTS, UPD
from helpers.procs import as_tester, dpkg_version, fakedsh_starts, sh, wait_exit, wait_for, wait_port
from helpers.windows import WhatsNewDialog


def _check_new_instance(launcher):
    """После обновления старый экземпляр выходит, новый поднимает dsh;
    старый dsh не запускал (один старт фейкового dsh — у нового)."""
    assert launcher.wait_exit(20) is not None, 'старый экземпляр завершился'
    new = launcher.successor()
    assert new, 'новый экземпляр запущен'
    assert wait_port(60), 'новый экземпляр поднял dsh'
    assert fakedsh_starts() == 1, 'старый экземпляр после обновления dsh не запускал'
    sh(f'kill -TERM {new}')
    wait_exit(new, 15)


def test_launcher_update(start, prompt):
    """Вопрос с «Что нового» → «Обновить» → скачано, sha256 сверен, pkexec
    поставил пакет, лаунчер перезапустился в новую версию."""
    launcher = start(FAKE)
    prompt.wait(rf'Update DSH Launcher.*Version {LATEST_RE} is available \(installed: {re.escape(INSTALLED)}\s*\)')
    # «Что нового»: от последней версии вниз до 1.1.1 (первая после 1.1.0)
    prompt.wait(rf'What.s new: {LATEST_RE} .*1\.1\.1 ', 5)
    page_url = prompt.url()
    prompt.mark()
    prompt.click_release_link()
    # ссылка открывается снаружи (shell.openExternal), окно остаётся на вопросе
    assert prompt.url() == page_url and prompt.not_reloaded(), 'клик по ссылке релиза не увёл окно'

    prompt.install()

    assert launcher.wait_log(f'dsh-launcher {LATEST} установлен', 400), 'скачано и установлено через pkexec'
    assert 'совпадает с релизом' in launcher.log(), 'sha256 совпал с digest релиза'
    assert dpkg_version() == LATEST_PKG, f'dpkg: установлен {LATEST_PKG}'
    _check_new_instance(launcher)
    assert not [f for f in os.listdir(UPD) if f.endswith('.deb')], 'скачанный .deb удалён'


def test_launcher_update_denied(start, prompt):
    """polkit отказал — команда для терминала; пользователь ставит пакет сам,
    «Проверить ещё раз» видит новую версию и перезапускает лаунчер."""
    launcher = start(FAKE)
    prompt.wait('Update DSH Launcher')

    prompt.install()

    prompt.wait(rf'Update manually.*Installation failed \(code 127\).*sudo apt install -y {re.escape(UPD)}/{re.escape(LATEST_DEB)}', 400)
    assert dpkg_version() == INSTALLED, f'dpkg: всё ещё {INSTALLED}'
    sh(f'apt-get install -y {UPD}/{LATEST_DEB}', check=True)  # пользователь ставит в терминале
    prompt.recheck()
    assert launcher.wait_log(f'обновлён до {LATEST_PKG} (вручную)', 30), '«Проверить ещё раз» — «Update installed»'
    _check_new_instance(launcher)


def test_launcher_update_nopkexec(start, prompt):
    """pkexec нет — сразу команда для терминала; «Не сейчас» — dsh запускается."""
    shutil.move('/usr/bin/pkexec', '/usr/bin/pkexec.off')
    launcher = start(FAKE)
    prompt.wait('Update DSH Launcher')

    prompt.install()

    prompt.wait(rf'pkexec was not found.*sudo apt install -y {re.escape(UPD)}/{re.escape(LATEST_DEB)}', 400)
    prompt.later()
    assert wait_port(60), '«Не сейчас» — dsh запускается'
    launcher.term()


def test_whats_new(start, gui, drv):
    """После обновления (в прошлый раз запускалась 1.0.0-1) — один раз окно
    «Что нового» с изменениями; версия запоминается."""
    last_version = f'{CONF}/last-version.json'
    as_tester(f"mkdir -p {CONF} && echo '{{\"version\":\"1.0.0-1\"}}' > {last_version}")
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()

    whats_new = WhatsNewDialog(drv)
    whats_new.wait_text(rf'updated: 1\.0\.0-1 → {re.escape(INSTALLED)} .*What.s new: 1\.1\.0 ', 30)
    assert json.load(open(last_version))['version'] == INSTALLED, 'установленная версия запомнена'
    whats_new.close()
    launcher.term()

    decided_before = os.stat(last_version).st_mtime_ns
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()
    # решение «показывать или нет» лаунчер принимает после загрузки GUI и
    # переписывает last-version.json — ждём этого, а потом проверяем
    assert wait_for(lambda: os.stat(last_version).st_mtime_ns != decided_before, 15, 0.2), 'лаунчер проверил версию'
    assert not whats_new.is_open(), 'повторный запуск — окна «Что нового» нет'
    launcher.term()
