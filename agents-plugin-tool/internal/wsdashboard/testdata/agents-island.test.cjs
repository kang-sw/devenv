// Run: node agents-plugin-tool/internal/wsdashboard/testdata/agents-island.test.cjs
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const src = readAssetSource('wsdashboard', 'dashboard-composer.js');
const code = src.slice(src.indexOf('  // --- agents island'), src.indexOf('  function interruptTurn()'));
assert(code.includes('function agProject()'));
function node(cls = '', text = '') {
  const n = { className: cls, textContent: text, attrs: {}, children: [], listeners: {}, style: { setProperty(k, v) { this[k] = v; } }, hidden: false,
    offsetWidth: 320, offsetHeight: 50, childNodes: [],
    getBoundingClientRect() { return { width: 280, height: 30 }; },
    setAttribute(k,v) { this.attrs[k] = String(v); }, getAttribute(k) { return this.attrs[k] || ''; },
    appendChild(x) { this.children.push(x); return x; }, addEventListener(k,f) { this.listeners[k] = f; },
    contains(x) { return x === this; }, focus() {}, querySelector(s) { return this.map?.[s] || null; },
    cloneNode() { return node(this.className, this.textContent); } };
  n.classList = { contains(k) { return n.className.split(' ').includes(k); }, toggle() {}, add() {} };
  let value = text;
  Object.defineProperty(n, 'textContent', { get() { return value; }, set(v) { value = v; n.children = []; } });
  return n;
}
function agent(name, phase, time) {
  const h = node(), a = node('agent running');
  const p = phase ? node('phase ' + phase, phase) : null;
  h.map = { '.amk': node('amk', '*'), '.aname': node('aname', name), '.aline .phase': p,
    '.am time[data-ms]': time ? node('t', time) : null, '.am': node('am') };
  a.map = { 'summary.ahead': h }; return a;
}
const pop = node(), pin = node(), island = node(), compact = node(), idle = node(), badge = node(), close = node();
pop.hidden = true; island.attrs['data-tool-label'] = 'localized tool';
const els = { agpop: pop, agpin: pin, agisland: island, agcompact: compact, agidle: idle, sbadge: badge, agclose: close };
let agents = [];
const peers = node(); peers.childNodes = [{nodeType: 3, nodeValue: 'Peers '}];
peers.map = { '.chip': node('', '1') };
const tree = node(); tree.map = { '.thead .cdone': node('', 'Done 2'), '.pgroup .pghead': peers };
const doc = node(); doc.createElement = () => node(); doc.activeElement = null;
const ctx = vm.createContext({ $: id => els[id], sbadge: badge, doc, window: node(),
  region: () => tree, q: (s, n) => s === '.ag-source .compact-agent' ? agents.filter(a => a.className.includes('running')) : s === '.tgroup .agent' ? agents : s === 'span' ? n.children : [],
  syncBadge() { badge.synced = (badge.synced || 0) + 1; }, fmtTime(t) { t.textContent = 'live'; }, save() {}, load() { return '1'; }, setTimeout(fn) { fn(); }, Date });
vm.runInContext(code, ctx);
assert.equal(island.hidden, true, 'persisted pin does not reveal an unseen island');
assert.equal(pop.hidden, true);
badge.listeners.click(); assert.equal(island.hidden, false, 'badge explicitly opens unseen island');
doc.listeners.keydown({ key: 'Escape', defaultPrevented: false });
assert.equal(island.hidden, true, 'collapsing unseen island hides it again');
agents = [agent('parent','thinking','1s'), agent('nested',null,'2s'), agent('other','permission','3s')];
ctx.agSwapped();
assert.equal(badge.synced, 1, 'tree swaps refresh the session badge');
assert.equal(island.hidden, false);
assert.equal(island.attrs['data-mode'], 'compact');
assert.equal(island.style.height, '50px', 'measured height is a pixel transition target');
assert.equal(island.style.transition, undefined, 'temporary measurement restores transitions');
assert.equal(compact.children[3].textContent, 'Done 2 · Peers 1');
assert.equal(compact.children.length, 4);
assert.deepEqual(compact.children.slice(0,3).map(r => r.children[2].className), ['phase thinking','phase tool','phase permission']);
assert.equal(compact.children[1].children[2].textContent, 'localized tool', 'a row with no pill reads as a tool call');
assert.equal(compact.children[2].children[2].textContent, 'permission', "a waiting approval keeps the full panel's approval label");
assert.equal(compact.children[0].children[3].children[0].textContent, 'live');
const staticAgent = agent('delegate child', 'tool', null);
staticAgent.map['summary.ahead'].map['.am'].appendChild(node('t', '12s'));
agents.push(staticAgent); ctx.agSwapped();
assert.equal(compact.children[3].children[3].textContent, '12s', 'static elapsed span is retained');
const rows = agents;
agents = [agent('squash', 'compacting', '1s'), agent('rest', 'idle', '2s'), agent('call', 'tool', '3s'), agent('say', 'replying', '4s')];
ctx.agSwapped();
assert.deepEqual(compact.children.slice(0,4).map(r => [r.children[2].className, r.children[2].textContent]),
  [['phase compacting','compacting'], ['phase idle','idle'], ['phase tool','tool'], ['phase replying','replying']],
  "compacting and idle keep the full panel's pill, never Tool");
agents = rows; ctx.agSwapped();
compact.listeners.click(); assert.equal(island.attrs['data-mode'], 'full');
assert.equal(pin.attrs['aria-pressed'], 'true');
doc.listeners.keydown({ key: 'Escape', defaultPrevented: false });
assert.equal(island.attrs['data-mode'], 'compact', 'Escape collapses even pinned');
compact.listeners.click(); assert.equal(island.attrs['data-mode'], 'full');
doc.listeners.pointerdown({ target: node() });
assert.equal(island.attrs['data-mode'], 'full', 'a pinned island ignores an outside click');
pin.listeners.click(); assert.equal(pin.attrs['aria-pressed'], 'false');
doc.listeners.pointerdown({ target: island });
assert.equal(island.attrs['data-mode'], 'full', 'a click inside the island keeps it open');
doc.listeners.pointerdown({ target: node() });
assert.equal(island.attrs['data-mode'], 'compact', 'an unpinned island collapses on an outside click');
agents = [node('agent done')]; ctx.agSwapped(); assert.equal(island.attrs['data-mode'], 'idle');
agents = []; ctx.agSwapped(); assert.equal(island.hidden, false, 'seen state survives empty swaps');
badge.listeners.click(); assert.equal(island.attrs['data-mode'], 'full');
agents = [agent('new','replying','5s')]; ctx.agSwapped();
assert.equal(island.attrs['data-mode'], 'full', 'stream swap preserves open mode');
assert.equal(compact.children[0].children[1].textContent, 'new');

// The drawer's approval signal reads this session's tree rows and peer
// rows, not the hidden compact roster (.ag-source), which repeats running
// rows and adds linked delegate sessions' agents.
const drawerSrc = readAssetSource('wsdashboard', 'dashboard-drawer.js');
const drawerWait = drawerSrc.match(/\n  function drawerWait\(\) \{[\s\S]*?\n  \}\n/);
assert(drawerWait, 'dashboard-drawer.js holds drawerWait');
function el(inSource) { return { closest(s) { return s === '.ag-source' && inSource ? {} : null; } }; }
let found = [], selector = '';
const handle = { classList: { toggle(k, v) { handle[k] = v; } } };
// doc.querySelector answers as a selector-only check would: the first match.
const dctx = vm.createContext({ drhandle: handle, drTitle() {}, doc: { querySelector(sel) { selector = sel; return found[0] || null; } }, q(sel) { selector = sel; return found; } });
vm.runInContext(drawerWait[0], dctx);
for (const [what, els, want] of [
  ['no approval', [], false],
  ["a linked delegate's approval in the hidden roster alone", [el(true)], false],
  ["the same agent's approval in the tree too", [el(true), el(false)], true],
  ['a conversation or peer approval', [el(false)], true],
]) {
  found = els; dctx.drawerWait();
  assert.equal(handle.wait, want, 'drawer wait: ' + what);
}
for (const src of ['#r-convlive .permwait', '#r-tree .phase.permission', '#r-tree .brow.perm']) assert(selector.includes(src), 'drawer wait reads ' + src);

// The tab title's approval count (tlPerms) leaves the roster out too: a
// roster copy sits outside any .agent, so counting it adds a lead wait.
const streamSrc = readAssetSource('wsdashboard', 'dashboard-stream.js');
const tlPerms = streamSrc.match(/\n  function tlPerms\(\) \{[\s\S]*?\n  \}\n/);
assert(tlPerms, 'dashboard-stream.js holds tlPerms');
function pill(agent, inSource) { return { closest(s) { return s === '.ag-source' ? (inSource ? {} : null) : agent; } }; }
const rootAgent = {}, waiting = {};
let treePills = [], convWait = false;
const treeRoot = { querySelector(s) { return s === '.agent.root' ? rootAgent : null; } };
const convRoot = { querySelector(s) { return s === '.permwait' && convWait ? {} : null; } };
const tctx = vm.createContext({ tlRoot: name => name === 'tree' ? treeRoot : name === 'convlive' ? convRoot : null, q: () => treePills });
vm.runInContext(tlPerms[0], tctx);
for (const [what, pills, conv, want] of [
  ['one waiting subagent and its roster copy', [pill(waiting, false), pill(null, true)], false, 1],
  ["a linked delegate's wait in the roster alone", [pill(null, true)], false, 0],
  ['a lead conversation wait beside a roster copy', [pill(null, true)], true, 1],
]) {
  treePills = pills; convWait = conv;
  assert.equal(tctx.tlPerms(), want, 'title approvals: ' + what);
}
console.log('agents island contract ok');
