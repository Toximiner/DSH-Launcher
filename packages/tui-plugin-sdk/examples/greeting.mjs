import { registerPanel } from '../index.mjs';
export const name = 'tui-greeting-example';
export function apply(ctx) {
  let result = 'No greeting yet';
  registerPanel(ctx, {
    name: 'tui-greeting', description: 'Declarative greeting example',
    render: () => ({ title: 'Greeting example', fields: [{ id: 'name', label: 'Name', type: 'string', required: true, default: 'World' }],
      actions: [{ label: 'Greet', command: 'tui-greeting-submit' }], status: [{ label: 'Result', value: result }] }),
    actions: { submit: ({ name }) => { result = 'Hello, ' + name; } },
  });
}
