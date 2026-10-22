/**
 * `dsh-subagent-council`: a Cordis subagent provider that runs one
 * `council ask -json` process per delegation and publishes it through the
 * out-of-process subagent seam.
 *
 * The provider advertises no start-time capabilities: an external process
 * cannot honour `agentOptions`, `agentPreset`, `outputSchema`, or a numeric
 * `depthLimit`. The bundled patch therefore mounts the delegation tools with
 * `maxDepth: provider-managed`.
 *
 * The process logic lives in `run.js` so the repository can test it without
 * the dsh packages installed; this file only owns the Cordis contract.
 */

// Deliberately node:child_process, not ctx.subprocess. Council needs the
// caller's full environment (its provider CLIs may read credentials from it)
// and a local cwd/worktree; the dsh subprocess seam scrubs credential-shaped
// environment entries and can be backed by a provider that is not this
// machine. The contract for this backend requires `env: process.env` and a
// local process-tree kill, so bypassing the seam is intentional.
import { spawn } from 'node:child_process';
import z from '@deepseek-ai/schemastery';
import {
  NO_START_CAPABILITIES,
  settleRunResult,
  subprocessRunHandle,
} from '@deepseek-ai/dsh-subagent';
import { startCouncilRun } from './run.js';

export const name = 'subagent-council';

export const inject = ['subagents'];

export const Config = z.object({
  providerName: z.string().default('council'),
  councilPath: z.string().default('council'),
  members: z.array(z.string()).default([]),
  // Empty string means "do not override `-summary`": Council then applies its
  // configured `[summary].agent`, if any. `none` is also accepted for older
  // configs, but the documented default is the empty string.
  summary: z.string().default(''),
  rounds: z.number().step(1).min(1).default(1),
  budgetUsd: z.number().min(0.5).default(5),
  workspace: z.boolean().default(false),
  timeoutMs: z.number().step(1).min(1000).default(570000),
});

/**
 * Register one provider instance. The patch mounts this plugin twice:
 * `council` (read-only) and `council-work` (`workspace: true`).
 *
 * @param {import('@deepseek-ai/cordis').Context} ctx - plugin context.
 * @param {object} config - validated {@link Config} value.
 */
export function apply(ctx, config) {
  ctx.subagents.registerProvider(new CouncilProvider(config));
}

class CouncilProvider {
  name;

  capabilities = NO_START_CAPABILITIES;

  inheritsParentContext = false;

  /**
   * @param {object} config - validated plugin config.
   */
  constructor(config) {
    this.config = config;
    this.name = config.providerName;
  }

  /**
   * Start one Council process. The returned handle is published immediately;
   * the child runs in the background and settles its own result.
   *
   * @param {object} request - subagent start request from `ctx.subagents`.
   * @returns {{ id: string, result: Promise<object>, dispose: () => Promise<void> }}
   */
  start(request) {
    return startCouncilRun(this.config, request, {
      spawn,
      settleRunResult,
      subprocessRunHandle,
      platform: process.platform,
    });
  }
}
