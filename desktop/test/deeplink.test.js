'use strict';

// 用 node --test 跑：`node --test desktop/test/`
//
// 这两条解析是安全边界（结果会被拼进窗口地址），所以反例比正例多。

const test = require('node:test');
const assert = require('node:assert/strict');

const { incomingDeepLink, conversationIDFromDeepLink } = require('../lib/deeplink');

test('incomingDeepLink 从 argv 里挑出 council://', () => {
  assert.equal(
    incomingDeepLink(['C:\\Council\\Council.exe', 'council://conversation/abc123']),
    'council://conversation/abc123',
  );
  assert.equal(incomingDeepLink(['council://CONVERSATION/abc']), 'council://CONVERSATION/abc');
  assert.equal(incomingDeepLink(['C:\\Council\\Council.exe', '--flag']), '');
  assert.equal(incomingDeepLink([]), '');
  assert.equal(incomingDeepLink(undefined), '');
});

test('conversationIDFromDeepLink 只认 council://conversation/<id>', () => {
  assert.equal(conversationIDFromDeepLink('council://conversation/abc123'), 'abc123');
  assert.equal(conversationIDFromDeepLink('council://conversation/a-b_c'), 'a-b_c');
  // 大小写不敏感的主机名（Windows 上用户可能手打大写）。
  assert.equal(conversationIDFromDeepLink('council://CONVERSATION/abc123'), 'abc123');
});

test('conversationIDFromDeepLink 忽略查询串与片段', () => {
  // 从聊天软件里复制出来的链接常带 utm 之类的尾巴；它不影响去哪个会话，
  // 所以这里放行而不是把人挡在外面。
  assert.equal(conversationIDFromDeepLink('council://conversation/abc123?utm=x'), 'abc123');
  assert.equal(conversationIDFromDeepLink('council://conversation/abc123#top'), 'abc123');
});

test('conversationIDFromDeepLink 拒绝一切别的形状', () => {
  const rejected = [
    'council://conversation/../etc/passwd', // 路径穿越
    'council://conversation/abc/extra', // 多段路径
    'council://conversation/', // 空 id
    'council://conversation/abc%2Fdef', // 编码过的分隔符
    'council://settings/abc', // 别的主机
    'council:///conversation/abc', // 没有主机
    'https://conversation/abc', // 别的协议
    'council:conversation/abc', // 少了 //
    'not a url',
    '',
  ];
  for (const raw of rejected) {
    assert.equal(conversationIDFromDeepLink(raw), '', `应当拒绝: ${raw}`);
  }
});
