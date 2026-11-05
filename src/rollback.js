'use strict';
// «Справка → Вернуть предыдущую версию…» (Help → Roll back to a previous
// version…): list of past launcher releases (GitHub) and rollback —
// a restart, and the new instance immediately installs the chosen version
// through the same path as an update (download, sha256, pkexec), with
// downgrade allowed; the newest version is "don't ask again".
const state = require('./state');
const { tr } = require('./i18n');
const { esc, timedCheck } = require('./util');
const { rollbackCandidates } = require('./updates-logic');
const dialogs = require('./dialogs');
const settingsWindow = require('./settings-window');
const updates = require('./updates');

let candidates = [];

async function showRollbackDialog() {
  const title = tr('Вернуть предыдущую версию', 'Roll back to a previous version');
  const gen = dialogs.openDialog(title);
  if (gen === null) return;
  state.dialogCopyText = '';
  dialogs.dialogShow(gen, () => dialogs.dialogPage(title,
    `<p>${tr('Загружаю список версий…', 'Loading the list of versions…')}</p><div class="actions">${dialogs.dialogCloseButton()}</div>`));
  const installed = await updates.launcherInstalledVersion();
  let latest = null;
  if (installed) latest = await timedCheck(updates.fetchLauncherLatest(), updates.UPDATE_CHECK_TIMEOUT);
  candidates = latest && latest.all ? rollbackCandidates(latest.all, installed) : [];
  dialogs.dialogShow(gen, () => {
    let body;
    if (!installed) {
      body = `<p>${tr('Лаунчер запущен не из пакета .deb — вернуть версию можно, только установив нужный пакет со',
        'The launcher is not running from a .deb package — to roll back, install the package you need from the')}
        <a href="https://github.com/${esc(updates.UPDATE_GITHUB_REPO)}/releases" target="_blank">${tr('страницы релизов', 'releases page')}</a>.</p>`;
    } else if (!latest) {
      body = `<p class="note err">${tr('Не удалось получить список версий с GitHub.', 'Could not get the list of versions from GitHub.')}</p>`;
    } else if (!candidates.length) {
      body = `<p>${tr(`Установлена <code>${esc(installed)}</code> — более ранних версий с пакетом .deb нет.`,
        `Installed: <code>${esc(installed)}</code> — there are no earlier versions with a .deb package.`)}</p>`;
    } else {
      body = `
        <p>${tr(`Установлена <code>${esc(installed)}</code>. Выберите версию:`, `Installed: <code>${esc(installed)}</code>. Choose a version:`)}</p>
        <table style="border-collapse:collapse; margin-bottom:10px">
          ${candidates.map((c) => `<tr>
            <td style="padding:4px 14px 4px 0"><code>${esc(c.version)}</code></td>
            <td style="padding:4px 14px 4px 0; color:#9aa4b2">${esc(c.date || '')}</td>
            <td style="padding:4px 14px 4px 0">${c.htmlUrl ? `<a href="${esc(c.htmlUrl)}" target="_blank">${tr('что в ней', 'release notes')}</a>` : ''}</td>
            <td style="padding:4px 0"><button onclick="location.href='dshlauncher://rollback/?v=${encodeURIComponent(c.version)}'">${tr('Вернуть', 'Roll back')}</button></td>
          </tr>`).join('')}
        </table>
        <p class="hint">${tr('Лаунчер перезапустится и установит выбранную версию (нужен пароль администратора), dsh будет остановлен. О более новой версии лаунчер спрашивать не будет — это можно вернуть в «Настройки → Обновления».',
          'The launcher will restart and install the chosen version (the administrator password is needed), dsh will be stopped. The launcher won’t offer the newer version again — this can be undone in “Settings → Updates”.')}</p>`;
    }
    dialogs.dialogPage(title, `${body}<div class="actions">${dialogs.dialogCloseButton()}</div>`);
  });
}

// The «Вернуть» (Roll back) button: only a version from the shown list.
function rollbackTo(version) {
  if (!candidates.some((c) => c.version === version)) return;
  process.env.DSH_LAUNCHER_UPDATE_NOW = 'launcher';
  process.env.DSH_LAUNCHER_INSTALL_VERSION = version;
  settingsWindow.relaunchLauncher(`чтобы вернуть версию ${version}`);
}

Object.assign(module.exports, { showRollbackDialog, rollbackTo }); // extend rather than replace: the modules require each other in a circle
