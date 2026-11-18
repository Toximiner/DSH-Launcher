'use strict';
const assert = require('node:assert/strict');
const { test } = require('node:test');
test('TUI plugin SDK validates fields and confines action commands', async () => {
  const { encodePanel, parseInput } = await import('../packages/tui-plugin-sdk/index.mjs');
  const fields = [{ id: 'name', label: 'Name', type: 'string', required: true }, { id: 'step', label: 'Step', type: 'integer' },
    { id: 'enabled', label: 'Enabled', type: 'boolean' }, { id: 'mode', label: 'Mode', type: 'enum', choices: ['safe'] }];
  assert.equal(JSON.parse(encodePanel('sample', { title: 'Sample', fields, actions: [{ label: 'Run', command: 'sample-run' }] })).$schema, 'dsh-tui-panel/v1');
  assert.deepEqual(parseInput('{"name":"Hi","step":2,"enabled":true,"mode":"safe"}', fields), { name: 'Hi', step: 2, enabled: true, mode: 'safe' });
  for (const value of ['{}', '[]', '{"name":"Hi","step":1.2}', '{"name":"Hi","mode":"unsafe"}', '{"name":"Hi","extra":true}']) assert.throws(() => parseInput(value, fields));
  assert.throws(() => encodePanel('sample', { title: 'Sample', actions: [{ label: 'Escape', command: 'other-command' }] }));
  assert.throws(() => encodePanel('sample', { title: 'Sample', fields: [{ id: 'password', label: 'Password', type: 'secret' }] }));
});

test('registered panels validate actions before calling plugin handlers', async () => {
  const { registerPanel, encodePanel, parseInput } = await import('../packages/tui-plugin-sdk/index.mjs');
  const commands = new Map();
  const ctx = { inject: (services, setup) => { assert.deepEqual(services, ['commands']); setup({ commands: { register: (definition) => commands.set(definition.name, definition) } }); } };
  let count = 0;
  const fields = [{ id: 'step', label: 'Step', type: 'integer', required: true }];
  registerPanel(ctx, { name: 'sample', description: 'Counter', render: () => ({ title: 'Counter', fields,
    tables: [{ title: 'State', columns: ['Value'], rows: [[String(count)]] }] }), actions: { increment: ({ step }) => { count += step; } } });
  const opening = await commands.get('sample').handler({});
  assert.equal(JSON.parse(opening.text).tables[0].rows[0][0], '0');
  const action = commands.get('sample-increment');
  assert.equal(action.recordInput, false);
  await assert.rejects(action.handler({ rawInput: '{"step":1.5}' }));
  assert.equal(count, 0, 'invalid form data reached the plugin');
  const result = await action.handler({ rawInput: '{"step":3}' });
  assert.equal(JSON.parse(result.text).tables[0].rows[0][0], '3');
  registerPanel(ctx, { name: 'empty', render: () => ({ title: 'Read only' }) });
  assert.equal(JSON.parse((await commands.get('empty').handler({})).text).title, 'Read only');
  assert.throws(() => registerPanel(ctx, { name: '../bad' }));
  assert.throws(() => registerPanel(ctx, { name: 'sample', render: () => ({}), actions: { INVALID: () => {} } }));
  assert.throws(() => encodePanel('sample', { title: 'Table', tables: [{ columns: ['A'], rows: [[]] }] }));
  assert.throws(() => encodePanel('sample', { title: 'Override', owner: 'other' }));
  assert.deepEqual(parseInput('{}', []), {});
});
