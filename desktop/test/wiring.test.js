'use strict';

// 源码级断言：这几条「网页痕迹」的修法都在 main.js 里，行为只能靠 CDP 观察，
// 但「接线有没有被删掉/改回去」可以在这里当场发现。
//
// 只读文本，不做任何解析 —— 目的是防回退（有人把 prefers-color-scheme 的
// 加载页改回写死暗色、把原生菜单改回 setApplicationMenu(null)）。

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const mainSource = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');
const setupSource = fs.readFileSync(path.join(__dirname, '..', 'setup.html'), 'utf8');

test('加载页跟随系统亮暗，不再写死暗色', () => {
  const start = mainSource.indexOf('function loadingDataUrl');
  assert.ok(start > 0, '找不到 loadingDataUrl');
  const block = mainSource.slice(start, mainSource.indexOf('return `data:text/html', start));
  assert.match(block, /prefers-color-scheme:\s*dark/, '加载页要有暗色媒体查询');
  assert.match(block, /--bg:\s*#f9fafb/, '加载页的默认底色要和工作台一致（#f9fafb）');
  assert.doesNotMatch(block, /color-scheme:\s*dark;\s*\}/, '加载页不该把 color-scheme 写死成 dark');
});

test('窗口首帧底色跟随系统', () => {
  assert.match(mainSource, /nativeTheme\.shouldUseDarkColors\s*\?\s*'#0b0c0e'\s*:\s*'#f9fafb'/);
  assert.match(mainSource, /nativeTheme\.on\('updated'/, '系统主题变化要跟着换底色');
});

test('键盘与右键的守卫都接着', () => {
  assert.match(mainSource, /isBlockedShortcut\(input\)/, 'before-input-event 要调拦截规则');
  assert.match(mainSource, /setVisualZoomLevelLimits\(1,\s*1\)/, '视觉缩放要锁死');
  assert.match(mainSource, /on\('context-menu'/, '要有原生右键菜单');
  assert.match(mainSource, /buildContextMenuItems\(/, '右键菜单要用 lib 里的规则');
  // 只匹配「真的调用」：行首、非注释。注释里提到旧写法是允许的。
  assert.doesNotMatch(mainSource, /\n\s*Menu\.setApplicationMenu\(null\);/, '不能再把应用菜单清空');
  assert.match(mainSource, /buildAppMenu\(\)/, 'start() 里要装应用菜单');
});

test('设置向导也跟随系统亮暗', () => {
  assert.match(setupSource, /prefers-color-scheme:\s*dark/, '向导要有暗色媒体查询');
  assert.match(setupSource, /color-scheme:\s*light/, '向导默认是亮色');
});
