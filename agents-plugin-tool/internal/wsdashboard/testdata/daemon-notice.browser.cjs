// TestDaemonNoticeBrowser starts the disposable daemon. SHOTS is optional.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base] = process.argv.slice(2);
const shots = process.env.SHOTS;

function stub() {
  window.werkschaftDesktop = { daemon: { onNotice(callback) { window.__showDaemonNotice = callback; } } };
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const reason = 'The starter refused replacement. Retry after checking daemon health.';
    const freshPage = () => browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: 'light', locale: 'en-US' });
    const settings = async page => {
      if (!await page.locator('#gear').isVisible()) await page.locator('#railbtn').click();
      await page.locator('#gear').click();
      await page.locator('#gmenu').waitFor({ state: 'visible' });
    };
    const capture = async (page, name) => {
      if (!shots) return;
      fs.mkdirSync(shots, { recursive: true });
      await page.screenshot({ path: path.join(shots, name) });
    };
    const oldShell = await freshPage();
    await oldShell.addInitScript(() => { window.werkschaftDesktop = { daemon: { availability() {} } }; });
    await oldShell.goto(base + '/');
    await settings(oldShell);
    assert.equal(await oldShell.locator('.wstoast.warn').count(), 0);
    assert.equal(await oldShell.evaluate(() => typeof window.__showDaemonNotice), 'undefined');
    await capture(oldShell, 'settings-before-in-page-notice.png');
    await oldShell.close();

    const page = await freshPage();
    await page.addInitScript(stub);
    await page.goto(base + '/');
    await page.waitForFunction(() => typeof window.__showDaemonNotice === 'function');
    await settings(page);
    const url = page.url();
    const emit = message => page.evaluate(value => window.__showDaemonNotice(value), message);
    const toast = page.locator('#wstoasts .wstoast.warn');
    assert.equal(await emit(reason), true);
    await toast.waitFor({ state: 'visible' });
    assert.equal(await toast.textContent(), reason);
    assert.equal(page.url(), url);
    await capture(page, 'settings-after-in-page-notice.png');
    assert.equal(await emit('<b>Plain text notice</b>'), true);
    assert.equal(await toast.count(), 1);
    assert.equal(await toast.textContent(), '<b>Plain text notice</b>');
    assert.equal(await toast.locator('b').count(), 0);

    await page.reload();
    await page.waitForFunction(() => typeof window.__showDaemonNotice === 'function');
    assert.equal(await toast.count(), 0);
    assert.equal(await emit(reason), true);
    await toast.waitFor({ state: 'visible' });
    assert.equal(await toast.textContent(), reason);
    await page.evaluate(() => { delete window.WsToast; });
    assert.equal(await emit('A missing toast asset cannot acknowledge this'), false);
    assert.equal(await toast.textContent(), reason);
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
