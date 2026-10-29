'use strict';

// 原生右键菜单的模板（纯函数，能直接用 node --test 覆盖）。
//
// 为什么要自己画：Electron 默认不给右键菜单，而 Chromium 的默认菜单会带
// 「重新加载 / 另存为 / 检查」—— 那三样是「这是个网页」最直白的自证。
// 桌面应用该给的是：剪切/复制/粘贴、复制链接、用系统浏览器打开。
//
// 只放行「对当前上下文有意义」的项：没有选中文字就不给复制，输入框里才给
// 剪切粘贴，链接才给「复制链接地址」。

// buildContextMenuItems 返回一组语义项（不是 Electron 的 Menu 对象），
// 由调用方翻译成真的模板。fields:
//   { editable, selectionText, linkURL, isDev }
function buildContextMenuItems(fields) {
  const f = fields || {};
  const hasSelection = Boolean(f.selectionText && String(f.selectionText).trim());
  const items = [];

  if (f.editable) {
    items.push({ id: 'cut', role: 'cut', label: '剪切' });
    items.push({ id: 'copy', role: 'copy', label: '复制' });
    items.push({ id: 'paste', role: 'paste', label: '粘贴' });
    items.push({ id: 'selectAll', role: 'selectAll', label: '全选' });
  } else if (hasSelection) {
    items.push({ id: 'copy', role: 'copy', label: '复制' });
    items.push({ id: 'selectAll', role: 'selectAll', label: '全选' });
  }

  if (f.linkURL) {
    if (items.length > 0) items.push({ id: 'sep-1', type: 'separator' });
    items.push({ id: 'copyLink', label: '复制链接地址', copyLinkURL: f.linkURL });
    items.push({ id: 'openLink', label: '用系统浏览器打开', openExternal: f.linkURL });
  }

  // 什么都没有可做时（点在空白处）：给一个「全选」都不合适 —— 桌面应用在
  // 空白处右键通常什么都不弹。返回空数组，调用方就不弹菜单。
  return items;
}

// menuItemsToTemplate 把语义项翻成 Electron 的 Menu 模板。
//
// openExternal 走调用方注入的 opener，菜单本身不直接碰 shell（便于测试）。
function menuItemsToTemplate(items, handlers) {
  const h = handlers || {};
  const template = [];
  for (const item of items || []) {
    if (item.type === 'separator') {
      template.push({ type: 'separator' });
      continue;
    }
    if (item.role) {
      template.push({ role: item.role, label: item.label });
      continue;
    }
    if (item.copyLinkURL) {
      const url = item.copyLinkURL;
      template.push({
        label: item.label,
        click: () => {
          if (typeof h.copyText === 'function') h.copyText(url);
        },
      });
      continue;
    }
    if (item.openExternal) {
      const url = item.openExternal;
      template.push({
        label: item.label,
        click: () => {
          if (typeof h.openExternal === 'function') h.openExternal(url);
        },
      });
    }
  }
  return template;
}

module.exports = { buildContextMenuItems, menuItemsToTemplate };
