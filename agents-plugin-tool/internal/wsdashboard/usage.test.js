const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function environment(now = Date.UTC(2026, 10, 15), locale = 'en') {
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
  // The header's center cluster (the tcen template): the usage handle and
  // the global alert slot beside it.
  const button = new Element('button'); button.className = 'usage-toggle';
  const alert = new Element('span'); alert.className = 'talert'; alert.textContent = 'Alert';
  const center = new Element('span'); center.className = 'tcen'; center.appendChild(button); center.appendChild(alert);
  const top = new Element('div'); top.className = 'top'; top.appendChild(center);
  const doc = { hidden: false, events: {}, createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), addEventListener(key, fn) { this.events[key] = fn; } };
  const calls = [], ticks = [], timers = [];
  let payload = { entries: [], warnPercent: 80, critPercent: 95 }, fail = false;
  const window = { setInterval(fn, ms) { ticks.push({ fn, ms }); }, setTimeout(fn, ms) { timers.push({ fn, ms, live: true }); return timers.length; }, clearTimeout(id) { if (timers[id - 1]) timers[id - 1].live = false; }, fetch(url, options) { calls.push({ url, options }); return fail ? Promise.reject(new Error('fake failure')) : Promise.resolve({ ok: true, json: () => Promise.resolve(payload) }); } };
  window.wsI18n = { lang: locale, t: JSON.parse(fs.readFileSync(__dirname + '/../wsi18n/tables/' + locale + '.json', 'utf8')) };
  window.wsT = function (key, ...args) {
    assert.ok(Object.hasOwn(window.wsI18n.t, key), key);
    return window.wsI18n.t[key].replace(/\{(\d+)\}/g, (match, i) => i < args.length ? String(args[i]) : match);
  };
  class Clock extends Date { static now() { return now; } }
  vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window, Date: Clock });
  return { window, top, doc, button, alert, center, calls, ticks, timers, now(value) { now = value; }, set(value) { payload = value; }, fail() { fail = true; } };
}
const flush = () => new Promise(resolve => setImmediate(resolve));
// groups are the handle's harness groups, after its compact trigger.
const groups = button => button.children.filter(c => c.className === 'usage-harness');
function nodes(root, tagOrClass) {
  return root.children.flatMap(child => [
    ...(tagOrClass.startsWith('.') ? (child.className || '').split(' ').includes(tagOrClass.slice(1)) : child.tagName === tagOrClass) ? [child] : [],
    ...nodes(child, tagOrClass)
  ]);
}
async function chartFixture({ history = 'ok', resetKnown = true, stale = false, percents = [10, 60], weekly = false, locale = 'en' } = {}) {
  const start = Date.UTC(2026, 10, 15), duration = 5 * 3600000, asOf = start + .4 * duration;
  const env = environment(asOf + (stale ? 42 * 60000 : 0), locale), resetsAt = resetKnown ? start + duration : 0;
  const samples = percents.map((percent, i) => ({ harness: 'claude', kind: 'five_hour', source: '<safe-source>', resetsAt, asOf: start + (i ? .4 : .2) * duration, percent }));
  const entry = { harness: 'claude', source: '<safe-source>', asOf, windows: [samples.at(-1)] };
  if (weekly) entry.windows.unshift({ kind: 'seven_day', percent: 99, asOf, resetsAt: asOf + 86400000 });
  const payload = { entries: [entry] };
  if (history !== 'missing') payload.history = { schema_version: 1, status: history, samples };
  env.set(payload);
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  return { env, pop, payload };
}

test('compact cards reuse crossing geometry, continuous endpoint gradients and excess-only fill', async () => {
  const { pop } = await chartFixture({ weekly: true });
  const cards = nodes(pop.el, '.usage-window-card');
  assert.equal(cards.length, 2);
  assert.ok(cards[0].querySelector('.usage-window').textContent.startsWith('5-hour'));
  assert.ok(cards[1].querySelector('.usage-window').textContent.startsWith('Weekly'));
  const chart = cards[0].querySelector('.usage-chart');
  assert.equal(chart.attributes.viewBox, '0 0 288 113');
  assert.equal(chart.attributes.role, 'img');
  assert.match(chart.attributes['aria-label'], /not prediction/);
  assert.equal(nodes(chart, '.usage-ideal')[0].attributes['stroke-dasharray'], '4 4');
  const gradients = nodes(chart, 'linearGradient'), lines = nodes(chart, '.usage-actual');
  assert.equal(gradients.length, 2); assert.equal(lines.length, 2);
  const colors = gradients.map(g => g.children.map(s => s.attributes['stop-color']));
  assert.deepEqual(colors, [
    ['color-mix(in srgb, var(--wb-muted-fg), var(--wb-ok) 10%)', 'var(--wb-muted-fg)'],
    ['var(--wb-muted-fg)', 'color-mix(in srgb, var(--wb-muted-fg), var(--wb-fail) 20%)']
  ]);
  gradients.forEach((g, i) => {
    assert.equal(g.attributes.gradientUnits, 'userSpaceOnUse');
    assert.equal(lines[i].attributes.stroke, 'url(#' + g.attributes.id + ')');
    assert.equal(lines[i].attributes['stroke-width'], '2.3');
    assert.equal(lines[i].attributes['stroke-dasharray'], '4 3');
    assert.equal(nodes(lines[i], 'title')[0].textContent, 'no record (estimated)');
  });
  const polygons = nodes(chart, 'polygon'); assert.equal(polygons.length, 1);
  assert.equal(polygons[0].attributes.fill, 'var(--wb-fail)');
  assert.equal(polygons[0].attributes.opacity, '0.09');
  const vertices = polygons[0].attributes.points.split(' ').map(p => p.split(',').map(Number));
  assert.equal(vertices[0][0], vertices[3][0]); assert.equal(vertices[1][0], vertices[2][0]);
  assert.equal(vertices[2][1], 88 - .4 * 76);
  assert.notEqual(vertices[2][1], 88); // closes against ideal, never the zero axis
  assert.equal(nodes(chart, '.usage-point').length, 2); // crossing is not a real dot
  assert.equal(nodes(chart, '.usage-point-current')[0].attributes.r, '4');
  assert.ok(!pop.el.textContent.includes('<safe-source>'));
});

test('legend and current-time guide explain the chart without advancing stale observations', async () => {
  const { env, pop } = await chartFixture({ stale: true });
  const legend = pop.el.querySelector('.usage-chart-legend');
  for (const label of ['Observed usage', 'Ideal burn, not prediction', 'Latest observation', 'No record (estimated)']) assert.ok(legend.textContent.includes(label), label);
  for (const kind of ['actual', 'ideal', 'point', 'estimated']) assert.equal(legend.querySelector('.usage-swatch-' + kind).attributes['aria-hidden'], 'true');
  const start = Date.UTC(2026, 10, 15), duration = 5 * 3600000;
  const assertGuide = now => {
    const chart = pop.el.querySelector('.usage-chart'), guide = chart.querySelector('.usage-now');
    assert.equal(nodes(chart, '.usage-point').length, 2);
    assert.equal(chart.querySelector('.usage-point-current').attributes.cx, String(24 + .4 * 246));
    assert.equal(nodes(chart, '.usage-actual').at(-1).attributes.x2, String(24 + .4 * 246));
    const x = Number(guide.attributes.x1);
    assert.ok(Math.abs(x - (24 + (now - start) / duration * 246)) < 1e-9);
    assert.equal(guide.attributes.x2, guide.attributes.x1);
    assert.equal(guide.attributes['stroke-dasharray'], '2 3');
    assert.equal(nodes(guide, 'title')[0].textContent, 'Current time; not an observation');
    assert.equal(chart.querySelector('.usage-now-label').textContent, 'Now');
  };
  assertGuide(start + .4 * duration + 42 * 60000);
  env.now(start + .8 * duration); env.ticks[0].fn(); await flush();
  assertGuide(start + .8 * duration);
  assert.equal(nodes(pop.el, '.usage-chart-legend').length, 1); // redraw never duplicates the key
  for (const now of [start - 1, start, start + duration - 1]) {
    env.now(now); env.ticks[0].fn(); await flush();
    assert.equal(nodes(pop.el, '.usage-now').length, now < start ? 0 : 1);
  }
});

test('all eleven locales localize popup labels, chart titles, accessible text and weekday dates', async () => {
  const locales = fs.readdirSync(__dirname + '/../wsi18n/tables').map(name => name.replace('.json', ''));
  assert.equal(locales.length, 11);
  for (const locale of locales) {
    const { env, pop, payload } = await chartFixture({ weekly: true, stale: true, locale });
    const t = key => env.window.wsT('js.usage.' + key.toLowerCase());
    assert.equal(pop.el.attributes['aria-label'], t('heading'));
    assert.equal(pop.el.querySelector('.usage-close').attributes['aria-label'], t('close'));
    const help = pop.el.querySelector('.usage-help');
    assert.equal(help.children[0].tagName, 'button');
    assert.equal(help.children[0].type, 'button');
    assert.equal(help.children[0].textContent, '?');
    assert.equal(help.children[0].attributes['aria-label'], t('legend'));
    assert.equal(help.children[0].attributes['aria-describedby'], help.children[1].attributes.id);
    assert.equal(help.children[1].attributes.role, 'tooltip');
    assert.equal(pop.el.children[0].children[1], help); // immediately after heading, before close
    for (const key of ['observed', 'ideal', 'latest', 'estimated']) assert.ok(help.textContent.includes(t(key)), locale + ': ' + key);
    const cards = nodes(pop.el, '.usage-window-card');
    assert.ok(cards[0].textContent.includes(t('fiveHour')));
    assert.ok(cards[0].attributes['aria-label'].includes(t('stalePace')));
    assert.ok(cards[0].title.includes(t('historyRecorded')));
    const titles = nodes(cards[0], 'title').map(n => n.textContent);
    for (const key of ['idealTitle', 'nowTitle', 'estimatedTitle']) assert.ok(titles.includes(t(key)), locale + ': ' + key);
    assert.ok(titles.some(text => text.includes(t('current')) && text.includes(t('stale'))));
    const weekly = cards[1].querySelector('.usage-chart');
    const reset = payload.entries[0].windows[0].resetsAt;
    const formatter = new Intl.DateTimeFormat(locale, { weekday: 'short' });
    const labels = nodes(weekly, '.usage-day-label');
    assert.ok(labels.length >= 3, locale);
    for (const label of labels) {
      const x = Number(label.attributes.x);
      const at = reset - 7 * 86400000 + (x - 24) / 246 * 7 * 86400000;
      assert.equal(label.textContent, formatter.format(new Date(at)), locale);
      assert.equal(label.attributes['text-anchor'], 'middle');
      assert.ok(x > 24 && x < 270);
    }
    assert.equal(nodes(weekly, '.usage-day-grid').length, 7);
    assert.equal(nodes(cards[0], '.usage-day-grid').length, 0);
    assert.equal(nodes(cards[0], '.usage-day-label').length, 0);
    assert.doesNotMatch(pop.el.textContent, /js\.usage\.|<safe-source>/);
  }
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  assert.match(css, /\.usage-chart-legend \{[^}]*display: none/);
  assert.match(css, /\.usage-help:hover \.usage-chart-legend, \.usage-help:focus-within \.usage-chart-legend \{ display: grid; \}/);
});

test('baseline equality is muted, below-baseline charts have no excess fill, and short intervals are solid', async () => {
  for (const percents of [[20, 40], [10, 20], [40, 60]]) {
    const { env, pop, payload } = await chartFixture({ percents });
    const chart = pop.el.querySelector('.usage-chart');
    assert.equal(nodes(chart, '.usage-excess').length, percents[0] === 40 ? 1 : 0);
    if (percents[0] === 20) assert.ok(nodes(chart, 'stop').every(s => s.attributes['stop-color'] === 'var(--wb-muted-fg)'));
    payload.history.samples[0].asOf = payload.entries[0].asOf - 600000;
    env.ticks[0].fn(); await flush();
    assert.ok(nodes(pop.el, '.usage-actual').every(n => !n.attributes['stroke-dasharray']));
  }
});

test('missing, empty and unavailable history are truthful current-only fallbacks', async () => {
  for (const history of ['missing', 'empty', 'unavailable']) {
    const { pop } = await chartFixture({ history });
    assert.equal(nodes(pop.el, '.usage-point').length, 1);
    assert.equal(nodes(pop.el, '.usage-actual').length, 0);
    assert.equal(nodes(pop.el, 'polygon').length, 0);
    assert.equal(nodes(pop.el, '.usage-ideal').length, 1);
    assert.equal(pop.el.querySelector('.usage-history-state'), null);
    const card = pop.el.querySelector('.usage-window-card');
    assert.match(card.title, /current observation only/);
    assert.match(card.title, history === 'empty' ? /No history recorded/ : /History unavailable/);
    assert.doesNotMatch(pop.el.textContent, /History unavailable|No history recorded|current observation only/);
  }
});

test('unknown resets have isolated real dots without baseline, gradients or connected lines', async () => {
  const { pop } = await chartFixture({ resetKnown: false });
  assert.equal(nodes(pop.el, '.usage-point').length, 2);
  for (const selector of ['.usage-ideal', '.usage-actual', '.usage-now', '.usage-now-label', 'linearGradient', 'polygon']) assert.equal(nodes(pop.el, selector).length, 0);
  assert.match(pop.el.textContent, /Resets unknown/);
  assert.match(pop.el.querySelector('.usage-window-card').title, /reset unknown, isolated observations only/);
});

test('gradient ids remain unique across cards, redraws and repeated mounts', async () => {
  const { env, pop } = await chartFixture({ weekly: true });
  const first = nodes(pop.el, 'linearGradient').map(g => g.attributes.id);
  env.ticks[0].fn(); await flush();
  const second = nodes(pop.el, 'linearGradient').map(g => g.attributes.id);
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const another = mounted.makePopover(() => {}, () => {}); another.open(); await flush();
  const third = nodes(another.el, 'linearGradient').map(g => g.attributes.id);
  const all = [...first, ...second, ...third]; assert.equal(new Set(all).size, all.length);
});

test('stale cards neutralize only the current marker and freeze pace without recoloring history', async () => {
  const { env, pop } = await chartFixture({ stale: true, percents: [10, 20] });
  const card = pop.el.querySelector('.usage-window-card');
  assert.ok(card.className.includes('usage-stale'));
  assert.ok(card.querySelector('.usage-clock'));
  assert.equal(card.querySelector('.usage-pace').textContent, '(−20%)');
  assert.match(card.attributes['aria-label'], /stale, pace at last observation/);
  assert.equal(card.querySelector('.usage-point-current').attributes.fill, 'var(--wb-muted-fg)');
  assert.match(nodes(card, 'stop')[0].attributes['stop-color'], /--wb-ok/);
  env.now(Date.UTC(2026, 10, 15) + 3 * 3600000); env.ticks[0].fn(); await flush();
  assert.equal(pop.el.querySelector('.usage-pace').textContent, '(−20%)');
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  assert.match(css, /\.usage-pop \{ width: 340px;/);
  assert.match(css, /\.usage-stale \.usage-number \{ text-decoration: underline dotted;/);
  assert.match(css, /\.usage-stale, \.usage-stale \.usage-pace \{ color: var\(--wb-muted-fg\); \}/);
});

test('usage ordering, expired windows, thresholds and ages', () => {
  const u = environment().window.WsUsage;
  const entries = [
    { harness: 'agy', group: 'claude_gpt', windows: [{ percent: 95, resetsAt: 50 }] },
    { harness: 'codex', windows: [{ percent: 80, resetsAt: 50 }] },
    { harness: 'agy', group: 'gemini', windows: [{ percent: 15, resetsAt: 50 }] },
    { harness: 'claude', windows: [{ percent: 99, resetsAt: 5 }, { percent: 40, resetsAt: 50 }] }
  ];
  assert.deepEqual(
    u.sorted(entries).map(e => e.group ? e.harness + ':' + e.group : e.harness),
    ['claude', 'codex', 'agy:gemini', 'agy:claude_gpt']
  );
  assert.equal(entries[0].harness, 'agy');
  assert.equal(u.active(entries[3], 10).length, 1);
  for (const [p, want] of [[79, 'normal'], [80, 'warning'], [94, 'warning'], [95, 'critical']]) assert.equal(u.level(p, 80, 95), want);
  assert.equal(u.age(60000, 180000), 'value from 2 min ago');
  assert.equal(u.age(0, 180000), 'usage unavailable');
});

test('footer and popover show detected harnesses, age, reset and safe text but hide source', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at), reset = at + 3600000;
  env.set({ warnPercent: 80, critPercent: 95, entries: [
    { harness: 'claude', plan: 'max', source: 'Claude push', asOf: at, windows: [{ kind: 'five_hour', percent: 35, resetsAt: reset, asOf: at }, { kind: 'seven_day', percent: 82, resetsAt: reset, asOf: at }] },
    { harness: 'codex', plan: '<fake-plan>', source: 'via Pi', status: 'Pi token expired - open Pi to refresh', asOf: at, windows: [{ kind: 'seven_day', percent: 96, resetsAt: reset, asOf: at }] }
  ] });
  const mounted = env.window.WsUsage.mount(env.doc, env.top);
  await flush();
  assert.equal(groups(env.button).length, 2);
  assert.equal(groups(env.button)[0].querySelector('.usage-columns').children.length, 2); // C: 5h, week
  assert.equal(groups(env.button)[1].querySelector('.usage-columns').children.length, 1); // X: week
  assert.equal(groups(env.button)[1].querySelector('.usage-number').textContent, '96%');
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); pop.focus(); await flush();
  const text = pop.el.textContent;
  assert.ok(text.indexOf('Claude') < text.indexOf('Codex / GPT'));
  for (const s of ['96%', 'Weekly', '5-hour', '0m ago', '<fake-plan>', 'Pi token expired']) assert.ok(text.includes(s), s);
  const resetEl = pop.el.querySelector('.usage-reset');
  assert.ok(resetEl.textContent.startsWith('↻ '));
  assert.ok(resetEl.title.startsWith('Resets '));
  for (const source of ['via Pi', 'Claude push']) assert.ok(!text.includes(source), source);
  assert.equal(pop.el.attributes.role, 'dialog');
  assert.equal(pop.el.focused, true);
  assert.equal(env.calls[1].url, '/rail/usage?history=1&refresh=1');
  assert.equal(env.calls[0].url, '/rail/usage?history=1');
  assert.equal(env.calls[1].options.headers['X-WS-Usage-Refresh'], '1');
  assert.equal(env.calls[0].options.credentials, 'same-origin');
  env.button.textContent = ''; mounted.sync(); assert.equal(groups(env.button).length, 2);
  env.fail(); env.ticks[0].fn(); await flush();
  assert.ok(pop.el.textContent.includes('usage unavailable'));
  assert.ok(pop.el.textContent.includes('96%')); // retained last value
  pop.close();
});

test('fractional percentages round like the server and pace, in footer, title and popover', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at), resetsAt = at + 3600000;
  const cases = [[59.6, '60'], [59.000000001, '59'], [28.999999999999996, '29'], [59.4, '59']];
  const mounted = env.window.WsUsage.mount(env.doc, env.top);
  const pop = mounted.makePopover(() => {}, () => {}); pop.open();
  for (const [percent, want] of cases) {
    env.set({ entries: [{ harness: 'codex', asOf: at, windows: [{ kind: 'seven_day', percent, resetsAt, asOf: at }] }] });
    env.ticks[0].fn(); await flush();
    assert.equal(env.button.querySelector('.usage-number').textContent, want + '%', String(percent));
    assert.ok(env.button.querySelector('.usage-column').title.includes('Weekly: ' + want + '%;'), String(percent));
    assert.ok(pop.el.querySelector('.usage-window').textContent.includes(want + '%'), String(percent));
  }
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
  env.window.WsUsage.mount(env.doc, env.top); await flush();
  const columns = groups(env.button).map(g => g.querySelector('.usage-columns').children);
  assert.deepEqual(columns.map(c => c.length), [2, 1]);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-key').textContent)), ['5h', '주', '주']);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-number').textContent)), ['20%', '53%', '38%']);
  assert.deepEqual(columns.flatMap(c => c.map(v => v.querySelector('.usage-pace').textContent)), ['−13%', '−13%', '−7%']);
  assert.ok(columns[0][0].querySelector('.under'));
  entries[0].windows[1].percent = 64;
  env.ticks[0].fn(); await flush();
  assert.equal(groups(env.button)[0].querySelector('.over').textContent, '+31%');
  env.set({ entries: [entries[1]] }); env.ticks[0].fn(); await flush();
  assert.equal(groups(env.button).length, 1);
  assert.equal(env.button.querySelector('.usage-columns').children.length, 1);
  assert.equal(env.button.querySelector('.hb').attributes['data-h'], 'codex');
});

test('stale boundary is strictly after 30 minutes, per window, with pace frozen at observation', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at + 30 * 60000);
  const entry = { harness: 'claude', asOf: at + 30 * 60000, windows: [
    { kind: 'five_hour', percent: 20, asOf: at, resetsAt: at + 5 * 3600000 * 2 / 3 },
    { kind: 'seven_day', percent: 53, resetsAt: at + 7 * 86400000 * .34 }
  ] };
  env.set({ entries: [entry] });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
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
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
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

test('history composes into the single coalesced viewer request', async () => {
  const env = environment();
  const mounted = env.window.WsUsage.mount(env.doc, env.top);
  const pop = mounted.makePopover(() => {}, () => {});
  pop.open(); env.ticks[0].fn(); env.doc.events.visibilitychange();
  assert.equal(env.calls.length, 1); // mount request is still pending
  assert.equal(env.calls[0].url, '/rail/usage?history=1');
  await flush();
  assert.equal(env.calls.length, 2); // the open queues one forced request after the poll
  assert.equal(env.calls[1].url, '/rail/usage?history=1&refresh=1');
  assert.equal(env.calls[1].options.cache, 'no-store');
  pop.open(); pop.open();
  assert.equal(env.calls.length, 3); // repeated opens join this forced request
  await flush();
  assert.equal(env.calls.length, 3);
});

test('opening during an ordinary pending request queues one forced refresh and its follow-ups', async () => {
  const env = environment();
  let finish;
  env.window.fetch = (url, options) => {
    env.calls.push({ url, options });
    return new Promise(resolve => { finish = () => resolve({ ok: true, json: () => Promise.resolve({ entries: [] }) }); });
  };
  const mounted = env.window.WsUsage.mount(env.doc, env.top);
  const pop = mounted.makePopover(() => {}, () => {});
  pop.open(); pop.open();
  assert.equal(env.calls.length, 1);
  finish(); await flush();
  assert.equal(env.calls.length, 2);
  assert.equal(env.calls[1].url, '/rail/usage?history=1&refresh=1');
  assert.equal(env.calls[1].options.headers['X-WS-Usage-Refresh'], '1');
  pop.open(); // already-forced request must not queue another probe
  finish(); await flush();
  assert.equal(env.calls.length, 2);
  assert.deepEqual(env.timers.filter(t => t.live).map(t => t.ms), [2000, 11000, 32000]);
});

test('a forced refresh refetches while the popover is open, so the background probe result shows', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  const entry = percent => ({ entries: [{ harness: 'codex', asOf: at, windows: [{ kind: 'seven_day', percent, asOf: at, resetsAt: at + 86400000 }] }] });
  env.set(entry(10));
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  assert.equal(env.timers.length, 0); // an ordinary poll schedules no follow-up
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.equal(env.calls.at(-1).url, '/rail/usage?history=1&refresh=1');
  assert.deepEqual(env.timers.map(t => t.ms), [2000, 11000, 32000]);
  env.set(entry(42)); // the daemon's background probe lands
  env.timers[0].fn(); await flush();
  assert.equal(env.calls.at(-1).url, '/rail/usage?history=1');
  assert.ok(pop.el.textContent.includes('42%'));
  assert.equal(env.button.querySelector('.usage-number').textContent, '42%');
  pop.open(); await flush(); // reopening replaces, not stacks, the follow-ups
  assert.deepEqual(env.timers.filter(t => t.live).map(t => t.ms), [2000, 11000, 32000]);
  const before = env.calls.length;
  pop.close(); env.timers.at(-1).fn(); await flush();
  assert.equal(env.calls.length, before); // a closed popover does not refetch
});

test('handle redraw and sync preserve the sibling alert slot', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const unchanged = () => {
    assert.deepEqual(env.center.children, [env.button, env.alert]);
    assert.equal(env.alert.textContent, 'Alert');
    assert.equal(env.alert.className, 'talert');
  };
  unchanged(); // empty usage leaves the alert slot alone
  env.set({ entries: [{ harness: 'codex', asOf: at, windows: [{ kind: 'seven_day', percent: 38, asOf: at, resetsAt: at + 86400000 }] }] });
  env.ticks[0].fn(); await flush();
  assert.equal(env.button.querySelector('.usage-number').textContent, '38%');
  unchanged();
  env.button.textContent = ''; mounted.sync();
  assert.equal(env.button.querySelector('.usage-number').textContent, '38%');
  unchanged();
  env.set({ entries: [] }); env.ticks[0].fn(); await flush();
  assert.equal(env.button.textContent, 'Plan usage');
  unchanged();
});

test('the header handle keeps one row: small badge marks clear of the columns, a compact trigger when short', () => {
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  const rule = selector => {
    const start = css.indexOf('\n' + selector + ' {') + 1;
    assert.ok(start > 0, selector);
    return css.slice(start, css.indexOf('}', start) + 1);
  };
  // Plan usage left the rail footer for the header's center.
  assert.doesNotMatch(css, /\.rail \.(rfoot|usage-toggle)/);
  for (const selector of ['.usage-harness', '.usage-columns']) assert.match(rule(selector), /flex-wrap: nowrap/);
  const mark = rule('.usage-mark');
  for (const part of ['width: 19px', 'height: 16px', 'margin-right: 3px', 'flex: none']) assert.ok(mark.includes(part), part);
  const clock = rule('.usage-clock');
  for (const part of ['right: -4px', 'bottom: -3px']) assert.ok(clock.includes(part), part);
  const column = rule('.usage-column');
  assert.match(column, /display: inline-flex/);
  assert.match(column, /flex-direction: column/); // current % above pace delta
  assert.match(rule('.tcen .usage-number'), /font-size: 10\.5px/);
  assert.match(rule('.tcen .usage-pace'), /font-size: 9\.5px/);
  assert.match(rule('.usage-key'), /9px\/1\.2/);
  // Where the header runs short (tu) only the compact trigger shows.
  assert.match(rule('.usage-mini'), /display: none/);
  assert.match(rule('.dwrap > .top.tu .tcen > .usage-toggle'), /width: 34px/);
  assert.match(rule('.dwrap > .top.tu .tcen > .usage-toggle > :not(.usage-mini)'), /display: none/);
  assert.match(rule('.dwrap > .top.tu .tcen > .usage-toggle > .usage-mini'), /display: inline-flex/);
  const handleCss = css.slice(0, css.indexOf('.rpop.usage-pop'));
  assert.doesNotMatch(handleCss, /overflow:\s*hidden|text-overflow:\s*ellipsis/);
});

test('the handle marks each harness with the small badge and leads with the compact trigger', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  env.set({ entries: [
    { harness: 'claude', asOf: at - 42 * 60000, windows: [{ kind: 'five_hour', percent: 26, asOf: at - 42 * 60000, resetsAt: at + 3600000 }, { kind: 'seven_day', percent: 53, asOf: at - 42 * 60000, resetsAt: at + 86400000 }] },
    { harness: 'codex', asOf: at, windows: [{ kind: 'seven_day', percent: 79, asOf: at, resetsAt: at + 86400000 }] }] });
  env.window.WsUsage.mount(env.doc, env.top); await flush();
  assert.equal(env.button.children[0].className, 'usage-mini');
  const marks = nodes(env.button, '.usage-mark');
  assert.equal(marks.length, 2);
  assert.deepEqual(marks.map(m => [m.children[0].className, m.children[0].attributes['data-h']]), [['hb sm', 'claude'], ['hb sm', 'codex']]);
  assert.equal(nodes(marks[0], 'svg').length, 1); // the stale clock rides the stale harness's badge
  assert.equal(nodes(marks[1], 'svg').length, 0);
});

test('a reply without the account list draws the empty handle, never throws', async () => {
  const env = environment();
  env.set({});
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  assert.equal(env.button.textContent, 'Plan usage');
  mounted.sync();
  assert.equal(env.button.textContent, 'Plan usage');
});

test('empty login state and hidden-page cadence', async () => {
  const env = environment();
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.ok(pop.el.textContent.includes('not logged in'));
  assert.equal(env.button.textContent, 'Plan usage');
  assert.equal(env.ticks[0].ms, 300000);
  const before = env.calls.length;
  env.doc.hidden = true; env.ticks[0].fn(); await flush(); assert.equal(env.calls.length, before);
  env.doc.hidden = false; env.doc.events.visibilitychange(); await flush(); assert.equal(env.calls.length, before + 1);
});

test('agy renders two per-group entries with shared badge, visible footer age, and disabled-bucket rules', async () => {
  const at = Date.UTC(2026, 10, 19, 12, 0), observed = at - 12 * 60000, env = environment(at);
  const reset5h = observed + 3 * 3600000, resetWk = observed + 4 * 86400000;
  env.set({
    entries: [
      { harness: 'claude', asOf: at, windows: [{ kind: 'five_hour', percent: 25, asOf: at, resetsAt: reset5h }] },
      { harness: 'agy', group: 'gemini', label: 'Gemini Models', source: 'via agy CLI', asOf: observed, windows: [
        { kind: 'five_hour', percent: 40, asOf: observed, resetsAt: reset5h },
        { kind: 'seven_day', percent: 55, asOf: observed, resetsAt: resetWk }
      ] },
      { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', source: 'via agy CLI', asOf: observed, windows: [
        { kind: 'five_hour', percent: 80, asOf: observed, resetsAt: reset5h, disabled: true },
        { kind: 'seven_day', percent: 30, asOf: observed, resetsAt: resetWk }
      ] }
    ],
    agyHistory: { schema_version: 1, status: 'ok', samples: [
      { harness: 'agy', group: 'gemini', kind: 'five_hour', source: 'via agy CLI', resetsAt: reset5h, asOf: observed - 20 * 60000, percent: 15 },
      { harness: 'agy', group: 'gemini', kind: 'five_hour', source: 'via agy CLI', resetsAt: reset5h, asOf: observed, percent: 40 },
      { harness: 'agy', group: 'claude_gpt', kind: 'seven_day', source: 'via agy CLI', resetsAt: resetWk, asOf: observed - 3600000, percent: 10 },
      { harness: 'agy', group: 'claude_gpt', kind: 'seven_day', source: 'via agy CLI', resetsAt: resetWk, asOf: observed, percent: 30 }
    ] }
  });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const footerGroups = groups(env.button);
  assert.equal(footerGroups.length, 3);
  const agyFooter = footerGroups.slice(1);
  assert.deepEqual(agyFooter.map(g => g.querySelector('.hb').attributes['data-h']), ['agy', 'agy']);
  assert.deepEqual(agyFooter.map(g => g.querySelector('.usage-group-label').textContent), ['Gemini', 'Claude/GPT']);
  assert.deepEqual(agyFooter.map(g => g.querySelector('.usage-group-age').textContent), ['12m ago', '12m ago']);
  assert.equal(agyFooter[0].querySelector('.usage-group-age').title, 'value from 12 min ago');
  assert.equal(agyFooter[0].querySelector('.usage-columns').children.length, 2);
  assert.equal(agyFooter[1].querySelector('.usage-columns').children.length, 1);
  assert.equal(agyFooter[1].querySelector('.usage-number').textContent, '30%');

  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  const sections = nodes(pop.el, '.usage-account');
  assert.equal(sections.length, 3);
  const titles = sections.map(s => s.querySelector('.usage-heading').textContent);
  assert.ok(titles.includes('Gemini Models'));
  assert.ok(titles.includes('Claude and GPT models'));
  assert.equal(sections.filter(s => s.querySelector('.hb').attributes['data-h'] === 'agy').length, 2);
  const cgSection = sections.find(s => s.querySelector('.usage-heading').textContent === 'Claude and GPT models');
  const cgCards = nodes(cgSection, '.usage-window-card');
  assert.equal(cgCards.length, 2);
  assert.ok(cgCards[0].className.includes('usage-disabled'));
  assert.equal(cgCards[0].querySelector('.usage-disabled-tag').textContent, 'disabled');
  assert.ok(!cgCards[1].className.includes('usage-disabled'));
  const gemSection = sections.find(s => s.querySelector('.usage-heading').textContent === 'Gemini Models');
  assert.ok(nodes(nodes(gemSection, '.usage-window-card')[0], '.usage-actual').length >= 1);
  assert.equal(nodes(cgCards[0], '.usage-actual').length, 0);

  env.set({ entries: [
    { harness: 'agy', group: 'gemini', label: 'Gemini Models', source: 'via agy CLI', asOf: observed, windows: [
      { kind: 'five_hour', percent: 40, asOf: observed, resetsAt: reset5h, disabled: true }
    ] }
  ] });
  env.ticks[0].fn(); await flush();
  assert.equal(groups(env.button).length, 0);
  assert.equal(env.button.textContent, 'Plan usage');
  pop.close();
});

test('agy sign-in required status localizes and D15 32s cache-only follow-up clears on close', async () => {
  const at = Date.UTC(2026, 10, 19, 12, 0), env = environment(at, 'en');
  env.set({ entries: [{ harness: 'agy', source: 'via agy CLI', status: 'sign-in required', windows: [] }] });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  assert.equal(groups(env.button).length, 1);
  assert.equal(groups(env.button)[0].querySelector('.hb').attributes['data-h'], 'agy');
  assert.ok(groups(env.button)[0].textContent.includes('sign-in required'));
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.ok(pop.el.textContent.includes('Antigravity'));
  assert.ok(pop.el.textContent.includes('sign-in required'));

  const live32 = env.timers.find(t => t.live && t.ms === 32000);
  assert.ok(live32, '32s follow-up timer scheduled');
  live32.fn(); await flush();
  assert.equal(env.calls.at(-1).url, '/rail/usage?history=1');
  assert.ok(!env.calls.at(-1).options.headers['X-WS-Usage-Refresh']);

  pop.close();
  assert.equal(env.timers.filter(t => t.live).length, 0);
  const callsAfterClose = env.calls.length;
  live32.fn(); await flush();
  assert.equal(env.calls.length, callsAfterClose);
});

test('agy pending groups reserve both chart slots and are replaced by probe data', async () => {
  const at = Date.UTC(2026, 10, 19, 12, 0), env = environment(at, 'en');
  const pending = [
    { harness: 'agy', group: 'gemini', label: 'Gemini Models', pending: true, windows: [] },
    { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', pending: true, windows: [] }
  ];
  env.set({ entries: pending });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  assert.equal(groups(env.button).length, 0);
  assert.equal(env.button.textContent, 'Plan usage');

  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.equal(nodes(pop.el, '.usage-vendor-bracket')[0].textContent, 'Antigravity· 2 model groups');
  const placeholders = nodes(pop.el, '.usage-pending-card');
  assert.equal(placeholders.length, 4);
  assert.deepEqual(placeholders.map(card => card.attributes['data-kind']), ['five_hour', 'seven_day', 'five_hour', 'seven_day']);
  assert.ok(placeholders.every(card => card.attributes['aria-label'] === 'Checking usage…' && card.attributes['aria-busy'] === 'true'));
  assert.equal(nodes(pop.el, '.spin').length, 4);

  const usageWindow = (kind, percent) => ({ kind, percent, asOf: at, resetsAt: at + 3600000 });
  env.set({ entries: [
    { harness: 'agy', group: 'gemini', label: 'Gemini Models', asOf: at, windows: [usageWindow('five_hour', 30), usageWindow('seven_day', 40)] },
    { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', asOf: at, windows: [usageWindow('five_hour', 50), usageWindow('seven_day', 60)] }
  ] });
  const early = env.timers.find(timer => timer.live && timer.ms === 2000);
  assert.ok(early, '2s cache-only follow-up scheduled');
  early.fn(); await flush();
  assert.equal(nodes(pop.el, '.usage-pending-card').length, 0);
  assert.equal(nodes(pop.el, '.usage-window-card').length, 4);
  assert.deepEqual(nodes(pop.el, '.usage-number').map(node => node.textContent), ['30%', '40%', '50%', '60%']);

  const dashboardCss = fs.readFileSync(__dirname + '/assets/dashboard.css', 'utf8');
  assert.match(dashboardCss, /:root\[data-animation-paused\][^{]*\.spin/);
  assert.match(dashboardCss, /@media \(prefers-reduced-motion: reduce\) \{ \.spin[^}]*animation: none/);
  pop.close();
});

test('popover and gutter follow fixed vendor order with adjacent agy groups regardless of utilization', async () => {
  const at = Date.UTC(2026, 10, 19, 12, 0), reset = at + 3 * 3600000, env = environment(at);
  env.set({
    entries: [
      { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', source: 'via agy CLI', asOf: at, windows: [{ kind: 'five_hour', percent: 96, asOf: at, resetsAt: reset }] },
      { harness: 'codex', source: 'via Codex CLI', asOf: at, windows: [{ kind: 'seven_day', percent: 88, asOf: at, resetsAt: reset }] },
      { harness: 'agy', group: 'gemini', label: 'Gemini Models', source: 'via agy CLI', asOf: at, windows: [{ kind: 'five_hour', percent: 12, asOf: at, resetsAt: reset }] },
      { harness: 'claude', source: 'Claude push', asOf: at, windows: [{ kind: 'five_hour', percent: 5, asOf: at, resetsAt: reset }] }
    ]
  });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const footerTitles = groups(env.button).map(g => g.title);
  assert.deepEqual(footerTitles, ['Claude', 'Codex / GPT', 'Gemini Models', 'Claude and GPT models']);
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  const popTitles = nodes(pop.el, '.usage-account').map(s => s.querySelector('.usage-heading').textContent);
  assert.deepEqual(popTitles, ['Claude', 'Codex / GPT', 'Gemini Models', 'Claude and GPT models']);
  pop.close();
});

test('popover uses window rows, transposed container rules and a localized missing-window slot', async () => {
  const at = Date.UTC(2026, 10, 19, 12, 0), reset = at + 3 * 3600000, env = environment(at);
  const window = (kind, percent) => ({ kind, percent, asOf: at, resetsAt: reset });
  env.set({ entries: [
    { harness: 'claude', plan: 'max', asOf: at, windows: [window('five_hour', 5), window('seven_day', 10)] },
    { harness: 'codex', plan: 'pro', asOf: at, windows: [window('seven_day', 20)] },
    { harness: 'agy', group: 'gemini', label: 'Gemini Models', asOf: at, windows: [window('five_hour', 30), window('seven_day', 40)] },
    { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', asOf: at, windows: [window('five_hour', 50), window('seven_day', 60)] }
  ] });
  const mounted = env.window.WsUsage.mount(env.doc, env.top); await flush();
  const pop = mounted.makePopover(() => {}, () => {}); pop.open(); await flush();
  assert.equal(pop.el.attributes['data-cols'], '4');
  const grid = pop.el.querySelector('.usage-details').querySelector('.usage-layout');
  assert.equal(grid.tagName, 'div');
  assert.deepEqual(nodes(pop.el, '.usage-row-label').map(n => n.textContent), ['5-hour5 h', 'Weekly7 d']);
  assert.deepEqual(nodes(pop.el, '.usage-col-head').map(n => n.textContent), ['5-hour', 'Weekly']);
  assert.equal(nodes(pop.el, '.usage-vendor-bracket')[0].textContent, 'Antigravity· 2 model groups');
  const empty = nodes(pop.el, '.usage-window-empty');
  assert.equal(empty.length, 1);
  assert.equal(empty[0].attributes['data-kind'], 'five_hour');
  assert.equal(empty[0].textContent, 'no 5-hour window');
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  assert.match(css, /@container usage-pop \(max-width: 956px\)/);
  assert.match(css, /@container usage-pop \(max-width: 600px\)/);
  assert.match(css, /@container usage-pop \(max-width: 336px\)/);
  pop.close();
});
