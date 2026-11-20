// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/permission-controls.test.cjs
// Common permission level controls (dashboard-leads.js): the creation
// popover and the session menu against the production source, on a small
// DOM that keeps parent links, so the native mode row's moves and the
// warning dialog are observable. Checks: absent support sends no level
// (never a silent fallback), the three levels and their native mapping,
// "full" needs the warning's box and only its grant carries
// permission_full_ack (cancel leaves the selection and sends nothing), a
// stream drop (a replaced daemon) makes support unknown until a fresh
// read, a refused selection route is never retried on the legacy route,
// and the session menu's state/selection consumer.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');

const source = readAssetSource('wsdashboard', 'dashboard.js');
const region = source.match(/  var LEAD_MODES = [\s\S]*?\n  function rpLeadMake\(\) \{[\s\S]*?\n  \}\n/);
assert(region, 'production lead choices, permission helpers and creation popover exist');
const sessMaker = source.match(/\n  function rpSessMake\(inRow\) \{[\s\S]*?\n  \}\n/);
assert(sessMaker, 'production session menu maker exists');
const gone = source.match(/\n  function leadGone\(st\) \{.*\}\n/);
const connected = source.match(/\n  function connected\(on\) \{[\s\S]*?\n  \}\n/);
assert(gone && connected, 'leadGone and the stream connected hook exist');
const supHarness = source.match(/\n  function supHarness\(name\) \{[\s\S]*?\n  \}\n/)[0];
const rail = readAssetSource('wsdashboard', 'rail.js');

class Node {
  constructor(tag, cls, text) {
    this.tag = tag; this.cls = cls || ''; this.children = []; this.parentNode = null;
    this.attrs = {}; this.listeners = {}; this.hidden = false; this.disabled = false; this.value = ''; this.text = '';
    if (text != null) this.text = String(text);
    const self = this;
    this.classList = {
      toggle(k, on) { const s = new Set(self.cls.split(' ').filter(Boolean)); if (on === undefined ? !s.has(k) : on) s.add(k); else s.delete(k); self.cls = [...s].join(' '); },
      contains(k) { return self.cls.split(' ').includes(k); },
      add(k) { this.toggle(k, true); }
    };
  }
  appendChild(c) { if (c.parentNode) c.parentNode.removeChild(c); c.parentNode = this; this.children.push(c); return c; }
  insertBefore(c, ref) {
    if (c.parentNode) c.parentNode.removeChild(c);
    const i = ref ? this.children.indexOf(ref) : -1;
    c.parentNode = this;
    if (i < 0) this.children.push(c); else this.children.splice(i, 0, c);
    return c;
  }
  removeChild(c) { const i = this.children.indexOf(c); if (i >= 0) this.children.splice(i, 1); c.parentNode = null; return c; }
  remove() { if (this.parentNode) this.parentNode.removeChild(this); }
  get firstChild() { return this.children[0] || null; }
  get lastChild() { return this.children[this.children.length - 1] || null; }
  replaceChild(c, old) { this.insertBefore(c, old); this.removeChild(old); return old; }
  get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
  set textContent(v) { this.children.forEach(c => { c.parentNode = null; }); this.children = []; this.text = String(v); }
  setAttribute(k, v) { this.attrs[k] = String(v); if (k === 'class') this.cls = String(v); }
  getAttribute(k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; }
  removeAttribute(k) { delete this.attrs[k]; }
  addEventListener(k, f) { (this.listeners[k] = this.listeners[k] || []).push(f); }
  fire(k, e) { (this.listeners[k] || []).forEach(f => f(Object.assign({ target: this, preventDefault() {} }, e))); }
  click() { this.fire('click'); }
  focus() { Node.focused = this; }
  showModal() { this.open = true; }
  close() { this.open = false; }
  matches(sel) {
    const a = /^\[([a-z-]+)\]$/.exec(sel);
    if (a) return this.getAttribute(a[1]) !== null;
    return this.cls.split(' ').includes(sel.replace(/^\./, ''));
  }
  closest(sel) { for (let n = this; n; n = n.parentNode) if (n.matches(sel)) return n; return null; }
  all(pred) { return [...(pred(this) ? [this] : []), ...this.children.flatMap(c => c.all(pred))]; }
  find(pred) { return this.all(pred)[0]; }
}
const shown = n => { for (; n; n = n.parentNode) if (n.hidden) return false; return true; };
const byCls = (root, cls) => root.find(n => n.cls.split(' ').includes(cls));
const plain = v => JSON.parse(JSON.stringify(v));
// selectOf is the option row's select labelled key, wherever the row is.
const selectOf = (root, key) => root.find(n => n.tag === 'select' && n.parentNode.children[0].textContent === key);

const LEVELS = ['read-only', 'workdir-write', 'full'];
const MODES = ['auto', 'default', 'acceptEdits', 'plan', 'dontAsk', 'bypassPermissions'];
function choices(levels = LEVELS, piLevels = LEVELS) {
  return { harnesses: [
    { harness: 'claude', lead: true, available: true, models: [{ id: 'opus' }], efforts: ['high'], permission_modes: MODES, ...(levels ? { permission_levels: levels } : {}) },
    { harness: 'pi', lead: true, available: true, models: [{ id: 'pi-model' }], efforts: ['off'], permission_modes: [], ...(piLevels ? { permission_levels: piLevels } : {}) }
  ] };
}

// world builds one page context: fetches and posts queue until answered.
function world() {
  const fetches = [], posts = [], events = [];
  const body = new Node('body');
  const ctx = {
    mk: (tag, cls, text) => new Node(tag, cls, text),
    wsT: (k, ...a) => a.length ? k + '(' + a.join(',') + ')' : k,
    wsTn: (k, n) => k + '#' + n,
    rpPop: () => { const n = new Node('div', 'rpop'); n.hidden = true; return n; },
    rpBtn: (cls, text) => new Node('button', cls, text),
    rpId: (() => { let i = 0; return () => 'id' + ++i; })(),
    gIcon: () => new Node('svg', 'i'),
    luIcon: () => new Node('svg', 'i'),
    doc: { body }, supChoices: null,
    tkFetch: (route, opt, done) => fetches.push({ route, done }),
    railPost: (route, b, done, timeout) => posts.push({ route, body: plain(b), done, timeout }),
    localStorage: { getItem: () => null, setItem() {} }, sessionStorage: { setItem() {} }, RENAME_NEW: 'rename',
    window: { confirm: () => true }, Date: { now: () => 1 },
    railOpen() {}, railPopPlace() {}, notifAsk() {}, saveDraft() {}, go: href => events.push(['go', href]),
    toast: t => events.push(['toast', t]), railPopClose() { events.push('close'); },
    archErr: (a, st, s) => s, leadDraw() {}, leadFetch() {}, hostedPage: false, handedPage: false,
    rid: 'viewed', lead: { st: null }, rp: { P: null }, q: () => [], sseOn: false
  };
  ctx.rpLive = (p, gen) => p.live !== false && p.gen === gen;
  vm.createContext(ctx);
  vm.runInContext(rail, ctx);
  ctx.WsRail = ctx.window.WsRail;
  vm.runInContext(supHarness + region[0] + sessMaker[0] + gone[0] + connected[0], ctx);
  return { ctx, fetches, posts, events, body,
    choices(c) { const f = fetches.shift(); assert.equal(f.route, '/support/choices'); f.done(200, plain(c)); },
    fail() { const f = fetches.shift(); assert.equal(f.route, '/support/choices'); f.done(0, {}); } };
}

function creation(w, c = choices()) {
  const p = w.ctx.rpLeadMake();
  p.open('/repo', 'repo', new Node('button'));
  p.el.hidden = false;
  w.choices(c);
  const f = {
    p,
    seg: byCls(p.el, 'pmseg'),
    level(l) { const b = f.seg.find(n => n.getAttribute('data-level') === l); assert(b, 'level button ' + l); f.seg.fire('click', { target: b }); },
    levels() { return f.seg.children.map(n => n.getAttribute('data-level')); },
    checked() { return f.seg.children.filter(n => n.attrs['aria-checked'] === 'true').map(n => n.getAttribute('data-level')); },
    harness(k) { const g = p.el.find(n => n.attrs['aria-label'] === 'js.sup.harness'); g.fire('click', { target: g.find(n => n.attrs['data-harness'] === k) }); },
    model: selectOf(p.el, 'js.sup.model'), mode: selectOf(p.el, 'js.lead.mode'), effort: selectOf(p.el, 'js.sup.effort'),
    adv: byCls(p.el, 'pmadv'), opts: byCls(p.el, 'spopts'), off: byCls(p.el, 'pmoff'), err: byCls(p.el, 'pmerr'),
    map: byCls(p.el, 'pmmap'), fine: byCls(p.el, 'pmfine'),
    start: p.el.find(n => n.tag === 'button' && n.cls === 'pbtn go'),
    dialog: () => w.body.find(n => n.tag === 'dialog')
  };
  return f;
}
function dialogParts(d) {
  return { box: d.find(n => n.tag === 'input'), go: d.find(n => n.cls === 'pbtn danger'), cancel: d.find(n => n.cls === 'pbtn') };
}

// 1. Absent support: no permission_levels on the current daemon's list.
// The native row stays where it was and Start keeps the legacy request;
// no level is ever invented, and the unsupported state is said.
{
  const w = world(), f = creation(w, choices(null, null));
  assert.equal(shown(f.seg), false, 'no level segment without positive support');
  assert.equal(shown(f.off), true);
  assert.equal(f.off.textContent, 'js.perm.off.create');
  assert.equal(f.mode.parentNode.parentNode, f.opts, 'native mode row stays inline');
  assert.equal(shown(f.adv), false);
  assert.equal(f.mode.disabled, false);
  assert.equal(f.mode.value, 'auto', 'Claude still starts in auto');
  f.start.click();
  assert.equal(w.posts.length, 1);
  assert.equal(w.posts[0].route, '/agents/leads');
  assert.deepEqual(w.posts[0].body, { root: '/repo', harness: 'claude', plugin_isolation: 'default', permission_mode: 'auto' });
  assert.equal(w.fetches.length, 0, 'the default start needs no preflight');
  f.harness('pi');
  assert.equal(f.off.textContent, 'js.perm.off.plain(Pi)');
}

// 2. Supported: default first and checked; the native mode moves under
// Advanced and stays usable; Start without a level is the legacy request.
{
  const w = world(), f = creation(w);
  assert.equal(shown(f.seg), true);
  assert.deepEqual(f.levels(), ['', 'read-only', 'workdir-write', 'full']);
  assert.deepEqual(f.checked(), ['']);
  assert.equal(f.seg.all(n => n.cls === 'pmtag' || n.cls.includes('derived')).length, 0, 'no level is tagged derived on creation');
  assert.equal(f.mode.parentNode.parentNode, f.adv, 'native mode lives under Advanced');
  assert.equal(shown(f.adv), true);
  assert.equal(f.mode.disabled, false, 'native mode still editable by default');
  assert.deepEqual(f.map.children.map(n => n.tag + ':' + n.textContent), ['span:js.perm.map.default(', 'span:Claude', 'code:auto', 'span:)']);
  assert.equal(shown(f.fine), true);
  f.mode.value = 'plan';
  f.mode.fire('change');
  f.start.click();
  assert.equal(w.fetches.length, 0);
  assert.deepEqual(w.posts[0].body, { root: '/repo', harness: 'claude', plugin_isolation: 'default', permission_mode: 'plan' }, 'native controls keep working under Advanced');
}

// 3. The three levels: each decides the native mode (the row fixed, the
// user's own pick kept), Start re-reads support first, and the request goes
// only to the selection route, without permission_mode or acknowledgement.
for (const [level, native] of [['read-only', 'plan'], ['workdir-write', 'acceptEdits']]) {
  const w = world(), f = creation(w);
  f.mode.value = 'default';
  f.mode.fire('change');
  f.level(level);
  assert.deepEqual(f.checked(), [level]);
  assert.equal(f.mode.value, native);
  assert.equal(f.mode.disabled, true, 'the level owns the native mode');
  assert.equal(shown(byCls(f.adv, 'pmown')), true);
  f.start.click();
  assert.equal(w.posts.length, 0, 'nothing is sent before the fresh support read');
  assert.equal(w.fetches.length, 1, 'Start re-reads the choices');
  w.choices(choices());
  assert.equal(w.posts.length, 1);
  assert.equal(w.posts[0].route, '/agents/leads/permission');
  assert.deepEqual(w.posts[0].body, { root: '/repo', harness: 'claude', plugin_isolation: 'default', permission_level: level });
  w.posts[0].done(201, { id: 'made' }, '');
  assert.deepEqual(w.events.slice(-1), [['go', '/s/made']]);
  // Back to the default the user's own native pick returns.
  const g = creation(world());
  g.mode.value = 'default'; g.mode.fire('change');
  g.level(level); g.level('');
  assert.equal(g.mode.value, 'default');
  assert.equal(g.mode.disabled, false);
}
{
  // Pi: no native mode, every level advisory, the level still sent.
  const w = world(), f = creation(w);
  f.harness('pi');
  assert.deepEqual(f.levels(), ['', 'read-only', 'workdir-write', 'full']);
  f.level('workdir-write');
  assert.equal(shown(f.adv), false, 'Pi has no native mode to disclose');
  assert.deepEqual(f.map.children.map(n => n.cls + ':' + n.textContent), [':js.perm.map.pi(Pi)', 'pmq advisory:js.perm.q.advisory']);
  f.start.click();
  w.choices(choices());
  assert.deepEqual(w.posts[0].body, { root: '/repo', harness: 'pi', plugin_isolation: 'default', permission_level: 'workdir-write' });
}

// 4. Full: Start opens the warning and sends nothing; cancel leaves the
// selection (still full) and sends nothing; the grant is off until the box
// is ticked; only the grant's request carries permission_full_ack.
{
  const w = world(), f = creation(w);
  f.level('full');
  assert.equal(f.start.textContent, 'js.perm.start.full');
  assert.equal(f.mode.value, 'bypassPermissions');
  f.start.click();
  let d = f.dialog();
  assert(d && d.open, 'the warning opens');
  assert.equal(w.posts.length + w.fetches.length, 0, 'opening the warning sends nothing');
  let parts = dialogParts(d);
  assert.equal(parts.go.disabled, true, 'grant disabled until the box is ticked');
  assert.equal(Node.focused, parts.cancel, 'focus starts on cancel');
  assert(d.textContent.includes('bypassPermissions'), 'the warning names the native mode');
  parts.go.click();
  assert.equal(d.open, true, 'an unticked grant does nothing');
  parts.cancel.click();
  assert.equal(d.open, false);
  assert.equal(w.posts.length + w.fetches.length, 0, 'cancel sends nothing');
  assert.deepEqual(f.checked(), ['full'], 'cancel leaves the prior selection unchanged');
  // Escape (the dialog's cancel event) is a cancel too.
  f.start.click();
  d.fire('cancel');
  assert.equal(d.open, false);
  assert.equal(w.posts.length + w.fetches.length, 0);
  // Confirm: tick, grant, fresh support read, then the acknowledged request.
  f.start.click();
  parts = dialogParts(d);
  assert.equal(parts.box.checked, false, 'the box starts unticked on every open');
  assert.equal(parts.go.disabled, true);
  parts.box.checked = true; parts.box.fire('change');
  assert.equal(parts.go.disabled, false);
  parts.go.click();
  assert.equal(d.open, false);
  assert.equal(w.posts.length, 0);
  w.choices(choices());
  assert.equal(w.posts[0].route, '/agents/leads/permission');
  assert.deepEqual(w.posts[0].body, { root: '/repo', harness: 'claude', plugin_isolation: 'default', permission_level: 'full', permission_full_ack: true });
  // leadBody never acknowledges without the grant, nor for another level.
  assert.equal('permission_full_ack' in w.ctx.WsRail.leadBody('/r', { harness: 'claude', level: 'full' }), false);
  assert.equal('permission_full_ack' in w.ctx.WsRail.leadBody('/r', { harness: 'claude', level: 'read-only', ack: true }), false);
  assert.equal('permission_mode' in w.ctx.WsRail.leadBody('/r', { harness: 'claude', level: 'read-only', mode: 'auto' }), false);
  // Closing the popover cancels an open warning (after a failed start the
  // grant is asked for again, never remembered).
  w.posts[0].done(500, {}, 'launch failed');
  f.start.click();
  assert.equal(d.open, true);
  f.p.close();
  assert.equal(d.open, false);
  assert.equal(w.posts.length, 1);
}

// 5. Reconnect / daemon replacement: a stream drop makes the held support
// unknown; no level is offered until a fresh read, and a fresh read that
// lacks permission_levels is unsupported: the picked level is not sent,
// and the user may start on a native mode only by an explicit, relabelled
// Start (never a silent fallback).
{
  const w = world(), f = creation(w);
  f.level('read-only');
  w.ctx.rp.P = f.p;
  w.ctx.connected(false);
  assert.equal(w.ctx.permEpoch, 1, 'a drop advances the support epoch');
  assert.equal(w.ctx.permLevels('claude'), null, 'support read before the drop no longer counts');
  assert.equal(shown(f.seg), true, 'the picked level stays visible');
  assert(f.seg.children.every(n => n.disabled), 'but cannot be changed while support is unknown');
  w.ctx.connected(true);
  assert.equal(w.fetches.length, 1, 'reconnecting re-reads the choices');
  w.choices(choices(null, null));
  assert.equal(w.posts.length, 0);
  assert.equal(shown(f.seg), false);
  assert.equal(f.off.textContent, 'js.perm.off.again(js.perm.level.readonly)');
  assert.equal(f.start.textContent, 'js.perm.start.native');
  assert.equal(f.mode.disabled, false);
  assert.equal(f.mode.parentNode.parentNode, f.opts, 'the native row is back inline for a manual pick');
  f.start.click();
  assert.equal(w.fetches.length, 0);
  assert.equal(w.posts[0].route, '/agents/leads', 'only the explicit, relabelled Start uses the legacy route');
  assert.equal('permission_level' in w.posts[0].body, false);
}
{
  // A preflight at Start that finds no support (a daemon swapped since the
  // popover's read) sends nothing at all.
  const w = world(), f = creation(w);
  f.level('workdir-write');
  f.start.click();
  w.choices(choices(null, null));
  assert.equal(w.posts.length, 0, 'unsupported after the fresh read: nothing sent');
  assert.equal(f.off.textContent, 'js.perm.off.again(js.perm.level.workdir)');
  // A failed read is unknown support, not support.
  const v = world(), g = creation(v);
  g.level('read-only');
  g.start.click();
  v.fail();
  assert.equal(v.posts.length, 0);
}

// 6. A daemon without the selection route (old router: 404/405) created
// nothing: the refusal is shown, Start becomes a re-check that sends
// nothing, and nothing is resent on /agents/leads.
for (const status of [404, 405]) {
  const w = world(), f = creation(w);
  f.level('read-only');
  f.start.click();
  w.choices(choices());
  w.posts[0].done(status, {}, 'not found');
  assert.equal(w.posts.length, 1, 'no legacy retry');
  assert.equal(shown(f.err), true);
  assert(f.err.textContent.startsWith('js.perm.refused(' + status + ')'));
  assert.equal(f.start.textContent, 'js.perm.recheck');
  assert.equal(w.ctx.permLevels('claude'), null, 'a refusal makes support unknown');
  f.start.click();
  assert.equal(w.posts.length, 1, 're-check sends no selection');
  w.choices(choices());
  assert.equal(shown(f.err), false);
  assert.equal(f.start.textContent, 'js.lead.start', 'support back: the user starts again explicitly');
  assert.equal(w.posts.length, 1);
}

// 7. The session menu: intent, native mode and evidence apart; a legacy
// derived level is tagged, not checked; changes go to settings/permission
// after a fresh read; full warns and its cancel sends nothing; the native
// mode under Advanced still posts the legacy settings route.
function menu(w, st, c = choices()) {
  const p = w.ctx.rpSessMake(true);
  w.ctx.rp.P = { sess: p };
  p.open('s1');
  const ch = w.fetches.find(x => x.route === '/support/choices');
  w.fetches.splice(w.fetches.indexOf(ch), 1);
  ch.done(200, plain(c));
  const sf = w.fetches.find(x => x.route === '/agents/s1');
  w.fetches.splice(w.fetches.indexOf(sf), 1);
  sf.done(200, st);
  const seg = byCls(p.el, 'pmseg');
  return { p, seg, state: byCls(p.el, 'pmstate'), chg: byCls(p.el, 'pmchg'), adv: byCls(p.el, 'pmadv'), mode: selectOf(p.el, 'js.lead.mode'),
    level(l) { seg.fire('click', { target: seg.find(n => n.getAttribute('data-level') === l) }); },
    checked() { return seg.children.filter(n => n.attrs['aria-checked'] === 'true').map(n => n.getAttribute('data-level')); } };
}
const base = { id: 's1', lead: true, backend: 'claude', can_control: true, status: 'idle', model: 'opus', effort: 'high' };
const rowsOf = m => m.state.children.map(n => n.textContent);
{
  // Legacy-derived Claude auto, process running and reporting auto.
  const w = world(), m = menu(w, { ...base, permission_mode: 'auto', active_mode: 'auto', process: true, permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' });
  assert.deepEqual(rowsOf(m), ['js.perm.k.intent', 'js.perm.level.workdirjs.perm.src.derived', 'js.perm.k.native', 'Claudeautojs.perm.native.reported', 'js.perm.k.evidence', 'js.perm.q.advisoryjs.perm.ev.advisory(Claude)']);
  assert.deepEqual(m.checked(), [], 'a derived level is not a selection');
  assert(m.seg.find(n => n.getAttribute('data-level') === 'workdir-write').cls.includes('derived'));
  assert.equal(m.mode.disabled, false, 'legacy sessions keep the native mode editable');
  assert.equal(m.mode.parentNode.parentNode, m.adv);
  m.mode.value = 'plan';
  m.mode.fire('change');
  assert.equal(w.posts[0].route, '/agents/s1/settings');
  assert.deepEqual(w.posts[0].body, { permission_mode: 'plan' }, 'the native control still posts the legacy route');
}
{
  // Explicit read-only: plan reported -> partial; asked but not reported
  // -> pending; a different reported mode -> mismatch, advisory.
  const ro = { ...base, permission_mode: 'plan', permission_level: 'read-only', permission_source: 'explicit' };
  let m = menu(world(), { ...ro, process: true, active_mode: 'plan', permission_enforcement: 'partial' });
  assert.deepEqual(rowsOf(m).slice(3), ['Claudeplanjs.perm.native.reported', 'js.perm.k.evidence', 'js.perm.q.partialjs.perm.ev.partial(Claude)']);
  assert.deepEqual(m.checked(), ['read-only']);
  assert.equal(m.mode.disabled, true, 'an explicit level owns the native mode');
  m = menu(world(), { ...ro, process: true, permission_enforcement: 'advisory' });
  assert.deepEqual(rowsOf(m).slice(3), ['Claudeplanjs.perm.native.next', 'js.perm.k.evidence', 'js.perm.q.pendingjs.perm.ev.pending(Claude)'], 'no observation is pending, never enforced');
  m = menu(world(), { ...ro, process: true, active_mode: 'acceptEdits', permission_enforcement: 'advisory' });
  assert.deepEqual(rowsOf(m).slice(3), ['ClaudeacceptEditsjs.perm.native.asked(plan)', 'js.perm.k.evidence', 'js.perm.q.advisoryjs.perm.ev.mismatch(Claude)']);
  // Pi: no fabricated native mode.
  m = menu(world(), { ...base, backend: 'pi', permission_level: 'workdir-write', permission_source: 'explicit', permission_enforcement: 'advisory' });
  assert.deepEqual(rowsOf(m).slice(3), ['js.perm.native.none(Pi)', 'js.perm.k.evidence', 'js.perm.q.advisoryjs.perm.ev.plain(Pi)']);
}
{
  // Lowering: fresh read, then settings/permission without acknowledgement.
  const w = world(), st = { ...base, permission_mode: 'auto', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' };
  const m = menu(w, st);
  m.level('read-only');
  assert.equal(w.posts.length, 0);
  w.choices(choices());
  assert.equal(w.posts[0].route, '/agents/s1/settings/permission');
  assert.deepEqual(w.posts[0].body, { permission_level: 'read-only' });
  // Full: cancel sends nothing and the segment shows the session as it was.
  const v = world(), n = menu(v, { ...st, permission_mode: 'plan', permission_level: 'read-only', permission_source: 'explicit' });
  n.level('full');
  const d = v.body.find(x => x.tag === 'dialog'), parts = dialogParts(d);
  assert.equal(d.open, true);
  assert(d.textContent.includes('js.perm.full.title.change'));
  parts.cancel.click();
  assert.equal(v.posts.length + v.fetches.length, 0, 'cancel sends nothing');
  assert.deepEqual(n.checked(), ['read-only'], 'cancel leaves the prior selection');
  n.level('full');
  parts.box.checked = true; parts.box.fire('change');
  parts.go.click();
  v.choices(choices());
  assert.deepEqual(v.posts[0].body, { permission_level: 'full', permission_full_ack: true });
  assert.equal(v.posts[0].route, '/agents/s1/settings/permission');
  // A refused route changes nothing and is not resent anywhere.
  v.posts[0].done(404, {}, 'no route');
  assert.equal(v.posts.length, 1);
  assert.equal(v.ctx.permLevels('claude'), null, 'support is read again after a refusal');
}
{
  // Absent support: the state shows, the change segment does not, the
  // native mode is back inline and still posts the legacy route.
  const w = world(), m = menu(w, { ...base, permission_mode: 'auto', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' }, choices(null, null));
  assert.equal(shown(m.chg), false);
  assert(m.state.textContent.endsWith('js.perm.off.settings'));
  assert.equal(shown(m.adv), false);
  assert.equal(m.mode.disabled, false);
  m.mode.value = 'acceptEdits'; m.mode.fire('change');
  assert.deepEqual(w.posts[0].body, { permission_mode: 'acceptEdits' });
  // A worker shows its state but offers no selection.
  const x = menu(world(), { ...base, lead: false, is_child: true, backend: 'pi', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' });
  assert.equal(shown(x.state), true);
  assert.equal(shown(x.chg), false);
  // A preflight that finds the level gone sends nothing.
  const v = world(), n = menu(v, { ...base, permission_mode: 'auto', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' });
  n.level('read-only');
  v.choices(choices(null, null));
  assert.equal(v.posts.length, 0);
}
console.log('permission control cases passed');
