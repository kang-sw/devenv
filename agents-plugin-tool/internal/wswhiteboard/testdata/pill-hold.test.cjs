'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const pillSource = fs.readFileSync(path.join(__dirname, '../assets/pill.js'), 'utf8');
const cardFindSource = fs.readFileSync(path.join(__dirname, '../assets/card-find.js'), 'utf8');

function makeStorage(seed = {}) {
  const map = new Map(Object.entries(seed));
  return {
    getItem: k => (map.has(k) ? map.get(k) : null),
    setItem: (k, v) => map.set(k, String(v)),
    removeItem: k => map.delete(k),
    dump: () => Object.fromEntries(map.entries())
  };
}

function makePillHarness(itemSpecs, opts = {}) {
  class El {
    constructor(tag) {
      this.tag = tag;
      this.children = [];
      this.style = {};
      this.attrs = {};
      this.cls = {};
      this.q = {};
      this.parent = null;
      this.top = 0;
      this.hidden = false;
      this.textContent = '';
      this.listeners = {};
      const me = this;
      this.classList = {
        contains: c => !!me.cls[c],
        toggle: (c, on) => { me.cls[c] = on === undefined ? !me.cls[c] : !!on; },
        add: c => { me.cls[c] = true; },
        remove: c => { delete me.cls[c]; }
      };
    }
    set className(v) {
      this.cls = {};
      String(v || '').split(/\s+/).forEach(c => { if (c) this.cls[c] = true; });
    }
    get firstChild() { return this.children[0] || null; }
    all(out = []) {
      this.children.forEach(c => { out.push(c); c.all(out); });
      return out;
    }
    querySelectorAll(sel) {
      if (sel in this.q) return this.q[sel];
      if (sel[0] === '.') {
        const cls = sel.slice(1).split('.');
        return this.all([]).filter(e => cls.every(c => e.cls[c]));
      }
      return [];
    }
    querySelector(sel) {
      if (sel in this.q) return this.q[sel][0] || null;
      return this.querySelectorAll(sel)[0] || null;
    }
    closest(sel) {
      const parts = sel.split(',').map(s => s.trim());
      for (let e = this; e; e = e.parent) {
        for (const p of parts) {
          const cls = p.replace(/^[a-z]+/i, '').split('.').filter(Boolean);
          if (cls.length && cls.every(c => e.cls[c])) return e;
        }
      }
      return null;
    }
    getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; }
    setAttribute(k, v) { this.attrs[k] = String(v); }
    hasAttribute(k) { return k in this.attrs; }
    appendChild(c) { c.parent = this; this.children.push(c); return c; }
    insertBefore(c) { c.parent = this; this.children.unshift(c); return c; }
    addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
    removeEventListener(type, fn) {
      if (!this.listeners[type]) return;
      this.listeners[type] = this.listeners[type].filter(f => f !== fn);
    }
    getBoundingClientRect() { return { top: this.top, bottom: this.top + 40, width: 100, height: 40 }; }
    scrollIntoView() { jumped.push(this.attrs['data-id'] || this.attrs.id); }
  }

  function makeItem(spec) {
    const el = new El('div');
    el.className = spec.cls || 'card policy';
    el.attrs.id = 'item-' + spec.id;
    el.attrs['data-id'] = String(spec.id);
    el.top = spec.top;
    const title = new El('span');
    title.className = 'title';
    title.textContent = spec.title || ('Question ' + spec.id);
    el.appendChild(title);
    if (spec.ctx) {
      const ctx = new El('div');
      ctx.className = 'ctx';
      ctx.textContent = spec.ctx;
      el.appendChild(ctx);
    }
    if (spec.options) {
      spec.options.forEach((opt, i) => {
        const o = new El('button');
        o.className = 'opt';
        o.attrs['data-opt'] = String.fromCharCode(97 + i);
        o.textContent = opt;
        el.appendChild(o);
      });
    }
    if (spec.askEntry) {
      const jump = new El('a');
      jump.className = 'wb-askjump';
      jump.attrs.href = '#' + spec.askEntry;
      if (spec.reasked) {
        const re = new El('span');
        re.className = 'wb-askre';
        re.textContent = 'Asked again ' + spec.reasked;
        jump.appendChild(re);
      }
      el.appendChild(jump);
    }
    const holdBtn = new El('button');
    holdBtn.className = 'btn mini wb-hold-toggle';
    holdBtn.attrs['data-wb-hold'] = '';
    holdBtn.attrs['aria-pressed'] = 'false';
    el.appendChild(holdBtn);
    el.holdBtn = holdBtn;
    if (spec.picked) el.q['.opt[data-opt][aria-pressed="true"]'] = [new El('button')];
    if (spec.note) {
      const n = new El('textarea');
      n.value = spec.note;
      el.q['textarea.note'] = [n];
    }
    if (spec.sent) el.cls['sent-sent'] = true;
    return el;
  }

  const storage = opts.storage || makeStorage();
  const items = itemSpecs.map(makeItem);
  const host = new El('div');
  const html = new El('html');
  html.attrs['data-rid'] = opts.rid || 'sess-1';
  if (opts.incarnation) html.attrs['data-board-incarnation'] = opts.incarnation;
  const frames = [], jumped = [], titleCounts = [];
  const doc = {
    body: new El('body'),
    documentElement: html,
    getElementById: id => (id === 'bcol' ? host : null),
    querySelector: sel => (sel === '.topic' && opts.hasTopics !== false ? new El('section') : null),
    querySelectorAll: () => items,
    createElement: tag => new El(tag)
  };
  const win = {
    localStorage: storage,
    IntersectionObserver: function () { this.observe = () => {}; this.disconnect = () => {}; },
    innerHeight: 800,
    scrollY: 20,
    addEventListener: () => {},
    removeEventListener: () => {},
    wbTitleCount: n => titleCounts.push(n)
  };
  const ctx = vm.createContext({
    console,
    document: doc,
    window: win,
    IntersectionObserver: win.IntersectionObserver,
    Set, Map, Array, Object, JSON, Math, String,
    requestAnimationFrame: fn => { frames.push(fn); return frames.length; },
    cancelAnimationFrame: () => {},
    getComputedStyle: () => ({ overflowY: 'visible' }),
    wsT: k => k
  });

  function run() {
    vm.runInContext(pillSource, ctx);
    flush();
  }
  function flush() {
    while (frames.length) frames.shift()();
  }
  function pill(idx) { return host.children[idx].children[0]; }
  function shown(p) {
    if (p.hidden) return null;
    return p.querySelectorAll('.oq-seg')
      .filter(b => !b.hidden)
      .map(b => [b.getAttribute('data-k'), b.children[0].textContent, b.querySelector('.n').textContent]);
  }
  function pressSeg(p, k) {
    const b = p.querySelectorAll('.oq-seg').find(s => s.getAttribute('data-k') === k);
    p.onclick({ target: b.children[1] });
  }
  function clickHold(item) {
    const handlers = host.listeners.click || [];
    handlers.forEach(fn => fn({ target: item.holdBtn, preventDefault() {} }));
    flush();
  }

  run();
  return { items, host, storage, win, jumped, titleCounts, flush, pill, shown, pressSeg, clickHold, run };
}

test('held ask leaves open count and tab title, surfaces in held pill segment, and jumps in one step', () => {
  const h = makePillHarness([
    { id: 1, top: -300, title: 'Choose storage' },
    { id: 2, top: -180, title: 'Confirm rollout' },
    { id: 3, top: 950, title: 'Approve retry cap' }
  ]);
  assert.equal(h.titleCounts.at(-1), 3);
  assert.deepEqual(h.shown(h.pill(0)), [['open', 'js.pill.open', '2']]);
  assert.deepEqual(h.shown(h.pill(1)), [['open', 'js.pill.open', '1']]);

  // Hold item 1 (above) and item 3 (below).
  h.clickHold(h.items[0]);
  h.clickHold(h.items[2]);
  assert.equal(h.items[0].classList.contains('wb-held'), true);
  assert.equal(h.items[0].holdBtn.getAttribute('aria-pressed'), 'true');
  assert.equal(h.titleCounts.at(-1), 1, 'held items leave tab-title open total');
  assert.deepEqual(h.shown(h.pill(0)), [
    ['open', 'js.pill.open', '1'],
    ['held', 'js.pill.held', '1']
  ]);
  assert.deepEqual(h.shown(h.pill(1)), [['held', 'js.pill.held', '1']]);

  // Clicking the held pill segment jumps directly to the held item.
  h.pressSeg(h.pill(0), 'held');
  h.pressSeg(h.pill(1), 'held');
  assert.deepEqual(h.jumped, ['1', '3']);

  // Persists in localStorage under board identity + item ID and survives reload.
  const dumped = h.storage.dump();
  assert.ok(dumped['ws.wb.holds.sess-1'], 'stored in localStorage keyed by board identity');
  const reloaded = makePillHarness([
    { id: 1, top: -300, title: 'Choose storage' },
    { id: 2, top: -180, title: 'Confirm rollout' },
    { id: 3, top: 950, title: 'Approve retry cap' }
  ], { storage: h.storage });
  assert.equal(reloaded.items[0].classList.contains('wb-held'), true);
  assert.equal(reloaded.titleCounts.at(-1), 1);
  assert.deepEqual(reloaded.shown(reloaded.pill(0)), [
    ['open', 'js.pill.open', '1'],
    ['held', 'js.pill.held', '1']
  ]);
});

test('hold auto-clears when item content or re-ask marker changes (fingerprint change)', () => {
  const storage = makeStorage();
  const h1 = makePillHarness([
    { id: 10, top: -200, title: 'Original question', options: ['Yes', 'No'], askEntry: 'e1' },
    { id: 11, top: -100, title: 'Unchanged question', options: ['Keep', 'Drop'], askEntry: 'e2' }
  ], { storage, rid: 'board-a', incarnation: 'inc-1' });
  h1.clickHold(h1.items[0]);
  h1.clickHold(h1.items[1]);
  assert.equal(h1.titleCounts.at(-1), 0);

  // Agent patches item 10's title; item 11 is unchanged (even if a topic log was added elsewhere).
  const h2 = makePillHarness([
    { id: 10, top: -200, title: 'Updated question wording', options: ['Yes', 'No'], askEntry: 'e1' },
    { id: 11, top: -100, title: 'Unchanged question', options: ['Keep', 'Drop'], askEntry: 'e2' }
  ], { storage, rid: 'board-a', incarnation: 'inc-1' });
  assert.equal(h2.items[0].classList.contains('wb-held'), false, 'edited item hold auto-cleared');
  assert.equal(h2.items[1].classList.contains('wb-held'), true, 'unchanged item keeps hold');
  assert.equal(h2.titleCounts.at(-1), 1);

  // Agent re-asks item 11 with same wording but new askEntry/reasked marker.
  const h3 = makePillHarness([
    { id: 10, top: -200, title: 'Updated question wording', options: ['Yes', 'No'], askEntry: 'e1' },
    { id: 11, top: -100, title: 'Unchanged question', options: ['Keep', 'Drop'], askEntry: 'e9', reasked: '1' }
  ], { storage, rid: 'board-a', incarnation: 'inc-1' });
  assert.equal(h3.items[1].classList.contains('wb-held'), false, 're-asked item hold auto-cleared');
  assert.equal(h3.titleCounts.at(-1), 2);
  assert.deepEqual(storage.dump(), {}, 'cleared holds remove empty localStorage entry');
});

test('hold is dropped when item is answered (drafted or sent) or withdrawn from open asks', () => {
  const storage = makeStorage();
  const h1 = makePillHarness([
    { id: 20, top: -300, title: 'Pick mode' },
    { id: 21, top: -200, title: 'Ack change' },
    { id: 22, top: -100, title: 'Temporary ask' }
  ], { storage, rid: 'board-b' });
  h1.clickHold(h1.items[0]);
  h1.clickHold(h1.items[1]);
  h1.clickHold(h1.items[2]);
  assert.equal(h1.titleCounts.at(-1), 0);

  // Item 20 is answered (option picked), item 21 is marked sent-sent, item 22 is withdrawn (gone).
  const h2 = makePillHarness([
    { id: 20, top: -300, title: 'Pick mode', picked: true },
    { id: 21, top: -200, title: 'Ack change', sent: true }
  ], { storage, rid: 'board-b' });
  assert.equal(h2.items[0].classList.contains('wb-held'), false);
  assert.equal(h2.items[1].classList.contains('wb-held'), false);
  assert.deepEqual(h2.shown(h2.pill(0)), [['ans', 'js.pill.answered', '2']]);
  assert.deepEqual(storage.dump(), {}, 'answered and withdrawn holds are pruned from localStorage');
});

test('in-card find resolves cards only when focus or pointer is inside a non-terminal card and wraps step index', () => {
  const listeners = {};
  const card = {
    nodeType: 1, isConnected: true, tagName: 'DETAILS',
    getClientRects: () => [{ width: 200, height: 100 }],
    closest: sel => (sel === 'details.entry-panel[open]:not(.msg)' ? card : null)
  };
  const insideCard = {
    nodeType: 1, isConnected: true,
    closest: sel => (sel === 'details.entry-panel[open]:not(.msg)' ? card : null)
  };
  const outsideChat = {
    nodeType: 1, isConnected: true,
    closest: () => null
  };
  const inTerminal = {
    nodeType: 1, isConnected: true,
    closest: sel => (sel.includes('.xterm') ? inTerminal : null)
  };
  const doc = {
    activeElement: outsideChat,
    body: outsideChat,
    querySelectorAll: () => [],
    addEventListener: (t, fn) => { listeners[t] = fn; },
    elementFromPoint: (x, y) => (x > 500 ? insideCard : outsideChat)
  };
  const win = { addEventListener: () => {}, getComputedStyle: () => ({ visibility: 'visible' }) };
  const ctx = vm.createContext({ document: doc, window: win, getComputedStyle: win.getComputedStyle, Set, Array });
  vm.runInContext(cardFindSource, ctx);
  const api = win.wbCardFind;

  // Outside card (focus + pointer in chat) -> null (native browser find untouched).
  assert.equal(api.resolveTarget(outsideChat, outsideChat, { x: 100, y: 100 }, null, doc), null);
  // Pointer inside card -> resolves card even when focus is outside.
  assert.equal(api.resolveTarget(outsideChat, outsideChat, { x: 600, y: 100 }, null, doc), card);
  // Focus inside card -> resolves card even when pointer is null or elsewhere.
  assert.equal(api.resolveTarget(insideCard, insideCard, { x: 100, y: 100 }, null, doc), card);
  // Focus inside terminal -> never intercepted.
  assert.equal(api.resolveTarget(inTerminal, inTerminal, { x: 600, y: 100 }, null, doc), null);

  // Step wrap-around.
  assert.equal(api.stepIndex(-1, 3, 1), 0);
  assert.equal(api.stepIndex(0, 3, 1), 1);
  assert.equal(api.stepIndex(2, 3, 1), 0);
  assert.equal(api.stepIndex(0, 3, -1), 2);
  assert.equal(api.stepIndex(0, 0, 1), -1);
});
