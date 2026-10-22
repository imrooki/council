import test from 'node:test';
import assert from 'node:assert/strict';

import {
  MAX_OUTPUT_BLOCK_BYTES,
  buildAskCommand,
  formatCouncilOutput,
  parseConversation,
  promptToText,
  truncateUtf8,
} from './command.js';

function config(overrides = {}) {
  return {
    councilPath: 'council.exe',
    members: [],
    summary: 'none',
    rounds: 1,
    budgetUsd: 5,
    workspace: false,
    ...overrides,
  };
}

function valueAfter(argv, flag) {
  const index = argv.indexOf(flag);
  return index === -1 ? undefined : argv[index + 1];
}

test('parseConversation: [conv:conv_abc] prefix becomes conversationId, task loses only the prefix', () => {
  assert.deepEqual(parseConversation('[conv:conv_abc] 问题'), {
    task: '问题',
    conversationId: 'conv_abc',
  });
  assert.deepEqual(parseConversation('[conv:conv_123]'), {
    task: '',
    conversationId: 'conv_123',
  });
});

test('parseConversation: no prefix leaves the task byte-for-byte intact', () => {
  const task = '  [conv:conv_abc] 问题\n第二行';
  assert.deepEqual(parseConversation(task), { task, conversationId: undefined });
  assert.deepEqual(parseConversation('[CONV:conv_abc] 问题'), {
    task: '[CONV:conv_abc] 问题',
    conversationId: undefined,
  });
  assert.deepEqual(parseConversation('[conv:] 问题'), {
    task: '[conv:] 问题',
    conversationId: undefined,
  });
  assert.deepEqual(parseConversation(undefined), { task: '', conversationId: undefined });
});

test('parseConversation: only a real conv_ id at line start is accepted', () => {
  for (const task of [
    '[conv:abc] 问题',
    '[conv:conv_a.b] 问题',
    '[conv:conv_abc',
  ]) {
    assert.deepEqual(parseConversation(task), { task, conversationId: undefined });
  }
  assert.deepEqual(parseConversation('[conv:conv_ABC-09_] 问题'), {
    task: '问题',
    conversationId: 'conv_ABC-09_',
  });
});

test('parseConversation: only the first prefix and at most one separator are stripped', () => {
  assert.deepEqual(parseConversation('[conv:conv_abc] 问题 [conv:conv_def]'), {
    task: '问题 [conv:conv_def]',
    conversationId: 'conv_abc',
  });
  assert.deepEqual(parseConversation('[conv:conv_abc]   问题'), {
    task: '  问题',
    conversationId: 'conv_abc',
  });
  assert.deepEqual(parseConversation('[conv:conv_abc]\n问题'), {
    task: '\n问题',
    conversationId: 'conv_abc',
  });
});

test('buildAskCommand: workspace:false never emits -workspace', () => {
  const { argv, note } = buildAskCommand(config({ workspace: false }), 'q', 'C:\\repo');
  assert.equal(valueAfter(argv, '-workspace'), undefined);
  assert.equal(note, '');
});

test('buildAskCommand: workspace:true emits -workspace only when cwd has a value', () => {
  const withCwd = buildAskCommand(config({ workspace: true }), 'q', 'C:\\repo');
  assert.equal(valueAfter(withCwd.argv, '-workspace'), 'C:\\repo');
  assert.equal(withCwd.note, '');

  for (const cwd of [undefined, '', '   ']) {
    const withoutCwd = buildAskCommand(config({ workspace: true }), 'q', cwd);
    assert.equal(valueAfter(withoutCwd.argv, '-workspace'), undefined);
    assert.notEqual(withoutCwd.note, '');
  }
});

test('buildAskCommand: empty members omits -members; a non-empty list is comma-joined', () => {
  const empty = buildAskCommand(config({ members: [] }), 'q', undefined);
  assert.equal(valueAfter(empty.argv, '-members'), undefined);

  const listed = buildAskCommand(config({ members: ['opus-5.5-max', 'gpt-6.1-sol-max'] }), 'q', undefined);
  assert.equal(valueAfter(listed.argv, '-members'), 'opus-5.5-max,gpt-6.1-sol-max');
});

test('buildAskCommand: summary "none" is a sentinel and omits -summary', () => {
  for (const summary of ['none', '']) {
    const { argv } = buildAskCommand(config({ summary }), 'q', undefined);
    assert.equal(valueAfter(argv, '-summary'), undefined);
  }

  const { argv } = buildAskCommand(config({ summary: 'opus-5.5-max' }), 'q', undefined);
  assert.equal(valueAfter(argv, '-summary'), 'opus-5.5-max');
});

test('buildAskCommand: fixed arguments and the budget value', () => {
  const { argv } = buildAskCommand(config({ budgetUsd: 0.5 }), 'q', undefined);
  assert.equal(argv[0], 'council.exe');
  assert.equal(argv[1], 'ask');
  assert.ok(argv.includes('-json'));
  assert.equal(valueAfter(argv, '-p'), 'q');
  assert.equal(valueAfter(argv, '-budget-usd'), '0.5');
  assert.equal(valueAfter(argv, '-rounds'), '1');
});

test('buildAskCommand: rounds > 1 also selects round mode', () => {
  const { argv } = buildAskCommand(config({ rounds: 3, summary: 'judge' }), 'q', undefined);
  assert.equal(valueAfter(argv, '-rounds'), '3');
  assert.equal(valueAfter(argv, '-mode'), 'round');

  const single = buildAskCommand(config({ rounds: 1 }), 'q', undefined);
  assert.equal(valueAfter(single.argv, '-mode'), undefined);
});

test('buildAskCommand: [conv:...] becomes -conversation and the task loses the prefix', () => {
  const { argv } = buildAskCommand(config(), '[conv:conv_abc] 追问', undefined);
  assert.equal(valueAfter(argv, '-conversation'), 'conv_abc');
  assert.equal(valueAfter(argv, '-p'), '追问');
});

test('buildAskCommand: quotes, newlines, and --flag text stay one argv element', () => {
  const task = '第一行 "双引号" 和 \'单引号\'\n--flag --budget-usd 0.01\n[conv:not-a-prefix]';
  const { argv } = buildAskCommand(config({ budgetUsd: 5 }), task, undefined);

  assert.equal(valueAfter(argv, '-p'), task);
  assert.equal(argv.filter((argument) => argument === task).length, 1);
  // The only real budget flag is the one the builder emitted.
  assert.equal(argv.filter((argument) => argument === '-budget-usd').length, 1);
  assert.equal(valueAfter(argv, '-budget-usd'), '5');

  const leadingFlag = buildAskCommand(config(), '--flag-looking-task', undefined);
  assert.equal(valueAfter(leadingFlag.argv, '-p'), '--flag-looking-task');
});

test('promptToText: text blocks flatten without dropping unsupported blocks silently', () => {
  assert.equal(promptToText('plain'), 'plain');
  assert.equal(
    promptToText([
      { type: 'text', text: 'one' },
      { type: 'text', text: 'two' },
    ]),
    'one\ntwo',
  );
  assert.match(promptToText([{ type: 'image', data: '...' }]), /unsupported image prompt block omitted/);
  assert.equal(promptToText(undefined), '');
});

test('truncateUtf8: honours the byte budget, keeps a marker, and never splits a code point', () => {
  assert.equal(truncateUtf8('short', 100), 'short');
  assert.equal(truncateUtf8('x'.repeat(20), 40), 'x'.repeat(20));

  const cut = truncateUtf8('x'.repeat(100), 40, '...');
  assert.ok(Buffer.byteLength(cut, 'utf8') <= 40);
  assert.ok(cut.endsWith('...'));

  const multibyte = truncateUtf8('你好世界', 10, '!');
  assert.ok(Buffer.byteLength(multibyte, 'utf8') <= 10);
  assert.ok(!multibyte.includes('\uFFFD'), 'no replacement character from a split sequence');
});

test('truncateUtf8: a marker that does not fit still returns a bounded prefix', () => {
  const chinese = truncateUtf8('你好世界', 8, '[a-long-marker]');
  assert.ok(Buffer.byteLength(chinese, 'utf8') <= 8);
  assert.ok(!chinese.includes('\uFFFD'));
  assert.equal(chinese, '你好');

  assert.equal(truncateUtf8('abcdef', 2, 'LONG_MARKER'), 'ab');
  assert.equal(truncateUtf8('你好', 1, 'LONG_MARKER'), '');
  assert.equal(truncateUtf8('你好', 0, 'LONG_MARKER'), '');
});

test('truncateUtf8: MAX_OUTPUT_BLOCK_BYTES is 8 KiB', () => {
  assert.equal(MAX_OUTPUT_BLOCK_BYTES, 8192);
});

test('formatCouncilOutput: ok result has summary, member section, and a JSON appendix', () => {
  const payload = {
    status: 'ok',
    conversation_id: 'conv_1',
    summary: '两票同意，一票保留。',
    members: ['opus-5.5-max'],
    turns: [
      {
        agent_id: 'opus-5.5-max',
        model: 'opus-5.5',
        effort: 'max',
        round: 1,
        outcome: 'ok',
        text: 'line 1\nline 2',
        cost_usd: 0.1234,
      },
    ],
    cost_usd: 0.1234,
    cost_complete: true,
  };

  const sections = formatCouncilOutput(payload, { includeDiff: false });
  assert.equal(sections[0], '## summary\n两票同意，一票保留。\n\n');
  assert.ok(sections.some((section) => section.includes('### opus-5.5-max / opus-5.5 / $0.1234')));
  assert.ok(sections.some((section) => section.includes('line 1\nline 2')));

  const appendix = JSON.parse(sections.at(-1));
  assert.deepEqual(appendix, {
    conversation_id: 'conv_1',
    cost_usd: 0.1234,
    status: 'ok',
    workspace: '',
  });
});

test('formatCouncilOutput: non-ok status writes the banner first', () => {
  const partial = formatCouncilOutput({
    status: 'partial',
    members: ['a', 'b'],
    turns: [
      { agent_id: 'a', model: 'm', outcome: 'ok', text: 'fine', cost_usd: 0 },
      { agent_id: 'b', model: 'm', outcome: 'provider_failed', text: '', cost_usd: 0 },
    ],
  });
  assert.equal(partial[0], '[partial 1/2]\n\n');

  const budget = formatCouncilOutput({ status: 'budget_exceeded', members: [], turns: [] });
  assert.equal(budget[0], '[budget_exceeded]\n\n');
});

test('formatCouncilOutput: partial prefers failed/total when the summary failed outside turns', () => {
  const sections = formatCouncilOutput({
    status: 'partial',
    failed: 1,
    total: 4,
    summary: '',
    members: ['a', 'b', 'c'],
    turns: [
      { agent_id: 'a', model: 'm', outcome: 'ok', text: 'a', cost_usd: 0 },
      { agent_id: 'b', model: 'm', outcome: 'ok', text: 'b', cost_usd: 0 },
      { agent_id: 'c', model: 'm', outcome: 'ok', text: 'c', cost_usd: 0 },
    ],
  });
  assert.equal(sections[0], '[partial 1/4]\n\n');
  assert.doesNotMatch(sections[0], /\[partial 0\//);
});

test('formatCouncilOutput: rounds are grouped under one member heading', () => {
  const sections = formatCouncilOutput({
    status: 'ok',
    members: ['a'],
    summary: '',
    turns: [
      { agent_id: 'a', model: 'm', round: 1, outcome: 'ok', text: 'first', cost_usd: 0.25 },
      { agent_id: 'a', model: 'm', round: 2, outcome: 'ok', text: 'second', cost_usd: 0.75 },
    ],
  });
  const member = sections.find((section) => section.startsWith('### a'));
  assert.ok(member.includes('### a / m / $1.0000'));
  assert.ok(member.includes('round 1:'));
  assert.ok(member.includes('round 2:'));
  assert.equal((member.match(/### a /g) ?? []).length, 1);
});

test('formatCouncilOutput: member text keeps only the first 20 lines', () => {
  const text = Array.from({ length: 25 }, (_, index) => `line ${index + 1}`).join('\n');
  const sections = formatCouncilOutput({
    status: 'ok',
    members: ['a'],
    turns: [{ agent_id: 'a', model: 'm', outcome: 'ok', text, cost_usd: 0 }],
  });
  const member = sections.find((section) => section.startsWith('### a'));
  assert.ok(member.includes('line 1\n'));
  assert.ok(member.includes('line 20\n'));
  assert.ok(!member.includes('line 21\n'));
  assert.match(member, /first 20 lines; 5 more/);
});

test('formatCouncilOutput: diff appears only for the workspace instance', () => {
  const payload = {
    status: 'ok',
    members: ['a'],
    summary: '',
    turns: [{ agent_id: 'a', model: 'm', outcome: 'ok', text: 'done', cost_usd: 0 }],
    workspace: {
      copy: 'C:\\data\\workspaces\\conv_1',
      diff: '--- a/x.go\n+++ b/x.go\n+hello\n',
      files: ['x.go'],
    },
  };

  const readOnly = formatCouncilOutput(payload, { includeDiff: false }).join('');
  assert.ok(!readOnly.includes('--- a/x.go'));

  const work = formatCouncilOutput(payload, { includeDiff: true }).join('');
  assert.ok(work.includes('### workspace diff (copy: C:\\data\\workspaces\\conv_1)'));
  assert.ok(work.includes('files: x.go'));
  assert.ok(work.includes('+hello'));
});

test('formatCouncilOutput: diff is cut to 8 KiB and points at the copy', () => {
  const sections = formatCouncilOutput({
    status: 'ok',
    members: [],
    summary: '',
    turns: [],
    workspace: {
      copy: 'C:\\copy',
      diff: 'x'.repeat(9000),
      files: [],
    },
  }, { includeDiff: true });

  const diffBlock = sections.find((section) => section.includes('[diff truncated at 8 KiB'));
  assert.ok(diffBlock);
  assert.ok(Buffer.byteLength(diffBlock.trimEnd(), 'utf8') <= MAX_OUTPUT_BLOCK_BYTES);
  assert.ok(diffBlock.includes('full copy: C:\\copy'));
});

test('formatCouncilOutput: a council-side diff_truncated result surfaces the path', () => {
  const sections = formatCouncilOutput({
    status: 'ok',
    members: [],
    summary: '',
    turns: [],
    workspace: {
      copy: 'C:\\copy',
      diff: '',
      files: ['big.bin'],
      diff_truncated: true,
      diff_path: 'C:\\copy.diff',
    },
  }, { includeDiff: true });
  assert.ok(sections.some((section) => section.includes('full diff: C:\\copy.diff')));
  assert.ok(sections.some((section) => section.includes('files: big.bin')));
});

test('formatCouncilOutput: a 500-file workspace header stays within 8 KiB and counts the rest', () => {
  const files = Array.from(
    { length: 520 },
    (_, index) => `src/some/deep/path/file-${String(index).padStart(3, '0')}-with-a-long-name.ts`,
  );
  const sections = formatCouncilOutput({
    status: 'ok',
    members: [],
    summary: '',
    turns: [],
    workspace: {
      copy: 'C:\\data\\workspaces\\conv_many',
      diff: '',
      files,
    },
  }, { includeDiff: true });

  const header = sections.find((section) => section.startsWith('### workspace diff'));
  assert.ok(header.includes('C:\\data\\workspaces\\conv_many'));
  assert.match(header, /还有 \d+ 个/);
  assert.doesNotMatch(header, /src\/some\/deep\/path\/file-519-with-a-long-name\.ts/);

  for (const section of sections) {
    assert.ok(
      Buffer.byteLength(section, 'utf8') <= MAX_OUTPUT_BLOCK_BYTES,
      `block exceeds 8 KiB: ${Buffer.byteLength(section, 'utf8')} bytes`,
    );
  }
});

test('formatCouncilOutput: workspace appears in the appendix only when present', () => {
  const withWorkspace = formatCouncilOutput({
    status: 'ok',
    members: [],
    summary: '',
    turns: [],
    conversation_id: 'conv_2',
    cost_usd: 1.5,
    workspace: { copy: 'C:\\copy', diff: '', files: [] },
  }, { includeDiff: true });
  assert.deepEqual(JSON.parse(withWorkspace.at(-1)), {
    conversation_id: 'conv_2',
    cost_usd: 1.5,
    status: 'ok',
    workspace: 'C:\\copy',
  });
});

test('formatCouncilOutput: every section ends with a newline and the appendix is last', () => {
  const sections = formatCouncilOutput({
    status: 'ok',
    members: ['a'],
    summary: 's',
    turns: [{ agent_id: 'a', model: 'm', outcome: 'ok', text: 't', cost_usd: 0 }],
  });
  for (const section of sections) assert.ok(section.endsWith('\n'));
  assert.doesNotThrow(() => JSON.parse(sections.at(-1)));
});

test('formatCouncilOutput: pathological text still yields blocks at or below 8 KiB', () => {
  const huge = 'x'.repeat(20_000);
  const sections = formatCouncilOutput({
    status: 'ok',
    members: ['a'],
    summary: huge,
    turns: [
      { agent_id: 'a', model: 'm', round: 1, outcome: 'ok', text: huge, cost_usd: 0 },
      { agent_id: 'a', model: 'm', round: 2, outcome: 'ok', text: huge, cost_usd: 0 },
    ],
    workspace: { copy: 'C:\\copy', diff: huge, files: [] },
  }, { includeDiff: true });

  for (const section of sections) {
    assert.ok(
      Buffer.byteLength(section, 'utf8') <= MAX_OUTPUT_BLOCK_BYTES,
      `block exceeds 8 KiB: ${Buffer.byteLength(section, 'utf8')} bytes`,
    );
  }
});
