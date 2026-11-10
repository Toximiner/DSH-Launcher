"""Installed-package scenarios, with no display server or runtime Internet."""
import os
import json
from pathlib import Path
import re
import signal
import socket
import subprocess

import pytest

from conftest import local_get
from terminal import alive, wait_for


def test_package(lab):
    result = subprocess.run([lab.executable, '--help'], capture_output=True, text=True, check=True, env=lab.env)
    assert 'dsh-launcher-tui [options]' in result.stdout
    assert Path(lab.executable).resolve().name == 'dsh-launcher-tui.js'
    major = int(subprocess.check_output(['node', '--version'], text=True).strip().lstrip('v').split('.')[0])
    assert major >= 22
    if os.environ.get('TUI_TEST_CONTAINER') == '1':
        assert os.geteuid() != 0, 'Scenarios must run unprivileged'
        state = subprocess.check_output(['dpkg-query', '-W', '-f=${Status}', 'dsh-launcher-tui'], text=True)
        assert state == 'install ok installed'
        dependencies = subprocess.check_output(['dpkg-query', '-W', '-f=${Depends}', 'dsh-launcher-tui'], text=True)
        assert 'nodejs (>= 22)' in dependencies
        assert not Path('/usr/lib/dsh-launcher-tui/node_modules/electron').exists()
    assert 'DISPLAY' not in lab.env and 'WAYLAND_DISPLAY' not in lab.env


@pytest.mark.parametrize('exit_mode', ['command', 'ctrl_d', 'SIGTERM', 'SIGHUP', 'SIGINT'])
def test_owned_shutdown(lab, exit_mode):
    app = lab.app()
    app.wait('Ready')
    backend = lab.ready()
    assert alive(backend['pid']) and alive(backend['worker'])
    assert os.getpgid(backend['worker']) == backend['pid']
    if exit_mode == 'command':
        app.command('/exit')
    elif exit_mode == 'ctrl_d':
        app.send('\x04')
    else:
        app.proc.send_signal(getattr(signal, exit_mode))
    app.exited()
    wait_for(lambda: not alive(backend['pid']) and not alive(backend['worker']))


def test_stubborn_process_group(lab):
    app = lab.app(env={'TUI_FAKE_IGNORE_TERM': '1'})
    app.wait('Ready')
    backend = lab.ready()
    app.command('/exit')
    app.exited(timeout=12)
    wait_for(lambda: not alive(backend['pid']) and not alive(backend['worker']))


def test_attachment_survives(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    backend = lab.ready()
    app.command('/exit')
    app.exited()
    assert alive(backend['pid']) and alive(backend['worker'])
    assert local_get(f'http://127.0.0.1:{lab.port}/health') == 'alive'


def test_stream_and_sessions(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.command('stream')
    app.wait('Assistant: FIRST_FRAGMENT')
    assert 'SECOND_FRAGMENT' not in app.text, 'Partial text was delayed until settlement'
    local_get(f'http://127.0.0.1:{lab.port}/release')
    app.wait('FIRST_FRAGMENT SECOND_FRAGMENT')
    assert app.text.count('Assistant: FIRST_FRAGMENT') == 1, 'Committed answer duplicated its live output'
    app.command('/new')
    app.wait('Session session-2')
    app.command('/sessions')
    lab.request('session/list')
    app.wait(f'session-1 {lab.root}')
    app.command('/use session-1')
    wait_for(lambda: len(lab.requests('session/create')) == 2)
    app.wait('DeepSeek Harness — session-1')
    app.wait('FIRST_FRAGMENT SECOND_FRAGMENT')


@pytest.mark.parametrize('decision, expected', [('/approve', 'allowed-once'), ('/reject', 'rejected')])
def test_explicit_approval(lab, decision, expected):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.command('approve')
    app.wait('Approval: bash')
    assert not lab.requests('$events/result'), 'Operation was approved without user input'
    app.command(decision)
    answer = lab.request('$events/result')[-1]['args']['outcome']
    assert answer == {'kind': 'result', 'value': expected}
    app.command('/approve')
    app.wait('No pending decision')
    assert len(lab.requests('$events/result')) == 1


def test_question_and_cancel(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.command('question')
    app.wait('Choose a test option')
    app.command('/answer 2')
    outcome = lab.request('$events/result')[-1]['args']['outcome']
    assert outcome == {'kind': 'result', 'value': {'answers': [{'id': 'choice', 'selected': ['Second']}]}}
    app.command('question')
    app.wait('Waiting for your decision')
    app.send('\x03')
    lab.request('session/cancel')
    app.wait('Ready')
    app.command('/answer 1')
    app.wait('No pending decision')
    assert len(lab.requests('$events/result')) == 1


def test_other_session_delegated(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.command('foreign')
    assert lab.request('$events/result')[-1]['args']['outcome'] == {'kind': 'next'}


def test_disconnect(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.command('disconnect')
    app.wait('Disconnected — exit and reconnect')
    app.command('must not be sent')
    app.wait('Connection is closed')
    assert len(lab.requests('session/prompt')) == 1
    app.command('/exit')
    app.exited()


def test_paste_and_resize(lab):
    app = lab.app(attach=True)
    app.wait('Ready')
    app.send('\x1b[200~/exit\r\x1b[201~')
    app.wait('> /exit')
    assert app.proc.poll() is None, 'Pasted return executed a command'
    app.send('\x15')
    # Confirm ordinary input still works after leaving bracketed paste.
    app.command('/sessions')
    lab.request('session/list')
    app.wait(f'session-1 {lab.root}')
    app.resize(80, 24)
    app.command('/help')
    app.wait('Ctrl+D exit')
    assert app.proc.poll() is None


def test_invalid_login_restores_terminal(lab):
    app = lab.app(attach=True, env={'DSH_TUI_URL': f'http://127.0.0.1:{lab.port}/?token=invalid'})
    app.exited(code=1)
    assert b'login failed' in app.raw
    assert alive(lab.ready()['pid'])


def test_missing_dsh_restores_terminal(lab):
    app = lab.app(args=['--dsh', '/nonexistent/dsh'])
    app.exited(code=1)
    assert b'Cannot start dsh' in app.raw


def test_decline_installation(lab):
    app = lab.app(missing=True, env=lab.install_environment())
    app.wait('dsh was not found')
    assert not lab.npm_calls(), 'npm started before confirmation'
    app.command('yes')
    app.wait('Use /install')
    assert not lab.npm_calls(), 'Ordinary text confirmed installation'
    app.command('/cancel')
    app.exited()
    assert not lab.npm_calls()
    assert lab.ready() is None


def test_install_and_reuse(lab):
    env = lab.install_environment()
    app = lab.app(missing=True, env=env)
    app.wait('dsh was not found')
    app.command('/install')
    app.wait('Ready')
    calls = lab.npm_calls()
    prefix = lab.root / 'data/dsh-launcher-tui/backend'
    assert calls == [['install', '--prefix', str(prefix), '--no-audit', '--no-fund', '--ignore-scripts', '--loglevel=http', '@deepseek-ai/dsh@0.2.2-rc.1']]
    assert 'Test npm installation progress' in app.text
    backend = lab.ready()
    app.command('/exit')
    app.exited()
    wait_for(lambda: not alive(backend['pid']))
    second = lab.app(missing=True, env=env)
    second.wait('Ready')
    assert len(lab.npm_calls()) == 1, 'Completed installation was not reused'
    assert 'dsh was not found' not in second.text


def test_failed_installation_can_retry(lab):
    app = lab.app(missing=True, env=lab.install_environment('fail'))
    app.wait('dsh was not found')
    app.command('/install')
    app.wait('Installation failed: npm exited (1)')
    assert lab.ready() is None
    assert not (lab.root / 'data/dsh-launcher-tui/backend/.installed.json').exists()
    (lab.state / 'npm-mode').write_text('ok')
    app.command('/install')
    app.wait('Ready')
    assert len(lab.npm_calls()) == 2


@pytest.mark.parametrize('cancel', ['command', 'signal'])
def test_cancel_installation(lab, cancel):
    app = lab.app(missing=True, env=lab.install_environment('hang'))
    app.wait('dsh was not found')
    app.command('/install')
    app.wait('Test npm is waiting')
    app.wait('1 packages fetched')
    initial_status = next(row for row in app.screen.display if 'packages fetched' in row)
    def activity_changed():
        app.pump()
        return any('packages fetched' in row and row != initial_status for row in app.screen.display)
    wait_for(activity_changed)
    child = json.loads((lab.state / 'npm-ready.json').read_text())
    app.command('/install')
    app.wait('Installation is running')
    assert len(lab.npm_calls()) == 1
    if cancel == 'command':
        app.command('/cancel')
    else:
        app.proc.send_signal(signal.SIGTERM)
    app.exited(timeout=12)
    wait_for(lambda: not alive(child['pid']) and not alive(child['worker']))
    assert not (lab.root / 'data/dsh-launcher-tui/backend/.installed.json').exists()
    assert lab.ready() is None


def test_real_dsh(lab):
    executable = os.environ.get('TUI_TEST_REAL_DSH')
    if not executable:
        pytest.skip('Set TUI_TEST_REAL_DSH to enable the real backend smoke scenario')
    assert Path(executable).is_file()
    app = lab.app(args=['--dsh', executable])
    app.wait('Ready', timeout=60)
    original = re.search(r'DeepSeek Harness — ([\w-]+)', app.text).group(1)
    children = [int(pid) for pid in subprocess.check_output(['ps', '-o', 'pid=', '--ppid', str(app.proc.pid)], text=True).split()]
    assert children, 'TUI did not start a backend child'
    app.command('/sessions')
    def listed():
        app.pump()
        return any(row.startswith(original + ' ') for row in app.screen.display)
    wait_for(listed)
    app.command('/exit')
    app.exited(timeout=12)
    wait_for(lambda: all(not alive(pid) for pid in children))
    with socket.socket() as sock:
        assert sock.connect_ex(('127.0.0.1', lab.port)) != 0, 'Owned real dsh still listens after exit'
