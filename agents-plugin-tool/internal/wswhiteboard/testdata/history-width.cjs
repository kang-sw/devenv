'use strict';

const fs = require('node:fs');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_VIEWED_PERF_PLAYWRIGHT);

(async () => {
  const browser = await chromium.launch({
    headless: true, executablePath: process.env.WS_VIEWED_PERF_CHROMIUM,
    args: ['--disable-background-networking', '--no-proxy-server'],
  });
  try {
    const context = await browser.newContext({serviceWorkers: 'block'});
    await context.route('**/*', route => route.abort());
    const page = await context.newPage();
    // The actual Go-rendered page, including production CSS and scripts.
    await page.setContent(fs.readFileSync(process.argv[2], 'utf8'));
    // Exercise the owner's "show answered checks" state, not hidden geometry.
    await page.locator('button.ckhide').click();
    assert.equal(await page.locator('html').getAttribute('data-ck-hide'), '0');
    const reports = [], violations = [];
    for (const width of [1000, 480]) {
      await page.setViewportSize({width, height: 900});
      for (const expanded of [false, true]) {
        await page.evaluate(expanded => {
          document.querySelectorAll('details').forEach(el => { el.open = expanded; });
        }, expanded);
        const rows = await page.locator('.wb-hitem').evaluateAll(items => items.map(item => {
          const text = item.querySelector('.wb-htext');
          const main = text.querySelector('.wb-hmain');
          const details = item.querySelector('details');
          return {
            kind: item.classList.contains('check') ? 'check' : 'policy',
            question: !!item.closest('.timeline-ask'),
            expanded: details ? details.open : null,
            textWidth: main.getBoundingClientRect().width,
            rowWidth: text.getBoundingClientRect().width,
            columns: getComputedStyle(text).gridTemplateColumns,
            cells: [...text.children].map(el => el.className),
          };
        }));
        reports.push({width, expanded, rows});
        for (const kind of ['check', 'policy']) {
          const matching = rows.filter(row => row.kind === kind);
          assert(matching.length > 0, `missing ${kind} history row`);
          for (const row of matching) {
            assert.equal(row.expanded, expanded, `${kind} disclosure state`);
            if (row.textWidth < 120) violations.push(`${width}px ${kind} ${row.question ? 'question' : 'settlement'} expanded=${expanded}: text width ${row.textWidth} < 120px`);
          }
        }
        assert(rows.some(row => row.kind === 'check' && row.question), 'must measure the anchored check question, not just its timestamped settlement');
      }
    }
    console.log(JSON.stringify({reports, violations}, null, 2));
    assert.equal(violations.length, 0, violations.join('\n'));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
