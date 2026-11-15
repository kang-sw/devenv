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
    // The user's own composer text must stay exactly as typed: an attached
    // selection is a separate bundle (source + its own memo), never composer text.
    const composerEditor = page.locator('#composer .wsc-editor, .composer .wsc-editor').first();
    await composerEditor.fill('prior text');
    const composerText = () => page.evaluate(() => document.querySelector('#ctext').value);
    assert.equal(await composerText(), 'prior text');
    const results = [];
    let n = 0;
    for (const [a, b] of [['alpha', 'delta.'], ['bold', 'inline code'], ['First paragraph', 'here.'], ['alpha', 'list two']]) {
      await drag(a, b);
      await pop.waitFor({ state: 'visible' });
      // The frame reports each selection change asynchronously; let the final
      // report land before the memo input takes focus and locks the capture.
      await page.waitForTimeout(400);
      const want = (await frame.evaluate(() => getSelection().toString())).trim();
      const note = page.locator('[data-wbmention-pop] .wsc-editor, [data-wbmention-pop] textarea:not(.wsc-backing)').first();
      const memo = 'memo ' + (++n);
      await note.fill(memo);
      await page.locator('[data-wbmention]').click();
      await page.waitForTimeout(150);
      assert.equal(await composerText(), 'prior text', 'attaching a selection leaves the composer text untouched');
      assert.equal(await page.locator('#cchips .selection-bundle').count(), n, 'each push is its own bundle element');
      assert.equal(await page.locator('#cchips .selection-bundle').nth(n - 1).locator('.selection-note').getAttribute('title'), memo, 'the bundle holds its own memo');
      results.push({ a, b, want, memo });
    }
    await page.locator('#csend').click();
    await page.waitForTimeout(500);
    assert.equal(sent.length, 1, 'one send');
    const un = t => t.replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&amp;/g, '&');
    const groups = [...sent[0].text.matchAll(/<wb:selection>\n([\s\S]*?)<\/wb:selection>/g)].map(m => m[1]);
    assert.equal(groups.length, results.length, 'one selection element per push');
    groups.forEach((g, i) => {
      const mentions = [...g.matchAll(/<wb:mention[^>]*>([\s\S]*?)<\/wb:mention>/g)].map(m => un(m[1]));
      const comments = [...g.matchAll(/<wb:comment>([\s\S]*?)<\/wb:comment>/g)].map(m => un(m[1]));
      assert.deepEqual(mentions, [results[i].want], 'block ' + i + ' is exactly the selected text');
      assert.deepEqual(comments, [results[i].memo], 'comment ' + i + ' is that push\'s own memo');
    });
    assert.equal(sent[0].text.replace(/<wb:selection>[\s\S]*?<\/wb:selection>\s*/g, '').trim(), 'prior text', 'only the typed text remains outside the selection elements');
    assert.deepEqual(sent[0].mentions || [], [], 'no loose legacy mentions beside the groups');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
