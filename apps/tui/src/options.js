'use strict';
const path = require('node:path');

function options(argv, env = process.env) {
  const result = { url: env.DSH_TUI_URL || '', bin: env.DSH_BIN || 'dsh',
    cwd: path.resolve(env.DSH_CWD || process.cwd()), profile: 'web', port: 3080, explicitBin: !!env.DSH_BIN };
  const keys = { '--url': 'url', '--dsh': 'bin', '--cwd': 'cwd', '--profile': 'profile', '--port': 'port', '--session': 'session' };
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--help' || argv[i] === '-h') { result.help = true; continue; }
    const key = keys[argv[i]];
    if (!key || !argv[i + 1] || argv[i + 1].startsWith('--')) throw new Error(`Invalid option: ${argv[i]}`);
    result[key] = argv[++i];
    if (key === 'bin') result.explicitBin = true;
  }
  result.port = Number(result.port);
  if (!Number.isInteger(result.port) || result.port < 1024 || result.port > 65535) throw new Error('Port must be between 1024 and 65535');
  if (!/^[\w.-]+$/.test(result.profile) || /^\.+$/.test(result.profile)) throw new Error('Invalid profile');
  result.cwd = path.resolve(result.cwd);
  if (result.url) {
    const url = new URL(result.url);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw new Error('Expected an HTTP(S) URL without embedded credentials');
    if (!['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)) throw new Error('Use a loopback URL; for remote hosts use an SSH tunnel');
    if (url.pathname !== '/') throw new Error('The dsh URL must use the root path /');
  }
  return result;
}

module.exports = { options };
