import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { resolveCouncilExecutable } from './run.js';

test('resolveCouncilExecutable: bare name resolves through PATH, not cwd', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'council-path-'));
  const name = process.platform === 'win32' ? 'council.exe' : 'council';
  const exe = path.join(dir, name);
  fs.writeFileSync(exe, '');
  if (process.platform !== 'win32') fs.chmodSync(exe, 0o755);

  const got = resolveCouncilExecutable(name, process.platform, {
    PATH: dir,
    PATHEXT: '.EXE',
  });
  assert.equal(got, exe);

  // 找不到时保持原样，让 spawn 报 ENOENT。
  assert.equal(resolveCouncilExecutable('no-such-council-xyz', process.platform, { PATH: dir }), 'no-such-council-xyz');
});

test('resolveCouncilExecutable: paths with separators are left alone', () => {
  assert.equal(resolveCouncilExecutable('./council', 'linux', { PATH: '/tmp' }), './council');
  assert.equal(resolveCouncilExecutable('C:\\tools\\council.exe', 'win32', { PATH: '' }), 'C:\\tools\\council.exe');
});
