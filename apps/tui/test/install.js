'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { EventEmitter } = require('node:events');
const { DshInstaller, DSH_VERSION, DSH_PACKAGE, supportsDsh, findExecutable, installPrefix } = require('../src/install');
const { InstallProgress } = require('../src/install-progress');

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tui-install-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const terminal = new EventEmitter();
  terminal.lines = [];
  terminal.log = text => { terminal.lines.push(text); terminal.emit('output', text); };
  terminal.draw = () => {};
  const script = (name, body) => {
    const file = path.join(dir, name);
    fs.writeFileSync(file, '#!/usr/bin/env node\n' + body, { mode: 0o755 });
    return file;
  };
  return { dir, terminal, script, installer: new DshInstaller(path.join(dir, 'backend')) };
}

test('resolves executables and user-local installation paths', t => {
  const f = fixture(t);
  const bin = f.script('dsh', '');
  assert.equal(findExecutable('dsh', f.dir, { PATH: f.dir }), bin);
  assert.equal(findExecutable('./dsh', f.dir, {}), bin);
  assert.equal(findExecutable('absent', f.dir, { PATH: f.dir }), null);
  assert.equal(findExecutable(f.dir, f.dir), null);
  assert.equal(installPrefix({}, '/home/test'), '/home/test/.local/share/dsh-launcher-tui/backend');
  assert.equal(installPrefix({ XDG_DATA_HOME: '/data' }, '/home/test'), '/data/dsh-launcher-tui/backend');
  assert.equal(installPrefix({ XDG_DATA_HOME: 'relative' }, '/home/test'), '/home/test/.local/share/dsh-launcher-tui/backend');
  assert.equal(supportsDsh('22.23.3'), false);
  assert.equal(supportsDsh('24.8.0'), false);
  assert.equal(supportsDsh('24.9.0'), true);
  assert.equal(supportsDsh('25.0.0'), true);
});

test('requires confirmation and exits a declined offer without starting npm', async t => {
  const f = fixture(t);
  const controller = new AbortController();
  const result = f.installer.ensure({ cwd: f.dir, bin: 'missing-tui-test-dsh' }, f.terminal, controller.signal);
  assert.match(f.terminal.lines.join('\n'), /\/install.*\/cancel/);
  f.terminal.emit('line', 'yes');
  assert.equal(f.installer.child, null);
  controller.abort();
  assert.equal(await result, null);
  assert.equal(f.terminal.listenerCount('line'), 0);
  assert.equal(fs.existsSync(f.installer.prefix), false);
});

test('keeps an invalid explicit path as an error and reuses a completed local install', async t => {
  const f = fixture(t);
  const config = { cwd: f.dir, bin: '/nonexistent/dsh', explicitBin: true };
  await assert.rejects(f.installer.ensure(config, f.terminal, new AbortController().signal), /Cannot start dsh/);
  fs.mkdirSync(path.dirname(f.installer.bin), { recursive: true });
  fs.writeFileSync(f.installer.bin, '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  assert.equal(f.installer.managedBinary(), null, 'partial installation must not be reused');
  fs.writeFileSync(f.installer.marker, JSON.stringify({ version: DSH_VERSION }));
  assert.equal(await f.installer.ensure({ ...config, explicitBin: false }, f.terminal, new AbortController().signal), f.installer.bin);
  assert.equal(f.terminal.lines.length, 0);
});

test('installs the pinned backend locally, streams npm output, and persists success', async t => {
  const f = fixture(t);
  const npm = f.script('npm', `
const fs = require('node:fs'); const path = require('node:path');
const prefix = process.argv[process.argv.indexOf('--prefix') + 1];
fs.writeFileSync(path.join(prefix, 'args.json'), JSON.stringify(process.argv.slice(2)));
const bin = path.join(prefix, 'node_modules/.bin/dsh');
fs.mkdirSync(path.dirname(bin), {recursive:true});
fs.writeFileSync(bin, '#!/bin/sh\\nexit 0\\n', {mode:0o755});
console.log('installation progress');
`);
  await f.installer.install(npm, f.terminal, new AbortController().signal);
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(f.installer.prefix, 'args.json'))),
    ['install', '--prefix', f.installer.prefix, '--no-audit', '--no-fund', '--ignore-scripts', '--loglevel=http', DSH_PACKAGE]);
  assert.ok(f.terminal.lines.some(line => line.includes('installation progress')));
  assert.equal(f.installer.managedBinary(), f.installer.bin);
  assert.equal(f.installer.running, false);
});

test('installation progress reports actual fetches, animates while idle, and never invents a percentage', () => {
  const progress = new InstallProgress(1000);
  const first = progress.text(1000);
  assert.notEqual(first, progress.text(1100), 'silent npm activity must remain visibly animated');
  progress.observe('npm http fetch GET 404 https://registry.npmjs.org/missing.tgz 10ms');
  assert.equal(progress.packages.size, 0, 'failed downloads must not count as fetched packages');
  progress.observe('npm http fetch GET 200 https://registry.npmjs.org/package 10ms');
  assert.match(progress.text(2000), /Resolving packages.*1 requests completed.*1s/);
  progress.observe('npm http fetch GET 200 https://registry.npmjs.org/package/-/package-1.0.0.tgz 30ms');
  progress.observe('npm http cache https://registry.npmjs.org/package/-/package-1.0.0.tgz 1ms');
  assert.match(progress.text(3000), /Fetching packages.*1 packages fetched/);
  assert.equal(progress.packages.size, 1, 'repeated URLs must not inflate progress');
  assert.equal(progress.text(3000).includes('%'), false);
});

test('an npm failure remains incomplete and can be retried', async t => {
  const f = fixture(t);
  const npm = f.script('npm', "console.error('installation error'); process.exitCode=1;");
  await assert.rejects(f.installer.install(npm, f.terminal, new AbortController().signal), /npm exited \(1\)/);
  assert.equal(f.installer.running, false);
  assert.equal(f.installer.managedBinary(), null);
  assert.equal(fs.existsSync(f.installer.marker), false);
  assert.ok(f.terminal.lines.some(line => line.includes('installation error')));
  await assert.rejects(f.installer.install(npm, f.terminal, new AbortController().signal), /npm exited/);
});

test('cancellation stops npm and duplicate install requests cannot start another child', async t => {
  const f = fixture(t);
  const npm = f.script('npm', "console.log('waiting'); setInterval(()=>{},1000);");
  const controller = new AbortController();
  const ready = new Promise(resolve => f.terminal.on('output', line => { if (line.includes('waiting')) resolve(); }));
  const installation = f.installer.install(npm, f.terminal, controller.signal);
  const rejected = assert.rejects(installation, /npm exited|aborted/);
  await ready;
  const pid = f.installer.child.pid;
  await assert.rejects(f.installer.install(npm, f.terminal, controller.signal), /already running/);
  controller.abort();
  await f.installer.stop();
  await rejected;
  assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
  assert.equal(fs.existsSync(f.installer.marker), false);
});
