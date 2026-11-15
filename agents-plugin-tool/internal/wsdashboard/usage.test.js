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
  const button = new Element('button'); button.className = 'usage-toggle';
  const gear = new Element('button'); gear.className = 'settings-toggle'; gear.textContent = 'Settings';
  const footer = new Element('div'); footer.className = 'rfoot'; footer.appendChild(gear); footer.appendChild(button);
  const rail = new Element('aside'); rail.appendChild(footer);
  const doc = { hidden: false, events: {}, createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), addEventListener(key, fn) { this.events[key] = fn; } };
  const calls = [], ticks = [];
  let payload = { entries: [], warnPercent: 80, critPercent: 95 }, fail = false;
  const window = { setInterval(fn, ms) { ticks.push({ fn, ms }); }, fetch(url, options) { calls.push({ url, options }); return fail ? Promise.reject(new Error('fake failure')) : Promise.resolve({ ok: true, json: () => Promise.resolve(payload) }); } };
  window.wsI18n = { lang: locale, t: JSON.parse(fs.readFileSync(__dirname + '/../wsi18n/tables/' + locale + '.json', 'utf8')) };
  window.wsT = function (key, ...args) {
    assert.ok(Object.hasOwn(window.wsI18n.t, key), key);
    return window.wsI18n.t[key].replace(/\{(\d+)\}/g, (match, i) => i < args.length ? String(args[i]) : match);
  };
  class Clock extends Date { static now() { return now; } }
  vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window, Date: Clock });
  return { window, rail, doc, button, gear, footer, calls, ticks, now(value) { now = value; }, set(value) { payload = value; }, fail() { fail = true; } };
}
const flush = () => new Promise(resolve => setImmediate(resolve));
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
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
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
    const labels = nodes(weekly, 'text').map(n => n.textContent);
    assert.ok(labels.includes(formatter.format(new Date(reset - 7 * 86400000))), locale);
    assert.ok(labels.includes(formatter.format(new Date(reset))), locale);
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
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
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

test('footer and popover show detected harnesses, age, reset and safe text but hide source', async () => {
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
  for (const s of ['96%', 'Weekly', '5-hour', 'value from 0 min ago', 'Resets ', '<fake-plan>', 'Pi token expired']) assert.ok(text.includes(s), s);
  for (const source of ['via Pi', 'Claude push']) assert.ok(!text.includes(source), source);
  assert.equal(pop.el.attributes.role, 'dialog');
  assert.equal(pop.el.focused, true);
  assert.equal(env.calls[1].url, '/rail/usage?history=1&refresh=1');
  assert.equal(env.calls[0].url, '/rail/usage?history=1');
  assert.equal(env.calls[1].options.headers['X-WS-Usage-Refresh'], '1');
  assert.equal(env.calls[0].options.credentials, 'same-origin');
  env.button.textContent = ''; mounted.sync(); assert.equal(env.button.children.length, 2);
  env.fail(); env.ticks[0].fn(); await flush();
  assert.ok(pop.el.textContent.includes('usage unavailable'));
  assert.ok(pop.el.textContent.includes('96%')); // retained last value
  pop.close();
});

test('fractional percentages round like the server and pace, in footer, title and popover', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at), resetsAt = at + 3600000;
  const cases = [[59.6, '60'], [59.000000001, '59'], [28.999999999999996, '29'], [59.4, '59']];
  const mounted = env.window.WsUsage.mount(env.doc, env.rail);
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

test('history composes into the single coalesced viewer request', async () => {
  const env = environment();
  const mounted = env.window.WsUsage.mount(env.doc, env.rail);
  const pop = mounted.makePopover(() => {}, () => {});
  pop.open(); env.ticks[0].fn(); env.doc.events.visibilitychange();
  assert.equal(env.calls.length, 1); // mount request is still pending
  assert.equal(env.calls[0].url, '/rail/usage?history=1');
  await flush();
  pop.open(); pop.open();
  assert.equal(env.calls.length, 2);
  assert.equal(env.calls[1].url, '/rail/usage?history=1&refresh=1');
  assert.equal(env.calls[1].options.cache, 'no-store');
  await flush();
});

test('footer redraw and sync preserve the sibling settings gear', async () => {
  const at = Date.UTC(2026, 10, 15), env = environment(at);
  const mounted = env.window.WsUsage.mount(env.doc, env.rail); await flush();
  const unchanged = () => {
    assert.deepEqual(env.footer.children, [env.gear, env.button]);
    assert.equal(env.gear.textContent, 'Settings');
    assert.equal(env.gear.className, 'settings-toggle');
  };
  unchanged(); // empty usage still leaves settings reachable
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

test('compact footer keeps percent, period and pace inline with wrapping rather than clipping', () => {
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  const rule = selector => {
    const start = css.indexOf(selector + ' {');
    assert.ok(start >= 0, selector);
    return css.slice(start, css.indexOf('}', start) + 1);
  };
  assert.match(rule('.rail .rfoot'), /flex-wrap: wrap/);
  const toggle = rule('.rail .usage-toggle');
  for (const part of ['flex: 1 1 180px', 'min-width: 0', 'flex-wrap: wrap', 'justify-content: flex-end', 'margin-left: auto']) assert.ok(toggle.includes(part), part);
  for (const selector of ['.usage-harness', '.usage-columns']) assert.match(rule(selector), /flex-wrap: wrap/);
  const column = rule('.usage-column');
  assert.match(column, /display: inline-flex/);
  assert.doesNotMatch(column, /flex-direction: column/);
  assert.match(rule('.usage-number'), /11px\/1\.2/);
  assert.match(rule('.usage-pace'), /10px\/1\.2/);
  assert.match(rule('.usage-key'), /9px\/1\.2/);
  assert.doesNotMatch(css, /overflow:\s*hidden|text-overflow:\s*ellipsis/);
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
