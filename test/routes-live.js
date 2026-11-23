'use strict';
/*
 * test/routes-live.js — dshlauncher://-обработчики из src/routes.js (extract
 * по маркерам, _extract.js): openRunningGui (куки → ссылка входа → paste),
 * onPasteRequest, onRetryRequest, onInstallRun (npm / pkexec / отказ).
 * Run: node test/routes-live.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const PLAIN_URL = 'http://127.0.0.1:3080/';
const PORT = 3080;
const tr = (ru) => ru; // русский — язык по умолчанию в тестах
const esc = (x) => String(x);
const fakeConsole = () => {
  const logs = [];
  return { logs, log: (...a) => logs.push(a.join(' ')), error: (...a) => logs.push(a.join(' ')), info: (...a) => logs.push(a.join(' ')), warn: (...a) => logs.push(a.join(' ')) };
};
const blank = { status: 200, headers: { get: () => 'text/html' } };

/* ------------------------------------------------------------------ */
/* openRunningGui                                                      */
/* ------------------------------------------------------------------ */
function guiEnv(over = {}) {
  const calls = { authed: 0, listener: 0, load: [], paste: 0, sleep: 0 };
  let now = 0;
  const state = { stopping: false };
  const d = {
    plainUrlIsAuthed: async () => { calls.authed++; return over.authedSeq ? over.authedSeq() : Boolean(over.authed); },
    Date: { now: () => now },
    listenerAuthUrl: () => { calls.listener++; return over.listenerUrl || null; },
    sleep: async () => { calls.sleep++; now += 500; },
    state,
    loadInWin: async (u) => { calls.load.push(u); },
    showPastePage: () => { calls.paste++; },
    console: fakeConsole(),
    PLAIN_URL,
    PORT,
  };
  const openRunningGui = extract('openRunningGui', d);
  return { openRunningGui, calls, state };
}

async function tOpenRunningGui() {
  // 1) куки валидны сразу — GUI, без поллинга
  {
    const { openRunningGui, calls } = guiEnv({ authed: true });
    await openRunningGui('attach');
    assert.deepStrictEqual(calls.load, [PLAIN_URL], 'открыт PLAIN_URL');
    assert.strictEqual(calls.listener, 0, 'к выводу процесса не ходили');
    assert.strictEqual(calls.paste, 0);
  }
  // 2) куки не валидны, есть ссылка входа из вывода — она
  {
    const { openRunningGui, calls } = guiEnv({ listenerUrl: PLAIN_URL + '?token=abc' });
    await openRunningGui('retry');
    assert.deepStrictEqual(calls.load, [PLAIN_URL + '?token=abc']);
    assert.strictEqual(calls.paste, 0);
  }
  // 3) куки появились после первого сна (dsh докрутил логин)
  {
    let n = 0;
    const { openRunningGui, calls } = guiEnv({ authedSeq: () => (n++ > 0) });
    await openRunningGui('attach');
    assert.deepStrictEqual(calls.load, [PLAIN_URL]);
    assert.strictEqual(calls.sleep, 1, 'один сон — и куки');
    assert.strictEqual(calls.paste, 0);
  }
  // 4) ничего не появилось до дедлайна (5000 мс) — страница вставки
  {
    const { openRunningGui, calls } = guiEnv({});
    await openRunningGui('attach');
    assert.deepStrictEqual(calls.load, []);
    assert.strictEqual(calls.paste, 1);
    assert.ok(calls.sleep >= 9, `поллинг до дедлайна (sleeps=${calls.sleep})`);
  }
  // 5) идёт остановка — не ждём, сразу paste
  {
    const { openRunningGui, calls, state } = guiEnv({});
    state.stopping = true;
    await openRunningGui('attach');
    assert.strictEqual(calls.sleep, 0, 'сон не засчитан');
    assert.strictEqual(calls.paste, 1);
  }
}

/* ------------------------------------------------------------------ */
/* onPasteRequest                                                      */
/* ------------------------------------------------------------------ */
function pasteEnv(over = {}) {
  const loadCalls = [];
  const win = over.destroyed
    ? { isDestroyed: () => true }
    : { isDestroyed: () => false, loadURL: (u) => { loadCalls.push(u); return Promise.resolve(); } };
  const c = fakeConsole();
  const d = { state: { win }, console: c, blankResponse: () => blank };
  const onPasteRequest = extract('onPasteRequest', d);
  return { onPasteRequest, loadCalls, c };
}

async function tOnPasteRequest() {
  // 1) валидный URL — loadURL
  {
    const { onPasteRequest, loadCalls } = pasteEnv();
    const r = await onPasteRequest('/paste/' + encodeURIComponent('https://example.com/gui?token=x'));
    assert.strictEqual(r.status, 200);
    assert.deepStrictEqual(loadCalls, ['https://example.com/gui?token=x']);
  }
  // 2) не URL — молча мимо
  {
    const { onPasteRequest, loadCalls } = pasteEnv();
    await onPasteRequest('/paste/hello%20world');
    assert.deepStrictEqual(loadCalls, []);
  }
  // 3) окно уничтожено — не грузим
  {
    const { onPasteRequest, loadCalls } = pasteEnv({ destroyed: true });
    await onPasteRequest('/paste/' + encodeURIComponent('https://example.com/'));
    assert.deepStrictEqual(loadCalls, []);
  }
  // 4) кривое percent-encoding — в catch, ответ всё равно blank
  {
    const { onPasteRequest, loadCalls, c } = pasteEnv();
    const r = await onPasteRequest('/paste/%zz');
    assert.strictEqual(r.status, 200);
    assert.deepStrictEqual(loadCalls, []);
    assert.ok(c.logs.some((l) => l.includes('paste handler')), 'ошибка залогирована');
  }
}

/* ------------------------------------------------------------------ */
/* onRetryRequest                                                      */
/* ------------------------------------------------------------------ */
function retryEnv(over = {}) {
  const calls = { openGui: [], installPage: [], page: [], status: [] };
  const win = over.destroyed ? { isDestroyed: () => true } : { isDestroyed: () => false };
  const state = { win, stopping: false, dshBin: '/usr/bin/dsh', weStartedDsh: true, dshProc: { pid: 5 }, dshFatal: true, portState: 'down' };
  const c = fakeConsole();
  const d = {
    state,
    portOpen: async () => Boolean(over.portUp),
    openRunningGui: async (why) => { calls.openGui.push(why); },
    dshInstallCheck: async () => over.check || { ok: true },
    showInstallPage: (noteFn) => { calls.installPage.push(noteFn()); },
    showPage: (renderFn) => { calls.page.push(renderFn()); },
    showStatus: (title, html) => { calls.status.push({ title, html }); return `<h1>${title}</h1>${html}`; },
    tr, esc, PORT,
    DSH_ARGS: ['--profile', 'web', '--no-open'],
    retryButton: () => 'RETRY_BTN',
    console: c,
    blankResponse: () => blank,
  };
  const onRetryRequest = extract('onRetryRequest', d);
  return { onRetryRequest, calls, state };
}

async function tOnRetryRequest() {
  // 1) порт открыт — чужой dsh: сброс флага «наш процесс» и в GUI
  {
    const { onRetryRequest, calls, state } = retryEnv({ portUp: true });
    await onRetryRequest();
    assert.deepStrictEqual(calls.openGui, ['retry: dsh запущен']);
    assert.strictEqual(state.weStartedDsh, false);
    assert.strictEqual(state.dshProc, null);
    assert.strictEqual(state.dshFatal, false);
    assert.strictEqual(state.portState, 'up');
    assert.deepStrictEqual(calls.page, []);
  }
  // 2) порт закрыт, dsh установлен — страница «запустите в терминале»
  {
    const { onRetryRequest, calls } = retryEnv({ portUp: false, check: { ok: true } });
    await onRetryRequest();
    assert.deepStrictEqual(calls.openGui, []);
    assert.deepStrictEqual(calls.installPage, []);
    assert.strictEqual(calls.page.length, 1);
    const st = calls.status[0];
    assert.ok(st.title.includes('dsh не запущен'));
    assert.ok(st.html.includes('/usr/bin/dsh --profile web --no-open'), 'команда в тексте');
    assert.ok(calls.page[0].includes('RETRY_BTN'), 'кнопка «Проверить снова»');
  }
  // 3) порт закрыт, dsh пропал — страница установки
  {
    const { onRetryRequest, calls } = retryEnv({ portUp: false, check: { ok: false, reason: 'missing' } });
    await onRetryRequest();
    assert.strictEqual(calls.installPage.length, 1);
    assert.ok(calls.installPage[0].includes('не найден'));
    assert.deepStrictEqual(calls.page, []);
  }
  // 4) окно уничтожено / идёт остановка — ничего не делаем
  for (const over of [{ destroyed: true }, { stopping: true }]) {
    const { onRetryRequest, calls, state } = retryEnv(over);
    if (over.stopping) state.stopping = true;
    await onRetryRequest();
    assert.deepStrictEqual(calls.openGui, []);
    assert.deepStrictEqual(calls.page, []);
    assert.deepStrictEqual(calls.installPage, []);
  }
}

/* ------------------------------------------------------------------ */
/* onInstallRun                                                        */
/* ------------------------------------------------------------------ */
function installEnv(over = {}) {
  const calls = { refresh: 0, pages: [], run: [], launch: 0 };
  const win = over.destroyed ? { isDestroyed: () => true } : { isDestroyed: () => false };
  const state = { win, stopping: false };
  const npmInfo = Object.assign({
    canAuto: true,
    rootMode: 'auto',
    command: 'npm install -g @deepseek-ai/dsh@latest',
    npm: '/home/u/.nvm/versions/node/v22.0.0/bin/npm',
    pkexec: '/usr/bin/pkexec',
  }, over.npmInfo);
  const installState = { running: false, npmInfo };
  const c = fakeConsole();
  const d = {
    state,
    installState,
    refreshInstallState: async () => { calls.refresh++; if (over.refreshThrows) throw new Error('boom'); },
    showInstallPage: (noteFn, isError) => { calls.pages.push({ note: noteFn ? noteFn() : '(no note)', isError: Boolean(isError) }); },
    DSH_NPM_PKG: '@deepseek-ai/dsh',
    runWithLiveLog: async (st, cmd, args, opts) => { calls.run.push({ cmd, args, env: opts.env }); return over.code === undefined ? 0 : over.code; },
    npmRunPlan: extract('npmRunPlan', {
      pkexecNpmArgs: (npm, args) => ['pkexec', npm, ...args],
      envWithBinDir: (npm) => ({ PATH: `BIN:${npm}` }),
    }),
    liveRedraw: () => ({}),
    dshInstallCheck: async () => over.check || { ok: true },
    launchDsh: async () => { calls.launch++; },
    tr, console: c,
    blankResponse: () => blank,
  };
  const onInstallRun = extract('onInstallRun', d);
  return { onInstallRun, calls, installState };
}

async function tOnInstallRun() {
  const ARGS = ['install', '-g', '@deepseek-ai/dsh@latest'];
  // 1) установка уже идёт — сразу выход
  {
    const { onInstallRun, calls, installState } = installEnv();
    installState.running = true;
    await onInstallRun();
    assert.strictEqual(calls.refresh, 0);
    assert.strictEqual(calls.run.length, 0);
  }
  // 2) окна нет — выход
  {
    const { onInstallRun, calls } = installEnv({ destroyed: true });
    await onInstallRun();
    assert.strictEqual(calls.run.length, 0);
  }
  // 3) prefix требует sudo, но не pkexec — отказ с подсказкой
  {
    const { onInstallRun, calls } = installEnv({ npmInfo: { canAuto: false, rootMode: 'sudo' } });
    await onInstallRun();
    assert.strictEqual(calls.run.length, 0, 'npm не запускался');
    assert.strictEqual(calls.pages.length, 1);
    assert.ok(calls.pages[0].note.includes('недоступна'));
    assert.ok(calls.pages[0].isError);
  }
  // 4) pkexec: cmd=pkexec-обёртка, args через pkexecNpmArgs, env не тронут;
  //    успех + dsh запускается → launchDsh
  {
    const { onInstallRun, calls, installState } = installEnv({ npmInfo: { canAuto: false, rootMode: 'pkexec' } });
    await onInstallRun();
    assert.strictEqual(installState.running, true, 'страница «идёт» уже до первой строки лога');
    assert.strictEqual(calls.run.length, 1);
    assert.strictEqual(calls.run[0].cmd, '/usr/bin/pkexec');
    assert.deepStrictEqual(calls.run[0].args, ['pkexec', '/home/u/.nvm/versions/node/v22.0.0/bin/npm', ...ARGS]);
    assert.strictEqual(calls.run[0].env, undefined, 'pkexec — без нашego env');
    assert.strictEqual(calls.launch, 1, 'установлен → запускаем');
  }
  // 5) canAuto: свой npm, env с bin-dir в начале PATH
  {
    const { onInstallRun, calls } = installEnv();
    await onInstallRun();
    assert.strictEqual(calls.run.length, 1);
    assert.strictEqual(calls.run[0].cmd, '/home/u/.nvm/versions/node/v22.0.0/bin/npm');
    assert.deepStrictEqual(calls.run[0].args, ARGS);
    assert.deepStrictEqual(calls.run[0].env, { PATH: 'BIN:/home/u/.nvm/versions/node/v22.0.0/bin/npm' });
    assert.strictEqual(calls.launch, 1);
  }
  // 6) npm «успешен», но dsh не запускается — не запускаем
  {
    const { onInstallRun, calls } = installEnv({ check: { ok: false, reason: 'broken', detail: 'segfault' } });
    await onInstallRun();
    assert.strictEqual(calls.launch, 0);
    // первая страница — «идёт установка» (без заметки), вторая — финальная
    assert.strictEqual(calls.pages.length, 2);
    assert.strictEqual(calls.pages[0].note, '(no note)');
    assert.ok(calls.pages[1].note.includes('так и не запускается'));
    assert.ok(calls.pages[1].isError);
  }
  // 7) npm завершился с кодом 3 — ошибка с кодом
  {
    const { onInstallRun, calls } = installEnv({ code: 3 });
    await onInstallRun();
    assert.strictEqual(calls.launch, 0);
    assert.strictEqual(calls.pages.length, 2); // «идёт» + финальная ошибка
    assert.ok(calls.pages[1].note.includes('кодом 3'));
    assert.ok(calls.pages[1].isError);
  }
  // 8) проверка упала (исключение) — catch, страница ошибки
  {
    const { onInstallRun, calls } = installEnv({ refreshThrows: true });
    await onInstallRun();
    assert.strictEqual(calls.run.length, 0);
    assert.strictEqual(calls.pages.length, 1);
    assert.ok(calls.pages[0].note.includes('Не удалось запустить установку'));
    assert.ok(calls.pages[0].isError);
  }
}

(async () => {
  await tOpenRunningGui();
  console.log('OK: openRunningGui — 5 scenarios');
  await tOnPasteRequest();
  console.log('OK: onPasteRequest — 4 scenarios');
  await tOnRetryRequest();
  console.log('OK: onRetryRequest — 6 scenarios');
  await tOnInstallRun();
  console.log('OK: onInstallRun — 8 scenarios');
  console.log('OK: routes-live — all checks passed');
})().catch((e) => { console.error('FAIL:', e); process.exit(1); });
