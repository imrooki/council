'use strict';

// 目前没有需要主进程参与的功能，工作台仍然是一个普通的本地网页。
// 这里只暴露一个只读标记，方便以后前端区分「桌面壳」与浏览器，不开放任何 IPC。
const { contextBridge } = require('electron');

contextBridge.exposeInMainWorld('councilDesktop', Object.freeze({
  isDesktop: true,
  platform: process.platform,
}));
