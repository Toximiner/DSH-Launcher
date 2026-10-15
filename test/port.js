'use strict';
/*
 * test/port.js — проверка portListenerPid (используется при остановке
 * «усыновлённого» dsh после самовозрождения) и listenerAuthUrl (ссылка
 * входа из файла вывода процесса на порту — dsh, перезапущенный маркетом).
 * Функции — из src/system.js. Запуск: node test/port.js
 * (или npm test). Только Linux (/proc).
 */
const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');

const { portListenerPid, listenerAuthUrl } = require('../src/system');

const PORT = 3199;
const PLAIN = `http://127.0.0.1:${PORT}/`;

// Слушатель на PORT; stdout — куда скажут. Готов — когда сообщил по IPC
// (порт уже слушается, строка входа уже напечатана).
async function listener(stdout) {
  const child = spawn(process.execPath, ['-e', `
    require('http').createServer((q, s) => s.end('ok')).listen(${PORT}, '127.0.0.1', () => {
      console.log('starting…');
      console.log('dsh web: ${PLAIN}?token=t1');
      process.send('ready');
    });`], { stdio: ['ignore', stdout, 'ignore', 'ipc'] });
  await new Promise((resolve, reject) => { child.once('message', resolve); child.once('exit', reject); });
  return child;
}

async function stop(child) {
  const exited = new Promise((r) => child.once('exit', r));
  child.kill('SIGKILL');
  await exited;
}

async function main() {
  // 1) слушателя нет → null
  assert.strictEqual(portListenerPid(PORT), null, 'нет слушателя → null');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), null, 'нет слушателя → ссылки нет');

  // 2) слушатель с выводом в файл (как перезапуск кнопкой маркета)
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-port-'));
  const out = fs.openSync(path.join(dir, 'out.log'), 'w');
  let child = await listener(out);
  fs.closeSync(out);
  assert.strictEqual(portListenerPid(PORT), child.pid, 'portListenerPid — pid дочернего');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), `${PLAIN}?token=t1`, 'ссылка входа — из файла вывода');
  assert.strictEqual(listenerAuthUrl(PORT, 'http://127.0.0.1:3080/'), null, 'ссылка на другой порт — не наша');
  await stop(child);
  assert.strictEqual(portListenerPid(PORT), null, 'после остановки слушателя → null');

  // 3) вывод не в файл (/dev/null, терминал) → null
  child = await listener('ignore');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), null, 'stdout не файл → null');
  await stop(child);

  fs.rmSync(dir, { recursive: true, force: true });
  console.log('OK: portListenerPid, listenerAuthUrl — все проверки прошли');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
