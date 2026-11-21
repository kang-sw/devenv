const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['light', 'dark']) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      await page.goto(process.argv[2]);
      await page.evaluate(() => {
        addEventListener('message', event => {
          if (!event.data || event.data.wbDoc !== 'height') return;
          const frame = [...document.querySelectorAll('iframe.doc-frame')].find(f => f.contentWindow === event.source);
          if (frame) frame.style.height = event.data.height + 'px';
        });
        document.querySelectorAll('div.doc-frame[data-srcdoc]').forEach(p => {
          const f = document.createElement('iframe');
          for (const a of p.attributes) if (a.name !== 'data-sandbox') f.setAttribute(a.name, a.value);
          f.setAttribute('sandbox', p.dataset.sandbox);
          f.srcdoc = p.dataset.srcdoc;
          f.style.height = '1px';
          p.replaceWith(f);
        });
      });
      await page.evaluate(value => document.documentElement.setAttribute('data-theme', value), theme);
      const frame = page.frameLocator('.entry-panel iframe.doc-frame');
      const code = frame.locator('.doc-body code.language-diff');
      await code.waitFor();
      for (const child of page.frames()) {
        if (child !== page.mainFrame()) await child.evaluate(value => document.documentElement.setAttribute('data-theme', value), theme);
      }
      assert.deepEqual(await code.locator(':scope > span').evaluateAll(nodes => nodes.map(n => n.className)),
        ['wb-diff-file', 'wb-diff-file', 'wb-diff-file', 'wb-diff-file', 'wb-diff-hunk',
          'wb-diff-context', 'wb-diff-remove', 'wb-diff-add', 'wb-diff-add']);
      const add = await code.locator('.wb-diff-add').first().evaluate(n => getComputedStyle(n).color);
      const remove = await code.locator('.wb-diff-remove').first().evaluate(n => getComputedStyle(n).color);
      assert.notEqual(add, remove);
      const source = await code.textContent();
      const wrap = frame.locator('.cb-wrapbtn');
      await wrap.click();
      assert.equal(await code.locator('.cl').count(), 9);
      assert.equal(await code.locator('.wb-diff-add.cl').count(), 2);
      await wrap.click();
      assert.equal(await code.textContent(), source);
      assert.equal(await code.locator('.wb-diff-remove').count(), 1);
      if (process.argv[3]) {
        fs.mkdirSync(process.argv[3], { recursive: true });
        await page.locator('.entry-panel').first().screenshot({ path: path.join(process.argv[3], `whiteboard-diff-${theme}.png`) });
      }
      await page.close();
    }
    console.log('ok');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
