'use strict';
// Страница «dsh не установлен»: команда установки, установка в окне.
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc } = require('./util');
const { DSH_BIN_DEFAULT, DSH_BIN_EXPLICIT } = require('./env');
const { dshInstallCheck, MIN_NODE_MAJOR, npmInstallInfo } = require('./dsh-install');
const pages = require('./pages');

/* ---------------- страница «dsh не установлен» ----------------
 * Порт закрыт и бинарника dsh нет — вместо попытки старта и ENOENT показываем
 * инструкцию: команда установки, «Скопировать», (если глобальный npm prefix
 * доступен без sudo) «Установить» с живым логом, и «Проверить ещё раз».
 * Состояние кэшируем: во время установки страница перерисовывается каждые
 * 800 мс, и запускать проверки в каждом рендере незачем.
 */
let installState = { check: null, npmInfo: null, running: false, log: [], timer: null, proc: null };

// Закрытие лаунчера: процесс установки и таймер перерисовки (before-quit →
// state.settleAllOperations, main.js).
function settleInstallState() {
  if (installState.proc) { try { installState.proc.kill('SIGTERM'); } catch { /* noop */ } }
  clearInterval(installState.timer);
}
state.settleOperation(settleInstallState);

async function refreshInstallState() {
  [installState.check, installState.npmInfo] = await Promise.all([dshInstallCheck(), npmInstallInfo()]);
}

async function showInstallPage(note, noteIsError = false) {
  if (!state.win || state.win.isDestroyed()) return;
  state.currentPage = () => showInstallPage(note, noteIsError);
  if (typeof note === 'function') note = note();
  if (!installState.running) {
    try { await refreshInstallState(); } catch (e) { console.error('[launcher] refreshInstallState:', e.message); }
    if (!state.win || state.win.isDestroyed() || !installState.check) return;
  }
  const check = installState.check;
  const npmInfo = installState.npmInfo;
  const installing = installState.running;
  const noteHtml = note
    ? `<p style="color:${noteIsError ? '#f85149' : '#3fb950'}">${esc(note)}</p>`
    : '';
  const body = `
    ${noteHtml}
    <p>${installing
      ? tr(`Лаунчер не может запустить DeepSeek Harness: идёт установка <code>dsh</code>.`,
           `The launcher cannot start DeepSeek Harness: <code>dsh</code> is being installed.`)
      : check.reason === 'missing' && DSH_BIN_EXPLICIT
        ? tr(`Лаунчер не может запустить DeepSeek Harness: по пути <code>${esc(state.dshBin)}</code> бинарника <code>dsh</code> нет.`,
             `The launcher cannot start DeepSeek Harness: there is no <code>dsh</code> binary at <code>${esc(state.dshBin)}</code>.`)
      : check.reason === 'missing'
        ? tr(`Лаунчер не нашёл <code>dsh</code>: искал <code>${DSH_BIN_DEFAULT}</code>, в PATH, в каталогах npm и nvm.`,
             `The launcher could not find <code>dsh</code>: looked at <code>${DSH_BIN_DEFAULT}</code>, in PATH, and in npm and nvm directories.`)
        : tr(`Лаунчер не может запустить DeepSeek Harness: бинарник <code>${esc(state.dshBin)}</code> не запускается.`,
             `The launcher cannot start DeepSeek Harness: the <code>${esc(state.dshBin)}</code> binary fails to run.`)}</p>
    ${!installing && check.detail ? `<p>${tr('Детали', 'Details')}: <code>${esc(check.detail)}</code></p>` : ''}
    ${installing
      ? `<p>${tr('Последние строки установки:', 'Latest install output:')}</p>\n     ${pages.liveLogPre(installState.log)}`
      : `<p>${tr('Установите dsh (нужен Node.js 22+ и npm):', 'Install dsh (requires Node.js 22+ and npm):')}</p>\n     <pre>` + esc(npmInfo.command) + '</pre>\n     ' + (
          npmInfo.nodeOld
            ? tr(`<p>Но сейчас найден Node.js <code>${esc(npmInfo.nodeVersion)}</code>, а dsh нужен Node.js ${MIN_NODE_MAJOR} или новее — на нём dsh не запустится. Сначала обновите Node.js (например, через nvm: <code>nvm install ${MIN_NODE_MAJOR}</code>, или пакеты NodeSource), затем установите dsh и нажмите «Проверить ещё раз».</p>`,
                 `<p>However, Node.js <code>${esc(npmInfo.nodeVersion)}</code> was found, and dsh needs Node.js ${MIN_NODE_MAJOR} or newer — dsh will not run on it. Upgrade Node.js first (e.g. with nvm: <code>nvm install ${MIN_NODE_MAJOR}</code>, or NodeSource packages), then install dsh and click “Check again”.</p>`)
          : npmInfo.canAuto
            ? tr('<p>Или нажмите «Установить» — лаунчер выполнит команду сам.</p>',
                 '<p>Or click “Install” — the launcher will run the command for you.</p>')
            : npmInfo.hasNpm && npmInfo.rootMode === 'pkexec'
              ? tr('<p>Или нажмите «Установить» — лаунчер поставит dsh от root: система спросит пароль.</p>',
                   '<p>Or click “Install” — the launcher will install dsh as root and the system will ask for your password.</p>')
              : npmInfo.hasNpm
                ? tr('<p>Этой команде нужен <code>sudo</code> — выполните её в терминале и нажмите «Проверить ещё раз».</p>',
                     '<p>This command needs <code>sudo</code> — run it in a terminal and click “Check again”.</p>')
                : tr('<p>npm в системе нет: сначала установите Node.js (22+) с npm, затем команду выше, и нажмите «Проверить ещё раз».</p>',
                     '<p>npm is not installed: install Node.js (22+) with npm first, then the command above, and click “Check again”.</p>')
        )}
    <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
      ${installing ? '' : `<button onclick="location.href='dshlauncher://install/copy/'">${tr('Скопировать команду', 'Copy command')}</button>`}
      ${installing ? '' : (npmInfo.canAuto || npmInfo.rootMode === 'pkexec' ? `<button onclick="location.href='dshlauncher://install/run/'">${tr('Установить', 'Install')}</button>` : '')}
      <button onclick="location.href='dshlauncher://install/recheck/'">${tr('Проверить ещё раз', 'Check again')}</button>
    </div>`;
  if (installing) pages.liveLogSync(installState);
  pages.showStatus(installing ? tr('Устанавливаю dsh…', 'Installing dsh…') : tr('Нужен DeepSeek Harness', 'DeepSeek Harness is required'), body, !installing);
}

Object.assign(module.exports, { installState, refreshInstallState, showInstallPage }); // дополняем, а не заменяем: модули подключают друг друга по кругу
