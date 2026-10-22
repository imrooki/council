/**
 * `council_ui`：面向模型的工具级入口——确保 Council 看板在跑，并返回它的 URL。
 *
 * 形态照 dsh 0.2.1-rc.2 的 `defineTool` 契约（本机 @deepseek-ai/dsh-tool-todo 是
 * 同一份形状）：`parameters` 是「属性名 -> 属性 spec」的映射，没有参数就给空对象；
 * `output.schema` 是返回值 schema；`execute` 只返回声明的那个 JSON 值；
 * `presentCall` 是给宿主 UI 的展示意图，不参与模型侧契约。
 *
 * 为什么要有单飞：模型一次回复里可能连着调两次，用户也可能连点两下。看板只应该
 * 起一个进程，第二个调用应当等第一个的结果，而不是再 spawn 一个（会撞端口）。
 * 缓存按 URL 分键、promise settle 就删：看板真挂了，下一次调用还能重新拉起。
 */

import { spawn as nodeSpawn } from 'node:child_process';
import z from '@deepseek-ai/schemastery';
import { defineTool } from '@deepseek-ai/dsh-tools';

export const name = 'tool-council';

export const inject = ['tools'];

/** 看板只监听回环地址；`council serve -addr` 的校验也只接受回环主机。 */
const DASHBOARD_HOST = '127.0.0.1';

/** 启动后的健康轮询间隔（任务书 §1.3 固定 150ms）。 */
const HEALTH_POLL_MS = 150;

/**
 * 分离进程的三个必须项：`detached` 让看板活过 dsh 会话，`stdio: 'ignore'`
 * 丢掉它的日志（工具里没有终端可展示），`windowsHide` 避免 Windows 弹黑窗。
 * 选项对象冻结，避免下游实现顺手改掉共享常量。
 */
const SERVE_SPAWN_OPTIONS = Object.freeze({
  detached: true,
  stdio: 'ignore',
  windowsHide: true,
});

export const Config = z.object({
  councilPath: z.string().default('council'),
  port: z.number().step(1).min(1024).max(65535).default(8765),
  // 空串 = 不传 --config，让 council serve 自己按 COUNCIL_CONFIG 或默认位置找。
  // 传了空串却硬塞 `--config ''` 会让 serve 把空路径当成显式配置并报错。
  configPath: z.string().default(''),
  healthTimeoutMs: z.number().step(1).min(100).default(800),
  startTimeoutMs: z.number().step(1).min(500).default(5000),
});

/**
 * 模块级单飞表：key 是看板 URL，value 是正在进行的启动 promise。
 * 放模块级而不是实例上，是因为 dsh 可能把同一个插件挂两次（不同端口），
 * 而「同一进程内只 spawn 一次」的约束是按看板算的。
 */
const starting = new Map();

/**
 * 注册 `council_ui`。
 *
 * `deps` 是测试注入口（`spawn` / `fetch`）。生产路径下 Cordis 只传
 * `(ctx, config)`，于是回落到 `node:child_process` 与全局 `fetch`；这跟
 * `run.js` 把 spawn/seam 从外面递进来是同一种写法，测试因此不必真起进程、
 * 真连端口。
 *
 * @param {import('@deepseek-ai/cordis').Context} ctx - 带 `ctx.tools` 的插件上下文。
 * @param {object} config - 校验过的 {@link Config} 值。
 * @param {{ spawn?: Function, fetch?: Function }} [deps] - 可注入的进程与网络实现。
 */
export function apply(ctx, config, deps = {}) {
  ctx.tools.register(createCouncilUiTool(config, deps));
}

/**
 * 构造工具定义。拆出来是为了让 `apply` 只剩注册这一件事；测试从 `apply` 的
 * 注册结果里取定义，所以这里不再单独导出。
 *
 * @param {object} config - 校验过的插件配置。
 * @param {{ spawn?: Function, fetch?: Function }} [deps] - 可注入的进程与网络实现。
 * @returns {object} `defineTool` 产出的工具定义。
 */
function createCouncilUiTool(config, deps = {}) {
  return defineTool({
    name: 'council_ui',
    description: 'Make sure the local Council dashboard (the multi-model review board) is running and return its URL. '
      + 'Call it only when the user asks to see the dashboard. When nothing answers it starts `council serve` in the '
      + 'background and waits for it; when the dashboard still cannot be reached it returns running: false with the '
      + 'last error instead of throwing. Do not use it for reviews: those are `council` and `council_work`.',
    // 无参数。defineTool 的参数 spec 是属性映射，空对象编译成 {type:'object', properties:{}}。
    parameters: {},
    output: {
      schema: {
        type: 'object',
        additionalProperties: false,
        properties: {
          url: { type: 'string', required: true },
          running: { type: 'boolean', required: true },
          // 失败路径不声明 started（任务书 §1.4 的失败形状只有 url/running/error），
          // 所以这里只能是可选字段。
          started: { type: 'boolean' },
          error: { type: 'string' },
        },
      },
      render: (_args, value) => [{ type: 'text', text: describeValue(value) }],
    },
    execute(_args, _exec) {
      return ensureDashboard(config, deps);
    },
    presentCall: (args) => ({
      card: 'generic',
      title: 'Council dashboard',
      kind: 'other',
      rawInput: args,
    }),
  });
}

/**
 * 保证看板在跑：先探一次健康，通了就直接返回；没通才进入单飞启动。
 *
 * 这个函数不 throw：看板没起来不该让模型这一轮崩掉，失败一律收敛成
 * `{ url, running: false, error }`。
 *
 * @param {object} config - 校验过的插件配置。
 * @param {{ spawn?: Function, fetch?: Function }} deps - 注入的进程与网络实现。
 * @returns {Promise<{ url: string, running: boolean, started?: boolean, error?: string }>}
 */
async function ensureDashboard(config, deps) {
  const fetchImpl = deps.fetch ?? globalThis.fetch;
  const url = dashboardUrl(config.port);
  const probe = () => probeHealth(fetchImpl, url, config.healthTimeoutMs);

  const first = await probe();
  if (first.ok) {
    // 已经通了就不 spawn：用户可能自己开着看板，再起一个只会撞端口。
    return { url, running: true, started: false };
  }

  const pending = starting.get(url);
  if (pending !== undefined) return pending;

  const started = startDashboard(config, deps, url, probe, first.error)
    .finally(() => {
      // settle 后立刻清表：既让并发的后来者能复用同一个 promise，又让看板
      // 之后真的挂掉时下一次调用还能重新拉起，而不是永远返回旧的失败。
      starting.delete(url);
    });
  starting.set(url, started);
  return started;
}

/**
 * spawn 一个分离的 `council serve`，然后轮询健康直到成功或超时。
 *
 * @param {object} config - 校验过的插件配置。
 * @param {{ spawn?: Function }} deps - 注入的 spawn 实现。
 * @param {string} url - 看板根 URL。
 * @param {() => Promise<{ ok: boolean, error?: string }>} probe - 健康探测。
 * @param {string | undefined} firstError - 启动前那次探测的错误，作为兜底文案。
 * @returns {Promise<{ url: string, running: boolean, started?: boolean, error?: string }>}
 */
async function startDashboard(config, deps, url, probe, firstError) {
  const spawnImpl = deps.spawn ?? nodeSpawn;
  let lastError = firstError ?? `no response from ${url}/api/health`;

  let child;
  try {
    child = spawnImpl(config.councilPath, serveArgs(config), SERVE_SPAWN_OPTIONS);
  } catch (error) {
    // 同步抛错（某些注入实现把 ENOENT 抛在这里）同样收敛成失败值。
    return { url, running: false, error: `failed to start council serve: ${errorMessage(error)}` };
  }

  // Node 的 spawn 对不存在的可执行文件是异步 `error` 事件，不是 throw；
  // 不挂监听会变成未捕获异常，把 dsh 进程一起带崩。这里只记下来，由轮询收口。
  let spawnError;
  child.once('error', (error) => {
    spawnError = error;
  });
  // detached 子进程必须 unref，否则 dsh 退出时会被看板拖住。
  child.unref?.();

  const deadline = Date.now() + config.startTimeoutMs;
  for (;;) {
    const attempt = await probe();
    if (attempt.ok) return { url, running: true, started: true };
    lastError = attempt.error;

    if (spawnError !== undefined) {
      // 进程都没起来，继续等只会白等满 5 秒；把 spawn 的原因原样带回去。
      return { url, running: false, error: `failed to start council serve: ${errorMessage(spawnError)}` };
    }

    const remaining = deadline - Date.now();
    if (remaining <= 0) return { url, running: false, error: lastError };
    await delay(Math.min(HEALTH_POLL_MS, remaining));
  }
}

/**
 * 探一次 `/api/health`。
 *
 * 超时用 AbortController 而不是 `AbortSignal.timeout`：后者留下的定时器无法在
 * 成功路径上显式清掉，会让进程多活一个 healthTimeoutMs。这里 finally 里必清。
 *
 * @param {Function} fetchImpl - fetch 实现。
 * @param {string} url - 看板根 URL。
 * @param {number} timeoutMs - 单次探测上限。
 * @returns {Promise<{ ok: boolean, error?: string }>} 失败时带原因，绝不 throw。
 */
async function probeHealth(fetchImpl, url, timeoutMs) {
  const controller = new AbortController();
  // 不用 unref：探测只活 800ms，而 unref 过的 timer 在「fetch 的 promise
  // 没有 ref 住任何句柄」的注入实现下可能永远不触发，进程会先退出。
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${url}/api/health`, { signal: controller.signal });
    // 端口上可能是别的服务：只有 Council 的 /api/health 会回 {"ok":true}。
    // 不读 body 会把连接一直挂着（Node 的 fetch 不消费 body 就不归还连接），
    // 所以无论成败都把它消费掉；body 为空时不作判断（204 之类）。
    let rawBody = '';
    if (typeof response.text === 'function') {
      rawBody = await response.text().catch(() => '');
    }
    if (response.ok !== true) {
      return { ok: false, error: `GET /api/health returned HTTP ${response.status ?? 'unknown'}` };
    }
    if (rawBody.trim() !== '' && !rawBody.includes('"ok"')) {
      return { ok: false, error: 'GET /api/health did not look like Council' };
    }
    return { ok: true };
  } catch (error) {
    // 超时和连接拒绝都只是「还没起来」，不是异常；原因留给最后的 error 字段。
    return {
      ok: false,
      error: controller.signal.aborted
        ? `GET /api/health timed out after ${timeoutMs} ms`
        : errorMessage(error),
    };
  } finally {
    clearTimeout(timer);
  }
}

/**
 * `council serve` 的 argv。数组直通 spawn，不拼 shell 字符串：configPath 里
 * 可能带空格，交给 CreateProcess 的 argv 规则比我们自己转义可靠。
 *
 * @param {object} config - 校验过的插件配置。
 * @returns {string[]}
 */
function serveArgs(config) {
  const args = ['serve', '-addr', `${DASHBOARD_HOST}:${config.port}`];
  if (config.configPath !== '') args.push('--config', config.configPath);
  return args;
}

function dashboardUrl(port) {
  return `http://${DASHBOARD_HOST}:${port}`;
}

/**
 * 模型看到的文本。URL 永远出现，失败时带原因，成功时说明是不是这次拉起来的。
 *
 * @param {{ url: string, running: boolean, started?: boolean, error?: string }} value
 * @returns {string}
 */
function describeValue(value) {
  if (value.running === true) {
    return value.started === true
      ? `Council dashboard started at ${value.url}`
      : `Council dashboard already running at ${value.url}`;
  }
  return `Council dashboard is not running at ${value.url}: ${value.error}`;
}

function delay(milliseconds) {
  return new Promise((resolve) => {
    setTimeout(resolve, milliseconds);
  });
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}
