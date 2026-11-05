"""Paths, environment variables, and versions shared by all scenarios."""
import os
import re
import secrets

T = '/opt/t'                                   # the tests directory in the container
HOME = '/home/tester'
CONF = f'{HOME}/.config/dsh-launcher'          # the launcher's userData
UPD = f'{CONF}/updates'                        # downloaded update .deb files
LAUNCHER_OUT = '/tmp/launcher.out'             # the launcher's stdout/stderr
STATE = f'{HOME}/.local/state/dsh-launcher'    # launcher and dsh logs (DSH_LOG_DIR from the package's run.sh)
FAKEDSH = f'{T}/fakes/fakedsh.py'
FAKEDSH_LOG = '/tmp/fakedsh.log'
PROXY = f'{T}/fakes/proxy.py'
PROXY_LOG = '/tmp/proxy.log'
GUI_URL = r'^http://127\.0\.0\.1:3080/'        # the dsh GUI address (a regular expression)

# The “latest” dsh version the launcher sees (DSH_LAUNCHER_FAKE_DSH_LATEST).
# Pinned: the real npm registry lives its own life — when 0.2.1-rc.2
# appeared there, the scenarios waiting for 0.2.0-rc.2 failed without a
# single code change. 0.2.0-rc.2 matches the fake dsh's FAKE_DSH_VERSION
# (which is also the “installed” one).
DSH_LATEST = '0.2.0-rc.2'

# Environment variable sets for launching the launcher
FAKE = {'DSH_BIN': FAKEDSH}                                         # fake dsh
NOUPD = {'DSH_LAUNCHER_NO_UPDATE_CHECK': '1'}                       # no update check
NO_PROMPTS = {'DSH_LAUNCHER_FAKE_LAUNCHER_LATEST': '1.0.0',         # the check runs, but the “latest”
              'DSH_LAUNCHER_FAKE_DSH_LATEST': '0.0.1'}              # versions are old — no prompts

# The CI token for the launcher's GitHub API requests: we pop it from the
# pytest environment at import time — the test's child processes do not
# inherit it; the launcher receives it explicitly.
GITHUB_TOKEN = os.environ.pop('DSH_LAUNCHER_GITHUB_TOKEN', '')

# The key for the dshlauncher://… routes (without it the launcher rejects commands):
# in tests we set it ourselves (DSH_LAUNCHER_ROUTE_KEY); the driver substitutes it into the address.
ROUTE_KEY = secrets.token_hex(16)

# Launcher code coverage from the scenarios: the Electron main process writes it here
# (NODE_V8_COVERAGE, on a clean exit); after the scenario conftest keeps
# only the launcher's files and puts them in /results; run.sh (c8) builds the report.
V8_COVERAGE = '/tmp/v8-coverage'

# The fake GitHub (fakes/github.py, helpers/github.py): the launcher takes the
# release list and .deb files from it (except the proxy scenarios — those go to the real one).
GITHUB_API = 'http://127.0.0.1:8765'

# The launcher package in the image is built at version 1.1.0-1 — “older” than the latest release.
INSTALLED = '1.1.0-1'
# The latest release in the fake GitHub: tag v2.0.0 → package 2.0.0-1 (from the current
# code, built by run.sh). ROLLBACK — an older release with a .deb, used for rollbacks.
LATEST = '2.0.0'
LATEST_PKG = f'{LATEST}-1'
ROLLBACK = '1.0.0-14'
LATEST_RE = re.escape(LATEST)
LATEST_DEB = f'dsh-launcher_{LATEST_PKG}_amd64.deb'
