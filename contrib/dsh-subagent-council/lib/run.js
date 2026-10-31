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
import fs from 'node:fs';
import { win32 as pathWin32 } from 'node:path';
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
 * Unix 上先发 SIGINT 让 council ask 自己取消在跑的 provider CLI，再在宽限
 * 到期后 SIGKILL 整个进程组。Windows 的 taskkill /F 没有「先礼后兵」这一步。
 */
export const KILL_GRACE_MS = 1_500;

/**
 * Start one Council process and publish an out-of-process subagent handle.
 *
 * `deps` is injectable so tests can avoid real processes:
 *   - `spawn` (default `node:child_process.spawn`)
 *   - `platform` (default `process.platform`)
 *   - `killSettleMs` (default {@link KILL_SETTLE_MS})
 *   - `killGraceMs` (default {@link KILL_GRACE_MS}; Unix SIGINT→SIGKILL gap)
 *   - `kill` (default `process.kill`; injectable for tests)
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
  const killGraceMs = deps.killGraceMs ?? KILL_GRACE_MS;
  const signalProcess = deps.kill ?? process.kill.bind(process);
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
  // 裸名在 Windows 上会先搜 cwd；先按 PATH 解析成绝对路径，找不到就保持
  // 原样让 spawn 自己报错（P2）。
  argv[0] = resolveCouncilExecutable(argv[0], platform, process.env);

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
  let killGraceTimer;
  let resolveExit;
  const exited = new Promise((resolve) => {
    resolveExit = resolve;
  });

  const settleExit = () => {
    if (resolvedExit) return;
    resolvedExit = true;
    clearTimeout(timeoutTimer);
    clearTimeout(forceTimer);
    clearTimeout(killGraceTimer);
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
    // Not unref'd: while the timer is pending the run is not settled yet, and
    // `node --test` treats an empty (only-unref'd) event loop as a cancelled
    // test. The child handle normally keeps the loop alive anyway; this timer
    // only has to cover the pathological "child never reports close" path.
    forceTimer = setTimeout(() => {
      forcedExit = true;
      settleExit();
    }, killSettleMs);
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
        // 必须用 System32 下的绝对路径：裸名会先搜 cwd，一个同名的假
        // taskkill.exe 就能把「杀进程」变成执行任意程序（P2）。
        const killer = spawn(systemTaskkillPath(), ['/T', '/F', '/PID', String(pid)], {
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
        // `detached: true` makes the child a process-group leader. SIGINT
        // gives council ask a chance to cancel the running provider CLIs and
        // kill their own process groups; SIGKILL is the fallback for a child
        // that ignores it. A bare SIGKILL on council's group never reaches a
        // provider CLI that council put in its own Setpgid group (B7).
        try {
          signalProcess(-pid, 'SIGINT');
        } catch {
          tryChildKill(child);
        }
        if (killGraceTimer === undefined && !resolvedExit) {
          killGraceTimer = setTimeout(() => {
            killGraceTimer = undefined;
            if (resolvedExit) return;
            try {
              signalProcess(-pid, 'SIGKILL');
            } catch {
              tryChildKill(child);
            }
          }, killGraceMs);
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

  // Same reason as armForceTimer: the timeout is part of the run's lifetime,
  // not a background housekeeping timer. clearTimeout in settleExit/teardown
  // is what releases it, so referencing it cannot keep the host alive after
  // the run ends.
  timeoutTimer = setTimeout(() => {
    if (resolvedExit) return;
    timedOut = true;
    killTree();
  }, config.timeoutMs);

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
    clearTimeout(killGraceTimer);
    if (!resolvedExit) {
      killTree();
      // requestCancel() normally armed this already; keep an explicit arm so
      // teardown cannot wait forever if it is called directly.
      armForceTimer();
    }
    await exited;
    clearTimeout(forceTimer);
    clearTimeout(killGraceTimer);
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

function systemTaskkillPath() {
  const root = process.env.SystemRoot || process.env.windir || 'C:\\Windows';
  return pathWin32.join(root, 'System32', 'taskkill.exe');
}

/**
 * 把一个裸命令名解析成 PATH 里的绝对路径。含分隔符的路径原样返回；找不到
 * 也原样返回，让 spawn 的 ENOENT 诊断保持原样。Windows 上不查 cwd，避免
 * 「同名假 council.exe」被优先执行。
 */
export function resolveCouncilExecutable(binary, platform, env = process.env) {
  if (typeof binary !== 'string' || binary === '') return binary;
  if (binary.includes('/') || binary.includes('\\')) return binary;
  const pathValue = env.PATH || env.Path || '';
  if (pathValue === '') return binary;
  const sep = platform === 'win32' ? '\\' : '/';
  const dirs = pathValue.split(platform === 'win32' ? ';' : ':').filter(Boolean);
  const exts = platform === 'win32'
    ? (env.PATHEXT || '.EXE;.CMD;.BAT;.COM').split(';').filter(Boolean)
    : [''];
  for (const dir of dirs) {
    const base = dir.replace(/[\\/]+$/, '');
    for (const ext of ['', ...exts]) {
      const candidate = `${base}${sep}${binary}${ext}`;
      try {
        if (fs.statSync(candidate).isFile()) return candidate;
      } catch {
        // 下一个候选
      }
    }
  }
  return binary;
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
