const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const js = fs.readFileSync(path.join(__dirname, 'assets/dashboard-drawer.js'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, 'assets/dashboard-drawer.css'), 'utf8');
function section(start, end) {
  const a = js.indexOf(start), b = js.indexOf(end, a);
  assert.ok(a >= 0 && b > a);
  return js.slice(a, b);
}

test('only an uncustomized wide panels drawer defaults to 60vh', () => {
  const stored = {};
  const c = vm.createContext({ window: { innerWidth: 1920, innerHeight: 1080 },
    load: () => null, save: (k, v) => { stored[k] = v; }, drMax: () => 700 });
  vm.runInContext(section('  var DR_OPEN =', '  var sbody ='), c);
  vm.runInContext(section('  function drClamp(h)', '  // drApply lays'), c);
  assert.equal(c.drHeight(), 648);
  assert.equal(c.drClamp(c.drHeight()), 648);
  c.window.innerWidth = 759;
  assert.equal(c.drHeight(), 320);
  c.window.innerWidth = 760;
  assert.equal(c.drHeight(), 648);
  c.drMode = 'git'; assert.equal(c.drHeight(), 320);
  c.drMode = 'search'; assert.equal(c.drHeight(), 320);
  c.drMode = 'panels'; c.drSetH(400);
  assert.equal(c.drHeight(), 400);
  c.window.innerWidth = 420; assert.equal(c.drHeight(), 400);
  assert.equal(stored['ws.dashboard.drawer.h'], '400');
  const saved = vm.createContext({ load: () => '275', window: c.window });
  vm.runInContext(section('  var DR_OPEN =', '  var sbody ='), saved);
  vm.runInContext(section('  function drHeight()', '  // drApply lays'), saved);
  assert.equal(saved.drHeight(), 275);
});

class El {
  constructor(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.className = ''; this.attrs = {}; this.textContent = ''; this.listeners = {}; }
  appendChild(el) { el.parent = this; this.children.push(el); return el; }
  setAttribute(k, v) { this.attrs[k] = v; }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(k, fn) { this.listeners[k] = fn; }
  querySelector(sel) {
    for (const el of this.children) {
      if (sel[0] === '.' ? el.className.split(' ').includes(sel.slice(1)) : el.tagName === sel.toUpperCase()) return el;
      const nested = el.querySelector(sel); if (nested) return nested;
    }
    return null;
  }
  remove() { this.parent.children.splice(this.parent.children.indexOf(this), 1); }
}
function panel() { const p = new El('details'), lanes = new El('div'); lanes.className = 'lanes'; p.appendChild(lanes); return p; }
const tick = () => new Promise(resolve => setImmediate(resolve));

test('ideas fetch only on demand; loading, retry, empty, swaps and safe file links', async () => {
  let p = panel(), finish, fail, requests = [];
  const c = vm.createContext({ doc: { createElement: tag => new El(tag) }, rid: 'session',
    $: id => id === 'p-tickets' ? p : null, wsT: k => k,
    fetch: (url, opts) => { requests.push([url, opts]); return new Promise((resolve, reject) => { finish = resolve; fail = reject; }); } });
  vm.runInContext(section('  var ideaRows =', '  // drFill stretches'), c);
  c.ticketIdeaSync(); c.ticketIdeaSync();
  assert.equal(requests.length, 0);
  assert.equal(p.querySelector('.lanes').children.length, 1);
  assert.equal(p.querySelector('button').textContent, 'js.drawer.idea.more');
  p.querySelector('button').listeners.click(); c.ticketIdeaLoad();
  assert.equal(requests.length, 1, 'concurrent clicks coalesce');
  assert.equal(requests[0][0], '/s/session/tickets/idea');
  assert.equal(requests[0][1].credentials, 'same-origin');
  assert.equal(p.querySelector('button').disabled, true);
  p = panel(); c.ticketIdeaSync();
  assert.equal(p.querySelector('button').disabled, true, 'live swap keeps loading state');
  finish({ ok: false }); await tick();
  assert.equal(p.querySelector('button').textContent, 'js.drawer.idea.retry');
  p.querySelector('button').listeners.click(); fail(new Error('offline')); await tick();
  assert.equal(p.querySelector('button').disabled, false);
  p.querySelector('button').listeners.click();
  finish({ ok: true, json: async () => ({ tickets: [{ category: 'feat', title: '<img onerror=bad()>', stem: 'stem', path: 'ai-docs/tickets/idea/a & b.md', abs: '/project/ai-docs/tickets/idea/a & b.md' }] }) });
  await tick();
  assert.equal(p.querySelector('a').textContent, '<img onerror=bad()>');
  assert.equal(p.querySelector('a').href, '/s/session/file?path=ai-docs%2Ftickets%2Fidea%2Fa%20%26%20b.md');
  assert.equal(p.querySelector('a').attrs['data-wl'], 'file');
  assert.equal(p.querySelector('a').attrs['data-rel'], 'ai-docs/tickets/idea/a & b.md');
  assert.equal(p.querySelector('a').attrs['data-abs'], '/project/ai-docs/tickets/idea/a & b.md');
  assert.equal(p.querySelector('.csub').textContent, 'stem');
  p = panel(); c.ticketIdeaSync(); c.ticketIdeaLoad();
  assert.equal(requests.length, 3, 'loaded ideas survive a live swap without another read');
  assert.ok(p.querySelector('a'));
  c.ideaRows = []; c.ticketIdeaRedraw();
  assert.equal(p.querySelector('.none').textContent, 'js.drawer.idea.empty');
  assert.equal(p.querySelector('.chip').textContent, '0');
});

test('notes load cached snapshot then wait, refresh by POST, and render server layers safely', async () => {
  const list = new El('div'), button = new El('button'), requests = [], pending = [];
  const c = vm.createContext({ doc: { createElement: tag => new El(tag) }, rid: 'session',
    $: id => ({ noteslist: list, notesrefresh: button })[id], wsT: k => k,
    glRel: () => 'ago', wsI18n: { lang: 'en' },
    fetch: (url, opts) => { requests.push([url, opts]); return new Promise((resolve, reject) => pending.push({ resolve, reject })); } });
  vm.runInContext(section('  var notesSeq =', '  // drFill stretches'), c);
  assert.equal(requests.length, 0);
  c.notesLoad(false);
  assert.equal(requests[0][0], '/s/session/notes');
  assert.equal(requests[0][1].method, 'GET');
  pending.shift().resolve({ ok: true, json: async () => ({ layers: [], loading: true }) }); await tick();
  assert.equal(requests[1][0], '/s/session/notes?wait=1');
  pending.shift().resolve({ ok: true, json: async () => ({ local_only: true, layers: [{ layer: 'repo', notes: [
    { key: '<unsafe>', value: '<value>', priority: 0, visible: false, written_at: '2026-01-01T00:00:00Z' }
  ] }, { layer: 'clone', notes: [] }] }) }); await tick();
  assert.equal(list.querySelector('.notes-key').textContent, '<unsafe>');
  assert.equal(list.querySelector('.notes-muted').textContent, 'js.notes.muted');
  assert.ok(list.querySelector('.notes-dim'), 'muted rows use a drawer-scoped state class');
  assert.ok(list.querySelector('.zero'));
  const value = list.querySelector('.notes-value');
  assert.equal(value.textContent, '<value>'); value.listeners.click(); assert.equal(value.getAttribute('aria-expanded'), 'true');
  assert.ok(list.children.some(el => el.textContent === 'js.notes.local'));
  assert.equal(button.disabled, false);
  button.listeners.click({ preventDefault() {} });
  assert.equal(requests[2][1].method, 'POST');
  pending.shift().reject(new Error('offline')); await tick();
  assert.equal(button.disabled, false);
  assert.ok(list.children.some(el => el.textContent === 'js.notes.error'));
});

test('wide ticket spans suppress the legacy last-row vertical fill', () => {
  const frames = [], cards = Array.from({ length: 5 }, () => ({ children: [], style: {} }));
  const body = { children: cards, hidden: false };
  const c = vm.createContext({ drbody: body, drRoom() {}, requestAnimationFrame: fn => frames.push(fn),
    getComputedStyle: el => el === body ? { display: 'grid', gridTemplateColumns: '300px 300px 300px 300px' }
      : { display: 'block', gridRowStart: 'auto', gridRowEnd: 'auto', gridColumnStart: el === cards[1] ? 'span 2' : 'auto', gridColumnEnd: 'auto' } });
  vm.runInContext(section('  var drFilled =', '  function drDot()'), c);
  c.drFill(); frames.shift()();
  assert.ok(cards.every(card => !card.style.gridRow), 'wide cards retain their two-row dense layout');
});

test('wide grid uses existing breakpoints and dense ticket span; narrow remains the DOM stack', () => {
  assert.match(css, /@media \(min-width: 1600px\)[\s\S]*?width: min\(1280px,/);
  const wide = css.slice(css.indexOf('@media (min-width: 760px)'), css.indexOf('.d-ticket-more'));
  assert.match(wide, /grid-auto-flow: dense/);
  for (const [id, order] of [['agenda', 1], ['tickets', 2], ['holders', 3], ['worktrees', 4], ['todo', 5], ['mailbox', 6], ['notes', 7]]) {
    assert.ok(wide.includes(`#drbody #p-${id} { order: ${order};`));
  }
  assert.match(wide, /#p-tickets \{ order: 2; grid-column: span 2;/);
  assert.match(wide, /#p-notes \{ order: 7; grid-column: span 2;/);
  assert.match(wide, /\.lanes \{ grid-template-columns: minmax\(0, 1fr\)/);
  assert.match(wide, /\.csub \{[^}]*text-align: right/);
  assert.match(css, /@media \(max-width: 759px\) \{\s*#drbody \{ display: flex; flex-direction: column; \}\s*\.dr-body details.panel \{ flex: none; overflow: hidden; \}\s*\.dr-body details.panel > summary \{ position: static; \}/);
});
