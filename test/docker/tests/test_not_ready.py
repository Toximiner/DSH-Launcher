"""«dsh is not ready»: the port is open, but the app does not respond (a long boot) —
the launcher does not leave the grey window, it shows the page with the «Check again» /
«Restart dsh» buttons; after «Check again» — back in the GUI.
"""
import os

from helpers.env import FAKE, NOUPD
from helpers.procs import fakedsh_events, wait_for


def test_dsh_not_ready(start, gui):
    """dsh restarted by itself (as after a plugin update) and «boots» for a long time: the
    port is open, but the HTTP server does not respond. The launcher waits for readiness
    (15 s); not in time — the «dsh is not ready» page; «Check again» (dshlauncher://retry/)
    — the connection, as «dsh was started manually»."""
    hold = '/tmp/hold-http'
    if os.path.exists(hold):
        os.remove(hold)
    launcher = start(FAKE, NOUPD, FAKE_SELF_RESTART_AFTER='3', FAKE_REBORN_HOLD_HTTP=hold)
    gui.wait()
    assert 'FAKE DSH main' in (gui.eval('document.body.innerText') or ''), 'the initial GUI'

    # 1) Self-restart: the child dsh is gone, the new one brought up the same port, but holds HTTP.
    assert launcher.wait_log('порт снова открыт — dsh перезапустился сам', 30), 'switched to the connecting mode'
    assert launcher.wait_log('recover: dsh не готов за 15000 мс', 40), '15 s without a response — "not ready"'
    assert 'http held' in fakedsh_events(), 'the reborn dsh holds the HTTP responses'

    # 2) The «dsh is not ready» page (the container's language — en): the explanation + buttons.
    assert gui.drv.wait_text(r'dsh is not ready.*port 3080 is open.*not responding yet', 10)
    assert gui.drv.wait_text(r'Check again.*Restart dsh', 5), 'the "Check again" and "Restart dsh" buttons'

    # 3) «Check again»: dsh already responds — the window is in the GUI again.
    open(hold, 'w').close()
    assert wait_for(lambda: 'http released' in fakedsh_events(), 10), 'dsh "finished booting"'
    gui.drv.go('dshlauncher://retry/')
    assert launcher.wait_log('retry: dsh запущен: куки валидны — открываю GUI', 30), 'connected via retry'
    assert wait_for(lambda: 'FAKE DSH reborn' in (gui.eval('document.body.innerText') or ''), 20), 'the GUI of the restarted dsh'

    launcher.term()
