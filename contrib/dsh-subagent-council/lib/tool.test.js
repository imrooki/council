// tool.js 顶部 import 了 @deepseek-ai/schemastery 与 @deepseek-ai/dsh-tools，
// 这两个包只在 dsh 运行时里存在，本仓库没有安装。这里用 module.registerHooks 的
// resolve 钩子把它们换成忠实替身，再动态 import tool.js —— 跟 index.test.js 用
// seam 替身的理由一样：被测的是我们写的契约与流程，不是 dsh 包本身。
// 全程不 spawn 真进程、不连端口。
import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { registerHooks } from 'node:module';
import test from 'node:test';

// 只需要支撑 Config 用到的链式调用：z.object / z.string / z.number + default/min/max/step。
const SCHEMASTERY_DOUBLE = `
const chain = (kind) => {
  const schema = () => {};
  schema.kind = kind;
  schema.default = (value) => { schema.defaultValue = value; return schema; };
  schema.min = (value) => { schema.minimum = value; return schema; };
  schema.max = (value) => { schema.maximum = value; return schema; };
  schema.step = (value) => { schema.stepValue = value; return schema; };
  return schema;
};
export default {
  object: (shape) => ({ shape }),
  string: () => chain('string'),
  number: () => chain('number'),
};
`;

// 真实 defineTool 会把 DSL 编译成 JSON Schema；这里原样返回，测试直接核对
// parameters / output.schema 的 DSL 形状。
const DEFINE_TOOL_DOUBLE = 'export const defineTool = (definition) => definition;';

const dshDoubles = new Map([
  ['@deepseek-ai/schemastery', SCHEMASTERY_DOUBLE],
  ['@deepseek-ai/dsh-tools', DEFINE_TOOL_DOUBLE],
]);

registerHooks({
  resolve(specifier, context, nextResolve) {
    const source = dshDoubles.get(specifier);
    if (source !== undefined) {
      return { url: `data:text/javascript,${encodeURIComponent(source)}`, shortCircuit: true };
    }
    return nextResolve(specifier, context);
  },
});

const { apply } = await import('./tool.js');

class FakeChild extends EventEmitter {
  constructor() {
    super();
    this.pid = 6001;
    this.unrefCount = 0;
  }

  unref() {
    this.unrefCount += 1;
  }
}

/**
 * 假 spawn：记录每次 argv/options，可同步抛错，可在 spawn 时切换「看板已就绪」。
 */
function createSpawn({ failWith, onSpawn } = {}) {
  const calls = [];
  const children = [];
  const spawn = (command, args, options) => {
    calls.push({ command, args, options });
    if (failWith !== undefined) throw failWith;
    const child = new FakeChild();
    children.push(child);
    onSpawn?.(child);
    return child;
  };
  return { calls, children, spawn };
}

/**
 * 假 fetch：记录每次请求，由 respond 决定成功、失败或抛错。
 */
function createFetch(respond) {
  const calls = [];
  const fetch = async (url, options) => {
    calls.push({ url, options });
    const response = respond();
    // 真 fetch 的 body 必须被消费，探测代码也会读一次；替身补齐 text()，
    // 默认回 Council 的 health 正文，免得「随便一个 200」也算健康。
    if (typeof response.text !== 'function') {
      response.text = async () => '{"ok":true}';
    }
    return response;
  };
  return { calls, fetch };
}

/**
 * 用假 ctx 走真实的 apply，拿到注册进 ctx.tools 的工具定义。
 */
function mount(config, deps) {
  const registered = [];
  const ctx = { tools: { register: (definition) => registered.push(definition) } };
  apply(ctx, config, deps);
  assert.equal(registered.length, 1, 'apply 只注册一个工具');
  assert.equal(registered[0].name, 'council_ui');
  return registered[0];
}

function councilConfig(overrides = {}) {
  return {
    councilPath: 'council',
    port: 8765,
    configPath: '',
    healthTimeoutMs: 100,
    startTimeoutMs: 500,
    ...overrides,
  };
}

/** 看板没起来时的典型错误：fetch 在 ECONNREFUSED 上直接 reject。 */
function connectionRefused() {
  throw new Error('connect ECONNREFUSED 127.0.0.1:8765');
}

/**
 * 按 tool.js 的 output.schema DSL 做一次结构校验：只实现这个 schema 用到的
 * type / required / properties / additionalProperties，不引第三方校验器。
 */
function assertConforms(schema, value, path = 'value') {
  if (schema.type === 'object') {
    assert.ok(value !== null && typeof value === 'object' && !Array.isArray(value), `${path} 必须是对象`);
    const properties = schema.properties ?? {};
    for (const [key, property] of Object.entries(properties)) {
      if (property.required === true) assert.ok(Object.hasOwn(value, key), `${path}.${key} 是必填`);
      if (Object.hasOwn(value, key)) assertConforms(property, value[key], `${path}.${key}`);
    }
    if (schema.additionalProperties === false) {
      for (const key of Object.keys(value)) {
        assert.ok(Object.hasOwn(properties, key), `${path}.${key} 不在 schema 里`);
      }
    }
    return;
  }
  if (schema.type === 'string') assert.equal(typeof value, 'string', `${path} 必须是字符串`);
  if (schema.type === 'boolean') assert.equal(typeof value, 'boolean', `${path} 必须是布尔值`);
}

test('council_ui: a healthy dashboard is reported without spawning', async () => {
  const spawnHarness = createSpawn();
  const fetchHarness = createFetch(() => ({ ok: true, status: 200 }));
  const tool = mount(councilConfig(), { spawn: spawnHarness.spawn, fetch: fetchHarness.fetch });

  const value = await tool.execute({}, {});

  assert.deepEqual(value, { url: 'http://127.0.0.1:8765', running: true, started: false });
  assert.equal(spawnHarness.calls.length, 0, 'health 已通时不许 spawn');
  assert.equal(fetchHarness.calls.length, 1);
  assert.equal(fetchHarness.calls[0].url, 'http://127.0.0.1:8765/api/health');
});

test('council_ui: an unhealthy dashboard is started once and reported as started', async () => {
  let healthy = false;
  const spawnHarness = createSpawn({ onSpawn: () => { healthy = true; } });
  const fetchHarness = createFetch(() => {
    if (!healthy) connectionRefused();
    return { ok: true, status: 200 };
  });
  const tool = mount(councilConfig(), { spawn: spawnHarness.spawn, fetch: fetchHarness.fetch });

  const value = await tool.execute({}, {});

  assert.deepEqual(value, { url: 'http://127.0.0.1:8765', running: true, started: true });
  assert.equal(spawnHarness.calls.length, 1);
  assert.equal(spawnHarness.calls[0].command, 'council');
  assert.equal(spawnHarness.calls[0].args[0], 'serve');
  assert.equal(spawnHarness.children[0].unrefCount, 1, 'detached 子进程必须 unref');
});

test('council_ui: concurrent calls share one start attempt', async () => {
  let healthy = false;
  const spawnHarness = createSpawn({ onSpawn: () => { healthy = true; } });
  const fetchHarness = createFetch(() => {
    if (!healthy) connectionRefused();
    return { ok: true, status: 200 };
  });
  const tool = mount(councilConfig(), { spawn: spawnHarness.spawn, fetch: fetchHarness.fetch });

  const [first, second] = await Promise.all([tool.execute({}, {}), tool.execute({}, {})]);

  assert.equal(spawnHarness.calls.length, 1, '并发调用只能 spawn 一次');
  assert.deepEqual(first, second);
  assert.equal(first.started, true);
});

test('council_ui: a dashboard that never answers settles running=false with the last error', async () => {
  const spawnHarness = createSpawn();
  const fetchHarness = createFetch(connectionRefused);
  const tool = mount(councilConfig({ startTimeoutMs: 500 }), {
    spawn: spawnHarness.spawn,
    fetch: fetchHarness.fetch,
  });

  const startedAt = Date.now();
  const value = await tool.execute({}, {});

  assert.equal(value.running, false);
  assert.equal(value.started, undefined, '失败路径不声明 started');
  assert.match(value.error, /ECONNREFUSED/);
  assert.equal(spawnHarness.calls.length, 1);
  assert.ok(Date.now() - startedAt >= 450, '轮询跑满 startTimeoutMs 才收口');
});

test('council_ui: a synchronous spawn failure settles running=false instead of throwing', async () => {
  const enoent = Object.assign(new Error('spawn council ENOENT'), { code: 'ENOENT' });
  const spawnHarness = createSpawn({ failWith: enoent });
  const fetchHarness = createFetch(connectionRefused);
  const tool = mount(councilConfig(), { spawn: spawnHarness.spawn, fetch: fetchHarness.fetch });

  const value = await tool.execute({}, {});

  assert.equal(value.running, false);
  assert.equal(value.started, undefined);
  assert.match(value.error, /ENOENT/);
  assert.equal(spawnHarness.calls.length, 1);
});

test('council_ui: serve argv carries the loopback address and only passes --config when set', async () => {
  let healthy = false;
  const spawnHarness = createSpawn({ onSpawn: () => { healthy = true; } });
  const fetchHarness = createFetch(() => {
    if (!healthy) connectionRefused();
    return { ok: true, status: 200 };
  });
  const deps = { spawn: spawnHarness.spawn, fetch: fetchHarness.fetch };

  await mount(councilConfig({ port: 8765 }), deps).execute({}, {});
  // 第二个实例换端口：单飞表按 URL 分键，两个实例各起一次，互不复用。
  healthy = false;
  await mount(councilConfig({ port: 9100, configPath: 'C:\\council\\config.toml' }), deps).execute({}, {});

  assert.equal(spawnHarness.calls[0].command, 'council');
  assert.deepEqual(spawnHarness.calls[0].args, ['serve', '-addr', '127.0.0.1:8765']);
  assert.deepEqual(spawnHarness.calls[1].args, [
    'serve',
    '-addr',
    '127.0.0.1:9100',
    '--config',
    'C:\\council\\config.toml',
  ]);
  assert.deepEqual(spawnHarness.calls[0].options, {
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  });
});

test('council_ui: execute values conform to output.schema', async () => {
  const healthySpawn = createSpawn();
  const healthyFetch = createFetch(() => ({ ok: true, status: 200 }));
  const successTool = mount(councilConfig(), {
    spawn: healthySpawn.spawn,
    fetch: healthyFetch.fetch,
  });
  const schema = successTool.output.schema;

  assert.deepEqual(successTool.parameters, {}, '无参数工具给空参数映射');
  assert.deepEqual(Object.keys(schema.properties).sort(), ['error', 'running', 'started', 'url']);
  assert.equal(schema.properties.url.required, true);
  assert.equal(schema.properties.running.required, true);
  assert.equal(schema.properties.started.required, undefined, 'started 只在成功路径出现');
  assert.equal(schema.properties.error.required, undefined, 'error 只在失败路径出现');

  const success = await successTool.execute({}, {});
  assertConforms(schema, success);
  assert.deepEqual(Object.keys(success).sort(), ['running', 'started', 'url']);

  const failingSpawn = createSpawn({ failWith: new Error('spawn council ENOENT') });
  const failingFetch = createFetch(connectionRefused);
  const failureTool = mount(councilConfig({ port: 8766 }), {
    spawn: failingSpawn.spawn,
    fetch: failingFetch.fetch,
  });
  const failure = await failureTool.execute({}, {});
  assertConforms(schema, failure);
  assert.deepEqual(Object.keys(failure).sort(), ['error', 'running', 'url']);
});
