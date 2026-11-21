const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.CHILD_BASE, shots = process.env.WS_SUBAGENT_SHOTS;
(async () => {
  const b = await chromium.launch({headless: true});
  try {
    for (const theme of ['dark', 'light']) {
      const ctx = await b.newContext({viewport: {width: 1280, height: 840}, deviceScaleFactor: 2});
      const tab = await ctx.newPage(), errors = [];
      tab.on('pageerror', e => errors.push(e.message));
      await tab.goto(`${base}/s/boardsession`);
      const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
      const openedRail = !await tab.locator('#gear').isVisible();
      if (openedRail) await tab.locator('#railbtn').click();
      await tab.locator('#gear').click();
      await tab.locator(`#gmenu [data-theme-set="${theme}"]`).click();
      await tab.locator(`html[data-theme="${theme}"]`).waitFor();
      await frame.locator(`html[data-theme="${theme}"]`).waitFor();
      await tab.keyboard.press('Escape');
      if (openedRail) { await tab.keyboard.press('Escape'); await tab.locator('html:not(.rail-open)').waitFor(); }
      const button = tab.locator('[data-child-close]');
      await button.waitFor({state: 'visible'});
      await frame.waitForTimeout(500);
      if (shots) await tab.screenshot({path: `${shots}/child-page-${theme}.png`});
      let calls = 0;
      await tab.route('**/agents/boardsession/close', async route => { calls++; await route.fulfill({status: 200, body: '{}'}); });
      await button.click();
      const dialog = tab.locator('dialog.rdeldlg[open]');
      await dialog.waitFor();
      await dialog.locator('.pbtn:not(.danger)').click();
      assert.equal(calls, 0);
      await button.click(); await dialog.waitFor(); await dialog.locator('.pbtn.danger').click();
      await frame.waitForTimeout(100); assert.equal(calls, 1);
      assert.equal(await button.isDisabled(), true);
      assert.deepEqual(errors, []);
      await ctx.close();
    }
    console.log('child close browser acceptance ok');
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
