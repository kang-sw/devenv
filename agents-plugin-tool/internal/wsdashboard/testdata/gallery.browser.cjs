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

    // ---- Phase 2 (wide 2400): the 2D split tree ----
    // Five-zone drops with their preview, edge splits from the board and
    // from other pages, moves within a page, the center swap of two docked
    // panes, the dividers (drag, clamp, keys, double click, kept across a
    // reload), unpinning one pane out of a split and the last pane's page
    // going; the docked topics keep their nodes and frame documents
    // throughout.
    {
      const context = await browser.newContext({ viewport: { width: 2400, height: 1000 }, locale: 'en-US', reducedMotion: 'reduce' });
      await seed(context, 'livesession', { cards: [], topics: [{ n: 'alpha', p: 1, a: 0, b: 0 }, { n: 'beta', p: 2, a: 0, b: 0 }, { n: 'deep', p: 3, a: 0, b: 0 }] });
      let { tab, page, errors } = await open(context, 'livesession');
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha"],["beta"],["deep"]]', 'three seeded pages');
      // The session page stands in a frame: in-frame rects plus its offset
      // are the mouse's coordinates.
      const off = async () => { const b = await tab.locator('iframe.sframe.on').boundingBox(); return { x: b.x, y: b.y }; };
      const paneRect = name => page.evaluate(n => { const p = document.querySelector(`#bgal .gal-pane[data-ref="t:${n}"]`), r = p.getBoundingClientRect(); return { left: r.left, top: r.top, width: r.width, height: r.height }; }, name);
      // at is a point of name's pane in zone z (an edge band, or the middle).
      const at = async (name, z) => {
        const r = await paneRect(name), o = await off();
        const fx = z === 'left' ? 0.08 : z === 'right' ? 0.92 : 0.5, fy = z === 'top' ? 0.12 : z === 'bottom' ? 0.92 : 0.5;
        return { x: o.x + r.left + r.width * fx, y: o.y + r.top + r.height * fy };
      };
      // zoneSeen is the zone the hovered pane shows and whether the overlay
      // covers that zone's part of it (3px inset).
      const zoneSeen = name => page.evaluate(n => {
        const p = document.querySelector(`#bgal .gal-pane[data-ref="t:${n}"]`), z = p.getAttribute('data-dz'), ov = document.querySelector('#bgal .gal-overlay');
        if (!z || !ov || ov.hidden) return { z, ok: false };
        const r = p.getBoundingClientRect(), o = ov.getBoundingClientRect();
        const w = z === 'left' || z === 'right' ? r.width / 2 : r.width, h = z === 'top' || z === 'bottom' ? r.height / 2 : r.height;
        const want = { left: r.left + (z === 'right' ? r.width - w : 0) + 3, top: r.top + (z === 'bottom' ? r.height - h : 0) + 3, width: w - 6, height: h - 6 };
        const ok = ['left', 'top', 'width', 'height'].every(k => Math.abs(o[k] - want[k]) < 1.5);
        return { z, ok, others: document.querySelectorAll('#bgal .gal-pane[data-dz]').length };
      }, name);
      const tree = () => page.evaluate(() => {
        const g = window.__gal.rec().gallery, i = g.pages.findIndex(p => p.id === g.active);
        const show = n => n.axis ? { [n.axis]: [show(n.a), show(n.b)] } : (n.topic || n.card);
        return show(g.pages[i < 0 ? 0 : i].root);
      });
      // domTree reads the shown page's DOM the same way, with each split's
      // divider between its children.
      const domTree = () => page.evaluate(() => {
        const pg = document.querySelector('#bgal .gal-page[data-active]:not(.gal-plus)');
        const show = el => el.classList.contains('gal-split')
          ? (el.children.length === 3 && el.children[1].classList.contains('gal-div') ? { [el.classList.contains('row') ? 'row' : 'col']: [show(el.children[0]), show(el.children[2])] } : 'bad split')
          : el.getAttribute('data-ref').slice(2);
        return pg.children.length === 1 ? show(pg.firstElementChild) : 'bad page';
      });
      // keep records each docked topic's node and mounted body frame (a
      // load counter on the element, and a mark set inside its document
      // through Playwright, the frames being cross-origin) and tells whether
      // all recorded so far still hold them: same nodes, connected, the
      // same frame document (not detached, mark intact), no load since.
      const marks = new Map();
      const keep = async () => {
        const r = await page.evaluate(() => {
          const k = window.__keepT || (window.__keepT = {});
          const bad = [];
          Object.keys(k).forEach(n => {
            const r = k[n];
            if (!r.sec.isConnected || (r.f && (!r.f.isConnected || r.f.__loads))) bad.push(n);
          });
          document.querySelectorAll('#bgal .gal-pane > section.topic[data-topic]').forEach(sec => {
            const n = sec.getAttribute('data-topic'), f = sec.querySelector('iframe.doc-frame:not(.doc-frame-next)');
            if (k[n]) { if (k[n].sec !== sec || (f && k[n].f && k[n].f !== f)) bad.push(n + ' (node)'); if (!k[n].f && f) { k[n].f = f; f.__loads = 0; f.addEventListener('load', () => { f.__loads++; }); } return; }
            if (f) { f.__loads = 0; f.addEventListener('load', () => { f.__loads++; }); }
            k[n] = { sec, f };
          });
          return { bad, kept: Object.keys(k).sort(), framed: Object.keys(k).filter(n => k[n].f) };
        });
        for (const n of r.framed) {
          const h = await page.locator(`#bgal section.topic[data-topic="${n}"] iframe.doc-frame:not(.doc-frame-next)`).first().elementHandle();
          const f = h && await h.contentFrame();
          const m = marks.get(n);
          if (!m) {
            if (f) { await f.evaluate(t => { window.__galMark = t; }, 'm-' + n); marks.set(n, f); }
            continue;
          }
          if (m.isDetached() || f !== m || await m.evaluate(() => window.__galMark) !== 'm-' + n) r.bad.push(n + ' (document)');
        }
        delete r.framed;
        r.marked = [...marks.keys()].sort();
        return r;
      };
      // forget stops keep watching names (they left the gallery).
      const forget = names => { names.forEach(n => marks.delete(n)); return page.evaluate(ns => { ns.forEach(n => { delete window.__keepT[n]; }); }, names); };
      // gamma closed above, so deep is the fourth docked topic.
      // Visit each page so its frames mount, then record them.
      // framed tells whether each docked topic's body frame is mounted (a
      // frame mounts once its page has shown).
      // deep renders no body frame here; its node is kept all the same.
      const framed = () => page.evaluate(() => [...document.querySelectorAll('#bgal .gal-pane > section.topic[data-topic]')].every(s => s.getAttribute('data-topic') === 'deep' || s.querySelector('iframe.doc-frame:not(.doc-frame-next)')));
      for (const i of [1, 2, 0]) {
        await dot(page, i).click();
        await until(async () => await pos(page) === i, 'visit ' + i);
        await until(() => page.evaluate(() => [...document.querySelectorAll('#bgal .gal-page[data-active] section.topic')].every(s => s.getAttribute('data-topic') === 'deep' || s.querySelector('iframe.doc-frame:not(.doc-frame-next)'))), 'page ' + i + ' frames', 15000);
      }
      await until(framed, 'docked frames mounted', 15000);
      let k = await keep();
      assert.deepEqual(k, { bad: [], kept: ['alpha', 'beta', 'deep'], marked: ['alpha', 'beta'] }, 'docked nodes and frames recorded, the frames\' documents marked');

      // A board item over a docked pane: each edge shows its zone's half,
      // the middle shows an edge (no center for a board item); the right
      // edge splits alpha in a row.
      const hovered = await drag(tab, await grip(bar(page, 'epsilon')), () => at('alpha', 'right'), async () => {
        const seen = {};
        for (const z of ['left', 'right', 'top', 'bottom', 'middle']) {
          const p = await at('alpha', z);
          await tab.mouse.move(p.x, p.y, { steps: 4 });
          seen[z] = await zoneSeen('alpha');
        }
        return seen;
      });
      for (const z of ['left', 'right', 'top', 'bottom']) assert.deepEqual(hovered[z], { z, ok: true, others: 1 }, 'the ' + z + ' edge zone and its overlay');
      assert(hovered.middle.ok && hovered.middle.z !== 'center', 'a board item gets no center zone: ' + JSON.stringify(hovered.middle));
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha","epsilon"],["beta"],["deep"]]', 'right edge splits alpha');
      assert.deepEqual([await tree(), await domTree()], [{ row: ['alpha', 'epsilon'] }, { row: ['alpha', 'epsilon'] }], 'a row split, rendered with its divider');
      assert.equal(await page.evaluate(() => document.querySelectorAll('#bgal [data-dz]').length + (document.querySelector('#bgal .gal-overlay').hidden ? 0 : 1)), 0, 'the preview clears on drop');
      assert.deepEqual(await page.evaluate(() => window.__gal.dots()), ['2*', '1', '1', '+'], 'the dot counts two panes');
      await until(() => page.evaluate(() => !!document.querySelector('#bgal section.topic[data-topic="epsilon"] iframe.doc-frame')), 'epsilon frame mounted');
      k = await keep();
      assert.deepEqual(k.bad, [], 'an edge split keeps the docked nodes and frames');
      done.push('five-zone preview over a docked pane (four edges for a board item, overlay geometry); right edge drop splits in a row');

      // Across pages: beta (page 2) dragged while the rail wheel turns back
      // to page 1 lands at alpha's bottom; its emptied page goes.
      await dot(page, 1).click();
      await until(async () => await pos(page) === 1, 'page 2');
      const railPt = await center(page.locator('#bgal .gal-rail .gal-rzone.up'));
      await drag(tab, await grip(bar(page, 'beta')), () => at('alpha', 'bottom'), async () => {
        await tab.mouse.move(railPt.x, railPt.y, { steps: 6 });
        await tab.mouse.wheel(0, -120);
        await until(async () => await pos(page) === 0, 'the rail wheel turns to page 1 mid-drag');
        const p = await at('alpha', 'bottom');
        await tab.mouse.move(p.x, p.y, { steps: 6 });
        return zoneSeen('alpha');
      });
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha","beta","epsilon"],["deep"]]', 'beta moves across pages to alpha\'s bottom');
      assert.deepEqual(await domTree(), { row: [{ col: ['alpha', 'beta'] }, 'epsilon'] }, 'bottom splits alpha in a column');
      assert.deepEqual(await page.evaluate(() => window.__gal.dots()), ['3*', '1', '+'], 'the emptied page left the rail');
      assert.deepEqual((await keep()).bad, [], 'a cross-page move keeps nodes and frames');
      done.push('pane move across pages (rail wheel mid-drag), bottom edge split, emptied page removed');

      // Within the page: epsilon to alpha's top.
      await drag(tab, await grip(bar(page, 'epsilon')), () => at('alpha', 'top'));
      await until(async () => JSON.stringify(await tree()) === JSON.stringify({ col: [{ col: ['epsilon', 'alpha'] }, 'beta'] }), 'epsilon moves to alpha\'s top');
      assert.deepEqual(await domTree(), { col: [{ col: ['epsilon', 'alpha'] }, 'beta'] }, 'the row collapsed, the top edge split alpha');
      assert.deepEqual((await keep()).bad, [], 'a move within the page keeps nodes and frames');
      done.push('pane move within a page (top edge), the vacated split collapses');

      // Left: deep (page 2) to beta's left; four panes on one page.
      await dot(page, 1).click();
      await until(async () => await pos(page) === 1, 'deep\'s page');
      await drag(tab, await grip(bar(page, 'deep')), () => at('beta', 'left'), async () => {
        await tab.mouse.move(railPt.x, railPt.y, { steps: 6 });
        await tab.mouse.wheel(0, -120);
        await until(async () => await pos(page) === 0, 'back to page 1 mid-drag');
      });
      await until(async () => JSON.stringify(await pages(page)) === '[["epsilon","alpha","deep","beta"]]', 'deep lands left of beta');
      assert.deepEqual(await domTree(), { col: [{ col: ['epsilon', 'alpha'] }, { row: ['deep', 'beta'] }] }, 'four panes');
      assert.deepEqual(await page.evaluate(() => window.__gal.dots()), ['4*', '+']);
      assert.deepEqual((await keep()).bad, [], 'four panes: nodes and frames kept');
      await shot(tab, 'wide-four-panes');
      done.push('left edge split from another page: four panes on one page, dot shows 4');

      // Center: a docked pane over another's middle swaps the two; the
      // preview covers the whole pane.
      const swapSeen = await drag(tab, await grip(bar(page, 'epsilon')), () => at('beta', 'middle'), async () => {
        const p = await at('beta', 'middle');
        await tab.mouse.move(p.x, p.y, { steps: 6 });
        return zoneSeen('beta');
      });
      assert.deepEqual(swapSeen, { z: 'center', ok: true, others: 1 }, 'the center zone previews the whole pane');
      await until(async () => JSON.stringify(await tree()) === JSON.stringify({ col: [{ col: ['beta', 'alpha'] }, { row: ['deep', 'epsilon'] }] }), 'center swaps epsilon and beta');
      assert.deepEqual(await domTree(), { col: [{ col: ['beta', 'alpha'] }, { row: ['deep', 'epsilon'] }] });
      assert.deepEqual((await keep()).bad, [], 'a swap keeps nodes and frames');
      done.push('center drop swaps two docked panes (whole-pane preview)');

      // Dividers: the root's (a column: row-resize) drags its ratio, clamps
      // at 0.1, nudges by the arrow keys, resets on a double click, and the
      // ratio survives a reload.
      const rootDiv = page.locator('#bgal .gal-page[data-active] > .gal-split > .gal-div');
      const divInfo = () => rootDiv.evaluate(v => ({ role: v.getAttribute('role'), orient: v.getAttribute('aria-orientation'), now: +v.getAttribute('aria-valuenow'), cursor: getComputedStyle(v).cursor, tab: v.tabIndex }));
      assert.deepEqual(await divInfo(), { role: 'separator', orient: 'horizontal', now: 50, cursor: 'row-resize', tab: 0 }, 'the root divider: a focusable horizontal separator');
      assert.equal(await page.locator('#bgal .gal-page[data-active] .gal-split.row > .gal-div').evaluate(v => getComputedStyle(v).cursor + ' ' + v.getAttribute('aria-orientation')), 'col-resize vertical', 'a row split\'s divider');
      const rootRatio = () => page.evaluate(() => { const g = window.__gal.rec().gallery; return g.pages[0].root.ratio; });
      const shares = () => page.evaluate(() => {
        const s = document.querySelector('#bgal .gal-page[data-active] > .gal-split'), a = s.children[0].getBoundingClientRect(), b = s.children[2].getBoundingClientRect();
        return a.height / (a.height + b.height);
      });
      const splitBox = async () => { const o = await off(); const r = await page.evaluate(() => { const b = document.querySelector('#bgal .gal-page[data-active] > .gal-split').getBoundingClientRect(); return { left: b.left, top: b.top, width: b.width, height: b.height }; }); return { ...r, left: r.left + o.x, top: r.top + o.y }; };
      let sb = await splitBox();
      const dv = await center(rootDiv);
      await tab.mouse.move(dv.x, dv.y);
      await tab.mouse.down();
      await tab.mouse.move(dv.x, sb.top + 3 + (sb.height - 6) * 0.3, { steps: 8 });
      await tab.mouse.up();
      let r0 = await rootRatio();
      assert(Math.abs(r0 - 0.3) < 0.01, 'the divider drag sets the ratio: ' + r0);
      assert(Math.abs(await shares() - 0.3) < 0.01, 'the panes follow the ratio');
      assert.equal((await divInfo()).now, 30);
      const dv2 = await center(rootDiv);
      await tab.mouse.move(dv2.x, dv2.y);
      await tab.mouse.down();
      await tab.mouse.move(dv2.x, sb.top - 50, { steps: 8 });
      await tab.mouse.up();
      assert.equal(await rootRatio(), 0.1, 'a drag past the end clamps at 0.1');
      await rootDiv.focus();
      await tab.keyboard.press('ArrowDown');
      await tab.keyboard.press('ArrowDown');
      assert(Math.abs(await rootRatio() - 0.2) < 1e-9, 'two ArrowDown presses nudge to 0.2');
      await tab.keyboard.press('ArrowUp');
      assert(Math.abs(await rootRatio() - 0.15) < 1e-9, 'ArrowUp nudges back');
      await tab.keyboard.press('ArrowLeft');
      assert(Math.abs(await rootRatio() - 0.15) < 1e-9, 'a column divider ignores ArrowLeft');
      await rootDiv.dblclick();
      assert.equal(await rootRatio(), 0.5, 'a double click resets to half');
      const dv3 = await center(rootDiv);
      sb = await splitBox();
      await tab.mouse.move(dv3.x, dv3.y);
      await tab.mouse.down();
      await tab.mouse.move(dv3.x, sb.top + 3 + (sb.height - 6) * 0.65, { steps: 8 });
      await tab.mouse.up();
      r0 = await rootRatio();
      assert(Math.abs(r0 - 0.65) < 0.01, 'dragged to 0.65: ' + r0);
      assert.deepEqual((await keep()).bad, [], 'divider drags keep nodes and frames');
      assert.deepEqual(errors, [], 'no page errors (split)');
      await tab.close();
      ({ tab, page, errors } = await open(context, 'livesession'));
      marks.clear();
      await until(async () => JSON.stringify(await pages(page)) === '[["beta","alpha","deep","epsilon"]]', 'the split page after a reload');
      assert.equal(await rootRatio(), r0, 'the ratio is kept across a reload');
      assert(Math.abs(await shares() - r0) < 0.01, 'and drawn');
      assert.deepEqual(await domTree(), { col: [{ col: ['beta', 'alpha'] }, { row: ['deep', 'epsilon'] }] }, 'the tree is kept across a reload');
      done.push('divider drag (ratio, clamp 0.1, arrow keys, double-click reset), persisted across reload');

      // Unpinning one pane: deep's header dropped on the Main Board takes
      // it back to the board and collapses its row; the others keep their
      // nodes and frames.
      if (await pos(page) !== 0) await dot(page, 0).click();
      await until(framed, 'frames mounted after the reload', 15000);
      k = await keep();
      assert.deepEqual(k, { bad: [], kept: ['alpha', 'beta', 'deep', 'epsilon'], marked: ['alpha', 'beta', 'epsilon'] }, 'docked frames recorded after the reload');
      // board is a point on the Main Board's open flow (the board column's middle).
      const board = () => center(page.locator('#bcol'));
      await drag(tab, await grip(bar(page, 'deep')), board);
      await until(async () => JSON.stringify(await pages(page)) === '[["beta","alpha","epsilon"]]', 'deep unpinned to the board');
      assert.equal(await topic(page, 'deep').evaluate(n => n.parentNode.id), 'r-board', 'deep stands on the Main Board');
      assert.deepEqual(await domTree(), { col: [{ col: ['beta', 'alpha'] }, 'epsilon'] }, 'deep\'s row collapsed into epsilon');
      assert.equal(await rootRatio(), r0, 'the root keeps its ratio');
      await forget(['deep']);
      assert.deepEqual((await keep()).bad, [], 'the remaining panes keep nodes and frames');
      assert.deepEqual(await page.evaluate(() => window.__gal.dots()), ['3*', '+']);
      done.push('header drag to the Main Board unpins one pane of a multi-pane page; its parent split collapses; others not remounted');

      // The pin button unpins the rest one by one; the last pane's page
      // goes.
      await bar(page, 'beta').locator('.tpin').click();
      await until(async () => JSON.stringify(await pages(page)) === '[["alpha","epsilon"]]', 'beta unpinned by its button');
      assert.deepEqual(await domTree(), { col: ['alpha', 'epsilon'] });
      await bar(page, 'alpha').locator('.tpin').click();
      await until(async () => JSON.stringify(await pages(page)) === '[["epsilon"]]', 'alpha unpinned by its button');
      assert.deepEqual(await domTree(), 'epsilon', 'a single pane fills the page');
      await forget(['beta', 'alpha']);
      assert.deepEqual((await keep()).bad, [], 'the last pane keeps its node and frame');
      await drag(tab, await grip(bar(page, 'epsilon')), board);
      await until(async () => JSON.stringify(await pages(page)) === '[]', 'the last pane\'s page goes');
      assert.equal(await page.evaluate(() => document.querySelectorAll('#bgal .gal-page:not(.gal-plus), #bgal .gal-split, #bgal .gal-pane, #bgal .gal-div').length), 0, 'no page, split, pane or divider left');
      done.push('last-pane page removal (pin button and header drag)');
      assert.deepEqual(errors, [], 'no page errors (split, reloaded)');
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
    // ---- captures for the side-by-side review with mockup v4 (opt-in) ----
    const captures = process.env.WS_GALLERY_CAPTURES, mockup = process.argv[4] || '';
    if (captures) {
      fs.mkdirSync(captures, { recursive: true });
      const two = { cards: [], topics: [{ n: 'alpha', p: 1, a: 0, b: 0 }, { n: 'deep', p: 2, a: 0, b: 0 }] };
      for (const [name, viewport, extra, home] of [
        ['live-wide-2400x1000', { width: 2400, height: 1000 }, {}, false],
        ['live-narrow-1200x900-page', { width: 1200, height: 900 }, {}, false],
        ['live-narrow-1200x900-home', { width: 1200, height: 900 }, {}, true],
        ['live-touch-420x860', { width: 420, height: 860 }, { hasTouch: true, isMobile: true }, false],
      ]) {
        const context = await browser.newContext({ viewport, ...extra, colorScheme: 'dark', locale: 'en-US', reducedMotion: 'reduce' });
        await seed(context, 'livesession', two);
        const { tab, page } = await open(context, 'livesession');
        if (await page.evaluate(() => getComputedStyle(document.getElementById('bcol')).display === 'none')) await page.locator('#tabbar [data-tab="board"]').click();
        await until(() => page.locator('#bgal .gal-rail').isVisible(), 'capture rail');
        const want = home ? -1 : 0;
        if (await pos(page) !== want) await dot(page, want).click();
        await until(async () => await pos(page) === want, 'capture page');
        await page.evaluate(() => window.__gal.frames());
        await tab.screenshot({ path: path.join(captures, name + '.png') });
        await context.close();
      }
      // Phase 2: two-pane and four-pane split pages at wide 2400x1000, the
      // shapes of mockup v4's split page (a column of two) and of that page
      // with a board item dropped on each pane's right edge; and the right
      // edge zone's preview mid-drag.
      const T = n => ({ topic: n }), S = (axis, a, b) => ({ axis, ratio: 0.5, a, b });
      const splitRec = root => ({ cards: [], topics: ['alpha', 'beta', 'epsilon', 'deep'].map((n, i) => ({ n, p: i + 1, a: 0, b: 0 })).filter(t => JSON.stringify(root).includes('"' + t.n + '"')),
        gallery: { v: 1, active: 'p1', pages: [{ id: 'p1', root }], rev: 1 } });
      for (const [name, root, preview] of [
        ['live-wide-2pane-2400x1000', S('col', T('alpha'), T('beta')), false],
        ['live-wide-4pane-2400x1000', S('col', S('row', T('alpha'), T('epsilon')), S('row', T('beta'), T('deep'))), false],
        ['live-wide-zone-right-2400x1000', S('col', T('alpha'), T('beta')), true],
      ]) {
        const context = await browser.newContext({ viewport: { width: 2400, height: 1000 }, colorScheme: 'dark', locale: 'en-US', reducedMotion: 'reduce' });
        await seed(context, 'livesession', splitRec(root));
        const { tab, page } = await open(context, 'livesession');
        await until(() => page.locator('#bgal .gal-rail').isVisible(), 'capture split rail');
        await until(() => page.evaluate(() => [...document.querySelectorAll('#bgal .gal-pane > section.topic[data-topic]')].every(s => s.getAttribute('data-topic') === 'deep' || s.querySelector('iframe.doc-frame')) && document.querySelectorAll('#bgal .gal-pane').length > 1), 'capture split frames', 15000);
        await page.evaluate(() => window.__gal.frames());
        if (preview) {
          const o = await tab.locator('iframe.sframe.on').boundingBox();
          const r = await page.evaluate(() => { const b = document.querySelector('#bgal .gal-pane[data-ref="t:alpha"]').getBoundingClientRect(); return { x: b.right - b.width * 0.08, y: b.top + b.height / 2 }; });
          const g = await grip(bar(page, 'epsilon'));
          await tab.mouse.move(g.x, g.y);
          await tab.mouse.down();
          await tab.mouse.move(g.x + 14, g.y + 14, { steps: 4 });
          await tab.mouse.move(o.x + r.x, o.y + r.y, { steps: 8 });
          await until(() => page.evaluate(() => document.querySelector('#bgal .gal-pane[data-ref="t:alpha"]').getAttribute('data-dz') === 'right'), 'capture zone preview');
        }
        await tab.screenshot({ path: path.join(captures, name + '.png') });
        if (preview) await tab.mouse.up();
        await context.close();
      }
      if (mockup) {
        const context = await browser.newContext({ viewport: { width: 1000, height: 640 }, locale: 'ko-KR', reducedMotion: 'reduce' });
        const tab = await context.newPage();
        await tab.goto('file://' + mockup);
        const proto = tab.locator('#galProtoV4');
        await tab.locator('[data-act="wide"]').click();
        await tab.locator('#galProtoV4 .gal-dot:not(.main):not(.empty)').nth(1).click();
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-wide-2pane.png') });
        // The mockup's own pane highlight for a right-edge hover (its
        // dragover sets data-dz; set here as a still).
        await tab.locator('#galProtoV4 .gal-pane').first().evaluate(p => p.setAttribute('data-dz', 'right'));
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-wide-zone-right.png') });
        await tab.locator('#galProtoV4 .gal-pane').first().evaluate(p => p.removeAttribute('data-dz'));
        // Its four-pane shape: a board item dropped on each pane's right edge.
        for (const i of [0, 1]) {
          const pane = tab.locator('#galProtoV4 .gal-tree .gal-pane').nth(i * 2), b = await pane.boundingBox();
          await tab.locator('#galProtoV4 .gal-bcard').first().dragTo(pane, { targetPosition: { x: b.width * 0.92, y: b.height / 2 } });
        }
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-wide-4pane.png') });
        await context.close();
      }
      if (mockup) {
        const context = await browser.newContext({ viewport: { width: 1000, height: 640 }, locale: 'ko-KR', reducedMotion: 'reduce' });
        const tab = await context.newPage();
        await tab.goto('file://' + mockup);
        const proto = tab.locator('#galProtoV4');
        await tab.locator('[data-act="wide"]').click();
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-wide.png') });
        await tab.locator('[data-act="narrow"]').click();
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-narrow-home.png') });
        await tab.locator('#galProtoV4 .gal-dot:not(.main):not(.empty)').first().click();
        await proto.screenshot({ path: path.join(captures, 'mockup-v4-narrow-page.png') });
        await context.close();
      }
      done.push('captures written to ' + captures);
    }
    console.log(done.map(d => 'ok - ' + d).join('\n'));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
