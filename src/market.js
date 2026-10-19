'use strict';
// Плагин маркета (dshmarket): вопрос и установка.
const fs = require('fs');
const os = require('os');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, profileFromArgs } = require('./util');
const { envWithBinDir } = require('./system');
const { runWithLiveLog } = require('./proc-log');
const { prefFile, readPref, writePref } = require('./prefs');
const { DSH_ARGS, CWD } = require('./env');
const { dshEnv, findBin, npmInstallInfo, pkexecNpmArgs } = require('./dsh-install');
const pages = require('./pages');
const { blankResponse } = require('./respond'); // чистый модуль — прямой require, без реестра

/* ---------------- плагин маркета (dshmarket) ----------------
 * Перед тем как лаунчер сам запускает dsh, проверяем, что в профиле стоит
 * плагин маркета. Если нет — спрашиваем пользователя: «Установить»
 * (dsh plugin --profile <профиль> add dshmarket, с живым логом), «Не сейчас»
 * или «Не спрашивать больше» (запоминается в userData/market-prompt.json).
 * Ставим именно до старта: плагины dsh подхватывает при запуске. К уже
 * запущенному dsh (режим подключения) вопрос не задаём.
 * Проверка — чтение $DSH_HOME/profiles/<профиль>/package.json (по умолчанию
 * ~/.dsh), без запуска dsh.
 */
const MARKET_PKG = 'dshmarket';


const DSH_PROFILE = profileFromArgs(DSH_ARGS);

/* ===== marketInPkg (чистая функция, тест test/parsing.js) =====
   Есть ли плагин маркета в package.json профиля: в dependencies или в
   dsh.profile.bundles. */
function marketInPkg(pkg) {
  const bundles = (pkg.dsh && pkg.dsh.profile && pkg.dsh.profile.bundles) || [];
  return Boolean(pkg.dependencies && pkg.dependencies[MARKET_PKG]) || bundles.includes(MARKET_PKG);
}
/* ===== end marketInPkg ===== */

function marketInstalled() {
  if (!DSH_PROFILE) return true; // профиль не задан — не знаем, где смотреть, не спрашиваем
  try {
    const dshHome = process.env.DSH_HOME || path.join(os.homedir(), '.dsh');
    const pkgFile = path.join(dshHome, 'profiles', DSH_PROFILE, 'package.json');
    const pkg = JSON.parse(fs.readFileSync(pkgFile, 'utf8'));
    return marketInPkg(pkg);
  } catch {
    return false; // профиля ещё нет — dsh plugin add создаст его сам
  }
}

const MARKET_PREFS = 'market-prompt.json';

function marketPromptDisabled() {
  const p = readPref(MARKET_PREFS);
  return Boolean(p && p.dontAsk === true);
}

let marketState = { resolve: null, running: false, log: [], timer: null, proc: null, cmdLine: null };

// Закрытие лаунчера: процесс установки и таймер перерисовки (before-quit →
// state.settleAllOperations, main.js).
function settleMarketState() {
  if (marketState.proc) { try { marketState.proc.kill('SIGTERM'); } catch { /* noop */ } }
  clearInterval(marketState.timer);
}
state.settleOperation(settleMarketState);

function marketCommand() {
  return `${state.dshBin} plugin --profile ${DSH_PROFILE} add ${MARKET_PKG}`;
}

function showMarketPage(note, noteIsError = false) {
  state.currentPage = () => showMarketPage(note, noteIsError);
  if (typeof note === 'function') note = note();
  const installing = marketState.running;
  const noteHtml = note
    ? `<p style="color:${noteIsError ? '#f85149' : '#3fb950'}">${esc(note)}</p>`
    : '';
  const body = installing
    ? `<p>${tr('Выполняю', 'Running')} <code>${esc(marketState.cmdLine || marketCommand())}</code></p>
       <p>${tr('Последние строки установки:', 'Latest install output:')}</p>
       ${pages.liveLogPre(marketState.log)}`
    : `${noteHtml}
       <p>${tr(`В профиле <code>${esc(DSH_PROFILE)}</code> не найден плагин маркета
          <code>${MARKET_PKG}</code>. Установить его перед запуском dsh?`,
          `The marketplace plugin <code>${MARKET_PKG}</code> is not installed in the
          <code>${esc(DSH_PROFILE)}</code> profile. Install it before starting dsh?`)}</p>
       <pre>${esc(marketCommand())}</pre>
       ${noteIsError && marketState.log.length ? `<pre>${esc(marketState.log.slice(-20).join('\n'))}</pre>` : ''}
       <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
         <button onclick="location.href='dshlauncher://market/install/'">${tr('Установить', 'Install')}</button>
         <button onclick="location.href='dshlauncher://market/skip/'">${tr('Не сейчас', 'Not now')}</button>
         <button onclick="location.href='dshlauncher://market/never/'">${tr('Не спрашивать больше', 'Don’t ask again')}</button>
       </div>`;
  if (installing) pages.liveLogSync(marketState);
  pages.showStatus(installing ? tr(`Устанавливаю ${MARKET_PKG}…`, `Installing ${MARKET_PKG}…`)
    : tr(`Установить ${MARKET_PKG}?`, `Install ${MARKET_PKG}?`), body, noteIsError);
}

// Показывает вопрос и ждёт нажатия: 'install' | 'skip' | 'never'.
function askMarket(note, noteIsError) {
  return new Promise((resolve) => {
    marketState.resolve = resolve;
    showMarketPage(note, noteIsError);
  });
}

function onMarketChoice(choice) {
  const resolve = marketState.resolve;
  if (resolve && !marketState.running) {
    marketState.resolve = null;
    resolve(choice);
  }
  return blankResponse();
}

// dsh plugin add вызывает pnpm — его каталог тоже должен быть в PATH.
function marketEnv() {
  const env = dshEnv();
  const pnpm = findBin('pnpm');
  if (pnpm) env.PATH = path.dirname(pnpm) + path.delimiter + env.PATH;
  return env;
}

// Запуск шага установки (pnpm или сам маркет) с живым логом на странице
// (pages.liveLogTick — строки дописываются без перезагрузки, скролл за
// пользователем).
function runMarketInstall(cmd, args, env) {
  marketState.cmdLine = [cmd, ...args].join(' ');
  console.log(`[launcher] установка: ${marketState.cmdLine}`);
  marketState.running = true;
  showMarketPage();
  return runWithLiveLog(marketState, cmd, args, { cwd: CWD, env, redraw: () => {
    // running — иначе страница уже переключилась (успех/ошибка/новый вопрос),
    // и доп. перерисовка затёрла бы её note (тик мог разрешиться позже).
    pages.liveLogTick(marketState).then((ok) => {
      if (!ok && marketState.running) showMarketPage();
    });
  } });
}

async function offerMarketIfMissing() {
  if (marketInstalled()) return;
  if (marketPromptDisabled()) {
    console.log(`[launcher] ${MARKET_PKG} не установлен, вопрос отключён (${prefFile(MARKET_PREFS)})`);
    return;
  }
  console.log(`[launcher] ${MARKET_PKG} не установлен в профиле ${DSH_PROFILE} — спрашиваю пользователя`);
  let note = null;
  let noteIsError = false;
  for (;;) {
    const choice = await askMarket(note, noteIsError);
    if (state.stopping) return;
    if (choice === 'never') {
      writePref(MARKET_PREFS, { dontAsk: true });
      console.log('[launcher] пользователь отказался от маркета навсегда');
      return;
    }
    if (choice !== 'install') {
      console.log('[launcher] установка маркета отложена (не сейчас)');
      return;
    }
    // dsh plugin add требует pnpm: на чистой системе его нет (код 127).
    if (!findBin('pnpm')) {
      const npmInfo = await npmInstallInfo();
      if (state.stopping) return;
      const pnpmArgs = ['install', '-g', 'pnpm'];
      // Системный npm — ставим через pkexec (пароль спросит polkit), иначе
      // просим выполнить в терминале: root в nvm/~/.npm-global — нельзя.
      const viaPkexec = !npmInfo.canAuto && npmInfo.rootMode === 'pkexec';
      if (!npmInfo.canAuto && !viaPkexec) {
        const cmd = npmInfo.hasNpm ? 'sudo npm install -g pnpm' : 'npm install -g pnpm';
        console.log(`[launcher] pnpm нет, поставить сами не можем — прошу «${cmd}»`);
        marketState.log = [];
        note = () => tr(`Для плагинов dsh нужен pnpm, а его в системе нет. Выполните в терминале «${cmd}» и нажмите «Установить» ещё раз.`,
          `dsh plugins need pnpm, which is not installed. Run “${cmd}” in a terminal and click “Install” again.`);
        noteIsError = true;
        continue;
      }
      console.log(viaPkexec ? '[launcher] pnpm нет — ставлю через pkexec' : '[launcher] pnpm нет — ставлю через npm');
      const pc = await runMarketInstall(
        viaPkexec ? npmInfo.pkexec : npmInfo.npm,
        viaPkexec ? pkexecNpmArgs(npmInfo.npm, pnpmArgs) : pnpmArgs,
        viaPkexec ? undefined : envWithBinDir(npmInfo.npm));
      if (state.stopping) return;
      if (pc !== 0 || !findBin('pnpm')) {
        console.error(`[launcher] установка pnpm не удалась (код ${pc})`);
        note = () => tr(`Не удалось установить pnpm (код ${pc}). Можно повторить или продолжить без маркета.`,
          `Failed to install pnpm (code ${pc}). You can retry or continue without the marketplace.`);
        noteIsError = true;
        continue;
      }
    }
    const code = await runMarketInstall(state.dshBin, ['plugin', '--profile', DSH_PROFILE, 'add', MARKET_PKG], marketEnv());
    if (state.stopping) return;
    if (code === 0 && marketInstalled()) {
      console.log(`[launcher] ${MARKET_PKG} установлен`);
      return;
    }
    console.error(`[launcher] установка ${MARKET_PKG} не удалась (код ${code})`);
    note = () => tr(`Установка не удалась (код ${code}). Можно повторить или продолжить без маркета.`,
      `Installation failed (code ${code}). You can retry or continue without the marketplace.`);
    noteIsError = true;
  }
}

Object.assign(module.exports, { marketState, onMarketChoice, offerMarketIfMissing }); // дополняем, а не заменяем: модули подключают друг друга по кругу
