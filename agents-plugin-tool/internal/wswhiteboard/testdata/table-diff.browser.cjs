const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.argv[2], shots = process.argv[3];
(async () => {
  const browser = await chromium.launch();
  try {
    for (const theme of ['dark', 'light']) for (const [label, width] of [['narrow', 700], ['wide', 950]]) {
      const page = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: theme });
      await page.goto(base + '/add');
      const result = await page.evaluate(() => {
        const table = document.querySelector('table'), rows = [...table.tBodies[0].rows];
        const bad = table.querySelectorAll('table > ins, table > del, thead > ins, thead > del, tbody > ins, tbody > del, tfoot > ins, tfoot > del, tr > ins, tr > del').length;
        const codes = ['gloom_fail', 'celebrate_done', 'subagent_dispatch', 'arm-headrub'].map(s => {
          const c = [...table.querySelectorAll('code')].find(n => n.textContent === s);
          const r = document.createRange(); r.selectNodeContents(c);
          return {name:s, rects:[...r.getClientRects()].map(x => [x.top, x.bottom])};
        });
        return {bad, counts:rows.map(r => r.children.length), header:table.tHead.rows[0].children.length,
          aligned:[...rows[2].cells].every((c,i) => { const a=c.getBoundingClientRect(), b=rows[1].cells[i].getBoundingClientRect(); return Math.abs(a.left-b.left)<1 && Math.abs(a.right-b.right)<1 && (i===0 || parseFloat(getComputedStyle(c).borderLeftWidth)>0); }),
          marked:[!!table.tHead.rows[0].cells[3].querySelector('ins.pdg-ins'), ...rows.slice(2).map(r => !!r.querySelector('td ins.pdg-ins'))], codes,
          width: table.getBoundingClientRect().width, height: table.getBoundingClientRect().height};
      });
      console.log(theme, label, JSON.stringify(result));
      assert.equal(result.bad, 0); assert.deepEqual(result.counts, [6,6,6,6]); assert.equal(result.header, 6); assert(result.aligned); assert(result.marked.every(Boolean));
      for (const c of result.codes) assert.equal(new Set(c.rects.map(r => r[0])).size, 1, c.name + ' wrapped');
      const sizing = await page.evaluate(() => {
        const body = document.querySelector('.doc-body');
        const fixture = document.createElement('div');
        fixture.innerHTML = '<table><tr><td>one</td><td>two</td></tr></table><table><tr><td><code>' + 'a'.repeat(80) + '</code></td><td>text</td></tr></table>';
        body.appendChild(fixture);
        const [compact, long] = fixture.querySelectorAll('table');
        const result = { compact: compact.getBoundingClientRect().width, available: body.clientWidth - 28, long: long.getBoundingClientRect().width, code: long.querySelector('code').getBoundingClientRect().width };
        fixture.remove(); return result;
      });
      assert(sizing.compact >= sizing.available - 1, 'compact table spans panel');
      assert(sizing.code < 400 && sizing.long < 1000, 'long code stays bounded');
      if (shots) await page.screenshot({path:path.join(shots, `table-diff-${theme}-${label}.png`), fullPage:true});
      await page.goto(base + '/remove');
      assert.equal(await page.locator('table > ins, table > del, thead > ins, thead > del, tbody > ins, tbody > del, tfoot > ins, tfoot > del, tr > ins, tr > del').count(), 0);
      assert(await page.locator('table del.pdg-w').count() > 0, 'deletion case exercised');
      await page.close();
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
