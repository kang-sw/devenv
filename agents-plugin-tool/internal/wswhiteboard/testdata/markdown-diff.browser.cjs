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
      await page.evaluate(value => document.documentElement.setAttribute('data-theme', value), theme);
      const code = page.locator('.entry-panel .doc-body code.language-diff');
      await code.waitFor();
      assert.deepEqual(await code.locator(':scope > span').evaluateAll(nodes => nodes.map(n => n.className)),
        ['wb-diff-file', 'wb-diff-file', 'wb-diff-file', 'wb-diff-file', 'wb-diff-hunk',
          'wb-diff-context', 'wb-diff-remove', 'wb-diff-add', 'wb-diff-add']);
      const add = await code.locator('.wb-diff-add').first().evaluate(n => getComputedStyle(n).color);
      const remove = await code.locator('.wb-diff-remove').first().evaluate(n => getComputedStyle(n).color);
      assert.notEqual(add, remove);
      const source = await code.textContent();
      const wrap = page.locator('.entry-panel .cb-wrapbtn');
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
