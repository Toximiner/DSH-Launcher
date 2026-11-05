"""The «File → Settings…» window: fields are saved to settings.json and take
effect after the launcher restarts; environment variables take precedence
(the field is inactive); invalid values are not saved."""
import json
import os

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import fakedsh_events, wait_exit, wait_for, wait_port

SETTINGS = f'{CONF}/settings.json'


def settings_file():
    try:
        return json.load(open(SETTINGS))
    except (OSError, ValueError):
        return {}


def test_settings_apply(start, gui):
    """Profile and working folder → settings.json and the «restart needed»
    banner; «Restart the launcher» — the new instance starts dsh with them."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    settings = gui.open_settings()
    assert not settings.restart_banner(), 'initially — no restart banner'

    settings.set('profile', 'second')
    settings.wait_note('Saved')
    settings.set('cwd', '/tmp')
    settings.wait_note('Saved')

    assert settings_file() == {'profile': 'second', 'cwd': '/tmp'}, settings_file()
    assert wait_for(settings.restart_banner, 5, 0.2), 'the «changes — after a restart» banner is shown'

    settings.relaunch()

    assert launcher.wait_exit(20) is not None, 'the previous instance exited'
    new = launcher.successor()
    assert new, 'the launcher relaunched'
    # the fake dsh log: «start pgid=… cwd=<folder> args=<arguments>»
    assert wait_for(lambda: 'cwd=/tmp args=--profile second --no-open' in fakedsh_events(), 60), \
        f'the new dsh — in folder /tmp and with profile second:\n{fakedsh_events()}'
    assert wait_port(30), 'the new instance brought up dsh'
    # the new instance starts after the old one exits — its window is reachable via CDP
    gui.wait()
    settings = gui.open_settings()
    assert settings.value('profile') == 'second', settings.value('profile')
    assert not settings.restart_banner(), 'settings applied — no «restart needed» banner'
    settings.close()
    os.kill(new, 15)
    assert wait_exit(new, 15) is not None, 'the new instance exited normally'


def test_settings_env_locked(start, gui):
    """The working folder is set by the DSH_CWD variable — it is shown in the
    window but cannot be changed (caption «set by DSH_CWD»)."""
    launcher = start(FAKE, NOUPD, DSH_CWD='/opt')
    gui.wait()
    settings = gui.open_settings()
    settings.section('dsh')

    settings.wait_text(r'Working folder /opt set by DSH_CWD', 5)
    assert 'Choose' not in settings.text().split('Working folder', 1)[1].split('Port', 1)[0], 'there is no «Choose…» button'
    assert settings.disabled('checkUpdates'), 'update check is set by DSH_LAUNCHER_NO_UPDATE_CHECK — the field is inactive'
    assert settings.disabled('pollUpdates'), 'and the check every 12 hours — the same'
    settings.close()
    launcher.term()


def test_settings_validation(start, gui):
    """An invalid port or profile is not saved — an error note is shown."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    settings = gui.open_settings()
    assert settings.active_section() == 'general', 'the window opens on the «General» section'

    settings.set('port', '80')
    settings.wait_note('The port must be a number from 1024 to 65535')
    assert settings.note_inside_card(), 'the note is in the corner of the card, not the whole window'
    settings.set('profile', 'a%20b')
    settings.wait_note('The profile may contain Latin letters')

    assert settings_file() == {}, f'nothing was saved: {settings_file()}'
    assert not settings.restart_banner(), 'no restart banner'
    settings.close()
    launcher.term()
