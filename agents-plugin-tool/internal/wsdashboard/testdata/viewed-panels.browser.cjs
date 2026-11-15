// ONE browser driver for two independently embedded, frozen fixture binaries.
// Static-authored: executable verification belongs only to the approved batch.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const [baselineFile, resultFile, capture] = process.argv.slice(2);
const PIN = '5585844a794a031a770ce4171c5e05daf1bd3353';
const MODULE = '/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright';
assert.equal(process.env.WS_PLAYWRIGHT_MODULE, MODULE, 'only approved cached module');
const { chromium } = require(MODULE);
const pair = [baselineFile, resultFile].map(file => JSON.parse(fs.readFileSync(file, 'utf8')));
assert.equal(pair[0].source_pin, PIN, 'strict baseline 558');
assert.equal(pair[1].source_pin, JSON.parse(fs.readFileSync(path.join(capture, 'manifest.json'), 'utf8')).result, 'frozen result identity');
assert.notEqual(pair[0].cache_dir, pair[1].cache_dir, 'separate fixture cache roots');
assert.notEqual(pair[0].project_dir, pair[1].project_dir, 'separate fixture projects');
const allowed = new Set(pair.flatMap(p => [p.dashboard_origin, p.fixture_origin]));
assert.equal(allowed.size, 4, 'four distinct exact Port 0 origins');
for (const origin of allowed) {
  const u = new URL(origin);
  assert(u.hostname === '127.0.0.1' && u.port && u.port !== '29417' && u.origin === origin, 'exact disposable origin');
}
const evidence = { baseline: pair[0], result: pair[1], scope: 'outer panel geometry; no opaque child DOM reads', checks: [], observations: [], documents: [], requests: [], serviceWorkers: [], completed: false };
let server, browser, active, closing, monitor, trackingError, sequence = 0;
const owned = new Map(), frameIDs = new WeakMap();
function save(name, data) {
  const dest = path.join(capture, name), tmp = dest + '.tmp';
  fs.writeFileSync(tmp, JSON.stringify(data, null, 2) + '\n'); fs.renameSync(tmp, dest);
}
function persist() { save('results.json', evidence); save('document-ledger.json', evidence.documents); }
function captureServiceWorkers(run) {
  run.serviceWorkers.remaining = run.context.serviceWorkers().map(worker => worker.url());
  persist();
}
function frameIdentity(frame) {
  if (!frame) return null;
  if (!frameIDs.has(frame)) frameIDs.set(frame, ++sequence);
  return { id: frameIDs.get(frame), url: frame.url(), parent: frame.parentFrame() ? frameIdentity(frame.parentFrame()).id : null };
}
function processes() {
  return execFileSync('ps', ['-axo', 'pid=,ppid=,lstart=,command='], { encoding: 'utf8', timeout: 2000 }).trim().split('\n').map(line => {
    const m = line.trim().match(/^(\d+)\s+(\d+)\s+(\S+\s+\S+\s+\d+\s+\S+\s+\d+)\s+(.*)$/);
    return m && { pid: +m[1], ppid: +m[2], started: m[3].replace(/\s+/g, ' '), command: m[4].slice(0, 800) };
  }).filter(Boolean);
}
function track() {
  if (!server) return;
  const rows = processes();
  const root = rows.find(r => r.pid === server.process().pid);
  if (root && server.process().exitCode === null) owned.set(root.pid + ':' + root.started, root);
  const ids = new Set(rows.filter(r => owned.has(r.pid + ':' + r.started)).map(r => r.pid));
  for (let i = 0; i < 16; i++) for (const row of rows) if (ids.has(row.ppid)) ids.add(row.pid);
  for (const row of rows) if (ids.has(row.pid)) owned.set(row.pid + ':' + row.started, row);
  save('browser-owned-live.json', { nodePID: process.pid, tracked: [...owned.values()] });
}
async function bounded(promise, ms, message) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(message)), ms); })]); }
  finally { clearTimeout(timer); }
}
async function cleanup() {
  if (closing) return closing;
  closing = (async () => {
    const errors = [];
    clearInterval(monitor);
    try { if (active) captureServiceWorkers(active); } catch (e) { errors.push(e.stack); }
    try { track(); } catch (e) { errors.push(e.stack); }
    try { if (browser) await bounded(browser.close(), 4000, 'browser close cap'); } catch (e) { errors.push(e.stack); }
    try { if (server) await bounded(server.close(), 4000, 'browser server close cap'); } catch (e) { errors.push(e.stack); }
    const final = processes().filter(r => owned.has(r.pid + ':' + r.started));
    save('browser-cleanup.json', { tracked: [...owned.values()], final_ps: final, errors });
    if (final.length || errors.length) throw new Error('browser cleanup incomplete; outer owns bounded TERM/KILL');
  })();
  return closing;
}
for (const sig of ['SIGINT', 'SIGTERM']) process.once(sig, () => {
  evidence.failure = { message: sig }; persist();
  cleanup().finally(() => process.exit(124));
});
// Leave room for diagnostic capture/closure inside the outer 180-second cap.
const cap = setTimeout(() => {
  evidence.failure = { message: 'Node 160-second cap' }; persist();
  cleanup().finally(() => process.exit(124));
}, 160000);
function check(name, condition, detail) {
  evidence.checks.push({ name, ok: !!condition, detail }); persist();
  assert(condition, name + ': ' + JSON.stringify(detail));
}
function observe(name, detail) { evidence.observations.push({ name, detail }); persist(); }
async function guard(run, promise) {
  if (trackingError) throw trackingError;
  try { const value = await Promise.race([promise, run.fatal]); if (run.error) throw run.error; return value; }
  catch (e) { throw run.error || e; }
}
async function quiet(run, ms = 240) { await guard(run, run.frame.waitForTimeout(ms)); }
async function geometry(run) {
  return guard(run, run.frame.evaluate(() => {
    const panels = [...document.querySelectorAll('#r-board .entry-panel, #bpins .entry-panel, #r-board .wb-entry:not(.panel), #bpins .wb-entry:not(.panel)')];
    return {
      url: location.href, assetID: document.documentElement.dataset.assets,
      focus: { tag: document.activeElement.tagName, id: document.activeElement.id },
      badge: [...document.querySelectorAll('.wb-oq-top .oq-updates')].filter(n => !n.hidden).map(n => n.textContent),
      topics: [...document.querySelectorAll('#r-board > section.topic, #bpins > section.topic')].map(n => n.dataset.topic),
      panels: [...new Set(panels)].map(n => {
        const h = window.__viewedGeometry(n), r = n.getBoundingClientRect(), s = h.scroller;
        return { id: n.id, topic: n.closest('[data-topic]').dataset.topic, open: n.open, top: r.top, bottom: r.bottom, height: r.height,
          visible: h.visible, exposedPixels: h.exposedPixels, viewport: h.viewport,
          scroller: s.id || s.tagName, scrollTop: s.scrollTop, owner: !!s.wbScrollOwner,
          nativeAnchor: getComputedStyle(s).overflowAnchor,
          frames: [...n.querySelectorAll('.doc-frame:not(.doc-frame-next)')].map(f => ({ key: f.dataset.docKey, height: f.style.height, tag: f.tagName, sandbox: f.getAttribute('sandbox') })) };
      })
    };
  }));
}
async function snap(run, name) {
  const g = await geometry(run); save(name + '.json', g);
  await guard(run, run.frame.page().screenshot({ path: path.join(capture, name + '.png'), timeout: 8000 }));
  track(); return g;
}
function preserved(before, after, exclude = []) {
  return before.panels.filter(p => p.visible && !exclude.includes(p.id)).map(p => {
    const n = after.panels.find(q => q.id === p.id);
    return { id: p.id, topic: p.topic, before: p.top, after: n && n.top,
      delta: n && n.top - p.top, ok: !!n && Math.abs(n.top - p.top) <= 1 && Math.abs(n.height - p.height) <= 1 };
  });
}
function allPreserved(name, before, after, exclude = []) {
  const list = preserved(before, after, exclude);
  check(name, list.length > 0 && list.every(p => p.ok), list);
}
async function bytes(run, id) {
  // Canonical frame payload/disclosures, not mutable mounted iframe markup.
  return guard(run, run.frame.locator('#' + id).evaluate(n => ({
    recap: n.querySelector('summary') && n.querySelector('summary').textContent,
    payloads: [...n.querySelectorAll('.doc-frame:not(.doc-frame-next)')].map(f => f.getAttribute('data-srcdoc'))
  })).then(v => JSON.stringify(v)));
}
async function position(run, id, offset) {
  // Finite geometry convergence accounts for sticky heads entering/leaving.
  for (let i = 0; i < 4; i++) {
    await guard(run, run.frame.evaluate(({ id, offset }) => {
      const n = document.getElementById(id), g = window.__viewedGeometry(n);
      g.scroller.scrollTop += n.getBoundingClientRect().top - g.viewport.top - offset;
    }, { id, offset }));
    await quiet(run, 45);
  }
  await quiet(run);
  const g = await geometry(run), p = g.panels.find(p => p.id === id);
  check(run.name + ' positioned ' + id, !!p && Math.abs(p.top - p.viewport.top - offset) <= 1, p);
  return g;
}
async function positionHistory(run, id) {
  // Header/chunk layout comes from this binary. Never assume a fixed 100px
  // gap: the shorter actual history-clip-to-topic distance is authoritative.
  const offset = await guard(run, run.frame.locator('#' + id).evaluate(n => {
    const topic = n.closest('.topic'), clip = n.closest('.wb-history').querySelector('.history-clip');
    return Math.min(100, clip.getBoundingClientRect().top - topic.getBoundingClientRect().top);
  }));
  check(run.name + ' measured history offset', Number.isFinite(offset) && offset >= 0, offset);
  return position(run, id, offset);
}
async function cancelJump(run, id) {
  await guard(run, run.frame.evaluate(id => {
    const n = document.getElementById(id), s = window.__viewedGeometry(n).scroller;
    s.dispatchEvent(new WheelEvent('wheel', { deltaY: 1, bubbles: true }));
  }, id));
  // Baseline navigation still uses the legacy quiet window; the result
  // cancels immediately. Wait for each real lifecycle, not a guessed delay.
  await guard(run, run.frame.waitForFunction(id => !window.wbHistory.keeping(window.__viewedGeometry(document.getElementById(id)).scroller), id, { timeout: 1500 }));
  await quiet(run, 260);
  check(run.name + ' admission jump settled/canceled', await guard(run, run.frame.evaluate(id => !window.wbHistory.keeping(window.__viewedGeometry(document.getElementById(id)).scroller), id)), id);
}
async function admit(run, id) {
  const found = await guard(run, run.frame.evaluate(id => {
    const n = window.wbHistory.loadTarget(document.getElementById('r-board'), id, window.WsCards.unfoldTopic);
    return !!n;
  }, id));
  check(run.name + ' production history admits ' + id, found, null);
  await quiet(run, 300);
  await cancelJump(run, id);
}
async function emit(run, variant) {
  const payload = await guard(run, run.frame.evaluate(async ({ endpoint, variant }) => {
    const response = await fetch(endpoint + '/state?v=' + encodeURIComponent(variant));
    if (!response.ok) throw new Error('fixture state HTTP ' + response.status);
    return response.json();
  }, { endpoint: run.info.fixture_origin, variant }));
  check(run.name + ' snapshot source identity', payload.source_pin === run.info.source_pin, payload.source_pin);
  await guard(run, run.frame.evaluate(({ html, variant }) => {
    const ws = window.__viewedSockets.find(s => s.url.includes('/s/livesession/events'));
    if (!ws || !ws.onmessage) throw new Error('no session-frame production stream');
    ws.onmessage({ data: JSON.stringify({ event: 'region', data: { name: 'board', hash: 'fixture-' + variant, html } }) });
  }, { html: payload.html, variant }));
  await quiet(run, 240);
}
async function mount(info, mode, width, label) {
  if (trackingError) throw trackingError;
  const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', locale: 'en-US', serviceWorkers: 'allow' });
  context.setDefaultTimeout(8000); context.setDefaultNavigationTimeout(8000);
  let reject;
  const run = { info, mode, width, name: mode + '-' + width + '-' + label, context, error: null,
    fatal: new Promise((_, fail) => { reject = fail; }), responseJobs: new Set(), sent: [] };
  run.fatal.catch(() => {});
  const fail = e => { if (!run.error) { run.error = e; reject(e); } };
  run.fail = fail; active = run;
  run.serviceWorkers = { run: run.name, attempts: [], events: [], remaining: [] };
  evidence.serviceWorkers.push(run.serviceWorkers); persist();
  await context.exposeBinding('__wsViewedServiceWorkerAttempt', ({ frame }, scriptURL) => {
    run.serviceWorkers.attempts.push({ frame: frameIdentity(frame), scriptURL });
    persist();
  });
  context.on('serviceworker', worker => { run.serviceWorkers.events.push(worker.url()); persist(); });
  // Preserve opaque frames' native restriction without Playwright's Navigator read.
  await context.addInitScript(() => {
    if (self.origin === 'null') return;
    const container = navigator.serviceWorker;
    if (!container) return;
    Object.getPrototypeOf(container).register = async function(scriptURL) {
      await globalThis.__wsViewedServiceWorkerAttempt(typeof scriptURL === 'string' ? scriptURL : { type: typeof scriptURL });
      throw new Error('Service worker registration is forbidden in the viewed fixture');
    };
  });
  await context.route('**/*', async route => {
    const request = route.request();
    try {
      const url = new URL(request.url());
      if (!allowed.has(url.origin)) throw new Error('blocked non-fixture HTTP ' + request.url());
      // Fixture response only: exercise real local send bookkeeping without a
      // provider, holder, or actual owner inbox. Receipt is a rendered snapshot.
      if (request.method() === 'POST' && url.origin === info.dashboard_origin && url.pathname === '/s/livesession/message') {
        run.sent.push(request.postDataJSON());
        await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
      } else await route.continue();
    } catch (e) {
      evidence.routeFailure = { message: e.message, url: request.url(), resourceType: request.resourceType(), frame: frameIdentity(request.frame()) };
      fail(e); persist();
      try { await route.abort(); } catch (_) {}
    }
  });
  await context.addInitScript(({ origin }) => {
    // Independent, read-only geometry for BOTH binaries. Never invoke old
    // keep(capture:false), which can have preservation side effects.
    window.__viewedGeometry = n => {
      let s = n.parentElement;
      while (s && s !== document.body && s !== document.documentElement && !(/auto|scroll|overlay/.test(getComputedStyle(s).overflowY) && s.scrollHeight > s.clientHeight)) s = s.parentElement;
      if (!s || s === document.body || s === document.documentElement) s = document.scrollingElement;
      const main = s === document.scrollingElement, r = main ? { top: 0, bottom: innerHeight, left: 0, right: innerWidth } : s.getBoundingClientRect();
      let top = Math.max(0, r.top + (main ? 0 : s.clientTop)), bottom = Math.min(innerHeight, main ? r.bottom : r.top + s.clientTop + s.clientHeight);
      top += parseFloat(getComputedStyle(s).scrollPaddingTop) || 0;
      for (const head of (main ? document : s).querySelectorAll('.tophead, .bc-hd, .wb-rail-wrap.up .wb-rail-stack')) {
        const hs = getComputedStyle(head), h = head.getBoundingClientRect();
        if ((hs.position === 'sticky' || head.matches('.wb-rail-stack')) && h.height && h.top <= top + (parseFloat(hs.top) || 0) + 1 && h.bottom > top) top = Math.max(top, h.bottom);
      }
      const viewport = { top, bottom, left: main ? 0 : Math.max(0, r.left + s.clientLeft), right: Math.min(innerWidth, main ? innerWidth : r.left + s.clientLeft + s.clientWidth) };
      const b = n.getBoundingClientRect(); let t = Math.max(top, b.top), end = Math.min(bottom, b.bottom), left = Math.max(viewport.left, b.left), right = Math.min(viewport.right, b.right), closed = false;
      for (let p = n.parentElement; p && p !== s && p !== document.body; p = p.parentElement) {
        if (p.tagName === 'DETAILS' && !p.open) closed = true;
        if (/hidden|clip|auto|scroll/.test(getComputedStyle(p).overflowY)) {
          const box = p.getBoundingClientRect(); t = Math.max(t, box.top); end = Math.min(end, box.bottom); left = Math.max(left, box.left); right = Math.min(right, box.right);
        }
      }
      return { scroller: s, viewport, exposedPixels: closed ? 0 : Math.max(0, end - t), visible: !closed && b.height > 0 && end - t >= 1 && right - left >= 1 };
    };
    window.__viewedSockets = [];
    window.WebSocket = class {
      constructor(url) {
        const u = new URL(url);
        if (u.host !== new URL(origin).host || !/\/events$/.test(u.pathname)) throw new Error('non-fixture WebSocket ' + url);
        this.url = url; window.__viewedSockets.push(this);
        setTimeout(() => { if (this.onopen) this.onopen(); }, 0);
      }
      send() {} close() {}
    };
  }, { origin: info.dashboard_origin });
  const page = await context.newPage(); run.page = page;
  page.on('pageerror', e => fail(new Error('page script: ' + e.message)));
  page.on('request', request => {
    const record = { url: request.url(), resourceType: request.resourceType(), frame: frameIdentity(request.frame()), run: run.name };
    evidence.lastRequest = record;
    if (request.isNavigationRequest()) evidence.requests.push(record);
    persist();
  });
  page.on('requestfailed', request => {
    evidence.lastFailedRequest = { url: request.url(), resourceType: request.resourceType(), frame: frameIdentity(request.frame()), failure: request.failure() }; persist();
    fail(new Error('request failed: ' + request.url() + ' ' + JSON.stringify(request.failure())));
  });
  page.on('response', response => {
    const request = response.request();
    const job = (async () => {
      const record = { url: response.url(), resourceType: request.resourceType(), frame: frameIdentity(request.frame()), status: response.status(), headers: await response.allHeaders(), run: run.name };
      // Network shell/session documents only, never sandbox srcdoc content.
      const u = new URL(response.url());
      if (u.origin === info.dashboard_origin && u.pathname === '/s/livesession') {
        const text = await response.text(); record.bodyPrefix = text.slice(0, 360);
        record.assetID = (text.match(/data-assets="([^"]+)"/) || [])[1] || null;
      }
      evidence.lastResponse = record;
      if (request.isNavigationRequest()) evidence.documents.push(record);
      persist();
      if (record.status >= 400) throw new Error('fixture HTTP ' + record.status + ': ' + record.url);
    })();
    run.responseJobs.add(job); job.catch(fail).finally(() => run.responseJobs.delete(job));
  });
  // Normal shell navigation is essential: top-level ?frame=1 redirects to a
  // shell that has no wbHistory. All product work below uses its active frame.
  await guard(run, page.goto(info.dashboard_origin + '/s/livesession'));
  const activeFrame = page.locator('iframe.sframe.on');
  await guard(run, activeFrame.waitFor({ state: 'visible' }));
  run.frame = await (await activeFrame.elementHandle()).contentFrame();
  assert(run.frame, 'active session contentFrame exists');
  await guard(run, run.frame.waitForFunction(() => window.wbHistory && window.WsCards && document.getElementById('r-board') && window.__viewedSockets.some(s => s.url.includes('/s/livesession/events'))));
  check(run.name + ' embedded asset identity', await guard(run, run.frame.evaluate(() => document.documentElement.dataset.assets)) === info.asset_id, info.asset_id);
  if (!(await guard(run, run.frame.locator('#p2').isVisible()))) await guard(run, run.frame.locator('[data-tab="board"]').first().click());
  await guard(run, run.frame.waitForFunction(() => {
    const panels = [...document.querySelectorAll('.entry-panel[open]')].filter(n => window.__viewedGeometry(n).visible);
    return panels.length > 0 && panels.every(n => { const frames = [...n.querySelectorAll('.doc-frame:not(.doc-frame-next)')]; return frames.length > 0 && frames.every(f => f.tagName === 'IFRAME' && parseFloat(f.style.height) > 0); });
  }));
  await quiet(run, 300);
  const initial = await geometry(run), hydrated = initial.panels.filter(p => p.visible && p.open && p.frames.length);
  check(run.name + ' initial visible content hydrates without pending changes', initial.badge.length === 0 && hydrated.length > 0 && hydrated.every(p => p.frames.every(f => f.tag === 'IFRAME' && parseFloat(f.height) > 0)), initial);
  // P1 exceeds the simultaneous 30-log/3-panel/1000px newest chunk budget.
  // Admit it through the real loader, then cancel/settle navigation before
  // testing exposure. No clip CSS or initial pending-bar bypass is permitted.
  await admit(run, 'p1');
  await positionHistory(run, 'p1');
  await guard(run, run.frame.evaluate(() => {
    for (const name of ['PinAbove', 'P', 'Q', 'PinBelow']) window.WsCards.move(document.querySelector('section[data-topic="' + name + '"]'), { pin: true, index: 99 });
  }));
  await position(run, 'p2', 70);
  if (width === 1800) { await guard(run, run.frame.locator('#p6').scrollIntoViewIfNeeded()); await quiet(run); await position(run, 'p6', 30); }
  await quiet(run);
  check(run.name + ' no unsolicited pending changes after admission', !(await geometry(run)).badge.length, null);
  return run;
}
async function closeRun(run) {
  await guard(run, Promise.all([...run.responseJobs]));
  captureServiceWorkers(run);
  check(run.name + ' no service worker registration attempts', run.serviceWorkers.attempts.length === 0, run.serviceWorkers.attempts);
  check(run.name + ' no service worker creations', run.serviceWorkers.events.length === 0, run.serviceWorkers.events);
  check(run.name + ' no remaining service workers', run.serviceWorkers.remaining.length === 0, run.serviceWorkers.remaining);
  await run.context.close(); if (active === run) active = null;
}
async function height(run, id, increment, wrong = false) {
  const host = await guard(run, run.frame.locator('#' + id + ' iframe.doc-frame:not(.doc-frame-next)').first().elementHandle());
  check(run.name + ' registered sandbox ' + id, !!host, null);
  const data = await host.evaluate((f, increment) => ({ key: f.dataset.docKey, height: parseFloat(f.style.height) + increment, sandbox: f.getAttribute('sandbox') }), increment);
  check(run.name + ' opaque sandbox has no same-origin permission', !!data.sandbox && !data.sandbox.includes('allow-same-origin'), data.sandbox);
  if (wrong) {
    const child = await host.contentFrame();
    // Boundary probe only: actual opaque WindowProxy, no child DOM reads.
    await child.evaluate(({ key, height }) => parent.postMessage({ wbDoc: 'height', docKey: 'forged:' + key, height }, '*'), data);
  } else {
    // The authored fixture payload grows its own document; the real production
    // ResizeObserver/bridge reports the resulting height. No fabricated valid
    // height report and no parent-side opaque DOM access are needed.
    await host.evaluate((f, stimulus) => f.contentWindow.postMessage(stimulus, '*'), { viewedFixturePanel: id, pixels: increment });
  }
  await quiet(run); return data;
}
async function queueScenario(info, mode, width) {
  const run = await mount(info, mode, width, 'queued');
  const before = await snap(run, run.name + '-before'), old = await bytes(run, 'p3');
  check(run.name + ' A/B simultaneous geometry precondition', before.panels.some(p => p.id === 'p2' && p.visible) && before.panels.some(p => p.id === 'p3' && p.visible), before);
  await emit(run, 'queue1'); const first = await snap(run, run.name + '-first');
  await emit(run, 'queue2'); const after = await snap(run, run.name + '-after');
  const observations = { first: preserved(before, first), queued: preserved(before, after), bytesHeld: old === await bytes(run, 'p3'), badge: after.badge };
  if (mode === 'baseline') observe(run.name, observations);
  else {
    allPreserved(run.name + ' first reorder freezes EVERY visible panel', before, first);
    allPreserved(run.name + ' queued reorder freezes EVERY visible panel', before, after);
    check(run.name + ' rendering stays frozen away from reading line', observations.bytesHeld && after.panels.find(p => p.id === 'p3').top > after.panels.find(p => p.id === 'p3').viewport.top + 1, observations);
    check(run.name + ' one Korean pill counts a twice-revised topic once', after.badge.length === 1 && after.badge[0] === '보드 업데이트 1', after.badge);
    check(run.name + ' owner/native anchoring policy', after.panels.filter(p => p.visible).every(p => p.owner && p.nativeAnchor === 'none'), after);
    await guard(run, run.frame.locator('.wb-oq-top .oq-updates').click()); await quiet(run);
    const clicked = await snap(run, run.name + '-explicit');
    check(run.name + ' explicit click releases latest rendering/count', (await bytes(run, 'p3')).includes('queue2') && !clicked.badge.length, clicked);
  }
  await closeRun(run);
}
async function onePixel(width) {
  const run = await mount(pair[1], 'result', width, 'one-pixel');
  // Position bottom one pixel beyond the independent exposed top; include
  // actual sticky/toolbar and history-clip clipping in the measurement.
  for (let i = 0; i < 5; i++) {
    await guard(run, run.frame.evaluate(() => {
      const n = document.getElementById('p1'), g = window.__viewedGeometry(n);
      g.scroller.scrollTop += n.getBoundingClientRect().bottom - g.viewport.top - 1;
    })); await quiet(run, 45);
  }
  await quiet(run);
  const before = await snap(run, run.name + '-before'), p = before.panels.find(p => p.id === 'p1'), sibling = before.panels.find(p => p.id === 'p2');
  check(run.name + ' exactly 1px (not zero or reading-line shortcut)', p.visible && Math.abs(p.exposedPixels - 1) <= .5 && Math.abs(p.bottom - p.viewport.top - 1) <= .5 && sibling.visible, { p, sibling });
  const old = await bytes(run, 'p1');
  await guard(run, run.frame.locator('#ctext').focus());
  await emit(run, 'changeA'); const held = await snap(run, run.name + '-held');
  check(run.name + ' sliver rendering and focus held', old === await bytes(run, 'p1') && held.focus.id === 'ctext' && held.badge.some(s => /^보드 업데이트 [1-9]/.test(s)), held);
  allPreserved(run.name + ' sliver holds EVERY exposed panel', before, held);
  // Scroll ONLY far enough for changed p1 to fully leave; avoid moving to a
  // guessed later reference. Capture after scroll and BEFORE staged release.
  const releaseBefore = await guard(run, run.frame.evaluate(() => {
    const n = document.getElementById('p1'), sibling = document.getElementById('p2'), g = window.__viewedGeometry(n);
    const shift = n.getBoundingClientRect().bottom - g.viewport.top + 2;
    const sTop = g.scroller === document.scrollingElement ? 0 : g.scroller.getBoundingClientRect().top + g.scroller.clientTop;
    // Set up release with a manager-owned write, not a synthetic user scroll
    // whose approved cooldown would intentionally suppress compensation.
    const move = window.wbHistory.keep(sibling, { offset: sibling.getBoundingClientRect().top - shift - sTop });
    move.apply();
    return [...document.querySelectorAll('.entry-panel')].filter(n => window.__viewedGeometry(n).visible).map(n => ({ id: n.id, top: n.getBoundingClientRect().top, height: n.getBoundingClientRect().height }));
  }));
  await quiet(run); const released = await snap(run, run.name + '-released');
  check(run.name + ' changed panel offscreen, unchanged sibling still visible', !released.panels.find(p => p.id === 'p1').visible && released.panels.find(p => p.id === 'p2').visible, released);
  check(run.name + ' fully-offscreen release admits changed payload', (await bytes(run, 'p1')).includes('External rewrite'), released.badge);
  check(run.name + ' release preserves EACH remaining visible panel', releaseBefore.length > 0 && releaseBefore.every(p => { const n = released.panels.find(n => n.id === p.id); return n && Math.abs(n.top - p.top) <= 1 && Math.abs(n.height - p.height) <= 1; }), { releaseBefore, after: released });
  await closeRun(run);
}
async function pinned(width) {
  const run = await mount(pair[1], 'result', width, 'pins');
  await position(run, 'p6', 30);
  const before = await snap(run, run.name + '-before'), p = before.panels.find(p => p.id === 'p6'), q = before.panels.find(p => p.id === 'p7');
  check(run.name + ' P/Q simultaneous pinned precondition', p.visible && q.visible && p.top < q.top && (width === 1800 ? p.scroller !== before.panels.find(n => n.id === 'p2').scroller : p.scroller === before.panels.find(n => n.id === 'p2').scroller), { p, q });
  const old = await bytes(run, 'p7');
  await emit(run, 'pin1'); await emit(run, 'pin2');
  const held = await snap(run, run.name + '-held');
  allPreserved(run.name + ' queued pinned recency updates preserve all visible panels', before, held);
  check(run.name + ' pinned rendering/count held', old === await bytes(run, 'p7') && held.badge.length === 1 && held.badge[0] === '보드 업데이트 1', held);
  // Pinned order is user-authored, not latest server recency. Record that
  // distinction instead of manufacturing a server reorder by patching weave.
  observe(run.name + ' pinned recency does not override manual pin order', held.topics);
  await guard(run, run.frame.locator('.wb-oq-top .oq-updates').click()); await quiet(run);
  check(run.name + ' pinned explicit release', (await bytes(run, 'p7')).includes('pin2'), null);
  const moved = await guard(run, run.frame.evaluate(() => {
    window.WsCards.move(document.querySelector('#bpins section[data-topic="Q"]'), { pin: true, index: 1 });
    return [...window.WsCards.pinList().children].filter(n => n.dataset.topic).map(n => n.dataset.topic);
  }));
  check(run.name + ' explicit owner pin reorder applies immediately', moved.indexOf('Q') < moved.indexOf('P'), moved);
  await closeRun(run);
}
async function growth(width, pinnedColumn) {
  const run = await mount(pair[1], 'result', width, pinnedColumn ? 'pin-growth' : 'main-growth');
  const ids = pinnedColumn ? ['p8', 'p9'] : ['p4', 'p5'];
  for (const id of ids) { await guard(run, run.frame.locator('#' + id).scrollIntoViewIfNeeded()); await quiet(run); }
  // Require two exposed reading panels; derive offset from their real heights
  // rather than assuming history header or payload height from an old cut.
  const focus = pinnedColumn ? 'p6' : 'p2';
  if (!pinnedColumn) await admit(run, 'p1');
  const interval = await guard(run, run.frame.evaluate(({ focus, ids }) => {
    const n = document.getElementById(focus), g = window.__viewedGeometry(n), r = n.getBoundingClientRect();
    const a = document.getElementById(ids[0]).getBoundingClientRect(), b = document.getElementById(ids[1]).getBoundingClientRect();
    const sibling = document.getElementById(focus === 'p6' ? 'p7' : 'p3').getBoundingClientRect();
    return { lower: Math.max(0, g.viewport.bottom - g.viewport.top - (b.top - r.top) + 4),
      upper: Math.min(r.top - a.bottom - 4, g.viewport.bottom - g.viewport.top - (sibling.top - r.top) - 4) };
  }, { focus, ids }));
  check(run.name + ' nonempty measured all-visible growth interval', interval.lower <= interval.upper, interval);
  await position(run, focus, Math.min(interval.upper, Math.max(30, interval.lower)));
  const before = await snap(run, run.name + '-before');
  const a = before.panels.find(p => p.id === ids[0]), b = before.panels.find(p => p.id === ids[1]);
  check(run.name + ' exact above/below precondition', !a.visible && a.bottom <= a.viewport.top && !b.visible && b.top >= b.viewport.bottom && before.panels.filter(p => p.visible && p.scroller === a.scroller).length >= 2, { a, b, before });
  for (const id of ids) await height(run, id, 61);
  const after = await snap(run, run.name + '-after');
  check(run.name + ' both offscreen frames really grew', ids.every(id => {
    const old = before.panels.find(p => p.id === id), next = after.panels.find(p => p.id === id);
    return Math.abs(parseFloat(next.frames[0].height) - parseFloat(old.frames[0].height) - 61) <= 1;
  }), { before: before.panels.filter(p => ids.includes(p.id)), after: after.panels.filter(p => ids.includes(p.id)) });
  allPreserved(run.name + ' iframe growth preserves EVERY exposed panel', before, after);
  const target = after.panels.find(p => p.id === focus), oldHeight = target.frames[0].height;
  await height(run, focus, 37, true);
  check(run.name + ' wrong docKey rejected', (await geometry(run)).panels.find(p => p.id === focus).frames[0].height === oldHeight, null);
  await guard(run, run.frame.evaluate(key => window.postMessage({ wbDoc: 'height', docKey: key, height: 9999 }, '*'), target.frames[0].key));
  await quiet(run);
  check(run.name + ' wrong source Window rejected', (await geometry(run)).panels.find(p => p.id === focus).frames[0].height === oldHeight, null);
  await height(run, focus, 37);
  const held = await snap(run, run.name + '-height-held');
  check(run.name + ' visible height held in shared pill', held.panels.find(p => p.id === focus).frames[0].height === oldHeight && held.badge.length === 1, held);
  allPreserved(run.name + ' held height preserves each exposed panel', after, held);
  await guard(run, run.frame.locator('.wb-oq-top .oq-updates').click()); await quiet(run);
  check(run.name + ' explicit height release', (await geometry(run)).panels.find(p => p.id === focus).frames[0].height !== oldHeight, null);
  await closeRun(run);
}
async function fetched(width) {
  const run = await mount(pair[1], 'result', width, 'fetched');
  const more = run.frame.locator('[data-closed-next]').last();
  await guard(run, more.scrollIntoViewIfNeeded()); await guard(run, more.click());
  await guard(run, run.frame.locator('.closed-page section[data-topic="Past"]').waitFor());
  await admit(run, 'p12'); // Real older content below Past supplies unclamped scroll range.
  await admit(run, 'p10'); await positionHistory(run, 'p10');
  const before = await snap(run, run.name + '-before'), old = await bytes(run, 'p10');
  const history = await guard(run, run.frame.evaluate(() => window.wbHistory.capture(document.getElementById('r-board')).map(h => ({ key: h.key, chunks: h.chunks }))));
  check(run.name + ' fetched panels exposed precondition', before.panels.some(p => p.id === 'p10' && p.visible) && before.panels.some(p => p.id === 'p11' && p.visible), before);
  await emit(run, 'transferEdit'); const held = await snap(run, run.name + '-held');
  check(run.name + ' transfer/content staged in fetched page', await guard(run, run.frame.locator('.closed-page #p10').count()) === 1 && old === await bytes(run, 'p10') && held.badge.some(s => /^보드 업데이트 [1-9]/.test(s)), held);
  allPreserved(run.name + ' fetched transfer freezes each visible panel', before, held);
  check(run.name + ' fetched transfer keeps mounted panel membership', JSON.stringify(before.panels.map(p => p.id)) === JSON.stringify(held.panels.map(p => p.id)), { before: before.panels.map(p => p.id), after: held.panels.map(p => p.id) });
  check(run.name + ' history boundaries survive held transfer', JSON.stringify(history) === JSON.stringify(await guard(run, run.frame.evaluate(() => window.wbHistory.capture(document.getElementById('r-board')).map(h => ({ key: h.key, chunks: h.chunks }))))), history);
  // Changed first panel leaves, unchanged sibling remains: eligible render
  // updates in place, while the card itself still blocks top-level transfer.
  const releaseBefore = await guard(run, run.frame.evaluate(() => {
    const changed = document.getElementById('p10'), next = document.getElementById('p11'), g = window.__viewedGeometry(next);
    const shift = changed.getBoundingClientRect().bottom - g.viewport.top + 2;
    const sTop = g.scroller === document.scrollingElement ? 0 : g.scroller.getBoundingClientRect().top + g.scroller.clientTop;
    window.wbHistory.keep(next, { offset: next.getBoundingClientRect().top - shift - sTop }).apply();
    return [...document.querySelectorAll('.entry-panel')].filter(n => window.__viewedGeometry(n).visible).map(n => ({ id: n.id, top: n.getBoundingClientRect().top, height: n.getBoundingClientRect().height }));
  }));
  await quiet(run);
  const inPlace = await snap(run, run.name + '-in-place');
  check(run.name + ' fetched rendering releases in place', (await bytes(run, 'p10')).includes('Fetched external revision') && await guard(run, run.frame.locator('.closed-page #p11').count()) === 1 && inPlace.panels.find(p => p.id === 'p11').visible, inPlace);
  check(run.name + ' in-place release preserves each remaining visible panel', releaseBefore.length > 0 && releaseBefore.every(p => { const n = inPlace.panels.find(n => n.id === p.id); return n && Math.abs(n.top - p.top) <= 1 && Math.abs(n.height - p.height) <= 1; }), { releaseBefore, inPlace });
  await position(run, 'p2', 70); await quiet(run);
  check(run.name + ' fetched membership releases once fully offscreen', await guard(run, run.frame.locator('#r-board > section[data-topic="Past"]').count()) === 1 && await guard(run, run.frame.locator('.closed-page #p10').count()) === 0, await geometry(run));
  await closeRun(run);
}
async function owner(width) {
  const run = await mount(pair[1], 'result', width, 'owner');
  const memo = run.frame.locator('section[data-topic="A"] textarea.tnote');
  await guard(run, run.frame.locator('section[data-topic="A"] .tnote-toggle').click());
  await guard(run, memo.fill('Owner local memo'));
  check(run.name + ' local memo immediate', await guard(run, memo.inputValue()) === 'Owner local memo', null);
  await guard(run, run.frame.locator('#csend').click()); await quiet(run);
  check(run.name + ' local send only grants A input', run.sent.length === 1 && run.sent[0].topics.some(t => t.topic === 'A'), run.sent);
  await admit(run, 'p1'); await positionHistory(run, 'p1');
  check(run.name + ' external rewrite panel visible precondition', (await geometry(run)).panels.find(p => p.id === 'p1').visible, null);
  const old = await bytes(run, 'p1');
  await emit(run, 'changeA');
  check(run.name + ' local send alone never exempts an external rewrite', old === await bytes(run, 'p1') && (await geometry(run)).badge.some(s => /^보드 업데이트 [1-9]/.test(s)), null);
  await emit(run, 'mixed'); const after = await snap(run, run.name + '-mixed');
  check(run.name + ' receipt-only partial snapshot isolates same-topic external rewrite', await guard(run, run.frame.locator('.wb-entry.owner').count()) > 0 && old === await bytes(run, 'p1') && after.badge.some(s => /^보드 업데이트 [1-9]/.test(s)), after);
  await closeRun(run);
}
async function core() {
  const run = await mount(pair[1], 'result', 1800, 'owner-core');
  // Required sliver/gap reading-line selection uses existing real panels.
  const gap = await guard(run, run.frame.evaluate(() => document.getElementById('p2').getBoundingClientRect().top - document.getElementById('p1').getBoundingClientRect().bottom));
  check(run.name + ' real inter-panel reading gap exists', gap > 4, gap);
  await position(run, 'p2', gap / 2);
  const selection = await guard(run, run.frame.evaluate(() => {
    const n = document.getElementById('p2'), g = window.__viewedGeometry(n), line = g.viewport.top + 1;
    const candidates = [...g.scroller.querySelectorAll('.entry-panel, .wb-entry:not(.panel), .doc-frame, [data-card]')].filter(p => window.__viewedGeometry(p).visible && !(p.matches('.doc-frame') && p.closest('.entry-panel'))).sort((a,b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top);
    return { line, p2Top: n.getBoundingClientRect().top, crossing: candidates.filter(p => p.getBoundingClientRect().top <= line && p.getBoundingClientRect().bottom > line).map(p => p.id), next: (candidates.find(p => p.getBoundingClientRect().top > line) || {}).id };
  }));
  check(run.name + ' real reading line lies in gap before p2', selection.crossing.length === 0 && selection.next === 'p2' && selection.p2Top > selection.line, selection);
  const transaction = await guard(run, run.frame.evaluate(() => {
    const n = document.getElementById('p2'), above = document.querySelector('section[data-topic="Above"]'), w = window.wbHistory;
    const before = n.getBoundingClientRect().top, outer = w.keep(n), inner = w.keep(n);
    const margin = parseFloat(getComputedStyle(above).marginBottom) || 0;
    above.style.marginBottom = margin + 60 + 'px'; inner.apply(); const nested = n.getBoundingClientRect().top;
    outer.apply(); return { before, nested, after: n.getBoundingClientRect().top };
  }));
  check(run.name + ' gap reference and nested outer apply once', Math.abs(transaction.nested - transaction.before - 60) <= 1 && Math.abs(transaction.after - transaction.before) <= 1, transaction);
  for (const kind of ['wheel', 'touchstart', 'pointerdown', 'keydown']) {
    await quiet(run, 260);
    const input = await guard(run, run.frame.evaluate(async kind => {
      const n = document.getElementById('p2'), above = document.querySelector('section[data-topic="Above"]'), s = window.__viewedGeometry(n).scroller, w = window.wbHistory;
      const send = () => {
        if (kind === 'keydown') { const summary = n.querySelector('summary'); summary.focus({ preventScroll: true }); summary.dispatchEvent(new KeyboardEvent('keydown', { key: 'PageDown', bubbles: true })); }
        else s.dispatchEvent(kind === 'wheel' ? new WheelEvent(kind, { deltaY: 10, bubbles: true }) : kind === 'pointerdown' ? new PointerEvent(kind, { pointerType: 'touch', bubbles: true }) : new Event(kind, { bubbles: true }));
      };
      const before = n.getBoundingClientRect().top, h = w.keep(n); send();
      const margin = parseFloat(above.style.marginBottom) || 0;
      above.style.marginBottom = margin + 20 + 'px'; h.apply(); const canceled = n.getBoundingClientRect().top;
      const during = w.keep(n); above.style.marginBottom = margin + 40 + 'px'; during.apply(); const cooldown = n.getBoundingClientRect().top;
      // User-associated momentum scroll extends the deadline. Measure from the
      // LAST scroll, not just the initial synthetic intent timestamp.
      await new Promise(r => setTimeout(r, 130)); s.scrollTop += 2;
      await new Promise(r => setTimeout(r, 130)); const m0 = n.getBoundingClientRect().top;
      const momentum = w.keep(n); above.style.marginBottom = margin + 60 + 'px'; momentum.apply(); const m1 = n.getBoundingClientRect().top;
      await new Promise(r => setTimeout(r, 230)); const r0 = n.getBoundingClientRect().top;
      const resumed = w.keep(n); above.style.marginBottom = margin + 80 + 'px'; resumed.apply();
      return { before, canceled, cooldown, m0, m1, r0, r1: n.getBoundingClientRect().top };
    }, kind));
    check(run.name + ' ' + kind + ' cancellation/cooldown/momentum', Math.abs(input.canceled - input.before - 20) <= 1 && Math.abs(input.cooldown - input.canceled - 20) <= 1 && Math.abs(input.m1 - input.m0 - 20) <= 1 && Math.abs(input.r1 - input.r0) <= 1, input);
  }
  await cancelJump(run, 'p2');
  // Reach the explicit owner through the PRODUCTION recap/history loader.
  const jump = await guard(run, run.frame.evaluate(async () => {
    const w = window.wbHistory, target = w.loadTarget(document.getElementById('r-board'), 'p3', window.WsCards.unfoldTopic), g = window.__viewedGeometry(target);
    const initial = target.getBoundingClientRect().top, active = !!w.keeping(g.scroller);
    await new Promise(r => setTimeout(r, 40)); document.querySelector('section[data-topic="Above"]').style.marginBottom = '250px';
    await new Promise(r => setTimeout(r, 65));
    const corrected = target.getBoundingClientRect().top, inspect = w.keep(target, { capture: false });
    g.scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: 1, bubbles: true }));
    return { initial, corrected, active, canceled: !w.keeping(g.scroller), reduced: matchMedia('(prefers-reduced-motion: reduce)').matches, inspect: typeof inspect.visible === 'function' && typeof inspect.viewport === 'function' };
  }));
  check(run.name + ' real loadTarget jump late correction/reduced-motion/cancel', jump.active && jump.canceled && jump.reduced && jump.inspect && Math.abs(jump.initial - jump.corrected) <= 1, jump);
  await cancelJump(run, 'p2'); await position(run, 'p2', -20);
  const gone = await guard(run, run.frame.evaluate(() => {
    const n = document.getElementById('p2'), g = window.__viewedGeometry(n), line = g.viewport.top + 1;
    const reference = n.getBoundingClientRect().top <= line && n.getBoundingClientRect().bottom > line;
    const h = window.wbHistory.keep(n), before = g.scroller.scrollTop; n.remove(); h.apply();
    return { reference, before, after: g.scroller.scrollTop };
  }));
  check(run.name + ' removed actual reading reference skips fallback', gone.reference && Math.abs(gone.before - gone.after) <= 1, gone);
  await snap(run, run.name + '-final'); await closeRun(run);
}
(async () => {
  try {
    server = await chromium.launchServer({ headless: true, timeout: 8000 });
    track(); monitor = setInterval(() => {
      try { track(); } catch (e) { trackingError = e; evidence.processTrackingFailure = e.stack; persist(); if (active) active.fail(e); }
    }, 250); monitor.unref();
    browser = await chromium.connect(server.wsEndpoint(), { timeout: 8000 });
    for (const width of [1800, 420]) {
      await queueScenario(pair[0], 'baseline', width);
      await queueScenario(pair[1], 'result', width);
      await onePixel(width); await pinned(width);
      await growth(width, false); await growth(width, true);
      await fetched(width); await owner(width);
    }
    await core(); evidence.completed = true; persist();
    console.log('PASS one paired fixture browser batch; assertions and captures retained');
  } catch (e) {
    evidence.failure = { message: e.message, stack: e.stack, lastRequest: evidence.lastRequest, lastFailedRequest: evidence.lastFailedRequest, routeFailure: evidence.routeFailure, lastDocument: evidence.documents[evidence.documents.length - 1], lastResponse: evidence.lastResponse };
    // Do not use guard here: preserve available evidence even after a latched
    // route error. Read only the trusted session document, not child srcdoc.
    if (active && active.page && !active.page.isClosed()) {
      try {
        if (active.frame && !active.frame.isDetached()) {
          const dom = await bounded(active.frame.evaluate(() => ({ url: location.href, assets: document.documentElement.dataset.assets, prefix: document.documentElement.outerHTML.slice(0, 360), panels: [...document.querySelectorAll('.entry-panel')].map(n => { const r = n.getBoundingClientRect(); return { id: n.id, top: r.top, bottom: r.bottom, height: r.height }; }) })), 2000, 'failure geometry cap');
          save('failure-geometry.json', dom);
        }
        await active.page.screenshot({ path: path.join(capture, 'failure.png'), timeout: 4000 });
      } catch (captureError) { evidence.failure.captureError = captureError.stack; }
    }
    persist(); console.error(e.stack); process.exitCode = 1;
  } finally {
    clearTimeout(cap); persist();
    await cleanup();
  }
})().catch(e => { evidence.cleanupFailure = e.stack; persist(); console.error(e.stack); process.exitCode = 1; });
