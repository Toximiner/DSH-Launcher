#!/usr/bin/env node
'use strict';
// npm stand-in for installed-package tests; never downloads or executes packages.
const fs = require('node:fs');
const path = require('node:path');
const { spawn } = require('node:child_process');
const state = process.env.TUI_FAKE_STATE;
const prefix = process.argv[process.argv.indexOf('--prefix') + 1];
fs.appendFileSync(path.join(state, 'npm.jsonl'), JSON.stringify(process.argv.slice(2)) + '\n');
const modeFile = path.join(state, 'npm-mode');
const mode = fs.existsSync(modeFile) ? fs.readFileSync(modeFile, 'utf8').trim() : 'ok';
console.log('Test npm installation progress');
if (mode === 'fail') {
  console.error('Test npm installation failed');
  process.exitCode = 1;
} else if (mode === 'hang') {
  console.error('npm http fetch GET 200 https://registry.npmjs.org/test/-/test-1.0.0.tgz 10ms');
  const worker = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { stdio: 'ignore' });
  fs.writeFileSync(path.join(state, 'npm-ready.json'), JSON.stringify({ pid: process.pid, worker: worker.pid }));
  console.log('Test npm is waiting');
  setInterval(() => {}, 1000);
} else {
  const bin = path.join(prefix, 'node_modules', '.bin', 'dsh');
  fs.mkdirSync(path.dirname(bin), { recursive: true });
  fs.symlinkSync(process.env.TUI_FAKE_DSH, bin);
  console.log('Test npm installation complete');
}
