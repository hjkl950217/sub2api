// 校验已打补丁的 dist：两处补丁都在，且 getAvailableModels 的 local-first 行为正确。
// 2026-09-30：「分组模型账号」改为真路由页后，原「入口面板注入」补丁已删，三处变两处。
// 站点升级后 chunk 哈希与压缩函数名都会变，因此按补丁特征自动定位，不写死文件名。
//
// 用法：node fork/frontend/test_patched_dist.mjs <dist 目录>
import fs from 'node:fs';
import path from 'node:path';

const dist = process.argv[2];
if (!dist) {
  console.error('用法: node test_patched_dist.mjs <dist 目录>');
  process.exit(2);
}
const assets = path.join(dist, 'assets');
const html = fs.readFileSync(path.join(dist, 'index.html'), 'utf8');
const entryName = html.match(/assets\/(index-[A-Za-z0-9_-]+\.js)/)?.[1];
if (!entryName) throw new Error('index.html 里找不到入口 chunk');
const bundle = fs.readFileSync(path.join(assets, entryName), 'utf8');

const assert = (cond, msg) => { if (!cond) throw new Error('FAIL: ' + msg); console.log('PASS:', msg); };

// --- 补丁 1：入口 local-first ---
const MARK = 'try{const{data:r}=await n.get(`/admin/accounts/${e}`)';
const i = bundle.indexOf(MARK);
assert(i > 0, '入口含 local-first 补丁特征');
const start = bundle.lastIndexOf('async function ', i);
const name = bundle.slice(start + 'async function '.length).match(/^(\w+)\(/)?.[1];
assert(Boolean(name), '解析出补丁函数名 ' + name);
const end = bundle.indexOf('async function ', i + MARK.length);
assert(end > i, '找到函数结束边界');

const calls = [];
const n = {
  get: async (url) => {
    calls.push(url);
    if (url.endsWith('/models')) return { data: [{ id: 'upstream-1' }, { id: 'upstream-2' }] };
    if (url === '/admin/accounts/1') return { data: { platform: 'openai', credentials: { model_mapping: { a: 'x', b: 'y' } } } };
    if (url === '/admin/accounts/2') return { data: { platform: 'openai', credentials: {} } };
    if (url === '/admin/accounts/3') return { data: { platform: 'anthropic', credentials: { model_mapping: { c: 'z' } } } };
    if (url === '/admin/accounts/4') throw new Error('detail 500');
    throw new Error('unexpected ' + url);
  },
};
const ds = new Function('n', bundle.slice(start, end) + '; return ' + name)(n);
const ids = (r) => r.map((o) => o.id).sort();

let r = await ds(1);
assert(JSON.stringify(ids(r)) === JSON.stringify(['a', 'b', 'x', 'y']), 'openai 有 mapping → 返回键∪值');
assert(!calls.some((u) => u.endsWith('/models')), 'openai 有 mapping → 不请求上游 /models');
assert(r.every((o) => o.id && o.display_name === o.id), '返回项含 id/display_name');

calls.length = 0; r = await ds(2);
assert(calls.includes('/admin/accounts/2/models'), 'openai 无 mapping → 回退 /models');
assert(JSON.stringify(ids(r)) === JSON.stringify(['upstream-1', 'upstream-2']), 'openai 无 mapping → 返回上游结果');

calls.length = 0; r = await ds(3);
assert(calls.includes('/admin/accounts/3/models'), 'anthropic → 仍走 /models');
assert(JSON.stringify(ids(r)) === JSON.stringify(['upstream-1', 'upstream-2']), 'anthropic → 返回值不变');

calls.length = 0; r = await ds(4);
assert(calls.includes('/admin/accounts/4/models'), '详情接口报错 → 回退 /models');
assert(r.length === 2, '回退结果正确');

// --- 补丁 2：渠道监控 ---
const csvName = bundle.match(/ChannelStatusView-[A-Za-z0-9_-]+\.js/)?.[0];
assert(Boolean(csvName), '入口引用到监控 chunk ' + csvName);
const csv = fs.readFileSync(path.join(assets, csvName), 'utf8');
assert(csv.includes('.slice().sort((A,B)=>A.id-B.id)'), '监控 chunk 含按 ID 排序补丁');

// --- 新页面随镜像发布（替代原来的 DOM 注入面板）---
const findFile = (re) => fs.readdirSync(assets).find((f) => re.test(f));
const groupModelChunk = findFile(/^GroupModelAccountsView-.*\.js$/);
assert(Boolean(groupModelChunk), 'dist 里有分组模型账号页 chunk ' + groupModelChunk);

// --- DOM 锚点 ---
const appLayout = findFile(/^AppLayout.*\.js$/);
assert(appLayout && fs.readFileSync(path.join(assets, appLayout), 'utf8').includes('sidebar-nav'), 'AppLayout 仍有 sidebar-nav 锚点');

console.log('\nALL PASS');
