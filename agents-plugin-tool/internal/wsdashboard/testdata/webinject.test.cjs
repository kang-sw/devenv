// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/webinject.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const raw = fs.readFileSync(path.join(__dirname, '../assets/webinject.js'), 'utf8');
const PLACEHOLDER = '"__WS_DASHBOARD_PORT__"';
// The listener replaces the first occurrence only, so it must be the only one.
const source = raw.replace(PLACEHOLDER, '"4242"');

// A minimal DOM: elements with attributes, children, and text, and a
// querySelectorAll that parses the selector forms the script emits
// (#id, [attr="value"], tag:nth-of-type(n), joined by " > ") with CSS
// escapes, so the script's uniqueness checks run against real matching.
class Node {
  constructor(localName, attrs) {
    this.nodeType = 1; this.localName = localName; this.attrs = { ...attrs };
    this.children = []; this.parentNode = null; this.text = ''; this.style = { cssText: '' };
  }
  get id() { return this.attrs.id || ''; }
  getAttribute(n) { return Object.hasOwn(this.attrs, n) ? this.attrs[n] : null; }
  setAttribute(n, v) { this.attrs[n] = String(v); }
  get parentElement() { return this.parentNode && this.parentNode.nodeType === 1 ? this.parentNode : null; }
  get previousElementSibling() {
    const p = this.parentNode; if (!p || !p.children) return null;
    const i = p.children.indexOf(this); return i > 0 ? p.children[i - 1] : null;
  }
  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }
  removeChild(c) { this.children.splice(this.children.indexOf(c), 1); c.parentNode = null; return c; }
  get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
  get isConnected() { let n = this; while (n.parentNode) n = n.parentNode; return n.nodeType === 9; }
  getBoundingClientRect() { return { left: 10, top: 20, width: 30, height: 40 }; }
}
function h(tag, attrs, ...kids) {
  const el = new Node(tag, attrs || {});
  for (const k of kids) { if (typeof k === 'string') el.text += k; else el.appendChild(k); }
  return el;
}
function* walk(el) { yield el; for (const c of el.children) yield* walk(c); }

function cssEscape(value) {
  const s = String(value), first = s.charCodeAt(0); let out = '';
  if (s.length === 1 && first === 0x2d) return '\\' + s;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c === 0) { out += '�'; continue; }
    if ((c >= 1 && c <= 0x1f) || c === 0x7f || (i === 0 && c >= 0x30 && c <= 0x39) ||
      (i === 1 && c >= 0x30 && c <= 0x39 && first === 0x2d)) { out += '\\' + c.toString(16) + ' '; continue; }
    if (c >= 0x80 || c === 0x2d || c === 0x5f || (c >= 0x30 && c <= 0x39) || (c >= 0x41 && c <= 0x5a) || (c >= 0x61 && c <= 0x7a)) { out += s[i]; continue; }
    out += '\\' + s[i];
  }
  return out;
}

function parseSelector(sel) {
  let i = 0;
  const fail = () => { throw new SyntaxError(`unsupported selector ${JSON.stringify(sel)} at ${i}`); };
  const escape = () => {
    i++; const hex = /^[0-9a-fA-F]{1,6}/.exec(sel.slice(i));
    if (hex) { i += hex[0].length; if (sel[i] === ' ') i++; return String.fromCodePoint(parseInt(hex[0], 16)); }
    if (i >= sel.length) fail(); return sel[i++];
  };
  const ident = () => {
    let out = '';
    while (i < sel.length) {
      const c = sel[i];
      if (c === '\\') out += escape();
      else if (/[A-Za-z0-9_\-]/.test(c) || c.charCodeAt(0) >= 0x80) { out += c; i++; }
      else break;
    }
    if (!out) fail(); return out;
  };
  const string = () => {
    if (sel[i] !== '"') fail(); i++; let out = '';
    while (i < sel.length && sel[i] !== '"') out += sel[i] === '\\' ? escape() : sel[i++];
    if (sel[i] !== '"') fail(); i++; return out;
  };
  const compounds = [];
  for (;;) {
    const c = {};
    if (sel[i] !== '#' && sel[i] !== '[' && sel[i] !== ':') c.tag = ident();
    while (i < sel.length && sel[i] !== ' ') {
      if (sel[i] === '#') { i++; c.id = ident(); }
      else if (sel[i] === '[') { i++; const name = ident(); if (sel[i] !== '=') fail(); i++; c.attr = [name, string()]; if (sel[i] !== ']') fail(); i++; }
      else if (sel.startsWith(':nth-of-type(', i)) { i += 13; const n = /^\d+/.exec(sel.slice(i)); if (!n) fail(); i += n[0].length; if (sel[i] !== ')') fail(); i++; c.nth = +n[0]; }
      else fail();
    }
    compounds.push(c);
    if (i >= sel.length) break;
    if (!sel.startsWith(' > ', i)) fail();
    i += 3;
  }
  return compounds;
}
function matches(el, c) {
  if (!el || el.nodeType !== 1) return false;
  if (c.tag && el.localName !== c.tag) return false;
  if (c.id !== undefined && el.id !== c.id) return false;
  if (c.attr && el.getAttribute(c.attr[0]) !== c.attr[1]) return false;
  if (c.nth !== undefined) {
    let n = 1; for (let s = el.previousElementSibling; s; s = s.previousElementSibling) if (s.localName === el.localName) n++;
    if (n !== c.nth) return false;
  }
  return true;
}

function makeDocument(body) {
  const html = h('html', {}, h('head'), body);
  const doc = { nodeType: 9, documentElement: html, body, queries: [],
    createElement: tag => new Node(tag, {}),
    querySelectorAll(sel) {
      doc.queries.push(sel);
      const cs = parseSelector(sel);
      return [...walk(html)].filter(el => {
        let n = el;
        for (let k = cs.length - 1; k >= 0; k--, n = n && n.parentElement) if (!matches(n, cs[k])) return false;
        return true;
      });
    } };
  html.parentNode = doc;
  return doc;
}

// A framed page: the parent records every post; listeners and timers are
// driven by hand.
function boot({ body = h('body'), hostname = 'localhost', loc = {}, framed = true, consoleError, history } = {}) {
  const posts = [], listeners = [], timers = [], errored = [];
  const parent = { postMessage(m, origin) { posts.push({ m: JSON.parse(JSON.stringify(m)), origin }); } };
  const document = makeDocument(body);
  const ctx = { document, CSS: { escape: cssEscape },
    location: { hostname, pathname: '/app', search: '?q=1', hash: '#top', ...loc },
    console: { error: consoleError || ((...args) => errored.push(args)), log() {} },
    setTimeout: fn => { timers.push(fn); return timers.length; },
    addEventListener: (type, fn, capture) => listeners.push({ type, fn, capture: !!capture }),
    removeEventListener: (type, fn, capture) => {
      const i = listeners.findIndex(l => l.type === type && l.fn === fn && l.capture === !!capture);
      if (i >= 0) listeners.splice(i, 1);
    } };
  if (history) ctx.history = history;
  ctx.window = ctx;
  ctx.parent = framed ? parent : ctx;
  vm.createContext(ctx);
  vm.runInContext(source, ctx);
  const dispatch = (type, props = {}) => {
    const e = { type, defaultPrevented: false, stopped: false, ...props,
      preventDefault() { this.defaultPrevented = true; }, stopImmediatePropagation() { this.stopped = true; }, stopPropagation() { this.stopped = true; } };
    for (const l of listeners.filter(l => l.type === type)) l.fn.call(ctx, e);
    return e;
  };
  const flush = () => { while (timers.length) timers.shift()(); };
  const fromHost = data => dispatch('message', { source: parent, origin: `http://${hostname}:4242`, data });
  const take = () => posts.splice(0).map(p => p.m);
  return { ctx, document, parent, posts, listeners, dispatch, flush, fromHost, take, errored };
}
function pickOf(page, el) {
  page.fromHost({ wsWeb: 'pick', on: true });
  page.take();
  page.dispatch('click', { target: el });
  const [pick, off] = page.take();
  assert.equal(pick.wsWeb, 'pick');
  assert.deepEqual(off, { wsWeb: 'picking', on: false });
  assert(bytes(pick.selector) >= 1 && bytes(pick.selector) <= 512 && !/[\r\n]/.test(pick.selector));
  return pick;
}
const bytes = s => Buffer.byteLength(s, 'utf8');
const pickTypes = ['mouseover', 'scroll', 'resize', 'pointerdown', 'pointerup', 'mousedown', 'mouseup', 'click', 'keydown'];

test('the port placeholder appears once and the listener replacement bakes it into PORT', () => {
  assert.equal(raw.split(PLACEHOLDER).length, 2);
  assert.match(source, /var PORT = "4242";/);
});

test('posts go only to the dashboard origin, built from the page host name', () => {
  const page = boot();
  page.fromHost({ wsWeb: 'pick', on: true });
  page.ctx.console.error('x');
  page.flush();
  page.fromHost({ wsWeb: 'attach' });
  assert(page.posts.length >= 5);
  for (const p of page.posts) assert.equal(p.origin, 'http://localhost:4242');
  assert.deepEqual(page.posts[0].m, { wsWeb: 'ready', path: '/app?q=1#top' });
  const v6 = boot({ hostname: '[::1]' });
  assert.deepEqual(v6.posts.map(p => p.origin), ['http://[::1]:4242']);
});

test('runs once and only when framed', () => {
  const top = boot({ framed: false });
  assert.equal(top.posts.length, 0);
  assert.equal(top.listeners.length, 0);
  const page = boot();
  vm.runInContext(source, page.ctx);
  assert.equal(page.posts.length, 1);
});

test('ready is posted again on popstate and hashchange', () => {
  const page = boot();
  page.take();
  page.ctx.location.hash = '#next';
  page.dispatch('hashchange');
  page.ctx.location.pathname = '/other';
  page.dispatch('popstate');
  assert.deepEqual(page.take(), [{ wsWeb: 'ready', path: '/app?q=1#next' }, { wsWeb: 'ready', path: '/other?q=1#next' }]);
});

test('a same-document move ends pick mode and posts the count the page still holds', () => {
  const page = boot();
  page.ctx.console.error('one');
  page.ctx.console.error('two');
  page.flush();
  page.fromHost({ wsWeb: 'pick', on: true });
  page.take();
  page.ctx.location.hash = '#next';
  page.dispatch('hashchange');
  assert.deepEqual(page.take(), [
    { wsWeb: 'picking', on: false },
    { wsWeb: 'ready', path: '/app?q=1#next' },
    { wsWeb: 'errors', count: 2 },
  ]);
  assert.equal(page.listeners.filter(l => l.capture).length, 0);
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take()[0].lines, ['one', 'two']);
});

test('pushState and replaceState count as a move once the address changed', () => {
  const calls = [];
  // A history whose methods move location as the browser does, and check
  // they are called on the history object.
  const history = {
    pushState(state, title, url) { assert.equal(this, history); calls.push(['push', url]); go(url); return 'pushed'; },
    replaceState(state, title, url) { assert.equal(this, history); calls.push(['replace', url]); if (url) go(url); },
  };
  const page = boot({ history });
  function go(url) {
    const u = new URL(url, 'http://localhost' + page.ctx.location.pathname);
    Object.assign(page.ctx.location, { pathname: u.pathname, search: u.search, hash: u.hash });
  }
  page.ctx.console.error('kept');
  page.flush();
  page.take();
  assert.equal(page.ctx.history.pushState({}, '', '/route?tab=2'), 'pushed');
  assert.deepEqual(page.take(), [{ wsWeb: 'ready', path: '/route?tab=2' }, { wsWeb: 'errors', count: 1 }]);
  page.ctx.history.replaceState({ scroll: 1 }, '');
  page.ctx.history.replaceState({}, '', '/route?tab=2');
  assert.deepEqual(page.take(), []);
  page.ctx.history.replaceState({}, '', '/route?tab=3');
  assert.deepEqual(page.take().map(m => m.path || m.count), ['/route?tab=3', 1]);
  assert.deepEqual(calls.map(c => c[0]), ['push', 'replace', 'replace', 'replace']);
});

test('messages from another origin, another source, or without a wsWeb type are ignored', () => {
  const page = boot();
  page.take();
  page.dispatch('message', { source: page.parent, origin: 'http://localhost:9999', data: { wsWeb: 'pick', on: true } });
  page.dispatch('message', { source: page.parent, origin: 'http://127.0.0.1:4242', data: { wsWeb: 'attach' } });
  page.dispatch('message', { source: {}, origin: 'http://localhost:4242', data: { wsWeb: 'pick', on: true } });
  page.dispatch('message', { source: page.ctx, origin: 'http://localhost:4242', data: { wsWeb: 'attach' } });
  page.fromHost('attach');
  page.fromHost({ type: 'attach' });
  page.fromHost({ wsWeb: 1 });
  page.fromHost(null);
  assert.deepEqual(page.take(), []);
  page.fromHost({ wsWeb: 'pick', on: true });
  assert.deepEqual(page.take(), [{ wsWeb: 'picking', on: true }]);
});

test('a unique id is the selector, escaped', () => {
  const plain = h('button', { id: 'save' }, 'Save');
  const odd = h('span', { id: '1 a"b' }, 'odd');
  const page = boot({ body: h('body', {}, h('div', {}, plain), odd) });
  assert.equal(pickOf(page, plain).selector, '#save');
  const sel = pickOf(page, odd).selector;
  assert.equal(sel, '#\\31 \\ a\\"b');
  assert.deepEqual(page.document.querySelectorAll(sel), [odd]);
});

test('a duplicate id falls back to a unique data-testid', () => {
  const el = h('button', { id: 'dup', 'data-testid': 'go "now"' }, 'Go');
  const page = boot({ body: h('body', {}, h('i', { id: 'dup' }), el) });
  const sel = pickOf(page, el).selector;
  assert.equal(sel, '[data-testid="go\\ \\"now\\""]');
  assert.deepEqual(page.document.querySelectorAll(sel), [el]);
  assert(page.document.queries.includes('#dup'), 'the id uniqueness was checked');
});

test('a duplicate data-testid falls back to an nth-of-type path from html', () => {
  const el = h('li', { 'data-testid': 'row' }, 'b');
  const page = boot({ body: h('body', {}, h('ul', {}, h('li', { 'data-testid': 'row' }, 'a'), h('p'), el)) });
  const sel = pickOf(page, el).selector;
  assert.equal(sel, 'html > body:nth-of-type(1) > ul:nth-of-type(1) > li:nth-of-type(2)');
  assert.deepEqual(page.document.querySelectorAll(sel), [el]);
});

test('the path is headed by the nearest ancestor with a unique id', () => {
  const el = h('a', {}, 'link');
  const page = boot({ body: h('body', { id: 'page' }, h('i', { id: 'nav' }), h('nav', { id: 'nav' }, h('section', { id: 'menu' }, h('a'), el))) });
  const sel = pickOf(page, el).selector;
  assert.equal(sel, '#menu > a:nth-of-type(2)');
  assert.deepEqual(page.document.querySelectorAll(sel), [el]);
  // An ancestor whose id is not unique does not head the path.
  const inner = h('b', {}, 'x');
  const page2 = boot({ body: h('body', {}, h('i', { id: 'w' }), h('div', { id: 'w' }, inner)) });
  assert.equal(pickOf(page2, inner).selector, 'html > body:nth-of-type(1) > div:nth-of-type(1) > b:nth-of-type(1)');
});

test('a path stops at 8 steps without a head and is sent even when not unique', () => {
  const chain = leaf => h('div', {}, h('div', {}, h('div', {}, h('div', {}, h('div', {}, h('div', {}, h('div', {}, h('div', {}, h('div', {}, leaf)))))))));
  const a = h('span', {}, 'a'), b = h('span', {}, 'b');
  const page = boot({ body: h('body', { id: 'root' }, h('section', {}, chain(a)), h('section', {}, chain(b))) });
  const sel = pickOf(page, b).selector;
  assert.equal(sel, Array(7).fill('div:nth-of-type(1)').join(' > ') + ' > span:nth-of-type(1)');
  assert.equal(sel.split(' > ').length, 8);
  assert.deepEqual(page.document.querySelectorAll(sel), [a, b]);
});

test('every selector stays within 512 bytes', () => {
  // An over-long id or data-testid is skipped.
  const longId = h('em', { id: 'x'.repeat(600), 'data-testid': 'short' }, 'e');
  const longBoth = h('em', { id: 'y'.repeat(600), 'data-testid': 'z'.repeat(600) }, 'e');
  const page = boot({ body: h('body', {}, longId, longBoth) });
  assert.equal(pickOf(page, longId).selector, '[data-testid="short"]');
  assert.equal(pickOf(page, longBoth).selector, 'html > body:nth-of-type(1) > em:nth-of-type(2)');
  // An over-long head is dropped and long steps lose the topmost first.
  const head = h('main', { id: 'h'.repeat(500) }, h('p', {}, h('b', {}, h('i', {}, 'deep'))));
  const leaf = head.children[0].children[0].children[0];
  const page2 = boot({ body: h('body', {}, head) });
  const sel2 = pickOf(page2, leaf).selector;
  assert(bytes(sel2) <= 512, sel2);
  assert.equal(sel2, 'p:nth-of-type(1) > b:nth-of-type(1) > i:nth-of-type(1)');
  const tag = n => 'x-' + String(n).padEnd(90, 'q');
  let deep = h(tag(0), {}, 'leaf'); const target = deep;
  for (let n = 1; n < 8; n++) deep = h(tag(n), {}, deep);
  const page3 = boot({ body: h('body', {}, deep) });
  const sel3 = pickOf(page3, target).selector;
  assert(bytes(sel3) <= 512, `${bytes(sel3)} bytes`);
  assert(sel3.endsWith(tag(0) + ':nth-of-type(1)'));
  assert(!sel3.includes(tag(7)), 'the topmost step was dropped');
  assert.deepEqual(page3.document.querySelectorAll(sel3), [target]);
});

test('pick mode: the overlay follows the hover, a click posts one pick and ends the mode', () => {
  const el = h('button', { id: 'go' }, '  Go\n  now  ');
  const page = boot({ body: h('body', {}, el) });
  page.take();
  page.fromHost({ wsWeb: 'pick', on: true });
  assert.deepEqual(page.take(), [{ wsWeb: 'picking', on: true }]);
  const box = page.document.documentElement.children.at(-1);
  assert.equal(box.localName, 'div');
  page.dispatch('mouseover', { target: box });
  page.dispatch('mouseover', { target: el });
  for (const want of ['position:fixed!important', 'pointer-events:none!important', 'z-index:2147483647!important', 'display:block!important', 'left:10px', 'height:40px']) {
    assert(box.style.cssText.includes(want), `overlay lacks ${want}: ${box.style.cssText}`);
  }
  for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup']) {
    const e = page.dispatch(type, { target: el });
    assert(e.defaultPrevented && e.stopped, `${type} swallowed`);
  }
  assert.deepEqual(page.take(), []);
  const click = page.dispatch('click', { target: el });
  assert(click.defaultPrevented && click.stopped);
  assert.deepEqual(page.take(), [
    { wsWeb: 'pick', path: '/app?q=1#top', selector: '#go', text: 'Go now' },
    { wsWeb: 'picking', on: false },
  ]);
  assert.equal(box.parentNode, null);
  assert.deepEqual(page.listeners.filter(l => pickTypes.includes(l.type)), []);
  const after = page.dispatch('click', { target: el });
  assert(!after.defaultPrevented);
  assert.deepEqual(page.take(), []);
});

test('Escape and the host end pick mode, removing the overlay and its listeners', () => {
  const page = boot();
  page.fromHost({ wsWeb: 'pick', on: true });
  page.fromHost({ wsWeb: 'pick', on: true });
  assert.equal(page.document.documentElement.children.length, 3, 'one overlay');
  page.take();
  const other = page.dispatch('keydown', { key: 'Enter' });
  assert(!other.defaultPrevented);
  const esc = page.dispatch('keydown', { key: 'Escape' });
  assert(esc.defaultPrevented);
  assert.deepEqual(page.take(), [{ wsWeb: 'picking', on: false }]);
  assert.equal(page.document.documentElement.children.length, 2);
  assert.deepEqual(page.listeners.filter(l => pickTypes.includes(l.type)), []);
  page.fromHost({ wsWeb: 'pick', on: true });
  page.fromHost({ wsWeb: 'pick', on: false });
  assert.deepEqual(page.take(), [{ wsWeb: 'picking', on: true }, { wsWeb: 'picking', on: false }]);
  assert.equal(page.document.documentElement.children.length, 2);
  assert.deepEqual(page.listeners.filter(l => pickTypes.includes(l.type)), []);
});

test('pick text collapses whitespace and is cut to 1024 bytes on a code point boundary', () => {
  const hangul = h('p', { id: 'k' }, 'a' + '한'.repeat(400));
  const emoji = h('p', { id: 'e' }, ' x\n\t' + 'y😀'.repeat(300));
  const page = boot({ body: h('body', {}, hangul, emoji) });
  const k = pickOf(page, hangul).text;
  assert.equal(bytes(k), 1024);
  assert.equal(k, 'a' + '한'.repeat(341));
  const e = pickOf(page, emoji).text;
  assert(bytes(e) <= 1024);
  assert(e.startsWith('x y😀'));
  assert.equal(Buffer.from(e, 'utf8').toString('utf8'), e, 'no split surrogate pair');
});

test('the console ring keeps the last 50 one-line errors within 1024 bytes and batches the count', () => {
  const page = boot();
  page.take();
  for (let n = 0; n < 60; n++) page.ctx.console.error('err', n);
  assert.equal(page.errored.length, 60, 'the original console.error is always called');
  assert.deepEqual(page.errored[3], ['err', 3]);
  assert.deepEqual(page.take(), [], 'the count waits for the tick');
  page.flush();
  assert.deepEqual(page.take(), [{ wsWeb: 'errors', count: 60 }]);
  page.flush();
  assert.deepEqual(page.take(), [], 'an unchanged count is not posted again');
  page.ctx.console.error('a\r\nb\nc\rd');
  page.ctx.console.error('é'.repeat(600));
  page.ctx.console.error('😀'.repeat(300));
  page.flush();
  page.fromHost({ wsWeb: 'attach' });
  const [, consoleMsg] = page.take();
  const lines = consoleMsg.lines;
  assert.equal(lines.length, 50);
  assert.equal(lines[0], 'err 13');
  assert.equal(lines[46], 'err 59');
  assert.equal(lines[47], 'a b c d');
  assert.equal(lines[48], 'é'.repeat(512));
  assert.equal(lines[49], '😀'.repeat(256));
  for (const l of lines) { assert(!/[\r\n]/.test(l)); assert(bytes(l) <= 1024); }
});

test('errors, rejections, and console.error arguments are formatted without throwing into the page', () => {
  const page = boot();
  page.dispatch('error', { message: 'Uncaught TypeError: x is undefined', filename: 'http://localhost:5173/src/app.js', lineno: 12, colno: 5 });
  page.dispatch('error', { target: {} }); // a resource error event carries no message
  const boom = new TypeError('boom');
  boom.stack = 'TypeError: boom\n    at load (http://localhost:5173/src/data.js:3:9)\n    at run (x.js:1:1)';
  page.dispatch('unhandledrejection', { reason: boom });
  page.dispatch('unhandledrejection', { reason: 'nope' });
  const cyclic = {}; cyclic.self = cyclic;
  const hostile = { toJSON() { throw new Error('no'); }, toString() { throw new Error('no'); } };
  assert.doesNotThrow(() => page.ctx.console.error('failed:', { code: 7 }, cyclic, hostile, undefined, 3));
  page.ctx.console.error();
  page.flush();
  page.take();
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take()[0].lines, [
    'Uncaught TypeError: x is undefined (http://localhost:5173/src/app.js:12:5)',
    'Unhandled rejection: TypeError: boom at load (http://localhost:5173/src/data.js:3:9)',
    'Unhandled rejection: nope',
    'failed: {"code":7} [object Object] [unprintable] undefined 3',
    '(no message)',
  ]);
});

test('console.error never recurses through the wrapper', () => {
  let page, depth = 0;
  page = boot({ consoleError: () => { depth++; if (depth < 3) page.ctx.console.error('again'); } });
  page.ctx.console.error('first');
  page.flush();
  page.take();
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take()[0].lines, ['first', 'again', 'again']);
});

test('attach posts the ring, then clears it and resets the count to 0', () => {
  const page = boot();
  page.take();
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take(), [{ wsWeb: 'console', path: '/app?q=1#top', lines: [] }, { wsWeb: 'errors', count: 0 }]);
  page.ctx.console.error('one');
  page.ctx.console.error('two');
  page.flush();
  assert.deepEqual(page.take(), [{ wsWeb: 'errors', count: 2 }]);
  page.ctx.location.pathname = '/later';
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take(), [{ wsWeb: 'console', path: '/later?q=1#top', lines: ['one', 'two'] }, { wsWeb: 'errors', count: 0 }]);
  page.flush();
  assert.deepEqual(page.take(), [], 'a pending tick after attach posts nothing');
  page.ctx.console.error('three');
  page.flush();
  assert.deepEqual(page.take(), [{ wsWeb: 'errors', count: 1 }]);
  page.fromHost({ wsWeb: 'attach' });
  assert.deepEqual(page.take()[0].lines, ['three']);
});
