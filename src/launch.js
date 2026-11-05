'use strict';
// Launching dsh and driving the window to the GUI (prompts at start, waiting for the port).
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc } = require('./util');
const { DSH_ARGS, PLAIN_URL } = require('./env');
const { waitForPort, waitForAuthUrl } = require('./port'); // pure module — direct require, no registry
const dshProcess = require('./dsh-process');
const market = require('./market');
const pages = require('./pages');
const updatesLive = require('./updates-live');
const updates = require('./updates');
const launcher = require('./launcher'); // shared registry (populated by main.js)

/* ---------------- launching dsh and driving the window to the GUI ---------------- */

// Protection against a double launch: a double click on "Check again" or
// "Install" would call launchDsh twice — the second spawn would overwrite
// dshProc, and the first process would be left unsupervised (not killed on close).


function ourDshAlive() {
  return state.weStartedDsh && state.dshProc && state.dshProc.exitCode === null && !state.dshFatal;
}

async function launchDsh() {
  if (state.launching || ourDshAlive()) {
    console.log('[launcher] запуск dsh уже идёт — повторный вызов пропущен');
    return;
  }
  state.launching = true;
  try {
    if (await updates.offerLauncherUpdate()) return; // the restart into the new version — it will bring dsh up
    if (state.stopping) return;
    await updates.offerDshUpdate();
    if (state.stopping) return;
    await market.offerMarketIfMissing();
    if (state.stopping) return;
    void updatesLive.refreshAvailableUpdates(); // a deferred update — as an item in "Help"
    await launchDshInner();
  } finally {
    state.launching = false;
  }
}

async function launchDshInner() {
  pages.showPage(() => pages.showStatus(tr('Запуск DeepSeek Harness…', 'Starting DeepSeek Harness…'), tr(
    `<p>Запускаю <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>.<br>Жду, пока сервис поднимется…</p>`,
    `<p>Running <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>.<br>Waiting for the service to come up…</p>`)));
  dshProcess.startDsh();

  const up = await waitForPort();
  if (!up) {
    pages.showPage(() => pages.showStatus(tr('Не удалось запустить dsh', 'Failed to start dsh'), tr(
      `<p>Процесс не поднял сервис на <code>${PLAIN_URL}</code>.<br>
         Проверьте, что команда запускается в терминале:
         <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>.<br>
         Последние строки журнала:</p>`,
      `<p>The process did not bring up the service at <code>${PLAIN_URL}</code>.<br>
         Check that the command runs in a terminal:
         <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>.<br>
         Latest log lines:</p>`) + pages.stderrBlock(), true));
    launcher.startWatchdog(); // if dsh is started manually — the window comes back to life on its own
    return;
  }

  // The port has already been brought up by us: a watchdog check that started during the launch
  // and finished after must not take this for a dsh restart.
  state.portState = 'up';
  const url = state.authUrl || (await waitForAuthUrl());
  if (url) {
    await pages.loadInWin(url); // exchanging the token for cookies, then a redirect to the plain URL
  } else if (await launcher.plainUrlIsAuthed()) {
    await pages.loadInWin(PLAIN_URL); // the old cookie is still valid
  } else {
    pages.showPastePage();
  }
  launcher.startWatchdog();
}

Object.assign(module.exports, { launchDsh, launchDshInner }); // we extend, not replace: the modules require each other cyclically
