'use strict';
// updates-github: the «latest» selection from the GitHub release list and the
// fake «latest» stubs. src/updates-github.js is not loaded whole (it requires
// electron): the marker blocks run with the injected fakes; the version
// comparison — the real one from src/versions.
const test = require('node:test');
const assert = require('node:assert');
const { extract } = require('./_extract');
const { compareDebianVer, tagToPackageVersion } = require('../src/versions');

test('updates-github: fakeLatest — the stub «latest» version', (t) => {
  const files = { '/fl/a.txt': '  7.7.7\n' };
  const fsFake = { readFileSync: (p) => { if (!(p in files)) throw new Error('ENOENT: ' + p); return files[p]; } };
  const fakeLatest = extract('fakeLatest', { fs: fsFake });
  const vars = {};
  const setVar = (n, v) => { vars[n] = process.env[n]; if (v === undefined) delete process.env[n]; else process.env[n] = v; };
  t.after(() => { for (const [n, v] of Object.entries(vars)) { if (v === undefined) delete process.env[n]; else process.env[n] = v; } });

  assert.strictEqual(fakeLatest('FL_TEST_UNSET'), null, 'no variable → null');
  setVar('FL_TEST_VALUE', '6.6.6');
  assert.strictEqual(fakeLatest('FL_TEST_VALUE'), '6.6.6', 'the value of the variable → the value');
  setVar('FL_TEST_FILE', '/fl/a.txt');
  assert.strictEqual(fakeLatest('FL_TEST_FILE'), '7.7.7', 'a path → the contents of the file (trimmed)');
  setVar('FL_TEST_MISSING', '/fl/none');
  assert.strictEqual(fakeLatest('FL_TEST_MISSING'), null, 'an unreadable path → null');
  files['/fl/empty.txt'] = '   ';
  setVar('FL_TEST_EMPTY', '/fl/empty.txt');
  assert.strictEqual(fakeLatest('FL_TEST_EMPTY'), null, 'an empty contents → null');
});

test('updates-github: fetchLauncherLatest — the latest published release', async () => {
  const parseRelease = (r) => ({ version: String(r.tag_name).replace(/^v/, ''), htmlUrl: r.html_url, assetUrl: null, assetSha256: null });
  const rel = (tag, extra = {}) => ({ tag_name: tag, html_url: `https://github.com/x/releases/${tag}`, body: `b ${tag}`, ...extra });
  const deps = {
    githubReleases: async () => [
      rel('v1.18.7'),
      rel('v1.18.10'),
      rel('v1.9.0'),
      rel('v1.18.8'),
      rel('v2.0.0', { prerelease: true }),
      rel('v9.9.9', { draft: true }),
      { html_url: 'no-tag' }, // no tag_name — excluded
    ],
    compareDebianVer,
    tagToPackageVersion,
    parseRelease,
  };

  const meta = await extract('fetchLauncherLatest', { ...deps, fakeLatest: () => null })();
  assert.strictEqual(meta.version, '1.18.10', 'the highest among published (draft/prerelease are ignored)');
  assert.strictEqual(meta.releases.length, 4, 'the descriptions of the published ones');
  assert.strictEqual(meta.all.length, 4, '«all» — for the rollback');

  const withFake = extract('fetchLauncherLatest', { ...deps, fakeLatest: () => '9.9.9' });
  assert.strictEqual((await withFake()).version, '9.9.9', 'the fake «latest» overwrites');

  const empty = extract('fetchLauncherLatest', { ...deps, githubReleases: async () => [rel('v1.0.0', { draft: true })], fakeLatest: () => null });
  await assert.rejects(empty(), /нет опубликованных релизов/, 'there are no published ones — an error');

  const notArray = extract('fetchLauncherLatest', { ...deps, githubReleases: async () => ({ error: 'api' }), fakeLatest: () => null });
  await assert.rejects(notArray(), /нет опубликованных релизов/, 'a non-array list — the same error');
});
