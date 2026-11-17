'use strict';
// Static Chromium regression: no daemon, server fixture or Go executable.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {chromium} = require(process.env.WS_PLAYWRIGHT_MODULE);
const assets = path.join(__dirname, '../assets');
(async () => {
  const browser = await chromium.launch({headless: true,
    ...(process.env.WS_CHROMIUM_EXECUTABLE ? {executablePath: process.env.WS_CHROMIUM_EXECUTABLE} : {})});
  try {
    const page = await browser.newPage({viewport: {width: 1200, height: 800}, reducedMotion: 'no-preference'});
    const errors = [];
    page.on('pageerror', e => errors.push(String(e)));
    await page.setContent(`<style>${fs.readFileSync(path.join(assets, 'page-sticky.css'), 'utf8')}</style>
      <style>#bcol{height:700px;overflow:auto}.topic{height:500px}.card{height:80px}</style>
      <div id="bcol"><section class="topic"><div class="card policy" id="item-a">Question</div></section>
      <section class="topic"><div class="card policy" id="item-b">Question</div></section>
      <section class="topic"><div class="card policy" id="item-c">Question</div></section></div>`);
    await page.evaluate(() => { window.wsT = key => key; });
    await page.addScriptTag({content: fs.readFileSync(path.join(assets, 'pill.js'), 'utf8')});
    await page.evaluate(() => { document.querySelector('#bcol').scrollTop = 300; });
    async function settle() { await page.waitForTimeout(500); }
    await settle();
    async function change(pending) {
      return page.evaluate(async pending => {
        const p = document.querySelector('.wb-oq-top > .oq-pill');
        const before = p.getBoundingClientRect().width;
        window.wbPill.pending = pending; window.wbPill.update();
        const frames = [];
        for (let i = 0; i < 3; i++) frames.push(await new Promise(resolve => requestAnimationFrame(() => resolve(p.getBoundingClientRect().width))));
        return {before, frames};
      }, pending);
    }
    for (const pending of [12345, 0]) {
      const sample = await change(pending);
      assert.ok(Math.abs(sample.frames[0] - sample.before) <= 1, JSON.stringify(sample));
      assert.ok(Math.abs(sample.frames[1] - sample.before) <= 1, JSON.stringify(sample));
      assert.ok(pending ? sample.frames[2] > sample.before : sample.frames[2] < sample.before, JSON.stringify(sample));
      await settle();
      assert.equal(await page.locator('.oq-pill').count(), 2, 'measurement copies leave after the grow starts');
      assert.equal(await page.locator('.wb-oq-top > .oq-pill').evaluate(p => p.style.width), '', 'final width stays intrinsic');
    }
    // Superseding a pending measurement and teardown must not strand copies
    // or the real capsule's held width.
    await page.evaluate(async () => {
      window.wbPill.pending = 12345; window.wbPill.update();
      await new Promise(requestAnimationFrame);
      window.wbPill.pending = 12; window.wbPill.update();
    });
    await settle();
    assert.equal(await page.locator('.oq-pill').count(), 2);
    await page.evaluate(async () => {
      window.wbPill.pending = 0; window.wbPill.update();
      await new Promise(requestAnimationFrame);
      window.wbPill.off();
    });
    assert.equal(await page.locator('.oq-pill').count(), 2);
    assert.equal(await page.locator('.wb-oq-top > .oq-pill').evaluate(p => p.style.width), '');
    assert.deepEqual(errors, []);
    console.log('pill grow: first painted frames, grow/shrink, supersession and teardown passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
