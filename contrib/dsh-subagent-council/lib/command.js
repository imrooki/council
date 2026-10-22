/**
 * Pure command construction and result rendering for the Council subagent
 * provider. Nothing here reads the environment, touches the filesystem, or
 * starts a process, so `node --test` covers it without a dsh runtime.
 *
 * The command line is always an argv array. A shell string is never built:
 * task text can contain quotes, newlines, and things that look like flags.
 */

/**
 * D9 follow-up prefix. Conversation ids are `conv_...`; the id is restricted
 * to the characters Council actually emits so a normal task that merely
 * starts with `[conv:` is not mistaken for a follow-up.
 */
const CONVERSATION_PREFIX = /^\[conv:(conv_[A-Za-z0-9_-]+)\](?:[ \t])?/;

/** dsh caps one event string at 8 KiB; keep every emitted block under it. */
export const MAX_OUTPUT_BLOCK_BYTES = 8192;

/** Blank line between blocks; the body budget leaves room for these bytes. */
const BLOCK_BODY_BYTES = MAX_OUTPUT_BLOCK_BYTES - 2;

const utf8Encoder = new TextEncoder();
const utf8Decoder = new TextDecoder();

/**
 * Split the D9 follow-up prefix off a task.
 *
 * `[conv:conv_123] 再做一次` -> `{ task: '再做一次', conversationId: 'conv_123' }`.
 * Anything that does not match verbatim at position 0 stays in `task`.
 *
 * @param {unknown} task - raw task text supplied by the caller.
 * @returns {{ task: string, conversationId: string | undefined }}
 */
export function parseConversation(task) {
  const text = typeof task === 'string' ? task : '';
  const match = CONVERSATION_PREFIX.exec(text);
  if (match === null) return { task: text, conversationId: undefined };
  return { task: text.slice(match[0].length), conversationId: match[1] };
}

/**
 * Flatten the dsh prompt content blocks into the single text argument Council
 * accepts. The delegation tool only sends text today; any other block is kept
 * visible as a marker instead of being dropped silently.
 *
 * @param {unknown} prompt - prompt content blocks (or a plain string).
 * @returns {string}
 */
export function promptToText(prompt) {
  if (typeof prompt === 'string') return prompt;
  if (!Array.isArray(prompt)) return '';
  const parts = [];
  for (const block of prompt) {
    if (block !== null && typeof block === 'object' && block.type === 'text' && typeof block.text === 'string') {
      parts.push(block.text);
      continue;
    }
    const kind = block !== null && typeof block === 'object' && typeof block.type === 'string' ? block.type : 'unknown';
    parts.push(`[unsupported ${kind} prompt block omitted: the Council backend accepts text only]`);
  }
  return parts.join('\n');
}

/**
 * Truncate a string to a UTF-8 byte budget without splitting a code point.
 * The marker is included in the budget and reports that content was removed.
 *
 * @param {unknown} text - candidate text.
 * @param {number} maxBytes - hard upper bound, in UTF-8 bytes.
 * @param {string} [marker] - visible truncation marker.
 * @returns {string}
 */
export function truncateUtf8(text, maxBytes, marker = '\n[truncated]') {
  const value = typeof text === 'string' ? text : '';
  const bytes = utf8Encoder.encode(value);
  if (bytes.byteLength <= maxBytes) return value;
  const markerBytes = utf8Encoder.encode(marker);
  if (markerBytes.byteLength >= maxBytes) {
    // The marker alone would consume the budget. Return a prefix with no
    // marker, backed off to a code-point boundary, rather than emitting a
    // replacement character that could itself overshoot maxBytes.
    let limit = Math.max(0, maxBytes);
    while (limit > 0 && (bytes[limit] & 0b1100_0000) === 0b1000_0000) limit -= 1;
    return utf8Decoder.decode(bytes.subarray(0, limit));
  }
  let limit = maxBytes - markerBytes.byteLength;
  // Back off continuation bytes so the decoded prefix ends on a code-point
  // boundary; a split multi-byte sequence would become U+FFFD.
  while (limit > 0 && (bytes[limit] & 0b1100_0000) === 0b1000_0000) limit -= 1;
  return utf8Decoder.decode(bytes.subarray(0, limit)) + marker;
}

/**
 * Build the exact `council ask` argv for one delegation.
 *
 * Fixed arguments:
 *   <councilPath> ask -json -p <task> -budget-usd <n> -rounds <n>
 * Optional arguments:
 *   -mode round                     (only when rounds > 1; `ask` rejects other rounds)
 *   -members a,b                    (only when the configured list is non-empty)
 *   -summary <id>                   (omitted for an empty string or `none`)
 *   -workspace <cwd>                (only when enabled and a cwd is known)
 *   -conversation <id>              (from the D9 task prefix)
 *
 * No string is ever shell-quoted or concatenated. Each value is one argv
 * element, so a task containing `"`, `'`, newlines, or `--flag` stays one
 * argument all the way to `spawn`.
 *
 * @param {{
 *   councilPath: string,
 *   members: string[],
 *   summary: string,
 *   rounds: number,
 *   budgetUsd: number,
 *   workspace: boolean,
 * }} config - validated plugin config.
 * @param {unknown} task - raw task text, possibly carrying `[conv:<id>]`.
 * @param {string | undefined} cwd - working directory to hand to `-workspace`.
 * @returns {{ argv: string[], note: string }}
 */
export function buildAskCommand(config, task, cwd) {
  const { task: prompt, conversationId } = parseConversation(task);
  const argv = [
    config.councilPath,
    'ask',
    '-json',
    '-p',
    prompt,
    '-budget-usd',
    String(config.budgetUsd),
    '-rounds',
    String(config.rounds),
  ];

  // council `ask` rejects `-rounds` other than 1. Exposing the rounds setting
  // without switching mode would make every multi-round delegation fail with
  // exit 2, so the configuration field implies round mode.
  if (Number.isFinite(config.rounds) && config.rounds > 1) argv.push('-mode', 'round');

  const members = Array.isArray(config.members) ? config.members : [];
  if (members.length > 0) argv.push('-members', members.join(','));

  if (typeof config.summary === 'string' && config.summary !== '' && config.summary !== 'none') {
    argv.push('-summary', config.summary);
  }

  let note = '';
  if (config.workspace === true) {
    if (typeof cwd === 'string' && cwd.trim() !== '') {
      argv.push('-workspace', cwd);
    } else {
      note = 'workspace mode requested but no cwd is available; ran read-only without -workspace';
    }
  }

  if (conversationId !== undefined) argv.push('-conversation', conversationId);

  return { argv, note };
}

/**
 * Render the parsed `council ask -json` payload as dsh text blocks.
 *
 * Contract §4.3:
 *   - a non-ok status turns into a first-line banner;
 *   - the summary comes first;
 *   - each speaking member gets one section (`agent_id / model / $cost` plus
 *     the first 20 lines of every turn);
 *   - the diff appears only for the workspace instance and is cut to 8 KiB;
 *   - the final block is one-line JSON carrying the conversation id for D9.
 *
 * @param {unknown} payload - parsed Council result object.
 * @param {{ includeDiff?: boolean, note?: string }} [options]
 * @returns {string[]} blocks, each already terminated with a newline.
 */
export function formatCouncilOutput(payload, options = {}) {
  const { includeDiff = false, note = '' } = options;
  const result = payload !== null && typeof payload === 'object' ? payload : {};
  const status = typeof result.status === 'string' && result.status !== '' ? result.status : 'error';
  const turns = Array.isArray(result.turns) ? result.turns.filter(isRecord) : [];
  const members = Array.isArray(result.members)
    ? result.members.filter((member) => typeof member === 'string' && member !== '')
    : [];

  const sections = [];

  if (status !== 'ok') sections.push(`${statusBanner(status, result, turns, members)}\n\n`);

  if (note !== '') sections.push(`note: ${note}\n\n`);

  const summary = typeof result.summary === 'string' ? result.summary : '';
  if (summary !== '') {
    sections.push(
      `${truncateUtf8(`## summary\n${summary}`, BLOCK_BODY_BYTES, '\n[summary truncated at 8 KiB]')}\n\n`,
    );
  }

  for (const group of groupTurnsByAgent(turns, members)) {
    sections.push(
      `${truncateUtf8(formatAgentSection(group), BLOCK_BODY_BYTES, '\n[member section truncated at 8 KiB]')}\n\n`,
    );
  }

  if (includeDiff === true && isRecord(result.workspace)) {
    sections.push(...formatWorkspaceSections(result.workspace));
  }

  sections.push(`${JSON.stringify(councilAppendix(result))}\n`);
  return sections;
}

/**
 * Council computes `failed`/`total` by seat: the speaking list plus the
 * summary member when it actually ran. The summary turn is not in `turns`,
 * so deriving the count from turns reports a successful speaking list with a
 * failed summary as `[partial 0/N]`. Prefer the explicit fields; an older
 * council without them falls back to the turns-based approximation.
 */
function statusBanner(status, result, turns, members) {
  if (status !== 'partial') return `[${status}]`;
  if (
    Number.isInteger(result.failed)
    && result.failed >= 0
    && Number.isInteger(result.total)
    && result.total >= 0
  ) {
    return `[partial ${result.failed}/${result.total}]`;
  }
  const total = new Set(members.length > 0 ? members : turns.map((turn) => agentIdOf(turn))).size;
  const failed = new Set(
    turns.filter((turn) => turn.outcome !== 'ok').map((turn) => agentIdOf(turn)),
  ).size;
  return `[partial ${failed}/${total}]`;
}

/** Order members as Council spoke them, then append any turn-only agents. */
function groupTurnsByAgent(turns, members) {
  const order = [];
  const byAgent = new Map();
  for (const member of members) {
    if (!byAgent.has(member)) {
      byAgent.set(member, []);
      order.push(member);
    }
  }
  for (const turn of turns) {
    const agentId = agentIdOf(turn);
    if (!byAgent.has(agentId)) {
      byAgent.set(agentId, []);
      order.push(agentId);
    }
    byAgent.get(agentId).push(turn);
  }
  return order
    .filter((agentId) => byAgent.get(agentId).length > 0)
    .map((agentId) => ({ agentId, turns: byAgent.get(agentId) }));
}

function agentIdOf(turn) {
  return typeof turn.agent_id === 'string' && turn.agent_id !== '' ? turn.agent_id : '(unknown)';
}

function formatAgentSection(group) {
  const first = group.turns[0];
  const model = typeof first.model === 'string' && first.model !== '' ? first.model : '(unknown)';
  const cost = group.turns.reduce(
    (sum, turn) => sum + (Number.isFinite(turn.cost_usd) ? turn.cost_usd : 0),
    0,
  );
  const lines = [`### ${group.agentId} / ${model} / $${cost.toFixed(4)}`];
  const multipleRounds =
    group.turns.length > 1 ||
    group.turns.some((turn) => Number.isInteger(turn.round) && turn.round > 1);

  for (const turn of group.turns) {
    if (multipleRounds) lines.push(`round ${Number.isInteger(turn.round) ? turn.round : '?'}:`);
    if (typeof turn.outcome === 'string' && turn.outcome !== 'ok') lines.push(`outcome: ${turn.outcome}`);
    lines.push(textPreview(turn.text));
  }

  return lines.join('\n');
}

/** First 20 lines and at most one dsh event string. */
function textPreview(text) {
  const value = typeof text === 'string' ? text : '';
  if (value === '') return '(no text)';
  const lines = value.split(/\r?\n/);
  const head = lines.slice(0, 20).join('\n');
  const more = lines.length > 20 ? `\n[text truncated to the first 20 lines; ${lines.length - 20} more]` : '';
  return truncateUtf8(`${head}${more}`, MAX_OUTPUT_BLOCK_BYTES, '\n[member text truncated at 8 KiB]');
}

function formatWorkspaceSections(workspace) {
  const copy = typeof workspace.copy === 'string' && workspace.copy !== '' ? workspace.copy : '(unknown copy path)';
  const files = Array.isArray(workspace.files)
    ? workspace.files.filter((file) => typeof file === 'string' && file !== '')
    : [];
  const header = formatWorkspaceHeader(copy, files);

  if (workspace.diff_truncated === true) {
    const path = typeof workspace.diff_path === 'string' && workspace.diff_path !== ''
      ? workspace.diff_path
      : '(path unavailable)';
    return [header, `(diff omitted by council: over 1 MiB; full diff: ${path})\n\n`];
  }

  const diff = typeof workspace.diff === 'string' ? workspace.diff : '';
  if (diff === '') return [header, '(no diff text)\n\n'];

  return [
    header,
    `${truncateUtf8(diff, BLOCK_BODY_BYTES, `\n[diff truncated at 8 KiB; full copy: ${copy}]`)}\n\n`,
  ];
}

/**
 * The header carries the copy path plus the changed-file list; both count
 * against the same 8 KiB block budget. When the list does not fit, show as
 * many files as the budget allows and report the remainder as `还有 M 个`.
 * A final `truncateUtf8` is the hard backstop for a pathologically long copy
 * path that cannot leave room even for the count.
 */
function formatWorkspaceHeader(copy, files) {
  const prefix = `### workspace diff (copy: ${copy})\nfiles: `;
  const full = files.length > 0 ? `${prefix}${files.join(', ')}` : `${prefix}(none)`;
  if (utf8Length(full) <= BLOCK_BODY_BYTES) return `${full}\n\n`;

  if (files.length > 0) {
    const render = (shownCount) => {
      const remaining = files.length - shownCount;
      if (shownCount === 0) return `${prefix}还有 ${remaining} 个`;
      return `${prefix}${files.slice(0, shownCount).join(', ')}, 还有 ${remaining} 个`;
    };

    // Largest prefix of the file list that still fits; `render(0)` keeps the
    // count even when no filename fits beside the copy path.
    let low = 0;
    let high = files.length;
    while (low < high) {
      const middle = Math.ceil((low + high) / 2);
      if (utf8Length(render(middle)) <= BLOCK_BODY_BYTES) low = middle;
      else high = middle - 1;
    }
    if (utf8Length(render(low)) <= BLOCK_BODY_BYTES) {
      return `${render(low)}\n\n`;
    }
  }

  return `${truncateUtf8(full, BLOCK_BODY_BYTES, '\n[workspace header truncated at 8 KiB]')}\n\n`;
}

function utf8Length(text) {
  return utf8Encoder.encode(text).byteLength;
}

function councilAppendix(result) {
  return {
    conversation_id: typeof result.conversation_id === 'string' ? result.conversation_id : '',
    cost_usd: Number.isFinite(result.cost_usd) ? result.cost_usd : 0,
    status: typeof result.status === 'string' && result.status !== '' ? result.status : 'error',
    workspace: isRecord(result.workspace) && typeof result.workspace.copy === 'string'
      ? result.workspace.copy
      : '',
  };
}

function isRecord(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
