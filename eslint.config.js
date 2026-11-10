'use strict';
// Static checks (npm run lint; part of npm test): unknown names, unused
// variables and imports, unreachable code, etc. — the things that would
// otherwise surface only at runtime (a lost import, a reference to a name
// that no longer exists).
const js = require('@eslint/js');
const globals = require('globals');

module.exports = [
  { ignores: ['node_modules/', 'test/docker/', 'logs/', 'build/'] },
  js.configs.recommended,
  {
    files: ['**/*.js'],
    languageOptions: { ecmaVersion: 'latest', sourceType: 'commonjs', globals: { ...globals.node } },
    rules: {
      'no-unused-vars': ['error', { args: 'none', caughtErrors: 'none', varsIgnorePattern: '^_' }],
      'no-empty': ['error', { allowEmptyCatch: true }],
    },
  },
  {
    files: ['apps/tui-go/web/preview.js'],
    languageOptions: {
      sourceType: 'script',
      globals: { ...globals.browser, Terminal: 'readonly', FitAddon: 'readonly', Go: 'readonly', dshTui: 'readonly' },
    },
  },
];
