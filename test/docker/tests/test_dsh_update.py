"""Installing and updating dsh via npm at start.

The image ships a real old dsh 0.1.7-rc.2: in the user's nvm (writable
prefix), in ~/.local («another prefix») and in /opt/noderoot (the prefix is
owned by root). «latest» on npm is substituted with 0.2.0-rc.2 (DSH_LATEST in
helpers/env.py): the live registry lives its own life — the released 0.2.1-rc.2
broke these scenarios although the code did not change.
"""
import json
import os
import shutil

from helpers.env import CONF, HOME
from helpers.github import DSH_RELEASES, write_dsh_releases
from helpers.procs import real_dsh_gone, sh, wait_for, wait_port

NVM_NODE = f'{HOME}/.nvm/versions/node'
NVM_BIN = f'{NVM_NODE}/{sorted(os.listdir(NVM_NODE))[0]}/bin' if os.path.isdir(NVM_NODE) else ''
OLD_LAUNCHER = {'DSH_LAUNCHER_FAKE_INSTALLED': '9.9.9-1'}  # the launcher update prompt is not needed


def test_dsh_update_auto(start, prompt):
    """the prefix is writable — the launcher updates dsh on its own and starts the new one."""
    launcher = start(OLD_LAUNCHER)
    prompt.wait(r'Update DeepSeek Harness.*0\.2\.0-rc\.2.*installed: 0\.1\.7-rc\.2')

    prompt.install()

    assert launcher.wait_log('обновлён → 0.2.0-rc.2', 300), 'npm install -g passed, dsh updated'
    _, out = sh(f'runuser -u tester -- env PATH={NVM_BIN}:/usr/bin:/bin {NVM_BIN}/dsh --version')
    assert out.splitlines()[-1:] == ['0.2.0-rc.2'], f'nvm now has dsh 0.2.0-rc.2: {out}'
    assert wait_port(120), 'the launcher moved on to starting dsh (port is up)'
    assert launcher.term() is not None, 'the launcher exited on SIGTERM'
    assert real_dsh_gone(), 'the real dsh is stopped'


def test_dsh_update_other_prefix(start, prompt):
    """npm installs into one prefix, but dsh runs from another — a terminal
    command (without sudo)."""
    launcher = start(OLD_LAUNCHER, DSH_BIN=f'{HOME}/.local/bin/dsh', PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    prompt.wait(r'npm installed the package into /home/tester/\.nvm.*'
                r'runs /home/tester/\.local/bin/dsh \(version 0\.1\.7-rc\.2\)', 300)
    prompt.wait(r'Run in a terminal: npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2', 5)
    prompt.later()
    assert wait_port(120), '"Not now" — dsh starts anyway'
    launcher.term()


def test_dsh_update_root_prefix(start, prompt):
    """the prefix is owned by root, npm is system-wide — the install goes
    through pkexec: polkit asks for the password in a system dialog (in the
    container the rule allows it), dsh is updated and started."""
    launcher = start(OLD_LAUNCHER, DSH_BIN='/opt/noderoot/bin/dsh')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    assert launcher.wait_log('dsh обновлён → 0.2.0-rc.2', 300), launcher.log()[-2000:]
    # runuser resets the environment, and dsh is «#!/usr/bin/env node»: node
    # lives in /opt/noderoot/bin, so PATH is set explicitly (as in dsh_update_auto).
    _, out = sh('runuser -u tester -- env PATH=/opt/noderoot/bin:/usr/bin:/bin /opt/noderoot/bin/dsh --version')
    assert out.splitlines()[-1:] == ['0.2.0-rc.2'], f'/opt/noderoot now has dsh 0.2.0-rc.2: {out}'
    assert wait_port(120), 'the launcher moved on to starting dsh (port is up)'
    assert launcher.term() is not None, 'the launcher exited on SIGTERM'
    assert real_dsh_gone(), 'the real dsh is stopped'


def test_dsh_update_root_prefix_nopkexec(start, prompt):
    """root prefix, but no pkexec — straight to a sudo command; «Don't ask
    again» is remembered per version."""
    shutil.move('/usr/bin/pkexec', '/usr/bin/pkexec.off')
    launcher = start(OLD_LAUNCHER, DSH_BIN='/opt/noderoot/bin/dsh')
    prompt.wait('Update DeepSeek Harness')

    prompt.install()

    prompt.wait(r'needs sudo.*sudo npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2')
    prompt.never()
    assert wait_port(120), '"Do not ask again" — dsh starts anyway'
    prefs = json.load(open(f'{CONF}/update-prefs.json'))
    assert prefs.get('dsh') == '0.2.0-rc.2', f'the refusal is remembered per version: {prefs}'
    launcher.term()


def test_dsh_update_notes(start, prompt):
    """The dsh update prompt shows «What's new»: the dsh release descriptions
    come from GitHub (deepseek-harness), the EN part of the bilingual body."""
    body = ('[中文](#cn) | [English](#en)\n'
            '\n<h3 id="cn-v0.2.0-rc.2">新增功能</h3>\n'
            '\n- 俄语界面测试功能。 @tester\n'
            '\n<h3 id="en-v0.2.0-rc.2">New Features</h3>\n'
            '\n- Russian interface test feature. by @tester\n'
            '- A second EN line for the notes block.\n'
            '\nFull Changelog: [dsh-v0.1.7-rc.2...dsh-v0.2.0-rc.2](https://github.com/deepseek-ai/deepseek-harness/compare/dsh-v0.1.7-rc.2...dsh-v0.2.0-rc.2)')
    write_dsh_releases([
        {'tag_name': 'dsh-v0.2.0-rc.2', 'name': 'v0.2.0-rc.2', 'body': body,
         'draft': False, 'prerelease': True,
         'html_url': 'https://github.com/deepseek-ai/deepseek-harness/releases/tag/dsh-v0.2.0-rc.2',
         'published_at': '2026-11-01T00:00:00Z', 'assets': []},
        {'tag_name': 'dsh-v0.1.7-rc.2', 'name': 'v0.1.7-rc.2', 'body': 'old',
         'draft': False, 'prerelease': True,
         'html_url': 'https://github.com/deepseek-ai/deepseek-harness/releases/tag/dsh-v0.1.7-rc.2',
         'published_at': '2026-10-01T00:00:00Z', 'assets': []},
    ])
    # The real old dsh 0.1.7-rc.2 in nvm — «installed»; latest — 0.2.0-rc.2.
    launcher = start(OLD_LAUNCHER, PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    prompt.wait(r'Update DeepSeek Harness.*0\.2\.0-rc\.2.*installed: 0\.1\.7-rc\.2')
    prompt.wait('What’s new:', 30)
    prompt.wait('Russian interface test feature', 10)
    prompt.wait('A second EN line for the notes block', 10)
    assert '新增功能' not in prompt.text(), 'the CN part is not shown'
    assert 'Full Changelog' not in prompt.text(), 'the "Full Changelog" line is dropped'
    assert os.path.exists(f'{CONF}/dsh-releases-cache.json'), 'the dsh releases cache is written'
    prompt.later()
    assert wait_port(120), '"Not now" — dsh starts anyway'
    launcher.term()


def test_dsh_install(start, prompt):
    """dsh is nowhere — the install page; «Install» puts it in via npm
    (the nvm prefix is writable), the launcher starts the installed dsh."""
    for link in (f'{NVM_BIN}/dsh', f'{HOME}/.local/bin/dsh'):  # keep only npm
        sh(f'rm -f {link}')
    launcher = start(OLD_LAUNCHER, PATH=f'{NVM_BIN}:/usr/local/bin:/usr/bin:/bin')
    prompt.wait(r'could not find dsh.*Install', 60)

    prompt.drv.go('dshlauncher://install/run/')

    assert launcher.wait_log('dsh установлен — продолжаю запуск', 300), 'npm install -g passed'
    assert wait_port(120), 'the installed dsh is running (port is up)'
    assert launcher.term() is not None, 'the launcher exited on SIGTERM'
    assert wait_for(real_dsh_gone, 5), 'the real dsh is stopped'
