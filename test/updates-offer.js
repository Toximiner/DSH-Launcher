'use strict';
/*
 * test/updates-offer.js — обновленческие сценарии из src/updates.js (extract
 * по маркерам, _extract.js): onUpdateChoice (кнопки страницы), offerUpdateFlow
 * (цикл вопроса), downloadDeb (.deb + sha256 по байтам), offerDshUpdate
 * (npm / pkexec / sudo / nodeOld / recheck).
 * Run: node test/updates-offer.js (or npm test).
 */
const assert = require('assert');
const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { Readable } = require('stream');
const { extract } = require('./_extract');
const { compareSemVer, notesBetween } = require('../src/versions');
const { dshNotesPart } = require('../src/notes');

const tr = (ru) => ru; // русский — язык по умолчанию в тестах
const blank = { status: 200 };
const fakeConsole = () => {
  const logs = [];
  return { logs, log: (...a) => logs.push(a.join(' ')), error: (...a) => logs.push(a.join(' ')), info: (...a) => logs.push(a.join(' ')), warn: (...a) => logs.push(a.join(' ')) };
};

/* ------------------------------------------------------------------ */
/* onUpdateChoice                                                      */
/* ------------------------------------------------------------------ */
function choiceEnv(over = {}) {
  const calls = { copy: [], page: [], restart: 0 };
  const updateState = { manualCmd: over.manualCmd || null, running: Boolean(over.running), resolve: over.resolve || null };
  if (updateState.resolve === null && !over.noResolve) updateState.resolve = (c) => { calls.resolved = c; };
  const d = {
    clipboard: { writeText: (t) => calls.copy.push(t) },
    updateState,
    showUpdatePage: (note, isError) => calls.page.push({ note, isError }),
    tr,
    blankResponse: () => blank,
    restartLauncher: () => { calls.restart++; },
  };
  const onUpdateChoice = extract('onUpdateChoice', d);
  return { onUpdateChoice, calls, updateState };
}

async function tOnUpdateChoice() {
  // 1) copy — команда в буфер + заметка, вопрос НЕ разрешается
  {
    const { onUpdateChoice, calls } = choiceEnv({ manualCmd: 'CMD' });
    await onUpdateChoice('copy');
    assert.deepStrictEqual(calls.copy, ['CMD']);
    assert.strictEqual(calls.page.length, 1);
    assert.ok(calls.page[0].note.includes('скопирована'));
    assert.strictEqual(calls.page[0].isError, false);
  }
  // 2) copy без команды — молча мимо
  {
    const { onUpdateChoice, calls } = choiceEnv();
    await onUpdateChoice('copy');
    assert.deepStrictEqual(calls.copy, []);
    assert.deepStrictEqual(calls.page, []);
  }
  // 3) restart — перезапуск, вопрос не разрешается
  {
    const { onUpdateChoice, calls } = choiceEnv();
    await onUpdateChoice('restart');
    assert.strictEqual(calls.restart, 1);
    assert.strictEqual(calls.resolved, undefined);
  }
  // 4) 'later' — вопрос разрешается, решатель сбрасывается
  {
    const { onUpdateChoice, calls, updateState } = choiceEnv();
    await onUpdateChoice('later');
    assert.strictEqual(calls.resolved, 'later');
    assert.strictEqual(updateState.resolve, null);
  }
  // 5) 'install', пока идёт процесс — не разрешается
  {
    const { onUpdateChoice, calls, updateState } = choiceEnv({ running: true });
    await onUpdateChoice('install');
    assert.strictEqual(calls.resolved, undefined);
    assert.notStrictEqual(updateState.resolve, null, 'ожидание осталось');
  }
}

/* ------------------------------------------------------------------ */
/* offerUpdateFlow                                                     */
/* ------------------------------------------------------------------ */
function flowEnv(over = {}) {
  const calls = { asks: [], dismiss: [], install: [] };
  const choices = over.choices.slice();
  const d = {
    UPDATE_NOW: over.UPDATE_NOW || null,
    askUpdate: async (note, isError) => { calls.asks.push({ note, isError }); return choices.shift(); },
    state: { stopping: false },
    updateDismiss: (k, v) => calls.dismiss.push([k, v]),
    console: over.console || fakeConsole(),
  };
  const installStep = async (choice) => {
    calls.install.push(choice);
    return over.step(choice);
  };
  const offerUpdateFlow = extract('offerUpdateFlow', d);
  return { offerUpdateFlow, calls, installStep };
}

async function tOfferUpdateFlow() {
  const done = (v) => ({ return: v });
  // 1) «Не спрашивать больше» — запомнили версию, шаг не запускался
  {
    const { offerUpdateFlow, calls, installStep } = flowEnv({
      choices: ['never'],
      step: () => { throw new Error('installStep не должен был вызываться'); },
    });
    const r = await offerUpdateFlow({ kind: 'dsh', version: '2.0.0', laterLog: 'L', installStep });
    assert.strictEqual(r, undefined);
    assert.deepStrictEqual(calls.dismiss, [['dsh', '2.0.0']]);
  }
  // 2) «Не сейчас» — лог, без шага
  {
    const c = fakeConsole();
    const calls = { asks: [], dismiss: [] };
    const d = {
      UPDATE_NOW: null,
      askUpdate: async (note) => { calls.asks.push(note); return 'later'; },
      state: { stopping: false },
      updateDismiss: (k, v) => calls.dismiss.push([k, v]),
      console: c,
    };
    const offerUpdateFlow = extract('offerUpdateFlow', d);
    await offerUpdateFlow({ kind: 'launcher', version: '1.2.3', laterLog: 'LATER_LOG', installStep: async () => assert.fail('не должен был') });
    assert.deepStrictEqual(calls.dismiss, []);
    assert.ok(c.logs.includes('LATER_LOG'));
  }
  // 3) { note } от шага — вопрос задаётся заново с заметкой, затем done
  {
    const { offerUpdateFlow, calls, installStep } = flowEnv({
      choices: ['recheck', 'install'],
      step: (c) => (c === 'recheck' ? { note: 'нота', noteIsError: true } : done(true)),
    });
    const r = await offerUpdateFlow({ kind: 'launcher', version: '1.2.3', laterLog: 'L', installStep });
    assert.strictEqual(r, true, 'done — результат функции');
    assert.deepStrictEqual(calls.install, ['recheck', 'install']);
    assert.deepStrictEqual(calls.asks.map((a) => a.note), [null, 'нота'], 'второй вопрос — с заметкой');
    assert.strictEqual(calls.asks[1].isError, true);
  }
  // 4) идёт остановка — выход без шага
  {
    const calls = { asks: [] };
    const d = {
      UPDATE_NOW: null,
      askUpdate: async () => { calls.asks.push(1); return 'install'; },
      state: { stopping: true },
      updateDismiss: () => {},
      console: fakeConsole(),
    };
    const offerUpdateFlow = extract('offerUpdateFlow', d);
    await offerUpdateFlow({ kind: 'dsh', version: '2.0.0', laterLog: 'L', installStep: async () => assert.fail('не должен был') });
    assert.strictEqual(calls.asks.length, 1);
  }
  // 5) UPDATE_NOW — первый раз без вопроса (auto='install')
  {
    const calls = { asks: [] };
    const d = {
      UPDATE_NOW: 'dsh',
      askUpdate: async () => { calls.asks.push(1); return 'install'; },
      state: { stopping: false },
      updateDismiss: () => {},
      console: fakeConsole(),
    };
    const offerUpdateFlow = extract('offerUpdateFlow', d);
    await offerUpdateFlow({ kind: 'dsh', version: '2.0.0', laterLog: 'L', installStep: async () => done('X') });
    assert.deepStrictEqual(calls.asks, [], 'вопрос не задавался');
  }
}

/* ------------------------------------------------------------------ */
/* downloadDeb                                                         */
/* ------------------------------------------------------------------ */
function fakeHttp(content, { status = 200, contentLength } = {}) {
  const r = Readable.from([Buffer.from(content)]);
  r.statusCode = status;
  r.headers = contentLength !== undefined ? { 'content-length': String(contentLength) } : {};
  // resume() не переопределять: Readable стартует в flowing-режим именно через него
  return r;
}
function dlEnv(over = {}) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-update-'));
  const updateState = { log: [] };
  const d = {
    updateState,
    console: fakeConsole(),
    showUpdatePage: () => {},
    path, fs,
    app: { getPath: () => tmp },
    httpGet: async () => over.http(),
    pages: { liveRedraw: () => () => {} },
    LOG_MAX_LINES: 500,
    MB: 1024 * 1024,
    DOWNLOAD_PROGRESS_MS: 10,
    crypto,
  };
  const downloadDeb = extract('downloadDeb', d);
  return { downloadDeb, tmp, updateState };
}

async function tDownloadDeb() {
  const URL_ = 'http://127.0.0.1:9/repo/dsh-launcher_1.2.3_amd64.deb';
  const content = Buffer.alloc(2048, 7);
  const sha = crypto.createHash('sha256').update(content).digest('hex');
  // 1) успех: файл на диске, sha совпадает, debPath в state
  {
    const { downloadDeb, tmp, updateState } = dlEnv({ http: () => fakeHttp(content, { contentLength: content.length }) });
    const r = await downloadDeb(URL_, sha);
    assert.strictEqual(r.ok, true);
    assert.strictEqual(r.sha256, sha);
    assert.strictEqual(r.path, path.join(tmp, 'updates', 'dsh-launcher_1.2.3_amd64.deb'));
    assert.ok(fs.readFileSync(r.path).equals(content), 'файл = скачанные байты');
    assert.strictEqual(updateState.debPath, r.path);
    assert.strictEqual(updateState.running, false);
    fs.rmSync(tmp, { recursive: true, force: true });
  }
  // 2) sha256 не совпадает — файл удалён, ok:false
  {
    const { downloadDeb, tmp, updateState } = dlEnv({ http: () => fakeHttp(content, { contentLength: content.length }) });
    const r = await downloadDeb(URL_, 'f'.repeat(32));
    assert.strictEqual(r.ok, false);
    assert.ok(r.err.includes('sha256'));
    assert.strictEqual(updateState.debPath, undefined);
    assert.ok(!fs.existsSync(path.join(tmp, 'updates', 'dsh-launcher_1.2.3_amd64.deb')), 'файл не остался');
    fs.rmSync(tmp, { recursive: true, force: true });
  }
  // 3) HTTP 404 — ok:false с кодом
  {
    const { downloadDeb, tmp } = dlEnv({ http: () => fakeHttp('nope', { status: 404 }) });
    const r = await downloadDeb(URL_, null);
    assert.strictEqual(r.ok, false);
    assert.ok(r.err.includes('404'));
    fs.rmSync(tmp, { recursive: true, force: true });
  }
  // 4) без content-length — прогресс без процентов, успех
  {
    const { downloadDeb, tmp, updateState } = dlEnv({ http: () => fakeHttp(content) });
    const r = await downloadDeb(URL_, null);
    assert.strictEqual(r.ok, true);
    assert.ok(!updateState.log.some((l) => l.includes('%')), 'процентов нет');
    fs.rmSync(tmp, { recursive: true, force: true });
  }
  // 5) старые .deb из прошлых попыток убраны до скачивания
  {
    const { downloadDeb, tmp } = dlEnv({ http: () => fakeHttp(content, { contentLength: content.length }) });
    const dir = path.join(tmp, 'updates');
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'old_0.1.deb'), 'x');
    const r = await downloadDeb(URL_, sha);
    assert.strictEqual(r.ok, true);
    assert.deepStrictEqual(fs.readdirSync(dir), ['dsh-launcher_1.2.3_amd64.deb'], 'старый .deb убран');
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

/* ------------------------------------------------------------------ */
/* offerDshUpdate                                                      */
/* ------------------------------------------------------------------ */
const PKG = '@deepseek-ai/dsh';
function dshEnv(over = {}) {
  const calls = { begin: null, run: [], flow: [] };
  const state = {
    updateChecks: { dsh: over.latest === undefined ? { version: '2.0.0', htmlUrl: 'https://npmjs', releases: [] } : over.latest },
    dshBin: '/usr/bin/dsh',
    dshInstalledVersion: over.installed === undefined ? '1.0.0' : over.installed,
    stopping: false,
  };
  const npmInfoBase = { hasNpm: true, canAuto: true, rootMode: 'auto', nodeOld: false, nodeVersion: '22.0.0', npm: '/x/npm', pkexec: '/usr/bin/pkexec', prefix: '/usr' };
  const npmInfo = Object.assign({}, npmInfoBase, over.npmInfo);
  const d = {
    state,
    probeDsh: async () => over.probe || { ok: true, version: over.probeVersion || '2.0.0' },
    compareSemVer,
    UPDATE_NOW: over.UPDATE_NOW || null,
    updateDismissed: () => Boolean(over.dismissed),
    console: fakeConsole(),
    beginUpdate: (f) => { calls.begin = f; },
    DSH_NPM_PKG: PKG,
    notesBetween,
    dshNotesPart,
    offerUpdateFlow: async (opts) => { calls.flow.push(opts); return opts.installStep(over.firstChoice || 'install'); },
    tr,
    npmInstallInfo: async () => npmInfo,
    runUpdateStep: async (cmd, args, env) => { calls.run.push({ cmd, args, env }); return over.code === undefined ? 0 : over.code; },
    npmRunPlan: extract('npmRunPlan', {
      pkexecNpmArgs: (npm, args) => ['pkexec', npm, ...args],
      envWithBinDir: (npm) => ({ PATH: `BIN:${npm}` }),
    }),
    MIN_NODE_MAJOR: 22,
    manual: (cmd, note) => ({ note, noteIsError: true, manualCmd: cmd || null }),
    done: (v) => ({ return: v }),
  };
  const offerDshUpdate = extract('offerDshUpdate', d);
  return { offerDshUpdate, calls, state };
}

async function tOfferDshUpdate() {
  const CMD = `npm install -g ${PKG}@2.0.0`;
  // 1) свежей версии нет (null / та же) — не предлагаем
  for (const over of [{ latest: null }, { latest: { version: '1.0.0' } }]) {
    const { offerDshUpdate, calls } = dshEnv(over);
    const r = await offerDshUpdate();
    assert.strictEqual(r, undefined);
    assert.strictEqual(calls.begin, null, 'страница не показана');
  }
  // 2) отклонено раньше («не спрашивать больше») — не предлагаем
  {
    const { offerDshUpdate, calls } = dshEnv({ dismissed: true });
    await offerDshUpdate();
    assert.strictEqual(calls.begin, null);
  }
  // 3) canAuto: свой npm, env с bin-dir; успех + проба версии → done
  {
    const { offerDshUpdate, calls, state } = dshEnv({});
    await offerDshUpdate();
    assert.strictEqual(calls.begin.kind, 'dsh');
    assert.strictEqual(calls.begin.version, '2.0.0');
    assert.strictEqual(calls.begin.installed, '1.0.0');
    assert.strictEqual(calls.run.length, 1);
    assert.strictEqual(calls.run[0].cmd, '/x/npm');
    assert.deepStrictEqual(calls.run[0].args, ['install', '-g', `${PKG}@2.0.0`]);
    assert.deepStrictEqual(calls.run[0].env, { PATH: 'BIN:/x/npm' });
    assert.strictEqual(state.dshInstalledVersion, '2.0.0', 'версия обновлена');
  }
  // 4) system prefix + pkexec: cmd=pkexec, args через pkexecNpmArgs
  {
    const { offerDshUpdate, calls } = dshEnv({ npmInfo: { canAuto: false, rootMode: 'pkexec' } });
    await offerDshUpdate();
    assert.strictEqual(calls.run.length, 1);
    assert.strictEqual(calls.run[0].cmd, '/usr/bin/pkexec');
    assert.deepStrictEqual(calls.run[0].args, ['pkexec', '/x/npm', 'install', '-g', `${PKG}@2.0.0`]);
    assert.strictEqual(calls.run[0].env, undefined);
  }
  // 5) prefix требует sudo (не pkexec) — команда в терминал
  {
    const { offerDshUpdate, calls } = dshEnv({ npmInfo: { canAuto: false, rootMode: 'sudo' } });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 0, 'npm не запускался');
    assert.strictEqual(r.manualCmd, `sudo ${CMD}`);
    assert.ok(r.note.includes('sudo'));
  }
  // 6) npm нет — команда без sudo, заметка про Node.js
  {
    const { offerDshUpdate, calls } = dshEnv({ npmInfo: { hasNpm: false } });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 0);
    assert.strictEqual(r.manualCmd, CMD);
    assert.ok(r.note.includes('npm не найден'));
  }
  // 7) Node старый — сначала обновляйте Node
  {
    const { offerDshUpdate, calls } = dshEnv({ npmInfo: { canAuto: false, rootMode: 'sudo', nodeOld: true, nodeVersion: '20.11.0' } });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 0, 'install не запускался');
    assert.ok(r.note.includes('20.11.0'));
    assert.ok(r.note.includes('слишком стар'));
  }
  // 8) recheck: версия уже новая (в терминале поставили) — done
  {
    const { offerDshUpdate, calls, state } = dshEnv({ firstChoice: 'recheck', probeVersion: '2.1.0' });
    await offerDshUpdate();
    assert.strictEqual(calls.run.length, 0);
    assert.strictEqual(state.dshInstalledVersion, '2.1.0');
  }
  // 9) recheck: версия старая — снова подсказка
  {
    const { offerDshUpdate, calls } = dshEnv({ firstChoice: 'recheck', probeVersion: '1.5.0' });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 0);
    assert.strictEqual(r.manualCmd, CMD, 'canAuto — без sudo');
    assert.ok(r.note.includes('1.5.0'));
  }
  // 10) код 127 (отмена пароля) — заметка про отмену
  {
    const { offerDshUpdate, calls } = dshEnv({ code: 127 });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 1, 'install запускался');
    assert.ok(r.note.includes('отменён'));
  }
  // 11) npm «успешен», но dsh — старый: prefix не тот
  {
    const { offerDshUpdate, calls } = dshEnv({ probeVersion: '1.9.0' });
    const r = await offerDshUpdate();
    assert.strictEqual(calls.run.length, 1);
    assert.ok(r.note.includes('npm поставил пакет в'));
    assert.strictEqual(r.manualCmd, CMD);
  }
}

(async () => {
  await tOnUpdateChoice();
  console.log('OK: onUpdateChoice — 5 scenarios');
  await tOfferUpdateFlow();
  console.log('OK: offerUpdateFlow — 5 scenarios');
  await tDownloadDeb();
  console.log('OK: downloadDeb — 5 scenarios');
  await tOfferDshUpdate();
  console.log('OK: offerDshUpdate — 11 scenarios');
  console.log('OK: updates-offer — all checks passed');
})().catch((e) => { console.error('FAIL:', e); process.exit(1); });
