'use strict';
// Small pure functions: argument parsing, HTML escaping, routes, errors,
// timeouts, hiding secrets in logs.

function splitArgs(str) {
  const out = [];
  let cur = '';
  let has = false; // was there a token (including an empty '' / "")
  let quote = null;
  for (let i = 0; i < str.length; i++) {
    const c = str[i];
    if (quote) {
      if (c === quote) quote = null;
      else if (c === '\\' && quote === '"' && i + 1 < str.length) cur += str[++i];
      else cur += c;
    } else if (c === "'" || c === '"') {
      quote = c; has = true;
    } else if (c === '\\' && i + 1 < str.length) {
      cur += str[++i]; has = true;
    } else if (/\s/.test(c)) {
      if (has) { out.push(cur); cur = ''; has = false; }
    } else {
      cur += c; has = true;
    }
  }
  if (has) out.push(cur);
  return out;
}

/* The line with the actual error is more useful than the tail of the
   stack trace ("at …", "Node.js vX"): the first line with "error"/"ERR_"
   that doesn't start with "at ", otherwise the last 3 lines; either way
   no longer than 300 characters. */
function firstErrLine(stderr) {
  const lines = String(stderr || '').trim().split('\n').map((l) => l.trim()).filter(Boolean);
  const errLine = lines.find((l) => !l.startsWith('at ') && /error|ERR_/i.test(l));
  return (errLine || lines.slice(-3).join(' ')).slice(0, 300);
}

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
}[c]));

function profileFromArgs(args) {
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--profile' && args[i + 1]) return args[i + 1];
    if (args[i].startsWith('--profile=')) return args[i].slice('--profile='.length);
  }
  return null;
}

function timedCheck(promise, ms) {
  return new Promise((resolve) => {
    const t = setTimeout(() => resolve(null), ms);
    promise.then(
      (v) => { clearTimeout(t); resolve(v); },
      () => { clearTimeout(t); resolve(null); }
    );
  });
}

// The route of a dshlauncher://… request: for a non-standard scheme the
// first segment (retry, install, paste, market) the URL parser puts into
// host, not pathname — we glue it back: dshlauncher://install/run/ ->
// '/install/run/'.
function launcherRoute(url) {
  const u = new URL(url);
  return '/' + u.host + u.pathname;
}

// The route key: dshlauncher://… commands are only accepted from the
// launcher's own pages. Each start — its own random key (env.js, ROUTE_KEY);
// the launcher pages get it in every link (withRouteKey:
// dshlauncher://<key>/…), and the dsh GUI, plugins and external sites in the
// window don't know it — their requests stripRouteKey drops (null).
function withRouteKey(html, key) {
  return String(html).split('dshlauncher://').join(`dshlauncher://${key}/`);
}

function stripRouteKey(route, key) {
  const p = `/${key}/`;
  return validRouteKey(key) && String(route).startsWith(p) ? route.slice(p.length - 1) : null;
}

function validRouteKey(key) {
  return /^[0-9a-f]{32,}$/.test(String(key || ''));
}

function redactLog(text) {
  return String(text || '')
    // eslint-disable-next-line no-control-regex -- ESC: strip the terminal color codes
    .replace(/\x1b\[[0-9;?]*[A-Za-z]/g, '') // ANSI colors
    .replace(/([?&](?:token|access_token|key|api_key)=)[^\s&#"'<>]+/gi, '$1***')
    .replace(/(\bBearer\s+)[A-Za-z0-9._~+/=-]{8,}/g, '$1***')
    .replace(/\b(sk|pk|rk)-[A-Za-z0-9_-]{16,}/g, '$1-***')
    .replace(/\b(gh[pousr]_)[A-Za-z0-9]{20,}/g, '$1***')
    .replace(/((?:api[_-]?key|apikey|secret|password|passwd|access[_-]?token|auth[_-]?token)["']?\s*[:=]\s*["']?)[^\s"'&,;]+/gi, '$1***');
}

// The CSP of the utility data: pages: no external resources (defense in
// depth together with esc() — in case of an injection into the logs), only
// inline scripts and styles (the pages' buttons and styles use them) and
// data: images are allowed.
const CSP_META = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:;">`;

module.exports = { splitArgs, firstErrLine, esc, profileFromArgs, timedCheck, launcherRoute, withRouteKey, stripRouteKey, validRouteKey, redactLog, CSP_META };
