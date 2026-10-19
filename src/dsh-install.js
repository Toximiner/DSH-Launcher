'use strict';
// Поиск и проверка программы dsh, npm и Node.js.
const { app } = require('electron');
const { execFile } = require('child_process');
const util = require('util');
const { promisify } = util;
const fs = require('fs');
const os = require('os');
const path = require('path');
const execFileAsync = promisify(execFile);
const state = require('./state');
const { firstErrLine } = require('./util');
const { envWithBinDir, withoutUnitEnv, writableOrCreatable } = require('./system');
const { DSH_BIN_DEFAULT, DSH_BIN_EXPLICIT } = require('./env');

/* ---------------- проверка установки dsh ---------------- */

// Окружение для запуска dsh: каталог бинарника — первым в PATH. dsh — это
// `#!/usr/bin/env node`-скрипт; установленный через nvm, он должен получить
// node из того же каталога, а при запуске с ярлыка nvm в PATH нет.
// Окружение dsh (и npm в его профиле): без переменных службы systemd — см.
// withoutUnitEnv в system.js.
function dshEnv(extra) {
  return withoutUnitEnv(envWithBinDir(state.dshBin, extra));
}



async function probeDsh(bin) {
  if (!bin || !fs.existsSync(bin)) return { ok: false, reason: 'missing' };
  try {
    const { stdout: out } = await execFileAsync(bin, ['--version'], { encoding: 'utf8', timeout: 8000, env: envWithBinDir(bin) });
    return { ok: true, version: (out.trim().split('\n').pop() || '').trim() };
  } catch (e) {
    const detail = firstErrLine(e.stderr || e.message);
    return { ok: false, reason: 'broken', detail };
  }
}

const dshBinPrefFile = () => path.join(app.getPath('userData'), 'dsh-bin.json');

// bin-каталоги всех версий node из nvm, свежие первыми.
function nvmBinDirs() {
  const nvmDir = process.env.NVM_DIR || path.join(os.homedir(), '.nvm');
  try {
    return fs.readdirSync(path.join(nvmDir, 'versions', 'node'))
      .sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))
      .map((v) => path.join(nvmDir, 'versions', 'node', v, 'bin'));
  } catch { return []; } // nvm нет
}

// Окружение с каталогом бинарника первым в PATH (для `#!/usr/bin/env node`-скриптов).


// Исполняемый файл: рядом с dsh, в PATH, иначе в nvm (при запуске с ярлыка
// nvm в PATH нет).
function findBin(name) {
  const dirs = [path.dirname(state.dshBin)]
    .concat(String(process.env.PATH || '').split(path.delimiter).filter(Boolean), nvmBinDirs());
  for (const dir of dirs) {
    const bin = path.join(dir, name);
    try { fs.accessSync(bin, fs.constants.X_OK); return bin; } catch { /* дальше */ }
  }
  return null;
}
const findNpm = () => findBin('npm');

// Где искать dsh, если DSH_BIN не задан: запомненный путь, /usr/bin/dsh,
// PATH, типичные каталоги пользовательских npm-установок, все версии nvm
// (свежие первыми). npm prefix -g — последним, он медленный.
function dshCandidates() {
  const home = os.homedir();
  const list = [];
  try { list.push(JSON.parse(fs.readFileSync(dshBinPrefFile(), 'utf8')).bin); } catch { /* не запоминали */ }
  list.push(DSH_BIN_DEFAULT);
  for (const dir of String(process.env.PATH || '').split(path.delimiter)) if (dir) list.push(path.join(dir, 'dsh'));
  list.push(path.join(home, '.npm-global', 'bin', 'dsh'), path.join(home, '.local', 'bin', 'dsh'), '/usr/local/bin/dsh');
  for (const dir of nvmBinDirs()) list.push(path.join(dir, 'dsh'));
  return [...new Set(list.filter(Boolean))];
}

// Проверяем не только наличие файла, но и что бинарник реально запускается:
// битый symlink, несовместимый Node или отсутствующий npm дадут ошибку здесь,
// а не загадочным ENOENT в момент старта. dsh --version занимает ~60 мс.
// Асинхронно: при медленной системе синхронный вызов замораживал бы окно.
// Если DSH_BIN не задан явно — перебираем кандидатов (dshCandidates) и
// переключаем DSH_BIN на первый рабочий, запоминая его в userData/dsh-bin.json.
async function dshInstallCheck() {
  if (DSH_BIN_EXPLICIT) return probeDsh(state.dshBin);
  let firstBroken = null;
  const tryBin = async (bin) => {
    const r = await probeDsh(bin);
    if (r.ok) {
      if (bin !== state.dshBin) console.log(`[launcher] dsh найден: ${bin} (${r.version})`);
      state.dshBin = bin;
      try {
        fs.mkdirSync(path.dirname(dshBinPrefFile()), { recursive: true });
        fs.writeFileSync(dshBinPrefFile(), JSON.stringify({ bin }));
      } catch (e) { console.error('[launcher] dsh-bin.json:', e.message); }
    } else if (r.reason === 'broken' && !firstBroken) {
      firstBroken = { ...r, bin };
    }
    return r.ok ? r : null;
  };
  for (const bin of dshCandidates()) {
    const r = await tryBin(bin);
    if (r) return r;
  }
  const npm = findNpm();
  if (npm) {
    try {
      const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
      const r = await tryBin(path.join(prefix, 'bin', 'dsh'));
      if (r) return r;
    } catch { /* npm не отвечает */ }
  }
  if (firstBroken) {
    state.dshBin = firstBroken.bin; // показываем на странице тот, что сломан
    return { ok: false, reason: 'broken', detail: firstBroken.detail };
  }
  state.dshBin = DSH_BIN_DEFAULT;
  return { ok: false, reason: 'missing' };
}

// Есть ли npm и доступен ли глобальный prefix без sudo — от этого зависит,
// можно ли ставить dsh кнопкой прямо из окна.
const INSTALL_CMD = 'npm install -g @deepseek-ai/dsh';
// Право записи проверяем у ближайшего существующего каталога: на чистой
// системе <prefix>/lib/node_modules ещё нет (появится при первой глобальной
// установке), и ENOENT от него раньше ошибочно означал «npm нет».


// dsh нужен Node.js 22+. npm ставит его и на более старый Node без единого
// предупреждения (Ubuntu 24.04: Node 18 из apt), а потом dsh не запускается.
const MIN_NODE_MAJOR = 22;

/* ===== rootInstallMode (тест test/dsh-install.js) =====
   Как ставить глобально, когда prefix принадлежит root: 'pkexec' — npm и
   prefix системные (не в домашнем каталоге), можно запустить установку от
   root через pkexec — polkit спросит пароль системным диалогом, как при
   обновлении самого лаунчера. 'manual' — всё остальное: npm из nvm или
   ~/.npm-global под sudo ставить нельзя (root создаст файлы в домашнем
   каталоге, и пользователь не сможет их обновлять), да и pkexec может не
   быть. */
function rootInstallMode({ npm, prefix, hasPkexec, home }) {
  if (!npm || !prefix || !home) return 'manual';
  const inHome = (p) => p === home || p.startsWith(home + path.sep);
  if (inHome(npm) || inHome(prefix)) return 'manual';
  return hasPkexec ? 'pkexec' : 'manual';
}
/* ===== end rootInstallMode ===== */

async function nodeVersionNear(npm) {
  try {
    const { stdout } = await execFileAsync('node', ['--version'], { encoding: 'utf8', timeout: 5000, env: envWithBinDir(npm) });
    return stdout.trim();
  } catch { return null; }
}

/* ===== pkexecNpmArgs (тест test/dsh-install.js) =====
   Аргументы pkexec для установки npm-пакета от root. pkexec запускает
   программу с минимальным окружением: PATH без каталога npm ломает
   `#!/usr/bin/env node` (npm — это JS-скрипт). Поэтому сначала каталог npm
   первым в PATH, а аргументы передаются через "$@" — версия пакета не
   склеивается в shell-строку. Результат: [pkexec, ...эти аргументы]. */
function pkexecNpmArgs(npm, npmArgs) {
  const script = 'PATH="$1:$PATH"; export PATH; shift; exec "$@"';
  return ['/bin/sh', '-c', script, 'sh', path.dirname(npm), npm, ...npmArgs];
}
/* ===== end pkexecNpmArgs ===== */

async function npmInstallInfo() {
  const npm = findNpm();
  if (!npm) return { hasNpm: false, canAuto: false, command: INSTALL_CMD };
  const nodeVersion = await nodeVersionNear(npm);
  const major = Number((/^v(\d+)/.exec(nodeVersion || '') || [])[1]);
  const nodeOld = Number.isFinite(major) && major < MIN_NODE_MAJOR;
  const pkexec = findBin('pkexec');
  try {
    const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
    const writable = writableOrCreatable(path.join(prefix, 'lib', 'node_modules')) &&
      writableOrCreatable(path.join(prefix, 'bin'));
    // prefix root'а: ставим через pkexec (системный npm) или руками (nvm и
    // прочие каталоги в домашнем).
    const rootMode = writable ? null : rootInstallMode({ npm, prefix, hasPkexec: Boolean(pkexec), home: os.homedir() });
    return { hasNpm: true, canAuto: writable && !nodeOld, nodeVersion, nodeOld, npm, prefix, pkexec, rootMode,
      command: writable ? INSTALL_CMD : 'sudo ' + INSTALL_CMD };
  } catch (e) {
    console.error('[launcher] npm prefix -g:', e.message);
    return { hasNpm: true, canAuto: false, nodeVersion, nodeOld, npm, pkexec, rootMode: 'manual', command: 'sudo ' + INSTALL_CMD };
  }
}

module.exports = { dshEnv, probeDsh, findBin, dshInstallCheck, MIN_NODE_MAJOR, rootInstallMode, pkexecNpmArgs, npmInstallInfo };
