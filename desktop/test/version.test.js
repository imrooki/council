'use strict';

// `node --test desktop/test/`
//
// 版本比较是「装到一半」的护栏：旧 serve 还在跑时，桌面版必须能认出它。

const test = require('node:test');
const assert = require('node:assert/strict');

const { parseVersion, versionsMatch } = require('../lib/version');

test('parseVersion 拆出 base 与 git 哈希', () => {
  assert.deepEqual(parseVersion('0.3.0+git.c18b83d'), {
    base: '0.3.0',
    git: 'c18b83d',
    raw: '0.3.0+git.c18b83d',
  });
  // 脏工作区：哈希照取，dirty 只是后缀。
  assert.equal(parseVersion('0.3.0+git.c18b83d.dirty').git, 'c18b83d');
  // 认不出的串：整串当 base，git 为空。
  const odd = parseVersion('0.0.0-dev');
  assert.equal(odd.base, '0.0.0-dev');
  assert.equal(odd.git, '');
  assert.equal(parseVersion(undefined).raw, '');
});

test('versionsMatch 以 git 哈希为准', () => {
  assert.equal(versionsMatch('0.3.0+git.c18b83d', '0.3.0+git.c18b83d'), true);
  assert.equal(versionsMatch('0.3.0+git.c18b83d', '0.3.0+git.c18b83d.dirty'), true);
  assert.equal(versionsMatch('0.3.0+git.c18b83d', '0.3.0+git.9e5b3c9'), false);
  // 版本号升了但哈希不同：仍然是不同构建。
  assert.equal(versionsMatch('0.2.0+git.9e5b3c9', '0.3.0+git.c18b83d'), false);
});

test('versionsMatch 在没有哈希时退回 base 版本', () => {
  assert.equal(versionsMatch('0.3.0', '0.3.0'), true);
  assert.equal(versionsMatch('0.3.0', '0.2.0'), false);
  // 一边认不出（空串）：不拦人。
  assert.equal(versionsMatch('', '0.3.0+git.c18b83d'), true);
  assert.equal(versionsMatch('0.3.0+git.c18b83d', ''), true);
});
