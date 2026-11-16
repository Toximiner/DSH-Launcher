"""A GUI window and a terminal lease share one managed dsh process tree."""
import os
import subprocess

import pytest

from helpers.env import FAKE, HOME, NOUPD
from helpers.procs import dsh_tree_alive, dsh_tree_gone, wait_for


@pytest.mark.parametrize('how', [pytest.param('window', id='shared_window'), pytest.param('signal', id='shared_signal'), pytest.param('terminal_first', id='shared_terminal_first')])
def test_shared_backend(start, gui, how):
    launcher = start(FAKE, NOUPD)
    gui.wait()
    script = r"""
const m = require('/opt/dsh-launcher/src/backend-clients');
const client = m.joinBackend(3080);
console.log('joined');
process.stdin.once('data', () => {
  const last = client.release();
  if (last) for (const group of client.targets()) process.kill(-group, 'SIGTERM');
  process.exit(0);
});
"""
    peer = subprocess.Popen(['node', '-e', script], user='tester', group='tester', extra_groups=[],
        env={'HOME': HOME, 'PATH': os.environ['PATH']}, stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    try:
        assert peer.stdout.readline().strip() == 'joined'
        if how == 'terminal_first':
            peer.communicate('close\n', timeout=10)
            assert dsh_tree_alive(), 'Closing terminal participant must preserve the GUI backend'
            gui.close()
            assert launcher.wait_exit(15) is not None
        else:
            if how == 'window':
                gui.close()
                assert launcher.wait_exit(15) is not None
            else:
                assert launcher.term(15) is not None
            assert dsh_tree_alive(), 'Closing GUI owner must preserve the terminal backend'
            peer.communicate('close\n', timeout=10)
        assert wait_for(dsh_tree_gone, 10), 'The last frontend must stop the managed process tree'
    finally:
        if peer.poll() is None:
            peer.kill()
            peer.wait()
