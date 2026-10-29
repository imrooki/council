'use strict';

// 右键菜单的语义规则（纯函数），见 lib/contextmenu.js。

const test = require('node:test');
const assert = require('node:assert/strict');

const { buildContextMenuItems, menuItemsToTemplate } = require('../lib/contextmenu');

test('输入框里有剪切/复制/粘贴/全选', () => {
  const items = buildContextMenuItems({ editable: true });
  const ids = items.map((i) => i.id);
  assert.deepEqual(ids, ['cut', 'copy', 'paste', 'selectAll']);
});

test('选中文字时给复制，不给剪切粘贴', () => {
  const items = buildContextMenuItems({ editable: false, selectionText: 'hello' });
  assert.deepEqual(items.map((i) => i.id), ['copy', 'selectAll']);
});

test('拖出来的空白没有菜单（桌面应用不该什么都弹）', () => {
  assert.deepEqual(buildContextMenuItems({ editable: false, selectionText: '   ' }), []);
  assert.deepEqual(buildContextMenuItems({}), []);
});

test('链接上多给两项，且不会出现刷新/检查/另存为', () => {
  const items = buildContextMenuItems({ editable: false, linkURL: 'https://example.com/x' });
  const ids = items.map((i) => i.id);
  // 只有链接时不需要分隔线（第一项就是它）。
  assert.deepEqual(ids, ['copyLink', 'openLink']);
  // 这三样是「这是网页」的自证，一个都不许出现。
  const joined = JSON.stringify(items);
  for (const banned of ['reload', 'inspect', 'saveAs', 'Save as', '重新加载', '检查']) {
    assert.equal(joined.includes(banned), false, `菜单里不该有 ${banned}`);
  }
});

test('选中文字再指向链接：中间有分隔线', () => {
  const items = buildContextMenuItems({
    editable: false,
    selectionText: 'hello',
    linkURL: 'https://example.com/y',
  });
  assert.deepEqual(items.map((i) => i.id), ['copy', 'selectAll', 'sep-1', 'copyLink', 'openLink']);
});

test('menuItemsToTemplate 把语义项翻成可点击模板', () => {
  const copied = [];
  const opened = [];
  const items = buildContextMenuItems({ editable: true, linkURL: 'https://example.com/a' });
  const template = menuItemsToTemplate(items, {
    copyText: (t) => copied.push(t),
    openExternal: (u) => opened.push(u),
  });

  const roles = template.filter((t) => t.role).map((t) => t.role);
  assert.deepEqual(roles, ['cut', 'copy', 'paste', 'selectAll']);

  const copyLink = template.find((t) => t.label === '复制链接地址');
  const openLink = template.find((t) => t.label === '用系统浏览器打开');
  assert.ok(copyLink && openLink, '链接相关的两项要在模板里');
  copyLink.click();
  openLink.click();
  assert.deepEqual(copied, ['https://example.com/a']);
  assert.deepEqual(opened, ['https://example.com/a']);
});
