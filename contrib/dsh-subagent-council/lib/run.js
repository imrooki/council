/**
 * Process backend for the Council subagent provider.
 *
 * Kept separate from `index.js` on purpose: the Cordis entry point must import
 * `@deepseek-ai/schemastery` and `@deepseek-ai/dsh-subagent`, which are not
 * installed in the Council repository. This module has no dsh imports, so
 * `node --test lib/` can exercise the real spawn/timeout/cancel/parse logic
 * with an injected `spawn` and injected seam functions.
 */

import { randomUUID } from 'node:crypto';
import { spawn as nodeSpawn } from 'node:child_process';
import {
  buildAskCommand,
  formatCouncilOutput,
  promptToText,
  truncateUtf8,
} from './command.js';

/** `SubagentResult.diagnostic` is capped at 4 KiB by the seam. */
const DIAGNOSTIC_LIMIT_BYTES = 4096;

/** How long to wait for the OS to reap the tree after a kill request. */
export const KILL_SETTLE_MS = 10_000;

/**
 * Start one Council process and publish an out-of-process subagent handle.
 *
 * `deps` is injectable so tests can avoid real processes:
 *   - `spawn` (default `node:child_process.spawn`)
 *   - `platform` (default `process.platform`)
 *   - `killSettleMs` (default {@link KILL_SETTLE_MS})
 *   - `settleRunResult` and `subprocessRunHandle` (the dsh seam functions,
 *     supplied by `index.js`; tests supply faithful doubles)
 *
 * @param {object} config - validated plugin config.
 * @param {object} request - subagent start request.
 * @param {object} [deps] - injectable process and seam dependencies.
 * @returns {{ id: string, result: Promise<object>, dispose: () => Promise<void> }}
 */
export function startCouncilRun(config, request, deps = {}) {
  const spawn = deps.spawn ?? nodeSpawn;
  const platform = deps.platform ?? process.platform;
  const killSettleMs = deps.killSettleMs ?? KILL_SETTLE_MS;
  const settleRunResult = deps.settleRunResult;
  const subprocessRunHandle = deps.subprocessRunHandle;
  if (typeof settleRunResult !== 'function' || typeof subprocessRunHandle !== 'function') {
    throw new TypeError('startCouncilRun requires settleRunResult and subprocessRunHandle');
  }

  const id = randomUUID();
  const candidateSignal = request?.signal;
  // Duck-typed rather than `instanceof`: the request may come from another
  // realm, and the seam only needs the AbortSignal surface.
  const signal = candidateSignal !== null
    && typeof candidateSignal === 'object'
    && typeof candidateSignal.addEventListener === 'function'
    && typeof candidateSignal.aborted === 'boolean'
    ? candidateSignal
    : new AbortController().signal;

  // Cancellation before this activation was published: never spawn at all.
  if (signal.aborted) {
    return settledRun(id, signal, { output: [], stopReason: 'aborted' }, subprocessRunHandle);
  }

  const cwd = resolveRequestCwd(request);
  const task = promptToText(request?.prompt);
  const { argv, note } = buildAskCommand(config, task, cwd);

  let child;
  try {
    child = spawn(argv[0], argv.slice(1), spawnOptions(cwd, platform));
  } catch (error) {
    return settledRun(id, signal, {
      output: [],
      stopReason: 'error',
      diagnostic: failureText(`failed to spawn council (${argv[0]}): ${errorMessage(error)}`, note),
    }, subprocessRunHandle);
  }

  let stdout = '';
  let stderr = '';
  child.stdout?.setEncoding('utf8');
  child.stderr?.setEncoding('utf8');
  child.stdout?.on('data', (chunk) => {
    stdout += chunk;
  });
  child.stderr?.on('data', (chunk) => {
    stderr += chunk;
  });

  let exitCode;
  let exitSignal;
  let spawnError;
  let forcedExit = false;
  let killRequested = false;
  let killIssued = false;
  let cancelled = false;
  let timedOut = false;
  let directExited = false;
  let resolvedExit = false;
  let forceTimer;
  let timeoutTimer;
  let resolveExit;
  const exited = new Promise((resolve) => {
    resolveExit = resolve;
  });

  const settleExit = () => {
    if (resolvedExit) return;
    resolvedExit = true;
    clearTimeout(timeoutTimer);
    clearTimeout(forceTimer);
    resolveExit();
  };

  child.on('error', (error) => {
    // ENOENT/EACCES for the binary or its cwd arrives here, not as a throw.
    spawnError = error;
    settleExit();
  });
  // `exit` fires before `close`; record it so a timeout that races the direct
  // child's exit does not taskkill an already-dead PID.
  child.on('exit', () => {
    directExited = true;
  });
  child.on('close', (code, signalName) => {
    exitCode = code;
    exitSignal = signalName;
    settleExit();
  });

  const armForceTimer = () => {
    if (forceTimer !== undefined || resolvedExit) return;
    forceTimer = setTimeout(() => {
      forcedExit = true;
      settleExit();
    }, killSettleMs);
    forceTimer.unref?.();
  };

  const issueKill = () => {
    if (killIssued || resolvedExit) return;
    const pid = child.pid;
    if (pid === undefined && !directExited) {
      // The direct child has not reported its pid yet. Do not mark the kill as
      // issued: the `spawn` listener retries once the pid exists. Arm the
      // force timer now so a spawn that never arrives still releases the seam.
      killRequested = true;
      armForceTimer();
      return;
    }
    killIssued = true;
    try {
      if (directExited) return;
      if (platform === 'win32') {
        // `/T` walks the child tree; `/F` forces exit. argv form only, no shell.
        const killer = spawn('taskkill', ['/T', '/F', '/PID', String(pid)], {
          windowsHide: true,
          stdio: 'ignore',
        });
        killer.on('error', () => {
          tryChildKill(child);
        });
        killer.on('close', (code) => {
          // taskkill can lose the race with a process that is already gone;
          // a failed taskkill still deserves one direct kill attempt.
          if (code !== 0) tryChildKill(child);
        });
        killer.unref?.();
      } else {
        // `detached: true` makes the child a process-group leader, so a
        // negative pid reaches grandchildren as well.
        try {
          process.kill(-pid, 'SIGKILL');
        } catch {
          tryChildKill(child);
        }
      }
    } catch {
      tryChildKill(child);
    } finally {
      // Arm the force-settle timer even when the kill path returned early or
      // threw; without it a stuck `close` would wedge the seam.
      armForceTimer();
    }
  };

  const killTree = () => {
    if (resolvedExit) return;
    killRequested = true;
    issueKill();
  };

  child.on('spawn', () => {
    // A force-settle can win a pathological race with a very late `spawn`;
    // the result is already settled, but the process must not be orphaned.
    if (resolvedExit) {
      if (killRequested) tryChildKill(child);
      return;
    }
    if (killRequested) issueKill();
  });

  timeoutTimer = setTimeout(() => {
    if (resolvedExit) return;
    timedOut = true;
    killTree();
  }, config.timeoutMs);
  timeoutTimer.unref?.();

  const onAbort = () => {
    if (cancelled) return;
    cancelled = true;
    killTree();
  };
  signal.addEventListener('abort', onAbort, { once: true });
  // The signal may have fired between the pre-spawn check and this listener.
  if (signal.aborted) onAbort();

  let outputBlocks = [];
  let diagnostic;

  const cancelledForSeam = () => cancelled && !timedOut && !forcedExit;

  const collectOutput = () => outputBlocks;

  const attempt = async () => {
    await exited;

    if (timedOut) {
      return {
        output: collectOutput(),
        stopReason: 'error',
        diagnostic: failureText(
          `council ask timed out after ${config.timeoutMs} ms; killed the process tree (pid ${child.pid ?? 'unknown'})`,
          note,
        ),
      };
    }

    if (spawnError !== undefined) {
      return {
        output: [],
        stopReason: 'error',
        diagnostic: failureText(
          `failed to start council (${argv[0]}) in ${cwd ?? '(inherited cwd)'}: ${errorMessage(spawnError)}`,
          note,
        ),
      };
    }

    if (forcedExit) {
      return {
        output: collectOutput(),
        stopReason: 'error',
        diagnostic: failureText(
          `council ask (pid ${child.pid ?? 'unknown'}) did not exit within ${killSettleMs} ms after the kill request`,
          note,
        ),
      };
    }

    const cleanStdout = stdout.replace(/^\uFEFF/, '');
    const parsed = parseCouncilJson(cleanStdout);

    if (exitCode !== 0) {
      // A failed run may still have emitted a complete JSON object (partial
      // results on budget/cancel paths). Keep those results, but report the
      // process-level failure as the stop reason.
      if (parsed.value !== undefined && isRecord(parsed.value)) {
        outputBlocks = renderBlocks(parsed.value, config.workspace, note);
      }
      return {
        output: collectOutput(),
        stopReason: 'error',
        diagnostic: exitDiagnostic(exitCode, exitSignal, stderr, cleanStdout, note),
      };
    }

    if (parsed.error !== undefined) {
      return {
        output: [],
        stopReason: 'error',
        diagnostic: failureText(
          `council ask exited 0 but stdout was not one JSON object (${errorMessage(parsed.error)}): ${cleanStdout}`,
          note,
        ),
      };
    }
    if (!isRecord(parsed.value)) {
      return {
        output: [],
        stopReason: 'error',
        diagnostic: failureText(`council ask exited 0 but stdout JSON was not an object: ${cleanStdout}`, note),
      };
    }

    const payload = parsed.value;
    outputBlocks = renderBlocks(payload, config.workspace, note);

    if (payload.status === 'ok' || payload.status === 'partial') {
      return { output: collectOutput(), stopReason: 'completed' };
    }

    return {
      output: collectOutput(),
      stopReason: 'error',
      diagnostic: failureText(
        `council ask exited 0 with an unexpected status ${JSON.stringify(payload.status)}`,
        note,
      ),
    };
  };

  const result = settleRunResult({
    attempt,
    cancelled: cancelledForSeam,
    collectOutput,
    onError: (error) => {
      diagnostic = failureText(`council subagent run failed: ${errorMessage(error)}`, note);
    },
    collectDiagnostic: () => diagnostic,
    signal,
    onAbort,
  });

  const requestCancel = () => {
    cancelled = true;
    killTree();
  };

  const teardown = async () => {
    clearTimeout(timeoutTimer);
    if (!resolvedExit) {
      killTree();
      // requestCancel() normally armed this already; keep an explicit arm so
      // teardown cannot wait forever if it is called directly.
      armForceTimer();
    }
    await exited;
    clearTimeout(forceTimer);
    signal.removeEventListener('abort', onAbort);
  };

  return subprocessRunHandle({
    id,
    result,
    signal,
    onAbort,
    requestCancel,
    teardown,
  });
}

/**
 * A pre-settled handle for failures that happen before a process exists.
 * `subprocessRunHandle` keeps the shape uniform (id/result/dispose).
 */
function settledRun(id, signal, result, subprocessRunHandle) {
  return subprocessRunHandle({
    id,
    result: Promise.resolve(result),
    signal,
    onAbort: () => {},
    requestCancel: () => {},
    teardown: () => Promise.resolve(),
  });
}

function tryChildKill(child) {
  try {
    child.kill();
  } catch {
    // The process is already gone or not killable; the force timer below
    // still releases the seam.
  }
}

/**
 * Prefer the cwd the service resolved for this child; fall back to the parent
 * session header for programmatic callers that omit it. Never invent one:
 * `-workspace` is skipped when neither exists.
 */
function resolveRequestCwd(request) {
  const direct = request?.cwd;
  if (typeof direct === 'string' && direct.trim() !== '') return direct;
  const inherited = request?.parent?.session?.header?.cwd;
  if (typeof inherited === 'string' && inherited.trim() !== '') return inherited;
  return undefined;
}

function spawnOptions(cwd, platform) {
  const options = {
    env: process.env,
    windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  };
  if (cwd !== undefined) options.cwd = cwd;
  if (platform !== 'win32') options.detached = true;
  return options;
}

function renderBlocks(payload, includeDiff, note) {
  const sections = formatCouncilOutput(payload, { includeDiff, note });
  return sections.map((text) => ({ type: 'text', text }));
}

function parseCouncilJson(text) {
  try {
    return { value: JSON.parse(text) };
  } catch (error) {
    return { error };
  }
}

function exitDiagnostic(code, signalName, stderrText, stdoutText, note) {
  const how = code === null || code === undefined
    ? (signalName ? `terminated by signal ${signalName}` : 'exited without a status code')
    : `exited with code ${code}`;
  const detail = stderrText.trim() !== '' ? stderrText.trim() : stdoutText.trim();
  return failureText(`council ask ${how}: ${detail !== '' ? detail : '(no stderr or stdout)'}`, note);
}

function failureText(text, note) {
  const withNote = note === '' ? text : `${text}\nnote: ${note}`;
  return truncateUtf8(withNote, DIAGNOSTIC_LIMIT_BYTES, '\n[diagnostic truncated]');
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function isRecord(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
