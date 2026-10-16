'use strict';
// Остановка dsh: дерево процессов, «усыновлённый» dsh после самовозрождения.
const state = require('./state');
const { groupAlive, portListenerPid, pgidOf, pickKillTargets } = require('./system');
const { PORT } = require('./env');
const { sleep } = require('./port'); // чистый модуль — прямой require, без реестра

/* ---------------- остановка дерева процессов ---------------- */

// groupOnly: главный процесс уже завершился — его pid мог занять чужой
// процесс, поэтому без запасного варианта «просто pid», только группа (-pid).
function killGroup(pid, signal, groupOnly = false) {
  try { process.kill(-pid, signal); } catch {
    if (groupOnly) return;
    try { process.kill(pid, signal); } catch { /* noop */ }
  }
}



// Главный процесс dsh завершился (по коду или по сигналу).
function dshLeaderExited() {
  return state.dshProc.exitCode !== null || state.dshProc.signalCode !== null;
}

async function stopDsh() {
  if (!state.weStartedDsh || !state.dshProc || state.dshProc.pid === undefined) return;
  const pid = state.dshProc.pid;
  if (state.dshFatal) return; // spawn не удался — процесса не было
  const groupOnly = dshLeaderExited();
  if (!groupAlive(pid, groupOnly)) return; // группа уже мёртва — нечего убивать
  killGroup(pid, 'SIGTERM', groupOnly);
  for (let i = 0; i < 40 && groupAlive(pid, groupOnly); i++) await sleep(100); // до 4 с
  if (groupAlive(pid, groupOnly)) {
    killGroup(pid, 'SIGKILL');
    for (let i = 0; i < 20 && groupAlive(pid); i++) await sleep(100);
  }
  console.log('[launcher] dsh stopped by launcher');
}

// pid процесса, слушающего порт (Linux, чистый fs — без fuser/ss):
// inode сокетов LISTEN берём из /proc/net/tcp{,6}, затем ищем fd
// socket:[inode] среди /proc/[pid]/fd/*.


// pgid процесса: поле 5 из /proc/<pid>/stat (comm в скобках может содержать
// пробелы — считаем от последней «)»).


// Что убивать у «усыновлённого» dsh (после самовозрождения). Ручки на новый
// процесс у нас нет, поэтому:
//  - процесс на порту: убиваем его группу по pgid — он может быть не лидером
//    группы, и тогда kill(-pid) не сработал бы, а потомки остались бы жить;
//  - группа исходного dsh: в ней могли остаться его потомки (лидер мёртв —
//    только группа, без запасного варианта по pid).


function adoptedTargets() {
  const pid = portListenerPid(PORT);
  return pickKillTargets(pid, pid ? pgidOf(pid) : null, pgidOf(process.pid), state.adoptedOldPgid);
}

async function stopAdoptedDsh() {
  const targets = adoptedTargets().filter((t) => groupAlive(t.id, t.groupOnly));
  if (targets.length === 0) {
    console.log('[launcher] усыновлённый dsh: живых процессов не найдено — не убиваем (порт уже закрыт?)');
    return;
  }
  console.log(`[launcher] останавливаю усыновлённый dsh (${targets.map((t) => (t.groupOnly ? 'pgid=' : 'pid=') + t.id).join(', ')})`);
  const alive = () => targets.filter((t) => groupAlive(t.id, t.groupOnly));
  for (const t of targets) killGroup(t.id, 'SIGTERM', t.groupOnly);
  for (let i = 0; i < 20 && alive().length; i++) await sleep(100); // до 2 с
  for (const t of alive()) killGroup(t.id, 'SIGKILL', t.groupOnly);
}

Object.assign(module.exports, { killGroup, dshLeaderExited, stopDsh, adoptedTargets, stopAdoptedDsh }); // дополняем, а не заменяем: модули подключают друг друга по кругу
