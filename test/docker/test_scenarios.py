"""Сценарии установленного лаунчера в Docker (Ubuntu 24.04 / 26.04, Debian 13).

Каждый тест — отдельный сценарий в свежем контейнере; run.sh запускает
`pytest test_scenarios.py::test_<имя>`. Окном управляем через CDP (cdp.py):
по нативному меню кликнуть нельзя, поэтому окна меню открываются по
маршрутам dshlauncher://… (ответ 204 — страница остаётся на месте).
"""
import json
import os
import re
import shutil
import subprocess
import time

from lib import (CONF, DEB_RE, FAKE, FAKEDSH_LOG, GUI, HOME, LATEST, LATEST_PKG, LATEST_RE, NOUPD, T, UPD,
                 alive, as_tester, fakedsh_starts, launcher_pids, log, port_up, real_dsh_gone, sh, tree_alive,
                 tree_gone, wait_exit, wait_for, wait_log, wait_port)

NO_PROMPTS = {'DSH_LAUNCHER_FAKE_LAUNCHER_LATEST': '1.0.0', 'DSH_LAUNCHER_FAKE_DSH_LATEST': '0.0.1'}


def gui(drv, timeout=60, page=None):
    drv.wait_url(GUI, timeout, page=page)


def dpkg_version():
    return sh("dpkg-query -Wf '${Version}' dsh-launcher")[1]


# ---------------- закрытие окна и сигналы ----------------

def test_close(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    assert tree_alive(), 'дерево fakedsh живо (dsh + sleep)'
    drv.eval('window.close()')
    assert wait_exit(l.pid, 15) is not None, 'лаунчер завершился после закрытия окна'
    assert tree_gone(), 'дерево dsh остановлено'
    assert 'SIGTERM' in open(FAKEDSH_LOG).read(), 'dsh получил SIGTERM (штатно)'


def test_sigterm(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    assert l.term(15) is not None, 'лаунчер вышел по SIGTERM за 15 с'
    assert tree_gone(), 'дерево dsh остановлено'
    assert 'SIGTERM' in open(FAKEDSH_LOG).read(), 'dsh получил SIGTERM (штатно, а не сразу SIGKILL)'
    assert 'SIGTERM — ' in log(), 'в логе лаунчера — обработчик сигнала'


def test_sigterm_ignored(start, drv):
    l = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui(drv)
    t = l.term(15)
    assert t is not None, 'лаунчер вышел за 15 с (ожидаем ~4 с: SIGTERM → SIGKILL)'
    assert tree_gone(), 'dsh, игнорирующий SIGTERM, добит SIGKILL'


def test_sigterm_double(start, drv):
    l = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui(drv)
    sh(f'kill -TERM {l.pid}')
    time.sleep(0.5)
    sh(f'kill -TERM {l.pid}')
    assert wait_exit(l.pid, 5) is not None, 'повторный SIGTERM — выход сразу'
    time.sleep(0.5)
    assert tree_gone(), 'дерево dsh остановлено'


# ---------------- «усыновлённый» dsh после самовозрождения ----------------

def _adopt(start, drv, detached):
    extra = {'FAKE_REBORN_DETACHED': '1'} if detached else {}
    l = start(FAKE, NOUPD, FAKE_SELF_RESTART_AFTER='5', **extra)
    gui(drv)
    assert wait_log('перезапустился сам', 30), 'лаунчер усыновил перезапущенный dsh'
    assert wait_port(10), 'перерождённый dsh слушает порт'
    assert sh("pgrep -f '^sleep 7777'")[0] == 0, 'в старой группе остался sleep 7777'
    drv.eval('window.close()')
    assert wait_exit(l.pid, 15) is not None, 'лаунчер завершился после закрытия окна'
    assert tree_gone(), 'усыновлённый dsh и потомки старой группы остановлены'


def test_adopt_samegroup(start, drv):
    _adopt(start, drv, detached=False)


def test_adopt_detached(start, drv):
    _adopt(start, drv, detached=True)


# ---------------- обновление dsh (npm) ----------------

NVM_BIN = next(iter(sorted(
    f'{HOME}/.nvm/versions/node/{v}/bin' for v in (os.listdir(f'{HOME}/.nvm/versions/node')
                                                    if os.path.isdir(f'{HOME}/.nvm/versions/node') else []))), '')


def test_dsh_update_auto(start, drv):
    l = start(DSH_LAUNCHER_FAKE_INSTALLED='9.9.9-1')
    drv.wait_text(r'Update DeepSeek Harness.*0\.2\.0-rc\.2.*installed: 0\.1\.7-rc\.2', 60)
    drv.go('dshlauncher://update/install/')
    assert wait_log('обновлён → 0.2.0-rc.2', 300), 'npm install -g прошёл, dsh обновлён'
    code, out = sh(f'runuser -u tester -- env PATH={NVM_BIN}:/usr/bin:/bin {NVM_BIN}/dsh --version')
    assert out.splitlines()[-1:] == ['0.2.0-rc.2'], f'в nvm теперь dsh 0.2.0-rc.2: {out}'
    assert wait_port(120), 'лаунчер перешёл к запуску dsh (порт поднят)'
    assert l.term(15) is not None, 'лаунчер вышел по SIGTERM'
    time.sleep(0.5)
    assert real_dsh_gone(), 'настоящий dsh остановлен'


def test_dsh_update_other_prefix(start, drv):
    l = start(DSH_LAUNCHER_FAKE_INSTALLED='9.9.9-1', DSH_BIN=f'{HOME}/.local/bin/dsh',
              PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    drv.wait_text('Update DeepSeek Harness', 60)
    drv.go('dshlauncher://update/install/')
    drv.wait_text(r'npm installed the package into /home/tester/\.nvm.*runs /home/tester/\.local/bin/dsh \(version 0\.1\.7-rc\.2\)', 300)
    drv.wait_text(r'Run in a terminal: npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2', 5)  # без sudo
    drv.go('dshlauncher://update/later/')
    assert wait_port(120), '«Не сейчас» — dsh запускается'
    l.term()


def test_dsh_update_root_prefix(start, drv):
    l = start(DSH_LAUNCHER_FAKE_INSTALLED='9.9.9-1', DSH_BIN='/opt/noderoot/bin/dsh')
    drv.wait_text('Update DeepSeek Harness', 60)
    drv.go('dshlauncher://update/install/')
    drv.wait_text(r'needs sudo.*sudo npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2', 60)
    drv.go('dshlauncher://update/never/')
    assert wait_port(120), '«Не спрашивать больше» — dsh запускается'
    prefs = json.load(open(f'{CONF}/update-prefs.json'))
    assert prefs.get('dsh') == '0.2.0-rc.2', f'отказ запомнен на версию: {prefs}'
    l.term()


# ---------------- обновление самого лаунчера (настоящий последний релиз) ----------------

def _new_instance(old_pid):
    time.sleep(3)
    pids = [p for p in launcher_pids() if p != old_pid]
    return pids[0] if pids else None


def test_launcher_update(start, drv):
    l = start(FAKE)
    drv.wait_text(rf'Update DSH Launcher.*Version {LATEST_RE} is available \(installed: 1\.1\.0-1\s*\)', 60)
    # «Что нового»: изменения от последней версии вниз до 1.1.1 (первая после 1.1.0)
    drv.wait_text(rf'What.s new: {LATEST_RE} .*1\.1\.1 ', 5)
    drv.eval("document.querySelector('a[target=_blank]').click()")
    time.sleep(1.5)
    drv.wait_text('Update DSH Launcher', 3)  # клик по ссылке не уводит окно со страницы вопроса
    drv.go('dshlauncher://update/install/')
    assert wait_log(f'dsh-launcher {LATEST} установлен', 400), 'скачано и установлено через pkexec'
    assert 'совпадает с релизом' in log(), 'sha256 совпал с digest релиза'
    assert dpkg_version() == LATEST_PKG, f'dpkg: установлен {LATEST_PKG}'
    assert wait_exit(l.pid, 20) is not None, 'старый экземпляр завершился'
    new = _new_instance(l.pid)
    assert new, 'новый экземпляр запущен'
    assert wait_port(60), 'новый экземпляр поднял dsh'
    assert fakedsh_starts() == 1, 'старый экземпляр после обновления dsh не запускал (один старт)'
    assert not [f for f in os.listdir(UPD) if f.endswith('.deb')], 'скачанный .deb удалён'
    sh(f'kill -TERM {new}')
    wait_exit(new, 15)


def test_launcher_update_denied(start, drv):
    l = start(FAKE)
    drv.wait_text('Update DSH Launcher', 60)
    drv.go('dshlauncher://update/install/')
    drv.wait_text(rf'Update manually.*Installation failed \(code 127\).*sudo apt install -y {re.escape(UPD)}/{DEB_RE}', 400)
    assert dpkg_version() == '1.1.0-1', 'dpkg: всё ещё 1.1.0-1'
    sh(f'apt-get install -y {UPD}/dsh-launcher_{LATEST_PKG}_amd64.deb', check=True)  # пользователь ставит сам
    drv.go('dshlauncher://update/recheck/')
    assert wait_log(f'обновлён до {LATEST_PKG} (вручную)', 30), '«Проверить ещё раз» — «Update installed»'
    assert wait_exit(l.pid, 20) is not None, 'старый экземпляр завершился'
    new = _new_instance(l.pid)
    assert new, 'новый экземпляр запущен'
    assert wait_port(60), 'новый экземпляр поднял dsh'
    assert fakedsh_starts() == 1, 'старый экземпляр после обновления dsh не запускал (один старт)'
    sh(f'kill -TERM {new}')
    wait_exit(new, 15)


def test_launcher_update_nopkexec(start, drv):
    shutil.move('/usr/bin/pkexec', '/usr/bin/pkexec.off')
    l = start(FAKE)
    drv.wait_text('Update DSH Launcher', 60)
    drv.go('dshlauncher://update/install/')
    drv.wait_text(rf'pkexec was not found.*sudo apt install -y {re.escape(UPD)}/{DEB_RE}', 400)
    drv.go('dshlauncher://update/later/')
    assert wait_port(60), '«Не сейчас» — dsh запускается'
    l.term()


# ---------------- окна меню, контекстное меню, поиск ----------------

def _dialog_gone(drv):
    return wait_for(lambda: not any(re.match(r'^(About|Update check)$', p.get('title', '')) for p in drv.pages()), 10)


def test_menu_about(start, drv):
    # Модальное окно поверх главного, после проверки обновлений при старте.
    l = start(FAKE, NO_PROMPTS)
    gui(drv)
    assert wait_log('последний @deepseek-ai/dsh', 30), 'проверка обновлений при старте прошла (npm)'
    drv.eval('window.__mark = 42; 1', page=GUI)
    gui_kept = lambda: drv.eval('window.__mark', page=GUI) == 42
    dimmed = lambda: '0.55' in (drv.eval("getComputedStyle(document.documentElement, '::after').backgroundColor", page=GUI) or '')
    for i in (1, 2, 3):
        drv.go('dshlauncher://menu/about/', page=GUI)
        drv.wait_text(r'DSH Launcher 1\.1\.0-1.*dsh backend 0\.2\.0-rc\.2.*Copy for report', 30, page='^About$')
        if i == 1:
            drv.wait_text(r'GPU acceleration off \(no /dev/dri/renderD.*logs /home/tester/\.local/state/dsh-launcher.*Logs folder', 5, page='^About$')
        assert gui_kept(), f'GUI dsh под окном не перезагружался (#{i})'
        assert dimmed(), f'GUI dsh под окном затемнён (#{i})'
        if i == 1:
            drv.go('dshlauncher://dialog/copy/', page='^About$')
            drv.wait_text('Copied', 10, page='^About$')
        drv.go('dshlauncher://dialog/close/', page='^About$')
        assert _dialog_gone(drv), f'«Close» #{i} — окно закрылось'
        assert wait_for(lambda: not dimmed(), 5), f'затемнение снято (#{i})'
        assert alive(l.pid), f'лаунчер жив после «О программе» #{i}'
    drv.go('dshlauncher://menu/check/', page=GUI)
    drv.wait_text(r'DSH Launcher.*up to date.*dsh backend.*up to date', 60, page='^Update check$')
    assert gui_kept(), 'GUI dsh под окном проверки не перезагружался'
    drv.go('dshlauncher://dialog/close/', page='^Update check$')
    assert _dialog_gone(drv), 'окно проверки закрылось'
    assert alive(l.pid), 'лаунчер жив после проверки обновлений'
    l.term()


def _ctx(drv, x, y, pattern):
    """Правый клик → последняя строка «контекстное меню: …» в логе лаунчера."""
    n = log().count('контекстное меню:')
    drv.right_click(x, y, page=GUI)
    wait_for(lambda: log().count('контекстное меню:') > n, 5, 0.25)
    lines = [s for s in log().splitlines() if 'контекстное меню:' in s]
    last = lines[-1] if lines else 'меню не показано'
    assert re.search(pattern, last), last


def test_context_menu(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    drv.eval("""document.body.innerHTML =
      '<p id=t style="position:absolute;left:20px;top:20px;margin:0;font-size:20px">Hello context menu</p>' +
      '<input id=i value="some input text" style="position:absolute;left:20px;top:80px;width:300px;height:30px">' +
      '<a id=l href="https://example.com/" style="position:absolute;left:20px;top:140px;font-size:20px">a link</a>'; 1""", page=GUI)
    drv.eval('getSelection().removeAllRanges(); 1', page=GUI)
    _ctx(drv, 600, 400, r'меню: Select all$')                                   # пустое место
    drv.eval("const r = document.createRange(); r.selectNodeContents(document.getElementById('t')); "
             "getSelection().removeAllRanges(); getSelection().addRange(r); 1", page=GUI)
    _ctx(drv, 60, 30, r'меню: Copy, \|, Select all$')                           # выделенный текст
    drv.eval("const i = document.getElementById('i'); i.focus(); i.select(); 1", page=GUI)
    _ctx(drv, 100, 95, r'меню: Cut, Copy, Paste( \(off\))?, \|, Select all$')   # поле ввода
    drv.eval('getSelection().removeAllRanges(); document.activeElement.blur(); 1', page=GUI)
    _ctx(drv, 40, 150, r'меню: Copy link address, \|, Select all$')             # ссылка
    assert alive(l.pid), 'лаунчер жив после контекстных меню'
    assert l.term() is not None, 'лаунчер штатно завершился'
    assert tree_gone(), 'дерево dsh остановлено'


FIND = '%3Cinput%20id%3D%22q%22'  # панель поиска — отдельная страница CDP


def test_find(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    drv.eval("document.body.innerHTML = '<p>needle one</p><p>needle two</p><p>needle three</p>'; 1", page=GUI)
    drv.key('Ctrl+KeyF', page=GUI)
    assert wait_for(lambda: drv.page(FIND) is not None, 10), 'Ctrl+F — панель поиска открылась'
    count = lambda: drv.eval("document.getElementById('n').textContent", page=FIND) or ''
    typed = lambda text: drv.eval("(() => { const q = document.getElementById('q'); q.value = %s; "
                                  "q.dispatchEvent(new Event('input')); return 1; })()" % json.dumps(text), page=FIND)
    typed('needle')
    assert wait_for(lambda: count().endswith('/ 3'), 5, 0.25), f'поиск «needle» — 3 совпадения ({count()})'
    before = count()
    drv.go('dshlauncher://find/next/', page=FIND)
    assert wait_for(lambda: count() != before, 5, 0.25), f'«следующее» сменило совпадение ({before})'
    typed('nothing-like-this')
    assert wait_for(lambda: count() == 'none', 5, 0.25), f'нет совпадений — «none» ({count()})'
    assert alive(l.pid), 'лаунчер жив'
    l.term()


# ---------------- перезапуск dsh, прокси, размер окна ----------------

def test_restart_dsh(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    assert tree_alive(), 'дерево fakedsh живо'
    drv.go('dshlauncher://menu/restart-dsh/')  # по меню — с подтверждением
    assert wait_log('перезапуск dsh по запросу', 15), 'перезапуск по запросу'
    assert wait_for(lambda: fakedsh_starts() >= 2, 30), 'dsh запущен заново (второй старт)'
    assert 'SIGTERM' in open(FAKEDSH_LOG).read(), 'прежний dsh получил SIGTERM'
    gui(drv)
    assert not re.search(r'dsh has stopped|dsh остановился', log()), 'без страницы «dsh остановился»'
    assert tree_alive(), 'новое дерево fakedsh живо'
    assert l.term() is not None, 'лаунчер штатно завершился'
    assert tree_gone(), 'дерево dsh остановлено'


def test_proxy(start, drv):
    # Системный прокси (session.resolveProxy → агент Node): в контейнере —
    # переменными окружения.
    if os.path.exists('/tmp/proxy.log'):
        os.remove('/tmp/proxy.log')
    subprocess.Popen(f'runuser -u tester -- /opt/node22/bin/node {T}/proxy.js 3129 >/dev/null 2>&1', shell=True)
    assert wait_for(lambda: os.path.exists('/tmp/proxy.log') and 'listening' in open('/tmp/proxy.log').read(), 5, 0.25)
    p = 'http://127.0.0.1:3129'
    l = start(FAKE, NO_PROMPTS, HTTPS_PROXY=p, https_proxy=p, HTTP_PROXY=p, http_proxy=p)
    gui(drv)  # localhost — мимо прокси
    assert wait_log('последний @deepseek-ai/dsh', 30), 'проверка обновлений npm прошла'
    plog = open('/tmp/proxy.log').read()
    assert 'CONNECT registry.npmjs.org:443' in plog, 'запрос к npm шёл через прокси'
    assert 'CONNECT api.github.com:443' in plog, 'запрос к GitHub шёл через прокси'
    l.term()


def test_window_state(start, drv):
    ws = f'{CONF}/window-state.json'
    as_tester(f"mkdir -p ~/.config/dsh-launcher && echo '{{\"width\":1000,\"height\":640}}' > {ws}")
    l = start(FAKE, NOUPD)
    gui(drv)
    assert drv.eval('window.outerWidth') == 1000, 'сохранённая ширина 1000 применена'
    drv.eval('window.close()')
    assert wait_exit(l.pid, 15) is not None, 'лаунчер завершился'
    assert '"width":1000' in open(ws).read(), 'при закрытии размер записан (1000)'
    os.remove(ws)
    l = start(FAKE, NOUPD)
    gui(drv)
    w = drv.eval('window.outerWidth') or 0
    assert 760 <= w <= 1280, f'по умолчанию, но не шире экрана (Xvfb 1280): {w}'
    drv.eval('window.close()')
    assert wait_exit(l.pid, 15) is not None, 'лаунчер завершился'
    assert '"width":' in open(ws).read(), 'размер записан при закрытии'


# ---------------- «Что нового» после обновления, журнал dsh ----------------

def test_whats_new(start, drv):
    lv = f'{CONF}/last-version.json'
    as_tester(f"mkdir -p ~/.config/dsh-launcher && echo '{{\"version\":\"1.0.0-1\"}}' > {lv}")
    l = start(FAKE, NO_PROMPTS)
    gui(drv, page=GUI)  # окно «What’s new» может быть первым в списке CDP
    drv.wait_text(r'updated: 1\.0\.0-1 → 1\.1\.0-1 .*What.s new: 1\.1\.0 ', 30, page='^What.s new$')
    assert '"1.1.0-1"' in open(lv).read(), 'версия 1.1.0-1 запомнена'
    drv.go('dshlauncher://dialog/close/', page='^What.s new$')
    l.term()
    l = start(FAKE, NO_PROMPTS)
    gui(drv, page=GUI)
    time.sleep(5)
    assert not drv.page('^What.s new$'), 'повторный запуск — окна «What’s new» нет'
    l.term()


def test_dsh_log(start, drv):
    l = start(FAKE, NOUPD)
    gui(drv)
    drv.go('dshlauncher://menu/log/')
    drv.wait_text(r'dsh web: http://127\.0\.0\.1:3080/\?token=\*\*\*', 15, page='^dsh log$')
    assert 'token=abc' not in drv.text(page='^dsh log$'), 'настоящего токена в окне нет'
    drv.go('dshlauncher://dialog/close/', page='^dsh log$')
    assert alive(l.pid), 'лаунчер жив'
    l.term()
