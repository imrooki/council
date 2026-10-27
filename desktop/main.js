'use strict';

// Council 桌面外壳：先找已经在跑的工作台，找不到才自己拉起 `council serve -rw`。
// 所有业务仍然在 Go 的 HTTP 服务里，Electron 这边不读库、不跑议会，
// 也不替用户登录任何东西。

const { app, BrowserWindow, Menu, dialog, shell } = require('electron');
const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const net = require('node:net');
const path = require('node:path');
const { URL } = require('node:url');

const APP_TITLE = 'Council';
const DEFAULT_PRIMARY_PORT = 8765;
const PRIMARY_PORT = resolvePrimaryPort();
const PRIMARY_BASE_URL = `http://127.0.0.1:${PRIMARY_PORT}`;
const CONFIGURED_BASE_URL = normalizeBaseUrl(process.env.COUNCIL_URL || '');
const DEFAULT_COUNCIL_EXE = '%USERPROFILE%\\bin\\council.exe';
const HEALTH_TIMEOUT_MS = 1500;
const READY_TIMEOUT_MS = 20000;
const POLL_INTERVAL_MS = 400;
const OUTPUT_TAIL_CHARS = 64 * 1024;
const DIALOG_TAIL_CHARS = 6000;

const repoRoot = path.resolve(__dirname, '..');

let mainWindow = null;
let baseUrl = null;
let lastExeCandidates = [];
let connectionMode = null;
let pageTitle = APP_TITLE;
let councilProcess = null;
let councilOwned = false;
let councilExit = null;
let councilSpawnError = null;
let councilStdoutTail = '';
let councilStderrTail = '';
let spawnedInfo = null;
let startupFinished = false;
let startupFailed = false;
let quitting = false;

class StartupFailure extends Error {
  constructor(title, message) {
    super(message);
    this.name = 'StartupFailure';
    this.title = title;
  }
}

function resolvePrimaryPort() {
  const raw = process.env.COUNCIL_DESKTOP_PORT;
  if (!raw) return DEFAULT_PRIMARY_PORT;
  const port = Number(raw);
  if (Number.isInteger(port) && port > 0 && port <= 65535) return port;
  return DEFAULT_PRIMARY_PORT;
}

function normalizeBaseUrl(value) {
  try {
    const parsed = new URL(String(value));
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return null;
    }
    return parsed.href.replace(/\/+$/, '') || parsed.origin;
  } catch {
    return null;
  }
}

function isHttpUrl(value) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
}

function isSameAppOrigin(value) {
  if (!baseUrl || !value) return false;
  try {
    const target = new URL(value);
    const appUrl = new URL(baseUrl);
    return target.protocol === appUrl.protocol
      && target.hostname === appUrl.hostname
      && target.port === appUrl.port;
  } catch {
    return false;
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function appendTail(current, chunk, maxChars) {
  const next = current + chunk;
  return next.length > maxChars ? next.slice(-maxChars) : next;
}

function childLogBlock(maxChars = DIALOG_TAIL_CHARS) {
  const sections = [];
  if (councilStderrTail.trim()) {
    sections.push(`stderr（末尾）:\n${councilStderrTail.trim()}`);
  }
  if (councilStdoutTail.trim()) {
    sections.push(`stdout（末尾）:\n${councilStdoutTail.trim()}`);
  }
  const text = sections.join('\n\n');
  return text.length > maxChars ? text.slice(-maxChars) : text;
}

// 启动决策写进用户数据目录，打包后没有控制台也能查 attach / 自启。
function logEvent(message) {
  try {
    const dir = app.getPath('userData');
    fs.mkdirSync(dir, { recursive: true });
    fs.appendFileSync(path.join(dir, 'desktop.log'), `${new Date().toISOString()} ${message}\n`, 'utf8');
  } catch {
    /* 日志失败不能影响启动。 */
  }
  try {
    console.log(`[council-desktop] ${message}`);
  } catch {
    /* 打包后没有控制台。 */
  }
}

function probeHealth(target, timeoutMs = HEALTH_TIMEOUT_MS) {
  return new Promise((resolve) => {
    if (!target) {
      resolve({ ok: false, reason: '地址为空' });
      return;
    }

    let finished = false;
    let request = null;
    let timer = null;

    const finish = (result) => {
      if (finished) return;
      finished = true;
      if (timer) clearTimeout(timer);
      resolve(result);
    };

    const effectiveTimeout = Math.max(100, Number(timeoutMs) || HEALTH_TIMEOUT_MS);
    try {
      const healthUrl = new URL('/api/health', target);
      const transport = healthUrl.protocol === 'https:' ? https : http;

      timer = setTimeout(() => {
        finish({ ok: false, reason: 'timeout' });
        if (request) request.destroy();
      }, effectiveTimeout);

      request = transport.get(healthUrl, {
        headers: { Accept: 'application/json' },
      }, (response) => {
        let body = '';
        response.setEncoding('utf8');
        response.on('data', (chunk) => {
          if (body.length < 64 * 1024) body += chunk;
        });
        response.on('end', () => {
          if (response.statusCode !== 200) {
            finish({ ok: false, reason: `HTTP ${response.statusCode}` });
            return;
          }
          try {
            const json = JSON.parse(body || '{}');
            if (!json || json.ok !== true) {
              finish({ ok: false, reason: 'health 没有返回 ok=true' });
              return;
            }
            finish({ ok: true, health: json });
          } catch {
            finish({ ok: false, reason: 'health 返回的不是 JSON' });
          }
        });
        response.on('error', (error) => finish({ ok: false, reason: error.message }));
      });

      request.setTimeout(effectiveTimeout, () => {
        request.destroy(new Error('timeout'));
      });
      request.on('error', (error) => {
        finish({ ok: false, reason: error.code || error.message });
      });
    } catch (error) {
      finish({ ok: false, reason: error.message });
    }
  });
}

// 只判断端口有没有人监听。超时也按「被占用」处理：宁可多试一个随机端口，
// 也不要和别人的服务抢 8765。
function isPortOpen(port, timeoutMs = 400) {
  return new Promise((resolve) => {
    const socket = net.connect({ host: '127.0.0.1', port });
    let finished = false;

    const finish = (value) => {
      if (finished) return;
      finished = true;
      socket.destroy();
      resolve(value);
    };

    socket.setTimeout(timeoutMs);
    socket.once('connect', () => finish(true));
    socket.once('timeout', () => finish(true));
    socket.once('error', (error) => {
      finish(Boolean(error && error.code !== 'ECONNREFUSED'));
    });
  });
}

function resolveCouncilExe() {
  const equalsArg = process.argv.find((arg) => arg.startsWith('--council-exe='));
  if (equalsArg) return path.resolve(equalsArg.slice('--council-exe='.length));

  const flagIndex = process.argv.indexOf('--council-exe');
  if (flagIndex !== -1 && process.argv[flagIndex + 1]) {
    return path.resolve(process.argv[flagIndex + 1]);
  }

  if (process.env.COUNCIL_EXE) return path.resolve(process.env.COUNCIL_EXE);

  // 打包后 council.exe 的位置取决于 extraResources 的映射方式与 portable 的自解压布局。
  // 与其猜一种，不如把已知的几种都试一遍——找不到时报错会把候选全列出来，
  // 下次一眼就能看出是「没打进去」还是「打进去了但路径不同」。
  const candidates = [];
  if (app.isPackaged) {
    candidates.push(path.join(process.resourcesPath, 'council.exe'));
    candidates.push(path.join(path.dirname(process.execPath), 'council.exe'));
    candidates.push(path.join(path.dirname(process.execPath), 'resources', 'council.exe'));
    const portableDir = process.env.PORTABLE_EXECUTABLE_DIR;
    if (portableDir) {
      candidates.push(path.join(portableDir, 'council.exe'));
      candidates.push(path.join(portableDir, 'resources', 'council.exe'));
    }
  } else {
    candidates.push(DEFAULT_COUNCIL_EXE);
    candidates.push(path.join(repoRoot, 'council.exe'));
  }
  for (const candidate of candidates) {
    if (fs.existsSync(candidate) && fs.statSync(candidate).isFile()) return candidate;
  }
  lastExeCandidates = candidates;
  return candidates[0];
}

function resolveCouncilCwd() {
  if (process.env.COUNCIL_CWD) return path.resolve(process.env.COUNCIL_CWD);
  if (!app.isPackaged) return repoRoot;
  // portable 包由 NSIS 自解压，进程 cwd 不一定是 exe 所在目录；
  // electron-builder 会给出 PORTABLE_EXECUTABLE_DIR。
  if (process.env.PORTABLE_EXECUTABLE_DIR) return process.env.PORTABLE_EXECUTABLE_DIR;
  // 安装版从开始菜单/桌面快捷方式启动时 cwd 可能是 System32，不要用它。
  return path.dirname(process.execPath);
}

function councilArgs(port) {
  const args = ['serve', '-rw'];
  // 8765 是 council 的默认端口；其它端口必须显式给 -addr。
  // 传 0 让操作系统分配一个空闲端口（8765 被别的程序占用时走这里）。
  if (port !== DEFAULT_PRIMARY_PORT) {
    args.push('-addr', `127.0.0.1:${port}`);
  }
  return args;
}

function startCouncil(port) {
  const exe = resolveCouncilExe();
  const cwd = resolveCouncilCwd();
  const args = councilArgs(port);

  if (!fs.existsSync(exe) || !fs.statSync(exe).isFile()) {
    const tried = lastExeCandidates.length ? lastExeCandidates : [exe];
    throw new StartupFailure(
      '找不到 council.exe',
      `期望路径：${exe}\n\n`
      + `已尝试：\n${tried.map((item) => `- ${item}`).join('\n')}\n\n`
      + '开发时可以用 --council-exe <路径> 指定，或设置环境变量 COUNCIL_EXE。\n'
      + '打包时会从 desktop/vendor/council.exe 复制到 resources/council.exe。',
    );
  }

  try {
    if (!fs.statSync(cwd).isDirectory()) throw new Error('不是目录');
  } catch {
    throw new StartupFailure(
      '工作目录不可用',
      `council 的工作目录不存在或不是目录：${cwd}\n\n请检查 COUNCIL_CWD 环境变量。`,
    );
  }

  let child;
  try {
    child = spawn(exe, args, {
      cwd,
      detached: false,
      windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
    });
  } catch (error) {
    throw new StartupFailure(
      'council 启动失败',
      `无法启动：${exe}\n参数：${args.join(' ')}\n工作目录：${cwd}\n\n${error.message}`,
    );
  }

  councilProcess = child;
  councilOwned = true;
  councilExit = null;
  councilSpawnError = null;
  councilStdoutTail = '';
  councilStderrTail = '';
  spawnedInfo = {
    exe,
    cwd,
    args,
    port,
    pid: child.pid,
    url: port === 0 ? null : `http://127.0.0.1:${port}`,
  };

  logEvent(`spawn command="${exe}" ${args.join(' ')} cwd="${cwd}" pid=${child.pid}`);

  child.stdout.setEncoding('utf8');
  child.stderr.setEncoding('utf8');
  child.stdout.on('data', (chunk) => {
    councilStdoutTail = appendTail(councilStdoutTail, chunk, OUTPUT_TAIL_CHARS);
    if (!spawnedInfo.url) {
      const match = councilStdoutTail.match(/council serve:\s+(https?:\/\/[^\s]+)/);
      if (match) {
        spawnedInfo.url = match[1];
        logEvent(`spawn announced url=${match[1]}`);
      }
    }
  });
  child.stderr.on('data', (chunk) => {
    councilStderrTail = appendTail(councilStderrTail, chunk, OUTPUT_TAIL_CHARS);
  });

  child.once('error', (error) => {
    councilSpawnError = error;
    logEvent(`spawn error: ${error.message}`);
  });
  child.once('exit', (code, signal) => {
    councilExit = { code, signal };
    logEvent(`council exited code=${code === null ? 'null' : code} signal=${signal || 'none'}`);
  });

  return spawnedInfo;
}

function targetLabel(url) {
  if (!url) {
    if (spawnedInfo && spawnedInfo.port === 0) return '系统分配的空闲端口';
    return `127.0.0.1:${PRIMARY_PORT}`;
  }
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function startupErrorFromExit(exitInfo, lastReason, url) {
  const code = exitInfo.code === null ? `信号 ${exitInfo.signal}` : `退出码 ${exitInfo.code}`;
  const stderr = councilStderrTail;
  const portInUse = /address already in use|Only one usage of each socket address|bind:\s|监听 .* 失败|listen tcp/i.test(stderr);
  const lockBusy = /council\.lock|另一个 council|已经在运行|无法获取.*锁|database is locked/i.test(stderr);
  const command = spawnedInfo
    ? `\n\n命令：${spawnedInfo.exe} ${spawnedInfo.args.join(' ')}`
      + `\n工作目录：${spawnedInfo.cwd}`
    : '';

  if (portInUse) {
    return new StartupFailure(
      'Council 启动失败：端口被占用',
      `无法在 ${targetLabel(url)} 上启动 council serve。\n\n`
      + '可能是别的程序占用了这个端口，或另一个 council 已经在运行。\n'
      + `子进程 ${code}，最后一次 health 探测：${lastReason || '无响应'}`
      + command,
    );
  }

  if (lockBusy) {
    return new StartupFailure(
      'Council 启动失败：数据目录被占用',
      '另一个 council 进程正在使用同一个数据目录（council.lock）。\n\n'
      + '先关掉那个 council 或终端里的 council，再打开桌面版。\n'
      + `子进程 ${code}，最后一次 health 探测：${lastReason || '无响应'}`
      + command,
    );
  }

  return new StartupFailure(
    'Council 启动失败',
    `council serve 在就绪前退出（${code}）。\n\n`
    + `目标：${targetLabel(url)}\n`
    + `最后一次 health 探测：${lastReason || '无响应'}`
    + command,
  );
}

async function waitForSpawnedUrl(spawned, timeoutMs = 8000) {
  if (spawned.url) return spawned.url;
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    if (spawned.url) return spawned.url;
    if (councilSpawnError) {
      throw new StartupFailure(
        'council 启动失败',
        `无法执行 ${spawned.exe}：${councilSpawnError.message}`,
      );
    }
    if (councilExit) {
      throw startupErrorFromExit(councilExit, '还没有报告监听端口', null);
    }
    await sleep(100);
  }

  throw new StartupFailure(
    'council 没有报告监听端口',
    `已经启动 council，但 ${timeoutMs / 1000} 秒内没有从 stdout 读到监听地址。\n\n`
    + `命令：${spawned.exe} ${spawned.args.join(' ')}`
    + `\n工作目录：${spawned.cwd}`,
  );
}

async function waitForReady(url, deadline) {
  let lastReason = '';

  while (Date.now() < deadline) {
    const probe = await probeHealth(url, Math.min(HEALTH_TIMEOUT_MS, Math.max(300, deadline - Date.now())));
    if (probe.ok) return probe;

    lastReason = probe.reason || '';
    if (councilOwned) {
      if (councilSpawnError) {
        throw new StartupFailure(
          'council 启动失败',
          `无法执行 ${spawnedInfo ? spawnedInfo.exe : 'council.exe'}：${councilSpawnError.message}`,
        );
      }
      if (councilExit) {
        throw startupErrorFromExit(councilExit, lastReason, url);
      }
    }

    await sleep(POLL_INTERVAL_MS);
  }

  throw new StartupFailure(
    'Council 启动超时',
    `等了 ${READY_TIMEOUT_MS / 1000} 秒，${url}/api/health 仍未就绪。\n\n`
    + `最后一次 health 探测：${lastReason || '无响应'}`,
  );
}

async function selectSpawnTarget() {
  const open = await isPortOpen(PRIMARY_PORT);
  logEvent(`primary port open=${open} port=${PRIMARY_PORT}`);

  if (!open) {
    return { port: PRIMARY_PORT };
  }

  // 端口有人监听但 health 还没通：可能是另一个 council 正在启动。
  // 再给 1.5 秒；仍然不通才当作「被别的程序占用」，退到随机空闲端口。
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await sleep(500);
    const probe = await probeHealth(PRIMARY_BASE_URL, 1000);
    if (probe.ok) {
      return { attach: probe, url: PRIMARY_BASE_URL, source: 'primary-retry' };
    }
  }

  logEvent('primary port occupied by non-workbench; fallback to ephemeral port');
  return { port: 0 };
}

function loadingDataUrl(statusText) {
  const html = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>${APP_TITLE}</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  html, body { height: 100%; margin: 0; }
  body {
    background: #0b0c0e;
    color: #e8eaed;
    font-family: "Segoe UI", "Microsoft YaHei UI", "Microsoft YaHei", sans-serif;
    display: grid;
    place-items: center;
    -webkit-user-select: none;
    user-select: none;
  }
  main { display: flex; flex-direction: column; align-items: center; gap: 18px; }
  .mark { width: 76px; height: 76px; position: relative; }
  .mark .core {
    position: absolute; left: 50%; top: 50%; width: 20px; height: 20px;
    margin: -10px 0 0 -10px; border-radius: 50%; background: #8ab4f8;
    box-shadow: 0 0 28px rgba(138, 180, 248, .45);
  }
  .mark .orbit {
    position: absolute; left: 50%; top: 50%; width: 58px; height: 58px;
    margin: -29px 0 0 -29px; border: 1px solid rgba(138, 180, 248, .28);
    border-radius: 50%;
  }
  .mark .dot {
    position: absolute; width: 7px; height: 7px; border-radius: 50%;
    background: #c6d4f2;
  }
  .mark .dot.a { left: 9px; top: 24px; }
  .mark .dot.b { right: 9px; top: 24px; }
  .mark .dot.c { left: 50%; bottom: 4px; margin-left: -3.5px; }
  h1 { margin: 0; font-size: 20px; font-weight: 600; letter-spacing: .12em; }
  p { margin: 0; color: #9aa0a6; font-size: 13px; min-height: 18px; }
  .spinner {
    width: 18px; height: 18px; border-radius: 50%;
    border: 2px solid rgba(138, 180, 248, .22);
    border-top-color: #8ab4f8;
    animation: spin .8s linear infinite;
  }
  footer { position: fixed; bottom: 22px; color: #5f6368; font-size: 11px; }
  @keyframes spin { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .spinner { animation-duration: 2.4s; } }
</style>
</head>
<body>
<main>
  <div class="mark" aria-hidden="true">
    <div class="orbit"></div>
    <div class="dot a"></div>
    <div class="dot b"></div>
    <div class="dot c"></div>
    <div class="core"></div>
  </div>
  <h1>COUNCIL</h1>
  <p id="status">${escapeHtml(statusText)}</p>
  <div class="spinner" aria-hidden="true"></div>
</main>
<footer>本地工作台 · 数据不离开这台电脑</footer>
</body>
</html>`;
  return `data:text/html;charset=utf-8,${encodeURIComponent(html)}`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function setLoadingStatus(text) {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  const script = `(() => {
    const el = document.getElementById('status');
    if (el) el.textContent = ${JSON.stringify(text)};
  })();`;
  mainWindow.webContents.executeJavaScript(script, true).catch(() => {});
}

function windowIconPath() {
  const candidates = app.isPackaged
    ? [path.join(process.resourcesPath, 'icon.ico')]
    : [path.join(__dirname, 'build', 'icon.ico'), path.join(__dirname, 'assets', 'icon.ico')];
  return candidates.find((candidate) => fs.existsSync(candidate));
}

function modeSuffix() {
  if (!connectionMode) return '';
  const label = connectionMode.kind === 'attach' ? '已连接' : '自启';
  let host = '';
  try {
    host = new URL(connectionMode.url).host;
  } catch {
    host = '';
  }
  return host ? ` · ${label} ${host}` : ` · ${label}`;
}

function updateWindowTitle() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  mainWindow.setTitle(`${pageTitle || APP_TITLE}${modeSuffix()}`);
}

function applyModeMarker() {
  if (!mainWindow || mainWindow.isDestroyed() || !connectionMode) return;
  const script = `(() => {
    document.documentElement.dataset.councilMode = ${JSON.stringify(connectionMode.kind)};
    document.documentElement.dataset.councilUrl = ${JSON.stringify(connectionMode.url)};
    document.documentElement.dataset.councilSource = ${JSON.stringify(connectionMode.source)};
  })();`;
  mainWindow.webContents.executeJavaScript(script, true).catch(() => {});
}

function focusWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function createWindow() {
  const options = {
    width: 1440,
    height: 900,
    minWidth: 980,
    show: false,
    title: APP_TITLE,
    backgroundColor: '#0b0c0e',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: false,
    },
  };
  const icon = windowIconPath();
  if (icon) options.icon = icon;

  mainWindow = new BrowserWindow(options);
  mainWindow.setMenuBarVisibility(false);
  pageTitle = APP_TITLE;

  mainWindow.once('ready-to-show', () => {
    if (!mainWindow || mainWindow.isDestroyed()) return;
    mainWindow.show();
    mainWindow.focus();
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  const contents = mainWindow.webContents;

  // 标题由外壳统一控制，保留页面自己的标题并加上 attach / 自启。
  contents.on('page-title-updated', (event, title) => {
    event.preventDefault();
    const next = String(title || '').trim();
    pageTitle = next || APP_TITLE;
    updateWindowTitle();
  });

  contents.on('did-finish-load', () => {
    applyModeMarker();
    updateWindowTitle();
  });

  contents.setWindowOpenHandler(({ url }) => {
    if (isHttpUrl(url)) {
      shell.openExternal(url).catch(() => {});
    }
    return { action: 'deny' };
  });

  // 页面自己跳走时也拦一下：工作台内部的同源跳转放行，外链交给系统浏览器。
  contents.on('will-navigate', (event, url) => {
    if (isSameAppOrigin(url)) return;
    event.preventDefault();
    if (isHttpUrl(url)) {
      shell.openExternal(url).catch(() => {});
    }
  });

  contents.on('did-fail-load', (_event, errorCode, errorDescription, failedUrl, isMainFrame) => {
    if (!isMainFrame || errorCode === -3 || startupFailed || quitting || !startupFinished) return;
    dialog.showErrorBox(
      '工作台加载失败',
      `无法加载 ${failedUrl}\n\n${errorDescription}（${errorCode}）\n\n`
      + 'council serve 可能已经退出。关闭窗口后重新打开 Council。',
    );
  });

  contents.on('render-process-gone', (_event, details) => {
    if (startupFailed || quitting) return;
    dialog.showErrorBox(
      '工作台渲染进程退出',
      `窗口内容进程已退出：${details.reason}\n\n关闭窗口后重新打开 Council。`,
    );
  });

  mainWindow.loadURL(loadingDataUrl(`正在检查 ${PRIMARY_PORT} 端口…`)).catch(() => {});
}

function isChildAlive() {
  return Boolean(councilOwned && councilProcess && councilProcess.exitCode === null && !councilExit);
}

function stopOwnedChild() {
  const child = councilProcess;
  if (!councilOwned || !child || child.exitCode !== null || councilExit) {
    councilProcess = null;
    return Promise.resolve();
  }

  logEvent(`stopping owned council pid=${child.pid}`);

  return new Promise((resolve) => {
    let done = false;
    let timer = null;

    const finish = () => {
      if (done) return;
      done = true;
      if (timer) clearTimeout(timer);
      if (councilProcess === child) councilProcess = null;
      resolve();
    };

    child.once('exit', finish);

    try {
      child.kill();
    } catch {
      /* 已经退出时 taskkill 也不会成功，下面的兜底会收尾。 */
    }

    // Windows 上 child.kill() 是强制结束；2 秒还活着就用 taskkill 连同这个
    // 子进程自己的进程树一起收掉（只针对我们自己启动的 pid）。
    timer = setTimeout(() => {
      if (child.exitCode === null) {
        try {
          spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], {
            windowsHide: true,
            stdio: 'ignore',
            timeout: 5000,
          });
        } catch {
          /* 尽力而为。 */
        }
      }
      setTimeout(finish, 300);
    }, 2000);
    if (typeof timer.unref === 'function') timer.unref();
  });
}

async function failStartup(error) {
  if (startupFailed || quitting) return;
  startupFailed = true;

  const title = error && error.title ? error.title : 'Council 启动失败';
  let message = error && error.message ? error.message : String(error);
  const childLog = childLogBlock();
  if (childLog) {
    message += `\n\n--- council 子进程输出（末尾） ---\n${childLog}`;
  }
  message += `\n\n主探测地址：${PRIMARY_BASE_URL}`;
  if (baseUrl) message += `\n当前地址：${baseUrl}`;

  logEvent(`failure title="${title}" message="${message.slice(0, 800).replace(/\n/g, ' | ')}"`);

  await stopOwnedChild();
  dialog.showErrorBox(title, message);
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.destroy();
  app.quit();
}

async function loadWorkbench(url, mode, health) {
  if (!mainWindow || mainWindow.isDestroyed()) return;

  baseUrl = url;
  connectionMode = mode;
  updateWindowTitle();
  applyModeMarker();
  logEvent(`mode=${mode.kind} source=${mode.source} url=${url}`);

  setLoadingStatus('正在打开工作台…');
  try {
    await mainWindow.loadURL(url);
    startupFinished = true;
    applyModeMarker();
    updateWindowTitle();
    if (health && health.writable === false) {
      logEvent('attached workbench is read-only');
    }
  } catch (error) {
    if (error && error.code === 'ERR_ABORTED') return;
    throw new StartupFailure(
      '工作台加载失败',
      `无法加载 ${url}\n\n${error.message}`,
    );
  }
}

async function start() {
  Menu.setApplicationMenu(null);
  app.setAppUserModelId('com.council.workbench');

  createWindow();

  logEvent(
    `start packaged=${app.isPackaged} primary=${PRIMARY_BASE_URL}`
    + ` council_url=${CONFIGURED_BASE_URL || '(none)'}`
    + ` cwd="${resolveCouncilCwd()}"`,
  );
  if (process.env.COUNCIL_URL && !CONFIGURED_BASE_URL) {
    logEvent(`COUNCIL_URL invalid, ignored: ${process.env.COUNCIL_URL}`);
  }
  if (process.env.COUNCIL_DESKTOP_PORT && PRIMARY_PORT === DEFAULT_PRIMARY_PORT) {
    logEvent(`COUNCIL_DESKTOP_PORT invalid, using ${DEFAULT_PRIMARY_PORT}`);
  }

  const readyDeadline = Date.now() + READY_TIMEOUT_MS;

  // 1. 永远先看默认端口 8765。用户桌面上已经有一个 serve -rw -app 时，
  //    这里直接 attach，不会再拉一个 council 去抢同一个 data_dir。
  //    冷启动/对方刚开库时单次探测可能误判，所以最多试三次（每次 1.5 秒）。
  setLoadingStatus(`正在检查 ${PRIMARY_BASE_URL} …`);
  let primaryProbe = { ok: false, reason: '无响应' };
  for (let attempt = 0; attempt < 3; attempt += 1) {
    primaryProbe = await probeHealth(PRIMARY_BASE_URL);
    if (primaryProbe.ok) break;
    if (attempt < 2) await sleep(400);
  }

  if (primaryProbe.ok) {
    await loadWorkbench(PRIMARY_BASE_URL, {
      kind: 'attach',
      source: 'primary',
      url: PRIMARY_BASE_URL,
    }, primaryProbe.health);
    return;
  }
  logEvent(`primary probe failed: ${primaryProbe.reason}`);

  if (quitting) return;

  // 2. COUNCIL_URL 只作为额外的 attach 目标，不再决定自启端口。
  if (CONFIGURED_BASE_URL && CONFIGURED_BASE_URL !== PRIMARY_BASE_URL) {
    setLoadingStatus(`正在检查 ${CONFIGURED_BASE_URL} …`);
    const configuredProbe = await probeHealth(CONFIGURED_BASE_URL);
    if (configuredProbe.ok) {
      await loadWorkbench(CONFIGURED_BASE_URL, {
        kind: 'attach',
        source: 'COUNCIL_URL',
        url: CONFIGURED_BASE_URL,
      }, configuredProbe.health);
      return;
    }
    logEvent(`COUNCIL_URL probe failed: ${configuredProbe.reason}`);
  }

  if (quitting) return;

  // 3. 8765 是空闲的就用它；有人监听但不是工作台，就退到系统分配的空闲端口。
  setLoadingStatus('正在确认端口…');
  const target = await selectSpawnTarget();
  if (target.attach) {
    await loadWorkbench(target.url, {
      kind: 'attach',
      source: target.source,
      url: target.url,
    }, target.attach.health);
    return;
  }

  if (quitting) return;
  const port = target.port;
  setLoadingStatus(port === 0
    ? '正在选择空闲端口并启动 council serve -rw …'
    : `正在启动 council serve -rw（${port}）…`);

  let spawned;
  try {
    spawned = startCouncil(port);
  } catch (error) {
    await failStartup(error);
    return;
  }

  if (quitting) {
    await stopOwnedChild();
    return;
  }

  try {
    const url = await waitForSpawnedUrl(spawned);
    baseUrl = url;
    logEvent(`spawn selected port=${port} url=${url} pid=${spawned.pid}`);
    setLoadingStatus('等待工作台就绪…');
    const ready = await waitForReady(url, readyDeadline);
    await loadWorkbench(url, {
      kind: 'spawn',
      source: port === 0
        ? 'fallback-ephemeral'
        : (PRIMARY_PORT === DEFAULT_PRIMARY_PORT ? 'primary' : 'COUNCIL_DESKTOP_PORT'),
      url,
    }, ready.health);
  } catch (error) {
    if (error instanceof StartupFailure && spawnedInfo) {
      error.message += `\n\n命令：${spawnedInfo.exe} ${spawnedInfo.args.join(' ')}`
        + `\n工作目录：${spawnedInfo.cwd}`;
    }
    await failStartup(error);
  }
}

app.setName(APP_TITLE);

const gotSingleInstanceLock = app.requestSingleInstanceLock();

if (!gotSingleInstanceLock) {
  app.quit();
} else {
  app.on('second-instance', () => {
    focusWindow();
  });

  app.whenReady().then(start).catch((error) => {
    failStartup(error).catch(() => app.exit(1));
  });

  app.on('window-all-closed', () => {
    app.quit();
  });

  app.on('activate', () => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      focusWindow();
    }
  });

  app.on('before-quit', (event) => {
    if (quitting) return;
    logEvent(`quit requested mode=${connectionMode ? connectionMode.kind : 'none'} owned=${councilOwned} child=${isChildAlive()}`);
    if (!isChildAlive()) {
      quitting = true;
      return;
    }
    event.preventDefault();
    quitting = true;
    stopOwnedChild().finally(() => {
      app.quit();
    });
  });

  // 极少数情况下 before-quit 没走到（例如开发时 Ctrl-C），最后再兜一次。
  app.on('will-quit', () => {
    if (isChildAlive()) {
      try {
        councilProcess.kill();
      } catch {
        /* 忽略。 */
      }
    }
  });

  process.on('exit', () => {
    if (isChildAlive()) {
      try {
        councilProcess.kill();
      } catch {
        /* 忽略。 */
      }
    }
  });
}
