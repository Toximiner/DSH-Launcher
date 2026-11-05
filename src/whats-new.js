'use strict';
// «Что нового» (What's new) after a launcher update.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck } = require('./util');
const { tagToPackageVersion, shouldShowWhatsNew, notesBetween } = require('./versions');
const { IS_PKG_INSTALL } = require('./env');
const dialogs = require('./dialogs');
const menus = require('./menus');
const updates = require('./updates');

/* ---------------- «Что нового» (What's new) after an update ---------------- */

// The version the launcher was started from last time. It became higher —
// after the GUI loads we show the changes between them once. The first
// start — we just remember it. No GitHub answer — we don't remember the
// version: we'll show it after a few repeat requests or on the nearest
// successful update check (retryWhatsNew).
const lastVersionFile = () => path.join(app.getPath('userData'), 'last-version.json');
let whatsNewChecked = false;
let whatsNewInFlight = false;
let whatsNewAttempted = false; // the first attempt after the GUI loads (main.js)

function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }

function saveLastVersion(v) {
  // a stand-in "installed" version (development/tests) — not the real one:
  // don't remember it, otherwise "What's new" will later call it the
  // previous version
  if (process.env.DSH_LAUNCHER_FAKE_INSTALLED) return;
  try {
    fs.mkdirSync(path.dirname(lastVersionFile()), { recursive: true });
    fs.writeFileSync(lastVersionFile(), JSON.stringify({ version: v }));
  } catch (e) { console.error('[launcher] last-version.json:', e.message); }
}

// freshLatest — a ready release list from a successful update check, if
// at start GitHub didn't answer and we show "What's new" late.
async function maybeShowWhatsNew(freshLatest) {
  if (whatsNewChecked || whatsNewInFlight) return;
  whatsNewAttempted = true;
  whatsNewInFlight = true;
  try {
    const current = await menus.launcherVersionRaw();
    if (!current) { whatsNewChecked = true; return; }
    let last = null;
    try { last = JSON.parse(fs.readFileSync(lastVersionFile(), 'utf8')).version || null; } catch { /* the first start */ }
    const scheme = IS_PKG_INSTALL ? 'deb' : 'semver';
    if (!shouldShowWhatsNew(last, current, scheme)) { saveLastVersion(current); whatsNewChecked = true; return; }
    if (updates.NO_UPDATE_CHECK) { saveLastVersion(current); whatsNewChecked = true; return; } // we can't go to the network — silently
    let latest = freshLatest || await state.updateChecks.launcher;
    // The release list from the cache (up to an hour) may have been fetched
    // before this version came out (e.g., the package was installed via
    // apt) — then its description isn't in it: request it again, bypassing
    // the cache.
    const hasCurrent = (l) => l && Array.isArray(l.releases) && l.releases.some((r) =>
      (scheme === 'deb' ? tagToPackageVersion(r.version) : r.version) === current);
    if (latest && !hasCurrent(latest)) {
      console.log(`[launcher] в кэше релизов нет ${current} — запрашиваю список заново`);
      latest = (await timedCheck(updates.fetchLauncherLatest({ force: true }), updates.UPDATE_CHECK_TIMEOUT)) || latest;
    }
    // No GitHub answer — a couple more attempts: the network may be slow.
    // We can't remember the version until the changes are shown.
    for (let i = 0; i < 2 && (!latest || !latest.releases); i++) {
      if (state.stopping) return;
      await sleep(5000);
      latest = (await timedCheck(updates.fetchLauncherLatest(), updates.UPDATE_CHECK_TIMEOUT)) || null;
    }
    if (!latest || !latest.releases) return; // we'll retry on the nearest successful check (retryWhatsNew)
    whatsNewChecked = true;
    const items = notesBetween(latest.releases, last, current, scheme);
    console.log(`[launcher] обновлён ${last} → ${current} — показываю «Что нового» (${items.length})`);
    saveLastVersion(current);
    const title = tr('Что нового', 'What’s new');
    const gen = dialogs.openDialog(title);
    if (gen === null) return;
    state.dialogCopyText = '';
    const link = `https://github.com/${updates.UPDATE_GITHUB_REPO}/releases`;
    dialogs.dialogShow(gen, () => dialogs.dialogPage(title, `
      <p style="margin:0 0 4px">${tr('DeepSeek Harness Launcher обновлён:', 'DeepSeek Harness Launcher was updated:')} <code>${esc(last)}</code> → <code>${esc(current)}</code></p>
      ${updates.notesBlockHtml(items) || `<p>${tr('Подробности — на', 'Details are on the')} <a href="${esc(link)}" target="_blank">${tr('странице релизов', 'releases page')}</a>.</p>`}
      <div class="actions">${dialogs.dialogCloseButton()}</div>`));
  } finally {
    whatsNewInFlight = false;
  }
}

// Called from the update check when the release list is already fetched:
// if at start GitHub didn't answer (the version wasn't remembered), we
// show the missed changes now, instead of accumulating them until the
// next update. (Only after the first attempt after the GUI loads — we
// don't open dialogs earlier.)
function retryWhatsNew(freshLatest) {
  if (whatsNewAttempted && !whatsNewChecked) void maybeShowWhatsNew(freshLatest);
}

Object.assign(module.exports, { maybeShowWhatsNew, retryWhatsNew }); // extend, don't replace: the modules require each other in a circle
