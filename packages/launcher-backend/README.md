# Launcher session management backend

The launcher mounts `index.mjs` through a temporary dsh CLI patch. It does not edit
profile plugin files or require Archive Manager. Both runtime files use Node
builtins only and can be copied or embedded together.

The native Typert namespace is `launcherSessions`:

- `capabilities`, arguments `{}`: protocol version 1, package version,
  `supportsPermanentDelete`, and an unsupported-host reason.
- `deleteSession`, arguments `{ "sessionId": "..." }`: confirmed receipt
  `{ "deleted": true, "sessionIds": ["requested-id", "descendant-id"] }`, or a
  definite preflight refusal with `deleted: false`, `reason` (`active`, `missing`,
  `unsupported`) and an empty ID list. Unexpected errors are uncertain outcomes:
  clients must refresh and must not retry writes automatically.

Deletion is serialized with native workspace operations. It enumerates genuine
subagent descendants from live and stored headers; forks remain independent.
Activity providers, live agent inboxes and pending input are checked for the
whole tree before any teardown. Running or queued work is refused rather than
stopped. Idle agent scopes and live sessions are released only after successful
preflight and flush; native write handles are drained before acquiring the
JSONL backend's kernel leases. Only validated session-owned transcript/spill
directories are removed. Projection/read/search caches, workspace membership,
archive/pin metadata and native session indexes are reconciled; clients receive
native removal notifications.

The public persistence API has no deletion method. This adapter therefore checks
native JSONL lifecycle and workspace capabilities and refuses unsupported hosts.
The implementation is original; no Archive Manager source is bundled or copied.
The native service source is the authority for its storage layout and lifecycle.

The configured storage root may be an intentional symlink. Project/session,
transcript and lock symlinks are refused before flush or deletion. Project files,
workspace roots and storage roots are never deletion targets. These checks assume
trusted stable storage ancestors; path validation and recursive removal do not
form an atomic defense against an external process changing directories. A tree
spans multiple artifacts and metadata stores, so deletion is not a transaction:
errors may follow partial cleanup. Do not claim atomic descendant deletion or
server-wide compare-and-swap semantics.

Run safety tests using Node 24:

```sh
node --test --test-isolation=none packages/launcher-backend/session-delete.test.mjs
```

The separate real-backend fixture verifies the mounted API against temporary
native dsh storage without Archive Manager or personal profiles.
