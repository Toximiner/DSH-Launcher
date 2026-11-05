'use strict';
// Language of the launcher's windows: tr(ru, en) — the string in the current language. The
// language is chosen by main.js (environment variable, the RU | EN switch, "Settings", the locale) — setLang.
let lang = 'en';

const tr = (ru, en) => (lang === 'ru' ? ru : en);
const getLang = () => lang;
function setLang(l) { lang = l === 'ru' ? 'ru' : 'en'; }

// Language from the system locale (LC_ALL / LC_MESSAGES / LANG): ru* — Russian,
// otherwise English. Used both at start (main.js) and for "Auto"
// in the settings — a single source of the rule.
const systemLang = () =>
  (/^ru/i.test(process.env.LC_ALL || process.env.LC_MESSAGES || process.env.LANG || '') ? 'ru' : 'en');

module.exports = { tr, getLang, setLang, systemLang };
