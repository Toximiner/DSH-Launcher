"""«What's new» when GitHub did not respond at startup: the release list was
not fetched — the window does not appear and the version is not remembered;
as soon as the periodic check gets the release list — the missed changes are
shown right away (not accumulated until the next update)."""
import json
import re
import subprocess
import time

from helpers.env import CONF, FAKE, INSTALLED, NO_PROMPTS
from helpers.github import DEBS, PORT, RELEASES, SERVER, github_log
from helpers.procs import as_tester, pgrep, sh, wait_for
from helpers.windows import WhatsNewDialog


def test_whats_new_retry(start, gui, drv):
    # The fake GitHub is stopped: at startup the launcher will not get the
    # release list (connection refused). Without a shell: the command line
    # of `sh -c "pkill -f …"` contains the pattern too — pkill would kill
    # the shell itself.
    subprocess.run(['pkill', '-f', 'fakes/github.py'], capture_output=True)
    assert wait_for(lambda: not pgrep('github.py'), 5, 0.1), 'fake GitHub did not stop'
    as_tester(f"mkdir -p {CONF} && echo '{{\"version\":\"1.0.0-1\"}}' > {CONF}/last-version.json")

    launcher = start(FAKE, NO_PROMPTS, DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()

    # The check cycle without GitHub: no window, the version not remembered
    # (we will show it at the next successful check).
    whats_new = WhatsNewDialog(drv)
    time.sleep(4)
    assert not whats_new.is_open(), "«What's new» window without a release list"
    assert 'обновлён' not in launcher.log(), 'version remembered without a release list'
    assert json.load(open(f'{CONF}/last-version.json'))['version'] == '1.0.0-1'

    # GitHub is available again — the next successful check (2 s) shows the
    # missed changes and remembers the version.
    sh('rm -f /tmp/github.log')
    subprocess.Popen(f'runuser -u tester -- python3 {SERVER} {PORT} {RELEASES} {DEBS} >/tmp/github.out 2>&1',
                     shell=True)
    assert wait_for(lambda: 'listening' in github_log(), 5, 0.1), 'fake GitHub did not start'

    whats_new.wait_text(rf'updated: 1\.0\.0-1 → {re.escape(INSTALLED)} .*What.s new: 1\.1\.0 ', 30)
    assert json.load(open(f'{CONF}/last-version.json'))['version'] == INSTALLED, 'version remembered'
    whats_new.close()
    launcher.term()
