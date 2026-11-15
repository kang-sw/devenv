// Run by TestRawImgTagRelayBrowser: every raw <img> in the Shots topic's
// body frame and panel frame decodes (naturalWidth > 0).
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shot] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark', locale: 'en-US' });
    const tab = await ctx.newPage();
    await tab.goto(base + '/s/livesession');
    const active = tab.locator('iframe.sframe.on');
    await active.waitFor({ state: 'visible' });
    const page = await (await active.elementHandle()).contentFrame();
    const topic = page.locator('section.topic[data-topic="Shots"]');
    await topic.waitFor({ state: 'attached' });
    await topic.evaluate(t => { t.querySelectorAll('details:not(.panel-versions)').forEach(d => { d.open = true; }); t.scrollIntoView({ block: 'start' }); });
    const frames = {
      body: topic.locator('.tdoc iframe.doc-frame:not(.doc-frame-next)'),
      panel: topic.locator('details.entry-panel[id="p1"] > .entry-body > iframe.doc-frame:not(.doc-frame-next)')
    };
    const results = {};
    for (const [name, el] of Object.entries(frames)) {
      await el.waitFor({ state: 'visible' });
      const f = await (await el.elementHandle()).contentFrame();
      await f.waitForFunction(() => document.readyState === 'complete');
      // Give the relay round trip time; assert after the screenshot.
      await f.waitForFunction(() => [...document.images].length > 0 && [...document.images].every(i => i.complete && i.naturalWidth > 0), null, { timeout: 8000 }).catch(() => {});
      results[name] = await f.evaluate(() => [...document.images].map(i => ({ alt: i.alt, src: (i.getAttribute('src') || '').slice(0, 40), relay: i.getAttribute('data-relay'), width: i.width, natural: i.naturalWidth })));
    }
    console.log(JSON.stringify(results));
    if (shot) await tab.screenshot({ path: shot });
    assert.equal(results.body.length, 2, 'body holds two images');
    assert.equal(results.panel.length, 1, 'panel holds one image');
    for (const [name, imgs] of Object.entries(results)) {
      for (const i of imgs) {
        assert(i.natural > 0, `${name} image ${i.alt} decodes`);
        assert.equal(i.width, 160, `${name} image ${i.alt} keeps its width attribute`);
      }
    }
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
