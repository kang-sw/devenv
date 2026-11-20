// Run through TestGalleryBrowser against a disposable daemon: the paged
// docking gallery (cards.js over gallery.js) at real viewports. The
// terminal list and socket are routed here; no shell starts.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, control] = process.argv.slice(2);
const shots = process.env.WS_GALLERY_SHOTS;
if (shots) fs.mkdirSync(shots, { recursive: true });
const KEY = 'ws.dashboard.cards.livesession';
const TID = 't-gallery';

async function until(fn, what, ms = 8000) {
  const limit = Date.now() + ms;
  for (;;) {
    let v;
    try { v = await fn(); } catch (e) { v = false; }
    if (v) return v;
    if (Date.now() > limit) throw new Error('timed out: ' + what);
    await new Promise(r => setTimeout(r, 40));
  }
}
async function frameOf(tab) {
  const host = tab.locator('iframe.sframe.on');
  await host.waitFor({ state: 'visible' });
  return (await host.elementHandle()).contentFrame();
}
async function center(loc) {
  const b = await loc.boundingBox();
  assert(b, 'no box for a drag end');
  return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
}
// grip is a point on a topic's title bar or a card's header that is not a
// control (a press there starts a drag).
async function grip(loc) {
  await loc.scrollIntoViewIfNeeded();
  const b = await loc.boundingBox();
  assert(b, 'no header box');
  return { x: b.x + Math.min(24, b.width / 4), y: b.y + b.height / 2 };
}
// drag presses at from, moves past the slop (during runs there), then
// releases over what to() returns.
async function drag(tab, from, to, during) {
  await tab.mouse.move(from.x, from.y);
  await tab.mouse.down();
  await tab.mouse.move(from.x + 14, from.y + 14, { steps: 4 });
  const seen = during ? await during() : null;
  const at = await to();
  await tab.mouse.move(at.x, at.y, { steps: 8 });
  await tab.mouse.up();
  return seen;
}
// In-page readers, installed in every session frame.
const helpers = () => {
  window.__gal = {
    rec: () => JSON.parse(localStorage.getItem('ws.dashboard.cards.livesession') || localStorage.getItem('ws.dashboard.cards.boardsession') || '{}'),
    pages: () => {
      const g = window.__gal.rec().gallery;
      return g ? g.pages.map(p => window.WsGallery.leaves(p.root).map(r => r.slice(2))) : [];
    },
    active: () => {
      const g = window.__gal.rec().gallery;
      if (!g || !g.active) return null;
      const i = g.pages.findIndex(p => p.id === g.active);
      return i < 0 ? null : i;
    },
    pos: () => +document.getElementById('bgal').getAttribute('data-pos'),
    dots: () => [...document.querySelectorAll('#bgal .gal-dot')].map(b => b.textContent + (b.classList.contains('active') ? '*' : '')),
    // seen tells, per element, whether a fresh IntersectionObserver
    // (root: the viewport) reports it intersecting.
    seen: els => new Promise(done => {
      const out = new Map();
      const io = new IntersectionObserver(es => {
        es.forEach(e => out.set(e.target, e.isIntersecting));
        if (out.size === els.length) { io.disconnect(); done(els.map(e => out.get(e))); }
      });
      els.forEach(e => io.observe(e));
    }),
    frames: () => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))),
  };
};
async function open(context, id, w, h) {
  const tab = await context.newPage();
  if (w) await tab.setViewportSize({ width: w, height: h });
  const errors = [];
  tab.on('pageerror', e => errors.push(e.message));
  await tab.goto(base + '/s/' + id);
  const page = await frameOf(tab);
  await page.locator('section.topic[data-topic="deep"]').waitFor({ state: 'attached' });
  await page.waitForFunction(() => window.WsCards && window.WsCards.gallery && window.WsCards.gallery());
  await page.evaluate(helpers);
  return { tab, page, errors };
}
const pages = page => page.evaluate(() => window.__gal.pages());
const active = page => page.evaluate(() => window.__gal.active());
const pos = page => page.evaluate(() => window.__gal.pos());
const dot = (page, i) => page.locator(`#bgal .gal-dot[data-pos="${i}"]`);
const topic = (page, name) => page.locator(`section.topic[data-topic="${name}"]`);
const bar = (page, name) => topic(page, name).locator(':scope > .tophead > .tbar');
async function plus(page) { return center(page.locator('#bgal .gal-dot.empty')); }
async function shot(tab, name) { if (shots) await tab.screenshot({ path: path.join(shots, name + '.png') }); }
async function seed(context, id, record, layout) {
  const tab = await context.newPage();
  await tab.goto(base + '/s/' + id);
  await tab.evaluate(([k, v, l]) => { localStorage.setItem(k, v); if (l) localStorage.setItem('ws.dashboard.layout', l); else localStorage.removeItem('ws.dashboard.layout'); },
    ['ws.dashboard.cards.' + id, JSON.stringify(record), layout || null]);
  await tab.close();
}
// The terminal capability is synthetic: the daemon's list and the card's
// socket are answered here, so a terminal card mounts its emulator with
// no shell behind it.
async function routeTerminal(context) {
  await context.route(/\/s\/livesession\/terminals(\?.*)?$/, route => route.request().method() === 'GET'
    ? route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ terminals: [{ id: TID, cwd: '/tmp/galterm', started_at: 1 }] }) })
    : route.continue());
  await context.routeWebSocket(new RegExp('/terminals/' + TID + '/ws'), ws => {
    ws.onMessage(() => {});
    ws.send(JSON.stringify({ t: 'hello' }));
  });
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const done = [];
  try {
    // ---- narrow live page: drag, buttons, keyboard, rail, IO, activation ----
    {
      const context = await browser.newContext({ viewport: { width: 1400, height: 900 }, locale: 'en-US', reducedMotion: 'reduce' });
      await routeTerminal(context);
      await seed(context, 'livesession', {});
      const { tab, page, errors } = await open(context, 'livesession');
      assert.equal(await page.locator('#bgal').isHidden(), true, 'no docked page and no drag: no gallery');
      assert.equal(await page.evaluate(() => document.documentElement.getAttribute('data-gal')), null);

      // Drag-dock from the Main Board: the rail shows on the drag start,
      // with ⌂ and + only, and + docks on a new page with no page before.
      const during = await drag(tab, await grip(bar(page, 'alpha')), () => plus(page), async () => {
        await until(() => page.locator('#bgal').isVisible(), 'rail on drag start');
        return { dots: await page.evaluate(() => window.__gal.dots()), gal: await page.evaluate(() => document.documentElement.getAttribute('data-gal')) };
      });
      assert.deepEqual(during, { dots: ['⌂*', '+'], gal: 'narrow' }, 'a drag start shows the narrow rail with no docked page');
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"]]', 'drop on + docks alpha');
      assert.equal(await active(page), 0, 'the new page shows');
      assert.deepEqual(await page.evaluate(() => window.__gal.dots()), ['⌂', '1*', '+'], 'rail: ⌂ · 1 · +, the page shows its pane count');
      done.push('drag-dock from the Main Board onto + with zero pages (narrow), rail on drag start');

      // The narrow Main Board behind a shown page reports out of view;
      // its scroll offset and nodes stay.
      await dot(page, -1).click();
      await until(async () => await pos(page) === -1, 'home');
      await page.evaluate(() => { const b = document.getElementById('bcol'); b.scrollTop = 240; window.__board = { beta: document.querySelector('section.topic[data-topic="beta"]'), top: b.scrollTop }; });
      await page.evaluate(() => window.__gal.frames());
      const homeSeen = await page.evaluate(() => window.__gal.seen([document.querySelector('section.topic[data-topic="epsilon"]'), document.querySelector('#bgal .gal-pane[data-ref="t:alpha"]')]));
      await dot(page, 0).click();
      await until(async () => await pos(page) === 0, 'page 1');
      await page.evaluate(() => window.__gal.frames());
      const away = await page.evaluate(async () => {
        const b = document.getElementById('bcol');
        const beta = document.querySelector('section.topic[data-topic="beta"]');
        return { seen: await window.__gal.seen([document.querySelector('section.topic[data-topic="epsilon"]'), beta, document.querySelector('#bgal .gal-pane[data-ref="t:alpha"]')]),
          same: beta === window.__board.beta, top: b.scrollTop, was: window.__board.top, attr: document.documentElement.hasAttribute('data-gal-away'),
          inert: document.getElementById('r-board').inert, cv: getComputedStyle(b.firstElementChild).visibility };
      });
      assert.deepEqual(homeSeen, [true, false], 'Page 0: the board in view, the inactive page out of it');
      assert.deepEqual(away.seen, [false, false, true], 'a shown page: the Main Board out of view (IO), the page in it');
      assert.deepEqual([away.same, away.top, away.attr, away.inert, away.cv], [true, away.was, true, true, 'visible'], 'the Main Board keeps its nodes and scroll offset, by transform alone');
      await dot(page, -1).click();
      await until(async () => await pos(page) === -1, 'home again');
      assert.equal(await page.evaluate(() => document.getElementById('bcol').scrollTop === window.__board.top && !document.documentElement.hasAttribute('data-gal-away')), true, 'back on Page 0 at the same offset');
      done.push('narrow Main Board non-intersecting behind a docked page, scroll offset and DOM kept');

      // The pin button path: a new page before +, shown; unpin drops the
      // emptied page.
      await bar(page, 'beta').locator('.tpin').click();
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"],["beta"]]', 'pin appends beta');
      assert.equal(await active(page), 1, 'the pinned page shows');
      assert.equal(await bar(page, 'beta').locator('.tpin').getAttribute('aria-pressed'), 'true');
      await bar(page, 'beta').locator('.tpin').click();
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"]]', 'unpin drops the emptied page');
      assert.equal(await topic(page, 'beta').evaluate(n => n.parentNode.id), 'r-board', 'unpinned beta is back on the Main Board');
      done.push('pin button appends+activates a page; unpin drops the emptied page');

      // The keyboard: Enter on a pin button and on a dot.
      if (await pos(page) !== -1) await dot(page, -1).click();
      await bar(page, 'gamma').locator('.tpin').focus();
      await tab.keyboard.press('Enter');
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"],["gamma"]]', 'Enter on pin docks gamma');
      assert.equal(await active(page), 1);
      await dot(page, 0).focus();
      await tab.keyboard.press('Enter');
      await until(async () => await active(page) === 0, 'Enter on a dot shows its page');
      done.push('keyboard Enter on pin button and on a rail dot');

      // A dot dragged onto another reorders the pages.
      await drag(tab, await center(dot(page, 0)), () => center(dot(page, 1)));
      await until(async () => JSON.stringify(await pages(page)) === '[["gamma"],["alpha"]]', 'dot drag reorders');
      done.push('rail dot reorder by dragging a dot');

      // The terminal card docks too; inactive pages stay mounted (frames
      // and the terminal keep their nodes) and are out of view for IO.
      const term = page.locator(`.bcard[data-card]:has(.tm-host)`);
      await until(() => term.count().then(n => n === 1), 'terminal card from the routed list');
      if (await pos(page) !== -1) await dot(page, -1).click();
      await term.locator('.bc-pin').click();
      await until(async () => (await pages(page)).length === 3, 'terminal docked');
      await until(() => page.evaluate(() => !!document.querySelector('#bgal .tm-host .xterm')), 'terminal emulator mounted', 15000);
      await until(() => page.evaluate(() => !!document.querySelector('#bgal section.topic[data-topic="alpha"] iframe.doc-frame')), 'alpha frame mounted');
      const keep = () => page.evaluate(() => {
        const t = document.querySelector('#bgal .tm-host'), f = document.querySelector('#bgal section.topic[data-topic="alpha"] iframe.doc-frame');
        if (!window.__keep) window.__keep = { t, x: t.querySelector('.xterm'), f, w: f.contentWindow };
        const k = window.__keep;
        return k.t === t && k.x === t.querySelector('.xterm') && k.x.isConnected && k.f === f && k.f.isConnected && k.w === f.contentWindow;
      });
      assert.equal(await keep(), true);
      const order = await pages(page);
      const ti = order.findIndex(p => p[0] !== 'gamma' && p[0] !== 'alpha'), ai = order.findIndex(p => p[0] === 'alpha');
      for (const i of [ti, ai, -1, ti, 0, ai]) {
        await dot(page, i).click();
        await until(async () => await pos(page) === i, 'switch to ' + i);
        await page.evaluate(() => window.__gal.frames());
        const seen = await page.evaluate(() => window.__gal.seen([document.querySelector('#bgal .tm-host').closest('.bcard'), document.querySelector('#bgal section.topic[data-topic="alpha"] iframe.doc-frame')]));
        assert.deepEqual(seen, [i === ti, i === ai], 'only the shown page intersects (at ' + i + ')');
        assert.equal(await keep(), true, 'terminal and frame keep their nodes and documents (at ' + i + ')');
      }
      done.push('inactive pages non-intersecting for IO with terminal and iframe DOM mounted, node identity kept across switches');

      // Activation: reveal (a card's show, a topic anchor), navigate (an
      // item reference) and an update card show the page their target
      // stands on first.
      await dot(page, ai).click();
      await page.evaluate(() => window.WsCards.list('terminal')[0].show());
      await until(async () => await pos(page) === ti, 'Card.show reveals the terminal page');
      const alphaID = await topic(page, 'alpha').getAttribute('id');
      await dot(page, -1).click();
      await page.evaluate(id => { location.hash = '#' + id; }, alphaID);
      await until(async () => await pos(page) === ai, 'a topic anchor reveals alpha\'s page');
      await page.evaluate(() => history.replaceState(null, '', location.pathname + location.search));
      done.push('reveal (Card.show, topic anchor) activates the inactive page');

      // beta docks; the conversation's update card for beta shows its page.
      await dot(page, -1).click();
      await bar(page, 'beta').locator('.tpin').click();
      await until(async () => (await pages(page)).some(p => p[0] === 'beta'), 'beta docked');
      const bi = (await pages(page)).findIndex(p => p[0] === 'beta');
      await dot(page, 0).click();
      const card = page.locator('.wbupdate-topic[data-wb-topic]').first();
      await card.scrollIntoViewIfNeeded();
      await card.locator('.wbupdate-text, .wbupdate-log').first().click();
      await until(async () => await pos(page) === bi, 'update card shows beta\'s page');
      done.push('update-card click activates the inactive page');

      // navigate + (12)=a: deep docks; an item reference on the Main Board
      // shows deep's page and scrolls the item into view below its head.
      await dot(page, -1).click();
      await bar(page, 'deep').locator('.tpin').click();
      await until(async () => (await pages(page)).some(p => p[0] === 'deep'), 'deep docked');
      const di = (await pages(page)).findIndex(p => p[0] === 'deep');
      // The answered item stands in deep's history far above its log's end,
      // where the pane is scrolled: reaching it scrolls the pane up, where
      // a sticky head would cover what lands at the scroller's top.
      const before = await page.evaluate(() => {
        const b = document.querySelector('section.topic[data-topic="deep"] > .gal-pbody'); b.scrollTop = b.scrollHeight;
        const t = document.getElementById('item-1'), r = t && t.getBoundingClientRect(), br = b.getBoundingClientRect();
        return { bottom: b.scrollTop, shown: !!r && r.bottom > br.top && r.top < br.bottom };
      });
      assert(before.bottom > 100 && !before.shown, 'deep\'s pane scrolled to its end, item 1 out of its view: ' + JSON.stringify(before));
      const bottom = before.bottom;
      await dot(page, -1).click();
      const ref = topic(page, 'epsilon').locator('a[href="#item-1"]').last();
      await ref.scrollIntoViewIfNeeded();
      await ref.click();
      await until(async () => await pos(page) === di, 'navigate shows deep\'s page');
      let last = null;
      const deep = await until(() => page.evaluate(async () => {
        await window.__gal.frames();
        const t = document.getElementById('item-1'), sec = t.closest('section.topic'), head = sec.querySelector(':scope > .tophead'), body = sec.querySelector(':scope > .gal-pbody');
        const r = t.getBoundingClientRect(), h = head.getBoundingClientRect(), b = body.getBoundingClientRect();
        const hit = document.elementFromPoint(r.left + Math.min(40, r.width / 2), r.top + Math.min(12, r.height / 2));
        return { top: r.top, head: h.bottom, inBody: r.top >= b.top - 0.5 && r.top < b.bottom, owned: !!hit && t.contains(hit), sticky: getComputedStyle(head).position, scrolled: body.scrollTop };
      }).then(v => { const still = last && Math.abs(last.top - v.top) < 0.5; last = v; return still && v.inBody && v.owned ? v : null; }), 'item-1 settled in view in its pane');
      assert(deep.top >= deep.head - 0.5, `deep link lands below the header: target top ${deep.top} >= header bottom ${deep.head}`);
      assert.equal(deep.sticky, 'static', 'the docked head does not stick');
      console.log('deep-link check: pane scrolled ' + bottom + ' -> ' + deep.scrolled + '; target top ' + deep.top.toFixed(1) + ' >= header bottom ' + deep.head.toFixed(1) + ', target owns its point');
      await shot(tab, 'narrow-deep-link');
      done.push('navigate (item reference) activates the inactive page; (12)=a deep link unobstructed by the header');

      // A docked pane's header dropped on ⌂ unpins it, the Main Board shown.
      await dot(page, di).click();
      await drag(tab, await grip(bar(page, 'deep')), () => center(dot(page, -1)));
      await until(async () => !(await pages(page)).some(p => p[0] === 'deep'), 'drop on ⌂ unpins deep');
      assert.equal(await pos(page), -1, 'the Main Board shows');
      assert.equal(await topic(page, 'deep').evaluate(n => n.parentNode.id + ':' + [...n.children].map(c => c.className).includes('gal-pbody')), 'r-board:false', 'deep is back on the board, unwrapped');
      done.push('drop on ⌂ unpins (narrow)');
      assert.deepEqual(errors, [], 'no page errors (narrow live)');
      await context.close();
    }

    // ---- closed and missing docked topics (wide, column) ----
    {
      const context = await browser.newContext({ viewport: { width: 1800, height: 900 }, locale: 'en-US', reducedMotion: 'reduce' });
      await seed(context, 'livesession', { cards: [], topics: [{ n: 'archive000', p: 1, a: 0, b: 0 }, { n: 'ghost', p: 2, a: 0, b: 0 }, { n: 'gamma', p: 3, a: 0, b: 0 }] });
      const closedCalls = [];
      context.on('request', r => { const u = new URL(r.url()); if (u.pathname === '/s/livesession/closed') closedCalls.push(u.searchParams.get('from')); });
      const { tab, page, errors } = await open(context, 'livesession');
      await until(async () => JSON.stringify(await pages(page)) === '[["archive000"],["gamma"]]', 'missing topic fetched, gone topic dropped', 15000);
      assert(closedCalls.length >= 1, 'the missing docked topic is fetched through the closed route: ' + closedCalls);
      const rec = await page.evaluate(() => window.__gal.rec());
      assert.deepEqual(rec.topics.filter(t => t.p).map(t => [t.n, t.p]).sort(), [['archive000', 1], ['gamma', 2]], 'ghost leaves the record quietly');
      assert.equal(await page.locator('#bgal section.topic[data-topic="archive000"].closed').count(), 1, 'a closed docked topic sits in its pane');
      assert.equal(await page.locator('#r-board section.topic[data-topic="archive000"]:not(.closed-slot)').count(), 0, 'no duplicate on the board');
      await fetch(control + '/close-gamma');
      await until(() => page.locator('#bgal section.topic[data-topic="gamma"].closed').count().then(n => n === 1), 'gamma closes in its pane');
      assert.deepEqual(await pages(page), [['archive000'], ['gamma']], 'a docked topic that closes keeps its slot');
      await dot(page, 1).click();
      const fold = await page.evaluate(() => {
        const s = document.querySelector('#bgal section.topic[data-topic="gamma"]'), b = s.querySelector(':scope > .gal-pbody'), p = s.parentNode.getBoundingClientRect(), r = s.getBoundingClientRect();
        return { body: !!b, overflow: b && getComputedStyle(b).overflowY, fills: Math.abs(r.height - p.height) < 1.5 };
      });
      assert.deepEqual(fold, { body: true, overflow: 'auto', fills: true }, 'a closed docked topic keeps its head over a scrolling body that fills the pane');
      await shot(tab, 'wide-closed-docked');
      done.push('closed docked topic keeps its slot; missing docked topic fetched via the closed route; confirmed-absent dropped with its page');
      assert.deepEqual(errors, [], 'no page errors (closed)');
      await context.close();
    }

    // ---- layout modes at real viewports ----
    {
      const context = await browser.newContext({ viewport: { width: 2400, height: 1000 }, locale: 'en-US', reducedMotion: 'reduce' });
      await seed(context, 'livesession', {});
      let { tab, page, errors } = await open(context, 'livesession');
      const layout = () => page.evaluate(() => {
        const r = document.documentElement, g = document.getElementById('bgal'), b = document.getElementById('bcol').getBoundingClientRect();
        const gb = g.getBoundingClientRect(), edge = g.querySelector('.bp-edge');
        return { gal: r.getAttribute('data-gal'), pincol: r.hasAttribute('data-pincol'), hidden: g.hidden, cols: getComputedStyle(document.querySelector('.dbody')).gridTemplateColumns.split(' ').length,
          edge: !!edge && getComputedStyle(edge).display !== 'none', right: Math.round(b.right), galLeft: Math.round(gb.left), w: innerWidth };
      });
      let l = await layout();
      assert.deepEqual([l.gal, l.pincol, l.hidden, l.edge], [null, false, true, false], '2400 with no page: no column, no handle');
      const zeroCols = l.cols;
      await shot(tab, 'layout-2400-zero');
      // A drag start on the wide view shows the rail over the right edge; + docks.
      const wide = await drag(tab, await grip(bar(page, 'beta')), () => plus(page), async () => {
        await until(() => page.locator('#bgal').isVisible(), 'wide rail on drag start');
        return page.evaluate(() => document.documentElement.getAttribute('data-gal'));
      });
      assert.equal(wide, 'drag', 'the wide drag with no page overlays the right edge');
      await until(async () => JSON.stringify(await pages(page)) === '[["beta"]]', 'wide drop on + docks beta');
      l = await layout();
      assert.deepEqual([l.gal, l.pincol, l.hidden, l.edge, l.cols], ['col', true, false, true, zeroCols + 1], '2400 with a page: the gallery column with its handle');
      assert(l.galLeft >= l.right, 'the gallery stands right of the board');
      done.push('drop on + with zero pages (wide drag overlay); 2400 zero pages reserves no column');
      await tab.setViewportSize({ width: 1800, height: 1000 });
      await until(async () => (await layout()).w === 1800, 'resize 1800');
      l = await layout();
      const shellPin = await tab.evaluate(() => document.documentElement.hasAttribute('data-pincol'));
      assert.deepEqual([l.gal, l.pincol, shellPin], ['col', true, true], '1800 with a page: the column, the session rail overlays (shell data-pincol)');
      await shot(tab, 'layout-1800-page');
      await tab.setViewportSize({ width: 1400, height: 1000 });
      await until(async () => (await layout()).w === 1400, 'resize 1400');
      l = await layout();
      assert.deepEqual([l.gal, l.pincol], ['narrow', false], '1400: the narrow rail over the board column');
      await shot(tab, 'layout-1400-page');
      assert.deepEqual(errors, []);
      const kept = JSON.parse(await page.evaluate(() => localStorage.getItem('ws.dashboard.cards.livesession')));
      await tab.close();
      // tabs: narrow at any width.
      await seed(context, 'livesession', kept, 'tabs');
      ({ tab, page, errors } = await open(context, 'livesession', 2400, 1000));
      await until(async () => (await layout()).gal === 'narrow' || (await layout()).gal === null, 'tabs layout');
      if (await page.evaluate(() => getComputedStyle(document.getElementById('bcol')).display === 'none')) await page.locator('#tabbar [data-tab="board"]').click();
      l = await until(async () => { const v = await layout(); return v.gal === 'narrow' ? v : null; }, 'tabs at 2400 is narrow');
      assert.equal(l.pincol, false, 'tabs never takes the column');
      await shot(tab, 'layout-2400-tabs');
      done.push('layout modes: 1400 narrow, 1800 column + rail overlay, 2400 zero pages no column, tabs narrow at 2400');
      assert.deepEqual(errors, []);
      await context.close();
    }

    // ---- touch narrow: tap the dots and the pin button ----
    {
      const context = await browser.newContext({ viewport: { width: 420, height: 860 }, hasTouch: true, isMobile: true, locale: 'en-US', reducedMotion: 'reduce' });
      await seed(context, 'livesession', { cards: [], topics: [{ n: 'alpha', p: 1, a: 0, b: 0 }, { n: 'gamma', p: 2, a: 0, b: 0 }] });
      const { tab, page, errors } = await open(context, 'livesession');
      if (await page.evaluate(() => getComputedStyle(document.getElementById('bcol')).display === 'none')) await page.locator('#tabbar [data-tab="board"]').tap();
      await until(() => page.locator('#bgal .gal-rail').isVisible(), 'touch rail');
      await dot(page, 1).tap();
      await until(async () => await pos(page) === 1, 'tap shows page 2');
      await dot(page, -1).tap();
      await until(async () => await pos(page) === -1, 'tap shows the Main Board');
      await bar(page, 'beta').locator('.tpin').tap();
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"],["gamma"],["beta"]]', 'tap on pin docks beta');
      assert.equal(await pos(page), 2, 'the tapped pin shows its page');
      await shot(tab, 'touch-420');
      done.push('touch-emulated narrow viewport: tapping dots and the pin button');
      assert.deepEqual(errors, []);
      await context.close();
    }

    // ---- the board-first page (document scroll): the Main Board away ----
    {
      const context = await browser.newContext({ viewport: { width: 1200, height: 900 }, locale: 'en-US', reducedMotion: 'reduce' });
      await seed(context, 'boardsession', { cards: [], topics: [{ n: 'alpha', p: 1, a: 0, b: 0 }] });
      const { tab, page, errors } = await open(context, 'boardsession');
      await until(() => page.locator('#bgal .gal-rail').isVisible(), 'board page rail');
      if (await pos(page) !== -1) await dot(page, -1).click();
      await page.evaluate(() => window.scrollTo(0, 200));
      const y = await page.evaluate(() => window.scrollY);
      await dot(page, 0).click();
      await until(async () => await pos(page) === 0, 'board page: page 1');
      await page.evaluate(() => window.__gal.frames());
      const seen = await page.evaluate(() => window.__gal.seen([...document.querySelectorAll('#r-board > section.topic:not(.closed)')].slice(0, 3)));
      assert.deepEqual(seen, [false, false, false], 'board-first page: the Main Board out of view behind a page');
      assert.equal(await page.evaluate(() => window.scrollY), y, 'the document keeps its offset');
      done.push('board-first page (document scroll): Main Board non-intersecting behind a page');
      assert.deepEqual(errors, []);
      await context.close();
    }
    console.log(done.map(d => 'ok - ' + d).join('\n'));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
