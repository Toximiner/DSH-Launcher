// Test-only tool: exercise actual dsh approval routing in an isolated profile.
import { readFile, writeFile } from 'node:fs/promises';
import { setTimeout } from 'node:timers/promises';
import { join } from 'node:path';
import { defineTool } from '@deepseek-ai/dsh-tools';
export const inject = ['tools', 'approval'];
export function apply(ctx, config) {
  ctx.tools.register(defineTool({
    name: 'tui_approval_probe',
    description: 'Request explicit permission before writing an isolated test marker.',
    parameters: { marker: { type: 'string', required: true } },
    output: {
      schema: { type: 'string' },
      render: (_args, result) => [{ type: 'text', text: result }],
    },
    async execute(args, exec) {
      if (!['grant', 'reject', 'cancel'].includes(args.marker)) throw new Error('Invalid test marker');
      const outcome = await ctx.approval.request({
        agent: exec.agent, callId: exec.callId, toolName: 'tui_approval_probe',
        reason: `Create ${args.marker}.marker in the isolated test workspace.`,
        signal: exec.signal,
      });
      if (outcome === 'allowed-once') await writeFile(join(config.directory, `${args.marker}.marker`), 'approved\n');
      return `approval ${outcome}`;
    },
  }));
  ctx.tools.register(defineTool({
    name: 'tui_nested_wait',
    description: 'Hold an isolated nested tool for PTY observation or cancellation.',
    parameters: { marker: { type: 'string', required: true } },
    output: { schema: { type: 'string' }, render: (_args, result) => [{ type: 'text', text: result }] },
    async execute(args, exec) {
      if (!['finish', 'cancel'].includes(args.marker)) throw new Error('Invalid nested marker');
      for (let i = 0; i < 600; i++) {
        exec.signal.throwIfAborted();
        if (args.marker === 'finish') {
          try { await readFile(join(config.directory, 'nested-release.marker')); return 'Nested tool finished'; }
          catch (error) { if (error.code !== 'ENOENT') throw error; }
        }
        await setTimeout(50, undefined, { signal: exec.signal });
      }
      throw new Error('Nested test gate was not released');
    },
  }));

}
