'use strict';

// 工作台页面不需要主进程参与，工作台仍然是一个普通的本地网页：这里只暴露
// 一个只读标记，方便前端区分「桌面壳」与浏览器。
//
// 唯一的例外是首次运行的设置向导（setup.html）：它需要问主进程「本机装了
// 哪些 CLI」「把勾选结果写成配置」。这两件事只在小窗口里、通过两个具名通道
// 走，且都在主进程侧做了参数校验 —— 工作台页面拿不到任何 IPC。
const { contextBridge, ipcRenderer } = require('electron');

const isSetupPage = window.location.pathname.endsWith('/setup.html');

contextBridge.exposeInMainWorld('councilDesktop', Object.freeze({
  isDesktop: true,
  platform: process.platform,
}));

if (isSetupPage) {
  contextBridge.exposeInMainWorld('councilSetup', Object.freeze({
    // detect → 主进程跑 `council init --json`，返回检测结果（不写文件）。
    detect: () => ipcRenderer.invoke('setup:detect'),
    // apply → 主进程跑 `council init --apply -`，把勾选写进配置。
    apply: (payload) => ipcRenderer.invoke('setup:apply', payload),
  }));
}
