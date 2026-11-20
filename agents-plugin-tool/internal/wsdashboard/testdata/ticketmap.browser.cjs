// Run by TestTicketMapBrowser: the ticket map overlay and View B on a real
// framed page over the synthetic board (the ticket's Phase 2 Playwright items
// 1-14), at 1600x960 and 1280x800 in both themes, plus 900x700 for the narrow
// checks. With a shots directory it also keeps the overlay's states beside the
// approved mockup's same states (mock-b-*) for the side-by-side check.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, graphFile, epicStem, shots, mockup] = process.argv.slice(2);

const G = JSON.parse(fs.readFileSync(graphFile, 'utf8'));
const byStem = new Map(G.nodes.map(n => [n.stem, n]));
const epic = byStem.get(epicStem);
assert.ok(epic, 'the fixture plants the epic');
// The epic's distinct explicit neighbours (what hover lights).
const epicNb = new Set();
for (const e of G.edges) {
  if (e[0] === e[1] || !['parent', 'blocked-by', 'related'].includes(e[2])) continue;
  if (e[0] === epic.i) epicNb.add(e[1]);
  if (e[1] === epic.i) epicNb.add(e[0]);
}
const laneIds = [...new Set(G.nodes.map(n => n.cluster))].sort((a, b) => a - b);
const SIZES = [[1600, 960], [1280, 800]];

function shot(page, name) {
  if (!shots) return Promise.resolve();
  fs.mkdirSync(shots, { recursive: true });
  return page.screenshot({ path: path.join(shots, name + '.png') });
}
const near = (a, b, tol, msg) => assert.ok(Math.abs(a - b) <= tol, `${msg}: ${a} vs ${b} (±${tol})`);

// frameOf is the shell's shown page frame.
async function frameOf(page) {
  const h = await page.locator('iframe.sframe.on').elementHandle();
  return h.contentFrame();
}
const raf2 = fr => fr.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));

// handles reads #thandle, #shandle and the header room (item 1).
function handles(fr) {
  return fr.evaluate(() => {
    const b = id => { const e = document.getElementById(id); if (!e || e.hidden) return null; const r = e.getBoundingClientRect(); return r.width ? { l: r.left, r: r.right, t: r.top, w: r.width } : null; };
    const top = document.querySelector('.dwrap > .top'), cs = getComputedStyle(document.documentElement);
    const px = v => parseFloat(v) || 0;
    return { t: b('thandle'), s: b('shandle'), d: b('drhandle'), room: top ? top.classList.contains('dr-room') : null,
      pad: top ? px(getComputedStyle(top).paddingRight) : null, drhw: px(cs.getPropertyValue('--drhw')), tkhw: px(cs.getPropertyValue('--tkhw')) };
  });
}

// wireCheck measures the hierarchy's connectors (item 7): one wire per
// visible hierarchy card, each with its group's marker, longer than 10px,
// and every endpoint within 2px of its anchor (verify.js anchorErr: the
// focal top and bottom by edge distance plus an x-range check, the focal
// right and the cards by their side's center).
function wireCheck(fr) {
  return fr.evaluate(() => {
    const vis = e => e.offsetParent !== null;
    const hier = document.getElementById('tmhier'), svg = document.getElementById('tmhwire');
    const cards = [...hier.querySelectorAll('.d-tm-xc')].filter(vis);
    const hero = document.getElementById('tmhero').getBoundingClientRect();
    const MK = { parent: 'tmmkp', child: 'tmmkp', 'blocked-by': 'tmmkw', blocks: 'tmmkw', related: 'tmmkr' };
    const errs = [], seen = new Map(), anchors = [];
    function rectOf(id) {
      if (id === 'focal') return hero;
      const c = cards.find(c => c.getAttribute('data-pivot') === id);
      return c ? c.getBoundingClientRect() : null;
    }
    function anchorErr(p, r, side, focal) {
      if (focal && (side === 'top' || side === 'bottom')) {
        const y = side === 'top' ? r.top : r.bottom;
        return Math.max(Math.abs(p.y - y), r.left - p.x, p.x - r.right, 0);
      }
      const cx = (r.left + r.right) / 2, cy = (r.top + r.bottom) / 2;
      const a = side === 'top' ? [cx, r.top] : side === 'bottom' ? [cx, r.bottom] : side === 'left' ? [r.left, cy] : [r.right, cy];
      return Math.hypot(p.x - a[0], p.y - a[1]);
    }
    const paths = [...svg.querySelectorAll('path.d-tm-w')];
    for (const p of paths) {
      const g = p.getAttribute('data-g'), from = p.getAttribute('data-from'), to = p.getAttribute('data-to');
      const fs = p.getAttribute('data-fs'), ts = p.getAttribute('data-ts');
      const len = p.getTotalLength(), m = p.getScreenCTM();
      const a = new DOMPoint(p.getPointAtLength(0).x, p.getPointAtLength(0).y).matrixTransform(m);
      const b = new DOMPoint(p.getPointAtLength(len).x, p.getPointAtLength(len).y).matrixTransform(m);
      const mk = (p.getAttribute('marker-end') || '').replace(/^url\(#(.*)\)$/, '$1');
      if (mk !== MK[g] || !document.getElementById(mk)) errs.push(`${g} ${from}->${to}: marker ${mk}`);
      if (!(len > 10)) errs.push(`${g} ${from}->${to}: length ${len}`);
      const card = from === 'focal' ? to : from;
      seen.set(card, (seen.get(card) || 0) + 1);
      const rf = rectOf(from), rt = rectOf(to);
      if (!rf || !rt) { errs.push(`${g} ${from}->${to}: endpoint card missing`); continue; }
      const ea = anchorErr(a, rf, fs, from === 'focal'), eb = anchorErr(b, rt, ts, to === 'focal');
      anchors.push([g, fs, ts, (to === 'focal' ? rf : rt).top]);
      if (ea > 2) errs.push(`${g} ${from}(${fs})->${to}: start off by ${ea.toFixed(2)}`);
      if (eb > 2) errs.push(`${g} ${from}->${to}(${ts}): end off by ${eb.toFixed(2)}`);
    }
    for (const c of cards) {
      const id = c.getAttribute('data-pivot');
      if (seen.get(id) !== 1) errs.push(`card ${id} (${c.getAttribute('data-g')}) has ${seen.get(id) || 0} wires`);
    }
    return { wires: paths.length, cards: cards.length, errs, anchors };
  });
}
async function wiresOk(fr, label) {
  await raf2(fr);
  const w = await wireCheck(fr);
  assert.deepEqual(w.errs, [], `${label}: connectors`);
  assert.equal(w.wires, w.cards, `${label}: one wire per visible hierarchy card`);
  assert.ok(w.cards > 0, `${label}: the hierarchy shows cards`);
  return w;
}
function overflow(fr) {
  return fr.evaluate(() => {
    const d = document.documentElement, vb = document.getElementById('tmvb');
    return { doc: d.scrollWidth - d.clientWidth, vb: vb.scrollWidth - vb.clientWidth };
  });
}
const heroStem = fr => fr.evaluate(() => { const e = document.querySelector('#tmhero .d-tm-stem'); return e ? e.textContent : null; });
const hist = fr => fr.evaluate(() => ({ hist: window.__tmap.hist.slice(), hp: window.__tmap.hp, sel: window.__tmap.sel }));
const isOpen = fr => fr.evaluate(() => { const b = document.getElementById('tmback'); return !!b && !b.hidden; });
async function pivot(fr, sel, label) {
  const target = fr.locator(sel).first();
  const i = +(await target.getAttribute('data-pivot'));
  await target.click();
  await raf2(fr);
  assert.equal(await heroStem(fr), G.nodes[i].stem, `${label}: pivot focuses ${G.nodes[i].stem}`);
  return i;
}

async function overlaySession(browser, theme, W, H, narrow) {
  const tag = `${theme}/${W}x${H}`;
  const context = await browser.newContext({ viewport: { width: W, height: H }, colorScheme: theme });
  await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
  await context.addInitScript(() => {
    if (self !== top) return;
    const workers = navigator.serviceWorker;
    if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('No service workers in fixture'); };
  });
  const offsite = [];
  await context.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== base) { offsite.push(url.href); return route.abort(); }
    return route.continue();
  });
  const page = await context.newPage();
  // Item 14. The bare page fixture configures no tracker store, so the
  // tracker's GET /tracker/stores answers 404 on every page load, with or
  // without the map; that one URL (and the console line Chromium logs for
  // its 404) is the only failure filtered out.
  const tracker = u => new URL(u).pathname === '/tracker/stores';
  const errors = [];
  page.on('pageerror', e => errors.push('pageerror: ' + e.message));
  page.on('console', m => { if (m.type() === 'error' && !(m.location().url && tracker(m.location().url))) errors.push('console: ' + m.text()); });
  page.on('requestfailed', r => { if (new URL(r.url()).origin === base) errors.push('requestfailed: ' + r.url() + ' ' + (r.failure() || {}).errorText); });
  page.on('response', r => { if (r.status() >= 400 && !(r.status() === 404 && tracker(r.url()))) errors.push(`response ${r.status()}: ${r.url()}`); });
  let graphReqs = 0;
  page.on('request', r => { if (new URL(r.url()).pathname.endsWith('/tickets/graph')) graphReqs++; });

  await page.goto(base + '/s/livesession');
  const fr = await frameOf(page);
  await fr.locator('#thandle').waitFor({ state: 'visible' });
  await page.waitForTimeout(400);
  assert.equal(await fr.evaluate(() => document.documentElement.getAttribute('data-theme')), theme, `${tag}: page theme`);
  await fr.evaluate(() => {
    document.documentElement.setAttribute('data-test-hooks', '');
    // Keys must never reach the page under the overlay (item 11).
    window.__tmleak = [];
    document.addEventListener('keydown', e => { if (document.getElementById('tmback') && !document.getElementById('tmback').hidden) window.__tmleak.push(e.key); });
  });

  // 1. No graph request on page open; #thandle hangs 6px left of #shandle,
  // and the header keeps room for every handle.
  assert.equal(graphReqs, 0, `${tag}: page open requests no ticket graph`);
  let h = await handles(fr);
  assert.ok(h.t && h.s && h.d, `${tag}: #thandle, #shandle and #drhandle show ${JSON.stringify(h)}`);
  near(h.t.r, h.s.l - 6, 0.5, `${tag}: #thandle right edge`);
  near(h.t.t, h.s.t, 0.5, `${tag}: #thandle hangs level with #shandle`);
  assert.equal(h.room, true, `${tag}: header takes the handle room`);
  near(h.pad, h.drhw + h.tkhw + 6, 0.5, `${tag}: header right padding is --drhw + --tkhw + 6`);
  await fr.evaluate(() => { document.getElementById('drhandle').hidden = true; });
  await page.waitForTimeout(150);
  h = await handles(fr);
  assert.ok(!h.d && h.t && h.s, `${tag}: drawer handle hidden`);
  near(h.t.r, h.s.l - 6, 0.5, `${tag}: #thandle without the drawer handle`);
  near(h.pad, h.drhw + h.tkhw + 6, 0.5, `${tag}: header padding without the drawer handle`);
  await fr.evaluate(() => { document.getElementById('drhandle').hidden = false; });
  await page.waitForTimeout(150);
  h = await handles(fr);
  near(h.t.r, h.s.l - 6, 0.5, `${tag}: #thandle with the drawer handle back`);

  // The drawer is open when the map opens: the map closes it (item 11).
  await fr.locator('#drhandle').click();
  await fr.waitForFunction(() => !!document.querySelector('.drawer.open'));

  // 2. One request, a viewport-covering overlay, View B first.
  await fr.locator('#thandle').click();
  await fr.locator('#tmvb .d-tm-card').first().waitFor({ state: 'visible' });
  await raf2(fr);
  assert.equal(graphReqs, 1, `${tag}: the first open sends one request`);
  assert.equal(await fr.evaluate(() => !!document.querySelector('.drawer.open')), false, `${tag}: the map closed the drawer`);
  const cover = await fr.evaluate(() => { const r = document.getElementById('tmback').getBoundingClientRect(); return { l: r.left, t: r.top, w: r.width, h: r.height, vw: innerWidth, vh: innerHeight }; });
  assert.deepEqual([cover.l, cover.t, cover.w, cover.h], [0, 0, cover.vw, cover.vh], `${tag}: the overlay covers the viewport`);
  assert.equal(await fr.locator('#thandle').getAttribute('aria-expanded'), 'true');
  assert.equal(await fr.locator('#tmviewb').getAttribute('aria-pressed'), 'true', `${tag}: View B first`);
  assert.equal(await fr.evaluate(() => window.__tmap && window.__tmap.view), 'B');

  // 12. The framed page dims the shell, and the overlay owns its top-left.
  await page.waitForFunction(() => document.documentElement.hasAttribute('data-fdim'));
  assert.equal(await fr.evaluate(() => document.getElementById('tmback').contains(document.elementFromPoint(1, 1))), true, `${tag}: the overlay's top-left pixel`);

  // 3. Lanes, cards, card grammar, clamp, no horizontal overflow.
  const mx = await fr.evaluate(() => {
    const cards = [...document.querySelectorAll('#tmvb .d-tm-card[data-i]')];
    return {
      lanes: [...document.querySelectorAll('#tmvb .d-tm-lane[data-cl]')].filter(l => !l.hidden).map(l => +l.getAttribute('data-cl')).sort((a, b) => a - b),
      stems: cards.map(c => c.title).sort(),
      kids: [...new Set(cards.map(c => [...c.children].map(k => k.className).join(' ')))],
      clamp: [...new Set(cards.map(c => getComputedStyle(c.querySelector('.d-tm-ctt')).webkitLineClamp))]
    };
  });
  assert.deepEqual(mx.lanes, laneIds, `${tag}: every cluster lane renders`);
  assert.deepEqual(mx.stems, G.nodes.map(n => n.stem).sort(), `${tag}: every node has a card`);
  assert.deepEqual(mx.kids, ['d-tm-ctop d-tm-ctt'], `${tag}: card children are exactly the top row and the title`);
  assert.deepEqual(mx.clamp, ['3'], `${tag}: title clamp`);
  assert.deepEqual(await overflow(fr), { doc: 0, vb: 0 }, `${tag}: no horizontal overflow in the matrix`);

  // 4. Hover lights neighbours; '/', a query with a hit count, Esc clears.
  const epicCard = fr.locator(`#tmvb .d-tm-card[title="${epicStem}"]`);
  await epicCard.scrollIntoViewIfNeeded();
  await epicCard.hover();
  await page.waitForTimeout(300);
  const lit = await fr.evaluate(i => {
    const hov = [...document.querySelectorAll('#tmvb .d-tm-hov')].map(c => +c.getAttribute('data-i'));
    const lit = [...document.querySelectorAll('#tmvb .d-tm-card.d-tm-lit')].map(c => +c.getAttribute('data-i')).sort((a, b) => a - b);
    const other = [...document.querySelectorAll('#tmvb .d-tm-card:not(.d-tm-lit):not(.d-tm-hov)')].find(c => c.offsetParent);
    return { hov, lit, faded: other ? getComputedStyle(other).opacity : null };
  }, epic.i);
  assert.deepEqual(lit.hov, [epic.i], `${tag}: the hovered card`);
  assert.deepEqual(lit.lit, [...epicNb].sort((a, b) => a - b), `${tag}: hover lights the explicit neighbours`);
  assert.equal(lit.faded, '0.5', `${tag}: hover fades the others`);
  await page.mouse.move(2, H - 2);
  await fr.evaluate(() => document.activeElement && document.activeElement.blur());
  await page.keyboard.press('/');
  assert.equal(await fr.evaluate(() => document.activeElement.id), 'tmq', `${tag}: '/' focuses the search`);
  await page.keyboard.type('orchard');
  await page.waitForTimeout(100);
  const q = await fr.evaluate(() => ({
    count: document.getElementById('tmqc').textContent, hits: window.__tmap.hits ? window.__tmap.hits.size : -1,
    shown: [...document.querySelectorAll('#tmvb .d-tm-card')].filter(c => !c.hidden).length,
    rings: document.querySelectorAll('#tmvb .d-tm-card.d-tm-hit:not([hidden])').length, view: window.__tmap.view
  }));
  assert.ok(/\d/.test(q.count) && q.hits > 0 && q.hits < G.nodes.length, `${tag}: a hit count ${JSON.stringify(q)}`);
  assert.equal(q.shown, q.hits, `${tag}: the query filters the cards`);
  assert.equal(q.rings, q.hits, `${tag}: hits get a ring`);
  assert.equal(q.view, 'B', `${tag}: typing never switches views`);
  await page.keyboard.press('Escape');
  assert.equal(await fr.inputValue('#tmq'), '', `${tag}: Esc clears the query`);
  assert.equal(await fr.evaluate(() => [...document.querySelectorAll('#tmvb .d-tm-card')].filter(c => !c.hidden).length), G.nodes.length);
  assert.equal(await isOpen(fr), true, `${tag}: clearing the query keeps the overlay`);
  await fr.evaluate(() => { document.getElementById('tmvb').scrollTop = 0; });
  await raf2(fr);
  await shot(page, `b-${theme}-${W}x${H}`);

  // 5. Click-focus on the epic: the matrix leaves the DOM, the inspector
  // stays hidden, and the three tiers keep their gaps.
  await epicCard.click();
  await raf2(fr);
  assert.equal(await heroStem(fr), epicStem);
  const tiers = await fr.evaluate(() => {
    const vis = e => e.offsetParent !== null, R = e => e.getBoundingClientRect();
    const hero = R(document.getElementById('tmhero')), vb = R(document.getElementById('tmvb')), stage = R(document.querySelector('.d-tm-stage'));
    const q = s => [...document.querySelectorAll(s)].filter(vis).map(R);
    return { mx: document.querySelectorAll('.d-tm-mx').length, insp: getComputedStyle(document.getElementById('tminsp')).display,
      vbW: vb.width, stageW: stage.width, hero: { t: hero.top, b: hero.bottom, r: hero.right },
      up: q('#tmhier .d-tm-up .d-tm-xc').map(r => r.bottom), down: q('#tmhier .d-tm-down .d-tm-xc').map(r => r.top), lat: q('#tmlat .d-tm-xc').map(r => r.left),
      excerpt: document.querySelectorAll('#tmvb [class*="excerpt"], #tmvb .d-tm-ex, #tmvb .d-tm-cex').length, cards: document.querySelectorAll('#tmvb .d-tm-card').length };
  });
  assert.equal(tiers.mx, 0, `${tag}: the matrix leaves the DOM`);
  assert.equal(tiers.insp, 'none', `${tag}: the inspector is hidden`);
  near(tiers.vbW, tiers.stageW, 0.5, `${tag}: #tmvb is full width`);
  assert.ok(tiers.up.length && tiers.down.length && tiers.lat.length, `${tag}: every tier shows ${JSON.stringify(tiers)}`);
  for (const b of tiers.up) assert.ok(b <= tiers.hero.t - 20, `${tag}: upstream ${b} >= 20px above the focal ${tiers.hero.t}`);
  for (const t of tiers.down) assert.ok(t >= tiers.hero.b + 20, `${tag}: downstream ${t} >= 20px below the focal ${tiers.hero.b}`);
  for (const l of tiers.lat) assert.ok(l >= tiers.hero.r + 20, `${tag}: related ${l} >= 20px right of the focal ${tiers.hero.r}`);
  assert.equal(tiers.excerpt + tiers.cards, 0, `${tag}: no excerpt nodes`);

  // 6. The semantic grid: descending, >= 15%, chips, column count.
  const sem = await fr.evaluate(() => {
    const cards = [...document.querySelectorAll('#tmsemgrid > .d-tm-sc')];
    return { pct: cards.map(c => +c.getAttribute('data-pct')), chips: cards.map(c => c.querySelector('.d-tm-schips').children.length),
      cols: getComputedStyle(document.getElementById('tmsemgrid')).gridTemplateColumns.split(' ').length };
  });
  assert.ok(sem.pct.length > 0, `${tag}: the semantic grid lists similar tickets`);
  assert.deepEqual(sem.pct, sem.pct.slice().sort((a, b) => b - a), `${tag}: sorted descending`);
  assert.ok(sem.pct.every(p => p >= 15), `${tag}: every % >= 15 ${sem.pct}`);
  assert.ok(sem.chips.every(c => c >= 1), `${tag}: every card has a chip`);
  assert.equal(sem.cols, W >= 1600 ? 4 : 3, `${tag}: semantic grid columns`);

  // 7. Connectors, again after +N and with the inspector open.
  const w0 = await wiresOk(fr, `${tag} focus`);
  await shot(page, `b-focus-${theme}-${W}x${H}`);
  const more = fr.locator('#tmhier .d-tm-down .d-tm-grp[data-g="child"] .d-tm-gmore');
  assert.equal(await more.textContent(), (await fr.evaluate(() => wsT('js.tmap.more', 3))), `${tag}: 11 children show 8 and +3`);
  await more.click();
  const w1 = await wiresOk(fr, `${tag} +N`);
  assert.equal(w1.cards, w0.cards + 3, `${tag}: +N adds the hidden children`);
  await fr.locator('#tmfinsp').click();
  await page.waitForTimeout(100);
  assert.equal(await fr.locator('#tmfinsp').getAttribute('aria-pressed'), 'true');
  assert.notEqual(await fr.evaluate(() => getComputedStyle(document.getElementById('tminsp')).display), 'none', `${tag}: the inspector opens`);
  await wiresOk(fr, `${tag} inspector`);
  await fr.locator('#tmfinsp').click();
  await page.waitForTimeout(100);
  await wiresOk(fr, `${tag} inspector closed`);

  // 8. Threshold and unlinked-only.
  const semCount = () => fr.evaluate(() => document.querySelectorAll('#tmsemgrid > .d-tm-sc').length);
  const setTh = v => fr.evaluate(v => { const e = document.getElementById('tmsemth'); e.value = String(v); e.dispatchEvent(new Event('input', { bubbles: true })); }, v);
  const at15 = await semCount();
  await setTh(10);
  assert.ok(await semCount() >= at15, `${tag}: threshold 10 shows at least as many as 15`);
  await setTh(15);
  assert.equal(await semCount(), at15);
  await fr.locator('#tmsemunl').click();
  assert.equal(await fr.locator('#tmsemunl').getAttribute('aria-pressed'), 'true');
  assert.equal(await fr.evaluate(() => document.querySelectorAll('#tmsemgrid .d-tm-lk-old').length), 0, `${tag}: unlinked-only shows no old badge`);
  await fr.locator('#tmsemall').click();
  assert.ok(await fr.evaluate(() => document.querySelectorAll('#tmsemgrid .d-tm-lk-old').length) > 0, `${tag}: all shows the linked ones again`);

  // 9. Pivots through an upstream, a downstream and a semantic card; Back,
  // Forward and the arrow key walk the history.
  const up = await pivot(fr, '#tmhier .d-tm-up .d-tm-xc', `${tag} upstream`);
  assert.deepEqual((await hist(fr)).hist, [epic.i, up]);
  await fr.locator('#tmfprev').click();
  assert.equal(await heroStem(fr), epicStem, `${tag}: Prev`);
  const down = await pivot(fr, '#tmhier .d-tm-down .d-tm-xc', `${tag} downstream`);
  assert.deepEqual((await hist(fr)).hist, [epic.i, down], `${tag}: a pivot truncates the forward entries`);
  const simI = await pivot(fr, '#tmsemgrid > .d-tm-sc', `${tag} semantic`);
  assert.deepEqual(await hist(fr), { hist: [epic.i, down, simI], hp: 2, sel: simI });
  await fr.locator('#tmfprev').click();
  assert.equal(await heroStem(fr), G.nodes[down].stem, `${tag}: Prev restores`);
  await fr.locator('#tmfnext').click();
  assert.equal(await heroStem(fr), G.nodes[simI].stem, `${tag}: Next restores`);
  assert.equal(await fr.locator('#tmfnext').isDisabled(), true, `${tag}: Next disabled at the end`);
  await page.keyboard.press('ArrowLeft');
  assert.equal(await heroStem(fr), G.nodes[down].stem, `${tag}: ArrowLeft restores`);
  await fr.locator('[data-hist="0"]').click();
  await raf2(fr);
  assert.equal(await heroStem(fr), epicStem, `${tag}: the trail restores`);
  assert.equal(await fr.locator('#tmfprev').isDisabled(), true, `${tag}: Prev disabled at the start`);

  // 10. Full View, Esc closing only it, the file-card hand-off, reopen
  // without a request, Refresh with one.
  await fr.locator('#tmffull').click();
  await fr.locator('#tmfull').waitFor({ state: 'visible' });
  assert.ok((await fr.locator('#tmfullb').textContent()).length > 50, `${tag}: Full View text`);
  await page.keyboard.press('Escape');
  assert.equal(await fr.locator('#tmfull').isHidden(), true, `${tag}: Esc closes Full View`);
  assert.equal(await isOpen(fr), true, `${tag}: Esc closes only Full View`);
  assert.equal(await heroStem(fr), epicStem, `${tag}: the focus stays`);
  assert.equal(await fr.evaluate(() => document.activeElement && document.activeElement.id), 'tmffull', `${tag}: keyboard focus returns`);
  await fr.locator('#tmffull').click();
  await fr.locator('#tmfullcard').waitFor({ state: 'visible' });
  const before = graphReqs;
  await fr.locator('#tmfullcard').click();
  assert.equal(await isOpen(fr), false, `${tag}: the hand-off hides the overlay`);
  await fr.waitForFunction(p => window.WsCards && window.WsCards.list('preview').some(c => c.data.path === p && c.el.isConnected && c.el.getClientRects().length > 0 &&
    c.body.querySelector('.pv-txt, .pv-md')), epic.path);
  assert.equal(await fr.evaluate(p => window.WsCards.list('preview').filter(c => c.data.path === p).length, epic.path), 1, `${tag}: one preview card for the ticket`);
  await fr.locator('#thandle').click();
  await fr.locator('#tmhero').waitFor({ state: 'visible' });
  assert.equal(await heroStem(fr), epicStem, `${tag}: reopen restores the focus`);
  assert.equal(graphReqs, before, `${tag}: reopen sends no request`);
  await fr.locator('#tmrefresh').click();
  await fr.waitForFunction(() => !document.getElementById('tmrefresh').disabled);
  await raf2(fr);
  assert.equal(graphReqs, before + 1, `${tag}: Refresh sends one request`);
  assert.equal(await heroStem(fr), epicStem, `${tag}: Refresh keeps the selection`);
  await wiresOk(fr, `${tag} after Refresh`);

  // 11. Esc layering: focus -> matrix -> closed; no key reached the page.
  await fr.evaluate(() => document.activeElement && document.activeElement.blur());
  await page.keyboard.press('Escape');
  assert.equal(await fr.evaluate(() => !!document.querySelector('#tmvb .d-tm-mx') && !document.getElementById('tmhero')), true, `${tag}: Esc returns to the matrix`);
  assert.equal(await isOpen(fr), true);
  await page.keyboard.press('Escape');
  assert.equal(await isOpen(fr), false, `${tag}: a further Esc closes the overlay`);
  assert.equal(await fr.locator('#thandle').getAttribute('aria-expanded'), 'false');
  assert.equal(await fr.evaluate(() => document.activeElement && document.activeElement.id), 'thandle', `${tag}: focus returns to #thandle`);
  assert.equal(await fr.evaluate(() => !!document.querySelector('.drawer.open')), false, `${tag}: the drawer state is unchanged`);
  assert.deepEqual(await fr.evaluate(() => window.__tmleak), [], `${tag}: no key reached the page`);
  await page.waitForFunction(() => !document.documentElement.hasAttribute('data-fdim'));

  // 13. Narrow: no overflow, Related below the focal card, wires within 2px.
  if (narrow) {
    const nt = `${theme}/900x700`;
    await page.setViewportSize({ width: 900, height: 700 });
    await page.waitForTimeout(200);
    await fr.locator('#thandle').click();
    await fr.locator('#tmvb .d-tm-mx').waitFor();
    await raf2(fr);
    assert.deepEqual(await overflow(fr), { doc: 0, vb: 0 }, `${nt}: no horizontal overflow in the matrix`);
    await shot(page, `b-${theme}-900x700`);
    await fr.locator(`#tmvb .d-tm-card[title="${epicStem}"]`).click();
    await raf2(fr);
    const n = await fr.evaluate(() => {
      const R = e => e.getBoundingClientRect(), hero = R(document.getElementById('tmhero'));
      return { hero: hero.bottom, lat: [...document.querySelectorAll('#tmlat .d-tm-xc')].filter(e => e.offsetParent).map(e => R(e).top) };
    });
    assert.ok(n.lat.length && n.lat.every(t => t >= n.hero + 20), `${nt}: Related sits below the focal card ${JSON.stringify(n)}`);
    const w = await wiresOk(fr, `${nt} focus`);
    // Related wraps here: like Tier 3, its first row is entered at the top
    // and a later row at the left edge, all from the focal bottom.
    const rel = w.anchors.filter(a => a[0] === 'related'), row1 = Math.min(...rel.map(a => a[3]));
    assert.ok(rel.length && rel.every(a => a[1] === 'bottom' && a[2] === (Math.abs(a[3] - row1) < 3 ? 'top' : 'left')),
      `${nt}: related wires leave the focal bottom ${JSON.stringify(rel)}`);
    assert.deepEqual(await overflow(fr), { doc: 0, vb: 0 }, `${nt}: no horizontal overflow in focus`);
    await shot(page, `b-focus-${theme}-900x700`);
    await page.keyboard.press('Escape');
    await page.keyboard.press('Escape');
    assert.equal(await isOpen(fr), false);
  }

  // 14. Zero console errors, page errors, or failed requests.
  await page.waitForTimeout(200);
  assert.deepEqual(errors, [], `${tag}: console errors, page errors, failed requests`);
  await context.close();
  return { tag, graphReqs, offsite: offsite.length };
}

// The approved mockup's same states, through its own tab and theme buttons;
// its focus is its first matrix card with the most links.
async function mockupShots(browser) {
  if (!mockup || !shots) return;
  const context = await browser.newContext({ viewport: { width: 1600, height: 960 } });
  const page = await context.newPage();
  for (const theme of ['dark', 'light']) {
    for (const [W, H] of SIZES) {
      await page.setViewportSize({ width: W, height: H });
      await page.goto(pathToFileURL(mockup).href);
      await page.locator('#vB .tc').first().waitFor();
      if (await page.evaluate(() => document.documentElement.dataset.theme) !== theme) await page.locator('#theme').click();
      await page.locator('#tabB').click();
      await page.waitForTimeout(300);
      await shot(page, `mock-b-${theme}-${W}x${H}`);
      const i = await page.evaluate(() => {
        let best = null;
        document.querySelectorAll('#vB .tc').forEach(el => {
          if (!el.offsetParent) return;
          const k = +el.dataset.i, n = window.__tmv.N[k].X.size;
          if (best === null || n > best.n) best = { k, n };
        });
        return best.k;
      });
      await page.locator(`#vB .tc[data-i="${i}"]`).first().click();
      await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
      await page.waitForTimeout(400);
      await shot(page, `mock-b-focus-${theme}-${W}x${H}`);
    }
  }
  await context.close();
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const report = [];
  try {
    await mockupShots(browser);
    for (const theme of ['dark', 'light']) {
      for (const [W, H] of SIZES) report.push(await overlaySession(browser, theme, W, H, W === 1280));
    }
    console.log(JSON.stringify({ nodes: G.nodes.length, lanes: laneIds.length, epic: epicStem, sessions: report }));
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e && e.stack || e); process.exit(1); });
