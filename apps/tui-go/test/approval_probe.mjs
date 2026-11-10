// Test-only tool: exercise actual dsh approval routing in an isolated profile.
import { writeFile } from 'node:fs/promises';
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
}
