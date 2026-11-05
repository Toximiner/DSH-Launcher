"""Launcher self-update: the latest release from the stub GitHub (2.0.0-1,
from the current code) is installed over the 1.1.0-1 package from the image
(via pkexec), rolling back to an older release, and «What's new».
"""
import json
import os
import re
import shutil
import time

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_DEB, LATEST_PKG, LATEST_RE, NO_PROMPTS, NOUPD, ROLLBACK, UPD
from helpers.github import github_log, releases, write_releases
from helpers.procs import as_tester, dpkg_version, fakedsh_starts, sh, wait_exit, wait_for, wait_port
from helpers.windows import WhatsNewDialog


def _check_new_instance(launcher):
    """After the update the old instance exits, the new one brings up dsh;
    the old dsh never started (one fake dsh start — by the new one)."""
    assert launcher.wait_exit(20) is not None, 'the old instance exited'
    new = launcher.successor()
    assert new, 'the new instance started'
    assert wait_port(60), 'the new instance brought up dsh'
    assert fakedsh_starts() == 1, 'the old instance did not start dsh after the update'
    sh(f'kill -TERM {new}')
    wait_exit(new, 15)


def test_launcher_update(start, prompt):
    """The prompt with «What's new» → «Update» → downloaded, sha256 verified,
    pkexec installed the package, the launcher restarted into the new version."""
    launcher = start(FAKE)
    prompt.wait(rf'Update DeepSeek Harness Launcher.*Version {LATEST_RE} is available \(installed: {re.escape(INSTALLED)}\s*\)')
    # «What's new»: from the latest version down to 1.1.1 (the first after 1.1.0)
    prompt.wait(rf'What.s new: {LATEST_RE} .*1\.1\.1 ', 5)
    page_url = prompt.url()
    prompt.mark()
    prompt.click_release_link()
    # the link opens externally (shell.openExternal), the window stays on the prompt
    assert prompt.url() == page_url and prompt.not_reloaded(), 'clicking the release link did not take the window away'

    prompt.install()

    assert launcher.wait_log(f'dsh-launcher {LATEST} установлен', 400), 'downloaded and installed via pkexec'
    assert 'совпадает с релизом' in launcher.log(), 'sha256 matched the release digest'
    assert dpkg_version() == LATEST_PKG, f'dpkg: installed {LATEST_PKG}'
    # the update marker (for «Roll back to the previous version…» if the new version fails)
    upd = json.load(open(f'{CONF}/last-update.json'))
    assert upd['from'] == INSTALLED and upd['to'] == LATEST_PKG and upd.get('at'), upd
    _check_new_instance(launcher)
    assert not [f for f in os.listdir(UPD) if f.endswith('.deb')], 'the downloaded .deb was removed'


def test_launcher_update_bad_sha(start, prompt):
    """The sha256 of the downloaded .deb did not match the release digest — the package is not
    installed and is deleted; the page with a terminal command; «Not now» — dsh starts."""
    write_releases(releases(latest_digest='sha256:' + '0' * 64))
    launcher = start(FAKE)
    prompt.wait('Update DeepSeek Harness Launcher')

    prompt.install()

    prompt.wait(r'Download failed: sha256 не совпадает с релизом \(\w+… ≠ 000000000000…\)', 400)
    assert f'/files/{LATEST_DEB} 200' in github_log(), 'the package was downloaded (via redirect)'
    assert dpkg_version() == INSTALLED, f'dpkg: still {INSTALLED}'
    assert 'шаг обновления' not in launcher.log(), 'installation (pkexec) was not started'
    assert not [f for f in os.listdir(UPD) if f.endswith('.deb')], 'the downloaded .deb was removed'
    prompt.later()
    assert wait_port(60), '"Not now" — dsh starts'
    launcher.term()


def test_launcher_update_denied(start, prompt):
    """polkit denied — a terminal command; the user installs the package
    themselves, «Check again» sees the new version and restarts the launcher."""
    launcher = start(FAKE)
    prompt.wait('Update DeepSeek Harness Launcher')

    prompt.install()

    prompt.wait(rf'Update manually.*Installation failed \(code 127\).*sudo apt install -y {re.escape(UPD)}/{re.escape(LATEST_DEB)}', 400)
    assert dpkg_version() == INSTALLED, f'dpkg: still {INSTALLED}'
    sh(f'apt-get install -y {UPD}/{LATEST_DEB}', check=True)  # the user installs in a terminal
    prompt.recheck()
    assert launcher.wait_log(f'обновлён до {LATEST_PKG} (вручную)', 30), '"Check again" — "Update installed"'
    _check_new_instance(launcher)


def test_launcher_update_nopkexec(start, prompt):
    """No pkexec — straight to a terminal command; «Not now» — dsh starts."""
    shutil.move('/usr/bin/pkexec', '/usr/bin/pkexec.off')
    launcher = start(FAKE)
    prompt.wait('Update DeepSeek Harness Launcher')

    prompt.install()

    prompt.wait(rf'pkexec was not found.*sudo apt install -y {re.escape(UPD)}/{re.escape(LATEST_DEB)}', 400)
    prompt.later()
    assert wait_port(60), '"Not now" — dsh starts'
    launcher.term()


def test_whats_new_stale_cache(start, gui, drv):
    """The release list in the cache (fresh, under an hour) was fetched before
    the installed version came out — «What's new» requests it again and shows
    the changes, not just a link."""
    as_tester(f"mkdir -p {CONF} && echo '{{\"version\":\"1.0.0-1\"}}' > {CONF}/last-version.json")
    old = [{'tag_name': 'v1.0.0-14', 'body': 'old', 'draft': False, 'prerelease': False, 'assets': [], 'html_url': ''}]
    as_tester(f"cat > {CONF}/releases-cache.json <<'EOF'\n" +
              json.dumps({'etag': None, 'list': old, 'fetchedAt': int(time.time() * 1000)}) + "\nEOF")
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()

    whats_new = WhatsNewDialog(drv)
    whats_new.wait_text(rf'updated: 1\.0\.0-1 → {re.escape(INSTALLED)} .*What.s new: 1\.1\.0 ', 30)
    assert f'в кэше релизов нет {INSTALLED} — запрашиваю список заново' in launcher.log()
    whats_new.close()
    launcher.term()


def test_whats_new(start, gui, drv):
    """After an update (last time 1.0.0-1 ran) — the «What's new» window with
    the changes once; the version is remembered."""
    last_version = f'{CONF}/last-version.json'
    as_tester(f"mkdir -p {CONF} && echo '{{\"version\":\"1.0.0-1\"}}' > {last_version}")
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()

    whats_new = WhatsNewDialog(drv)
    whats_new.wait_text(rf'updated: 1\.0\.0-1 → {re.escape(INSTALLED)} .*What.s new: 1\.1\.0 ', 30)
    assert json.load(open(last_version))['version'] == INSTALLED, 'the installed version was remembered'
    whats_new.close()
    launcher.term()

    decided_before = os.stat(last_version).st_mtime_ns
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()
    # the «show it or not» decision is made by the launcher after the GUI load
    # and it rewrites last-version.json — wait for that, then check
    assert wait_for(lambda: os.stat(last_version).st_mtime_ns != decided_before, 15, 0.2), 'the launcher checked the version'
    assert not whats_new.is_open(), "restart — no 'What's new' window"
    launcher.term()


def test_rollback(start, gui):
    """«Help → Roll back to the previous version…»: the list of past releases;
    «Roll back» — a restart, the new instance installs the chosen (older)
    version via pkexec with a downgrade; it does not ask about the very
    latest version again. ROLLBACK in the stub GitHub's list is the 31st:
    visible only if the launcher asks for more than 30 releases at once."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    dlg = gui.open_rollback()
    dlg.wait_text(rf'Installed: {re.escape(INSTALLED)}\. Choose a version: .*{re.escape(ROLLBACK)} ', 60)

    dlg.go(f'dshlauncher://rollback/?v={ROLLBACK}')
    assert launcher.wait_log(f'перезапуск лаунчера, чтобы вернуть версию {ROLLBACK}', 10)
    assert launcher.wait_exit(20) is not None, 'the previous instance exited'
    assert launcher.successor(), 'the launcher restarted'
    assert launcher.wait_log(f'откат: ставлю {ROLLBACK} (установлена {INSTALLED})', 60), launcher.log()[-1500:]
    assert launcher.wait_log(f'dsh-launcher {ROLLBACK} установлен', 400), launcher.log()[-1500:]
    assert 'совпадает с релизом' in launcher.log(), 'sha256 matched the release digest'
    assert dpkg_version() == ROLLBACK, f'dpkg: {dpkg_version()}'
    assert json.load(open(f'{CONF}/update-prefs.json')).get('launcher') == LATEST, 'do not ask about the latest version again'
