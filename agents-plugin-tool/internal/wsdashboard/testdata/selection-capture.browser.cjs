// Run by TestSelectionCaptureBrowser: real pointer drags inside a topic body
// frame; the attached source must equal the frame's own selection text.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US' });
    const tab = await context.newPage(), sent = [];
    tab.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/message')) sent.push(r.postDataJSON()); });
    await tab.goto(base + '/s/livesession');
    const el = tab.locator('iframe.sframe.on');
    await el.waitFor({ state: 'visible' });
    const page = await (await el.elementHandle()).contentFrame();
    await page.locator('iframe.doc-frame').first().waitFor();
    const frameHandle = await page.locator('iframe.doc-frame').first().elementHandle();
    const frame = await frameHandle.contentFrame();
    await frame.locator('.doc-body').waitFor();
    const pop = page.locator('[data-wbmention-pop]');
    const cases = [
      ['mid paragraph plain', 'Second paragraph: alpha beta', 'p:nth-of-type(2)'],
      ['across inline markup', 'bold words** and', 'p:nth-of-type(1)'],
    ];
    // Drag from the middle of one word range to another using real coordinates.
    async function drag(startText, endText) {
      const pts = await frame.evaluate(([a, b]) => {
        const w = document.createTreeWalker(document.querySelector('.doc-body'), 4); let n, s, e;
        while ((n = w.nextNode())) {
          let i = n.nodeValue.indexOf(a); if (i >= 0 && !s) { const r = document.createRange(); r.setStart(n, i); r.setEnd(n, i + 1); s = r.getBoundingClientRect(); s = { x: s.left + 0.5, y: s.top + s.height / 2 }; }
          let j = n.nodeValue.indexOf(b); if (j >= 0 && !e) { const r = document.createRange(); r.setStart(n, j + b.length - 1); r.setEnd(n, j + b.length); e = r.getBoundingClientRect(); e = { x: e.right - 0.5, y: e.top + e.height / 2 }; }
        }
        return { s, e };
      }, [startText, endText]);
      const off = await frameHandle.boundingBox();
      await tab.mouse.move(off.x + pts.s.x, off.y + pts.s.y);
      await tab.mouse.down();
      await tab.mouse.move(off.x + pts.e.x, off.y + pts.e.y, { steps: 8 });
      await tab.mouse.up();
    }
    const results = [];
    for (const [a, b] of [['alpha', 'delta.'], ['bold', 'inline code'], ['First paragraph', 'here.'], ['alpha', 'list two']]) {
      await drag(a, b);
      await pop.waitFor({ state: 'visible' });
      const want = (await frame.evaluate(() => getSelection().toString())).trim();
      const note = page.locator('[data-wbmention-pop] .wsc-editor, [data-wbmention-pop] textarea:not(.wsc-backing)').first();
      await note.fill('n');
      await page.locator('[data-wbmention]').click();
      await page.waitForTimeout(150);
      results.push({ a, b, want });
    }
    await page.locator('#csend').click();
    await page.waitForTimeout(500);
    assert.equal(sent.length, 1, 'one send');
    const un = t => t.replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&amp;/g, '&');
    const inline = [...sent[0].text.matchAll(/<wb:mention[^>]*>([\s\S]*?)<\/wb:mention>/g)].map(m => un(m[1]));
    const captured = (sent[0].mentions || []).map(m => m.text).concat(inline);
    assert.deepEqual(captured, results.map(r => r.want), 'attached sources are exactly the selected text, in push order');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
