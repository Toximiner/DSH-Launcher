"""The «Help» menu windows: «About», «Check for updates», «dsh log».

The windows are modal on top of the main one; the dsh GUI under them is
dimmed, but not reloaded. The update check at start is enabled, the «latest»
versions are substituted with old ones — no update prompts.
"""
import json
import os

import pytest

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_RE, NO_PROMPTS, NOUPD, STATE
from helpers.procs import as_tester, wait_for, wait_port


@pytest.fixture
def launched(start, gui):
    """The launcher with the dsh GUI after the update check at start."""
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()
    assert launcher.wait_log('последний @deepseek-ai/dsh', 30), 'the update check at start passed (npm)'
    gui.mark()
    yield launcher
    launcher.term()


def test_menu_about(launched, gui):
    """«About» — versions, GPU, logs, «Copy»; three openings in a row
    (1.3.0 crashed on «About» after the update check)."""
    for attempt in (1, 2, 3):
        about = gui.open_about()
        assert about.value('DeepSeek Harness Launcher') == INSTALLED, f'#{attempt}: the launcher version'
        assert about.value('dsh backend') == '0.2.0-rc.2', f'#{attempt}: the dsh version'
        assert gui.not_reloaded(), f'#{attempt}: the dsh GUI under the dialog was not reloaded'
        assert about.is_overlay(), f'#{attempt}: the dialog is a dimmed layer inside the main window'
        if attempt == 1:
            assert about.value('GPU acceleration').startswith('off (no /dev/dri/renderD'), 'GPU (off in the container)'
            assert about.value('logs') == '/home/tester/.local/state/dsh-launcher', 'the path to the logs'
            about.copy_for_report()

        about.close()

        assert not about.is_open(), f'#{attempt}: the dialog layer was removed'
        assert launched.alive, f'#{attempt}: the launcher is alive'


def test_update_check_dialog(launched, gui):
    """«Check for updates» — both components are up to date."""
    check = gui.open_update_check()

    assert 'up to date' in check.status('DeepSeek Harness Launcher'), check.rows()
    assert 'up to date' in check.status('dsh backend'), check.rows()
    assert gui.not_reloaded(), 'the dsh GUI under the dialog was not reloaded'
    check.close()
    assert launched.alive, 'the launcher is alive'


def test_dsh_log(start, gui):
    """«dsh log» — the tail of dsh.log; the login token is masked."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    log = gui.open_dsh_log()

    text = log.log_text()
    assert 'dsh web: http://127.0.0.1:3080/?token=***' in text, text[-500:]
    assert 'token=abc' not in text, 'the real token is not in the window'
    log.close()
    assert launcher.alive, 'the launcher is alive'
    launcher.term()


def test_update_available(start, gui, prompt):
    """«Check for updates» when a new version exists: the «available» status
    and «What's new» in the window. (The prompt at start — «Not now».)"""
    launcher = start(FAKE)
    prompt.wait('Update DeepSeek Harness Launcher')
    prompt.later()
    gui.wait()

    check = gui.open_update_check()

    assert f'version {LATEST} is available' in check.status('DeepSeek Harness Launcher'), check.rows()
    check.wait_text(rf'What.s new: {LATEST_RE} ', 5)
    check.close()
    launcher.term()


def test_dialog_keys(start, gui):
    """Esc closes a dialog window; Ctrl+= / Ctrl+0 — zoom (persisted)."""
    zoom = f'{CONF}/zoom-level.json'
    launcher = start(FAKE, NOUPD)
    gui.wait()

    about = gui.open_about()
    about.press('Escape')
    assert wait_for(lambda: not about.is_open(), 5), 'Esc closed "About"'

    gui.press('Ctrl+Equal')
    assert wait_for(lambda: os.path.exists(zoom) and json.load(open(zoom))['zoomLevel'] == 0.5, 5), 'Ctrl+= — the zoom +0.5 was saved'
    gui.press('Ctrl+Digit0')
    assert wait_for(lambda: json.load(open(zoom))['zoomLevel'] == 0, 5), 'Ctrl+0 — the zoom was reset'
    assert launcher.alive, 'the launcher is alive'
    launcher.term()


def test_gpu_reenable(start, gui):
    """GPU is disabled after crashes (marker) — in «About» the «Re-enable
    GPU» button removes the marker; GPU — from the next start."""
    marker = f'{STATE}/gpu-disabled.marker'
    as_tester(f'mkdir -p {STATE} && echo "GPU process crashed x2 (test)" > {marker}')
    launcher = start(FAKE, NOUPD)
    gui.wait()

    about = gui.open_about()
    assert about.value('GPU acceleration').startswith('off after GPU process crashes'), about.rows()
    about.reenable_gpu()

    assert wait_for(lambda: not os.path.exists(marker), 5), 'the crash marker was removed'
    about.wait_text('off until the launcher restarts', 5)
    about.close()
    launcher.term()
