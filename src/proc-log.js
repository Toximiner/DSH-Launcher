'use strict';
// Command with a live log on the service page: dsh installation, market
// plugin, update steps. Output (stdout and stderr) goes line by line into
// st.log (the last LOG_MAX_LINES lines) and the st.logSeq counter (total of
// all lines — for live rendering, pages.js); the page is redrawn immediately
// and every REDRAW_MS. st — the page state object ({ running, log, proc,
// timer }); while running, running = true and proc is the process; both are
// reset afterwards, whatever happens. The result is the exit code; -1 if the
// process failed to start or was killed by a signal.
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
      // failed to start (ENOENT …) — close may never arrive
      p.on('error', (e) => { st.log.push(e.message); done(-1); });
    });
  } finally {
    clearInterval(st.timer);
    st.timer = null;
    st.running = false;
    st.proc = null;
  }
}

module.exports = { runWithLiveLog, LOG_MAX_LINES, REDRAW_MS };
