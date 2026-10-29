'use strict';

// 网页痕迹的拦截规则：哪些按键与滚轮在桌面应用里不该有浏览器语义。
//
// 单独抽成纯函数是为了能测：真窗口里按一次 F5 是否刷新，只能靠 CDP 观察；
// 而「F5 该不该被挡」这种判断可以逐条写成用例，改规则时立刻知道影响了谁。
//
// 三条硬规则：
//   1. 刷新（F5 / Ctrl+R / Ctrl+Shift+R）—— 桌面应用没有「刷新页面」这回事；
//   2. 开发者工具（F12 / Ctrl+Shift+I/J/C）—— 用户不该看到 DevTools；
//   3. 网页专属的打印、查看源码、页面缩放。
//
// 明确**不**挡：Ctrl+C/V/X/A/Z/Y（编辑）、Ctrl+F（查找，Chromium 内建）、
// Ctrl+W（关闭窗口，由我们自己的菜单语义处理）、Ctrl+滚轮（由 zoom 上限挡）。

// BLOCK_REASONS 是原因标签，只进日志，便于回查「为什么这次按键没反应」。
const BLOCK_REASONS = Object.freeze({
  reload: 'reload',
  devtools: 'devtools',
  print: 'print',
  viewSource: 'view-source',
  zoom: 'page-zoom',
});

// normalizeKey 把 Electron 的 input.key 折成大写形式，便于比较。
function normalizeKey(key) {
  if (typeof key !== 'string') return '';
  if (key.length === 1) return key.toUpperCase();
  return key.toUpperCase();
}

// isBlockedShortcut 判断一次键盘输入该不该被拦。
//
// input 是 Electron 的 Input 对象里我们关心的那几位：
//   { type: 'keyDown' | 'keyUp' | 'char', key, code, control, shift, alt, meta }
// 非 keyDown 一律放行（按键抬起与字符输入不该被重复处理）。
function isBlockedShortcut(input) {
  if (!input || input.type !== 'keyDown') return '';
  const key = normalizeKey(input.key);
  const code = typeof input.code === 'string' ? input.code : '';
  const ctrl = Boolean(input.control);
  const shift = Boolean(input.shift);
  const alt = Boolean(input.alt);

  // F5 / Ctrl+R / Ctrl+Shift+R：刷新。
  if (key === 'F5' || (ctrl && (key === 'R' || code === 'KeyR'))) return BLOCK_REASONS.reload;
  // F12 / Ctrl+Shift+I|J|C：开发者工具。
  if (key === 'F12') return BLOCK_REASONS.devtools;
  if (ctrl && shift && (key === 'I' || key === 'J' || key === 'C')) return BLOCK_REASONS.devtools;
  // Ctrl+P：打印对话框。
  if (ctrl && !shift && (key === 'P' || code === 'KeyP')) return BLOCK_REASONS.print;
  // Ctrl+U：查看源码。
  if (ctrl && !shift && (key === 'U' || code === 'KeyU')) return BLOCK_REASONS.viewSource;
  // Ctrl +/-/0 与 Ctrl+Shift+'+'：页面缩放。Ctrl+0 在原生应用里通常是「重置缩放」，
  // 我们没有缩放，所以一起挡掉。
  if (ctrl && (key === '=' || key === '+' || key === '-' || key === '_' || key === '0')) {
    return BLOCK_REASONS.zoom;
  }
  // 明确放行 Alt（菜单键位）与编辑类组合：不在上面命中的都不动。
  void alt;
  return '';
}

// shouldBlockWheelZoom 判断一次滚轮是否构成「页面缩放」（Ctrl + 滚轮）。
function shouldBlockWheelZoom(input) {
  return Boolean(input && input.control);
}

module.exports = { BLOCK_REASONS, isBlockedShortcut, shouldBlockWheelZoom };
