/** Declarative terminal panels carried by dsh's existing human-command RPC. */
import { Buffer } from 'node:buffer';
const namePattern = /^[a-z][a-z0-9-]{0,63}$/;
const fieldPattern = /^[a-zA-Z][a-zA-Z0-9_]{0,63}$/;
export const SCHEMA = 'dsh-tui-panel/v1';

export function encodePanel(owner, panel) {
  if (!namePattern.test(owner) || !panel?.title || !Array.isArray(panel.fields || []) || (panel.fields || []).length > 20 || (panel.actions || []).length > 8) throw Error('Invalid panel');
  if (Object.keys(panel).some((key) => !['title', 'fields', 'actions', 'tables', 'status'].includes(key))) throw Error('Unknown panel property');
  const ids = new Set();
  for (const field of panel.fields || []) {
    if (!fieldPattern.test(field.id) || ids.has(field.id) || !field.label || !['string', 'integer', 'boolean', 'enum'].includes(field.type)) throw Error('Invalid field');
    if (field.type === 'enum' && (!field.choices?.length || field.choices.length > 30)) throw Error('Invalid choices');
    ids.add(field.id);
  }
  for (const action of panel.actions || []) {
    if (!action.label || !namePattern.test(action.command) || !action.command.startsWith(owner + '-')) throw Error('Action is outside its command namespace');
  }
  for (const table of panel.tables || []) {
    if (!table.columns?.length || table.columns.length > 10 || table.rows.length > 200 || table.rows.some((row) => row.length !== table.columns.length)) throw Error('Invalid table');
  }
  if ((panel.tables || []).length > 4 || (panel.status || []).length > 20) throw Error('Panel exceeds limits');
  const text = JSON.stringify({ $schema: SCHEMA, owner, ...panel });
  if (Buffer.byteLength(text) > 65536) throw Error('Panel exceeds 64 KiB');
  return text;
}

export function parseInput(rawInput, fields) {
  if (Buffer.byteLength(rawInput) > 16384) throw Error('Input exceeds 16 KiB');
  const input = JSON.parse(rawInput);
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw Error('Expected a field object');
  if (Object.keys(input).some((key) => !fields.some((field) => field.id === key))) throw Error('Unknown field');
  for (const field of fields) {
    const value = input[field.id];
    if (field.required && (value === undefined || (typeof value === 'string' && !value.trim()))) throw Error('Required field: ' + field.id);
    if (value === undefined && !field.required) continue;
    if (field.type === 'string' && (typeof value !== 'string' || Buffer.byteLength(value) > 4096)) throw Error('Expected bounded text: ' + field.id);
    if (field.type === 'integer' && !Number.isSafeInteger(value)) throw Error('Expected safe integer: ' + field.id);
    if (field.type === 'boolean' && typeof value !== 'boolean') throw Error('Expected boolean: ' + field.id);
    if (field.type === 'enum' && !field.choices.includes(value)) throw Error('Expected listed option: ' + field.id);
  }
  return input;
}

/** Bind the same renderer to any plugin; handlers retain all domain validation. */
export function registerPanel(ctx, { name, description, render, actions = {} }) {
  if (!namePattern.test(name)) throw Error('Invalid command name');
  return ctx.inject(['commands'], (scope) => {
    scope.commands.register({ name, description, recordInput: false,
      handler: async (invocation) => ({ kind: 'success', text: encodePanel(name, await render(invocation)) }) });
    for (const [suffix, handler] of Object.entries(actions)) {
      const command = name + '-' + suffix;
      if (!namePattern.test(command)) throw Error('Invalid action command');
      scope.commands.register({ name: command, description, recordInput: false, input: { hint: 'JSON field object' },
        handler: async (invocation) => {
          const before = await render(invocation);
          const values = parseInput(invocation.rawInput, before.fields || []);
          await handler(values, invocation);
          return { kind: 'success', text: encodePanel(name, await render(invocation)) };
        } });
    }
  });
}
