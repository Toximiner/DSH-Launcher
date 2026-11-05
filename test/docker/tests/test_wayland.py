"""Smoke scenarios under Wayland (headless weston; Ubuntu 26.04 only).
All other scenarios run under X11 (Xvfb). The container has no GPU — same
as a VM."""
import os

from helpers.env import FAKE, NOUPD
from helpers.procs import wait_exit


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
