'use strict';

// 版本比较：桌面版要判断「我连上的那个看板，是不是和我同一版」。
//
// 为什么必须有：一个旧版的 `council serve -rw` 可能还在 8765 上跑（比如两天前
// 忘了关）。桌面版会 attach 到它，于是用户装了新版却看到旧界面、旧 schema ——
// 而且因为写锁在旧进程手里，新版连自己起一个都做不到。这里把「不是同一版」
// 变成一句明确的话，而不是一个安静的错误。
//
// 版本串形如 `0.2.0+git.9e5b3c9`（干净工作区）或 `0.2.0+git.9e5b3c9.dirty`。

// parseVersion 拆出 base 与 git 提交哈希；认不出时给空字段。
function parseVersion(raw) {
  if (typeof raw !== 'string') return { base: '', git: '', raw: '' };
  const text = raw.trim();
  const match = /^([0-9A-Za-z.\-]+)\+git\.([0-9a-fA-F]{4,40})(\.dirty)?$/.exec(text);
  if (!match) return { base: text, git: '', raw: text };
  return { base: match[1], git: match[2].toLowerCase(), raw: text };
}

// versionsMatch 判断两个版本串是不是同一次构建。
//
// 两边都有 git 哈希时以哈希为准（0.3.0+git.abc 与 0.3.0+git.def 是不同的
// 构建）；有一边没有哈希时退回 base 版本比较（认不出就别拦人）。
function versionsMatch(a, b) {
  const left = parseVersion(a);
  const right = parseVersion(b);
  if (!left.raw || !right.raw) return true;
  if (left.git && right.git) return left.git === right.git;
  return left.base === right.base;
}

module.exports = { parseVersion, versionsMatch };
