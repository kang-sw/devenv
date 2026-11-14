// Opt-in, standalone acceptance; no installed daemon or application server.
// WS_PLAYWRIGHT_MODULE=/absolute/path/to/playwright node testdata/latest-log.browser.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE || 'playwright');
const assets = path.join(__dirname, '../assets');
const css = fs.readFileSync(path.join(assets, 'page.css'), 'utf8');
const stream = fs.readFileSync(path.join(assets, 'stream.js'), 'utf8');
const arrival = stream.slice(stream.indexOf('/* Latest agent arrival:'));
function recap(id) {
  return `<div class="trecap" data-kind="agent" data-entry="${id}"><time datetime="${new Date().toISOString()}">now</time><div class="rtext">Log ${id}</div></div>`;
}
function board() {
  return `<style>${css}</style><section class="topic" data-topic="top">${recap('e1')}</section><div style="height:2200px"></div><section class="topic" data-topic="bottom">${recap('e2')}</section>`;
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 900, height: 600 } });
    async function load() {
      await page.goto('about:blank');
      await page.setContent(board());
      await page.addScriptTag({ content: arrival });
    }
    async function replace(topic, html) {
      await page.locator(`[data-topic="${topic}"]`).evaluate((node, text) => { node.innerHTML = text; }, html);
    }
    async function animation(id) {
      return page.locator(`[data-entry="${id}"]`).first().evaluate(node => getComputedStyle(node).animationName);
    }
    async function waitBlink(id) {
      await page.waitForFunction(id => [...document.querySelectorAll(`[data-entry="${id}"]`)].some(node => getComputedStyle(node).animationName === 'wb-log-arrival'), id);
    }
    await load();
    assert.equal(await animation('e1'), 'none', 'initial load never blinks');
    await replace('bottom', recap('e3'));
    await page.waitForTimeout(150);
    assert.equal(await animation('e3'), 'none', 'off-screen arrival waits');
    await page.locator('[data-entry="e3"]').scrollIntoViewIfNeeded();
    await waitBlink('e3');
    const timing = await page.locator('[data-entry="e3"]').evaluate(node => {
      const s = getComputedStyle(node); return [s.animationDuration, s.animationIterationCount];
    });
    assert.deepEqual(timing, ['1s', '3']);
    await page.waitForTimeout(3200);
    assert.equal(await animation('e3'), 'none', 'animation ends at about three seconds');
    const fill = await page.locator('[data-entry="e3"]').evaluate(node => getComputedStyle(node).backgroundColor);
    await replace('bottom', `<article class="wb-entry agent" data-entry="e3">Previous latest</article>${recap('e4')}`);
    await waitBlink('e4');
    const oldFill = await page.locator('[data-entry="e3"]').evaluate(node => getComputedStyle(node).backgroundColor);
    assert.notEqual(oldFill, fill, 'previous latest loses its fill');
    await page.waitForTimeout(3200);
    assert.equal(await page.locator('[data-entry="e4"]').evaluate(node => getComputedStyle(node).backgroundColor), fill, 'new latest keeps green');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await replace('bottom', recap('e5'));
    await page.waitForTimeout(150);
    assert.equal(await animation('e5'), 'none', 'reduced motion has no blink');
    assert.equal(await page.locator('[data-entry="e5"]').evaluate(node => getComputedStyle(node).backgroundColor), fill);
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    await replace('bottom', `<details><summary>Closed history</summary>${recap('e6')}</details>`);
    await page.waitForTimeout(150);
    assert.equal(await animation('e6'), 'none', 'closed history does not consume arrival');
    await page.locator('[data-topic="bottom"] summary').click();
    await waitBlink('e6');
    await page.evaluate(() => { document.documentElement.dataset.theme = 'dark'; });
    await page.waitForTimeout(3200);
    const darkFill = await page.locator('[data-entry="e6"]').evaluate(node => getComputedStyle(node).backgroundColor);
    assert.notEqual(darkFill, fill, 'green fill follows the dark surface');
    await load(); // Fresh page-script lifetime, as with reload; no persistent blink state.
    assert.equal(await animation('e1'), 'none', 'reload has no blink');
    console.log('latest log browser acceptance passed');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
