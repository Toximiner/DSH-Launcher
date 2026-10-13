"""Подставной GitHub (fakes/github.py): список релизов лаунчера и .deb к ним.

Список устроен как настоящий (новые первыми):
  - черновик и prerelease — лаунчер их пропускает;
  - LATEST (2.0.0, с .deb) — последний, на него обновляются;
  - 1.1.1 и 1.1.0 — без .deb, для «Что нового»;
  - 25 релизов 1.0.x без .deb;
  - ROLLBACK (1.0.0-14, с .deb) — на 31-м месте: виден, только если лаунчер
    просит больше 30 на страницу (с per_page=20 и по умолчанию он выпадал);
  - 1.0.0-1.
.deb (из текущего кода, с этими номерами версий) собирает run.sh и
подключает в контейнер каталогом DEBS.
"""
import hashlib
import json
import os

from .env import GITHUB_API, LATEST, LATEST_DEB, ROLLBACK, T

DEBS = '/opt/github'
RELEASES = '/tmp/github-releases.json'
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
    """Список релизов; latest_digest — свой digest у .deb последнего релиза."""
    return ([_release('3.0.0', draft=True), _release('2.1.0-rc.1', prerelease=True),
             _release(LATEST, LATEST_DEB, digest=latest_digest),
             _release('1.1.1'), _release('1.1.0')] +
            [_release(f'1.0.{n}') for n in range(25, 0, -1)] +
            [_release(ROLLBACK, f'dsh-launcher_{ROLLBACK}_amd64.deb'), _release('1.0.0-1')])


def write_releases(lst):
    with open(RELEASES, 'w') as f:
        json.dump(lst, f)
    os.chmod(RELEASES, 0o644)


def github_log():
    try:
        with open(GITHUB_LOG) as f:
            return f.read()
    except OSError:
        return ''
