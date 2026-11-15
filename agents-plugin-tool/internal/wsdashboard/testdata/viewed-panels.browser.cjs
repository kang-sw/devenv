// Opt-in production-rendered, offline acceptance. No opaque iframe DOM reads.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, fixture] = process.argv.slice(2);
const capture = process.env.WS_VIEWED_CAPTURE_DIR;
const allowed = new Set([new URL(base).origin, new URL(fixture).origin]);
const evidence = { baseline: '72df121fcadb5e4e027b84c15474e402f9adfc34', current: '48764f937', scope: 'outer panel geometry only', runs: [], checks: [], blocked: [], processes: [] };
const owned = new Map();
let server, browser, closing, activePage;
function save(name, value) { fs.writeFileSync(path.join(capture, name), JSON.stringify(value, null, 2) + '\n'); }
function ps() {
  return execFileSync('ps', ['-axo', 'pid=,ppid=,stat=,command='], { encoding: 'utf8', timeout: 2000 }).trim().split('\n').map(line => {
    const m = line.trim().match(/^(\d+)\s+(\d+)\s+(\S+)\s+(.*)$/);
    return m && { pid: +m[1], ppid: +m[2], stat: m[3], command: m[4] };
  }).filter(Boolean);
}
function track() {
  if (!server) return;
  const rows = ps(), root = server.process().pid, ids = new Set([root]);
  for (let i = 0; i < 12; i++) for (const row of rows) if (ids.has(row.ppid)) ids.add(row.pid);
  for (const row of rows) if (ids.has(row.pid)) owned.set(row.pid, row);
}
async function cleanup() {
  if (closing) return closing;
  closing = (async () => {
    const errors = [];
    try { track(); } catch (e) { errors.push(e.message); }
    try { if (browser) await browser.close(); } catch (e) { errors.push(e.message); }
    try { if (server) await server.close(); } catch (e) { errors.push(e.message); }
    const final = ps().filter(row => owned.has(row.pid));
    save('browser-cleanup.json', { nodePID: process.pid, tracked: [...owned.values()], finalPS: final, errors });
    if (final.length) throw new Error('owned browser descendants remain: ' + final.map(r => r.pid));
  })();
  return closing;
}
for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { cleanup().finally(() => process.exit(124)); });
const cap = setTimeout(() => { console.error('Node 120s cap'); cleanup().finally(() => process.exit(124)); }, 120000);
async function settle(page, ms = 300) { await page.waitForTimeout(ms); }
async function geometry(page) {
  return page.evaluate(() => {
    const panels = [...document.querySelectorAll('.entry-panel, .wb-entry:not(.panel)')].filter(n => n.closest('#r-board, #bpins'));
    const inspect = n => window.__viewedInspect(n);
    return {
      focus: document.activeElement && { tag: document.activeElement.tagName, id: document.activeElement.id },
      badge: [...document.querySelectorAll('.board-viewed-pending')].filter(n => !n.hidden).map(n => n.textContent),
      topics: [...document.querySelectorAll('#r-board > section.topic, #bpins > section.topic')].map(n => n.dataset.topic),
      panels: panels.map(n => { const r = n.getBoundingClientRect(), h = inspect(n), s = h.scroller; return {
        id: n.id, topic: n.closest('[data-topic]').dataset.topic, top: r.top, bottom: r.bottom, left: r.left, height: r.height,
        visible: h.visible ? h.visible(n) : null, viewport: h.viewport ? h.viewport() : null,
        scroller: s && (s.id || s.tagName), scrollTop: s && s.scrollTop, owner: s && !!s.wbScrollOwner,
        nativeAnchor: s && getComputedStyle(s).overflowAnchor,
        frames: [...n.querySelectorAll('.doc-frame')].map(f => ({ key: f.dataset.docKey, height: f.style.height, tag: f.tagName, sandbox: f.getAttribute('sandbox') }))
      }; })
    };
  });
}
async function snap(page, name) {
  track();
  const g = await geometry(page);
  save(name + '.json', g);
  await page.screenshot({ path: path.join(capture, name + '.png') });
  return g;
}
function frozen(before, after) {
  return before.panels.filter(p => p.visible).map(p => {
    const n = after.panels.find(n => n.id === p.id);
    return { id: p.id, topic: p.topic, before: p.top, after: n && n.top, delta: n && n.top - p.top, sameHeight: !!n && Math.abs(n.height - p.height) <= 1, ok: !!n && Math.abs(n.top - p.top) <= 1 && Math.abs(n.height - p.height) <= 1 };
  });
}
function check(name, ok, details) {
  evidence.checks.push({ name, ok: !!ok, details });
  save('results.json', evidence);
  assert(ok, name + ': ' + JSON.stringify(details));
}
async function position(page, id, offset = 80) {
  await page.evaluate(([id, offset]) => {
    const n = document.getElementById(id), h = window.__viewedInspect(n), s = h.scroller;
    const top = h.viewport ? h.viewport().top : s.getBoundingClientRect().top;
    s.scrollTop += n.getBoundingClientRect().top - top - offset;
  }, [id, offset]);
  await settle(page);
}
async function emit(page, variant) {
  const { html } = await (await fetch(fixture + '/state?v=' + variant)).json();
  await page.evaluate(({ html, variant }) => {
    const ws = window.__viewedSockets.find(s => s.url.includes('/s/livesession/events'));
    if (!ws || !ws.onmessage) throw new Error('fixture has no production session stream');
    ws.onmessage({ data: JSON.stringify({ event: 'region', data: { name: 'board', hash: 'fixture-' + variant, html } }) });
  }, { html, variant });
  await settle(page);
}
async function mount(mode, width, assets) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', locale: 'en-US', serviceWorkers: 'block' });
  context.setDefaultTimeout(8000); context.setDefaultNavigationTimeout(8000);
  let routeError;
  await context.route('**/*', async route => {
    try {
    const url = new URL(route.request().url());
    if (!allowed.has(url.origin)) { evidence.blocked.push(url.href); return route.abort(); }
    if (mode === 'baseline' && route.request().resourceType() === 'document' && url.origin === base && url.pathname === '/s/livesession') {
      const response = await route.fetch();
      let html = await response.text();
      const replacements = [];
      for (const asset of assets) {
        const count = html.split(asset.current).length - 1;
        if (count !== 1) {
          const at = html.indexOf(asset.current.slice(0, 100));
          let same = 0;
          if (at >= 0) while (same < asset.current.length && html[at + same] === asset.current[same]) same++;
          save('asset-mismatch.json', { path: asset.path, at, same, expectedLength: asset.current.length, expected: asset.current.slice(Math.max(0, same - 80), same + 120), actual: at < 0 ? null : html.slice(at + Math.max(0, same - 80), at + same + 120) });
          throw new Error('exact inline asset occurrence: ' + asset.path + ' = ' + count);
        }
        html = html.replace(asset.current, () => asset.baseline);
        replacements.push({ path: asset.path, count });
      }
      evidence.runs.push({ mode, width, replacements });
      return route.fulfill({ response, body: html });
    }
    return route.continue();
    } catch (e) { routeError = e; await route.abort(); }
  });
  await context.addInitScript(({ base, mode }) => {
    window.__viewedInspect = n => {
      if (mode === 'current') return window.wbHistory.keep(n, { capture: false });
      // Read-only baseline measurement: old keep ignores capture:false and
      // would itself start a long-lived preservation session.
      let s = n.parentElement;
      while (s && s !== document.body && !(/auto|scroll|overlay/.test(getComputedStyle(s).overflowY) && s.scrollHeight > s.clientHeight)) s = s.parentElement;
      if (!s || s === document.body) s = document.scrollingElement;
      const viewport = () => {
        const r = s.getBoundingClientRect(); let top = Math.max(0, r.top + s.clientTop), bottom = Math.min(innerHeight, r.top + s.clientTop + s.clientHeight);
        for (const h of s.querySelectorAll('.tophead, .bc-hd, .wb-rail-wrap.up .wb-rail-stack')) {
          const b = h.getBoundingClientRect(), c = getComputedStyle(h);
          if ((c.position === 'sticky' || h.matches('.wb-rail-stack')) && b.height && b.top <= top + (parseFloat(c.top) || 0) + 1 && b.bottom > top) top = Math.max(top, b.bottom);
        }
        return { top, bottom, left: Math.max(0, r.left), right: Math.min(innerWidth, r.right) };
      };
      return { scroller: s, viewport, visible: node => { const r = node.getBoundingClientRect(), v = viewport(); return Math.min(r.bottom, v.bottom) - Math.max(r.top, v.top) >= 1 && Math.min(r.right, v.right) - Math.max(r.left, v.left) >= 1; } };
    };
    // Faithful openStream transport: only local session/rail event channels,
    // no network socket and no provider. Production onRegion/swap is unmodified.
    window.__viewedSockets = [];
    window.WebSocket = class {
      constructor(url) {
        const u = new URL(url);
        if (u.host !== new URL(base).host || !/\/events$/.test(u.pathname)) throw new Error('non-fixture websocket ' + url);
        this.url = url; window.__viewedSockets.push(this);
        setTimeout(() => { if (this.onopen) this.onopen(); }, 0);
      }
      send() {} close() {}
    };
  }, { base, mode });
  const page = await context.newPage(); activePage = page;
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  try { await page.goto(base + '/s/livesession?frame=1'); } catch (e) { throw routeError || e; }
  await page.waitForFunction(() => window.wbHistory && window.WsCards && window.__viewedSockets.length);
  if (!(await page.locator('#p2').isVisible())) await page.locator('[data-tab="board"]').first().click();
  // Production placement API, same effect as the pin buttons without scroll.
  await page.evaluate(() => {
    for (const name of ['PinAbove', 'P', 'Q', 'PinBelow']) window.WsCards.move(document.querySelector('section[data-topic="' + name + '"]'), { pin: true, index: 99 });
  });
  // Mount relevant frame bands, then let actual frame reports settle.
  await page.locator('#p2').scrollIntoViewIfNeeded();
  await settle(page, 500);
  await position(page, 'p2', 80);
  if (width === 1800) { await page.locator('#p6').scrollIntoViewIfNeeded(); await settle(page, 300); await position(page, 'p6', 30); }
  await settle(page, 400);
  return { context, page, errors };
}
async function frameHeight(page, id, increment, wrongKey = false) {
  const el = page.locator('#' + id + ' iframe.doc-frame').first();
  const handle = await el.elementHandle();
  assert(handle, 'fixture requires a mounted sandbox for ' + id);
  const data = await handle.evaluate((f, increment) => ({ docKey: f.dataset.docKey, height: parseFloat(f.style.height) + increment }), increment);
  const child = await handle.contentFrame();
  // Execute a bridge report from the actual opaque frame. No child DOM reads.
  await child.evaluate(({ docKey, height, wrongKey }) => parent.postMessage({ wbDoc: 'height', docKey: wrongKey ? 'forged' : docKey, height }, '*'), { ...data, wrongKey });
  await settle(page);
  return data;
}
(async () => {
  try {
    const assets = await (await fetch(fixture + '/assets')).json();
    server = await chromium.launchServer({ headless: true });
    track();
    browser = await chromium.connect(server.wsEndpoint());
    const monitor = setInterval(() => { try { track(); } catch (_) {} }, 500);
    monitor.unref();
    // Capture the identical A/B update at both widths before judging current.
    const current = [];
    for (const mode of ['baseline', 'current']) for (const width of [1800, 420]) {
      const run = await mount(mode, width, assets), { page } = run;
      const before = await snap(page, mode + '-' + width + '-before');
      const p3 = await page.locator('#p3').evaluate(n => n.outerHTML);
      await emit(page, 'queue1'); await emit(page, 'queue2');
      const after = await snap(page, mode + '-' + width + '-queued');
      const heldBytes = p3 === await page.locator('#p3').evaluate(n => n.outerHTML);
      const result = { mode, width, frozen: frozen(before, after), heldBytes, badge: after.badge, errors: run.errors };
      evidence.runs.push(result); save('results.json', evidence);
      if (mode === 'current') current.push({ ...run, before, after, result }); else await run.context.close();
    }
    for (const run of current) {
      const { page, before, after, result } = run, width = result.width;
      check(width + ' visible A and B precondition', before.panels.some(p => p.id === 'p2' && p.visible) && before.panels.some(p => p.id === 'p3' && p.visible), before.panels);
      check(width + ' EVERY exposed panel frozen over queued reorder', result.frozen.length >= 2 && result.frozen.every(p => p.ok), result.frozen);
      check(width + ' visible panel bytes held', result.heldBytes, result);
      check(width + ' shared Korean bar counts two B revisions', after.badge.length === 1 && /^최신 변경 2건 반영 · /.test(after.badge[0]) && after.badge[0].includes('B'), after.badge);
      check(width + ' permanent owner disables native anchoring', after.panels.filter(p => p.visible).every(p => p.owner && p.nativeAnchor === 'none'), after.panels);
      await page.locator('.board-viewed-pending').click(); await settle(page);
      const clicked = await snap(page, 'current-' + width + '-explicit');
      check(width + ' explicit click applies latest bytes', (await page.locator('#p3').evaluate(n => n.outerHTML)).includes('queue2') && !clicked.badge.length, clicked.badge);
      await run.context.close();
    }
    // Focused fresh fixtures isolate render release, heights and timing from
    // explicit-application exceptions. Baseline limitations do not waive them.
    for (const width of [1800, 420]) {
      const { context, page, errors } = await mount('current', width, assets);
      await position(page, 'p1', 30);
      let g = await geometry(page);
      const v = g.panels.find(p => p.id === 'p1').viewport;
      // Position p1's bottom at exactly one exposed pixel below clearance.
      await page.evaluate(v => { const n = document.getElementById('p1'), s = window.wbHistory.keep(n, { capture: false }).scroller; s.scrollTop += n.getBoundingClientRect().bottom - v.top - 1; }, v);
      await settle(page);
      const one = await snap(page, 'current-' + width + '-one-pixel');
      const p1 = one.panels.find(p => p.id === 'p1');
      check(width + ' exact one-pixel exposure precondition', Math.abs(p1.bottom - p1.viewport.top - 1) <= 0.5 && p1.visible, p1);
      const old = await page.locator('#p1').evaluate(n => n.outerHTML);
      await page.locator('#ctext').focus();
      await emit(page, 'changeA');
      check(width + ' one pixel defers bytes and preserves focus', old === await page.locator('#p1').evaluate(n => n.outerHTML) && await page.locator('#ctext').evaluate(n => document.activeElement === n), await geometry(page));
      // Changed p1 fully leaves; unchanged p2 is still exposed in topic A.
      await position(page, 'p2', -2);
      const released = await snap(page, 'current-' + width + '-offscreen-release');
      check(width + ' changed panel releases with unchanged sibling visible', (await page.locator('#p1').evaluate(n => n.outerHTML)).includes('External rewrite') && released.panels.find(p => p.id === 'p2').visible, released);
      // A removed reading reference must not select a replacement target.
      await settle(page);
      const disappearance = await page.evaluate(() => {
        const n = document.getElementById('p2'), h = window.wbHistory.keep(n), s = h.scroller, top = s.scrollTop;
        n.remove(); h.apply(); return { before: top, after: s.scrollTop };
      });
      check(width + ' disappeared reference skips substitute correction', Math.abs(disappearance.before - disappearance.after) <= 1, disappearance);
      check(width + ' no browser script errors', !errors.length, errors);
      await context.close();
    }
    for (const width of [1800, 420]) {
      const { context, page } = await mount('current', width, assets);
      // Keep frame reports real (origin null, registered WindowProxy), while
      // changing only host heights; sandbox internals are never inspected.
      for (const id of ['p4', 'p5']) { await page.locator('#' + id).scrollIntoViewIfNeeded(); await settle(page); }
      await position(page, 'p2', 180);
      if (width === 1800) {
        for (const id of ['p8', 'p9']) { await page.locator('#' + id).scrollIntoViewIfNeeded(); await settle(page); }
        await position(page, 'p6', 30);
      }
      const before = await snap(page, 'current-' + width + '-height-before');
      const ids = width === 1800 ? ['p4', 'p5', 'p8', 'p9'] : ['p4', 'p5'];
      check(width + ' above/below growth units fully offscreen', ids.every(id => !before.panels.find(p => p.id === id).visible), before.panels.filter(p => ids.includes(p.id)));
      for (const id of ids) await frameHeight(page, id, 57);
      const after = await snap(page, 'current-' + width + '-height-after');
      check(width + ' offscreen iframe growth preserves EVERY visible panel', frozen(before, after).every(p => p.ok), frozen(before, after));
      const visible = after.panels.find(p => p.visible && p.frames.some(f => f.tag === 'IFRAME'));
      const heightBefore = await page.locator('#' + visible.id + ' iframe.doc-frame').first().evaluate(f => f.style.height);
      await frameHeight(page, visible.id, 37, true);
      check(width + ' forged docKey height rejected', heightBefore === await page.locator('#' + visible.id + ' iframe.doc-frame').first().evaluate(f => f.style.height), visible.id);
      await page.evaluate(() => window.postMessage({ wbDoc: 'height', docKey: 'panel:p2', height: 9999 }, '*'));
      await settle(page);
      await frameHeight(page, visible.id, 37);
      check(width + ' visible iframe height joins shared pending bar', heightBefore === await page.locator('#' + visible.id + ' iframe.doc-frame').first().evaluate(f => f.style.height) && (await geometry(page)).badge.length === 1, await geometry(page));
      await page.locator('.board-viewed-pending').click(); await settle(page);
      check(width + ' explicit height release', heightBefore !== await page.locator('#' + visible.id + ' iframe.doc-frame').first().evaluate(f => f.style.height), visible.id);
      await context.close();
    }
    {
      const { context, page } = await mount('current', 1800, assets);
      const more = page.locator('[data-closed-next]').last();
      await more.scrollIntoViewIfNeeded(); await more.click();
      await page.locator('.closed-page section[data-topic="Past"]').waitFor();
      // Closed topics arrive folded; use the real disclosure control.
      const past = page.locator('.closed-page section[data-topic="Past"]');
      if (!(await past.locator('#p10').isVisible())) await past.locator('.t-fold').click();
      await past.locator('#p10').scrollIntoViewIfNeeded(); await settle(page, 400);
      await position(page, 'p10', 50);
      const history = await page.evaluate(() => window.wbHistory.capture(document.getElementById('r-board')));
      const before = await snap(page, 'current-1800-fetched-before');
      await emit(page, 'transfer');
      const held = await snap(page, 'current-1800-fetched-held');
      check('visible fetched closed-page transfer staged in original page', await page.locator('.closed-page #p10').count() === 1 && frozen(before, held).every(p => p.ok) && held.badge.some(s => s.includes('Past')), held);
      check('history membership survives held transfer', JSON.stringify(history) === JSON.stringify(await page.evaluate(() => window.wbHistory.capture(document.getElementById('r-board')))), history);
      await position(page, 'p2', 80);
      await settle(page, 400);
      check('fetched transfer releases after card fully leaves', await page.locator('#r-board > section[data-topic="Past"]').count() === 1 && await page.locator('.closed-page #p10').count() === 0, await geometry(page));
      await context.close();
    }
    {
      const { context, page } = await mount('current', 1800, assets);
      const sent = [];
      await context.route(base + '/s/livesession/message', async route => { sent.push(route.request().postDataJSON()); await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' }); });
      const memo = page.locator('section[data-topic="A"] textarea.tnote');
      await page.locator('section[data-topic="A"] .tnote-toggle').click();
      await memo.fill('Owner local memo');
      check('owner memo edits apply immediately', await memo.inputValue() === 'Owner local memo', null);
      await page.locator('#csend').click(); await settle(page);
      check('fixture local send carries only A owner input', sent.length === 1 && sent[0].topics.some(t => t.topic === 'A'), sent);
      await position(page, 'p1', 30);
      const bytes = await page.locator('#p1').evaluate(n => n.outerHTML);
      await emit(page, 'mixed');
      check('same-topic receipt admitted without lending exemption to external rewrite', await page.locator('.wb-entry.owner').count() > 0 && bytes === await page.locator('#p1').evaluate(n => n.outerHTML) && (await geometry(page)).badge.some(s => s.includes('A')), await geometry(page));
      await context.close();
    }
    // Representative real owner transaction, input cancellation/cooldown and
    // reduced-motion late jump correction; virtual tests cover full timing.
    const run = await mount('current', 1800, assets), { page } = run;
    const core = await page.evaluate(async () => {
      const n = document.getElementById('p2'), w = window.wbHistory, s = w.keep(n, { capture: false }).scroller;
      const before = n.getBoundingClientRect().top, outer = w.keep(n), inner = w.keep(n);
      document.getElementById('p1').style.marginBottom = '70px'; inner.apply();
      const nested = n.getBoundingClientRect().top; outer.apply(); const after = n.getBoundingClientRect().top;
      const stopped = w.keep(n); s.dispatchEvent(new WheelEvent('wheel', { deltaY: 20 }));
      document.getElementById('p1').style.marginBottom = '100px'; stopped.apply();
      const canceled = n.getBoundingClientRect().top;
      const during = w.keep(n); document.getElementById('p1').style.marginBottom = '130px'; during.apply();
      const cooldown = n.getBoundingClientRect().top;
      await new Promise(r => setTimeout(r, 230));
      const resumed = w.keep(n); document.getElementById('p1').style.marginBottom = '160px'; resumed.apply();
      return { before, nested, after, canceled, cooldown, resumed: n.getBoundingClientRect().top };
    });
    check('nested transaction applies only outermost and input wins through cooldown', Math.abs(core.nested - core.before - 70) <= 1 && Math.abs(core.after - core.before) <= 1 && Math.abs(core.canceled - core.after - 30) <= 1 && Math.abs(core.cooldown - core.canceled - 30) <= 1 && Math.abs(core.resumed - core.cooldown) <= 1, core);
    const jump = await page.evaluate(async () => {
      const n = document.getElementById('p3'), w = window.wbHistory, s = w.keep(n, { capture: false }).scroller;
      let ended; w.jump(n, { block: 'start', behavior: 'smooth', onEnd: reason => { ended = reason; } });
      const initial = n.getBoundingClientRect().top;
      await new Promise(r => setTimeout(r, 45));
      const inspection = w.keep(n, { capture: false });
      document.getElementById('p1').style.marginBottom = '210px';
      await new Promise(r => setTimeout(r, 70));
      const corrected = n.getBoundingClientRect().top;
      s.dispatchEvent(new WheelEvent('wheel', { deltaY: 1 }));
      return { initial, corrected, ended, inspectDuringJump: typeof inspection.visible === 'function', reducedMotion: matchMedia('(prefers-reduced-motion: reduce)').matches };
    });
    check('reduced-motion jump corrects late layout and wheel cancels', Math.abs(jump.initial - jump.corrected) <= 1 && jump.ended === 'user' && jump.inspectDuringJump && jump.reducedMotion, jump);
    await run.context.close();
    evidence.completed = true;
    console.log('PASS viewed browser acceptance; see results.json');
  } catch (e) {
    evidence.failure = { message: e.message, stack: e.stack };
    if (activePage && !activePage.isClosed()) {
      try { await snap(activePage, 'failure'); } catch (captureError) { evidence.failure.captureError = captureError.message; }
    }
    console.error(e.stack); process.exitCode = 1;
  } finally {
    clearTimeout(cap); save('results.json', evidence);
    await cleanup();
  }
})().catch(e => { console.error(e.stack); process.exitCode = 1; });
