const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function environment(now = Date.UTC(2026, 10, 15)) {
  class Element {
    constructor(tag) { this.tagName = tag; this.children = []; this.style = {}; this.attributes = {}; this.events = {}; this._text = ''; }
    set textContent(value) { this._text = value; this.children = []; }
    get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
    appendChild(child) { this.children.push(child); return child; }
    setAttribute(key, value) { this.attributes[key] = value; if (key === 'class') this.className = value; }
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
  const doc = { hidden: false, events: {}, createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), addEventListener(key, fn) { this.events[key] = fn; } };
  const calls = [], ticks = [];
  let payload = { entries: [], warnPercent: 80, critPercent: 95 }, fail = false;
  const window = { setInterval(fn, ms) { ticks.push({ fn, ms }); }, fetch(url, options) { calls.push({ url, options }); return fail ? Promise.reject(new Error('fake failure')) : Promise.resolve({ ok: true, json: () => Promise.resolve(payload) }); } };
  class Clock extends Date { static now() { return now; } }
  vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window, Date: Clock });
  return { window, rail, doc, button, calls, ticks, now(value) { now = value; }, set(value) { payload = value; }, fail() { fail = true; } };
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
  const at = Date.UTC(2026, 10, 15), env = environment(at), reset = at + 3600000;
  env.set({ warnPercent: 80, critPercent: 95, entries: [
    { harness: 'claude', plan: 'max', source: 'Claude push', asOf: at, windows: [{ kind: 'five_hour', percent: 35, resetsAt: reset, asOf: at }, { kind: 'seven_day', percent: 82, resetsAt: reset, asOf: at }] },
    { harness: 'codex', plan: '<fake-plan>', source: 'via Pi', status: 'Pi token expired - open Pi to refresh', asOf: at, windows: [{ kind: 'seven_day', percent: 96, resetsAt: reset, asOf: at }] }
  ] });
  const mounted = env.window.WsUsage.mount(env.doc, env.rail);
  await flush();
  assert.equal(env.button.children.length, 2);
  assert.equal(env.button.children[0].querySelector('.usage-columns').children.length, 2); // C: 5h, week
  assert.equal(env.button.children[1].querySelector('.usage-columns').children.length, 1); // X: week
  assert.equal(env.button.children[1].querySelector('.usage-number').textContent, '96%');
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

test('footer matches normal, over-pace and weekly-only reference columns', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  const w = (kind, percent, elapsed) => ({ kind, percent, asOf: at, resetsAt: at + (1 - elapsed) * (kind === 'five_hour' ? 5 * 3600000 : 7 * 86400000) });
  const entries = [
    { harness: 'claude', windows: [w('seven_day', 53, .66), w('five_hour', 20, 1 / 3)] },
    { harness: 'codex', windows: [w('seven_day', 38, .45)] }
  ];
  env.set({ entries });
  env.window.WsUsage.mount(env.doc, env.rail); await flush();
  const columns = env.button.children.map(g => g.querySelector('.usage-columns').children);
  assert.deepEqual(columns.map(c => c.length), [2, 1]);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-key').textContent)), ['5h', '주', '주']);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-number').textContent)), ['20%', '53%', '38%']);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-pace').textContent)), ['−13%', '−13%', '−7%']);
  assert.ok(columns[0][0].querySelector('.under'));
  entries[0].windows[1].percent = 64;
  env.ticks[0].fn(); await flush();
  assert.equal(env.button.children[0].querySelector('.over').textContent, '+31%');
  env.set({ entries: [entries[1]] }); env.ticks[0].fn(); await flush();
  assert.equal(env.button.children.length, 1);
  assert.equal(env.button.querySelector('.usage-columns').children.length, 1);
  assert.equal(env.button.querySelector('.hi').textContent, 'X');
});

test('stale boundary is strictly after 30 minutes, per window, with pace frozen at observation', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at + 30 * 60000);
  const entry = { harness: 'claude', asOf: at + 30 * 60000, windows: [
    { kind: 'five_hour', percent: 20, asOf: at, resetsAt: at + 5 * 3600000 * 2 / 3 },
    { kind: 'seven_day', percent: 53, resetsAt: at + 7 * 86400000 * .34 }
  ] };
  env.set({ entries: [entry] });
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
  assert.equal(env.button.querySelector('.usage-stale'), null);
  assert.equal(env.button.querySelector('.usage-pace').textContent, '−23%');
  env.now(at + 30 * 60000 + 1); mounted.sync();
  assert.equal(env.button.querySelector('.usage-stale').querySelector('.usage-pace').textContent, '−13%');
  assert.ok(env.button.querySelector('.usage-clock'));
  assert.ok(env.button.querySelector('.usage-stale').attributes['aria-label'].includes('stale, pace at last observation'));
  assert.equal(env.button.querySelector('.usage-columns').children[1].className, 'usage-column');
  env.now(at + 42 * 60000); mounted.sync();
  assert.equal(env.button.querySelector('.usage-stale').querySelector('.usage-pace').textContent, '−13%');
  env.now(at + 55 * 60000); mounted.sync();
  assert.equal(env.button.querySelector('.usage-stale').querySelector('.usage-pace').textContent, '−13%');
  // Stale positive pace must also be neutralized, not just negative pace.
  entry.windows[0].percent = 64; mounted.sync();
  assert.equal(env.button.querySelector('.usage-stale').querySelector('.usage-pace').textContent, '+31%');
  assert.ok(env.button.querySelector('.usage-stale').querySelector('.over'));
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  assert.match(css, /\.usage-stale, \.usage-stale \.usage-pace \{ color: var\(--wb-muted-fg\); \}/);
  assert.match(css, /\.usage-stale \.usage-number \{ text-decoration: underline dotted;/);
});

test('missing and expired windows never create placeholder columns or fabricated pace', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  const entry = { harness: 'claude', windows: [
    { kind: 'five_hour', percent: 99, resetsAt: at, asOf: at },
    { kind: 'seven_day', percent: 38, asOf: at }
  ] };
  env.set({ entries: [entry] });
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
  assert.equal(env.button.querySelector('.usage-columns').children.length, 1);
  assert.equal(env.button.querySelector('.usage-number').textContent, '38%');
  assert.equal(env.button.querySelector('.usage-pace').textContent, '—');
  assert.ok(env.button.querySelector('.usage-column').attributes['aria-label'].includes('pace unavailable'));
  entry.windows[1].resetsAt = at + 7 * 86400000;
  delete entry.windows[1].asOf; mounted.sync();
  assert.equal(env.button.querySelector('.usage-pace').textContent, '—');
  entry.asOf = at; mounted.sync();
  assert.equal(env.button.querySelector('.usage-pace').textContent, '+38%');
  entry.windows[1].percent = 0; mounted.sync();
  assert.equal(env.button.querySelector('.usage-pace').textContent, '0%');
  assert.equal(env.button.querySelector('.under'), null);
  assert.equal(env.button.querySelector('.over'), null);
  entry.windows = []; mounted.sync();
  assert.equal(env.button.querySelector('.usage-column'), null);
  assert.ok(env.button.textContent.includes('usage unavailable'));
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
