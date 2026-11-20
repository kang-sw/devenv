// Run by TestAgyOutputRateHoverAndRowBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, capturePath] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const ctx = await browser.newContext({
      viewport: { width: 1800, height: 1000 },
      locale: 'ko-KR',
      extraHTTPHeaders: { 'Accept-Language': 'ko-KR,ko;q=0.9' },
    });
    await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));

    await page.goto(base + '/s/livesession');

    // Open the parent's agents popover inside the session iframe and verify
    // the 2nd-turn agy subagent row rate.
    const iframeEl = await page.locator('iframe.sframe.on').elementHandle();
    const frame = await iframeEl.contentFrame();
    await frame.locator('#sbadge:not([hidden])').waitFor({ state: 'visible' });
    await frame.click('#sbadge');
    const rowTps = frame.locator('#agpop .atps').first();
    await rowTps.waitFor({ state: 'visible' });
    const rowTpsText = (await rowTps.textContent()).trim();
    assert.equal(rowTpsText, '70 t/s', 'parent subagent row shows 2nd-turn agy rate');

    // Focus the agy subagent's own rail row on the shell page to open its hover card.
    const agyRow = page.locator('#r-rail .rs[data-rid="agychild"]');
    await agyRow.waitFor({ state: 'visible' });
    await agyRow.locator('.rl').focus();
    const popup = page.locator('.shp');
    await popup.waitFor({ state: 'visible' });

    const hoverText = await popup.innerText();
    assert.ok(hoverText.includes('출력 속도'), `hover lacks 출력 속도: ${hoverText}`);
    assert.ok(hoverText.includes('알 수 없음'), `hover observed model not unknown: ${hoverText}`);
    const tpsText = (await popup.locator('.shp-stat b.tps').textContent()).trim();
    assert.equal(tpsText, '70', `hover rate = ${tpsText}, want 70`);

    if (capturePath) {
      await page.screenshot({ path: capturePath });
    }
    assert.deepEqual(errors, []);
    console.log('PASS: agy subagent 2nd-turn row (70 t/s) and hover card (출력 속도 70, observed model 알 수 없음) verified');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
