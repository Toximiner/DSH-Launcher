'use strict';

/*
 * src/dsh-install.js — the module requires electron, so the pure functions
 * are extracted by markers (test/_extract.js):
 *  - rootInstallMode: when the install of a global npm package goes
 *    through pkexec (a system prefix — polkit asks for the password, as
 *    with a launcher update), and when it stays a manual command (nvm,
 *    ~/.npm-global: root in the home directory would leave files the
 *    user can't update later);
 *  - pkexecNpmArgs: npm is a JS script with `#!/usr/bin/env node`, and
 *    pkexec runs a program with a minimal PATH; the npm directory must be
 *    in PATH, and the arguments — passed via "$@" (no string gluing).
 */

const assert = require('assert');
const path = require('path');
const { test } = require('node:test');
const { extract } = require('./_extract');

const rootInstallMode = extract('rootInstallMode', { path });

test('rootInstallMode: system npm + root prefix + pkexec — install via pkexec', () => {
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/u',
  }), 'pkexec', 'system prefix — polkit asks for the password');
  assert.strictEqual(rootInstallMode({
    npm: '/usr/local/bin/npm', prefix: '/usr/local', hasPkexec: true, home: '/home/u',
  }), 'pkexec', '/usr/local — system too');
});

test('rootInstallMode: npm or prefix in the home directory — manual only', () => {
  // nvm: the user's npm, root would create files in their directory
  assert.strictEqual(rootInstallMode({
    npm: '/home/u/.nvm/versions/node/v22.1.0/bin/npm', prefix: '/home/u/.nvm/versions/node/v22.1.0',
    hasPkexec: true, home: '/home/u',
  }), 'manual', 'npm from nvm');
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/home/u/.npm-global', hasPkexec: true, home: '/home/u',
  }), 'manual', 'prefix in the home directory');
  assert.strictEqual(rootInstallMode({
    npm: '/home/u/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/u',
  }), 'manual', 'npm in the home directory');
});

test('rootInstallMode: no pkexec or unknown paths — manual', () => {
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: false, home: '/home/u',
  }), 'manual', 'pkexec not found');
  assert.strictEqual(rootInstallMode({ npm: null, prefix: '/usr', hasPkexec: true, home: '/home/u' }), 'manual', 'no npm');
  assert.strictEqual(rootInstallMode({ npm: '/usr/bin/npm', prefix: '', hasPkexec: true, home: '/home/u' }), 'manual', 'no prefix');
  assert.strictEqual(rootInstallMode({ npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: true, home: '' }), 'manual', 'no home');
});

test('rootInstallMode: a similar home-directory prefix is not counted as home', () => {
  // /home/user2 is not inside /home/user
  assert.strictEqual(rootInstallMode({
    npm: '/home/user2/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/user',
  }), 'pkexec', 'match only at the path boundary');
});

test('pkexecNpmArgs: npm directory in PATH, arguments via "$@"', () => {
  const pkexecNpmArgs = extract('pkexecNpmArgs', { path });
  const args = pkexecNpmArgs('/usr/bin/npm', ['install', '-g', '@deepseek-ai/dsh@0.2.1-rc.2']);
  assert.deepStrictEqual(args, [
    '/bin/sh', '-c', 'PATH="$1:$PATH"; export PATH; shift; exec "$@"', 'sh',
    '/usr/bin', '/usr/bin/npm', 'install', '-g', '@deepseek-ai/dsh@0.2.1-rc.2',
  ], 'npm directory first in PATH, package as a separate argument');
  assert.strictEqual(args[4], path.dirname('/usr/bin/npm'), 'dirname npm');
  // a version with shell characters doesn't get into the script string
  const evil = pkexecNpmArgs('/opt/npm/bin/npm', ['install', '-g', 'pkg@1.0.0; rm -rf /']);
  assert.strictEqual(evil[2].includes('rm -rf'), false, 'script does not contain the arguments');
  assert.strictEqual(evil[evil.length - 1], 'pkg@1.0.0; rm -rf /', 'argument passed as-is');
});

console.log('OK: dsh-install — rootInstallMode, pkexecNpmArgs');
