'use strict';
// Protection of the windows against foreign pages: the dsh GUI shows the model's
// text and plugin interfaces, and any site could get into the window via a link.
//  - navigation: inside the window — only the dsh GUI (and the page opened from "Insert
//    link"), the launcher's routes; other http(s) — in the system browser,
//    the rest (file:, javascript: …) — no;
//  - browser permissions: only what the dsh GUI uses (writing to the clipboard)
//    and fullscreen mode; the rest (microphone, camera, geolocation,
//    notifications …) — denied, with a log entry (once per permission).
// The dshlauncher://… routes are additionally protected by the launch key (util.js).
const { shell, session } = require('electron');
const { PLAIN_URL } = require('./env');
const { redactLog } = require('./util');

/* ===== navAction (pure function, test test/parsing.js) =====
   Navigation inside the window to url from the page currentUrl: 'allow' — let through;
   'external' — open in the system browser; 'deny' — nowhere. guiOrigin —
   the origin of the dsh GUI (null — a launcher window without the GUI: dialog, toast). */
function navAction(url, currentUrl, guiOrigin) {
  let u;
  try { u = new URL(url); } catch { return 'deny'; }
  if (u.protocol === 'dshlauncher:') return 'allow';
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return 'deny';
  let current = null;
  try { current = new URL(currentUrl).origin; } catch { /* data: etc. */ }
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

// Navigation of the main frame; gui — a window with the dsh GUI (the main one), otherwise a launcher window.
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

// A layer window (dialog, toast, search panel): new tabs are forbidden,
// http(s) — in the system browser. One policy for all such layers.
function denyExternalOpen(wc) {
  wc.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url)) shell.openExternal(url).catch(() => {});
    return { action: 'deny' };
  });
}

module.exports = { navAction, restrictPermissions, guardNavigation, ALLOWED_PERMISSIONS, denyExternalOpen };
