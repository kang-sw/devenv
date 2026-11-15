// Run by TestDesktopUpdateBrowser against a disposable fixture daemon. A
// browser tab (no werkschaftDesktop) shows no update row and no toast; a page
// with a stub of the desktop shell's preload bridge shows the settings row and
// the bottom-right toasts and calls back into the bridge. With SHOTS set it
// also writes the review screenshots there.
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base] = process.argv.slice(2);
const shots = process.env.SHOTS || '';

// The stub mirrors dashboard-preload.js: check/download/skip/restart and onStatus.
function stub() {
  window.__calls = [];
  window.__emit = s => window.__listener && window.__listener(s);
  window.werkschaftDesktop = { update: {
    check() { window.__calls.push('check'); },
    download() { window.__calls.push('download'); },
    skip(v) { window.__calls.push('skip:' + v); },
    restart() { window.__calls.push('restart'); },
    onStatus(cb) { window.__listener = cb; window.__calls.push('subscribe'); },
  } };
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const plain = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale: 'en-US' });
    const errors = [];
    plain.on('pageerror', e => errors.push(e.message));
    await plain.goto(base + '/');
    await plain.locator('#gear').click();
    await plain.locator('#gmenu').waitFor({ state: 'visible' });
    assert.equal(await plain.locator('#dupd-check').count(), 0, 'a browser tab has no update row');
    assert.equal(await plain.locator('.wstoast').count(), 0, 'a browser tab has no update toast');
    if (shots) await plain.screenshot({ path: path.join(shots, 'settings-browser-tab-light.png') });
    await plain.close();

    for (const [scheme, locale] of [['dark', 'ko-KR'], ['light', 'ko-KR'], ['dark', 'en-US']]) {
      const page = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale, colorScheme: scheme });
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(stub);
      await page.goto(base + '/');
      await page.waitForFunction(() => window.__calls.includes('subscribe'));
      const shot = async name => { if (shots && locale === 'ko-KR') await page.screenshot({ path: path.join(shots, name) }); };
      const calls = () => page.evaluate(() => window.__calls.slice());
      const emit = s => page.evaluate(s => window.__emit(s), s);

      // Settings: the row shows, with the standing phase beside the button.
      await emit({ state: 'phase', phase: { phase: 'none', version: null, skipped: false } });
      await page.locator('#gear').click();
      await page.locator('#dupd-check').waitFor({ state: 'visible' });
      assert.equal(await page.locator('#dupd-restart').isVisible(), false, 'no restart before an update is ready');
      const label = await page.locator('#dupd-check .gs').textContent();
      assert.ok(label.length > 0, 'the none phase has a line');
      await shot(`settings-check-button-${scheme}.png`);
      await page.locator('#dupd-check').click();
      assert.ok((await calls()).includes('check'));
      await shot(`settings-state-none-${scheme}.png`);
      await emit({ state: 'phase', phase: { phase: 'available', version: '0.51.0', skipped: false } });
      assert.match(await page.locator('#dupd-check .gs').textContent(), /0\.51\.0/);
      await shot(`settings-state-available-${scheme}.png`);
      await emit({ state: 'phase', phase: { phase: 'ready', version: '0.51.0', skipped: false } });
      await page.locator('#dupd-restart').waitFor({ state: 'visible' });
      await shot(`settings-state-ready-${scheme}.png`);
      await page.locator('#dupd-restart').click();
      assert.equal((await calls()).at(-1), 'restart');
      await page.keyboard.press('Escape');
      await page.locator('body').click({ position: { x: 1000, y: 500 } });

      // Available: text, [Update], [Skip this version], and a dismiss button.
      await emit({ state: 'available', version: '0.51.0', current: '0.50.0', skipped: false,
        phase: { phase: 'available', version: '0.51.0', skipped: false } });
      const toast = page.locator('.wstoast').first();
      await toast.waitFor({ state: 'visible' });
      assert.match(await toast.locator('.wst-body').textContent(), /v0\.51\.0/);
      assert.equal(await toast.locator('.wst-act').count(), 2);
      assert.equal(await toast.locator('.wst-x').count(), 1);
      const box = await toast.boundingBox();
      assert.ok(box.x + box.width > 1800 - 40 && box.y + box.height > 1000 - 60, 'the toast sits bottom-right');
      await shot(`toast-new-${scheme}.png`);
      await page.waitForTimeout(5000);
      assert.equal(await page.locator('.wstoast').count(), 1, 'the update toast stays until acted on');
      await toast.locator('.wst-act').nth(1).click();
      assert.equal((await calls()).at(-1), 'skip:0.51.0');
      assert.equal(await page.locator('.wstoast').count(), 0);
      await emit({ state: 'available', version: '0.51.0', skipped: true, phase: { phase: 'available', version: '0.51.0', skipped: true } });
      await page.locator('.wstoast .wst-act').first().click();
      assert.equal((await calls()).at(-1), 'download');

      // Ready: the restart button, and the full-quit/reboot note in the text.
      await emit({ state: 'downloading', version: '0.51.0', phase: { phase: 'available', version: '0.51.0', skipped: false } });
      await emit({ state: 'ready', version: '0.51.0', phase: { phase: 'ready', version: '0.51.0', skipped: false } });
      assert.equal(await page.locator('.wstoast').count(), 1, 'a new status replaces the previous update toast');
      assert.equal(await page.locator('.wstoast .wst-act').count(), 1);
      await shot(`toast-ready-${scheme}.png`);
      await page.locator('.wstoast .wst-act').click();
      assert.equal((await calls()).at(-1), 'restart');

      await emit({ state: 'current', version: '0.50.0', manual: true, phase: { phase: 'none', version: null, skipped: false } });
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /v0\.50\.0/);
      await shot(`toast-uptodate-${scheme}.png`);
      await emit({ state: 'error', manual: true, phase: { phase: 'none', version: null, skipped: false } });
      assert.equal(await page.locator('.wstoast.warn').count(), 1);
      await shot(`toast-error-${scheme}.png`);
      await page.close();
    }
    assert.deepEqual(errors, []);
    console.log('desktop update browser acceptance ok');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exit(1); });
