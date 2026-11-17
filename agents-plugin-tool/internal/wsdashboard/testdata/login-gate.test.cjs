// TestLoginGateCopyAutoPaste: the real login-gate.js in a node vm with a fake
// DOM, fetch and clipboard. Covers the post-copy clipboard read; the Chromium
// acceptance (login-gate.browser.cjs) covers rendering and real auth routes.
// Never print clipboard text: the fixture link stands in for a credential.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../assets/login-gate.js'), 'utf8');
const LINK = 'http://127.0.0.1:4000/s/x?login=fixture-code';

class El {
  constructor(id) { this.id = id; this.hidden = false; this.disabled = false; this.className = ''; this.value = ''; this.attrs = {}; this.listeners = {}; this.html = ''; this.kids = []; this.text = ''; }
  get classList() {
    const set = (c, on) => { const all = this.className.split(' ').filter(x => x && x !== c); if (on) all.push(c); this.className = all.join(' '); };
    return { toggle: (c, on) => set(c, on), remove: c => set(c, false), contains: c => this.className.split(' ').includes(c) };
  }
  set innerHTML(v) { this.html = v; this.kids = []; }
  get innerHTML() { return this.html; }
  set textContent(v) { this.text = v; }
  get textContent() { return this.text; }
  appendChild(n) { this.kids.push(n); return n; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  removeAttribute(k) { delete this.attrs[k]; }
  hasAttribute(k) { return k in this.attrs; }
  querySelector() { return new El(); }
  addEventListener(k, f) { this.listeners[k] = f; }
  focus() {} select() {}
}

// read: () => Promise | throws; null removes navigator.clipboard entirely.
async function run({ copyStatus = 200, read }) {
  const ids = ['lg-input', 'lg-go', 'lg-clip', 'lg-hint', 'lg-native', 'lg-copy', 'lg-again', 'lg-or', 'lg-mod', 'lg-msg', 'lg-message', 'lg-sub', 'lg-native-msg', 'lg-native-message', 'lg-form', 'lg-done', 'lg-row'];
  const els = Object.fromEntries(ids.map(id => [id, new El(id)]));
  els['lg-hint'].hidden = els['lg-msg'].hidden = els['lg-native-msg'].hidden = true;
  els['lg-hint'].className = 'lg-hint go';
  els['lg-hint'].innerHTML = '<span>copied</span><kbd>V</kbd>';
  const calls = [], reads = { count: 0 }, docListeners = {};
  const root = new El('html'); root.attrs['data-theme'] = 'light';
  const document = {
    documentElement: root,
    getElementById: id => els[id] || null,
    createTextNode: s => ({ text: s }),
    addEventListener: (k, f) => { docListeners[k] = f; },
  };
  const navigator = { platform: 'Win32' };
  if (read !== null) navigator.clipboard = { readText() { reads.count++; return read(); } };
  function respond(status, body) { return Promise.resolve({ ok: status >= 200 && status < 300, status, json: () => Promise.resolve(body) }); }
  const context = vm.createContext({
    console, URL, Promise,
    window: { wsLogin: { next: '/s/x', checking: 'checking', invalidText: 'invalid', rate: 'rate', clipfail: 'clipfail', unavailable: 'unavailable' }, matchMedia: () => ({ matches: false, addEventListener() {} }) },
    document, navigator,
    location: { hash: '', origin: 'http://127.0.0.1:4000', pathname: '/s/x', search: '', replace() {}, reload() {} },
    history: { replaceState() {} },
    setTimeout: () => 0,
    fetch(url, init) {
      const body = init && init.body ? JSON.parse(init.body) : null;
      calls.push({ url, body });
      if (url === '/auth/status') return respond(401, {});
      if (url === '/auth/clipboard') return respond(copyStatus, copyStatus === 200 ? { ok: true } : { error: 'x' });
      if (url === '/auth/exchange') return respond(200, { next: '/s/x' });
      throw new Error('unexpected fetch ' + url);
    },
  });
  vm.runInContext(source, context);
  const settle = () => new Promise(r => setImmediate(r));
  return { els, calls, reads, docListeners, settle };
}

async function copyThen(read, copyStatus) {
  const g = await run({ read, copyStatus });
  await g.settle();
  g.els['lg-copy'].listeners.click();
  for (let i = 0; i < 5; i++) await g.settle();
  return g;
}
const urls = g => g.calls.map(c => c.url);
const exchanges = g => g.calls.filter(c => c.url === '/auth/exchange');

(async () => {
  // Copy succeeds, the clipboard holds the link: read once, exchange once.
  let g = await copyThen(() => Promise.resolve(LINK));
  assert.deepEqual(urls(g), ['/auth/status', '/auth/clipboard', '/auth/exchange'], 'copy ok -> read -> exchange');
  assert.equal(g.reads.count, 1);
  assert.equal(exchanges(g)[0].body.code === LINK, true, 'exchange carries the clipboard text');
  assert.equal(g.els['lg-input'].value, '…/?login=••••••••', 'credential stays masked');
  assert.equal(g.els['lg-done'].hidden, false, 'exchange success reaches done');
  console.log('PASS copy ok -> read -> exchange');

  // Every read failure falls back quietly: no exchange, no warning, no second
  // copy, and the copied/paste hint stays visible.
  const failures = {
    'read refused': () => Promise.reject(new Error('NotAllowedError')),
    'read throws': () => { throw new Error('sync throw'); },
    'not a login link': () => Promise.resolve('unrelated clipboard text'),
    'empty clipboard': () => Promise.resolve('   '),
    'no clipboard API': null,
  };
  for (const [name, read] of Object.entries(failures)) {
    g = await copyThen(read);
    assert.deepEqual(urls(g), ['/auth/status', '/auth/clipboard'], name + ': no exchange, no recopy');
    assert.equal(g.reads.count, read === null ? 0 : 1, name + ': one read, no loop');
    const hint = g.els['lg-hint'];
    assert.equal(hint.hidden, false, name + ': paste hint shown');
    assert.equal(hint.className, 'lg-hint go', name + ': copied hint, not busy');
    assert.equal(hint.innerHTML, '<span>copied</span><kbd>V</kbd>', name + ': hint names the paste shortcut');
    assert.equal(g.els['lg-msg'].hidden, true, name + ': quiet, no warning');
    assert.equal(g.els['lg-input'].disabled, false, name + ': input stays usable');
    assert.equal(g.els['lg-copy'].disabled, false, name + ': copy stays usable');
    // The paste listener still submits after the quiet fallback.
    g.docListeners.paste({ preventDefault() {}, clipboardData: { getData: () => LINK } });
    await g.settle();
    assert.equal(exchanges(g).length, 1, name + ': paste listener still exchanges');
    console.log('PASS fallback: ' + name);
  }

  // A refused copy never reads the clipboard.
  g = await copyThen(() => Promise.resolve(LINK), 503);
  assert.equal(g.reads.count, 0, 'failed copy does not read');
  assert.deepEqual(urls(g), ['/auth/status', '/auth/clipboard']);
  assert.equal(g.els['lg-native-msg'].hidden, false, 'failed copy shows its note');
  console.log('PASS copy failure skips read');

  // The paste button keeps accepting a bare code and warns on failure.
  g = await run({ read: () => Promise.resolve('bare-code') });
  await g.settle();
  g.els['lg-clip'].listeners.click();
  for (let i = 0; i < 3; i++) await g.settle();
  assert.equal(exchanges(g).length, 1, 'paste button submits a bare code');
  g = await run({ read: () => Promise.reject(new Error('denied')) });
  await g.settle();
  g.els['lg-clip'].listeners.click();
  for (let i = 0; i < 3; i++) await g.settle();
  assert.equal(exchanges(g).length, 0);
  assert.equal(g.els['lg-msg'].hidden, false, 'paste button failure warns');
  assert.equal(g.els['lg-message'].textContent, 'clipfail');
  console.log('PASS paste button unchanged');
})().catch(e => { console.error(e); process.exitCode = 1; });
