# TUI plugin SDK

[Русский](README.md) | **English**

The minimal `dsh-tui-panel/v1` contract for the native TUI uses the existing
`commands/list` and `commands/execute` API. A command returns a JSON panel in
`result.text`; each plugin needs neither a new server endpoint nor a Go screen.
The SDK is currently distributed as source in this repository.

`registerPanel(ctx, {name, description, render, actions})` registers the opening
command and `name-suffix` action commands. `render(invocation)` returns:

- `title`, `fields`: `string`, `integer`, `boolean`, `enum`; `id`, `label`,
  `required`, `default`, and enum `choices`;
- `actions`: a `label` and a `command` within the owner's command namespace;
- `status`: `label`/`value` pairs;
- `tables`: `title`, `columns`, and string `rows`.

See [greeting.mjs](examples/greeting.mjs) and [counter.mjs](examples/counter.mjs).
Add an example to a test profile's `cordis.patch.yml`:

```yaml
- insert:
    - id: tui-greeting-example
      name: /absolute/path/to/DSH-Launcher/packages/tui-plugin-sdk/examples/greeting.mjs
```

Restart dsh, open `/commands`, find `tui-greeting`, and confirm opening it.
↑/↓ select a field/action; Enter edits strings/numbers, Space toggles booleans,
and ←/→ choose enum options. Each action requires separate confirmation.
Esc cancels editing/confirmation while retaining the form. F5 reloads commands.
Plugin content and labels are not translated by the client.

V1 limits: 64 KiB JSON; 20 fields, 8 actions, 20 statuses, 4 tables, each with
200 rows × 10 columns. Input is limited to 16 KiB and each field to 4096 bytes;
integers use JavaScript's exact range. Unknown versions or field types are refused.
The renderer does not evaluate HTML, JavaScript or arbitrary RPC methods.
Secret fields are outside v1; use dsh credentials for keys. The SDK validates
input again on the server. Handlers own authorization, concurrency checks and
atomic domain changes. Action commands do not record rawInput in command logs.
Disconnected requests are never replayed automatically.

Both examples run against actual dsh in `real_workbench.py`. Their state is kept
in memory only; they demonstrate the contract rather than durable storage.
