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
      const geom = await tab.evaluate(() => {
        const top = document.querySelector('.dwrap > .top');
        const btn = top.querySelector('[data-child-close]');
        const lbl = btn.querySelector('.tph-close-lbl');
        const tr = top.getBoundingClientRect();
        const br = btn.getBoundingClientRect();
        const lr = lbl.getBoundingClientRect();
        const cs = getComputedStyle(btn);
        const lcs = getComputedStyle(lbl);
        return {
          hasTph: btn.classList.contains('tph') && btn.classList.contains('tph-close'),
          topH: tr.height, topB: tr.bottom,
          btnT: br.top, btnH: br.height, btnB: br.bottom,
          scrollW: btn.scrollWidth, clientW: btn.clientWidth,
          whiteSpace: cs.whiteSpace,
          lblDisplay: lcs.display, lblH: lr.height,
        };
      });
      assert.equal(geom.hasTph, true, 'child-close carries .tph and .tph-close classes');
      assert.equal(geom.topH, 34, 'topbar height stays 34px');
      assert.equal(geom.btnT, 0, 'child-close hangs from the top edge');
      assert.equal(geom.btnH, 34, 'child-close height equals --toph 34px');
      assert.ok(geom.btnB <= geom.topB + 0.5, 'child-close bottom does not hang past topbar');
      assert.equal(geom.whiteSpace, 'nowrap', 'child-close stays single-line');
      assert.ok(geom.scrollW <= geom.clientW + 0.5, 'child-close does not overflow horizontally');
      assert.notEqual(geom.lblDisplay, 'none', 'child-close label shows on wide strip');
      assert.ok(geom.lblH <= 20, 'child-close label renders on one line');
      if (shots) await tab.screenshot({path: `${shots}/child-page-${theme}.png`});
      await tab.setViewportSize({width: 460, height: 840});
      await tab.locator('.dwrap > .top.tch').waitFor();
      const compact = await tab.evaluate(() => {
        const top = document.querySelector('.dwrap > .top');
        const btn = top.querySelector('[data-child-close]');
        const lbl = btn.querySelector('.tph-close-lbl');
        const tr = top.getBoundingClientRect();
        const br = btn.getBoundingClientRect();
        return { btnT: br.top, btnW: br.width, btnH: br.height, btnB: br.bottom, topB: tr.bottom, lblDisplay: getComputedStyle(lbl).display };
      });
      assert.equal(compact.btnT, 0, 'compact child-close hangs from top edge');
      assert.equal(compact.btnW, 34, 'compact child-close collapses to 34px handle');
      assert.equal(compact.btnH, 34, 'compact child-close keeps 34px height');
      assert.ok(compact.btnB <= compact.topB + 0.5, 'compact child-close stays inside topbar');
      assert.equal(compact.lblDisplay, 'none', 'compact child-close hides label');
      await tab.setViewportSize({width: 1280, height: 840});
      await tab.locator('.dwrap > .top:not(.tch)').waitFor();
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
