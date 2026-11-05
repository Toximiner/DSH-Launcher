'use strict';
// Stopping dsh: the process tree, the "adopted" dsh after self-revival.
const state = require('./state');
const { groupAlive, portListenerPid, pgidOf, pickKillTargets } = require('./system');
const { PORT } = require('./env');
const { sleep } = require('./port'); // pure module — direct require, no registry

/* ---------------- stopping the process tree ---------------- */

// groupOnly: the main process has already exited — its pid may have been
// taken by another process, so there is no "just the pid" fallback, only the group (-pid).
function killGroup(pid, signal, groupOnly = false) {
  try { process.kill(-pid, signal); } catch {
    if (groupOnly) return;
    try { process.kill(pid, signal); } catch { /* noop */ }
  }
}



// The main dsh process has exited (by code or by signal).
function dshLeaderExited() {
  return state.dshProc.exitCode !== null || state.dshProc.signalCode !== null;
}

async function stopDsh() {
  if (!state.weStartedDsh || !state.dshProc || state.dshProc.pid === undefined) return;
  const pid = state.dshProc.pid;
  if (state.dshFatal) return; // spawn failed — there was no process
  const groupOnly = dshLeaderExited();
  if (!groupAlive(pid, groupOnly)) return; // the group is already dead — nothing to kill
  killGroup(pid, 'SIGTERM', groupOnly);
  for (let i = 0; i < 40 && groupAlive(pid, groupOnly); i++) await sleep(100); // up to 4 s
  if (groupAlive(pid, groupOnly)) {
    killGroup(pid, 'SIGKILL');
    for (let i = 0; i < 20 && groupAlive(pid); i++) await sleep(100);
  }
  console.log('[launcher] dsh stopped by launcher');
}

// pid of the process listening on the port (Linux, pure fs — no fuser/ss):
// the LISTEN socket inodes are taken from /proc/net/tcp{,6}, then the fd
// socket:[inode] is searched among /proc/[pid]/fd/*.


// pgid of a process: field 5 of /proc/<pid>/stat (comm in parentheses may contain
// spaces — we count from the last ")").


// What to kill for the "adopted" dsh (after self-revival). We have no handles
// on the new process, so:
//  - the process on the port: kill its group by pgid — it may not be the
//    group leader, in which case kill(-pid) would not work and descendants would survive;
//  - the group of the original dsh: its descendants may remain in it (the leader is dead —
//    the group only, no fallback by pid).


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
  for (let i = 0; i < 20 && alive().length; i++) await sleep(100); // up to 2 s
  for (const t of alive()) killGroup(t.id, 'SIGKILL', t.groupOnly);
}

Object.assign(module.exports, { killGroup, dshLeaderExited, stopDsh, adoptedTargets, stopAdoptedDsh }); // we extend, not replace: the modules require each other cyclically
