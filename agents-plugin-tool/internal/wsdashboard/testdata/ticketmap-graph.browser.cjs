// Run by TestTicketMapGraphBrowser: View C on a real framed page over the
// synthetic board (the ticket's Phase 3 Playwright items 1-17), at 1600x960
// and 1280x800 in both themes, item 16 in a touch context, and the
// Performance evidence (layout ms at 170 and 300 open tickets, wheel frame
// time, no request after the single fetch). With a shots directory it also
// keeps the c-*, ego-* and kw-filter-* states beside the approved mockup's
// same states (mock-c-*), reached through the mockup's own controls.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, graphFile, graph300File, epicStem, shots, mockup] = process.argv.slice(2);

const G = JSON.parse(fs.readFileSync(graphFile, 'utf8'));
const G300 = fs.readFileSync(graph300File);
const N300 = JSON.parse(G300.toString('utf8')).nodes.length;
const epicI = G.nodes.findIndex(n => n.stem === epicStem);
assert.ok(epicI >= 0, 'the fixture plants the epic');
// deg: distinct explicit neighbours, as the client derives it.
const nbr = G.nodes.map(() => new Set());
for (const e of G.edges) {
  if (e[0] === e[1] || !['parent', 'blocked-by', 'related'].includes(e[2])) continue;
  nbr[e[0]].add(e[1]);
  nbr[e[1]].add(e[0]);
}
const deg = nbr.map(s => s.size);
const clusterIds = G.clusters.map(c => c.id);
const bigCluster = clusterIds.map(id => [id, G.nodes.filter(n => n.cluster === id).length]).sort((a, b) => b[1] - a[1] || a[0] - b[0])[0][0];
const SIZES = [[1600, 960], [1280, 800]];
const ZOOMS = ['fit', 0.8, 1.0, 1.2, 1.4, 1.8, 2.6];

const sleep = ms => new Promise(r => setTimeout(r, ms));
const near = (a, b, tol, msg) => assert.ok(Math.abs(a - b) <= tol, `${msg}: ${a} vs ${b} (±${tol})`);
const median = a => { const s = a.slice().sort((x, y) => x - y); return s.length ? (s[(s.length - 1) >> 1] + s[s.length >> 1]) / 2 : NaN; };
const pct = (a, p) => { const s = a.slice().sort((x, y) => x - y); return s[Math.min(s.length - 1, Math.floor(p * s.length))]; };
const r2 = v => Math.round(v * 100) / 100;
const reEsc = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
function shot(page, name) {
  if (!shots) return Promise.resolve();
  fs.mkdirSync(shots, { recursive: true });
  return page.screenshot({ path: path.join(shots, name + '.png') });
}
// zoomAt is the camera at k2 keeping the screen point (mx, my) fixed.
function zoomAt(c, mx, my, k2) {
  const wx = (mx - c.x) / c.k, wy = (my - c.y) / c.k;
  return { x: mx - wx * k2, y: my - wy * k2, k: k2 };
}
// centroid is a soup word's anchor recomputed from the payload: its visible
// carrying tickets' settled positions, weighted max(.15, tw).
function centroid(item, base, visible) {
  let sx = 0, sy = 0, sw = 0;
  item.ticketIds.forEach((i, ix) => {
    if (!visible(i)) return;
    const w = Math.max(0.15, +(item.tw || [])[ix] || 0);
    sx += base[i][0] * w; sy += base[i][1] * w; sw += w;
  });
  return sw ? { x: sx / sw, y: sy / sw } : null;
}
function soupItem(cl, term) {
  const c = G.clusters.find(x => x.id === cl), names = new Set((c.nameTerms || []).map(t => t.toLowerCase()));
  return (G.clusterKeywords[String(cl)] || []).filter(x => !names.has(x.term.toLowerCase())).slice(0, 8).find(x => x.term === term);
}

// Frame helpers (window.__tmt), installed in every frame: the camera and
// probe stats, label collisions, and what lies under a viewport point.
function frameHelpers() {
  if (self === top) return;
  const R = e => e.getBoundingClientRect();
  const shown = e => !!e && !e.hidden && getComputedStyle(e).display !== 'none' && R(e).width > 0;
  const vpEl = () => document.getElementById('tmvp');
  window.__tmt = {
    vp() { const r = R(vpEl()); return { l: r.left, t: r.top, w: r.width, h: r.height }; },
    st() {
      const s = window.__tmapc.stats(), S = window.__tmap, c = S.cam;
      return Object.assign({}, s, { cam: { x: c.x, y: c.y, k: c.k }, sel: S.sel, ego: S.ego, kw: S.kw ? S.kw.term : null, view: S.view,
        lb: document.querySelectorAll('#tmlbl .d-tm-lb.on').length, sk: document.querySelectorAll('#tmlbl .d-tm-sk.on').length });
    },
    ctrls() {
      return [...['tmctool', 'tmchint', 'tmlegend', 'tmegobar'].map(id => document.getElementById(id)), document.querySelector('#tmvc .d-tm-zoom')]
        .filter(shown).map(e => ({ id: e.id || 'zoom', r: R(e) }));
    },
    // collide: every placed label, header, soup word and ego caption,
    // pairwise and against the overlay controls and the viewport.
    collide() {
      const items = [...document.querySelectorAll('#tmlbl .d-tm-lb.on, #tmlbl .d-tm-cb.on, #tmlbl .d-tm-sk.on, #tmlbl .d-tm-sect.on')].map(e => ({
        id: e.className.split(' ')[0] + ':' + (e.dataset.i || e.dataset.j || e.dataset.cl || e.textContent), r: R(e)
      }));
      const ov = (a, b) => Math.min(a.right, b.right) - Math.max(a.left, b.left) > 0.5 && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 0.5;
      const v = R(vpEl()), ctrls = this.ctrls(), pairs = [], ctrl = [], out = [], n = {};
      items.forEach((a, ix) => {
        const kind = a.id.split(':')[0];
        n[kind] = (n[kind] || 0) + 1;
        if (a.r.left < v.left - 0.5 || a.r.top < v.top - 0.5 || a.r.right > v.right + 0.5 || a.r.bottom > v.bottom + 0.5) out.push(a.id);
        for (let b = ix + 1; b < items.length; b++) if (ov(a.r, items[b].r)) pairs.push(a.id + ' x ' + items[b].id);
        ctrls.forEach(c => { if (ov(a.r, c.r)) ctrl.push(a.id + ' x ' + c.id); });
      });
      return { n, pairs, ctrl, out };
    },
    // at: what lies under viewport point (x, y), top first.
    at(x, y) {
      const v = R(vpEl()), els = document.elementsFromPoint(v.left + x, v.top + y), t = els[0];
      if (!t || !vpEl().contains(t)) return { k: 'ctrl' };
      let c = t.closest('.d-tm-cnd');
      if (c) return { k: 'node', i: +c.dataset.i };
      if ((c = t.closest('.d-tm-lb'))) return { k: 'label', i: +c.dataset.i };
      if ((c = t.closest('.d-tm-sk'))) {
        const u = els.find(e => e.closest && e.closest('.d-tm-cnd'));
        return { k: 'soup', j: +c.dataset.j, under: u ? +u.closest('.d-tm-cnd').dataset.i : null };
      }
      if (t.classList && t.classList.contains('d-tm-cehit')) return { k: 'edge' };
      return { k: 'bg' };
    },
    // screen: node i's viewport point at the live camera.
    screen(i) { const p = window.__tmapc.pos()[i], c = window.__tmap.cam; return { x: p[0] * c.k + c.x, y: p[1] * c.k + c.y }; },
    // nodes: shown stars whose center is the star itself (or, with
    // soupOk, a soup word over it: the star wins), 40px inside.
    nodes(soupOk) {
      const v = this.vp(), out = [];
      document.querySelectorAll('#tmworld .d-tm-cnd').forEach(g => {
        const i = +g.dataset.i;
        if (getComputedStyle(g).display === 'none' || getComputedStyle(g).opacity === '0') return;
        const p = this.screen(i);
        if (p.x < 40 || p.y < 40 || p.x > v.w - 40 || p.y > v.h - 40) return;
        const a = this.at(p.x, p.y);
        if ((a.k === 'node' && a.i === i) || (soupOk && a.k === 'soup' && a.under === i)) out.push({ i, x: p.x, y: p.y, soup: a.k === 'soup' });
      });
      return out;
    },
    // soups: placed soup words with no star under their center.
    soups() {
      const v = R(vpEl()), out = [];
      document.querySelectorAll('#tmlbl .d-tm-sk.on').forEach(e => {
        const r = R(e), x = (r.left + r.right) / 2 - v.left, y = (r.top + r.bottom) / 2 - v.top, a = this.at(x, y);
        if (a.k === 'soup' && a.j === +e.dataset.j && a.under === null) out.push({ j: +e.dataset.j, cl: +e.dataset.cl, term: e.textContent, x, y });
      });
      return out;
    },
    // bg: an empty-canvas point, searched outward from the center, whose
    // drag by (dx, dy) stays well inside the viewport.
    bg(dx, dy) {
      const v = this.vp();
      for (let rad = 0; rad < Math.max(v.w, v.h); rad += 17) {
        for (let a = 0; a < 16; a++) {
          const x = Math.round(v.w / 2 + rad * Math.cos(a * Math.PI / 8)), y = Math.round(v.h / 2 + rad * Math.sin(a * Math.PI / 8));
          if (Math.min(x, x + dx) < 60 || Math.min(y, y + dy) < 80 || Math.max(x, x + dx) > v.w - 60 || Math.max(y, y + dy) > v.h - 80) continue;
          if ([[0, 0], [6, 0], [-6, 0], [0, 6], [0, -6]].every(d => this.at(x + d[0], y + d[1]).k === 'bg')) return { x, y };
        }
      }
      return null;
    },
    // wheel dispatches one wheel event at viewport point (x, y) and
    // returns the drift of the world point under the handler's own anchor.
    wheel(x, y, dy, mode) {
      const vp = vpEl(), r = R(vp), c0 = Object.assign({}, window.__tmap.cam);
      let m = null;
      vp.addEventListener('wheel', e => { m = { x: e.clientX - r.left, y: e.clientY - r.top }; }, { once: true, capture: true });
      vp.dispatchEvent(new WheelEvent('wheel', { deltaY: dy, deltaMode: mode, clientX: r.left + x, clientY: r.top + y, bubbles: true, cancelable: true }));
      const c1 = Object.assign({}, window.__tmap.cam), wx = (m.x - c0.x) / c0.k, wy = (m.y - c0.y) / c0.k;
      return { c0, c1, drift: Math.hypot(wx * c1.k + c1.x - m.x, wy * c1.k + c1.y - m.y) };
    }
  };
}

const frameOf = async page => (await page.locator('iframe.sframe.on').elementHandle()).contentFrame();
const raf2 = fr => fr.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
const st = fr => fr.evaluate(() => window.__tmt.st());
const collide = fr => fr.evaluate(() => window.__tmt.collide());
const setCam = (fr, c) => fr.evaluate(c => window.__tmapc.setCam(c), c);
function clean(c, label) {
  assert.deepEqual(c.pairs, [], `${label}: label collisions`);
  assert.deepEqual(c.ctrl, [], `${label}: labels over the overlay controls`);
  assert.deepEqual(c.out, [], `${label}: labels outside the viewport`);
}

// newSession opens a framed page; err collects console errors, page
// errors and failed requests (item 17; the bare fixture's tracker store
// 404 is the only one filtered, as in the Phase 2 suite), reqs every
// request after mark().
async function newSession(browser, theme, W, H, extra) {
  const context = await browser.newContext(Object.assign({ viewport: { width: W, height: H }, colorScheme: theme }, extra || {}));
  await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
  await context.addInitScript(() => {
    if (self !== top) return;
    const workers = navigator.serviceWorker;
    if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('No service workers in fixture'); };
  });
  await context.addInitScript(frameHelpers);
  const s = { context, errors: [], graphReqs: 0, after: null, tag: `${theme}/${W}x${H}${extra && extra.hasTouch ? '/touch' : ''}`, W, H };
  await context.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== base && url.protocol !== 'file:') { s.errors.push('offsite: ' + url.href); return route.abort(); }
    return route.continue();
  });
  const page = s.page = await context.newPage();
  const tracker = u => new URL(u).pathname === '/tracker/stores';
  page.on('pageerror', e => s.errors.push('pageerror: ' + e.message));
  page.on('console', m => { if (m.type() === 'error' && !(m.location().url && tracker(m.location().url))) s.errors.push('console: ' + m.text()); });
  page.on('requestfailed', r => { if (new URL(r.url()).origin === base) s.errors.push('requestfailed: ' + r.url() + ' ' + (r.failure() || {}).errorText); });
  page.on('response', r => { if (r.status() >= 400 && !(r.status() === 404 && tracker(r.url()))) s.errors.push(`response ${r.status()}: ${r.url()}`); });
  page.on('request', r => {
    if (new URL(r.url()).pathname.endsWith('/tickets/graph')) s.graphReqs++;
    if (s.after) s.after.push(r.method() + ' ' + r.url());
  });
  return s;
}
// openC opens the overlay straight into View C (item 1's stored view).
async function openC(s) {
  const page = s.page;
  await page.goto(base + '/s/livesession');
  const fr = s.fr = await frameOf(page);
  await fr.locator('#thandle').waitFor({ state: 'visible' });
  await page.waitForTimeout(300);
  await fr.evaluate(() => {
    document.documentElement.setAttribute('data-test-hooks', '');
    localStorage.setItem('ws.ticketmap.view', 'C');
  });
  await fr.locator('#thandle').click();
  await fr.waitForFunction(() => window.__tmapc && window.__tmapc.stats().W > 0 && document.querySelectorAll('#tmworld .d-tm-cnd').length > 0);
  await raf2(fr);
  s.off = await page.locator('iframe.sframe.on').boundingBox();
  s.vp = await fr.evaluate(() => window.__tmt.vp());
  s.after = [];
  return fr;
}
// pt maps a viewport point to page coordinates.
const pt = (s, p) => ({ x: s.off.x + s.vp.l + p.x, y: s.off.y + s.vp.t + p.y });
async function away(s) {
  // The overlay's top bar: off the canvas, so the hover clears.
  await s.page.mouse.move(s.off.x + 4, s.off.y + 4);
  await raf2(s.fr);
}
async function drag(s, from, d, button) {
  const a = pt(s, from), m = s.page.mouse;
  await m.move(a.x, a.y);
  await m.down({ button: button || 'left' });
  for (let k = 1; k <= 6; k++) await m.move(a.x + d.x * k / 6, a.y + d.y * k / 6);
  await m.up({ button: button || 'left' });
  await raf2(s.fr);
}
async function key(s, k) {
  await s.fr.evaluate(() => document.activeElement && document.activeElement.blur());
  await s.page.keyboard.press(k);
}

async function viewC(browser, theme, W, H) {
  const s = await newSession(browser, theme, W, H);
  const { page, tag } = s;
  const fr = await openC(s);

  // 1. The stored view opens C; Fit is stage 1.
  let c = await st(fr);
  assert.equal(c.view, 'C', `${tag}: ws.ticketmap.view = C opens View C`);
  assert.equal(await fr.locator('#tmviewc').getAttribute('aria-pressed'), 'true');
  assert.equal(s.graphReqs, 1, `${tag}: one request`);
  assert.ok(c.cam.k < 0.85 && c.stage === 1, `${tag}: Fit is stage 1 ${JSON.stringify(c.cam)}`);
  const fit0 = c.cam;
  assert.deepEqual(await fr.evaluate(() => window.__tmap.st), { ready: true, todo: true, idea: true, done: true, dropped: true }, `${tag}: every status shows`);
  const visible = () => true;

  // 2. Zero titles; the soup count, and no collision anywhere.
  let col = await collide(fr);
  assert.equal(c.lb, 0, `${tag}: zero ticket titles at Fit`);
  assert.ok(c.sk >= Math.max(20, 2 * clusterIds.length), `${tag}: soup ${c.sk} >= max(20, 2 x ${clusterIds.length})`);
  clean(col, `${tag} Fit`);
  await shot(page, `c-soup-${theme}-${W}x${H}`);

  // 3. The soup sits on its centroid, recomputed from the payload.
  const base0 = await fr.evaluate(() => window.__tmapc.base());
  const placed = await fr.evaluate(() => {
    const v = document.getElementById('tmvp').getBoundingClientRect();
    return [...document.querySelectorAll('#tmlbl .d-tm-sk.on')].map(e => {
      const r = e.getBoundingClientRect(), cs = getComputedStyle(e);
      return { j: +e.dataset.j, cl: +e.dataset.cl, term: e.textContent, x: (r.left + r.right) / 2 - v.left, y: (r.top + r.bottom) / 2 - v.top,
        bg: cs.backgroundColor, bw: [cs.borderTopWidth, cs.borderRightWidth, cs.borderBottomWidth, cs.borderLeftWidth], ts: cs.textShadow, fs: parseFloat(cs.fontSize), op: +cs.opacity };
    });
  });
  const disp = placed.map(p => {
    const it = soupItem(p.cl, p.term);
    assert.ok(it, `${tag}: soup word ${p.term} is cluster ${p.cl}'s keyword`);
    const a = centroid(it, base0, visible);
    return Math.hypot(p.x - (a.x * c.cam.k + c.cam.x), p.y - (a.y * c.cam.k + c.cam.y));
  });
  const soupDisp = { median: r2(median(disp)), max: r2(Math.max(...disp)) };
  assert.ok(soupDisp.median < 30 && soupDisp.max < 110, `${tag}: soup displacement ${JSON.stringify(soupDisp)}`);

  // 4. Soup styling; borderless cluster headers.
  for (const p of placed) {
    assert.equal(p.bg, 'rgba(0, 0, 0, 0)', `${tag}: soup ${p.term} background`);
    assert.deepEqual(p.bw, ['0px', '0px', '0px', '0px'], `${tag}: soup ${p.term} border`);
    assert.ok(p.ts && p.ts !== 'none', `${tag}: soup ${p.term} text shadow`);
    assert.ok(p.fs >= 11 && p.fs <= 16, `${tag}: soup ${p.term} font ${p.fs}`);
    assert.ok(p.op >= 0.54 && p.op <= 0.89, `${tag}: soup ${p.term} opacity ${p.op}`);
  }
  assert.ok(new Set(placed.map(p => p.fs)).size > 1, `${tag}: soup font sizes vary`);
  const heads = await fr.evaluate(() => [...document.querySelectorAll('#tmlbl .d-tm-cb.on')].map(e => { const cs = getComputedStyle(e); return cs.borderTopWidth + ' ' + cs.borderLeftWidth + ' ' + cs.backgroundColor; }));
  assert.ok(heads.length > 0, `${tag}: cluster headers show`);
  assert.deepEqual([...new Set(heads)], ['0px 0px rgba(0, 0, 0, 0)'], `${tag}: cluster headers are borderless`);

  // 5. The calm backdrop.
  const calm = await fr.evaluate(() => {
    const on = e => getComputedStyle(e).display !== 'none';
    const op = s => [...new Set([...document.querySelectorAll(s)].filter(on).map(e => +getComputedStyle(e).opacity))];
    return { calm: document.getElementById('tmsvg').classList.contains('d-tm-calm'), st: op('#tmworld .d-tm-ced:not(.d-tm-ce-sim)'), sim: op('#tmworld .d-tm-ced.d-tm-ce-sim') };
  });
  assert.ok(calm.calm && calm.st.length && calm.sim.length, `${tag}: calm backdrop ${JSON.stringify(calm)}`);
  calm.st.forEach(o => near(o, 0.28, 0.011, `${tag}: structural edge opacity`));
  calm.sim.forEach(o => near(o, 0.22, 0.011, `${tag}: similarity edge opacity`));

  // 6. Hover a hub: its edges light, its neighbours, a framed label, the
  // tooltip. The fixture's hubs may lie under a soup word: the star wins.
  const hubs = (await fr.evaluate(() => window.__tmt.nodes(true))).filter(n => deg[n.i] >= 4).sort((a, b) => a.soup - b.soup);
  assert.ok(hubs.length, `${tag}: a star with deg >= 4 under the pointer`);
  const hub = hubs[0], hp = pt(s, hub);
  await page.mouse.move(hp.x - 6, hp.y - 6);
  await page.mouse.move(hp.x, hp.y);
  await raf2(fr);
  const hv = await fr.evaluate(i => {
    const lb = document.querySelector(`#tmlbl .d-tm-lb[data-i="${i}"]`), cs = getComputedStyle(lb);
    return { edges: [...document.querySelectorAll('#tmworld .d-tm-ced.d-tm-hv')].filter(e => getComputedStyle(e).display !== 'none').map(e => getComputedStyle(e).opacity),
      nb: [...document.querySelectorAll('#tmworld .d-tm-cnd.d-tm-hvn')].map(e => +e.dataset.i).filter(j => j !== i).length,
      lb: [lb.classList.contains('on'), lb.classList.contains('d-tm-hov'), cs.borderTopWidth, cs.borderTopStyle], tip: document.getElementById('tmtip').classList.contains('on') };
  }, hub.i);
  assert.ok(hv.edges.length > 0 && hv.edges.every(o => o === '1'), `${tag}: hover lights the edges ${JSON.stringify(hv)}`);
  assert.ok(hv.nb > 1, `${tag}: hover lights more than one neighbour`);
  assert.deepEqual(hv.lb, [true, true, '1px', 'solid'], `${tag}: the hovered title is framed`);
  assert.equal(hv.tip, true, `${tag}: the tooltip shows`);
  await away(s);

  // 7. A soup word: hover lights its n stars, a click filters, Esc clears.
  const soups = (await fr.evaluate(() => window.__tmt.soups())).map(w => Object.assign(w, { ids: [...new Set(soupItem(w.cl, w.term).ticketIds)].filter(visible) }))
    .filter(w => w.ids.length >= 2);
  assert.ok(soups.length, `${tag}: a soup word clear of stars`);
  const w = soups[0], wp = pt(s, w);
  await page.mouse.move(wp.x, wp.y);
  await raf2(fr);
  const lit = () => fr.evaluate(() => [...document.querySelectorAll('#tmworld .d-tm-cnd.d-tm-kwh')].map(e => +e.dataset.i).sort((a, b) => a - b));
  const ids = w.ids.slice().sort((a, b) => a - b);
  assert.deepEqual(await lit(), ids, `${tag}: hovering '${w.term}' lights exactly its stars`);
  const cam7 = (await st(fr)).cam;
  await page.mouse.click(wp.x, wp.y);
  await page.waitForTimeout(560);
  const prefix = await fr.evaluate(() => wsT('js.tmap.c.kw'));
  const chip = await fr.evaluate(() => { const e = document.getElementById('tmckw'); return { shown: !e.hidden && e.offsetParent !== null, text: e.textContent }; });
  assert.ok(chip.shown, `${tag}: the keyword chip shows`);
  assert.match(chip.text, new RegExp(`^${reEsc(prefix)} ?${reEsc(w.term)} ?\\(${ids.length}\\) ?×$`), `${tag}: chip text`);
  assert.deepEqual(await lit(), ids, `${tag}: the filter lights its stars`);
  c = await st(fr);
  assert.equal(c.kw, w.term);
  assert.ok(Math.hypot(c.cam.x - cam7.x, c.cam.y - cam7.y) > 20, `${tag}: the camera frames the matches`);
  await away(s);
  await shot(page, `kw-filter-${theme}-${W}x${H}`);
  await key(s, 'Escape');
  await raf2(fr);
  assert.equal((await st(fr)).kw, null, `${tag}: Esc clears the keyword`);
  assert.equal(await fr.locator('#tmckw').isHidden(), true);
  assert.deepEqual(await lit(), [], `${tag}: no keyword highlight after Esc`);

  // 8. Pan: empty canvas, a soup word, a star; Alt+drag moves the star; middle button.
  await setCam(fr, fit0);
  const panned = async (from, d, button, label) => {
    const c0 = (await st(fr)).cam;
    await drag(s, from, d, button);
    const c1 = (await st(fr)).cam;
    near(c1.x - c0.x, d.x, 1.5, `${label}: pan x`);
    near(c1.y - c0.y, d.y, 1.5, `${label}: pan y`);
    assert.equal(c1.k, c0.k, `${label}: k unchanged`);
    await setCam(fr, fit0);
  };
  const bg = await fr.evaluate(() => window.__tmt.bg(-90, -50));
  assert.ok(bg, `${tag}: an empty canvas point`);
  await panned(bg, { x: -90, y: -50 }, 'left', `${tag} empty drag`);
  const sw = (await fr.evaluate(() => window.__tmt.soups()))[0];
  assert.ok(sw, `${tag}: a soup word to drag`);
  await panned(sw, { x: -60, y: -40 }, 'left', `${tag} soup drag`);
  assert.equal((await st(fr)).kw, null, `${tag}: a soup drag sets no filter`);
  const nd = (await fr.evaluate(() => window.__tmt.nodes()))[0];
  const nb0 = (await fr.evaluate(() => window.__tmapc.base()))[nd.i];
  await panned(nd, { x: 70, y: 30 }, 'left', `${tag} star drag`);
  const nb1 = (await fr.evaluate(() => window.__tmapc.base()))[nd.i];
  assert.ok(Math.hypot(nb1[0] - nb0[0], nb1[1] - nb0[1]) < 0.01, `${tag}: a plain drag leaves the star`);
  assert.equal((await st(fr)).sel, null, `${tag}: a drag selects nothing`);
  const ca = (await st(fr)).cam;
  await page.keyboard.down('Alt');
  await drag(s, nd, { x: 40, y: 25 });
  await page.keyboard.up('Alt');
  const nb2 = (await fr.evaluate(() => window.__tmapc.pos()))[nd.i], cb = (await st(fr)).cam;
  near(nb2[0] - nb0[0], 40 / ca.k, 0.6, `${tag}: Alt+drag x by delta/k`);
  near(nb2[1] - nb0[1], 25 / ca.k, 0.6, `${tag}: Alt+drag y by delta/k`);
  assert.deepEqual(cb, ca, `${tag}: Alt+drag keeps the camera`);
  await page.keyboard.down('Alt');
  await drag(s, { x: nd.x + 40, y: nd.y + 25 }, { x: -40, y: -25 });
  await page.keyboard.up('Alt');
  await panned(bg, { x: 50, y: 35 }, 'middle', `${tag} middle drag`);

  // 9. Wheel: deltaMode 0, 1, 2 at mixed anchors, then a real wheel.
  const anchors = [[s.vp.w / 2, s.vp.h / 2], [140, 170], [s.vp.w - 230, s.vp.h - 140]];
  for (const [x, y] of anchors) {
    await setCam(fr, fit0);
    let r = await fr.evaluate(a => window.__tmt.wheel(a[0], a[1], -120, 0), [x, y]);
    assert.ok(r.c1.k / r.c0.k > 1.3 && r.drift < 1e-9, `${tag}: deltaMode 0 at ${x},${y} ${JSON.stringify(r)}`);
    r = await fr.evaluate(a => window.__tmt.wheel(a[0], a[1], 3, 1), [x, y]);
    assert.ok(r.c0.k / r.c1.k > 1.25 && r.drift < 1e-9, `${tag}: deltaMode 1 at ${x},${y} ${JSON.stringify(r)}`);
    let k = r.c1.k;
    for (let n = 0; n < 4; n++) {
      r = await fr.evaluate(a => window.__tmt.wheel(a[0], a[1], 1, 2), [x, y]);
      assert.ok(r.c1.k <= k && r.c1.k >= 0.18 && r.drift < 1e-9, `${tag}: deltaMode 2 at ${x},${y} ${JSON.stringify(r)}`);
      k = r.c1.k;
    }
    assert.equal(k, 0.18, `${tag}: deltaMode 2 clamps at .18`);
  }
  await setCam(fr, fit0);
  const mid = pt(s, { x: s.vp.w / 2, y: s.vp.h / 2 });
  await page.mouse.move(mid.x, mid.y);
  await page.mouse.wheel(0, -100);
  await page.waitForTimeout(150);
  assert.ok((await st(fr)).cam.k / fit0.k > 1.2, `${tag}: a real wheel zooms in`);

  // 10. Buttons: +, -, 1:1 and the readout.
  await setCam(fr, fit0);
  const zr = () => fr.evaluate(() => document.getElementById('tmzrd').textContent);
  await fr.locator('#tmzin').click();
  await page.waitForTimeout(330);
  c = await st(fr);
  assert.ok(c.cam.k / fit0.k > 1.3, `${tag}: + zooms in`);
  assert.equal(await zr(), Math.round(c.cam.k * 100) + '%', `${tag}: + readout`);
  await fr.locator('#tmzout').click();
  await page.waitForTimeout(330);
  near((await st(fr)).cam.k, fit0.k, 1e-6, `${tag}: - returns`);
  await fr.locator('#tmz100').click();
  await page.waitForTimeout(420);
  near((await st(fr)).cam.k, 1, 1e-3, `${tag}: 1:1`);
  assert.equal(await zr(), '100%', `${tag}: 1:1 readout`);

  // 11-13. The semantic zoom ladder: collisions, stages, soup alpha, titles.
  await away(s);
  const ladder = {};
  for (const z of ZOOMS) {
    await setCam(fr, z === 'fit' ? fit0 : zoomAt(fit0, s.vp.w / 2, s.vp.h / 2, z));
    await raf2(fr);
    const q = await st(fr), cc = await collide(fr);
    clean(cc, `${tag} k=${z}`);
    ladder[z] = { stage: q.stage, soupA: r2(q.soupA), soup: q.sk, titles: q.lb };
  }
  assert.equal(ladder[0.8].soupA, 1, `${tag}: soup alpha at .8`);
  assert.ok(ladder[1.0].soupA > 0.55 && ladder[1.0].soupA < 0.7, `${tag}: soup alpha at 1.0`);
  assert.ok(ladder[1.2].soupA < 0.15, `${tag}: soup alpha at 1.2`);
  assert.ok(ladder[1.4].soupA === 0 && ladder[1.4].soup === 0, `${tag}: no soup at 1.4`);
  assert.equal(ladder[0.8].titles, 0, `${tag}: no titles at .8`);
  assert.ok(ladder[1.0].titles > 0 && ladder[1.4].titles > ladder[1.0].titles, `${tag}: titles grow ${JSON.stringify(ladder)}`);
  assert.deepEqual([ladder[1.0].stage, ladder[1.4].stage], [2, 3], `${tag}: stages`);
  // 13. Plain borderless titles at k >= 1.4.
  await setCam(fr, zoomAt(fit0, s.vp.w / 2, s.vp.h / 2, 1.45));
  const plain = await fr.evaluate(() => [...document.querySelectorAll('#tmlbl .d-tm-lb.on:not(.d-tm-sel):not(.d-tm-hov)')].map(e => { const cs = getComputedStyle(e); return cs.backgroundColor + ' ' + cs.borderTopWidth + ' ' + cs.borderLeftWidth; }));
  assert.ok(plain.length > 15, `${tag}: ${plain.length} titles at 1.45`);
  assert.deepEqual([...new Set(plain)], ['rgba(0, 0, 0, 0) 0px 0px'], `${tag}: titles are borderless`);
  clean(await collide(fr), `${tag} k=1.45`);
  // The tooltip at the right edge and near the bottom edge.
  const tipAt = async (i, x, y, label) => {
    const p = (await fr.evaluate(() => window.__tmapc.pos()))[i];
    await setCam(fr, { k: 1.45, x: x - p[0] * 1.45, y: y - p[1] * 1.45 });
    await raf2(fr);
    const m = pt(s, { x, y });
    await page.mouse.move(m.x - 3, m.y - 3);
    await page.mouse.move(m.x, m.y);
    await raf2(fr);
    const t = await fr.evaluate(() => {
      const b = document.getElementById('tmvc').getBoundingClientRect(), e = document.getElementById('tmtip'), r = e.getBoundingClientRect();
      return { on: e.classList.contains('on'), l: r.left - b.left, t: r.top - b.top, r: b.right - r.right, b: b.bottom - r.bottom, top: r.top - b.top, bottom: r.bottom - b.top, left: r.left - b.left };
    });
    assert.ok(t.on, `${label}: the tooltip shows`);
    assert.ok(t.l >= 4 && t.t >= 4 && t.r >= 4 && t.b >= 4, `${label}: inside with 4px margins ${JSON.stringify(t)}`);
    await away(s);
    return t;
  };
  const tipNode = hub.i, vt = s.vp.t - (await fr.evaluate(() => document.getElementById('tmvc').getBoundingClientRect().top));
  const tr = await tipAt(tipNode, s.vp.w - 24, s.vp.h / 2, `${tag} right-edge tooltip`);
  assert.ok(tr.left < s.vp.w - 24, `${tag}: the right-edge tooltip flips left`);
  const tb = await tipAt(tipNode, Math.round(s.vp.w * 0.6), s.vp.h - 30, `${tag} bottom tooltip`);
  assert.ok(tb.bottom <= s.vp.h - 30 + vt, `${tag}: the bottom tooltip flips above ${JSON.stringify(tb)}`);
  // c-zoom: k = 1.45 on the largest cluster.
  const members = G.nodes.map((n, i) => i).filter(i => G.nodes[i].cluster === bigCluster);
  const cx = members.reduce((a, i) => a + base0[i][0], 0) / members.length, cy = members.reduce((a, i) => a + base0[i][1], 0) / members.length;
  await setCam(fr, { k: 1.45, x: s.vp.w / 2 - cx * 1.45, y: s.vp.h / 2 - cy * 1.45 });
  await raf2(fr);
  clean(await collide(fr), `${tag} c-zoom`);
  await shot(page, `c-zoom-${theme}-${W}x${H}`);

  // 14. The Ego Ring on the epic: a double click, Esc, E.
  await setCam(fr, fit0);
  await raf2(fr);
  const ep = pt(s, await fr.evaluate(i => window.__tmt.screen(i), epicI));
  await page.mouse.dblclick(ep.x, ep.y);
  await page.waitForTimeout(600);
  await away(s);
  const ego = async label => {
    const q = await st(fr), cc = await collide(fr);
    const m = await fr.evaluate(() => [...document.querySelectorAll('#tmworld .d-tm-cnd.d-tm-eg')].map(e => [+e.dataset.i, document.querySelector(`#tmlbl .d-tm-lb[data-i="${e.dataset.i}"]`).classList.contains('on')]));
    assert.equal(q.ego, epicI, `${label}: S.ego is the epic`);
    assert.ok(m.length - 1 > 3, `${label}: ${m.length - 1} members`);
    assert.deepEqual(m.filter(x => !x[1]).map(x => G.nodes[x[0]].stem), [], `${label}: every member is labeled`);
    clean(cc, label);
    assert.equal(q.sk, 0, `${label}: no soup`);
    return m.length - 1;
  };
  const members14 = await ego(`${tag} ego`);
  await shot(page, `ego-${theme}-${W}x${H}`);
  await key(s, 'Escape');
  await page.waitForTimeout(560);
  c = await st(fr);
  assert.deepEqual([c.ego, c.sel], [null, epicI], `${tag}: Esc leaves the ring and keeps the selection`);
  await key(s, 'e');
  await page.waitForTimeout(600);
  await ego(`${tag} E re-enters`);
  await key(s, 'Escape');
  await page.waitForTimeout(560);
  await key(s, 'Escape');
  await raf2(fr);
  assert.equal((await st(fr)).sel, null, `${tag}: a further Esc deselects`);

  // 15. B focus -> 2 keeps the selection, framed; C selection -> 1 opens B's focus.
  await key(s, '1');
  await fr.locator('#tmvb .d-tm-mx').waitFor();
  await fr.locator(`#tmvb .d-tm-card[title="${epicStem}"]`).click();
  await fr.locator('#tmhero').waitFor();
  await key(s, '2');
  await page.waitForTimeout(400);
  c = await st(fr);
  const framed = await fr.evaluate(i => { const e = document.querySelector(`#tmlbl .d-tm-lb[data-i="${i}"]`), cs = getComputedStyle(e); return [e.classList.contains('on'), e.classList.contains('d-tm-sel'), cs.borderTopWidth]; }, epicI);
  assert.deepEqual([c.view, c.sel], ['C', epicI], `${tag}: 2 shows C with B's selection`);
  assert.ok(c.cam.k >= 0.95, `${tag}: the camera centers it at k >= .95`);
  assert.deepEqual(framed, [true, true, '1px'], `${tag}: the selected title is framed`);
  s.vp = await fr.evaluate(() => window.__tmt.vp());
  const other = (await fr.evaluate(() => window.__tmt.nodes())).find(n => n.i !== epicI);
  assert.ok(other, `${tag}: another star to click`);
  const op = pt(s, other);
  await page.mouse.click(op.x, op.y);
  await raf2(fr);
  const sel15 = (await st(fr)).sel;
  assert.ok(sel15 !== null && sel15 !== epicI, `${tag}: a click selects in C`);
  await key(s, '1');
  await fr.locator('#tmhero').waitFor();
  assert.equal(await fr.evaluate(() => document.querySelector('#tmhero .d-tm-stem').textContent), G.nodes[sel15].stem, `${tag}: 1 opens B's focus on it`);
  assert.equal(await fr.evaluate(() => getComputedStyle(document.getElementById('tminsp')).display), 'none', `${tag}: with the inspector closed`);

  // The single fetch: nothing after it; 17. no errors.
  await page.waitForTimeout(200);
  assert.deepEqual(s.after, [], `${tag}: no request after the single fetch`);
  assert.equal(s.graphReqs, 1);
  assert.deepEqual(s.errors, [], `${tag}: console errors, page errors, failed requests`);
  await s.context.close();
  return { tag, fitK: r2(fit0.k), clusters: clusterIds.length, soupDisp, ladder, ego: members14 };
}

// 16. Touch: a one-finger drag pans; a CDP pinch zooms about the midpoint.
async function touch(browser) {
  const s = await newSession(browser, 'dark', 1280, 800, { hasTouch: true });
  const fr = await openC(s), cdp = await s.context.newCDPSession(s.page);
  const send = (type, pts) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: pts.map((p, id) => ({ x: p.x, y: p.y, id })) });
  const c0 = (await st(fr)).cam, bg = await fr.evaluate(() => window.__tmt.bg(-80, -40));
  const a = pt(s, bg);
  await send('touchStart', [a]);
  for (let k = 1; k <= 8; k++) await send('touchMove', [{ x: a.x - 80 * k / 8, y: a.y - 40 * k / 8 }]);
  await send('touchEnd', []);
  await raf2(fr);
  const c1 = (await st(fr)).cam;
  near(c1.x - c0.x, -80, 1.5, `${s.tag}: one-finger pan x`);
  near(c1.y - c0.y, -40, 1.5, `${s.tag}: one-finger pan y`);
  const m = { x: Math.round(s.vp.w / 2), y: Math.round(s.vp.h / 2) }, mp = pt(s, m);
  await send('touchStart', [{ x: mp.x - 50, y: mp.y }, { x: mp.x + 50, y: mp.y }]);
  for (let k = 1; k <= 10; k++) await send('touchMove', [{ x: mp.x - 50 - 50 * k / 10, y: mp.y }, { x: mp.x + 50 + 50 * k / 10, y: mp.y }]);
  await send('touchEnd', []);
  await raf2(fr);
  const c2 = (await st(fr)).cam, wx = (m.x - c1.x) / c1.k, wy = (m.y - c1.y) / c1.k;
  const drift = Math.hypot(wx * c2.k + c2.x - m.x, wy * c2.k + c2.y - m.y);
  near(c2.k / c1.k, 2, 0.05, `${s.tag}: the pinch doubles k`);
  assert.ok(drift < 1, `${s.tag}: pinch drift ${drift}`);
  assert.equal((await st(fr)).sel, null, `${s.tag}: a pinch selects nothing`);
  assert.deepEqual(s.after, [], `${s.tag}: no request after the single fetch`);
  assert.deepEqual(s.errors, [], `${s.tag}: errors`);
  await s.context.close();
  return { pan: [r2(c1.x - c0.x), r2(c1.y - c0.y)], pinch: r2(c2.k / c1.k), drift };
}

// Performance: layout ms at 170 and 300 open tickets, the draw cost and
// the rAF frame time during a continuous wheel, and no request after the
// single fetch (Refresh aside).
async function perf(browser) {
  const s = await newSession(browser, 'dark', 1600, 960);
  let body = fs.readFileSync(graphFile);
  await s.context.route('**/tickets/graph', r => r.fulfill({ status: 200, contentType: 'application/json', body }));
  const fr = await openC(s), out = { layout170: [], layout300: [] };
  out.layout170.push(r2((await st(fr)).layoutMs));
  const wheelRun = async label => {
    await fr.evaluate(() => {
      window.__ft = []; window.__ftOn = true;
      let last = performance.now();
      const loop = t => { window.__ft.push(t - last); last = t; if (window.__ftOn) requestAnimationFrame(loop); };
      requestAnimationFrame(loop);
    });
    const v = await fr.evaluate(() => window.__tmt.vp()), mid = pt(s, { x: v.w / 2, y: v.h / 2 });
    await s.page.mouse.move(mid.x, mid.y);
    for (let n = 0; n < 80; n++) { await s.page.mouse.wheel(0, n < 40 ? -30 : 30); await sleep(8); }
    const ft = await fr.evaluate(() => { window.__ftOn = false; return window.__ft.slice(2); });
    const draw = await fr.evaluate(() => {
      const c0 = Object.assign({}, window.__tmap.cam), t = [];
      [0.5, 0.8, 1.0, 1.4, 2.0].forEach(k => {
        const v = window.__tmt.vp(), wx = (v.w / 2 - c0.x) / c0.k, wy = (v.h / 2 - c0.y) / c0.k;
        window.__tmapc.setCam({ k, x: v.w / 2 - wx * k, y: v.h / 2 - wy * k });
        for (let n = 0; n < 10; n++) { const a = performance.now(); window.__tmapc.draw(); t.push(performance.now() - a); }
      });
      window.__tmapc.setCam(c0);
      return t;
    });
    out[label] = { frames: ft.length, frameMean: r2(ft.reduce((a, b) => a + b, 0) / ft.length), frameP95: r2(pct(ft, 0.95)), frameMax: r2(Math.max(...ft)),
      drawMean: r2(draw.reduce((a, b) => a + b, 0) / draw.length), drawMax: r2(Math.max(...draw)) };
  };
  await wheelRun('wheel170');
  const refresh = async n => {
    await fr.locator('#tmrefresh').click();
    await fr.waitForFunction(n => !document.getElementById('tmrefresh').disabled && window.__tmapc.pos().length === n, n);
    await raf2(fr);
    return r2((await st(fr)).layoutMs);
  };
  body = G300;
  for (let n = 0; n < 3; n++) out.layout300.push(await refresh(N300));
  await wheelRun('wheel300');
  body = fs.readFileSync(graphFile);
  for (let n = 0; n < 2; n++) out.layout170.push(await refresh(G.nodes.length));
  const others = s.after.filter(u => !new URL(u.split(' ')[1]).pathname.endsWith('/tickets/graph'));
  assert.deepEqual(others, [], 'perf: no request but Refresh after the single fetch');
  assert.equal(s.graphReqs, 1 + 5, 'perf: one fetch per open and Refresh');
  assert.ok(Math.min(...out.layout300) <= 150, `perf: the 300-ticket layout within 150ms ${JSON.stringify(out.layout300)}`);
  assert.deepEqual(s.errors, [], 'perf: errors');
  await s.context.close();
  return Object.assign(out, { nodes170: G.nodes.length, nodes300: N300 });
}

// The approved mockup's same states through its own controls: Fit; a drag
// bringing its largest cluster to the center, + to k >= 1.35 and a wheel
// notch to 1.45; a double click on its most linked ticket; and a click on a
// soup word clear of stars.
async function mockupShots(browser) {
  if (!mockup || !shots) return null;
  const context = await browser.newContext({ viewport: { width: 1600, height: 960 } });
  const page = await context.newPage(), out = {};
  const cam = () => page.evaluate(() => Object.assign({}, window.__tmv.C.cam));
  for (const theme of ['dark', 'light']) {
    for (const [W, H] of SIZES) {
      const tag = `${theme}-${W}x${H}`;
      await page.setViewportSize({ width: W, height: H });
      // A fresh document each time: the same URL would only change the hash.
      await page.goto('about:blank');
      await page.goto(pathToFileURL(mockup).href + '#c');
      await page.locator('#vC .nd').first().waitFor();
      if (await page.evaluate(() => document.documentElement.dataset.theme) !== theme) await page.locator('#theme').click();
      await page.locator('#zFit').click();
      await page.waitForTimeout(450);
      await shot(page, `mock-c-soup-${tag}`);
      const vp = await page.locator('#vp').boundingBox();
      // The largest cluster's center, brought to the viewport center by a drag.
      const ctr = await page.evaluate(() => {
        const { N, C } = window.__tmv, by = new Map();
        N.forEach(n => { if (!by.has(n.k)) by.set(n.k, []); by.get(n.k).push(n.i); });
        const big = [...by.values()].sort((a, b) => b.length - a.length)[0], c = C.cam;
        const x = big.reduce((a, i) => a + C.Bp[i].x, 0) / big.length, y = big.reduce((a, i) => a + C.Bp[i].y, 0) / big.length;
        return { x: x * c.k + c.x, y: y * c.k + c.y };
      });
      const from = { x: vp.x + vp.width * 0.5, y: vp.y + vp.height * 0.5 };
      const off = await page.evaluate(([x, y]) => {
        // An empty-canvas start point near the center.
        for (let r = 0; r < 300; r += 9) for (let a = 0; a < 12; a++) {
          const px = x + r * Math.cos(a * Math.PI / 6), py = y + r * Math.sin(a * Math.PI / 6), e = document.elementFromPoint(px, py);
          if (e && !e.closest('.nd, .lb, .sk, .cb') && document.getElementById('vp').contains(e)) return { x: px, y: py };
        }
        return { x, y };
      }, [from.x, from.y]);
      const d = { x: vp.x + vp.width / 2 - (vp.x + ctr.x), y: vp.y + vp.height / 2 - (vp.y + ctr.y) };
      await page.mouse.move(off.x, off.y);
      await page.mouse.down();
      for (let k = 1; k <= 8; k++) await page.mouse.move(off.x + d.x * k / 8, off.y + d.y * k / 8);
      await page.mouse.up();
      while ((await cam()).k < 1.35) { await page.locator('#zIn').click(); await page.waitForTimeout(260); }
      // + steps by 1.35x; one wheel notch at the center lands on k = 1.45.
      await page.mouse.move(vp.x + vp.width / 2, vp.y + vp.height / 2);
      await page.mouse.wheel(0, Math.log((await cam()).k / 1.45) / 0.0028);
      await page.waitForTimeout(150);
      await page.mouse.move(vp.x + 2, vp.y + 2);
      await page.waitForTimeout(200);
      out[tag] = { zoomK: r2((await cam()).k) };
      await shot(page, `mock-c-zoom-${tag}`);
      await page.locator('#zFit').click();
      await page.waitForTimeout(450);
      // The most linked ticket's Ego Ring.
      const hubAt = await page.evaluate(() => {
        const { N, C } = window.__tmv, c = C.cam, v = document.getElementById('vp').getBoundingClientRect();
        const i = N.slice().sort((a, b) => b.X.size - a.X.size)[0].i, p = C.P[i];
        return { i, x: v.left + p.x * c.k + c.x, y: v.top + p.y * c.k + c.y };
      });
      await page.mouse.dblclick(hubAt.x, hubAt.y);
      await page.waitForTimeout(650);
      await page.mouse.move(vp.x + 2, vp.y + 2);
      out[tag].ego = await page.evaluate(() => window.__tmv.S.ego);
      await shot(page, `mock-c-ego-${tag}`);
      await page.keyboard.press('Escape');
      await page.waitForTimeout(500);
      await page.keyboard.press('Escape');
      await page.locator('#zFit').click();
      await page.waitForTimeout(450);
      // A soup word clear of stars.
      const word = await page.evaluate(() => {
        for (const e of document.querySelectorAll('#vC .sk.on')) {
          const r = e.getBoundingClientRect(), x = (r.left + r.right) / 2, y = (r.top + r.bottom) / 2;
          if (document.elementFromPoint(x, y) === e && !document.elementsFromPoint(x, y).some(t => t.closest && t.closest('.nd'))) return { x, y, term: e.textContent };
        }
        return null;
      });
      if (word) {
        await page.mouse.click(word.x, word.y);
        await page.waitForTimeout(600);
        await page.mouse.move(vp.x + 2, vp.y + 2);
        out[tag].kw = word.term;
        await shot(page, `mock-c-kw-filter-${tag}`);
      }
    }
  }
  await context.close();
  return out;
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const report = { nodes: G.nodes.length, clusters: clusterIds.length, epic: epicStem, sessions: [] };
  try {
    report.mockup = await mockupShots(browser);
    for (const theme of ['dark', 'light']) {
      for (const [W, H] of SIZES) report.sessions.push(await viewC(browser, theme, W, H));
    }
    report.touch = await touch(browser);
    report.perf = await perf(browser);
    console.log(JSON.stringify(report, null, 1));
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e && e.stack || e); process.exit(1); });
