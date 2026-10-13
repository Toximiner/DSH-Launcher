'use strict';
// Защита окон от чужих страниц: GUI dsh показывает текст модели и интерфейсы
// плагинов, и по ссылке в окно мог попасть любой сайт.
//  - переходы: внутри окна — только GUI dsh (и страница, открытая из «Вставить
//    ссылку»), маршруты лаунчера; другие http(s) — в системном браузере,
//    остальное (file:, javascript: …) — нет;
//  - разрешения браузера: только то, чем пользуется GUI dsh (запись в буфер
//    обмена) и полноэкранный режим; остальное (микрофон, камера, геолокация,
//    уведомления …) — отказ, с записью в журнал (один раз на разрешение).
// Маршруты dshlauncher://… дополнительно защищены ключом запуска (util.js).
const { shell, session } = require('electron');
const { PLAIN_URL } = require('./env');
const { redactLog } = require('./util');

/* ===== navAction (чистая функция, тест test/parsing.js) =====
   Переход внутри окна на url со страницы currentUrl: 'allow' — пустить;
   'external' — открыть в системном браузере; 'deny' — никуда. guiOrigin —
   происхождение GUI dsh (null — окно лаунчера без GUI: диалог, плашка). */
function navAction(url, currentUrl, guiOrigin) {
  let u;
  try { u = new URL(url); } catch { return 'deny'; }
  if (u.protocol === 'dshlauncher:') return 'allow';
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return 'deny';
  let current = null;
  try { current = new URL(currentUrl).origin; } catch { /* data: и т. п. */ }
  if (guiOrigin && (u.origin === guiOrigin || (current && current !== 'null' && u.origin === current))) return 'allow';
  return 'external';
}
/* ===== end navAction ===== */

const ALLOWED_PERMISSIONS = new Set(['clipboard-sanitized-write', 'fullscreen']);
const deniedLogged = new Set();

function restrictPermissions() {
  const ses = session.defaultSession;
  ses.setPermissionRequestHandler((wc, permission, callback) => {
    const ok = ALLOWED_PERMISSIONS.has(permission);
    if (!ok && !deniedLogged.has(permission)) {
      deniedLogged.add(permission);
      let from = '';
      try { from = new URL(wc.getURL()).origin; } catch { /* noop */ }
      console.log(`[launcher] разрешение «${permission}» для ${from || 'страницы'} — отказ`);
    }
    callback(ok);
  });
  ses.setPermissionCheckHandler((_wc, permission) => ALLOWED_PERMISSIONS.has(permission));
}

// Переходы основного кадра; gui — окно с GUI dsh (главное), иначе окно лаунчера.
function guardNavigation(wc, { gui = false } = {}) {
  const guiOrigin = gui ? new URL(PLAIN_URL).origin : null;
  wc.on('will-navigate', (e, url) => {
    const action = navAction(url, wc.getURL(), guiOrigin);
    if (action === 'allow') return;
    e.preventDefault();
    if (action === 'external') shell.openExternal(url).catch(() => {});
    console.log(`[launcher] переход ${action === 'external' ? 'открыт в браузере' : 'запрещён'}: ${redactLog(String(url).slice(0, 120))}`);
  });
}

module.exports = { navAction, restrictPermissions, guardNavigation, ALLOWED_PERMISSIONS };
