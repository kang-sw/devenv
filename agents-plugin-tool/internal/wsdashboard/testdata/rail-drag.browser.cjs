// Run by TestRailDragReorderBrowser against disposable fixture sessions.
// argv: the fixture daemon's base URL, the mode ("static", or "live" for
// TestRailDragLiveBrowser, whose daemon re-renders the rail several times a
// second meanwhile), and an optional absolute directory for the
// during/after drag screenshots.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, mode, shots] = process.argv.slice(2);

const row = (id) => `#r-rail .rs[data-rid="${id}"]`;
const KIDS = '[execfour,execfive,execone,exectwo,execthree]';
const BASELINE = `discuss mainlead ${KIDS} cirow sosterm`;

// order is the group's list as shown: top-level rows, a parent's children
// in brackets after it. mainlead is a parent, so it never nests and its
// list is the group's.
async function order(page) {
  return page.evaluate(() => {
    const list = document.querySelector('#r-rail .rs[data-rid="mainlead"]').parentNode;
    const out = [];
    for (const c of list.children) {
      if (c.classList.contains('rs')) out.push(c.getAttribute('data-rid'));
      else if (c.classList.contains('rkids')) out.push('[' + [...c.children].filter((k) => k.classList.contains('rs')).map((k) => k.getAttribute('data-rid')).join(',') + ']');
    }
    return out.join(' ');
  });
}
async function post(page, body) {
  return page.evaluate(async (body) => {
    const r = await fetch('/rail/layout', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    return r.status;
  }, body);
}
async function showRail(page) {
  await page.locator(row('discuss')).waitFor({ state: 'attached' });
  await page.waitForTimeout(300);
  // Visibility can remain true briefly while the overlay is closing.
  if (!(await page.locator('html').evaluate(e => e.classList.contains('rail-open')))) {
    await page.locator('#railbtn').click();
    await page.locator(row('discuss')).waitFor();
  }
  await page.addStyleTag({ content: '*{animation-play-state:paused !important; transition:none !important}' });
}
// reset lays the group out as the owner's screenshot through the layout
// route, then reloads: discuss, mainlead with five children, cirow, sosterm.
async function reset(page) {
  for (const id of ['discuss', 'cirow', 'sosterm']) await post(page, { op: 'detach', rid: id });
  for (const id of ['execfour', 'execfive', 'execone', 'exectwo', 'execthree']) {
    assert.equal(await post(page, { op: 'attach', rid: id, parent: 'mainlead' }), 200, 'attach ' + id);
  }
  await post(page, { op: 'move', rid: 'mainlead', after: 'discuss' });
  await post(page, { op: 'move', rid: 'cirow', after: 'mainlead' });
  await post(page, { op: 'move', rid: 'sosterm', after: 'cirow' });
  await page.reload();
  await showRail(page);
  await page.waitForTimeout(150);
  assert.equal(await order(page), BASELINE, 'reset layout');
}
async function box(page, sel) {
  const b = await page.locator(sel).first().boundingBox();
  assert.ok(b, 'no box for ' + sel);
  return b;
}
async function indicator(page) {
  return page.evaluate(() => {
    const l = document.querySelector('.rdrop'), k = document.querySelector('#r-rail .rs.rdkid'), p = document.querySelector('.rpop.shp');
    return { pop: !!p && !p.hidden, line: l && !l.hidden ? Math.round(parseFloat(l.style.top)) : null, left: l && !l.hidden ? Math.round(parseFloat(l.style.left)) : null, kid: k ? k.getAttribute('data-rid') : null };
  });
}

// drag presses from's grip or its body (its name), moves to the point
// (x, y) given by at() (read after the press, as rows do not move during a
// drag), optionally holds, checks the indicator, shoots, and releases. It
// answers the indicator and the layout post's answer (null: none posted).
async function drag(page, from, { grip = false, at, hold = 60, shot = '', releaseMove = false, lost = false, cancel = '' }) {
  const fb = await box(page, row(from) + (grip ? ' .drgrip' : ' .rid'));
  const sx = fb.x + fb.width / 2, sy = fb.y + Math.min(fb.height / 2, 10);
  await page.mouse.move(sx, sy);
  const h0 = await page.evaluate(() => window.__swaps || 0);
  await page.mouse.down();
  const [tx, ty] = await at();
  await page.mouse.move(sx, sy + (ty > sy ? 8 : -8), { steps: 2 });
  await page.mouse.move(tx, ty, { steps: 10 });
  await page.waitForTimeout(hold);
  const ind = await indicator(page);
  const styles = await page.evaluate(() => ['#rail', '#rlist', '#r-rail .rs'].map(s => { const c = getComputedStyle(document.querySelector(s)); return [c.touchAction, c.userSelect]; }));
  assert.deepEqual(styles, [['none', 'none'], ['none', 'none'], ['none', 'none']], 'active drag suppresses selection and touch gestures');
  if (shot && shots) await page.screenshot({ path: path.join(shots, shot) });
  const h1 = await page.evaluate(() => window.__swaps || 0);
  const resp = page.waitForResponse((r) => r.url().endsWith('/rail/layout') && r.request().method() === 'POST', { timeout: 1500 }).then((r) => r.json()).catch(() => null);
  if (releaseMove) {
    await page.evaluate(() => document.dispatchEvent(new PointerEvent('pointermove', { pointerId: 1, pointerType: 'mouse', buttons: 0, clientX: 0, clientY: 0, bubbles: true })));
    assert.deepEqual(await indicator(page), ind, 'release transition keeps the visible target');
  }
  if (lost) await page.waitForTimeout(180);
  if (cancel === 'escape') await page.keyboard.press('Escape');
  if (cancel === 'pointercancel') await page.evaluate(() => document.dispatchEvent(new PointerEvent('pointercancel', { pointerId: 1, pointerType: 'mouse', bubbles: true })));
  await page.mouse.up();
  const v = await resp;
  await page.waitForTimeout(250);
  return { ind, post: v, held: h1 - h0 };
}
// mid answers a point at frac of row id's height, at its middle x.
const at = (page, id, frac) => async () => {
  const b = await box(page, row(id));
  return [b.x + b.width / 2, b.y + b.height * frac];
};

async function main() {
  const browser = await chromium.launch();
  if (mode === 'live') return live(browser);
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: 'dark', locale: 'ko-KR' });
  await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(base + '/');
  await showRail(page);
  await reset(page);

  // A trackpad's button-up move can precede pointerup; the last line wins.
  let track = await drag(page, 'discuss', { at: at(page, 'cirow', 0.75), releaseMove: true });
  assert.equal(track.post && track.post.applied, true, 'release transition commits');
  assert.equal(await order(page), `mainlead ${KIDS} cirow discuss sosterm`);
  await reset(page);
  await watchSwaps(page);
  track = await drag(page, 'discuss', { at: at(page, 'cirow', 0.75), releaseMove: true, lost: true });
  assert.equal(track.post, null, 'missing pointerup cancels');
  assert.equal(await order(page), BASELINE);
  assert.equal(await page.locator('html').evaluate(e => e.classList.contains('rdragging')), false, 'fallback releases drag');

  for (const cancel of ['pointercancel', 'escape']) {
    track = await drag(page, 'discuss', { at: at(page, 'cirow', 0.75), releaseMove: true, cancel });
    assert.equal(track.post, null, `${cancel} does not drop`);
    await showRail(page);
    assert.equal(await order(page), BASELINE, `${cancel} leaves order intact`);
  }

  // Downward: the first row after the third top-level row (cirow's lower
  // half), by the grip. The line is at cirow's bottom, where it lands.
  const cb = await box(page, row('cirow'));
  let r = await drag(page, 'discuss', { grip: true, at: at(page, 'cirow', 0.75), shot: 'during-drag-down.png' });
  assert.equal(r.ind.kid, null, 'a pass over a row arms no child zone');
  assert.equal(r.ind.pop, false, 'no session hover card during a drag');
  assert.ok(Math.abs(r.ind.line - (cb.y + cb.height)) <= 2, `down line ${r.ind.line} at cirow's bottom ${cb.y + cb.height}`);
  assert.equal(r.post && r.post.applied, true, 'down drop posted and applied');
  assert.equal(await order(page), `mainlead ${KIDS} cirow discuss sosterm`, 'down: first lands after the third');
  if (shots) await page.screenshot({ path: path.join(shots, 'after-drop-down.png') });

  if (shots) {
    for (const theme of ['dark', 'light']) {
      await page.locator('.settings-toggle').first().click();
      await page.locator(`.gmenu [data-theme-set="${theme}"]`).click();
      await page.locator('#settings-close').click();
      await reset(page);
      await drag(page, 'discuss', { at: at(page, 'cirow', 0.75), shot: `drag-line-${theme}.png` });
    }
  }

  // Upward: the last row before the first (discuss's upper half).
  await reset(page);
  const db = await box(page, row('discuss'));
  r = await drag(page, 'sosterm', { grip: true, at: at(page, 'discuss', 0.25), shot: 'during-drag-up.png' });
  assert.ok(Math.abs(r.ind.line - db.y) <= 2, `up line ${r.ind.line} at discuss's top ${db.y}`);
  assert.equal(r.post && r.post.applied, true, 'up drop posted and applied');
  assert.equal(await order(page), `sosterm discuss mainlead ${KIDS} cirow`, 'up: last lands first');
  if (shots) await page.screenshot({ path: path.join(shots, 'after-drop-up.png') });

  // The boundary between two rows (cirow | sosterm) is the line there.
  await reset(page);
  r = await drag(page, 'discuss', { at: async () => { const b = await box(page, row('sosterm')); return [b.x + b.width / 2, b.y + 0.5]; } });
  assert.equal(r.post && r.post.applied, true, 'boundary drop applied');
  assert.equal(await order(page), `mainlead ${KIDS} cirow discuss sosterm`, 'boundary: lands between cirow and sosterm');
  // Exercise an actual empty seam beneath the expanded children, rather than
  // just a boundary pixel that still hits a row.
  await reset(page);
  await page.addStyleTag({ content: '#r-rail .rkids { margin-bottom: 4px !important }' });
  r = await drag(page, 'discuss', { at: async () => { const b = await box(page, row('cirow')); return [b.x + b.width / 2, b.y - 2]; } });
  assert.notEqual(r.ind.line, null, 'gap retains a visible insertion line');
  assert.equal(r.post && r.post.applied, true, 'gap drop applied');
  assert.equal(await order(page), `mainlead ${KIDS} discuss cirow sosterm`, 'gap lands after the expanded unit');
  await reset(page);
  // Below the last row, in the group's empty space: after the last.
  r = await drag(page, 'mainlead', { at: async () => { const b = await box(page, row('sosterm')); return [b.x + b.width / 2, b.y + b.height + 6]; } });
  assert.equal(r.post && r.post.applied, true, 'below-last drop applied');
  assert.equal(await order(page), `discuss cirow sosterm mainlead ${KIDS}`, 'below the last row: lands last, children along');

  // From the row body, not the grip; a drop over the row itself or its
  // own slot (the row above's lower half) posts nothing.
  await reset(page);
  r = await drag(page, 'cirow', { at: at(page, 'cirow', 0.3) });
  assert.equal(r.post, null, 'a drop on the row itself posts nothing');
  r = await drag(page, 'cirow', { at: at(page, 'sosterm', 0.2) });
  assert.equal(r.post, null, 'a drop in its own slot posts nothing');
  assert.equal(await order(page), BASELINE, 'no-op drops leave the order');
  r = await drag(page, 'discuss', { at: at(page, 'cirow', 0.7) });
  assert.equal(r.post && r.post.applied, true, 'body drag applied');
  assert.equal(await order(page), `mainlead ${KIDS} cirow discuss sosterm`, 'body drag: lands after cirow');

  // Held still in a valid parent's middle half, the child zone arms.
  await reset(page);
  r = await drag(page, 'discuss', { at: at(page, 'cirow', 0.5), hold: 900 });
  assert.equal(r.ind.kid, 'cirow', 'a dwell outlines the parent');
  assert.equal(r.ind.line, null, 'no line while the child zone is armed');
  assert.equal(r.post && r.post.applied, true, 'attach applied');
  assert.equal(await order(page), `mainlead ${KIDS} cirow [discuss] sosterm`, 'dwell: attaches as a child');

  // A click on a row's body opens it and moves nothing.
  await reset(page);
  let posted = 0;
  const count = (q) => { if (q.url().endsWith('/rail/layout') && q.method() === 'POST') posted++; };
  page.on('request', count);
  await page.locator(row('cirow') + ' .rid').click();
  await page.waitForURL(/\/s\/cirow/, { timeout: 5000 });
  await showRail(page);
  page.off('request', count);
  assert.equal(posted, 0, 'a click posts no layout op');
  assert.equal(await order(page), BASELINE, 'a click moves nothing');

  // Ten round trips on the live page (no reload between), down then up,
  // alternating grip and body: every drop lands where its line was.
  for (let i = 0; i < 10; i++) {
    const grip = i % 2 === 0;
    r = await drag(page, 'discuss', { grip, at: at(page, 'cirow', 0.8) });
    assert.equal(r.post && r.post.applied, true, `round ${i} down applied`);
    assert.equal(await order(page), `mainlead ${KIDS} cirow discuss sosterm`, `round ${i} down`);
    r = await drag(page, 'discuss', { grip: !grip, at: at(page, 'mainlead', 0.2) });
    assert.equal(r.post && r.post.applied, true, `round ${i} up applied`);
    assert.equal(await order(page), BASELINE, `round ${i} up`);
  }
  // The server agrees with what the page showed.
  await page.goto(base + '/');
  await showRail(page);
  assert.equal(await order(page), BASELINE, 'server order after the round trips');
  assert.deepEqual(errors, [], 'no page errors');
  await browser.close();
  console.log('rail drag: ok');
}

// layTop lays the group's top-level rows out in ids' order (mainlead
// keeping its five children), through the layout route, and reloads.
async function layTop(page, ids) {
  for (const id of ['discuss', 'cirow', 'sosterm']) await post(page, { op: 'detach', rid: id });
  for (const id of ['execfour', 'execfive', 'execone', 'exectwo', 'execthree']) await post(page, { op: 'attach', rid: id, parent: 'mainlead' });
  for (let k = 1; k < ids.length; k++) await post(page, { op: 'move', rid: ids[k], after: ids[k - 1] });
  await page.reload();
  await showRail(page);
  await page.waitForTimeout(150);
}

// watchSwaps counts the rail region's swaps (its data-h changes) in
// window.__swaps, kept across reloads by the caller's sums.
async function watchSwaps(page) {
  await page.evaluate(() => {
    window.__swaps = window.__swaps || 0;
    const r = document.getElementById('r-rail');
    new MutationObserver((ms) => { window.__swaps += ms.filter((m) => m.attributeName === 'data-h').length; }).observe(r, { attributes: true });
  });
}

// live: twenty drags, down and up, alternating grip and body, while the
// daemon re-renders the rail. Every drop must land where its line showed;
// the script counts the rail swaps seen during each drag and reports the
// success rate before asserting it is 20/20.
async function live(browser) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: 'dark', locale: 'ko-KR' });
  await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(base + '/');
  await showRail(page);
  await reset(page);
  await watchSwaps(page);
  const beforeLost = await page.evaluate(() => window.__swaps);
  const lost = await drag(page, 'discuss', { at: at(page, 'cirow', 0.75), releaseMove: true, lost: true });
  assert.equal(lost.post, null, 'lost release cancels under live churn');
  await page.waitForTimeout(700);
  assert.ok(await page.evaluate(() => window.__swaps) > beforeLost, 'fallback releases held rail swaps');
  const M = `mainlead ${KIDS}`;
  // Each step aims at a place between two top-level units as the owner
  // sees it, and starts from the order the step before leaves. Steps 1 and
  // 3 aim at the seam between mainlead's children and the next top-level
  // row from the last child's side; the rail shows mainlead with its
  // children as one unit, and a top-level row dropped there lands
  // top-level, never as a child.
  const plan = [
    { from: 'discuss', to: 'execthree', frac: 0.85, start: ['discuss', 'mainlead', 'cirow', 'sosterm'], want: `${M} discuss cirow sosterm`, dir: 'down' },
    { from: 'discuss', to: 'mainlead', frac: 0.2, start: ['mainlead', 'discuss', 'cirow', 'sosterm'], want: BASELINE, dir: 'up' },
    { from: 'sosterm', to: 'execthree', frac: 0.9, start: ['discuss', 'mainlead', 'cirow', 'sosterm'], want: `discuss ${M} sosterm cirow`, dir: 'up' },
    { from: 'sosterm', to: 'cirow', frac: 0.8, start: ['discuss', 'mainlead', 'sosterm', 'cirow'], want: BASELINE, dir: 'down' },
    { from: 'cirow', to: 'mainlead', frac: 0.15, start: ['discuss', 'mainlead', 'cirow', 'sosterm'], want: `discuss cirow ${M} sosterm`, dir: 'up' },
    { from: 'cirow', to: 'sosterm', frac: 0.2, start: ['discuss', 'cirow', 'mainlead', 'sosterm'], want: BASELINE, dir: 'down' },
  ];
  let ok = 0, swapsDuring = 0, swapsTotal = 0, swapsPressed = 0;
  const fails = [];
  for (let i = 0; i < 20; i++) {
    const p = plan[i % plan.length];
    const before = await order(page);
    if (before !== p.start.join(' ').replace('mainlead', M)) {
      await layTop(page, p.start);
      await watchSwaps(page);
    }
    const s0 = await page.evaluate(() => window.__swaps);
    const shot = shots && i < 4 ? `live-during-drag-${p.dir}-${i}.png` : '';
    const r = await drag(page, p.from, { grip: i % 2 === 0, at: at(page, p.to, p.frac), hold: 400, shot });
    const s1 = await page.evaluate(() => window.__swaps);
    await page.waitForTimeout(500);
    const got = await order(page);
    if (shots && i < 4) await page.screenshot({ path: path.join(shots, `live-after-drop-${p.dir}-${i}.png`) });
    swapsDuring += s1 - s0;
    swapsPressed += r.held;
    swapsTotal += await page.evaluate(() => window.__swaps) - s0;
    const good = got === p.want && r.ind.line !== null && !!(r.post && r.post.applied);
    if (good) ok++;
    else fails.push(`#${i} ${p.from}->${p.to}@${p.frac}: line=${r.ind.line}/${r.ind.left} post=${JSON.stringify(r.post)} got="${got}" want="${p.want}"`);
  }
  console.log(`live rail drag: ${ok}/20 landed where the line showed; rail swaps while a button was down ${swapsPressed} (held by the swap gate), by each drop's settle ${swapsDuring}, in all ${swapsTotal}`);
  for (const f of fails) console.log('  miss ' + f);
  assert.ok(swapsTotal > 0, 'the fixture re-rendered the rail meanwhile');
  assert.equal(ok, 20, 'every live drag lands where its line showed');
  assert.deepEqual(errors, [], 'no page errors');
  await browser.close();
  console.log('rail drag live: ok');
}

main().catch((e) => { console.error(e); process.exit(1); });
