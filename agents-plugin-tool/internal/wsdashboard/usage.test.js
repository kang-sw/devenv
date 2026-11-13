const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function environment() {
  class Element {
    constructor(tag) { this.tagName = tag; this.children = []; this.style = {}; this.attributes = {}; this.events = {}; this._text = ''; }
    set textContent(value) { this._text = value; this.children = []; }
    get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
    appendChild(child) { this.children.push(child); return child; }
    setAttribute(key, value) { this.attributes[key] = value; }
    addEventListener(key, fn) { this.events[key] = fn; }
    focus() { this.focused = true; }
    querySelector(selector) {
      for (const child of this.children) {
        if ((child.className || '').split(' ').includes(selector.slice(1))) return child;
        const found = child.querySelector(selector); if (found) return found;
      }
      return null;
    }
  }
  const button = new Element('button'); button.className = 'usage-toggle';
  const rail = new Element('aside'); rail.appendChild(button);
  const doc = { hidden: false, events: {}, createElement: tag => new Element(tag), addEventListener(key, fn) { this.events[key] = fn; } };
  const calls = [], ticks = [];
  let payload = { entries: [], warnPercent: 80, critPercent: 95 }, fail = false;
  const window = { setInterval(fn, ms) { ticks.push({ fn, ms }); }, fetch(url, options) { calls.push({ url, options }); return fail ? Promise.reject(new Error('fake failure')) : Promise.resolve({ ok: true, json: () => Promise.resolve(payload) }); } };
  vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window });
  return { window, rail, doc, button, calls, ticks, set(value) { payload = value; }, fail() { fail = true; } };
}
const flush = () => new Promise(resolve => setImmediate(resolve));

test('usage ordering, expired windows, thresholds and ages', () => {
  const u = environment().window.WsUsage;
  const entries = [
    { harness: 'claude', windows: [{ percent: 99, resetsAt: 5 }, { percent: 40, resetsAt: 50 }] },
    { harness: 'codex', windows: [{ percent: 80, resetsAt: 50 }] }
  ];
  assert.equal(u.sorted(entries, 10)[0].harness, 'codex');
  assert.equal(entries[0].harness, 'claude');
  assert.equal(u.active(entries[0], 10).length, 1);
  for (const [p, want] of [[79, 'normal'], [80, 'warning'], [94, 'warning'], [95, 'critical']]) assert.equal(u.level(p, 80, 95), want);
  assert.equal(u.age(60000, 180000), 'value from 2 min ago');
  assert.equal(u.age(0, 180000), 'usage unavailable');
});

test('footer and popover show only detected harnesses, source, age, reset and safe text', async () => {
  const env = environment(), at = Date.now(), reset = at + 3600000;
  env.set({ warnPercent: 80, critPercent: 95, entries: [
    { harness: 'claude', plan: 'max', source: 'Claude push', asOf: at, windows: [{ kind: 'five_hour', percent: 35, resetsAt: reset, asOf: at }, { kind: 'seven_day', percent: 82, resetsAt: reset, asOf: at }] },
    { harness: 'codex', plan: '<fake-plan>', source: 'via Pi', status: 'Pi token expired - open Pi to refresh', asOf: at, windows: [{ kind: 'seven_day', percent: 96, resetsAt: reset, asOf: at }] }
  ] });
  const mounted = env.window.WsUsage.mount(env.doc, env.rail);
  await flush();
  assert.equal(env.button.children.length, 2);
  assert.equal(env.button.children[0].children.length, 3); // C, 5h, week
  assert.equal(env.button.children[1].children.length, 2); // X, week
  assert.equal(env.button.children[1].children[1].children[0].style.height, '96%');
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); pop.focus(); await flush();
  const text = pop.el.textContent;
  assert.ok(text.indexOf('Codex / GPT') < text.indexOf('Claude'));
  for (const s of ['96%', 'Weekly', '5-hour', 'via Pi', 'Claude push', 'value from 0 min ago', 'Resets ', '<fake-plan>', 'Pi token expired']) assert.ok(text.includes(s), s);
  assert.equal(pop.el.attributes.role, 'dialog');
  assert.equal(pop.el.focused, true);
  assert.ok(env.calls[1].url.endsWith('?refresh=1'));
  assert.equal(env.calls[0].options.credentials, 'same-origin');
  env.button.textContent = ''; mounted.sync(); assert.equal(env.button.children.length, 2);
  env.fail(); env.ticks[0].fn(); await flush();
  assert.ok(pop.el.textContent.includes('usage unavailable'));
  assert.ok(pop.el.textContent.includes('96%')); // retained last value
  pop.close();
});

test('empty login state and hidden-page cadence', async () => {
  const env = environment();
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.ok(pop.el.textContent.includes('not logged in'));
  assert.equal(env.button.textContent, 'Plan usage');
  assert.equal(env.ticks[0].ms, 300000);
  const before = env.calls.length;
  env.doc.hidden = true; env.ticks[0].fn(); await flush(); assert.equal(env.calls.length, before);
  env.doc.hidden = false; env.doc.events.visibilitychange(); await flush(); assert.equal(env.calls.length, before + 1);
});
