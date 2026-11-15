// Run through TestModelHistoryEntriesBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of [1440, 420]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US' });
      const tab = await context.newPage(), errors = [], events = [];
      tab.on('pageerror', e => errors.push(e.message));
      tab.on('websocket', ws => ws.on('framereceived', f => {
        try { events.push(JSON.parse(f.payload)); } catch {}
      }));
      assert.equal((await fetch(updates)).status, 204);
      await tab.goto(base + '/s/livesession');
      const host = tab.locator('iframe.sframe.on');
      await host.waitFor({ state: 'visible' });
      const page = await (await host.elementHandle()).contentFrame();
      await page.locator('#r-entries .centries [data-entry]').first().waitFor({ state: 'attached' });
      if (!(await page.locator('#convo').isVisible())) await page.locator('[data-tab="convo"]').first().click();
      // The initial stream sends mode; entries already match the page's held hash.
      const deadline = Date.now() + 15000;
      while (!events.some(e => e.event === 'region' && e.data.name === 'mode')) {
        assert(Date.now() < deadline, 'initial session mode delivered');
        await new Promise(resolve => setTimeout(resolve, 20));
      }
      assert.equal(await page.locator('#r-entries .modelhistory').count(), 0, 'absent at load');
      await page.evaluate(() => {
        window.originalBox = document.querySelector('#r-entries .centries');
        window.originalEntries = [...window.originalBox.children];
      });
      const history = page.locator('#r-entries .modelhistory');
      async function update(model) {
        const hash = await page.locator('#r-entries').getAttribute('data-h');
        assert.equal((await fetch(updates + '?model=' + encodeURIComponent(model))).status, 204);
        await page.waitForFunction(h => document.querySelector('#r-entries').getAttribute('data-h') !== h, hash);
        assert.equal(await page.evaluate(() => {
          const box = document.querySelector('#r-entries .centries');
          return box === window.originalBox && box.children.length === window.originalEntries.length &&
            window.originalEntries.every((n, i) => box.children[i] === n);
        }), true, 'entries container and every existing entry retain identity');
      }
      async function setOpen(open) {
        // Wait for the native toggle, which is the existing openState owner.
        await history.evaluate((d, value) => new Promise(resolve => {
          d.addEventListener('toggle', resolve, { once: true });
          d.open = value;
        }), open);
      }
      async function checkModel(model, open) {
        assert.equal(await history.count(), 1, 'exactly one model history block');
        assert.match(await history.textContent(), new RegExp(model), 'current rows rendered');
        assert.equal(await history.evaluate(d => d.open), open, 'disclosure state retained');
        assert.equal(await history.evaluate(d => d.previousElementSibling.matches('.thead') &&
          d.nextElementSibling.matches('.centries')), true, 'template sibling order retained');
        assert.equal(await history.locator('time[data-ms]').evaluateAll(times => times.length > 0 &&
          times.every(t => t.wsFmt && t.title)), true, 'new timestamps formatted');
      }
      await update('first-model');
      await checkModel('first-model', false);
      await setOpen(true);
      await page.evaluate(() => { window.previousHistory = document.querySelector('#r-entries .modelhistory'); });
      await update('second-model');
      await checkModel('second-model', true);
      assert.equal(await page.evaluate(() => window.previousHistory !== document.querySelector('#r-entries .modelhistory')), true, 'existing block replaced');
      assert(!(await history.textContent()).includes('first-model'), 'old rows removed');
      await setOpen(false);
      await update('third-model');
      await checkModel('third-model', false);
      assert(!(await history.textContent()).includes('second-model'), 'closed block refreshed too');
      await setOpen(true);
      await update('');
      assert.equal(await history.count(), 0, 'history removed when absent from payload');
      await update('restored-model');
      await checkModel('restored-model', true);
      assert.deepEqual(errors, [], 'no page errors');
      console.log(width + ': entries region add/replace/remove, open/closed state, formatted times and entry identity passed');
      await context.close();
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
