'use strict';
// Язык окон лаунчера: tr(ru, en) — строка на текущем языке. Язык выбирает
// main.js (переменная, переключатель RU | EN, «Настройки», локаль) — setLang.
let lang = 'en';

const tr = (ru, en) => (lang === 'ru' ? ru : en);
const getLang = () => lang;
function setLang(l) { lang = l === 'ru' ? 'ru' : 'en'; }

// Язык по локали системы (LC_ALL / LC_MESSAGES / LANG): ru* — русский,
// иначе английский. Используется и при старте (main.js), и при «Авто»
// в настройках — один источник правила.
const systemLang = () =>
  (/^ru/i.test(process.env.LC_ALL || process.env.LC_MESSAGES || process.env.LANG || '') ? 'ru' : 'en');

module.exports = { tr, getLang, setLang, systemLang };
