'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const dir = path.resolve(__dirname, '..');
const names = ['tracker.js', 'tracker-panel.js', 'tracker-wiki.js', 'tracker-window.js', 'tracker-forms.js', 'tracker-menu.js', 'tracker-writes.js', 'tracker-render.js', 'tracker-mount.js'];
const renderer = names.map(n => fs.readFileSync(path.join(dir, 'assets', n), 'utf8')).join('\n');
const adapterFile = fs.readFileSync(path.join(dir, 'assets/dashboard-tracker.js'), 'utf8');
const adapter = adapterFile.slice(0, adapterFile.indexOf('  // The drawer hangs from the header'));
const css = fs.readFileSync(path.join(dir, 'assets/tracker.css'), 'utf8');
function fixture(n = 3, eventCount = n) {
  const v = { store: { name: 's', label: '', remote: null }, viewer: { member: 'me', stream: 'me.pc', isMember: true, admin: true },
    now: '2026-11-18T12:00:00Z', sync: { online: true, unpushed: [], wikiWarnings: [] },
    members: { me: { id: 'me', name: 'Me', leads: [], keys: [], revoked: [], profile: { given: 'Me' } } },
    priority: { names: ['Normal'], default: 0 }, attachments: { max_bytes: 1000000 }, labels: [], clones: [], admins: ['me'], rolesProblems: [], heads: {},
    items: {}, assignments: {}, requests: {}, rules: {}, comments: {}, events: [], notices: [], itemOrder: [], html: { items: {}, comments: {} },
    stats: { events: eventCount, commits: 0, flagged: 0, concurrent: 0 },
    my: { badge: [], assignments: [], given: [], team: [], created: [], received: { pending: [], answered: [], sent: [] },
      today: { work: [], pending: [], confirms: [] }, requests: { incoming: [], outgoing: [] }, related: { cc: [], mentioned: [], closed: [] }, ccNotices: [] } };
  for (let i = 0; i < n; i++) {
    const id = 'i' + i, a = 'a' + i;
    v.itemOrder.push(id);
    v.items[id] = { id, alias: id, kind: 'task', title: 'Task ' + i, author: 'me', ts: v.now, lastActivity: v.now,
      status: 'todo', body: 'body ' + i, bodyRev: ['v1'], attachments: [], mentions: [], cc: [], assignments: [a], pending: [], thread: [], children: [], backrefs: [], chain: [id], depth: 0,
      parent: null, labels: [], priority: 0, priorityLevel: 0, priorityName: 'Normal', concurrent: 0, project: '', due: null, rollup: { total: 0, counts: {}, blocked: [] }, requests: [], refs: [] };
    v.assignments[a] = { id: a, item: id, by: 'me', to: 'me', sender: 'me', state: 'live', status: 'todo', ts: v.now, lastUpdate: v.now, completion: 'assignee', chain: [] };
    v.my.assignments.push(a); v.my.today.work.push(a);
  }
  for (let i = 0; i < eventCount; i++) {
    const id = 'e' + i, item = 'i' + (i % n);
    v.events.push({ id, alias: id, type: 'item.create', member: 'me', stream: 'me.pc', ts: v.now, status: 'applied', origin: 'human', markers: [], thread: item, payload: { item, title: v.items[item].title } });
    v.items[item].thread.push(id);
  }
  const globals = {};
  for (const k of ['store', 'viewer', 'now', 'sync', 'members', 'priority', 'attachments', 'labels', 'clones', 'admins', 'rolesProblems', 'heads', 'stats', 'itemOrder', 'my']) globals[k] = v[k];
  const rows = {};
  for (const id of v.itemOrder) {
    const { body, bodyRev, attachments, ...item } = v.items[id];
    rows[id] = { detailVersion: 'v1', seen: { badge: [], read: [] }, data: { item, assignments: { [item.assignments[0]]: v.assignments[item.assignments[0]] }, requests: {}, rules: {}, notices: [], time: { recentlyClosed: false, recentAssignments: {} } } };
  }
  return { v, summary: { cursor: 'epoch.1', reset: true, items: rows, removed: [], globals, feed: [] } };
}
function detail(f, iid, version, body) {
  return { detailVersion: version, data: { body, bodyRev: [version], attachments: [], comments: {}, events: f.v.events.filter(e => e.thread === iid), html: { items: { [iid]: '<p>' + body + '</p>' }, comments: {} } } };
}
const html = '<!doctype html><html><head><style>body{margin:0} header{height:50px} button{font:inherit}</style></head><body><header id="head"><span id="slot"></span></header><main id="main"></main></body></html>';
const translations = 'window.wsT=(k,...a)=>k+ (a.length?" "+a.join(" "):"");window.wsTn=window.wsT;';
async function setup(browser, { legacy = false, details = true } = {}) {
  const page = await browser.newPage({ viewport: { width: 1400, height: 900 } }), errors = [], requests = [], pendingDetails = [];
  const f = fixture(); let current = f.summary;
  page.on('pageerror', e => errors.push(e.message));
  await page.route('http://tracker.test/**', async route => {
    const u = new URL(route.request().url()); requests.push(u.pathname + u.search);
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html });
    const json = data => route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
    if (u.pathname === '/tracker/stores') return json({ rev: 1, stores: [{ id: 's' }] });
    if (u.pathname.endsWith('/summary')) return legacy ? route.fulfill({ status: 404 }) : json(current);
    if (u.pathname.endsWith('/delta')) return u.searchParams.get('since') === current.cursor ? route.fulfill({ status: 304 }) : json(current);
    if (u.pathname.endsWith('/view')) return json({ view: f.v, seen: { badge: [], read: [] } });
    if (u.pathname.endsWith('/check')) return json({});
    if (u.pathname.endsWith('/seen') || u.pathname.endsWith('/sync')) return json({});
    if (u.pathname.endsWith('/detail')) {
      if (!details) { pendingDetails.push({ route, ids: u.searchParams.getAll('id') }); return; }
      return json({ items: Object.fromEntries(u.searchParams.getAll('id').map(id => [id, detail(f, id, current.items[id]?.detailVersion || 'v1', 'body ' + id)])) });
    }
    return route.fulfill({ status: 404 });
  });
  await page.goto('http://tracker.test/');
  await page.addStyleTag({ content: css });
  await page.addScriptTag({ content: translations + renderer });
  await page.addScriptTag({ content: adapter + `
function tkMount(){ if(!tkApi) tkApi=WsTracker.mount(tkAdapter,{header:document.getElementById('head'),qsSlot:document.getElementById('slot'),pageParent:document.getElementById('main')}); }
tkReload();` });
  await page.waitForFunction(() => tkViews.s?.items?.i0 && window.__tracker);
  return { page, f, errors, requests, pendingDetails, setCurrent(a) { current = a; } };
}
async function acceptance(browser) {
  const h = await setup(browser), { page } = h;
  assert.equal(h.requests.filter(p => p.includes('/detail')).length, 0, 'closed handles never request detail');
  assert(!h.requests.some(p => p.endsWith('/view')), 'summary first');
  await page.evaluate(() => tkApi.openStore('s'));
  await page.waitForFunction(() => tkDetailReady('s', 'i0'));
  assert(h.requests.some(p => p.includes('/detail?')), 'visible items load batched detail');
  await page.evaluate(() => {
    window.row0 = document.querySelector('[data-rk="mine:a0"]') || document.querySelector('.irow[data-item="i0"]');
    window.row1 = document.querySelector('.irow[data-item="i1"]');
    window.title1 = row1.querySelector('.title'); row1.focus();
    window.mutations = []; window.observer = new MutationObserver(records => mutations.push(...records)); observer.observe(row1, { subtree: true, childList: true, characterData: true, attributes: true });
  });
  const changed = structuredClone(h.f.summary.items.i0); changed.data.item.title = 'Changed task';
  h.setCurrent({ cursor: 'epoch.2', reset: false, items: { i0: changed }, removed: [], globals: {} });
  await page.evaluate(() => tkReload());
  await page.waitForFunction(() => document.querySelector('.irow[data-item="i0"] .title')?.textContent === 'Changed task');
  assert.deepEqual(await page.evaluate(() => ({ retained: row1 === document.querySelector('.irow[data-item="i1"]'), titleRetained: title1 === row1.querySelector('.title'), focus: document.activeElement === row1, mutations: mutations.length })), { retained: true, titleRetained: true, focus: true, mutations: 0 });
  await page.evaluate(() => tkApi.openItemWindow('s', 'i0'));
  await page.locator('#tkwinbody .iw-thread').waitFor();
  await page.locator('#cbox').fill('unsent draft');
  const changed2 = structuredClone(h.f.summary.items.i2); changed2.data.item.title = 'Other changed';
  h.setCurrent({ cursor: 'epoch.3', reset: false, items: { i2: changed2 }, removed: [], globals: {} });
  await page.evaluate(() => { window.composer = document.getElementById('cbox'); tkReload(); });
  await page.waitForFunction(() => tkProjections.s.cursor === 'epoch.3');
  await page.waitForFunction(() => !window.__tracker.ui.itemChanged);
  assert.equal(await page.locator('#cbox').inputValue(), 'unsent draft');
  assert(await page.evaluate(() => composer === document.getElementById('cbox')), 'unrelated delta retains item composer');
  assert.deepEqual(h.errors, []);
  console.log('PASS tracker browser: summary/visible batches/keyed unchanged row + focus/draft');
  await page.close();

  const old = await setup(browser, { legacy: true });
  await old.page.evaluate(() => tkApi.openItemWindow('s', 'i0'));
  await old.page.locator('#tkwinbody .iw-thread').waitFor();
  assert(old.requests.some(p => p.endsWith('/summary')) && old.requests.some(p => p.endsWith('/view')));
  assert.deepEqual(old.errors, []);
  console.log('PASS tracker browser: new page / old daemon full-view fallback');
  await old.page.close();

  const race = await setup(browser, { details: false });
  await race.page.evaluate(() => tkApi.openItemWindow('s', 'i0'));
  await race.page.waitForFunction(() => document.querySelector('#tkwinbody [role="status"]'));
  await assertEventually(() => race.pendingDetails.length > 0);
  const future = detail(race.f, 'i0', 'v2', 'future body');
  await race.pendingDetails.shift().route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: { i0: future } }) });
  await race.page.waitForFunction(() => tkProjections.s.held.i0);
  assert(!(await race.page.locator('#tkwinbody').textContent()).includes('future body'));
  const nextRow = structuredClone(race.f.summary.items.i0); nextRow.detailVersion = 'v2';
  race.setCurrent({ cursor: 'epoch.2', reset: false, items: { i0: nextRow }, removed: [], globals: {} });
  await race.page.evaluate(() => tkReload());
  await race.page.waitForFunction(() => document.getElementById('tkwinbody').textContent.includes('future body'));
  await race.page.locator('#cbox').fill('draft across detail gap');
  await race.page.evaluate(() => document.getElementById('cbox').setSelectionRange(2, 6));
  const newer = structuredClone(nextRow); newer.detailVersion = 'v3';
  race.setCurrent({ cursor: 'epoch.3', reset: false, items: { i0: newer }, removed: [], globals: {} });
  await race.page.evaluate(() => tkReload());
  await race.page.waitForFunction(() => document.querySelector('#tkwinbody [role="status"]'));
  await assertEventually(() => race.pendingDetails.length > 0);
  await race.pendingDetails.shift().route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: { i0: detail(race.f, 'i0', 'v2', 'late old body') } }) });
  await race.page.waitForFunction(() => tkProjections.s.held.i0?.data.body === 'late old body');
  assert(!(await race.page.locator('#tkwinbody').textContent()).includes('late old body'));
  // A duplicate request is explicitly retried after a stale answer at an equal
  // cursor; no automatic busy loop can mask an ordering defect.
  await race.page.evaluate(() => tkAdapter.retryDetail('s', 'i0'));
  await assertEventually(() => race.pendingDetails.length > 0);
  await race.pendingDetails.shift().route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: { i0: detail(race.f, 'i0', 'v3', 'matching body') } }) });
  await race.page.waitForFunction(() => document.getElementById('tkwinbody').textContent.includes('matching body'));
  assert.equal(await race.page.locator('#cbox').inputValue(), 'draft across detail gap');
  assert.deepEqual(await race.page.evaluate(() => { const c = document.getElementById('cbox'); return [document.activeElement === c, c.selectionStart, c.selectionEnd]; }), [true, 2, 6]);
  assert.deepEqual(race.errors, []);
  console.log('PASS tracker browser: future-before-delta / old-after-delta / matching detail only / draft and caret across loading gap');
  await race.page.close();
}
async function benchmark(browser) {
  const baseline = names.map(n => execFileSync('git', ['show', '6629e21b8:agents-plugin-tool/internal/wsdashboard/assets/' + n], { cwd: dir, encoding: 'utf8' })).join('\n');
  for (const n of [10000, 30000]) {
    const row = { items: n, events: 1000000, surface: 'closed handles', samples: 1 };
    for (const [label, script, modern] of [['before_ms', baseline, false], ['after_ms', renderer, true]]) {
      const page = await browser.newPage();
      await page.route('http://tracker.test/', route => route.fulfill({ contentType: 'text/html', body: html }));
      await page.goto('http://tracker.test/'); await page.addScriptTag({ content: translations + script });
      row[label] = await page.evaluate(({ factory, n, modern }) => {
        const { v, summary } = new Function('return (' + factory + ')')()(n, 1000000);
        let current = v;
        if (modern) {
          current = { ...summary.globals, projection: true, items: {}, assignments: v.assignments, requests: {}, rules: {}, notices: [], comments: {}, events: [], itemFlags: {}, feed: [] };
          for (const id of v.itemOrder) current.items[id] = summary.items[id].data.item;
        }
        const api = WsTracker.mount({ stores: () => ['s'], view: () => current, error: () => null, seen: () => ({ badge: {}, read: {} }), markSeen() {}, onChange() {} }, { header: document.getElementById('head'), qsSlot: document.getElementById('slot'), pageParent: document.getElementById('main') });
        const wire = JSON.stringify(modern ? { cursor: 'epoch.2', reset: false, items: { i0: { ...summary.items.i0, data: { ...summary.items.i0.data, item: { ...summary.items.i0.data.item, title: 'changed' } } } }, globals: {} } : { ...v, items: { ...v.items, i0: { ...v.items.i0, title: 'changed' } } });
        const t = performance.now(), answer = JSON.parse(wire);
        if (modern) current.items.i0 = answer.items.i0.data.item; else current = answer;
        api.refresh();
        return +(performance.now() - t).toFixed(2);
      }, { factory: fixture.toString(), n, modern });
      await page.close();
    }
    console.log('BENCH tracker browser ' + JSON.stringify(row));
  }
}
async function assertEventually(fn) { for (let i = 0; i < 100; i++) { if (fn()) return; await new Promise(r => setTimeout(r, 20)); } throw Error('timed out waiting for request'); }
(async () => {
  const browser = await chromium.launch();
  try { await acceptance(browser); if (process.env.WS_TRACKER_BROWSER_BENCH === '1') await benchmark(browser); }
  finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
