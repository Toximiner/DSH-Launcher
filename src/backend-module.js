'use strict';
// Launcher-owned backend assets. The profile and its user patch remain untouched.
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const { pathToFileURL } = require('url');

const ASSETS = ['index.mjs', 'session-delete.mjs'];

// dsh consumes launcher options only before the app's first argument. Preserve
// existing --patch order while placing our final overlay before app flags/--.
function backendModuleArgs(args, overlay) {
  const values = new Set(['--profile', '--patch', '--from-default-profile']);
  const flags = new Set(['--dump-config', '--dump-config-schema', '--dump-default-config']);
  let index = 0;
  while (index < args.length) {
    const argument = args[index];
    if (values.has(argument) && index + 1 < args.length) index += 2;
    else if ([...values].some((name) => argument.startsWith(name + '=')) || flags.has(argument)) index++;
    else break;
  }
  return [...args.slice(0, index), '--patch', overlay, ...args.slice(index)];
}

function prepareBackendModule({ assetDir = path.join(__dirname, '..', 'packages', 'launcher-backend'), dataDir = process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local', 'share') } = {}) {
  const files = ASSETS.map((name) => {
    const source = path.join(assetDir, name);
    try {
      if (!fs.statSync(source).isFile()) throw new Error('not a file');
      return { name, content: fs.readFileSync(source) };
    } catch (cause) {
      throw new Error(`Bundled backend asset is missing or unreadable: ${source}. Reinstall DSH Launcher.`, { cause });
    }
  });
  const hash = crypto.createHash('sha256');
  for (const { name, content } of files) hash.update(name + '\0').update(content).update('\0');
  const base = path.resolve(dataDir, 'dsh-launcher', 'backend-module');
  const directory = path.join(base, hash.digest('hex'));
  const overlay = path.join(directory, 'launcher.patch.json');
  const patch = JSON.stringify([{ insert: [{ id: 'dsh-launcher-session-management', name: pathToFileURL(path.join(directory, 'index.mjs')).href }] }]) + '\n';
  const expected = [...files, { name: 'launcher.patch.json', content: Buffer.from(patch) }];
  fs.mkdirSync(base, { recursive: true, mode: 0o700 });
  if (!fs.existsSync(directory)) {
    const staging = fs.mkdtempSync(path.join(base, '.staging-'));
    try {
      for (const { name, content } of expected) fs.writeFileSync(path.join(staging, name), content, { mode: 0o600 });
      try { fs.renameSync(staging, directory); }
      catch (error) {
        // Another GUI/TUI may have prepared the same immutable version.
        if (!['EEXIST', 'ENOTEMPTY'].includes(error.code)) throw error;
      }
    } finally { fs.rmSync(staging, { recursive: true, force: true }); }
  }
  if (fs.lstatSync(directory).isSymbolicLink()) throw new Error('Bundled backend cache is a symbolic link; remove it and restart DSH Launcher.');
  for (const { name, content } of expected) {
    const target = path.join(directory, name);
    if (fs.lstatSync(target).isSymbolicLink() || !fs.readFileSync(target).equals(content)) {
      throw new Error(`Bundled backend cache changed: ${target}. Remove this cache directory and restart DSH Launcher.`);
    }
  }
  return overlay;
}

module.exports = { backendModuleArgs, prepareBackendModule };
