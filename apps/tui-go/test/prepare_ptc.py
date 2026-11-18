"""Prepare an official source worker missing from pinned dsh npm packaging."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import tarfile
import urllib.request

COMMIT = 'fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159'
SHA256 = '44926eb97c791d1b7a640a0146aa1c522aae7c21a8e238576820d01445cff976'
URL = f'https://codeload.github.com/deepseek-ai/deepseek-harness/tar.gz/{COMMIT}'
PREFIX = f'deepseek-harness-{COMMIT}/packages/ptc-runtime/ptc-runtime-node/'
parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--archive', type=Path, help='Optional previously downloaded official archive')
args = parser.parse_args()
data = args.archive.read_bytes() if args.archive else urllib.request.urlopen(URL, timeout=60).read(64 << 20)
assert hashlib.sha256(data).hexdigest() == SHA256, 'Official PTC source archive checksum mismatch'
args.directory.mkdir(parents=True, exist_ok=True)
with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
    package = json.load(archive.extractfile(PREFIX + 'package.json'))
    assert package['version'] == '0.2.2-rc.1', 'Pinned PTC source version changed'
    for entry in archive:
        if not entry.isfile() or not entry.name.startswith(PREFIX + 'src/'):
            continue
        relative = Path(entry.name.removeprefix(PREFIX + 'src/'))
        assert not relative.is_absolute() and '..' not in relative.parts, 'Unsafe PTC source path'
        target = args.directory / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(archive.extractfile(entry).read())
    license_data = archive.extractfile(f'deepseek-harness-{COMMIT}/LICENSE').read()
    (args.directory / 'UPSTREAM-LICENSE').write_bytes(license_data)
(args.directory / 'package.json').write_text('{"type":"module"}\n')
(args.directory / 'provenance.json').write_text(json.dumps({'repository': 'deepseek-ai/deepseek-harness',
    'commit': COMMIT, 'archive_sha256': SHA256, 'version': package['version'],
    'purpose': 'Official source bootstrap for isolated PTC integration checks only'}, indent=2) + '\n')
assert (args.directory / 'process-entry.ts').is_file(), 'Official PTC source bootstrap is missing'
print('Prepared official PTC source bootstrap:', args.directory / 'process-entry.ts')
