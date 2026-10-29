'use strict';

// 深链解析：council://conversation/<id> → 会话 id。
//
// 单独抽出来是因为它是**安全边界**：解析结果会被拼进窗口地址，放行任意
// 字符串等于给外部一个导航入口。这里只认一种形状，其余一律返回空串；
// desktop/test/deeplink.test.js 用 node --test 钉住这些反例。

const PROTOCOL = 'council';
const CONVERSATION_HOST = 'conversation';
// 会话 id 是十六进制串（32 位）；这里放宽到 64 个安全字符，但拒绝一切
// 路径分隔符、点、百分号与空白，避免拼出 ../ 或二次编码。
const SAFE_ID = /^[A-Za-z0-9_-]{1,64}$/;

// incomingDeepLink 从一次启动的 argv 里找深链（Windows 走这条路）。
function incomingDeepLink(argv) {
  if (!Array.isArray(argv)) return '';
  for (const arg of argv) {
    if (typeof arg === 'string' && arg.toLowerCase().startsWith(`${PROTOCOL}://`)) {
      return arg;
    }
  }
  return '';
}

// conversationIDFromDeepLink 只认 council://conversation/<id>。
function conversationIDFromDeepLink(raw) {
  if (!raw || typeof raw !== 'string') return '';
  let parsed;
  try {
    parsed = new URL(raw);
  } catch {
    return '';
  }
  if (parsed.protocol !== `${PROTOCOL}:`) return '';
  if (parsed.hostname.toLowerCase() !== CONVERSATION_HOST) return '';
  const id = parsed.pathname.replace(/^\/+/, '');
  if (!SAFE_ID.test(id)) return '';
  return id;
}

// shouldHandleDeepLink 去重：同一条深链在极短时间里可能被投递多次。
//
// 原因在单实例锁的重试：拿不到锁时每次重试都会触发一次 second-instance，
// 于是同一个 argv 被处理三次 —— 真机上日志里就是三条一模一样的
// 「deep link switched url=…」（页面被重新加载三次）。
//
// state 由调用方持有：{ raw: string, at: number }。
function shouldHandleDeepLink(state, raw, now, windowMs = 3000) {
  if (!raw) return false;
  if (!state || typeof state !== 'object') return true;
  if (state.raw !== raw) return true;
  if (!Number.isFinite(state.at)) return true;
  return now - state.at > windowMs;
}

module.exports = { PROTOCOL, incomingDeepLink, conversationIDFromDeepLink, shouldHandleDeepLink };
