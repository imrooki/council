'use strict';

// 打包前把 council.exe 放进 desktop/vendor/，electron-builder 再从那里
// 以 extraResources 的形式复制到 resources/council.exe。
//
// 查找顺序：COUNCIL_EXE 环境变量 -> 本机默认安装位置。
// 用法：set COUNCIL_EXE=D:\path\council.exe && npm run dist

const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const DEFAULT_COUNCIL_EXE = path.join(os.homedir(), 'bin', 'council.exe');
const root = path.resolve(__dirname, '..');
const vendorDir = path.join(root, 'vendor');
const target = path.join(vendorDir, 'council.exe');

function fail(message) {
  console.error(`[prepare-council] ${message}`);
  process.exit(1);
}

const source = path.resolve(process.env.COUNCIL_EXE || DEFAULT_COUNCIL_EXE);
if (!fs.existsSync(source) || !fs.statSync(source).isFile()) {
  fail(`找不到 council.exe：${source}\n` +
    '请设置环境变量 COUNCIL_EXE 指向已编译的 council.exe，再运行 npm run dist。');
}

fs.mkdirSync(vendorDir, { recursive: true });

const sourceSize = fs.statSync(source).size;
let skip = false;
if (fs.existsSync(target)) {
  const old = fs.statSync(target);
  skip = old.size === sourceSize && old.mtimeMs >= fs.statSync(source).mtimeMs;
}

if (!skip) {
  fs.copyFileSync(source, target);
}

const buffer = fs.readFileSync(target);
const sha256 = crypto.createHash('sha256').update(buffer).digest('hex');
const mb = (buffer.length / 1024 / 1024).toFixed(2);

console.log(`[prepare-council] source: ${source}`);
console.log(`[prepare-council] target: ${target}`);
console.log(`[prepare-council] size:   ${buffer.length} bytes (${mb} MiB)`);
console.log(`[prepare-council] sha256: ${sha256}`);
