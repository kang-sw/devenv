// Run: node agents-plugin-tool/internal/wsdashboard/testdata/agents-island.test.cjs
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const hudSrc = readAssetSource('wsdashboard', 'dashboard-hud.js');
const compSrc = readAssetSource('wsdashboard', 'dashboard-composer.js');
const code = hudSrc + '\n' + compSrc.slice(compSrc.indexOf('  // --- agents island'), compSrc.indexOf('  function interruptTurn()'));
assert(code.includes('function hudProject(') && code.includes('function agProject()'));

function node(tag = 'div', cls = '', text = '') {
  const n = {
    tagName: tag.toUpperCase(), className: cls, attrs: {}, children: [], listeners: {},
    style: { setProperty(k, v) { this[k] = v; } }, hidden: false, open: false,
    offsetWidth: 140, offsetHeight: 56, childNodes: [], parentNode: null, nextElementSibling: null,
    getBoundingClientRect() { return { left: 100, top: 20, right: 240, bottom: 76, width: 140, height: 56 }; },
    setAttribute(k, v) { this.attrs[k] = String(v); },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : ''; },
    appendChild(x) { x.parentNode = this; if (this.children.length) this.children[this.children.length - 1].nextElementSibling = x; this.children.push(x); return x; },
    addEventListener(k, f) { (this.listeners[k] ||= []).push(f); },
    contains(x) { return x === this || this.children.some(c => c.contains && c.contains(x)); },
    focus() { n.focused = true; }, scrollIntoView() { n.scrolled = true; },
    closest(s) {
      let cur = this;
      while (cur) {
        if (s.startsWith('.') && cur.className.split(/\s+/).includes(s.slice(1))) return cur;
        cur = cur.parentNode;
      }
      return null;
    },
    querySelector(s) {
      if (this.map && this.map[s]) return this.map[s];
      const matchPart = (el, part) => {
        if (part.startsWith('.')) {
          const classes = part.slice(1).split('.');
          const have = el.className ? el.className.split(/\s+/) : [];
          return classes.every(c => have.includes(c));
        }
        const [tag, ...classes] = part.split('.');
        if (tag && el.tagName !== tag.toUpperCase()) return false;
        const have = el.className ? el.className.split(/\s+/) : [];
        return classes.every(c => have.includes(c));
      };
      const parts = s.trim().split(/\s+/);
      const findFirst = (root, idx) => {
        for (const c of root.children) {
          if (matchPart(c, parts[idx])) {
            if (idx === parts.length - 1) return c;
            const nested = findFirst(c, idx + 1);
            if (nested) return nested;
          }
          const deep = findFirst(c, idx);
          if (deep) return deep;
        }
        return null;
      };
      return findFirst(this, 0);
    }
  };
  n.classList = {
    contains(k) { return n.className.split(/\s+/).includes(k); },
    toggle(k, force) {
      const set = new Set(n.className ? n.className.split(/\s+/) : []);
      const on = force === undefined ? !set.has(k) : !!force;
      if (on) set.add(k); else set.delete(k);
      n.className = [...set].join(' ');
    },
    add(k) { n.classList.toggle(k, true); },
    remove(k) { n.classList.toggle(k, false); }
  };
  let value = text;
  Object.defineProperty(n, 'textContent', {
    get() { return value; },
    set(v) { value = String(v); n.children = []; }
  });
  return n;
}

const pop = node('div', 'agpop'), pin = node('button', 'agpin'), island = node('div', 'agisland');
const compact = node('div', 'agcompact'), idle = node('div', 'agidle'), badge = node('button', 'sbadge'), close = node('button', 'agclose');
const hudCanvas = node('div', 'hud-canvas'), hudLines = node('svg', 'hud-lines'), hudRoots = node('div', 'hud-roots');
const expandBtn = node('button', 'hud-expand-btn'), summaryText = node('span', 'hud-summary-text');
const tooltip = node('div', 'hud-tooltip'), convo = node('div', 'convo');
tooltip.hidden = true; pop.hidden = true;
convo.getBoundingClientRect = () => ({ left: 0, top: 0, right: 640, bottom: 480, width: 640, height: 480 });
expandBtn.appendChild(summaryText);
hudCanvas.appendChild(hudLines); hudCanvas.appendChild(hudRoots);
compact.appendChild(hudCanvas); compact.appendChild(expandBtn);

const els = { agpop: pop, agpin: pin, agisland: island, agcompact: compact, agidle: idle, sbadge: badge, agclose: close, 'hud-tooltip': tooltip, convo };
let hudPayload = { lead: 'coordinator', completed: 0, peers: 0, roots: [] };
let treeDetails = [], treeAgents = [];
const hudDataEl = node('script', 'ag-hud-data');
const tree = {
  querySelector(s) {
    if (s === '.ag-source .ag-hud-data') { hudDataEl.textContent = JSON.stringify(hudPayload); return hudDataEl; }
    return null;
  }
};
const doc = node('div');
doc.createElement = tag => node(tag);
doc.createElementNS = (_, tag) => node(tag);
doc.createTextNode = txt => node('#text', '', txt);
doc.activeElement = null;

const navigated = [];
const win = node('window');
win.innerWidth = 800; win.innerHeight = 600;
const ctx = vm.createContext({
  $: id => els[id] || null, sbadge: badge, doc, window: win, SVGNS: 'http://www.w3.org/2000/svg',
  region: () => tree,
  q(s, el) {
    if (s === 'details[data-key]' && el === tree) return treeDetails;
    if (s === 'details[data-key]' && el === pop) return treeDetails;
    if (s === '.tgroup .agent') return treeAgents;
    return [];
  },
  wsT(k, ...args) { return args.length ? `${k}:${args.join(',')}` : k; },
  fmtTime(t) { t.textContent = 'live'; },
  dur(ms) { return Math.round(ms / 1000) + 's'; },
  go(href) { navigated.push(href); },
  syncBadge() { badge.synced = (badge.synced || 0) + 1; },
  save() {}, load() { return '1'; },
  requestAnimationFrame(fn) { fn(); return 1; }, setTimeout(fn) { fn(); }, Date
});
vm.runInContext(code, ctx);

assert.equal(island.hidden, true, 'persisted pin does not reveal an unseen island');
assert.equal(pop.hidden, true);
badge.listeners.click[0]();
assert.equal(island.hidden, false, 'badge explicitly opens unseen island');
doc.listeners.keydown[0]({ key: 'Escape', defaultPrevented: false });
assert.equal(island.hidden, true, 'collapsing unseen island hides it again');

// Alive-idle + holding + permission + failed + linked + overflow HUD forest
hudPayload = {
  lead: 'coord', completed: 2, peers: 5,
  roots: [
    { id: 'a1', label: 'alpha', harness: 'codex', state: 'holding', model: 'gpt-5.4', effort: 'high', elapsedMs: 12000,
      children: [{ id: 'a2', label: 'beta', harness: 'claude', state: 'permission', replyId: 'r2', linked: true, task: 'bash ls' }] },
    { id: 'a3', label: 'gamma', harness: 'agy', state: 'idle', overflow: 2 },
    { id: 'a4', label: 'delta', harness: 'pi', state: 'failed' }
  ]
};
const d1 = node('details'); d1.setAttribute('data-key', 'agent-a1');
const s1 = node('summary', 'ahead'); s1.appendChild(node('span', 'acap', 'waiting on beta')); d1.appendChild(s1);
const liveOl = node('ol', 'brows live'); const br = node('li', 'brow live');
br.appendChild(node('span', 'tool', 'Read')); br.appendChild(node('span', 'args', 'foo.go')); liveOl.appendChild(br);
const agRow = node('div', 'agent running'); agRow.appendChild(d1); agRow.appendChild(liveOl);
pop.appendChild(agRow);
treeDetails = [d1]; treeAgents = [agRow];

ctx.agSwapped();
assert.equal(badge.synced, 1);
assert.equal(island.hidden, false);
assert.equal(island.attrs['data-mode'], 'compact', 'holding/alive-idle forest opens compact HUD');
assert.equal(island.style.height, '56px');
assert.equal(island.style['--ag-cw'], '140px', 'agSize uses measured natural width, not fixed 320px');
assert.equal(summaryText.textContent, 'js.hud.summary:2,5');
const buttons = [];
(function walk(el) { if (el.classList?.contains('hud-node')) buttons.push(el); (el.children || []).forEach(walk); })(hudRoots);
assert.equal(buttons.length, 4);
assert.equal(buttons[1].querySelector('.hud-badge').textContent, '!');
assert.equal(buttons[3].querySelector('.hud-badge').textContent, '\u00d7');
assert.equal(buttons[2].parentNode.querySelector('.hud-overflow').textContent, '+2');

// Own-row task fallback from #r-tree and tooltip display
hudRoots.listeners.mouseover[0]({ target: buttons[0] });
assert.equal(tooltip.hidden, false);
assert.equal(tooltip.querySelector('.tt-task').textContent, 'Read foo.go');
assert.equal(tooltip.querySelector('.tt-hint').textContent, 'js.hud.hint.details');

// Container click on #agcompact does NOT open #agpop; expand button and unlinked node DO
assert.equal((compact.listeners.click || []).length, 0, '#agcompact container has no click handler');
expandBtn.listeners.click[0]();
assert.equal(island.attrs['data-mode'], 'full');
assert.equal(expandBtn.attrs['aria-expanded'], 'true');
assert.equal(tooltip.hidden, true, 'opening full view hides compact tooltip');
doc.listeners.keydown[0]({ key: 'Escape', defaultPrevented: false });
assert.equal(island.attrs['data-mode'], 'compact');

// Unlinked node click opens #agpop and focuses <details data-key="agent-a1">
hudRoots.listeners.click[0]({ target: buttons[0] });
assert.equal(island.attrs['data-mode'], 'full');
assert.equal(d1.open, true);
assert.equal(s1.focused, true);
doc.listeners.keydown[0]({ key: 'Escape', defaultPrevented: false });

// Linked node click navigates to /s/<replyId>
hudRoots.listeners.click[0]({ target: buttons[1] });
assert.deepEqual(navigated, ['/s/r2']);

// Empty visible HUD forest falls back to idle pill using HUD.Completed
hudPayload = { lead: 'coord', completed: 4, peers: 2, roots: [] };
ctx.agSwapped();
assert.equal(island.attrs['data-mode'], 'idle');
assert.equal(idle.querySelector('.agidle-label').textContent, 'js.hud.done:4');

// Preserve drawerWait and tlPerms exclusions of .ag-source
const drawerSrc = readAssetSource('wsdashboard', 'dashboard-drawer.js');
const drawerWait = drawerSrc.match(/\n  function drawerWait\(\) \{[\s\S]*?\n  \}\n/);
assert(drawerWait, 'dashboard-drawer.js holds drawerWait');
function el(inSource) { return { closest(s) { return s === '.ag-source' && inSource ? {} : null; } }; }
let found = [], selector = '';
const handle = { classList: { toggle(k, v) { handle[k] = v; } } };
const dctx = vm.createContext({ drhandle: handle, drfold: null, drTitle() {}, doc: { querySelector(sel) { selector = sel; return found[0] || null; } }, q(sel) { selector = sel; return found; } });
vm.runInContext(drawerWait[0], dctx);
found = [el(true)]; dctx.drawerWait(); assert.equal(handle.wait, false);
found = [el(true), el(false)]; dctx.drawerWait(); assert.equal(handle.wait, true);
for (const src of ['#r-convlive .permwait', '#r-tree .phase.permission', '#r-tree .brow.perm']) assert(selector.includes(src));

const streamSrc = readAssetSource('wsdashboard', 'dashboard-stream.js');
const tlPerms = streamSrc.match(/\n  function tlPerms\(\) \{[\s\S]*?\n  \}\n/);
assert(tlPerms, 'dashboard-stream.js holds tlPerms');
function pill(agent, inSource) { return { closest(s) { return s === '.ag-source' ? (inSource ? {} : null) : agent; } }; }
const tctx = vm.createContext({ tlRoot: name => name === 'tree' ? { querySelector: () => ({}) } : null, q: () => [pill({}, false), pill(null, true)] });
vm.runInContext(tlPerms[0], tctx);
assert.equal(tctx.tlPerms(), 1);
console.log('agents island contract ok');
