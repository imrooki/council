'use strict';

// `node --test desktop/test/`
//
// 这两条规则决定「窗口里还有没有网页痕迹」，所以反例要写实。

const test = require('node:test');
const assert = require('node:assert/strict');

const { BLOCK_REASONS, isBlockedShortcut, shouldBlockWheelZoom } = require('../lib/webguard');

function key(k, mods) {
  return Object.assign({ type: 'keyDown', key: k, code: '' }, mods || {});
}

test('刷新类按键被拦（F5 / Ctrl+R / Ctrl+Shift+R）', () => {
  assert.equal(isBlockedShortcut(key('F5')), BLOCK_REASONS.reload);
  assert.equal(isBlockedShortcut(key('r', { control: true })), BLOCK_REASONS.reload);
  assert.equal(isBlockedShortcut(key('R', { control: true, shift: true })), BLOCK_REASONS.reload);
});

test('开发者工具被拦（F12 / Ctrl+Shift+I J C）', () => {
  assert.equal(isBlockedShortcut(key('F12')), BLOCK_REASONS.devtools);
  for (const k of ['I', 'J', 'C']) {
    assert.equal(
      isBlockedShortcut(key(k, { control: true, shift: true })),
      BLOCK_REASONS.devtools,
      `Ctrl+Shift+${k} 应被拦`,
    );
  }
});

test('打印 / 查看源码 / 页面缩放被拦', () => {
  assert.equal(isBlockedShortcut(key('p', { control: true })), BLOCK_REASONS.print);
  assert.equal(isBlockedShortcut(key('u', { control: true })), BLOCK_REASONS.viewSource);
  for (const k of ['=', '+', '-', '_', '0']) {
    assert.equal(
      isBlockedShortcut(key(k, { control: true })),
      BLOCK_REASONS.zoom,
      `Ctrl+${k} 是页面缩放，应被拦`,
    );
  }
});

test('编辑类与导航按键放行', () => {
  const allowed = [
    key('c', { control: true }),
    key('v', { control: true }),
    key('x', { control: true }),
    key('a', { control: true }),
    key('z', { control: true }),
    key('y', { control: true }),
    key('f', { control: true }), // 查找交给 Chromium 内建，不算网页痕迹
    key('Tab'),
    key('Escape'),
    key('Enter'),
  ];
  for (const input of allowed) {
    assert.equal(isBlockedShortcut(input), '', `${input.key} 不该被拦`);
  }
  // Ctrl+C 是编辑，Ctrl+Shift+C 是 DevTools —— 必须分得开。
  assert.equal(isBlockedShortcut(key('C', { control: true, shift: true })), BLOCK_REASONS.devtools);
  assert.equal(isBlockedShortcut(key('C', { control: true })), '');
});

test('只有 keyDown 参与判断，抬起与字符输入一律放行', () => {
  assert.equal(isBlockedShortcut({ type: 'keyUp', key: 'F5' }), '');
  assert.equal(isBlockedShortcut({ type: 'char', key: 'r', control: true }), '');
  assert.equal(isBlockedShortcut(null), '');
});

test('Ctrl+滚轮才算缩放，普通滚轮放行', () => {
  assert.equal(shouldBlockWheelZoom({ control: true }), true);
  assert.equal(shouldBlockWheelZoom({ control: false }), false);
  assert.equal(shouldBlockWheelZoom(undefined), false);
});
