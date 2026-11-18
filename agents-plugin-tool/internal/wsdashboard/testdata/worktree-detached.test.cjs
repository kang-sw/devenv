// Run: node agents-plugin-tool/internal/wsdashboard/testdata/worktree-detached.test.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
const maker = source.match(/  function rpWtMake\(\) \{[\s\S]*?\n  \}\n/);
assert(maker);
const nodes = [], requests = [];
function mk(tag, cls, text) {
  const n = { tag, cls, textContent: text || '', value: '', hidden: false, disabled: false,
    attrs: {}, children: [], listeners: {},
    appendChild(c) { this.children.push(c); return c; },
    setAttribute(k, v) { this.attrs[k] = String(v); },
    getAttribute(k) { return this.attrs[k] || ''; },
    removeAttribute(k) { delete this.attrs[k]; },
    addEventListener(k, fn) { this.listeners[k] = fn; },
    querySelector() { return null; }, focus() {} };
  nodes.push(n); return n;
}
let active;
const ctx = vm.createContext({ window: {}, mk, wsT: k => k, rpPop: () => mk('div'),
  rpBtn: (c, t) => mk('button', c, t), rpInput: (c, t) => mk('input', c, t),
  rpId: () => 'list', rpEnter() {}, railPopPlace() {}, railPopClose() {}, toast() {},
  railErr: () => 'error', rpLive: (p, gen) => p === active && p.gen === gen,
  railPost: (route, body, done) => requests.push({ route, body, done }) });
vm.runInContext(readAssetSource('wsdashboard', 'rail.js'), ctx);
ctx.WsRail = ctx.window.WsRail;
vm.runInContext(maker[0], ctx);
const p = active = ctx.rpWtMake();
p.open('/repo', 'repo');
requests[0].done(200, { current: 'main', bases: ['main', 'other'], free: ['free'], folder: '/default' }, '');
const group = nodes.find(n => n.attrs.role === 'radiogroup');
assert.deepEqual(group.children.map(n => n.attrs['data-mode']), ['new', 'old', 'detached']);
const name = nodes.find(n => n.tag === 'input' && n.textContent === 'js.rail.wt.name');
const base = nodes.find(n => n.tag === 'select');
const go = nodes.find(n => n.tag === 'button' && n.textContent === 'js.rail.wt.create');
function pick(mode) { const button = group.children.find(n => n.attrs['data-mode'] === mode); group.listeners.click({ target: { closest: () => button } }); }
pick('detached');
assert.equal(name.hidden, true);
assert.equal(base.hidden, false);
assert.equal(go.disabled, false);
base.value = 'other';
go.listeners.click();
assert.deepEqual(JSON.parse(JSON.stringify(requests[1].body)), { workspace: '/repo', branch: '', new: false, base: 'other', folder: '/default', detached: true });
requests[1].done(400, {}, '');
pick('new');
assert.equal(name.hidden, false);
assert.equal(go.disabled, true);
name.value = ' topic '; name.listeners.input();
go.listeners.click();
assert.deepEqual(JSON.parse(JSON.stringify(requests[2].body)), { workspace: '/repo', branch: 'topic', new: true, base: 'other', folder: '/default' });
requests[2].done(400, {}, '');
pick('old');
go.listeners.click();
assert.deepEqual(JSON.parse(JSON.stringify(requests[3].body)), { workspace: '/repo', branch: 'free', new: false, base: '', folder: '/default' });
assert.deepEqual(JSON.parse(JSON.stringify(ctx.WsRail.worktreeBody('/repo', { mode: 'detached', base: '', custom: true, folder: ' /custom ', shown: '/default', name: 'ignored', pick: 'ignored' }))),
  { workspace: '/repo', branch: '', new: false, base: '', folder: '/custom', detached: true });
const tables = path.join(__dirname, '../../wsi18n/tables');
for (const file of fs.readdirSync(tables).filter(f => f.endsWith('.json'))) {
  const t = JSON.parse(fs.readFileSync(path.join(tables, file), 'utf8'));
  assert.ok(t['js.rail.wt.detached'], `${file} detached label`);
  if (file === 'en.json') assert.equal(t['js.rail.wt.detached'], 'Detached');
  if (file === 'ko.json') assert.equal(t['js.rail.wt.detached'], '브랜치 없음');
}
console.log('worktree detached contract: ok');
