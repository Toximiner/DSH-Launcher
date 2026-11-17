/** Self-contained Cordis plugin mounted by the launcher's temporary CLI overlay. */
import { capabilities, deleteSession, sessionId } from './session-delete.mjs';
export const name = 'dsh-launcher-session-management';
export const inject = ['sessions', 'sessionPersistence', 'workspaceRegistry', 'sessionProjectionCache', 'agents', 'typertGateway', 'typert'];

function schema(parse) {
  return { parse, safeParse(value) {
    try { return { success: true, data: parse(value) }; }
    catch (error) { return { success: false, error }; }
  } };
}
function codec(symbol, parser) {
  const value = schema(parser);
  return { mode: 'strict', typeSymbol: '@dsh-launcher/backend#' + symbol,
    create: () => value, schema: value };
}
const idCodec = codec('SessionId', sessionId);
const resultCodec = codec('DeleteReceipt', value => {
  if (!value || typeof value.deleted !== 'boolean' || !Array.isArray(value.sessionIds)) throw new Error('Invalid launcher deletion receipt');
  value.sessionIds.forEach(sessionId);
  if (!value.deleted && !['active', 'unsupported', 'missing'].includes(value.reason)) throw new Error('Invalid launcher refusal');
  return value;
});
const capabilityCodec = codec('Capabilities', value => {
  if (!value || value.protocolVersion !== 1 || typeof value.supportsPermanentDelete !== 'boolean' || typeof value.reason !== 'string' || typeof value.version !== 'string') throw new Error('Invalid launcher capabilities');
  return value;
});
class LauncherSessions {
  constructor(ctx, deleting) {
    this.ctx = ctx;
    this.deleting = deleting;
    // Versioned public binding/marker shape consumed by native Typert source discovery.
    this.typertRemote = Object.freeze({ service: this, serviceKey: 'launcherSessions', namespace: 'launcherSessions' });
  }
  capabilities() { return capabilities(this.ctx); }
  deleteSession(sessionId) { return deleteSession(this.ctx, sessionId, this.deleting); }
}
Object.defineProperty(LauncherSessions.prototype, '@deepseek-ai/dsh-typert-protocol/remote-methods', {
  value: Object.freeze({ version: 1, methods: Object.freeze([
    Object.freeze({ method: 'capabilities', invocation: Object.freeze({ kind: 'direct' }) }),
    Object.freeze({ method: 'deleteSession', invocation: Object.freeze({ kind: 'direct' }) }),
  ]) }),
});
const contribution = { package: '@dsh-launcher/backend', face: 'host', schemas: [],
  model: { services: [], events: [], objects: [] }, invocations: [
    { id: '@dsh-launcher/backend#launcherSessions/capabilities', service: 'launcherSessions', namespace: 'launcherSessions', method: 'capabilities',
      invocation: { kind: 'direct' }, parameters: [], result: capabilityCodec,
      sourceLocation: { file: '@dsh-launcher/backend/index.mjs', line: 1, column: 1 } },
    { id: '@dsh-launcher/backend#launcherSessions/deleteSession', service: 'launcherSessions', namespace: 'launcherSessions', method: 'deleteSession',
      invocation: { kind: 'direct' }, parameters: [{ name: 'sessionId', wire: 'sessionId', source: 'json', codec: idCodec }], result: resultCodec,
      sourceLocation: { file: '@dsh-launcher/backend/index.mjs', line: 1, column: 1 } },
  ] };
export function apply(ctx) {
  const deleting = new Set();
  ctx.on('session/created', session => {
    if (deleting.has(session.id) || (session.header.origin === 'subagent' && deleting.has(session.header.parentSession))) throw new Error('Session deletion in progress');
  });
  ctx.provide('launcherSessions', new LauncherSessions(ctx, deleting));
  const dispose = ctx.typert.register(contribution);
  return () => { if (typeof dispose === 'function') dispose(); };
}
