'use strict';
// Small JSON settings files in userData (update-prefs.json,
// market-prompt.json, ui-lang.json): read with a fallback value and write.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');

const prefFile = (name) => path.join(app.getPath('userData'), name);

// File missing or corrupted — null.
function readPref(name) {
  try { return JSON.parse(fs.readFileSync(prefFile(name), 'utf8')); }
  catch { return null; }
}

// Write error goes to the log, no exception: the setting simply won't be remembered.
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
