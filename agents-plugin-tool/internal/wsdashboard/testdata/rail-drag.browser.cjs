// Run by TestRailDragReorderBrowser against disposable fixture sessions.
// argv: the fixture daemon's base URL, and an optional absolute directory
// for the during/after drag screenshots.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

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
  if (!(await page.locator(row('discuss')).isVisible())) {
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
async function drag(page, from, { grip = false, at, hold = 60, shot = '' }) {
  const fb = await box(page, row(from) + (grip ? ' .drgrip' : ' .rid'));
  const sx = fb.x + fb.width / 2, sy = fb.y + Math.min(fb.height / 2, 10);
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  const [tx, ty] = await at();
  await page.mouse.move(sx, sy + (ty > sy ? 8 : -8), { steps: 2 });
  await page.mouse.move(tx, ty, { steps: 10 });
  await page.waitForTimeout(hold);
  const ind = await indicator(page);
  if (shot && shots) await page.screenshot({ path: path.join(shots, shot) });
  const resp = page.waitForResponse((r) => r.url().endsWith('/rail/layout') && r.request().method() === 'POST', { timeout: 1500 }).then((r) => r.json()).catch(() => null);
  await page.mouse.up();
  const v = await resp;
  await page.waitForTimeout(250);
  return { ind, post: v };
}
// mid answers a point at frac of row id's height, at its middle x.
const at = (page, id, frac) => async () => {
  const b = await box(page, row(id));
  return [b.x + b.width / 2, b.y + b.height * frac];
};

async function main() {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: 'dark', locale: 'ko-KR' });
  await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(base + '/');
  await showRail(page);
  await reset(page);

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
  // Below the last row, in the group's empty space: after the last.
  r = await drag(page, 'mainlead', { at: async () => { const b = await box(page, row('sosterm')); return [b.x + b.width / 2, b.y + b.height + 6]; } });
  assert.equal(r.post && r.post.applied, true, 'below-last drop applied');
  assert.equal(await order(page), `cirow discuss sosterm mainlead ${KIDS}`, 'below the last row: lands last, children along');

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

main().catch((e) => { console.error(e); process.exit(1); });
