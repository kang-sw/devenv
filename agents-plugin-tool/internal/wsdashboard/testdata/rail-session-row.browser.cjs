// TestRailSessionRowBrowser: real served UI against a disposable daemon.
// Only session creation is a canned agent-host boundary; no harness starts.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);
const row = id => `#r-rail .rs[data-rid="${id}"]`;
const menu = '.rpop.rmenu:not([hidden])';
const marker = 'ws.dashboard.renameNew';

async function until(fn, message) {
  for (let i = 0; i < 150; i++) {
    if (await fn()) return;
    await new Promise(r => setTimeout(r, 50));
  }
  assert.fail(message);
}
async function showRail(page) {
  await page.locator(row('plainrow')).waitFor({ state: 'attached' });
  if (!await page.locator(row('plainrow')).isVisible()) await page.locator('#railbtn').click();
  try { await page.locator(row('plainrow')).waitFor({ timeout: 5000 }); }
  catch (e) {
    await shot(page, 'failure-rail');
    console.error('RAIL STATE:', await page.evaluate(() => ({ root: document.documentElement.outerHTML.slice(0, 800), rail: document.querySelector('#rail').getBoundingClientRect().toJSON() })));
    throw e;
  }
}
async function shot(page, name) {
  if (!shots) return;
  // Keep the rename focused, but clear incidental hover cards and the
  // deliberately triggered failure toast before capturing owner evidence.
  const { width, height } = page.viewportSize();
  await page.mouse.move(width - 5, height - 5);
  await page.waitForFunction(() => !document.querySelector('.rpop.shp:not([hidden]), .wstoast'));
  await page.screenshot({ path: path.join(shots, name + '.png'), scale: 'css' });
}
async function openMenu(page, id, keyboard = false) {
  if (keyboard) {
    await page.locator(row(id) + ' .rl').focus();
    await page.keyboard.press('Shift+F10');
  } else await page.locator(row(id) + ' .rid').click({ button: 'right' });
  await page.locator(menu).waitFor();
}
async function toggle(page, id, on, keyboard = false) {
  await openMenu(page, id, keyboard);
  const item = page.locator(menu).getByRole('menuitem', { name: on ? '즐겨찾기에 추가' : '즐겨찾기 해제', exact: true });
  assert.equal(await item.getAttribute('data-on'), String(!on));
  if (keyboard) {
    // Rename -> copy -> favorite, using the menu's existing arrow path.
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    assert.equal(await item.evaluate(e => e === document.activeElement), true);
    await page.keyboard.press('Enter');
  } else await item.click();
  await page.locator(menu).waitFor({ state: 'hidden' });
}
async function order(page) {
  return page.locator(row('plainrow')).evaluate(e => [...e.parentNode.children].filter(e => e.classList.contains('rs') && !e.classList.contains('ended')).map(e => e.dataset.rid));
}
async function create(page) {
  await showRail(page);
  await page.locator('#r-rail .rnew').first().click();
  const response = page.waitForResponse(r => r.url().endsWith('/agents/leads') && r.request().method() === 'POST');
  await page.locator('.rpop:not([hidden]) .go').click();
  const r = await response;
  assert.equal(r.status(), 201);
  const { id } = await r.json();
  await page.waitForURL(base + '/s/' + id);
  return id;
}
async function editing(page, id, wide) {
  const input = page.locator(wide ? row(id) + ' .drinp' : '#r-head .sinp');
  await input.waitFor();
  await until(() => input.evaluate(e => e === document.activeElement), 'new-session rename must keep focus after shell navigation');
  assert.deepEqual(await input.evaluate(e => [e.value, e.placeholder, e.selectionStart, e.selectionEnd]), ['', id, 0, 0], 'default name stays a placeholder; empty text is selected');
  assert.equal(await page.evaluate(key => sessionStorage.getItem(key), marker), null, 'creation intent consumed once');
  return input;
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    if (shots) fs.mkdirSync(shots, { recursive: true });
    const errors = [];
    for (const theme of ['light', 'dark']) {
      const ctx = await browser.newContext({ viewport: { width: 1680, height: 900 }, locale: 'ko-KR', colorScheme: theme, reducedMotion: 'reduce' });
      await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
      const page = await ctx.newPage();
      page.on('pageerror', e => { errors.push(theme + ': ' + e.message); console.error('PAGE ERROR:', e.message); });
      await page.goto(base + '/s/plainrow');
      await showRail(page);
      assert.equal(await page.locator('#r-rail button.star').count(), 0, 'no inline favorite button');
      assert.equal(await page.locator('#r-rail .drinp, #r-head .sinp').count(), 0, 'opening an existing session does not rename');
      // Saved ordering and grouping still come from the existing layout route.
      await toggle(page, 'agentsrow', true);
      await until(async () => (await order(page))[0] === 'agentsrow', 'favorite moves to the front');
      const badge = page.locator(row('agentsrow') + ' .hstar');
      await badge.waitFor();
      assert.equal(await badge.getAttribute('aria-label'), '즐겨찾기');
      assert.equal(await badge.getAttribute('title'), '즐겨찾기');
      assert.equal(await badge.locator('button').count(), 0, 'badge is display-only');
      const geometry = await badge.evaluate(e => {
        const b = e.getBoundingClientRect(), i = e.parentNode.querySelector('.hb').getBoundingClientRect();
        return [b.width, b.height, b.left - i.left, b.bottom - i.bottom];
      });
      assert.deepEqual(geometry, [10, 10, -4, 4], 'badge matches mockup bottom-left geometry');
      const gap = id => page.locator(row(id)).evaluate(e => e.querySelector('.rid').getBoundingClientRect().left - e.querySelector('.hb').getBoundingClientRect().right);
      assert.equal(await gap('plainrow'), 6, 'no-count gap equals mockup 6px');
      assert.equal(await gap('kidrow'), 6, 'child no-count gap equals mockup 6px');
      assert.ok(await gap('agentsrow') > 6, 'one-digit count pushes the name');
      assert.ok(await gap('dotrow') > await gap('agentsrow'), 'two-digit count pushes farther');
      await openMenu(page, 'agentsrow');
      assert.equal(await page.locator(menu).getByRole('menuitem', { name: '즐겨찾기 해제', exact: true }).count(), 1);
      await page.keyboard.press('Escape');
      // A page refresh keeps the favorite but never starts rename.
      await page.reload();
      await showRail(page);
      await badge.waitFor();
      assert.equal(await page.locator('.drinp, #r-head .sinp').count(), 0);
      // Overlay widths: all four favorite/count combinations (including
      // the child with neither), and the open context menu.
      await toggle(page, 'plainrow', true);
      await until(async () => (await order(page))[0] === 'plainrow', 'favorite without count leads the screenshot rows');
      for (const width of [1280, 760]) {
        await page.setViewportSize({ width, height: 900 });
        await showRail(page);
        await page.mouse.move(width - 5, 895);
        await shot(page, `rows-${theme}-${width}`);
        await openMenu(page, 'agentsrow');
        await shot(page, `menu-${theme}-${width}`);
        await page.keyboard.press('Escape');
      }
      await toggle(page, 'plainrow', false);
      await until(async () => (await order(page))[0] === 'agentsrow', 'snapshot favorite cleared');
      // Child and archived rows retain their original favorite restrictions.
      await openMenu(page, 'kidrow');
      assert.equal(await page.locator(menu + ' .rmstar').count(), 0, 'no child favorite item');
      await page.keyboard.press('Escape');
      assert.equal(await page.locator(row('archivedrow')).getAttribute('data-star'), null, 'archived rows cannot favorite');
      await toggle(page, 'agentsrow', false, true);
      await until(async () => await badge.count() === 0, 'unfavorite removes badge');
      await until(async () => (await order(page))[0] === 'plainrow', 'unfavorite restores normal ordering');
      // Refusal: badge appears optimistically and is then rolled back.
      let release;
      await page.route('**/rail/layout', route => new Promise(resolve => {
        release = async () => { await route.fulfill({ status: 409, contentType: 'application/json', body: '{"applied":false,"reason":"store"}' }); resolve(); };
      }));
      await toggle(page, 'agentsrow', true);
      await until(async () => typeof release === 'function', 'favorite POST in flight');
      assert.equal(await badge.count(), 1, 'optimistic badge');
      await release();
      await until(async () => await badge.count() === 0, 'failed favorite restores badge');
      await until(async () => await page.locator(row('agentsrow')).getAttribute('data-star') === '0', 'failed favorite restores row state');
      await page.unroute('**/rail/layout');
      // Wide rail: new row at the end, empty input focused (not reply-id
      // as a label), and default name retained on blur/Escape/empty Enter.
      await page.setViewportSize({ width: 1680, height: 900 });
      let id = await create(page);
      let input = await editing(page, id, true);
      assert.equal((await order(page)).at(-1), id, 'new row appended at group end');
      await shot(page, `rename-${theme}-1680`);
      await input.fill('discard on blur');
      await input.evaluate(e => e.blur());
      assert.equal(await page.locator(row(id) + ' .rid').textContent(), id, 'blur keeps default name');
      // Another tab sees the session but has no creation intent.
      const peer = await ctx.newPage();
      await peer.goto(base + '/s/' + id);
      await peer.locator('#r-head .sname').waitFor();
      assert.equal(await peer.locator('.drinp, #r-head .sinp').count(), 0, 'sessions created elsewhere do not rename');
      await peer.close();
      id = await create(page);
      input = await editing(page, id, true);
      await input.press('Enter');
      await until(async () => await page.locator('.drinp').count() === 0, 'empty Enter closes rename');
      assert.equal(await page.locator(row(id) + ' .drren').getAttribute('data-label'), '', 'empty Enter stores no default label');
      // The shared rename editor also selects the whole existing label.
      await page.locator(row(id) + ' .rid').hover();
      await page.locator(row(id) + ' .drren').click();
      await page.locator(row(id) + ' .drinp').fill('selected name');
      await page.locator(row(id) + ' .drinp').press('Enter');
      await until(async () => await page.locator(row(id) + ' .rid').textContent() === 'selected name', 'rename saved');
      await page.locator(row(id) + ' .rid').hover();
      await page.locator(row(id) + ' .drren').click();
      input = page.locator(row(id) + ' .drinp');
      await input.waitFor();
      assert.deepEqual(await input.evaluate(e => [e.selectionStart, e.selectionEnd]), [0, 'selected name'.length], 'existing label fully selected');
      await input.press('Escape');
      // Collapse defaults to the shell header and closes the overlay.
      for (const width of [1280, 760]) {
        await page.setViewportSize({ width, height: 900 });
        id = await create(page);
        input = await editing(page, id, false);
        assert.equal(await page.locator(row(id)).isVisible(), false, 'collapsed rail stays closed');
        await shot(page, `rename-${theme}-${width}`);
        if (width === 1280) {
          await input.fill('저장한 세션');
          const saved = page.waitForResponse(r => r.url().endsWith('/s/' + id + '/label') && r.request().method() === 'POST');
          await input.press('Enter');
          assert.equal((await saved).status(), 200);
          await until(async () => await page.locator('#r-head .snm').textContent() === '저장한 세션', 'header rename saved');
        } else {
          await input.fill('discard on escape');
          await input.press('Escape');
          assert.equal(await page.locator('#r-head .snm').textContent(), id, 'Escape keeps default name');
        }
        await page.reload();
        await page.locator('#r-head .sname').waitFor();
        assert.equal(await page.locator('.drinp, #r-head .sinp').count(), 0, 'reload never repeats consumed rename');
      }
      // Even an unconsumed marker is discarded on reload, and stale intent
      // is discarded when the rail next marks its rows.
      await page.evaluate(({ key, id }) => sessionStorage.setItem(key, JSON.stringify({ id, at: Date.now() })), { key: marker, id });
      await page.reload();
      await page.locator('#r-head .sname').waitFor();
      assert.equal(await page.locator('.drinp, #r-head .sinp').count(), 0, 'reload discards unconsumed creation intent');
      assert.equal(await page.evaluate(key => sessionStorage.getItem(key), marker), null);
      await page.evaluate(({ key, id }) => sessionStorage.setItem(key, JSON.stringify({ id, at: Date.now() - 11000 })), { key: marker, id });
      await showRail(page);
      await page.locator(row('plainrow') + ' .kidtog').click();
      await until(async () => await page.evaluate(key => sessionStorage.getItem(key), marker) === null, 'expired intent cleared');
      assert.equal(await page.locator('.drinp, #r-head .sinp').count(), 0, 'expired creation intent never opens rename');
      await ctx.close();
    }
    assert.deepEqual(errors, [], 'no browser page errors');
    console.log('PASS: favorite menu/badge/persistence/rollback, 6px count-free spacing, one-shot rename after create (wide/header), cancel/save/reload/tab boundaries');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
