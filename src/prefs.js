'use strict';
// Маленькие JSON-файлы настроек в userData (update-prefs.json,
// market-prompt.json, ui-lang.json): чтение с запасным значением и запись.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');

const prefFile = (name) => path.join(app.getPath('userData'), name);

// Файла нет или он битый — null.
function readPref(name) {
  try { return JSON.parse(fs.readFileSync(prefFile(name), 'utf8')); }
  catch { return null; }
}

// Ошибка записи — в лог, без исключения: настройка просто не запомнится.
function writePref(name, obj) {
  try {
    fs.mkdirSync(path.dirname(prefFile(name)), { recursive: true });
    fs.writeFileSync(prefFile(name), JSON.stringify(obj));
    return true;
  } catch (e) {
    console.error(`[launcher] ${name}:`, e.message);
    return false;
  }
}

module.exports = { prefFile, readPref, writePref };
