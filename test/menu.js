#!/usr/bin/env node
// Юнит-тест чистого помощника menuBackTarget (main.js, между маркерами
// «===== menuBackTarget … =====»). main.js целиком не загружается (electron).
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const menuBackTarget = extract('menuBackTarget');
const page = function fakePage() {};

// GUI dsh (тот же origin, что PLAIN_URL) → null = вернуться в GUI
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080/?token=abc', 'http://127.0.0.1:3080', page), null, 'GUI с токеном → null');
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080/chat/xyz', 'http://127.0.0.1:3080', page), null, 'маршрут внутри GUI → null');
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080', 'http://127.0.0.1:3080', page), null, 'корень GUI → null');

// Служебная страница (data:) → её рендер-функция
assert.strictEqual(menuBackTarget('data:text/html;charset=utf-8,hello', 'http://127.0.0.1:3080', page), page, 'data: URL → currentPage');

// about:blank / мусор → currentPage (new URL бросит или origin не совпадёт)
assert.strictEqual(menuBackTarget('about:blank', 'http://127.0.0.1:3080', page), page, 'about:blank → currentPage');
assert.strictEqual(menuBackTarget('not a url', 'http://127.0.0.1:3080', page), page, 'мусор → currentPage');

// Другой порт — это не GUI dsh
assert.strictEqual(menuBackTarget('http://127.0.0.1:3181/?token=x', 'http://127.0.0.1:3080', page), page, 'чужой порт → currentPage');

// currentPage == null (окно ещё на about:blank) → null в любом случае
assert.strictEqual(menuBackTarget('data:text/html,hi', 'http://127.0.0.1:3080', null), null, 'currentPage null → null');

console.log('OK: menuBackTarget — все проверки прошли');
