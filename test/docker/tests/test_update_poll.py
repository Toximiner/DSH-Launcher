"""Update checks while the launcher is running (every 12 hours; in the test
— every 2 s): a new version → a notification and the «Help → Update … to X…»
menu item; its window — what's new and «Update now» (the launcher restarts,
the update proceeds without a prompt) / «Later»."""
import json
import os
import re

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_PKG, LATEST_RE, NO_PROMPTS, STATE
from helpers.launcher import launcher_pids
from helpers.procs import dpkg_version, fakedsh_starts, wait_exit, wait_for, wait_port
from helpers.windows import UpdateToast

# the «latest» dsh version on npm — from a file: the test «releases» a new one on the fly
DSH_LATEST = '/tmp/dsh-latest'
# the «latest» launcher version on GitHub — also from a file (the notes and
# the .deb come from the latest release on the fake GitHub)
LAUNCHER_LATEST = '/tmp/launcher-latest'


def test_update_poll(start, gui, drv):
    with open(DSH_LATEST, 'w') as f:
        f.write('0.0.1')          # no new version at startup — no prompt
    os.chmod(DSH_LATEST, 0o644)
    launcher = start(FAKE, DSH_LAUNCHER_FAKE_LAUNCHER_LATEST='1.0.0', DSH_LAUNCHER_FAKE_DSH_LATEST=DSH_LATEST,
                     DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert launcher.wait_log('периодическая проверка обновлений: раз в 2 с', 5), launcher.log()[-800:]
    assert 'доступно обновление' not in launcher.log(), 'no updates at startup'

    with open(DSH_LATEST, 'w') as f:
        f.write('99.0.0')         # a new dsh version has been released

    assert launcher.wait_log('уведомление: доступна dsh 99.0.0', 30), 'the notification'
    assert launcher.wait_log('доступно обновление dsh 99.0.0 — пункт в меню «Справка»', 5), 'the menu item'
    gui.mark()

    # banner in the window: «Details…» — the update window, the banner closes
    toast = UpdateToast(drv).wait_shown()
    assert 'dsh 99.0.0 is available' in toast.text() and 'installed: 0.2.0-rc.2' in toast.text(), toast.text()
    upd = toast.details('dsh')
    assert wait_for(lambda: not toast.is_shown(), 10), 'the toast closed'

    # «Later» — nothing happens: dsh keeps running, the GUI was not reloaded
    upd.wait_text(r'Version 99\.0\.0 is available \(installed: 0\.2\.0-rc\.2\)', 5)
    upd.wait_text('The launcher will restart and dsh will be stopped', 5)
    upd.later()
    assert launcher.alive and gui.not_reloaded(), '«Later» — the launcher and the dsh GUI are unchanged'

    # «Update now» — a restart; the new instance goes straight to installing dsh
    gui.open_update_now('dsh').update_now()
    assert launcher.wait_log('перезапуск лаунчера, чтобы обновить dsh до 99.0.0', 10)
    assert launcher.wait_exit(20) is not None, 'the previous instance exited'
    new = launcher.successor()
    assert new, 'the launcher relaunched'
    # the new instance's output — in the same log
    assert launcher.wait_log('обновление dsh — по просьбе пользователя, без вопроса', 30), launcher.log()[-1500:]
    assert launcher.wait_log('новый @deepseek-ai/dsh: 99.0.0', 30), launcher.log()[-1500:]
    # npm is not in PATH — the install stopped at a hint; the new instance
    # did not start dsh before the update
    assert fakedsh_starts() == 1, 'dsh was started by the previous instance only'
    os.kill(new, 15)
    assert wait_exit(new, 15) is not None, 'the new instance exited normally'


def test_update_poll_setting(start, gui):
    """The «Check for updates every 12 hours» checkbox takes effect
    immediately, without a restart; the check at startup — a separate
    checkbox."""
    launcher = start(FAKE, NO_PROMPTS, DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert launcher.wait_log('периодическая проверка обновлений: раз в 2 с', 5)
    settings = gui.open_settings()

    settings.set('pollUpdates', 'false')
    settings.wait_note('Saved')
    assert launcher.wait_log('периодическая проверка обновлений выключена', 5)
    assert json.load(open(f'{CONF}/settings.json')) == {'pollUpdates': False}
    assert not settings.restart_banner(), 'without a restart'

    settings.set('pollUpdates', 'true')
    settings.wait_note('Saved')
    assert launcher.wait_log_count('периодическая проверка обновлений: раз в 2 с', 2, 5), 'enabled again'
    assert json.load(open(f'{CONF}/settings.json')) == {}, 'by default — no key'
    settings.close()
    launcher.term()


def test_update_now_launcher(start, gui, drv):
    """«Update now» for the launcher itself: the timer check found a new
    version → banner → the «What's new» window → «Update now» — a restart;
    the new instance, without a prompt, downloads the latest (fake) release,
    verifies sha256, installs it via pkexec and restarts into the new
    version; it starts dsh."""
    with open(LAUNCHER_LATEST, 'w') as f:
        f.write('1.0.0')          # no new version at startup — no prompt
    os.chmod(LAUNCHER_LATEST, 0o644)
    launcher = start(FAKE, DSH_LAUNCHER_FAKE_LAUNCHER_LATEST=LAUNCHER_LATEST, DSH_LAUNCHER_FAKE_DSH_LATEST='0.0.1',
                     DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()
    assert 'доступно обновление' not in launcher.log(), 'no updates at startup'

    with open(LAUNCHER_LATEST, 'w') as f:
        f.write(LATEST)           # the latest release has «come out» (fake GitHub)

    toast = UpdateToast(drv).wait_shown()
    assert f'DeepSeek Harness Launcher {LATEST} is available' in toast.text(), toast.text()
    upd = toast.details('launcher')
    upd.wait_text(rf'Version {LATEST_RE} is available \(installed: {re.escape(INSTALLED)}\).*What.s new: {LATEST_RE} ', 10)

    upd.update_now()
    assert launcher.wait_log(f'перезапуск лаунчера, чтобы обновить DeepSeek Harness Launcher до {LATEST}', 10)
    assert launcher.wait_exit(20) is not None, 'the previous instance exited'
    # the new instance's output — in the same log: straight to the install, without a prompt
    assert launcher.wait_log('обновление launcher — по просьбе пользователя, без вопроса', 30), launcher.log()[-1500:]
    assert launcher.wait_log(f'dsh-launcher {LATEST} установлен', 400), launcher.log()[-1500:]
    assert 'совпадает с релизом' in launcher.log(), 'the sha256 matched the release digest'
    assert dpkg_version() == LATEST_PKG, f'dpkg: {LATEST_PKG} is installed'

    # then — a restart into the new version (its output — in launcher.log), it starts dsh
    assert wait_for(lambda: f'DeepSeek Harness Launcher {LATEST},' in _launcher_log() or
                    f'DSH Launcher {LATEST},' in _launcher_log(), 60), 'the new version is running'
    assert wait_port(60), 'the new version brought up dsh'
    assert fakedsh_starts() == 2, 'dsh was started by the first instance and the new version; the intermediate one — no'
    for pid in launcher_pids():
        os.kill(pid, 15)
        wait_exit(pid, 15)


def _launcher_log():
    try:
        with open(f'{STATE}/launcher.log', encoding='utf-8', errors='replace') as f:
            return f.read()
    except OSError:
        return ''
