"""The fake GitHub (fakes/github.py): the launcher release list and their .deb files.

The list is structured like the real one (newest first):
  - a draft and a prerelease — the launcher skips them;
  - LATEST (2.0.0, with a .deb) — the latest, what updates go to;
  - 1.1.1 and 1.1.0 — no .deb, for “What’s new”;
  - 25 releases of 1.0.x without a .deb;
  - ROLLBACK (1.0.0-14, with a .deb) — 31st in the list: visible only if the launcher
    asks for more than 30 per page (with per_page=20 and the default it fell out);
  - 1.0.0-1.
The .deb files (from the current code, with these version numbers) are built by run.sh
and mounted into the container as the DEBS directory.
"""
import hashlib
import json
import os

from .env import GITHUB_API, LATEST, LATEST_DEB, ROLLBACK, T

DEBS = '/opt/github'
RELEASES = '/tmp/github-releases.json'
# dsh releases (deepseek-ai/deepseek-harness): a separate file of the fake GitHub
# (4th argument of fakes/github.py). Created by a scenario that needs the
# dsh descriptions (“What’s new”); without the file, dsh requests go to the main list.
DSH_RELEASES = '/tmp/github-dsh-releases.json'
GITHUB_LOG = '/tmp/github.log'
SERVER = f'{T}/fakes/github.py'
PORT = int(GITHUB_API.rsplit(':', 1)[1])


def sha256(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def _release(tag, deb=None, draft=False, prerelease=False, digest=None):
    assets = []
    if deb:
        assets.append({'name': deb, 'browser_download_url': f'{GITHUB_API}/download/{deb}',
                       'digest': digest or 'sha256:' + sha256(os.path.join(DEBS, deb))})
    return {'tag_name': f'v{tag}', 'name': tag, 'body': f'- Изменения в {tag}.', 'draft': draft,
            'prerelease': prerelease, 'html_url': f'https://github.com/Toximiner/DSH-Launcher/releases/tag/v{tag}',
            'published_at': '2026-01-01T00:00:00Z', 'assets': assets}


def releases(latest_digest=None):
    """The release list; latest_digest — a custom digest for the latest release's .deb."""
    return ([_release('3.0.0', draft=True), _release('2.1.0-rc.1', prerelease=True),
             _release(LATEST, LATEST_DEB, digest=latest_digest),
             _release('1.1.1'), _release('1.1.0')] +
            [_release(f'1.0.{n}') for n in range(25, 0, -1)] +
            [_release(ROLLBACK, f'dsh-launcher_{ROLLBACK}_amd64.deb'), _release('1.0.0-1')])


def write_releases(lst):
    with open(RELEASES, 'w') as f:
        json.dump(lst, f)
    os.chmod(RELEASES, 0o644)


def write_dsh_releases(lst):
    """The dsh release list (deepseek-ai/deepseek-harness) — tags dsh-v<version>."""
    with open(DSH_RELEASES, 'w') as f:
        json.dump(lst, f)
    os.chmod(DSH_RELEASES, 0o644)


def github_log():
    try:
        with open(GITHUB_LOG) as f:
            return f.read()
    except OSError:
        return ''
