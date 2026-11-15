// Relevant, offline behavior checks only; no visual matrix or runtime install.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, emptyBase] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1700, height: 900 }, serviceWorkers: 'allow', reducedMotion: 'reduce' });
    context.setDefaultTimeout(10000);
    // Playwright's built-in blocker throws in opaque sandboxed topic frames.
    // Match the existing dashboard fixtures: forbid registration without
    // reading Navigator.serviceWorker in an opaque origin.
    await context.addInitScript(() => {
      if (self.origin === 'null') return;
      const workers = navigator.serviceWorker;
      if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('Service workers forbidden in isolated Settings fixture'); };
    });
    const errors = [];
    const at = Date.now();
    const entries = [{ harness: 'claude', asOf: at, windows: [
      { kind: 'five_hour', percent: 20, asOf: at, resetsAt: at + 3 * 3600000 },
      { kind: 'seven_day', percent: 53, asOf: at, resetsAt: at + 4 * 86400000 }
    ] }, { harness: 'codex', asOf: at, windows: [{ kind: 'seven_day', percent: 38, asOf: at, resetsAt: at + 4 * 86400000 }] }];
    await context.route('**/*', route => {
      const url = new URL(route.request().url());
      if (![base, emptyBase].includes(url.origin)) return route.abort();
      if (url.pathname === '/rail/usage') return route.fulfill({ json: { entries: url.origin === emptyBase ? [] : entries, warnPercent: 80, critPercent: 95 } });
      return route.continue();
    });
    const page = await context.newPage();
    page.on('pageerror', e => errors.push(e.message));
    const current = () => page.frameLocator('iframe.sframe.on');
    const gear = page.locator('.settings-toggle');
    const dialog = page.locator('#gmenu');
    async function openSettings() {
      if (!await gear.isVisible()) await page.locator('#railbtn').click();
      await gear.click();
      await dialog.waitFor({ state: 'visible' });
    }
    async function rootIsEmpty() {
      await current().locator('#grid').waitFor();
      assert.equal(await page.locator('html').getAttribute('data-rid'), '');
      assert.equal(await page.locator('.rs.cur').count(), 0);
      assert.equal(await current().locator('#grid').evaluate(e => e.children.length), 0);
      assert.equal(await current().locator('[data-region="sessions"], #finding, #composer').count(), 0);
    }
    await page.goto(base + '/');
    await rootIsEmpty();
    await page.locator('.usage-number').first().waitFor();
    assert.equal(await gear.count(), 1);
    assert.equal(await current().locator('#gear, #gmenu, #p-settings').count(), 0);
    await gear.focus(); await page.keyboard.press('Enter');
    await dialog.waitFor({ state: 'visible' });
    assert.equal(await gear.getAttribute('aria-expanded'), 'true');
    assert.equal(await dialog.evaluate(e => e === document.activeElement), true);
    await page.keyboard.press('Escape'); await dialog.waitFor({ state: 'hidden' });
    assert.equal(await gear.evaluate(e => e === document.activeElement), true);
    await openSettings(); await page.locator('#settings-close').click();
    assert.equal(await gear.evaluate(e => e === document.activeElement), true);
    await openSettings(); await page.mouse.click(1600, 700);
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(await gear.getAttribute('aria-expanded'), 'false');

    await page.locator('.rl[href="/s/livesession"]').click();
    await current().locator('#ctext').waitFor();
    assert.equal(await page.locator('html').getAttribute('data-rid'), 'livesession');
    const oldHint = await current().locator('#ctext').getAttribute('placeholder');
    await openSettings();
    await page.locator('#gmenu [data-layout="tabs"]').click();
    await page.locator('#gmenu [data-input="mod"]').click();
    await page.locator('#gmenu [data-theme-set="dark"]').click();
    await current().locator('#grid[data-layout="tabs"]').waitFor();
    await current().locator('html[data-theme="dark"]').waitFor();
    await page.waitForFunction(old => document.querySelector('.sframe.on').contentDocument.getElementById('ctext').placeholder !== old, oldHint);
    assert.equal(await page.evaluate(() => localStorage.getItem('ws.dashboard.input')), 'mod');
    assert.equal(await page.locator('#gmenu [data-layout="tabs"]').getAttribute('aria-pressed'), 'true');
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
    await page.keyboard.press('Escape');
    await current().locator('#ctext').fill('Preserve this cached draft');
    await page.goBack();
    await page.waitForURL(base + '/');
    await rootIsEmpty();
    await openSettings();
    await page.locator('#glang').selectOption('ko');
    assert.equal((await context.cookies()).find(c => c.name === 'ws_dashboard_lang').value, 'ko');
    // The hidden live frame's unsent text must hold the shell reload.
    assert.equal(await page.locator('html').getAttribute('data-lang'), 'en');
    await page.keyboard.press('Escape');
    await page.locator('.rl[href="/s/livesession"]').click();
    await current().locator('#ctext').waitFor();
    assert.equal(await current().locator('#ctext').inputValue(), 'Preserve this cached draft');
    await current().locator('#ctext').fill('');
    await page.reload();
    await current().locator('#grid[data-layout="tabs"]').waitFor();
    assert.equal(await page.locator('html').getAttribute('data-lang'), 'ko');
    await openSettings();
    assert.equal(await gear.getAttribute('aria-label'), '설정');
    assert.equal(await page.locator('#glang').inputValue(), 'ko');
    assert.equal(await page.locator('#gmenu [data-input="mod"]').getAttribute('aria-pressed'), 'true');
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
    await page.keyboard.press('Escape');

    await page.setViewportSize({ width: 360, height: 760 });
    await openSettings();
    assert.equal(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth), true);
    assert.equal(await page.locator('.rfoot').evaluate(e => e.scrollWidth <= e.clientWidth), true);
    const footer = await page.locator('.rfoot').boundingBox();
    const gearBox = await gear.boundingBox();
    const meterBox = await page.locator('.usage-column').first().boundingBox();
    assert.ok(meterBox.height <= 18, 'meter is approximately one text line');
    assert.ok(gearBox.x >= footer.x && gearBox.x < meterBox.x, 'gear is left of usage');
    await page.locator('#settings-close').click();
    await page.locator('.usage-toggle').click();
    await page.getByRole('dialog', { name: 'Plan usage', exact: true }).waitFor({ state: 'visible' });
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('.usage-toggle').getAttribute('aria-expanded'), 'false');

    await page.goto(emptyBase + '/');
    await rootIsEmpty();
    await openSettings();
    assert.equal(await page.locator('.settings-version').count(), 1);
    assert.equal(await page.locator('.lanbox').count(), 0);
    assert.equal(await page.locator('.usage-column').count(), 0);
    await page.keyboard.press('Escape');
    assert.deepEqual(context.serviceWorkers(), []);
    assert.deepEqual(errors, []);
    console.log('Settings/root: keyboard, focus, dismissal, frame preferences, cookies/reload, cached draft, empty selection, narrow geometry, usage interaction and no-session reachability passed.');
    await context.close();
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
