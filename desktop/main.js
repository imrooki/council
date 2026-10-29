'use strict';

// Council 桌面外壳：先找已经在跑的工作台，找不到才自己拉起 `council serve -rw`。
// 所有业务仍然在 Go 的 HTTP 服务里，Electron 这边不读库、不跑议会，
// 也不替用户登录任何东西。

const { app, BrowserWindow, Menu, Tray, dialog, shell } = require('electron');
const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const { URL } = require('node:url');

// 自动更新只对「装好的 app」有意义：开发态（npm start）没有 app-update.yml，
// 也没有可替换的安装目录，直接跳过。
let autoUpdater = null;
try {
  ({ autoUpdater } = require('electron-updater'));
} catch (error) {
  // 依赖缺失不该让应用起不来：没有更新能力远好于打不开窗口。
  console.warn('[council-desktop] electron-updater unavailable:', error && error.message);
}

const APP_TITLE = 'Council';
const DEFAULT_PRIMARY_PORT = 8765;
const PRIMARY_PORT = resolvePrimaryPort();
const PRIMARY_BASE_URL = `http://127.0.0.1:${PRIMARY_PORT}`;
const CONFIGURED_BASE_URL = normalizeBaseUrl(process.env.COUNCIL_URL || '');
const DEFAULT_COUNCIL_EXE = path.join(os.homedir(), 'bin', 'council.exe');
const HEALTH_TIMEOUT_MS = 1500;
const READY_TIMEOUT_MS = 20000;
const POLL_INTERVAL_MS = 400;
const OUTPUT_TAIL_CHARS = 64 * 1024;
const DIALOG_TAIL_CHARS = 6000;
// portable 的自解压目录可能正被上一个实例清理：找不到 exe 时按短间隔重试。
const EXE_RESOLVE_TIMEOUT_MS = 2000;
const EXE_RESOLVE_INTERVAL_MS = 150;
// 关掉立刻再开时，上一个进程可能还没完全退出、单实例锁还没释放。
const SINGLE_INSTANCE_WAIT_MS = 2000;
const SINGLE_INSTANCE_POLL_MS = 150;

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
let startupStarted = false;
let startupFailed = false;
let updaterStarted = false;
let quitting = false;
let quitInProgress = false;
let tray = null;
let lastSecondInstanceLogAt = 0;

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
const LOG_FILE_NAME = 'desktop.log';

// logFilePath 是诊断日志的绝对路径。它只在两处出现：logEvent 与托盘里的
// 「打开诊断日志」—— 用户不需要知道 app 把日志放在哪，只需要能点开它。
function logFilePath() {
  return path.join(app.getPath('userData'), LOG_FILE_NAME);
}

function logEvent(message) {
  try {
    const dir = app.getPath('userData');
    fs.mkdirSync(dir, { recursive: true });
    fs.appendFileSync(logFilePath(), `${new Date().toISOString()} ${message}\n`, 'utf8');
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

// councilExeCandidates 收集本次启动要尝试的所有路径。显式指定（--council-exe /
// COUNCIL_EXE）时只有一个候选：用户点名的路径不对，重试别的布局没有意义。
//
// 打包后 council.exe 的位置取决于 extraResources 的映射方式与 portable 的
// 自解压布局。与其猜一种，不如把已知的几种都试一遍——找不到时报错会把候选
// 全列出来，一眼就能看出是「没打进去」还是「打进去了但路径不同」。
function councilExeCandidates() {
  const equalsArg = process.argv.find((arg) => arg.startsWith('--council-exe='));
  if (equalsArg) return [path.resolve(equalsArg.slice('--council-exe='.length))];

  const flagIndex = process.argv.indexOf('--council-exe');
  if (flagIndex !== -1 && process.argv[flagIndex + 1]) {
    return [path.resolve(process.argv[flagIndex + 1])];
  }

  if (process.env.COUNCIL_EXE) return [path.resolve(process.env.COUNCIL_EXE)];

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
  return candidates;
}

function firstExistingCouncilExe(candidates) {
  for (const candidate of candidates) {
    try {
      if (fs.existsSync(candidate) && fs.statSync(candidate).isFile()) return candidate;
    } catch {
      // 目录可能正被上一个实例删除；当作这一次不存在，留给重试。
    }
  }
  return null;
}

// resolveCouncilExe 解析 council.exe。portable 关掉立刻再开时，候选路径可能
// 落在上一个实例正在清理的自解压目录里，第一次 existsSync 全部落空——
// 这并不等于「没打进去」。先按 150ms 重试最多 2 秒，仍找不到才把第一个候选
// 交给 startCouncil 报错（错误框里保留完整候选列表）。
async function resolveCouncilExe() {
  const candidates = councilExeCandidates();
  lastExeCandidates = candidates;

  const deadline = Date.now() + EXE_RESOLVE_TIMEOUT_MS;
  let announced = false;
  for (;;) {
    const found = firstExistingCouncilExe(candidates);
    if (found) {
      if (announced) logEvent(`council.exe appeared after retry: ${found}`);
      return found;
    }
    if (Date.now() >= deadline) break;
    if (!announced) {
      announced = true;
      logEvent(`council.exe not found yet, retrying ${EXE_RESOLVE_TIMEOUT_MS}ms: ${candidates.join(' | ')}`);
    }
    await sleep(EXE_RESOLVE_INTERVAL_MS);
  }
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

async function startCouncil(port) {
  const exe = await resolveCouncilExe();
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

// startupErrorFromExit 把「子进程没起来」翻译成用户能行动的一句话。
//
// 细节（端口、命令行、cwd、health 探测结果）不进对话框：它们是诊断信息，
// 由 failStartup 统一写进日志。对话框只回答「怎么办」——会不会有人真的
// 去读「127.0.0.1:8765 已被占用」然后自己解决？我们的用户不会。
function startupErrorFromExit(exitInfo, lastReason, url) {
  const code = exitInfo.code === null ? `信号 ${exitInfo.signal}` : `退出码 ${exitInfo.code}`;
  const stderr = councilStderrTail;
  const portInUse = /address already in use|Only one usage of each socket address|bind:\s|监听 .* 失败|listen tcp/i.test(stderr);
  const lockBusy = /council\.lock|另一个 council|已经在运行|无法获取.*锁|database is locked/i.test(stderr);

  // 诊断细节单独记日志：这里写全，对话框里就不用写。
  logEvent(`startup failure code=${code} target=${targetLabel(url)} lastReason=${lastReason || '无响应'}`);
  if (spawnedInfo) {
    logEvent(`startup failure command="${spawnedInfo.exe}" ${spawnedInfo.args.join(' ')} cwd="${spawnedInfo.cwd}"`);
  }
  if (councilStderrTail) {
    logEvent(`startup failure stderr_begin\n${councilStderrTail}\nstartup failure stderr_end`);
  }

  if (portInUse) {
    return new StartupFailure(
      'Council 启动失败',
      'Council 需要的本地端口被别的程序占着。\n\n'
      + '先关掉占用它的程序（常见的是一个还在跑的 council 或另一个开发服务器），再重新打开 Council。',
    );
  }

  if (lockBusy) {
    return new StartupFailure(
      'Council 启动失败：另一个 Council 正在运行',
      '已经有一个 council 在用同一份数据目录。\n\n'
      + '先关掉终端里的 council（或托盘里的旧窗口），再重新打开。',
    );
  }

  return new StartupFailure(
    'Council 启动失败',
    '本地服务没能启动。\n\n'
    + '重新打开 Council 通常就好了；如果反复出现，托盘菜单 → 诊断 → 打开诊断日志，把那一份发给我们。',
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

// modeSuffix 只标「这份窗口连的是谁」，不标地址。
//
// 旧版本会把 host（127.0.0.1:8765）拼进标题与托盘 tip。那是实现细节：
// 用户不该知道内部有个本地 HTTP 服务，更不该被引导去浏览器里打开它。
// 真需要地址排查时，诊断日志里仍然有完整 url（每次 loadWorkbench 都记一行）。
function modeSuffix() {
  if (!connectionMode) return '';
  return connectionMode.kind === 'attach' ? ' · 已连接' : ' · 自启';
}

function appTitleText() {
  return `${pageTitle || APP_TITLE}${modeSuffix()}`;
}

// 托盘 tip 与窗口标题用同一句话：页面标题本身带「写模式 / 只读看板」，
// modeSuffix 再补「已连接 / 自启 + host」，正好是任务要求的模式与地址。
function updateWindowTitle() {
  const title = appTitleText();
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.setTitle(title);
  if (!tray) return;
  try {
    tray.setToolTip(title);
  } catch {
    /* 托盘刚被销毁时忽略。 */
  }
  refreshTrayMenu();
}

function browserTargetUrl() {
  if (baseUrl) return baseUrl;
  if (!mainWindow || mainWindow.isDestroyed()) return null;
  const current = mainWindow.webContents.getURL();
  if (isHttpUrl(current) && isSameAppOrigin(current)) return current;
  return null;
}

function refreshTrayMenu() {
  if (!tray) return;
  const target = browserTargetUrl();
  const menu = Menu.buildFromTemplate([
    {
      label: '打开 Council',
      click: () => showMainWindow(),
    },
    // 「在浏览器中打开」不在第一层：桌面版是主界面，用户不该被引导去开浏览器。
    // 它仍然有用（分享给同事、排查渲染问题），所以收进「诊断」子菜单而不是删掉。
    {
      label: '诊断',
      submenu: [
        {
          label: '在浏览器中打开',
          enabled: Boolean(target),
          click: () => {
            if (!target) return;
            logEvent(`tray open in browser url=${target}`);
            shell.openExternal(target).catch((error) => {
              logEvent(`tray open in browser failed: ${error.message}`);
            });
          },
        },
        {
          label: '检查更新…',
          click: () => {
            checkForUpdatesManually().catch((error) => {
              logEvent(`updater: tray check failed ${error && error.message ? error.message : String(error)}`);
            });
          },
        },
        {
          label: '打开诊断日志',
          click: () => {
            openDiagnosticsLog();
          },
        },
      ],
    },
    { type: 'separator' },
    {
      label: '彻底退出',
      click: () => {
        quitFromTray().catch((error) => {
          logEvent(`tray quit failed: ${error && error.message ? error.message : String(error)}`);
          app.exit(1);
        });
      },
    },
  ]);
  tray.setContextMenu(menu);
}

// openDiagnosticsLog 用系统默认方式打开这个 app 的日志文件。
//
// 为什么要有它：桌面版不再把端口、命令行塞进错误对话框（那属于诊断），
// 但「出问题要能查」这条不能丢 —— 入口就从「错误框里堆细节」换成「一个能点开的日志」。
function openDiagnosticsLog() {
  try {
    // 先写一行：文件一定存在，用户点开一个空目录会以为坏了。
    logEvent('diagnostics log opened from tray');
    const logPath = logFilePath();
    if (!fs.existsSync(logPath)) {
      logEvent(`diagnostics log missing at ${logPath}`);
      return;
    }
    shell.openPath(logPath).then((error) => {
      if (error) logEvent(`open diagnostics log failed: ${error}`);
    });
  } catch (error) {
    logEvent(`open diagnostics log threw: ${error && error.message ? error.message : String(error)}`);
  }
}

function createTray() {
  if (tray) return;
  const icon = windowIconPath();
  if (!icon) {
    // 没有图标就不隐藏窗口：否则用户只能靠任务管理器结束一个看不见的进程。
    logEvent('tray icon missing; close will quit instead of hiding');
    return;
  }
  try {
    tray = new Tray(icon);
  } catch (error) {
    tray = null;
    logEvent(`tray creation failed: ${error.message}; close will quit instead of hiding`);
    return;
  }
  tray.setToolTip(appTitleText());
  tray.on('click', () => showMainWindow());
  tray.on('double-click', () => showMainWindow());
  refreshTrayMenu();
  logEvent(`tray created icon="${icon}"`);
}

function destroyTray() {
  if (!tray) return;
  try {
    tray.destroy();
  } catch {
    /* 已经销毁。 */
  }
  tray = null;
}

// 点 × 只隐藏窗口。真正退出的路径是托盘菜单的「彻底退出」（或系统结束进程），
// 这样自启的 council 不会被顺手带走，再打开时界面状态也还在。
function hideMainWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  try {
    // 显式再确认一次；webPreferences 里同样是 true，双保险，绝不打开节流开关的反面。
    mainWindow.webContents.setBackgroundThrottling(true);
  } catch {
    /* 窗口正在销毁时忽略。 */
  }
  mainWindow.hide();
  logEvent('window hidden to tray; backgroundThrottling=true');
}

function showMainWindow() {
  if (quitting || startupFailed) return;
  if (!startupStarted) return;

  if (!mainWindow || mainWindow.isDestroyed()) {
    if (!startupFinished) return;
    logEvent('window is gone; recreating it from tray');
    createWindow();
    return;
  }
  if (mainWindow.isMinimized()) mainWindow.restore();
  if (!mainWindow.isVisible()) mainWindow.show();
  // single-instance 的 2 秒重试会连着发多次事件：已经在前台就不要再抢焦点。
  if (!mainWindow.isFocused()) mainWindow.focus();
}

async function quitFromTray() {
  if (quitInProgress || quitting) return;
  quitInProgress = true;
  quitting = true;
  logEvent(
    `tray quit requested mode=${connectionMode ? connectionMode.kind : 'none'}`
    + ` owned=${councilOwned} child=${isChildAlive()}`,
  );
  // 先摘掉托盘，用户立刻能看到「确实在退出」。
  destroyTray();
  try {
    // attach 模式下 councilOwned=false，这里不会碰用户自己的 serve。
    await stopOwnedChild();
  } finally {
    app.quit();
  }
}

function focusWindow() {
  showMainWindow();
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
      // 窗口隐藏后 Chromium 默认会节流后台计时器；这里显式写明 true，
      // 避免以后有人为了「后台刷新」把它改掉。前端自己也有 document.hidden
      // 判断（visibilitychange 时 stopPolling），两层一起保证隐藏后不空转。
      backgroundThrottling: true,
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

  // 点 × / Alt+F4：只隐藏。没有托盘时不能藏（会变成看不见的后台进程），
  // 此时放行 close，由 window-all-closed 负责退出。
  mainWindow.on('close', (event) => {
    if (quitting) return;
    if (!tray) {
      logEvent('close requested without tray; allowing quit');
      return;
    }
    event.preventDefault();
    hideMainWindow();
  });

  mainWindow.on('hide', () => {
    logEvent('window hide event');
  });

  mainWindow.on('show', () => {
    logEvent('window show event');
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
    if (!quitting && tray) logEvent('window destroyed; tray keeps app alive');
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
    // 只给用户一句能行动的话；地址与错误码是诊断信息，进日志。
    logEvent(`did-fail-load url=${failedUrl} code=${errorCode} desc=${errorDescription}`);
    dialog.showErrorBox(
      '工作台没有响应',
      'Council 背后的本地服务可能已经退出。\n\n'
      + '请用托盘菜单里的「彻底退出」结束 Council，然后重新打开它。\n'
      + '如果反复出现，托盘菜单 → 诊断 → 打开诊断日志，把那一份发给我们。',
    );
  });

  contents.on('render-process-gone', (_event, details) => {
    if (startupFailed || quitting) return;
    dialog.showErrorBox(
      '工作台渲染进程退出',
      `窗口内容进程已退出：${details.reason}\n\n关闭窗口后重新打开 Council。`,
    );
  });

  // 首次启动先画加载页；窗口若被托盘重建，则直接回到已知的 baseUrl。
  const initialUrl = (startupFinished && baseUrl)
    ? baseUrl
    : loadingDataUrl('正在启动工作台…');
  mainWindow.loadURL(initialUrl).catch(() => {});
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

// failStartup 是「应用起不来」时用户看到的唯一一屏。
//
// 分工要清楚：对话框只回答「怎么办」，诊断细节（子进程输出、命令、cwd、
// 探测过的地址）全部进日志 —— 以前这些东西都堆在消息里，用户看到的是端口号
// 和命令行，而不是一句能行动的话。
async function failStartup(error) {
  if (startupFailed || quitting) return;
  startupFailed = true;

  const title = error && error.title ? error.title : 'Council 启动失败';
  let message = error && error.message ? error.message : String(error);

  // 诊断细节写日志（不截断，全量留给排查）。
  logEvent(`failure title="${title}"`);
  logEvent(`failure message=${message.replace(/\n/g, ' | ')}`);
  logEvent(`failure primary_url=${PRIMARY_BASE_URL} current_url=${baseUrl || '(none)'}`);
  const childLog = childLogBlock(1 << 20);
  if (childLog) {
    logEvent(`failure child_output_begin\n${childLog}\nfailure child_output_end`);
  }

  message += `\n\n诊断日志：${logFilePath()}`;
  message += '\n（托盘菜单 → 诊断 → 打开诊断日志，可看到完整原因）';

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
    // 窗口已经起来了才谈更新：更新永远不是启动的前提。
    if (!updaterStarted) {
      updaterStarted = true;
      setupAutoUpdater();
    }
  } catch (error) {
    if (error && error.code === 'ERR_ABORTED') return;
    throw new StartupFailure(
      '工作台加载失败',
      '界面没有加载成功。\n\n'
      + '重新打开 Council 通常就好了；如果反复出现，托盘菜单 → 诊断 → 打开诊断日志。',
    );
  }
}

async function start() {
  Menu.setApplicationMenu(null);
  app.setAppUserModelId('com.council.workbench');

  startupStarted = true;
  // 托盘先建好：窗口从 ready-to-show 出现之前，× 就已经有地方可去。
  createTray();
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
  setLoadingStatus('正在启动工作台…');
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
    setLoadingStatus('正在启动工作台…');
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
  setLoadingStatus('正在启动工作台…');
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
  // 端口不进加载文案：这是用户不需要知道的实现细节（日志里有）。
  setLoadingStatus('正在启动工作台…');

  let spawned;
  try {
    spawned = await startCouncil(port);
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

// ---- 自动更新 ----
//
// 「DSH 那种」和「网页快捷方式」最实际的一条差别就在这里：发版之后用户
// 不需要知道有新版，也不需要自己来下载。electron-updater 读 release 里的
// latest.yml（electron-builder 配了 build.publish 后自动生成）。
//
// 三条自我约束：
//   - 只在装好的 app 上启用：开发态没有 app-update.yml，检查只会报错；
//   - 包在后台下，装不装由用户点「立即重启」；重启前先把自己拉起的 serve
//     收掉，否则那个进程会变成孤儿；
//   - 任何更新失败都只写日志，绝不让它挡住应用启动。

const UPDATE_CHECK_DELAY_MS = 8000;

function updatesSupported() {
  return Boolean(autoUpdater) && app.isPackaged;
}

function updaterLogger() {
  return {
    info: (m) => logEvent(`updater info: ${m}`),
    warn: (m) => logEvent(`updater warn: ${m}`),
    error: (m) => logEvent(`updater error: ${m}`),
    debug: (m) => logEvent(`updater debug: ${m}`),
  };
}

// setupAutoUpdater 接上事件、延迟做一次静默检查。只在窗口起来之后调用：
// 更新永远不是启动的前提。
function setupAutoUpdater() {
  if (!updatesSupported()) {
    logEvent(`auto update disabled (packaged=${app.isPackaged} updater=${Boolean(autoUpdater)})`);
    return;
  }

  autoUpdater.logger = updaterLogger();
  autoUpdater.autoDownload = true;
  autoUpdater.autoInstallOnAppQuit = false;

  autoUpdater.on('checking-for-update', () => logEvent('updater: checking for update'));
  autoUpdater.on('update-available', (info) => {
    logEvent(`updater: ${info && info.version} available, downloading in background`);
  });
  autoUpdater.on('update-not-available', () => logEvent('updater: already up to date'));
  autoUpdater.on('download-progress', (progress) => {
    logEvent(`updater: download ${Math.round(progress.percent || 0)}%`);
  });
  autoUpdater.on('error', (error) => {
    logEvent(`updater: error ${error && error.message ? error.message : String(error)}`);
  });
  autoUpdater.on('update-downloaded', (info) => {
    logEvent(`updater: ${info && info.version} downloaded, waiting for user`);
    promptRestartForUpdate(info).catch((error) => {
      logEvent(`updater: restart prompt failed ${error && error.message ? error.message : String(error)}`);
    });
  });

  const timer = setTimeout(() => {
    autoUpdater.checkForUpdates().catch((error) => {
      logEvent(`updater: initial check failed ${error && error.message ? error.message : String(error)}`);
    });
  }, UPDATE_CHECK_DELAY_MS);
  if (typeof timer.unref === 'function') timer.unref();
}

// promptRestartForUpdate 问一次「现在重启还是稍后」。
async function promptRestartForUpdate(info) {
  if (quitting) return;
  const version = info && info.version ? info.version : '新版本';
  const { response } = await dialog.showMessageBox(mainWindow || undefined, {
    type: 'info',
    title: 'Council 已就绪更新',
    message: `Council ${version} 已经下载好了。`,
    detail: '现在重启会关闭当前窗口并更新；正在跑的议会会被中断。',
    buttons: ['立即重启', '稍后'],
    defaultId: 0,
    cancelId: 1,
    noLink: true,
  });
  if (response !== 0) {
    logEvent('updater: user postponed restart');
    return;
  }
  await restartForUpdate();
}

// restartForUpdate 停掉自己拉起的 serve 再交给 updater。
// 顺序不能反：quitAndInstall 会直接退出进程，留下的子进程没人收。
async function restartForUpdate() {
  quitting = true;
  try {
    await stopOwnedChild();
  } catch (error) {
    logEvent(`updater: stop child before install failed ${error && error.message ? error.message : String(error)}`);
  }
  logEvent('updater: quitting to install');
  autoUpdater.quitAndInstall(false, true);
}

// checkForUpdatesManually 是托盘「检查更新」的实现：有结果都要给用户一句话，
// 不能像后台检查那样只写日志。
async function checkForUpdatesManually() {
  if (!updatesSupported()) {
    await dialog.showMessageBox(mainWindow || undefined, {
      type: 'info',
      title: '检查更新',
      message: '这个版本不支持自动更新。',
      detail: '开发态（npm start）或安装包里缺少更新组件时不会检查更新。',
      buttons: ['好'],
      noLink: true,
    });
    return;
  }
  try {
    const result = await autoUpdater.checkForUpdates();
    const current = app.getVersion();
    const version = result && result.updateInfo ? result.updateInfo.version : current;
    if (version === current) {
      await dialog.showMessageBox(mainWindow || undefined, {
        type: 'info',
        title: '检查更新',
        message: '已经是最新版本。',
        detail: `当前版本 ${current}。`,
        buttons: ['好'],
        noLink: true,
      });
      return;
    }
    // 有新版时后台已经在下载，update-downloaded 会再弹一次「立即重启」。
    await dialog.showMessageBox(mainWindow || undefined, {
      type: 'info',
      title: '发现新版本',
      message: `Council ${version} 正在后台下载。`,
      detail: '下载完成后会提示你重启。',
      buttons: ['好'],
      noLink: true,
    });
  } catch (error) {
    logEvent(`updater: manual check failed ${error && error.message ? error.message : String(error)}`);
    await dialog.showMessageBox(mainWindow || undefined, {
      type: 'error',
      title: '检查更新失败',
      message: '暂时连不上更新服务器。',
      detail: '稍后再试。托盘菜单 → 诊断 → 打开诊断日志可看到具体原因。',
      buttons: ['好'],
      noLink: true,
    });
  }
}

app.setName(APP_TITLE);

// acquireSingleInstanceLock 在另一个实例正在退出时不要立刻放弃：portable
// 「关掉立刻再开」会赶在上一个进程释放单实例锁之前走到这里，直接 quit
// 等于双击没反应。最多等 2 秒，每 150ms 重试一次；等不到才认定确实已有
// 实例在跑（此时让出启动，由它的 second-instance 把窗口拉到前面）。
//
// 重复调用 requestSingleInstanceLock 是 Electron 的既有语义：拿不到锁时
// 每次调用都会重新尝试获取，第一个实例还活着就继续返回 false。
async function acquireSingleInstanceLock(timeoutMs = SINGLE_INSTANCE_WAIT_MS) {
  if (app.requestSingleInstanceLock()) return true;

  logEvent(`single instance lock busy, waiting up to ${timeoutMs}ms for the previous instance`);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await sleep(SINGLE_INSTANCE_POLL_MS);
    if (app.requestSingleInstanceLock()) {
      logEvent('single instance lock acquired after wait');
      return true;
    }
  }
  return false;
}

async function boot() {
  if (!(await acquireSingleInstanceLock())) {
    logEvent('another instance is still running; quitting');
    app.quit();
    return;
  }

  // 已有实例在托盘时再次双击 exe：把它的窗口唤出来，新实例自己退出。
  // 拿锁重试会连续触发这个事件，日志按 1.5 秒合并，避免一次双击写十几行。
  app.on('second-instance', () => {
    const now = Date.now();
    if (now - lastSecondInstanceLogAt > 1500) {
      lastSecondInstanceLogAt = now;
      logEvent('second instance launched; focusing existing window');
    }
    focusWindow();
  });

  app.whenReady().then(start).catch((error) => {
    failStartup(error).catch(() => app.exit(1));
  });

  // 窗口关掉不代表进程退出：托盘还在，council 也还在。只有托盘不可用
  // （图标缺失等异常）时才退回「关窗即退出」。
  app.on('window-all-closed', () => {
    if (quitting) return;
    if (!tray) {
      app.quit();
      return;
    }
    logEvent('all windows closed; tray keeps app alive');
  });

  app.on('activate', () => {
    showMainWindow();
  });

  app.on('before-quit', (event) => {
    if (quitting) {
      destroyTray();
      return;
    }
    logEvent(`quit requested mode=${connectionMode ? connectionMode.kind : 'none'} owned=${councilOwned} child=${isChildAlive()}`);
    if (!isChildAlive()) {
      quitting = true;
      destroyTray();
      return;
    }
    event.preventDefault();
    quitting = true;
    destroyTray();
    stopOwnedChild().finally(() => {
      app.quit();
    });
  });

  // 极少数情况下 before-quit 没走到（例如开发时 Ctrl-C），最后再兜一次。
  app.on('will-quit', () => {
    destroyTray();
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

boot().catch((error) => {
  try {
    logEvent(`boot failed: ${error && error.message ? error.message : String(error)}`);
  } catch {
    /* 日志失败不能影响退出。 */
  }
  app.exit(1);
});
