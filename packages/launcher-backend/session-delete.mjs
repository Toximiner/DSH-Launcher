/** Launcher-owned adapter for the native JSONL session store. No project files. */
import { createHash } from 'node:crypto';
import { lstat, realpath, rm } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';

export const version = '1.0.0';
function refuse(reason, message) { throw Object.assign(new Error(message), { reason }); }
export function sessionId(value) {
  if (typeof value !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$/.test(value) || value === '.' || value === '..') {
    throw new Error('Invalid launcher session ID');
  }
  return value;
}
const service = (ctx, name) => ctx.get(name);
function requireMethods(value, methods, name) {
  if (!value || methods.some(key => typeof value[key] !== 'function')) throw new Error(`Unsupported native ${name} deletion adapter`);
}
export function capabilities(ctx) {
  try {
    const persistence = service(ctx, 'sessionPersistence');
    requireMethods(persistence, ['list', 'stat', 'locate', 'acquireLease'], 'persistence');
    if (persistence.name !== 'session-persistence-jsonl' || !isAbsolute(persistence.root ?? '')) throw new Error('Only native JSONL session storage is supported');
    if (!(persistence.tracker?.writers instanceof Map) || !(persistence.coldLogMemo instanceof Map)) throw new Error('Unsupported native persistence lifecycle');
    requireMethods(service(ctx, 'sessions'), ['get', 'list', 'flush', 'liveEntryFor', 'detachEntered'], 'session store');
    requireMethods(service(ctx, 'workspaceRegistry'), ['enqueueOperation', 'requireState', 'requireTable', 'setState'], 'workspace registry');
    const registry = service(ctx, 'workspaceRegistry');
    const state = registry.requireState();
    if (![state.workspaceIds, state.archivedSessionIds, state.pinnedSessionIds].every(Array.isArray) || !(registry.entities instanceof Map) || !(registry.headers instanceof Map) || !(registry.sessionPaths instanceof Map) || !registry.invalidSessionPaths || !registry.missingSessionPaths) throw new Error('Unsupported native workspace metadata');
    requireMethods(registry.requireTable(), ['get', 'update'], 'workspace table');
    requireMethods(service(ctx, 'sessionProjectionCache'), ['requireTable'], 'projection cache');
    requireMethods(service(ctx, 'sessionProjectionCache').requireTable(), ['delete'], 'projection table');
    const query = service(ctx, 'sessionQuery');
    if (query && !(query._observations?.cache instanceof Map)) throw new Error('Unsupported native session read cache');
    if (query?._db) requireMethods(query, ['_serialized', '_deleteSession'], 'session search index');
    requireMethods(service(ctx, 'agents'), ['get'], 'agent store');
    requireMethods(service(ctx, 'typertGateway'), ['getPendingRemoteEvents'], 'pending input');
    if (typeof ctx.waterfall !== 'function') throw new Error('Native activity providers are unavailable');
    return { protocolVersion: 1, supportsPermanentDelete: true, reason: '', version };
  } catch (error) {
    return { protocolVersion: 1, supportsPermanentDelete: false, reason: error.message, version };
  }
}

export function descendantTree(headers, rootId) {
  sessionId(rootId);
  const byId = new Map();
  for (const header of headers) {
    sessionId(header.id);
    const previous = byId.get(header.id);
    if (previous && (previous.origin !== header.origin || previous.parentSession !== header.parentSession || previous.cwd !== header.cwd)) throw new Error('Conflicting native session headers');
    byId.set(header.id, header);
  }
  if (!byId.has(rootId)) refuse('missing', 'Session is absent from fresh native storage');
  const selected = new Set([rootId]);
  for (let changed = true; changed;) {
    changed = false;
    for (const header of byId.values()) {
      if (header.origin === 'subagent' && selected.has(header.parentSession) && !selected.has(header.id)) {
        selected.add(header.id); changed = true;
      }
    }
  }
  // Children first; fork sessions sharing parentSession remain independent.
  return [...selected].reverse().map(id => byId.get(id));
}

async function directory(path, optional = false) {
  try {
    const stat = await lstat(path);
    if (stat.isSymbolicLink() || !stat.isDirectory()) throw new Error('Refusing non-directory or symlink in session storage');
  } catch (error) {
    if (optional && error.code === 'ENOENT') return false;
    throw error;
  }
  return true;
}
export async function transcriptDirectory(persistence, header, allowMissing = false) {
  const location = persistence.locate(header);
  if (location?.kind !== 'jsonl' || !isAbsolute(location.path ?? '')) throw new Error('Unsupported native transcript location');
  const root = resolve(persistence.root);
  const parts = relative(root, location.path).split(sep);
  if (parts.length !== 3 || parts.some(part => !part || part === '.' || part === '..') || parts[1] !== sessionId(header.id) || !/^(?:v[1-9][0-9]*|session(?:\.v[1-9][0-9]*)?)\.jsonl(?:\.zstd?)?$/.test(parts[2])) {
    throw new Error('Transcript is outside the expected session-owned JSONL directory');
  }
  const target = dirname(location.path);
  if (header.cwd && (resolve(header.cwd) === resolve(target) || resolve(header.cwd) === root)) throw new Error('Refusing deletion of project or storage root');
  // The configured root may intentionally be an alias. Descendant links may not.
  const canonicalRoot = await realpath(root);
  const projectExists = await directory(join(root, parts[0]), allowMissing);
  const targetExists = projectExists && await directory(target, allowMissing);
  if (!targetExists && allowMissing) return target;
  for (const file of [location.path, join(target, 'session.lock')]) {
    try {
      const info = await lstat(file);
      if (info.isSymbolicLink() || !info.isFile()) throw new Error('Refusing linked or non-file native transcript/lock');
    } catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  if (await realpath(target) !== join(canonicalRoot, parts[0], parts[1])) throw new Error('Session directory escaped native storage');
  return target;
}
async function spillDirectory(ctx, id) {
  const spill = service(ctx, 'spillStore');
  if (spill === undefined) return undefined;
  if (typeof spill.root !== 'string' || !isAbsolute(spill.root)) throw new Error('Unsupported native spill storage');
  const root = resolve(spill.root);
  // Matches native dsh-spill-local's published per-session SHA-256 layout.
  const target = join(root, 'session-' + createHash('sha256').update(id).digest('hex').slice(0, 12));
  if (!await directory(root, true)) return undefined;
  if (!await directory(target, true)) return undefined;
  if (await realpath(target) !== join(await realpath(root), basename(target))) throw new Error('Spill directory escaped native storage');
  return target;
}
function assertIdleNow(ctx, ids) {
  const agents = service(ctx, 'agents');
  const pending = service(ctx, 'typertGateway').getPendingRemoteEvents();
  for (const id of ids) {
    const agent = agents.get(id);
    if (agent) {
      if (agent.status !== 'idle' || (agent.phase && agent.phase.kind !== 'idle') || !Array.isArray(agent.inbox?.nextTurn) || !Array.isArray(agent.inbox?.nextStep)) refuse('active', 'Session activity cannot be safely excluded');
      if (agent.inbox.nextTurn.length || agent.inbox.nextStep.length) refuse('active', 'Session has queued work');
      requireMethods(agent.scope, ['dispose'], 'idle agent lifecycle');
    }
    if (pending.some(entry => String(entry.agentId) === id)) refuse('active', 'Session has pending input');
  }
}
async function activity(ctx, ids) {
  assertIdleNow(ctx, ids);
  for (const id of ids) {
    const items = await ctx.waterfall('workspace/session-activity', { sessionId: id }, () => Promise.resolve([]));
    if (!Array.isArray(items) || items.length) refuse('active', 'Session or descendant has active work');
  }
  assertIdleNow(ctx, ids);
}
async function forgetMetadata(ctx, ids) {
  const registry = service(ctx, 'workspaceRegistry');
  const removed = new Set(ids);
  const state = registry.requireState();
  const table = registry.requireTable();
  for (const workspaceId of state.workspaceIds) {
    const row = table.get(workspaceId);
    if (!row || !row.sessionIds.some(id => removed.has(id))) continue;
    const next = await table.update(workspaceId, current => ({ ...current,
      sessionIds: current.sessionIds.filter(id => !removed.has(id)), updatedAt: new Date().toISOString() }));
    const entity = registry.entities?.get(workspaceId);
    if (entity) entity.record = next;
  }
  await registry.setState({ ...state,
    archivedSessionIds: state.archivedSessionIds.filter(id => !removed.has(id)),
    pinnedSessionIds: state.pinnedSessionIds.filter(id => !removed.has(id)) });
  for (const id of ids) {
    registry.headers?.delete(id); registry.sessionPaths?.delete(id);
    registry.missingSessionPaths?.delete(id); registry.invalidSessionPaths?.delete(id);
  }
}
async function forgetCaches(ctx, ids) {
  const persistence = service(ctx, 'sessionPersistence');
  const projection = service(ctx, 'sessionProjectionCache').requireTable();
  const query = service(ctx, 'sessionQuery');
  // Respect the SQLite engine's existing mutation queue, if this host mounts it.
  const clearQuery = async () => {
    for (const id of ids) {
      query?._observations?.cache?.delete(id);
      if (query?._db && typeof query._deleteSession === 'function') {
        query._deleteSession('persisted', id); query._deleteSession('live', id);
      }
    }
  };
  if (query && typeof query._serialized === 'function') await query._serialized(undefined, clearQuery);
  else { if (query?._tail) await query._tail; await clearQuery(); }
  for (const id of ids) {
    persistence.coldLogMemo.delete(id);
    await projection.delete(id); // ordered after native detach's final checkpoint.
  }
}

async function deleteSessionCore(ctx, id, deleting) {
  sessionId(id);
  const support = capabilities(ctx);
  if (!support.supportsPermanentDelete) refuse('unsupported', support.reason);
  const registry = service(ctx, 'workspaceRegistry');
  return registry.enqueueOperation(async () => {
    const current = capabilities(ctx);
    if (!current.supportsPermanentDelete) refuse('unsupported', current.reason);
    const persistence = service(ctx, 'sessionPersistence');
    const sessions = service(ctx, 'sessions');
    const stored = await persistence.list();
    const headers = [...stored.map(item => item.header), ...sessions.list().map(item => item.header)];
    const tree = descendantTree(headers, id);
    const ids = tree.map(header => header.id);
    if (ids.some(value => deleting.has(value))) throw new Error('Session deletion is already in progress');
    ids.forEach(value => deleting.add(value));
    const leases = [];
    let receipt, operationError, operationFailed = false;
    try {
      // Validate the complete tree and paths before detaching/deleting any entry.
      await activity(ctx, ids);
      for (const header of tree) {
        await transcriptDirectory(persistence, header, true);
        await spillDirectory(ctx, header.id);
        const live = sessions.get(header.id);
        if (live) await sessions.flush(live); // fail closed on durability failure.
      }
      const plans = [];
      for (const header of tree) plans.push({ header,
        transcript: await transcriptDirectory(persistence, header),
        spill: await spillDirectory(ctx, header.id),
        writer: persistence.tracker.writers.get(header.id) });
      await activity(ctx, ids);
      // No await between the final live check and starting all idle-agent teardowns.
      assertIdleNow(ctx, ids);
      const disposals = ids.map(value => {
        try { return service(ctx, 'agents').get(value)?.scope.dispose(); }
        catch (error) { return Promise.reject(error); }
      });
      // scope.dispose() yields before the native agent registry is cleared.
      // Detach every validated-idle Session in this same synchronous boundary:
      // retained Agent references then cannot append a competing prompt, and
      // resumed agents are refused by the session/created deletion guard.
      let detachError;
      try {
        for (const { header } of plans) {
          const live = sessions.get(header.id);
          if (live) sessions.detachEntered(sessions.liveEntryFor(live));
        }
      } catch (error) { detachError = error; }
      const disposed = await Promise.allSettled(disposals);
      if (detachError) throw detachError;
      const disposalFailure = disposed.find(item => item.status === 'rejected');
      if (disposalFailure) throw disposalFailure.reason;
      for (const plan of plans) {
        await plan.writer?.close(); // native detach starts async draining; await it.
        const target = await transcriptDirectory(persistence, plan.header);
        leases.push(await persistence.acquireLease(plan.header.id, plan.header.cwd, target));
      }
      // Native kernel write leases protect existing artifacts from foreign writers.
      // This is a multi-artifact operation, not a cross-process transaction.
      for (const plan of plans) {
        await transcriptDirectory(persistence, plan.header);
        if (plan.spill) {
          const fresh = await spillDirectory(ctx, plan.header.id);
          if (fresh !== plan.spill) throw new Error('Spill location changed during deletion');
          await rm(plan.spill, { recursive: true, force: false });
        }
        await rm(plan.transcript, { recursive: true, force: false });
      }
      await forgetCaches(ctx, ids);
      for (const value of ids) if (await persistence.stat(value) !== undefined) throw new Error('Native storage did not confirm session deletion');
      await forgetMetadata(ctx, ids);
      for (const value of ids) ctx.emit('api-session/removed', value);
      receipt = { deleted: true, sessionIds: [id, ...ids.filter(value => value !== id)] };
    } catch (error) {
      operationFailed = true;
      operationError = error;
    }
    // Catch even synchronous release failures without skipping other leases.
    const released = await Promise.allSettled(leases.reverse().map(lease => Promise.resolve().then(() => lease.release())));
    ids.forEach(value => deleting.delete(value));
    if (operationFailed) throw operationError;
    const failures = released.filter(item => item.status === 'rejected').map(item => item.reason);
    if (failures.length) throw new AggregateError(failures, 'Native deletion lease release failed');
    return receipt;
  });
}

export async function deleteSession(ctx, id, deleting = new Set()) {
  try { return await deleteSessionCore(ctx, id, deleting); }
  catch (error) {
    if (['active', 'unsupported', 'missing'].includes(error.reason)) {
      return { deleted: false, reason: error.reason, sessionIds: [] };
    }
    throw error; // I/O failures are uncertain outcomes, never automatic retries.
  }
}
