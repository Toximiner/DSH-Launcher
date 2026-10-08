'use strict';
/*
 * test/port.js — проверка portListenerPid (используется при остановке
 * «усыновлённого» dsh после самовозрождения). Функция извлекается из
 * main.js по маркерам, как в test/version.js. Запуск: node test/port.js
 * (или npm test). Только Linux (/proc).
 */
const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const src = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');
const lines = src.split('\n');
const start = lines.findIndex((l) => l.includes('===== portListenerPid ====='));
const end = lines.findIndex((l) => l.includes('===== end portListenerPid ====='));
assert.ok(start >= 0 && end > start, 'main.js: не найдены маркеры portListenerPid');
const portListenerPid = new Function(
  'fs',
  lines.slice(start, end + 1).join('\n') + '\nreturn portListenerPid;'
)(fs);

const PORT = 3199;

async function main() {
  // 1) слушателя нет → null
  assert.strictEqual(portListenerPid(PORT), null, 'нет слушателя → null');

  // 2) дочерний процесс слушает 127.0.0.1:PORT → возвращается его pid
  const child = spawn(
    process.execPath,
    ['-e', `require('http').createServer((q, s) => s.end('ok')).listen(${PORT}, '127.0.0.1');`],
    { stdio: 'ignore' }
  );
  await new Promise((r) => setTimeout(r, 800));
  const pid = portListenerPid(PORT);
  assert.strictEqual(pid, child.pid, `portListenerPid: ${pid} !== pid дочернего ${child.pid}`);

  // 3) слушатель остановлен → снова null
  child.kill('SIGKILL');
  await new Promise((r) => setTimeout(r, 300));
  assert.strictEqual(portListenerPid(PORT), null, 'после остановки слушателя → null');

  console.log('OK: portListenerPid — все проверки прошли');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
