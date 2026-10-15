'use strict';
// Команда с живым логом на служебной странице: установка dsh, плагина маркета,
// шаги обновления. Вывод (stdout и stderr) — построчно в st.log (последние
// LOG_MAX_LINES строк) и в счётчик st.logSeq (суммарно всех строк — для
// живой отрисовки, pages.js), страница перерисовывается сразу и каждые
// REDRAW_MS. st — объект состояния страницы ({ running, log, proc, timer });
// на время работы running = true и proc — процесс, после — сброшены, что бы
// ни случилось. Результат — код выхода; -1, если процесс не запустился или
// убит сигналом.
const { spawn } = require('child_process');

const LOG_MAX_LINES = 500;
const REDRAW_MS = 800;

async function runWithLiveLog(st, cmd, args, { env, cwd, redraw } = {}) {
  st.running = true;
  st.log = [];
  st.logSeq = 0;
  try {
    const p = spawn(cmd, args, { cwd, env: env || process.env, stdio: ['ignore', 'pipe', 'pipe'] });
    st.proc = p;
    const onData = (d) => {
      for (const line of d.toString().split('\n')) {
        const t = line.trim();
        if (!t) continue;
        st.log.push(t);
        if (st.log.length > LOG_MAX_LINES) st.log.shift();
        st.logSeq++;
      }
    };
    p.stdout.on('data', onData);
    p.stderr.on('data', onData);
    if (redraw) {
      redraw();
      st.timer = setInterval(redraw, REDRAW_MS);
    }
    return await new Promise((resolve) => {
      let settled = false;
      const done = (c) => { if (!settled) { settled = true; resolve(c); } };
      p.on('close', (c) => done(c === null ? -1 : c));
      // не запустился (ENOENT …) — close может не прийти
      p.on('error', (e) => { st.log.push(e.message); done(-1); });
    });
  } finally {
    clearInterval(st.timer);
    st.timer = null;
    st.running = false;
    st.proc = null;
  }
}

module.exports = { runWithLiveLog, LOG_MAX_LINES };
