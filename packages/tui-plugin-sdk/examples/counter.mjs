import { registerPanel } from '../index.mjs';
export const name = 'tui-counter-example';
export function apply(ctx) {
  let count = 0;
  registerPanel(ctx, {
    name: 'tui-counter', description: 'Declarative counter example',
    render: () => ({ title: 'Counter example', fields: [{ id: 'step', label: 'Step', type: 'integer', required: true, default: 2 },
      { id: 'enabled', label: 'Enabled', type: 'boolean', default: true }],
      actions: [{ label: 'Increment', command: 'tui-counter-increment' }], tables: [{ title: 'Counters', columns: ['Name', 'Value'], rows: [['Total', String(count)]] }] }),
    actions: { increment: ({ step, enabled }) => { if (enabled) count += step; } },
  });
}
