// Test producer: exercise the real shared-job registry without a Session owner.
export const inject = ['jobs'];
export function apply(ctx) {
  ctx.jobs.attachController("tui-test-producer");
  ctx.jobs.start({
    kind: 'tui-fixture',
    label: 'Shared host fixture',
    run(job) {
      let finish;
      const done = new Promise(resolve => { finish = resolve; });
      job.append('SHARED_HOST_OUTPUT привет\n', { gapBefore: true });
      job.updateProgress('Waiting for explicit stop');
      return { done, cancel() { finish({ status: 'killed' }); } };
    },
  });
}
