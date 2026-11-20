// Run through TestModelHistoryEntriesBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);
const screenshots = process.env.MODEL_HISTORY_SCREENSHOTS || '';
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
      assert.equal(await page.locator('.modelhistory-trigger').count(), 0, 'trigger absent at load');
      assert.equal(await page.locator('.modelhistory-popover').count(), 0, 'popover absent at load');
      await page.evaluate(() => {
        window.originalBox = document.querySelector('#r-entries .centries');
        window.originalEntries = [...window.originalBox.children];
      });
      const trigger = page.locator('.modelhistory-trigger');
      const history = page.locator('.modelhistory-popover');
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
      async function checkModel(model, open) {
        assert.equal(await trigger.count(), 1, 'exactly one title-bar trigger');
        assert.equal(await page.locator('#r-entries > .modelhistory').count(), 0, 'no model-history block above entries');
        assert.equal(await page.evaluate(() => {
          const root = document.querySelector('#r-entries'), head = root.querySelector(':scope > .thead'), box = root.querySelector(':scope > .centries');
          return head.nextElementSibling === box;
        }), true, 'conversation entries start directly below the title bar');
        assert.match(await page.locator('template.modelhistory-source').evaluate(t => t.innerHTML), new RegExp(model), 'current source rows rendered');
        assert.equal(await trigger.getAttribute('aria-expanded'), String(open), 'trigger state retained');
        assert.equal(await history.isVisible().catch(() => false), open, 'popover visibility retained');
        if (!open) return;
        assert.match(await history.textContent(), new RegExp(model), 'open popover has current rows');
        assert.equal(await history.locator('time[data-ms]').evaluateAll(times => times.length > 0 &&
          times.every(t => t.wsFmt && t.title)), true, 'new timestamps formatted');
        if (open) assert.equal(await history.locator('.brow').evaluateAll(rows => rows.length > 0 && rows.every(r => {
          const bp = r.querySelector('.bp'), bm = r.querySelector('.bm');
          return bp.scrollWidth <= bp.clientWidth && Math.abs(bp.getBoundingClientRect().top - bm.getBoundingClientRect().top) < 4;
        })), true, 'model row label unclipped, time on the same row');
      }
      await update('first-model');
      await checkModel('first-model', false);
      await trigger.hover();
      assert.equal(await history.count(), 0, 'hover alone does not create or open the popover');
      if (screenshots && width === 1440) {
        fs.mkdirSync(screenshots, { recursive: true });
        await tab.screenshot({ path: path.join(screenshots, 'closed.png') });
      }
      await trigger.click();
      await history.waitFor({ state: 'visible' });
      await checkModel('first-model', true);
      if (screenshots && width === 1440) await tab.screenshot({ path: path.join(screenshots, 'open.png') });
      await tab.keyboard.press('Escape');
      await checkModel('first-model', false);
      assert.equal(await trigger.evaluate(b => document.activeElement === b), true, 'Escape returns focus to trigger');
      await trigger.click();
      await page.evaluate(() => { window.previousHistory = document.querySelector('.modelhistory-popover'); });
      await update('second-model');
      await checkModel('second-model', true);
      assert.equal(await page.evaluate(() => window.previousHistory === document.querySelector('.modelhistory-popover')), true, 'open popover node retained');
      assert(!(await history.textContent()).includes('first-model'), 'old rows removed');
      await page.locator('#r-entries .centries [data-entry]').first().evaluate(n => n.click());
      await checkModel('second-model', false);
      await update('third-model');
      await checkModel('third-model', false);
      await trigger.click();
      await checkModel('third-model', true);
      assert(!(await history.textContent()).includes('second-model'), 'closed source refreshed too');
      await update('');
      assert.equal(await trigger.count(), 0, 'trigger removed when history is absent');
      assert.equal(await history.isVisible(), false, 'open popover hidden when history becomes absent');
      await update('restored-model');
      await checkModel('restored-model', true);
      await tab.keyboard.press('Escape');
      assert.deepEqual(errors, [], 'no page errors');
      console.log(width + ': click-only model-history popover, refresh/open state, Escape/outside close, shell order and entry identity passed');
      await context.close();
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
