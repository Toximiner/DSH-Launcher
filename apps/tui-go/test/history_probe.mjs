// Test-only snapshot rewrites through the real Session append/validation path.
import { readFile, writeFile, unlink, rename } from 'node:fs/promises';
import { join } from 'node:path';
export const inject = ['sessions'];
async function publishResult(directory, value) {
  const result = join(directory, 'history-rewrite-result.json');
  await writeFile(result + '.tmp', JSON.stringify(value));
  await rename(result + '.tmp', result);
}
export function apply(ctx, config) {
  let busy = false;
  const timer = setInterval(async () => {
    if (busy) return;
    busy = true;
    const request = join(config.directory, 'history-rewrite.json');
    let command;
    try {
      command = JSON.parse(await readFile(request, 'utf8'));
      await unlink(request);
      const session = ctx.sessions.get(command.sessionId);
      if (!session) throw new Error('Isolated test session is not active');
      const nodes = session.surface.nodes;
      const start = nodes.indexOf(command.startSeq);
      const end = nodes.indexOf(command.endSeq);
      if (start < 0 || end < start) throw new Error('Invalid test surface range');
      const original = session.snapshotEvents()[command.startSeq];
      if (original.type !== 'user/message') throw new Error('Test rewrite requires a user message');
      const event = session.append('user/message', {
        ...original.data, content: [{ type: 'text', text: command.text }],
      }, { surfaceOp: { op: 'replace', startSeq: command.startSeq, endSeq: command.endSeq },
           sourceEventSeqs: nodes.slice(start, end + 1) });
      await publishResult(config.directory, { seq: event.seq });
    } catch (error) {
      if (command) await publishResult(config.directory, { error: String(error) });
      else if (error.code !== 'ENOENT') console.error('Isolated history probe:', error);
    } finally { busy = false; }
  }, 50);
  ctx.effect(() => () => clearInterval(timer));
}
