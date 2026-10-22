// The Cordis entry point (index.js) imports the dsh packages, which this
// repository does not install. `run.js` is the injectable process backend that
// index.js wires to the real spawn/seam; these tests cover the backend with a
// fake spawn and faithful seam doubles.
import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import test from 'node:test';

import { KILL_SETTLE_MS, startCouncilRun } from './run.js';

// `node --test lib/` resolves the directory entry to this file on Node 24+.
// Import the command suite too so the required directory command runs both.
import './command.test.js';

class FakeStream extends EventEmitter {
  setEncoding() {}
}

class FakeChild extends EventEmitter {
  constructor() {
    super();
    this.pid = 4242;
    this.stdout = new FakeStream();
    this.stderr = new FakeStream();
    this.killCount = 0;
  }

  kill() {
    this.killCount += 1;
    return true;
  }

  stdoutText(text) {
    this.stdout.emit('data', text);
  }

  stderrText(text) {
    this.stderr.emit('data', text);
  }

  close(code, signal = null) {
    this.emit('close', code, signal);
  }
}

function createSeam() {
  const state = {
    requestCancelCalls: 0,
    teardownCalls: 0,
  };

  // Faithful doubles of @deepseek-ai/dsh-subagent/out-of-process. The
  // repository does not install the dsh packages, so the real seam cannot be
  // imported here; run.js receives it by injection from index.js at runtime.
  const settleRunResult = (parts) => (async () => {
    try {
      const result = await parts.attempt();
      return parts.cancelled()
        ? { output: parts.collectOutput(), stopReason: 'aborted' }
        : result;
    } catch (error) {
      if (parts.cancelled()) {
        return { output: parts.collectOutput(), stopReason: 'aborted' };
      }
      try {
        parts.onError?.(error, 'error');
      } catch {
        // The diagnostic sink cannot reject the run result.
      }
      const diagnostic = parts.collectDiagnostic?.();
      return {
        output: parts.collectOutput(),
        ...(diagnostic === undefined ? {} : { diagnostic }),
        stopReason: 'error',
      };
    } finally {
      parts.signal.removeEventListener('abort', parts.onAbort);
    }
  })();

  const subprocessRunHandle = (parts) => {
    let disposal;
    return {
      id: parts.id,
      result: parts.result,
      dispose() {
        if (disposal !== undefined) return disposal;
        parts.signal.removeEventListener('abort', parts.onAbort);
        state.requestCancelCalls += 1;
        parts.requestCancel();
        state.teardownCalls += 1;
        disposal = parts.teardown();
        return disposal;
      },
    };
  };

  return {
    state,
    deps: { settleRunResult, subprocessRunHandle },
  };
}

function createHarness() {
  const calls = [];
  const children = [];
  const killers = [];
  const spawn = (command, args, options) => {
    calls.push({ command, args, options });
    if (command === 'taskkill') {
      const killer = new EventEmitter();
      killer.unref = () => {};
      killers.push(killer);
      return killer;
    }
    const child = new FakeChild();
    children.push(child);
    return child;
  };
  const seam = createSeam();
  return {
    calls,
    children,
    killers,
    seam,
    deps: {
      spawn,
      platform: 'win32',
      killSettleMs: 25,
      ...seam.deps,
    },
    taskkillCalls: () => calls.filter((call) => call.command === 'taskkill'),
  };
}

function councilConfig(overrides = {}) {
  return {
    councilPath: 'council.exe',
    members: [],
    summary: '',
    rounds: 1,
    budgetUsd: 5,
    workspace: false,
    timeoutMs: 500,
    ...overrides,
  };
}

function councilRequest(overrides = {}) {
  return {
    prompt: [{ type: 'text', text: 'question' }],
    cwd: 'C:\\repo',
    signal: new AbortController().signal,
    parent: { session: { header: { cwd: 'C:\\repo' } } },
    ...overrides,
  };
}

function start(harness, { config = {}, request = {} } = {}) {
  const handle = startCouncilRun(
    councilConfig(config),
    councilRequest(request),
    harness.deps,
  );
  return { handle, child: harness.children[0], harness };
}

function payload(overrides = {}) {
  return {
    status: 'ok',
    conversation_id: 'conv_smoke',
    summary: 'smoke summary',
    members: ['agent-a'],
    turns: [{
      agent_id: 'agent-a',
      model: 'model-a',
      effort: 'low',
      round: 1,
      outcome: 'ok',
      text: 'turn text',
      usage: { in: 0, cache_create: 0, cache_read: 0, out: 0 },
      cost_usd: 0.25,
      duration_ms: 1,
    }],
    cost_usd: 0.25,
    cost_complete: true,
    failed: 0,
    total: 1,
    ...overrides,
  };
}

const tick = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

async function waitFor(predicate, timeoutMs = 1000) {
  const startedAt = Date.now();
  while (!predicate()) {
    if (Date.now() - startedAt > timeoutMs) throw new Error('timed out waiting for test condition');
    await tick(5);
  }
}

test('startCouncilRun: spawn ENOENT settles an error with a diagnostic', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);
  child.pid = undefined;
  child.emit('error', Object.assign(new Error('spawn council.exe ENOENT'), { code: 'ENOENT' }));

  const result = await handle.result;
  assert.equal(result.stopReason, 'error');
  assert.equal(result.output.length, 0);
  assert.match(result.diagnostic, /failed to start council/);
  assert.match(result.diagnostic, /ENOENT/);
});

test('startCouncilRun: timeout issues taskkill and force-settles without a close event', async () => {
  const harness = createHarness();
  const { handle } = start(harness, { config: { timeoutMs: 10 } });
  const startedAt = Date.now();

  const result = await handle.result;
  assert.equal(result.stopReason, 'error');
  assert.match(result.diagnostic, /timed out after 10 ms/);
  assert.equal(harness.taskkillCalls().length, 1);
  assert.ok(Date.now() - startedAt < 1000, 'force-settle timer released the seam');
});

test('startCouncilRun: a dispose that cannot reap the child reports the force-settle failure', async () => {
  const harness = createHarness();
  const { handle } = start(harness);
  const disposal = handle.dispose();

  const result = await handle.result;
  assert.equal(result.stopReason, 'error');
  assert.match(result.diagnostic, /did not exit within 25 ms after the kill request/);
  await disposal;
  assert.equal(harness.taskkillCalls().length, 1);
});

test('startCouncilRun: a kill request before the pid exists still force-settles', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);
  child.pid = undefined;

  const disposal = handle.dispose();
  const result = await handle.result;
  assert.equal(result.stopReason, 'error');
  assert.match(result.diagnostic, /did not exit within 25 ms after the kill request/);
  await disposal;
  assert.equal(harness.taskkillCalls().length, 0);
});

test('startCouncilRun: abort kills the tree and settles aborted', async () => {
  const harness = createHarness();
  const controller = new AbortController();
  const { handle, child } = start(harness, { request: { signal: controller.signal } });

  controller.abort();
  assert.equal(harness.taskkillCalls().length, 1, 'abort issues taskkill synchronously');
  harness.killers[0].emit('close', 0);
  child.close(1);

  const result = await handle.result;
  assert.equal(result.stopReason, 'aborted');
  assert.equal(result.output.length, 0);
});

test('startCouncilRun: dispose is idempotent and requests one kill', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  const first = handle.dispose();
  const second = handle.dispose();
  assert.equal(first, second);
  assert.equal(harness.seam.state.requestCancelCalls, 1);
  assert.equal(harness.seam.state.teardownCalls, 1);
  assert.equal(harness.taskkillCalls().length, 1);

  child.close(1);
  await first;
  const result = await handle.result;
  assert.equal(result.stopReason, 'aborted');
});

test('startCouncilRun: bad JSON at exit 0 is an error with the stdout diagnostic', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  child.stdoutText('this is not json');
  child.close(0);
  const result = await handle.result;

  assert.equal(result.stopReason, 'error');
  assert.equal(result.output.length, 0);
  assert.match(result.diagnostic, /not one JSON object/);
  assert.match(result.diagnostic, /this is not json/);
});

test('startCouncilRun: empty stdout at exit 0 is an error', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  child.close(0);
  const result = await handle.result;

  assert.equal(result.stopReason, 'error');
  assert.equal(result.output.length, 0);
  assert.match(result.diagnostic, /not one JSON object/);
});

test('startCouncilRun: a UTF-8 BOM before the JSON object is ignored', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  child.stdoutText(`\uFEFF${JSON.stringify(payload())}`);
  child.close(0);
  const result = await handle.result;

  assert.equal(result.stopReason, 'completed');
  assert.ok(result.output.some((block) => block.text.startsWith('## summary')));
});

test('startCouncilRun: exit != 0 with valid JSON keeps the output and reports the failure', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  child.stdoutText(JSON.stringify(payload()));
  child.stderrText('council: 预算触线');
  child.close(1);
  const result = await handle.result;

  assert.equal(result.stopReason, 'error');
  assert.ok(result.output.some((block) => block.text.startsWith('## summary')));
  assert.match(result.diagnostic, /exited with code 1/);
  assert.match(result.diagnostic, /预算触线/);
});

test('startCouncilRun: diagnostics are capped at 4 KiB', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness);

  child.stdoutText('x'.repeat(10_000));
  child.close(0);
  const result = await handle.result;

  assert.equal(result.stopReason, 'error');
  assert.ok(Buffer.byteLength(result.diagnostic, 'utf8') <= 4096);
  assert.match(result.diagnostic, /diagnostic truncated/);
});

test('startCouncilRun: an exit before the timeout does not taskkill an already-dead pid', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness, { config: { timeoutMs: 10 } });

  child.emit('exit', 1, null);
  const result = await handle.result;

  assert.equal(result.stopReason, 'error');
  assert.match(result.diagnostic, /timed out after 10 ms/);
  assert.equal(harness.taskkillCalls().length, 0);
});

test('startCouncilRun: a non-zero taskkill close falls back to child.kill()', async () => {
  const harness = createHarness();
  const { handle, child } = start(harness, { config: { timeoutMs: 10 } });

  await waitFor(() => harness.killers.length === 1);
  const killer = harness.killers[0];
  killer.emit('close', 1);
  assert.equal(child.killCount, 1);

  const result = await handle.result;
  assert.equal(result.stopReason, 'error');
  assert.match(result.diagnostic, /timed out after 10 ms/);
});

test('KILL_SETTLE_MS remains the default force-settle bound', () => {
  assert.equal(KILL_SETTLE_MS, 10_000);
});
