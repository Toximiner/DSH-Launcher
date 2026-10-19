'use strict';

/*
 * src/dsh-install.js — модуль требует electron, поэтому чистые функции
 * извлекаются по маркерам (test/_extract.js):
 *  - rootInstallMode: когда установка глобального npm-пакета идёт через
 *    pkexec (системный prefix — пароль спросит polkit, как при обновлении
 *    лаунчера), а когда остаётся ручной командой (nvm, ~/.npm-global:
 *    root в домашнем каталоге оставил бы файлы, которые пользователь
 *    потом не обновит);
 *  - pkexecNpmArgs: npm — JS-скрипт с `#!/usr/bin/env node`, а pkexec
 *    запускает программу с минимальным PATH; каталог npm должен быть в
 *    PATH, а аргументы — передаваться через "$@" (без склейки строк).
 */

const assert = require('assert');
const path = require('path');
const { test } = require('node:test');
const { extract } = require('./_extract');

const rootInstallMode = extract('rootInstallMode', { path });

test('rootInstallMode: системный npm + root-prefix + pkexec — ставим через pkexec', () => {
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/u',
  }), 'pkexec', 'системный prefix — пароль спросит polkit');
  assert.strictEqual(rootInstallMode({
    npm: '/usr/local/bin/npm', prefix: '/usr/local', hasPkexec: true, home: '/home/u',
  }), 'pkexec', '/usr/local — тоже системный');
});

test('rootInstallMode: npm или prefix в домашнем каталоге — только вручную', () => {
  // nvm: npm пользователя, root создал бы файлы в его каталоге
  assert.strictEqual(rootInstallMode({
    npm: '/home/u/.nvm/versions/node/v22.1.0/bin/npm', prefix: '/home/u/.nvm/versions/node/v22.1.0',
    hasPkexec: true, home: '/home/u',
  }), 'manual', 'npm из nvm');
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/home/u/.npm-global', hasPkexec: true, home: '/home/u',
  }), 'manual', 'prefix в домашнем каталоге');
  assert.strictEqual(rootInstallMode({
    npm: '/home/u/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/u',
  }), 'manual', 'npm в домашнем каталоге');
});

test('rootInstallMode: нет pkexec или неизвестны пути — вручную', () => {
  assert.strictEqual(rootInstallMode({
    npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: false, home: '/home/u',
  }), 'manual', 'pkexec не найден');
  assert.strictEqual(rootInstallMode({ npm: null, prefix: '/usr', hasPkexec: true, home: '/home/u' }), 'manual', 'нет npm');
  assert.strictEqual(rootInstallMode({ npm: '/usr/bin/npm', prefix: '', hasPkexec: true, home: '/home/u' }), 'manual', 'нет prefix');
  assert.strictEqual(rootInstallMode({ npm: '/usr/bin/npm', prefix: '/usr', hasPkexec: true, home: '' }), 'manual', 'нет home');
});

test('rootInstallMode: похожий префикс домашнего каталога не считается домашним', () => {
  // /home/user2 не внутри /home/user
  assert.strictEqual(rootInstallMode({
    npm: '/home/user2/bin/npm', prefix: '/usr', hasPkexec: true, home: '/home/user',
  }), 'pkexec', 'совпадение только по границе пути');
});

test('pkexecNpmArgs: каталог npm в PATH, аргументы — через "$@"', () => {
  const pkexecNpmArgs = extract('pkexecNpmArgs', { path });
  const args = pkexecNpmArgs('/usr/bin/npm', ['install', '-g', '@deepseek-ai/dsh@0.2.1-rc.2']);
  assert.deepStrictEqual(args, [
    '/bin/sh', '-c', 'PATH="$1:$PATH"; export PATH; shift; exec "$@"', 'sh',
    '/usr/bin', '/usr/bin/npm', 'install', '-g', '@deepseek-ai/dsh@0.2.1-rc.2',
  ], 'каталог npm первым в PATH, пакет — отдельным аргументом');
  assert.strictEqual(args[4], path.dirname('/usr/bin/npm'), 'dirname npm');
  // версия с символами shell не попадает в строку скрипта
  const evil = pkexecNpmArgs('/opt/npm/bin/npm', ['install', '-g', 'pkg@1.0.0; rm -rf /']);
  assert.strictEqual(evil[2].includes('rm -rf'), false, 'скрипт не содержит аргументов');
  assert.strictEqual(evil[evil.length - 1], 'pkg@1.0.0; rm -rf /', 'аргумент передан как есть');
});

console.log('OK: dsh-install — rootInstallMode, pkexecNpmArgs');
