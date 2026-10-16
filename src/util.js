'use strict';
// Мелкие чистые функции: разбор аргументов, экранирование HTML, маршруты,
// ошибки, таймауты, скрытие секретов в логах.

function splitArgs(str) {
  const out = [];
  let cur = '';
  let has = false; // был ли токен (в т.ч. пустой '' / "")
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

/* Строка с самой ошибкой полезнее хвоста стектрейса («at …», «Node.js vX»):
   первая строка с «error»/«ERR_», не начинающаяся с «at », иначе — последние
   3 строки; в любом случае не длиннее 300 символов. */
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

// Маршрут запроса dshlauncher://…: у нестандартной схемы первый сегмент
// (retry, install, paste, market) URL-парсер кладёт в host, а не в pathname —
// склеиваем обратно: dshlauncher://install/run/ -> '/install/run/'.
function launcherRoute(url) {
  const u = new URL(url);
  return '/' + u.host + u.pathname;
}

// Ключ маршрутов: команды dshlauncher://… принимаются только со страниц самого
// лаунчера. Каждый запуск — свой случайный ключ (env.js, ROUTE_KEY); страницы
// лаунчера получают его в каждой ссылке (withRouteKey: dshlauncher://<ключ>/…),
// а GUI dsh, плагины и внешние сайты в окне его не знают — их запросы
// stripRouteKey отбрасывает (null).
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
    // eslint-disable-next-line no-control-regex -- ESC: убираем цветовые коды терминала
    .replace(/\x1b\[[0-9;?]*[A-Za-z]/g, '') // ANSI-цвета
    .replace(/([?&](?:token|access_token|key|api_key)=)[^\s&#"'<>]+/gi, '$1***')
    .replace(/(\bBearer\s+)[A-Za-z0-9._~+/=-]{8,}/g, '$1***')
    .replace(/\b(sk|pk|rk)-[A-Za-z0-9_-]{16,}/g, '$1-***')
    .replace(/\b(gh[pousr]_)[A-Za-z0-9]{20,}/g, '$1***')
    .replace(/((?:api[_-]?key|apikey|secret|password|passwd|access[_-]?token|auth[_-]?token)["']?\s*[:=]\s*["']?)[^\s"'&,;]+/gi, '$1***');
}

// CSP служебных data:-страниц: внешних ресурсов нет (защита в глубину
// вместе с esc() — при инъекции в логи), разрешены только inline-
// скрипты и стили (кнопки и стили страниц их используют) и data:-изображения.
const CSP_META = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:;">`;

module.exports = { splitArgs, firstErrLine, esc, profileFromArgs, timedCheck, launcherRoute, withRouteKey, stripRouteKey, validRouteKey, redactLog, CSP_META };
