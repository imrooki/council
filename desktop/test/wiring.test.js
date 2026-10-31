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

test('B15：只读看板与未知版本不再静默 attach', () => {
  assert.match(mainSource, /health\.writable !== true/, '只读看板要用 writable !== true 判断，不能等 omitempty 的 false');
  assert.match(mainSource, /local === '\(未知\)'/, 'engineVersion 探测失败时不能当成版本不同');
  const exeStart = mainSource.indexOf('function resolveCouncilExeSync');
  assert.ok(exeStart > 0);
  const exeBlock = mainSource.slice(exeStart, mainSource.indexOf('\nfunction ', exeStart + 10));
  assert.match(exeBlock, /COUNCIL_EXE/, '同步定位也要认 COUNCIL_EXE');
  const attachStart = mainSource.indexOf('if (target.attach)');
  assert.ok(attachStart > 0);
  const attachBlock = mainSource.slice(attachStart, mainSource.indexOf('\n  }', attachStart));
  assert.match(attachBlock, /attachVersionMatches\(target\.attach\.health/, 'primary-retry 的 attach 也要做版本检查');
});

test('B17：便携版不注册协议、不启用更新器', () => {
  const regStart = mainSource.indexOf('function registerProtocol');
  assert.ok(regStart > 0);
  const regBlock = mainSource.slice(regStart, mainSource.indexOf('\n}', regStart));
  assert.match(regBlock, /PORTABLE_EXECUTABLE_DIR/, '便携版应在 registerProtocol 里直接跳过');
  const updStart = mainSource.indexOf('function updatesSupported');
  assert.ok(updStart > 0);
  const updBlock = mainSource.slice(updStart, mainSource.indexOf('\n}', updStart));
  assert.match(updBlock, /PORTABLE_EXECUTABLE_DIR/, '便携版应禁用 updater');
});

test('P2: serve URL waits for a whole line and saved bounds need 80px visible', () => {
  assert.ok(
    mainSource.includes('(?:\\r?\\n|$)'),
    'URL regex must not accept a chunk split in the middle of the port',
  );
  assert.match(mainSource, /bounds\.y \+ bounds\.height > area\.y \+ 80/, 'need 80px of the window visible');
  assert.doesNotMatch(mainSource, /bounds\.y \+ bounds\.height > area\.y \+ 40/, '40px is not enough');
});
