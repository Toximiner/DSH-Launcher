"""Smoke scenarios under Wayland (headless weston; Ubuntu 26.04 only).
All other scenarios run under X11 (Xvfb). The container has no GPU — same
as a VM."""
import os
from pathlib import Path

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import as_tester, wait_exit, wait_for


def matching_backends(mode):
    """Check actual process arguments, not just the launcher's configuration log."""
    roles = set()
    for proc in Path('/proc').iterdir():
        if not proc.name.isdigit():
            continue
        try:
            # Electron can rewrite its process title into one argv slot. The
            # installed path and backend switches contain no whitespace.
            args = (proc / 'cmdline').read_bytes().replace(b'\0', b' ').split()
        except OSError:
            continue
        if not args or args[0] != b'/opt/dsh-launcher/electron/electron':
            continue
        role = next((a for a in args if a.startswith(b'--type=')), b'main')
        if role not in (b'main', b'--type=renderer', b'--type=gpu-process'):
            continue
        if f'--ozone-platform={mode}'.encode() not in args:
            return False
        roles.add(role)
    return b'main' in roles and b'--type=renderer' in roles


def test_wayland_settings_restart(start, gui, wayland):
    """Settings are resolved before native initialization, including on restart."""
    as_tester(f'mkdir -p {CONF} && printf \'%s\' \'{{"ozone":"x11"}}\' > {CONF}/settings.json')
    launcher = start(FAKE, NOUPD, wayland)
    gui.wait()
    assert wait_for(lambda: matching_backends('x11'), 10), 'main and renderer start on X11'

    for mode in ('wayland', 'x11', 'auto'):
        settings = gui.open_settings()
        settings.set('ozone', mode)
        settings.wait_note('Saved')
        settings.relaunch()
        previous = launcher.pid
        assert wait_exit(previous, 20) is not None, 'previous instance exited'
        new = launcher.successor()
        assert new and new != previous, 'launcher restarted'
        launcher.pid = new
        gui.wait()
        # The headless container has no render node, so Auto selects X11.
        expected = 'x11' if mode == 'auto' else mode
        assert wait_for(lambda: matching_backends(expected), 10), f'all processes use {expected}'
        about = gui.open_about()
        assert about.is_overlay(), 'the new window remains interactive'
        about.close()
    launcher.term()


def test_wayland_cli_override(start, gui, wayland):
    """A desktop shortcut's X11 argument wins over a conflicting preference."""
    as_tester(f'mkdir -p {CONF} && printf \'%s\' \'{{"ozone":"wayland"}}\' > {CONF}/settings.json')
    launcher = start(FAKE, NOUPD, wayland, launcher_args=['--ozone-platform=x11'])
    gui.wait()
    assert wait_for(lambda: matching_backends('x11'), 10), 'explicit X11 wins for every process'
    settings = gui.open_settings()
    settings.set('ozone', 'auto')
    settings.wait_note('Saved')
    settings.relaunch()
    assert launcher.wait_exit(20) is not None, 'previous instance exited'
    new = launcher.successor()
    assert new, 'launcher restarted'
    gui.wait()
    assert wait_for(lambda: matching_backends('x11'), 10), 'CLI override survives settings restart'
    launcher.pid = new
    launcher.term()


def test_wayland_auto(start, gui, wayland):
    """A Wayland session without a GPU: the launcher itself picks X11 via
    XWayland (on a VM a native Wayland window without a GPU is often
    invisible) — the window is shown, the GUI is loaded."""
    launcher = start(FAKE, NOUPD, wayland)  # DISPLAY stays — this is «XWayland»
    gui.wait()
    assert 'Wayland-сессия — ozone-platform=x11 (render-узлов нет' in launcher.log(), launcher.log()[:800]
    assert launcher.wait_log('окно показано', 10)
    launcher.term()


def test_wayland_native(start, gui, wayland):
    """Native Wayland (DSH_LAUNCHER_OZONE=wayland), no X display at all: the
    window is shown, the GUI is loaded, the dialog — a layer inside the
    window — opens and closes, «Restart the launcher» — the new instance is
    on Wayland again and responds."""
    launcher = start(FAKE, NOUPD, wayland, DSH_LAUNCHER_OZONE='wayland', DISPLAY=None)
    gui.wait()
    assert 'Wayland-сессия — ozone-platform=wayland (принудительно: DSH_LAUNCHER_OZONE)' in launcher.log()
    assert launcher.wait_log('окно показано', 10)

    about = gui.open_about()
    assert about.is_overlay(), 'dialog is a layer inside the window'
    about.close()

    gui.open_settings().relaunch()
    assert launcher.wait_exit(20) is not None, 'previous instance exited'
    new = launcher.successor()
    assert new, 'launcher restarted'
    gui.wait()  # the new instance's window — via CDP
    assert launcher.log().count('ozone-platform=wayland (принудительно') == 2, 'new instance — on Wayland too'
    os.kill(new, 15)
    assert wait_exit(new, 15) is not None, 'new instance exited cleanly'
