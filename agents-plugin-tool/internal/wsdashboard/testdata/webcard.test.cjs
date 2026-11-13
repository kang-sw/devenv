// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/webcard.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../assets/webcard.js'), 'utf8');
const HOST = 'http://localhost:4242', PROXY = 'http://localhost:5555';

// A fake element: enough of the DOM for the card's body, its header
// buttons, and the "+ Web" popover.
function el(tag) {
  const e = {
    tagName: tag.toUpperCase(), className: '', children: [], attrs: {}, listeners: {}, hidden: false, value: '', style: {},
    textContent: '', innerHTML: '', disabled: false, title: '', src: '', contentWindow: null,
    appendChild(c) { this.children.push(c); c.parentNode = this; return c; },
    setAttribute(k, v) { this.attrs[k] = String(v); }, getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; },
    hasAttribute(k) { return k in this.attrs; }, removeAttribute(k) { delete this.attrs[k]; },
    addEventListener(t, fn) { (this.listeners[t] ||= []).push(fn); },
    fire(t, ev = {}) { for (const fn of this.listeners[t] || []) fn(ev); },
    focus() {}, blur() {}, contains(x) { return x === this; }, getBoundingClientRect() { return { left: 100 }; },
  };
  return e;
}

// boot runs webcard.js on a fake session page and returns the kind, the
// window's message listeners, and recorders for the page hooks.
function boot({ sysopen = true, port = '4242', composer = true } = {}) {
  const posts = [], picks = [], consoles = [], toasts = [], opened = [], timers = [], winListeners = [];
  const kinds = {}, list = [];
  const attrs = { 'data-rid': 'rid1' };
  if (sysopen) attrs['data-sysopen'] = '1';
  const document = {
    documentElement: { hasAttribute: n => n in attrs, getAttribute: n => attrs[n] ?? null },
    createElement: el, body: el('body'), activeElement: null,
    getElementById: () => null, querySelector: () => null, addEventListener() {},
  };
  const WsCards = {
    icon: n => '<svg>' + n + '</svg>',
    register: (name, k) => { kinds[name] = k; },
    list: kind => list.filter(c => !kind || c.kind === kind),
    open: (kind, data) => { const c = card(kind, data); kinds[kind].open(c); return c; },
  };
  function card(kind, data) {
    const c = {
      kind, data, body: el('div'), saved: 0, shown: 0,
      title(p) { this.head = p; }, save() { this.saved++; }, show() { this.shown++; }, fold() {},
      actions(acts) {
        this.acts = acts;
        return acts.map(a => { const b = el('button'); b.className = a.cls || ''; b.addEventListener('click', () => a.run(c)); return b; });
      },
    };
    list.push(c);
    return c;
  }
  const ctx = {
    document, WsCards, URL, JSON, Math, String, Array,
    location: { origin: HOST, hostname: 'localhost', port, protocol: 'http:', href: HOST + '/s/rid1' },
    wsT: (k, ...a) => k + (a.length ? ':' + a.join(',') : ''),
    setTimeout: fn => timers.push(fn),
    addEventListener: (type, fn) => winListeners.push({ type, fn }),
    open: (...a) => opened.push(a), getComputedStyle: () => ({ right: '0' }),
  };
  ctx.window = ctx;
  vm.createContext(ctx);
  vm.runInContext(source, ctx);
  const api = ctx.WsWeb;
  if (api) api.setup({
    rid: 'rid1', toast: t => toasts.push(t),
    post: (route, body, done) => posts.push({ route, body: JSON.parse(JSON.stringify(body)), done }),
    ...(composer ? { addPick: p => { picks.push(JSON.parse(JSON.stringify(p))); return true; }, addConsole: c => { consoles.push(JSON.parse(JSON.stringify(c))); return true; } } : {}),
  });
  const message = (source, origin, data) => { for (const l of winListeners) if (l.type === 'message') l.fn({ source, origin, data }); };
  const tick = () => { while (timers.length) timers.shift()(); };
  return { api, kinds, list, posts, picks, consoles, toasts, opened, message, tick, card };
}

// openCard opens a card on target t, answers its registration with reg,
// and gives its iframe a window that records what the card posts.
function openCard(b, t, reg) {
  const c = b.api.open(t);
  const frame = c.w.frame, sent = [];
  frame.contentWindow = { postMessage(m, origin) { sent.push({ m: JSON.parse(JSON.stringify(m)), origin }); } };
  if (reg) b.posts.shift().done(true, reg);
  return { c, frame, win: frame.contentWindow, sent };
}
const DEV = { id: 'abc', kind: 'dev', base: 'http://localhost:5173', path: '/settings' };
const STATIC = { id: 'def', kind: 'static', base: 'docs/site', path: '/index.html' };
function ready(b, o, p) {
  b.message(o.win, HOST, { wsWeb: 'frame', proxy: PROXY });
  b.message(o.win, HOST, { wsWeb: 'ready', path: p });
}
function button(o, cls) { return o.c.w[cls]; }

test('the kind registers only on a page for a viewer on this machine', () => {
  assert.equal(boot({ sysopen: false }).api, undefined);
  const b = boot();
  assert.ok(b.kinds.web);
  assert.equal(b.kinds.web.base, 420);
});

test('open registers the target and points the frame at its wrapper', () => {
  const b = boot();
  const c = b.api.open({ url: 'http://localhost:5173/settings' });
  assert.deepEqual(b.posts.map(p => [p.route, p.body]), [['web', { url: 'http://localhost:5173/settings' }]]);
  b.posts.shift().done(true, DEV);
  assert.equal(c.w.frame.src, '/s/rid1/web/abc?p=%2Fsettings');
  assert.deepEqual(JSON.parse(JSON.stringify(c.data)), { url: 'http://localhost:5173/settings' }, 'the record is the target, never the id');
});

test('a refused registration shows its reason in the card', () => {
  const b = boot();
  const c = b.api.open({ url: 'http://localhost:4242/' });
  b.posts.shift().done(false, 'port 4242 is the dashboard\'s own');
  assert.equal(c.w.note.hidden, false);
  assert.equal(c.w.note.textContent, 'js.web.failed:port 4242 is the dashboard\'s own');
  assert.equal(c.w.frame.hidden, true);
});

test('the host drops a message from another origin, another frame, or the inner page', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/settings' }, DEV);
  const inner = {};
  b.message(o.win, 'http://localhost:6666', { wsWeb: 'frame', proxy: PROXY });
  b.message(o.win, PROXY, { wsWeb: 'ready', path: '/' });
  b.message({}, HOST, { wsWeb: 'ready', path: '/' });
  b.message(inner, PROXY, { wsWeb: 'ready', path: '/' });
  b.message(inner, HOST, { wsWeb: 'ready', path: '/' });
  b.message(null, HOST, { wsWeb: 'ready', path: '/' });
  for (const data of [null, 'ready', { wsWeb: 1 }]) b.message(o.win, HOST, data);
  assert.equal(o.c.w.ready, false);
  assert.equal(o.c.w.proxy, '');
  b.message(o.win, HOST, { wsWeb: 'ready', path: '/' });
  assert.equal(o.c.w.ready, true);
});

test('a frame message naming a proxy off this host name is ignored', () => {
  const b = boot();
  const o = openCard(b, { path: 'docs/site/index.html' }, STATIC);
  for (const proxy of ['http://evil.example', 'http://evil.example:5555', 'https://localhost:5555', 'http://localhost:5555/x',
    'http://localhost.evil.example:5555', 'http://127.0.0.1:5555', 'http://localhost:', 'http://localhost:5555 ', 7, null]) {
    b.message(o.win, HOST, { wsWeb: 'frame', proxy });
  }
  assert.equal(o.c.w.proxy, '');
  assert.equal(o.c.w.framed, false);
  b.message(o.win, HOST, { wsWeb: 'frame', proxy: PROXY });
  assert.equal(o.c.w.proxy, PROXY);
});

test('host posts name this page origin, never "*"', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/settings' }, DEV);
  ready(b, o, '/settings');
  button(o, 'pick').fire('click');
  button(o, 'att').fire('click');
  assert.deepEqual(o.sent, [
    { m: { wsWeb: 'pick', on: true }, origin: HOST },
    { m: { wsWeb: 'attach' }, origin: HOST },
  ]);
});

test('pick and attach wait for the current page to answer', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  assert.equal(button(o, 'pick').disabled, true);
  assert.equal(button(o, 'pick').title, 'js.web.unavailable');
  button(o, 'pick').fire('click');
  assert.deepEqual(o.sent, []);
  ready(b, o, '/');
  assert.equal(button(o, 'pick').disabled, false);
  assert.equal(button(o, 'pick').title, 'js.web.pick');
});

test('a pick adds one pick with the dev page URL', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/settings' }, DEV);
  ready(b, o, '/settings');
  b.message(o.win, HOST, { wsWeb: 'pick', path: '/settings?tab=2', selector: '#save', text: 'Save' });
  assert.deepEqual(b.picks, [], 'a pick the user did not ask for is dropped');
  button(o, 'pick').fire('click');
  b.message(o.win, HOST, { wsWeb: 'picking', on: true });
  assert.equal(button(o, 'pick').getAttribute('aria-pressed'), 'true');
  b.message(o.win, HOST, { wsWeb: 'pick', path: '/settings?tab=2', selector: '#save', text: 'Save' });
  b.message(o.win, HOST, { wsWeb: 'picking', on: false });
  b.message(o.win, HOST, { wsWeb: 'pick', path: '/settings', selector: '#again', text: '' });
  assert.deepEqual(b.picks, [{ url: 'http://localhost:5173/settings?tab=2', selector: '#save', text: 'Save' }]);
  assert.equal(button(o, 'pick').getAttribute('aria-pressed'), 'false');
});

test('Escape on the pick toggle ends pick mode', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/');
  let prevented = 0;
  const esc = { key: 'Escape', preventDefault() { prevented++; } };
  button(o, 'pick').fire('keydown', esc);
  assert.equal(prevented, 0, 'Escape with pick mode off is left to the page');
  button(o, 'pick').fire('click');
  b.message(o.win, HOST, { wsWeb: 'picking', on: true });
  button(o, 'pick').fire('keydown', { key: 'Enter', preventDefault() { prevented++; } });
  button(o, 'pick').fire('keydown', esc);
  assert.equal(prevented, 1);
  assert.deepEqual(o.sent.map(s => s.m), [{ wsWeb: 'pick', on: true }, { wsWeb: 'pick', on: false }]);
  assert.ok(o.sent.every(s => s.origin === HOST));
  b.message(o.win, HOST, { wsWeb: 'picking', on: false });
  b.message(o.win, HOST, { wsWeb: 'pick', path: '/', selector: '#late', text: '' });
  assert.deepEqual(b.picks, [], 'a pick after Escape is dropped');
});

test('a static pick names the workspace path', () => {
  const b = boot();
  const o = openCard(b, { path: 'docs/site/index.html' }, STATIC);
  ready(b, o, '/index.html');
  button(o, 'pick').fire('click');
  b.message(o.win, HOST, { wsWeb: 'pick', path: '/my%20page.html#top', selector: 'html > body:nth-of-type(1)', text: '' });
  assert.deepEqual(b.picks, [{ url: 'docs/site/my page.html#top', selector: 'html > body:nth-of-type(1)', text: '' }]);
  const root = boot();
  const r = openCard(root, { path: 'a.html' }, { id: 'r', kind: 'static', base: '', path: '/a.html' });
  ready(root, r, '/a.html');
  button(r, 'pick').fire('click');
  root.message(r.win, HOST, { wsWeb: 'pick', path: '/a.html', selector: '#x', text: 'x' });
  assert.equal(root.picks[0].url, 'a.html');
});

test('attach adds one console block and resets the badge', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/');
  b.message(o.win, HOST, { wsWeb: 'errors', count: 3 });
  assert.equal(o.c.w.badge.hidden, false);
  assert.equal(o.c.w.badge.textContent, '3');
  b.message(o.win, HOST, { wsWeb: 'console', path: '/', lines: ['a'] });
  assert.deepEqual(b.consoles, [], 'a console block nobody asked for is dropped');
  button(o, 'att').fire('click');
  b.message(o.win, HOST, { wsWeb: 'console', path: '/', lines: ['TypeError: x', 'boom', 7] });
  b.message(o.win, HOST, { wsWeb: 'errors', count: 0 });
  assert.deepEqual(b.consoles, [{ url: 'http://localhost:5173/', lines: ['TypeError: x', 'boom'] }]);
  assert.equal(o.c.w.badge.hidden, true);
  button(o, 'att').fire('click');
  b.message(o.win, HOST, { wsWeb: 'console', path: '/', lines: [] });
  assert.equal(b.consoles.length, 1, 'an empty attach adds nothing');
  assert.deepEqual(b.toasts, ['js.web.noerrors']);
});

test('gone registers again once, then shows the failure', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/x');
  b.message(o.win, HOST, { wsWeb: 'gone' });
  assert.deepEqual(b.posts.map(p => p.body), [{ url: 'http://localhost:5173/x' }]);
  b.posts.shift().done(true, { id: 'abc', kind: 'dev', base: DEV.base, path: '/x' });
  b.message(o.win, HOST, { wsWeb: 'gone' });
  assert.equal(b.posts.length, 0, 'a second gone with no frame between does not register again');
  assert.equal(o.c.w.note.textContent, 'js.web.lost');
});

test('a load with no frame message registers again once', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  o.frame.fire('load');
  b.tick();
  assert.equal(b.posts.length, 1, 'a wrapper that never posted frame (a 404) registers again');
  b.posts.shift().done(true, DEV);
  o.frame.fire('load');
  b.tick();
  assert.equal(b.posts.length, 0);
  assert.equal(o.c.w.note.textContent, 'js.web.lost');
  // A wrapper that comes up resets the retry.
  b.message(o.win, HOST, { wsWeb: 'frame', proxy: PROXY });
  o.frame.fire('load');
  b.tick();
  assert.equal(b.posts.length, 0);
  assert.equal(o.c.w.note.hidden, true);
  o.frame.fire('load');
  b.tick();
  assert.equal(b.posts.length, 1);
});

test('a load soon after the last one does not register again', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/');
  o.frame.fire('load');
  // Within the settle time the address bar moves the wrapper, which comes up.
  o.c.w.bar.value = 'http://localhost:5173/next';
  o.c.w.bar.fire('keydown', { key: 'Enter', preventDefault() {} });
  ready(b, o, '/next');
  o.frame.fire('load');
  b.tick();
  assert.equal(b.posts.length, 0, 'the first load\'s check took the second load\'s frame message');
  assert.equal(o.frame.src, '/s/rid1/web/abc?p=%2Fnext');
});

test('the record follows the page the card shows', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/a?b=1#c');
  assert.deepEqual(JSON.parse(JSON.stringify(o.c.data)), { url: 'http://localhost:5173/a?b=1#c' });
  const s = boot();
  const st = openCard(s, { path: 'docs/site/index.html' }, STATIC);
  ready(s, st, '/other.html?x=1');
  assert.deepEqual(JSON.parse(JSON.stringify(st.c.data)), { path: 'docs/site/other.html' });
  ready(s, st, '/sub/deeper.html');
  assert.deepEqual(JSON.parse(JSON.stringify(st.c.data)), { path: 'docs/site/other.html' }, 'a deeper page keeps the record');
  // The page posts the path: an encoded separator or a dot step never moves
  // the record out of the registered directory.
  for (const forged of ['/..%2Fother%2Fx.html', '/..%5Cx.html', '/sub%2Fx.html', '/..', '/%2E%2E', '/.', '/%E0%A4%A.html', '/a%00.html']) {
    ready(s, st, forged);
    assert.deepEqual(JSON.parse(JSON.stringify(st.c.data)), { path: 'docs/site/other.html' }, 'forged ' + forged);
  }
  ready(s, st, '/..%2Fother%2Fx.html');
  assert.equal(st.c.w.bar.value, 'docs/site/..%2Fother%2Fx.html', 'a ".." step stays encoded in the address');
  ready(s, st, '/caf%C3%A9.html');
  assert.deepEqual(JSON.parse(JSON.stringify(st.c.data)), { path: 'docs/site/café.html' });
});

test('the address bar moves the wrapper on the same dev origin and registers otherwise', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/');
  o.c.w.bar.value = 'http://localhost:5173/next?q=1';
  o.c.w.bar.fire('keydown', { key: 'Enter', preventDefault() {} });
  assert.equal(b.posts.length, 0);
  assert.equal(o.frame.src, '/s/rid1/web/abc?p=%2Fnext%3Fq%3D1');
  o.c.w.bar.value = 'localhost:3000';
  o.c.w.bar.fire('keydown', { key: 'Enter', preventDefault() {} });
  assert.deepEqual(b.posts.map(p => p.body), [{ url: 'http://localhost:3000' }]);
  o.c.w.bar.value = 'docs/a.html';
  o.c.w.bar.fire('keydown', { key: 'Enter', preventDefault() {} });
  assert.deepEqual(b.posts.map(p => p.body)[1], { path: 'docs/a.html' });
});

test('open in tab opens the stored url for a dev card and the proxy URL for a static card', () => {
  const b = boot();
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  ready(b, o, '/a');
  o.c.w.tab.fire('click');
  const s = boot();
  const st = openCard(s, { path: 'docs/site/index.html' }, STATIC);
  ready(s, st, '/index.html#x');
  st.c.w.tab.fire('click');
  assert.deepEqual(b.opened, [['http://localhost:5173/a', '_blank', 'noopener']]);
  assert.deepEqual(s.opened, [[PROXY + '/index.html#x', '_blank', 'noopener']]);
});

test('reload registers again and reloads the page the card shows', () => {
  const b = boot();
  const o = openCard(b, { path: 'docs/site/index.html' }, STATIC);
  ready(b, o, '/sub/deeper.html');
  o.frame.src = '';
  o.c.acts.find(a => a.icon === 'refresh-cw').run(o.c);
  assert.deepEqual(b.posts.map(p => p.body), [{ path: 'docs/site/index.html' }]);
  b.posts.shift().done(true, STATIC);
  assert.equal(o.frame.src, '/s/rid1/web/def?p=%2Fsub%2Fdeeper.html');
});

test('a page without a composer offers no pick or attach', () => {
  const b = boot({ composer: false });
  const o = openCard(b, { url: 'http://localhost:5173/' }, DEV);
  assert.deepEqual(JSON.parse(JSON.stringify(o.c.acts.map(a => a.icon))), ['refresh-cw', 'external-link']);
});

test('the localhost link predicate', () => {
  const b = boot({ port: '4242' });
  for (const href of ['http://localhost:5173/', 'https://127.0.0.1:8443/x', 'http://[::1]:3000', 'http://LOCALHOST:1/', 'http://localhost/']) {
    assert.equal(b.api.localLink(href), true, href);
  }
  for (const href of ['http://localhost:4242/s/x', 'http://127.0.0.1:4242/', 'http://example.com:5173/', 'http://192.168.0.2:5173/',
    'http://localhost.example.com/', 'ftp://localhost:21/', 'http://user@localhost:5173/', 'not a url', '']) {
    assert.equal(b.api.localLink(href), false, href);
  }
});

test('the "+ Web" input reads a URL or a workspace path', () => {
  const b = boot();
  const plain = v => JSON.parse(JSON.stringify(v));
  assert.deepEqual(plain(b.api.target(' http://localhost:5173/a ')), { url: 'http://localhost:5173/a' });
  assert.deepEqual(plain(b.api.target('localhost:5173/a')), { url: 'http://localhost:5173/a' });
  assert.deepEqual(plain(b.api.target('127.0.0.1:8080')), { url: 'http://127.0.0.1:8080' });
  assert.deepEqual(plain(b.api.target('[::1]:3000')), { url: 'http://[::1]:3000' });
  assert.deepEqual(plain(b.api.target('docs/index.html')), { path: 'docs/index.html' });
  assert.equal(b.api.target('  '), null);
});
