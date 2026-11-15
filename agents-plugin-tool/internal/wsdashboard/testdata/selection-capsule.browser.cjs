// Run by TestSelectionCapsuleBrowser against a static render of four
// whiteboard selections: mention+comment, mention only, two mentions+comment,
// and a long topic with a long quote.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const failures = [];
    for (const theme of ['dark', 'light']) {
      for (const width of [1440, 700]) {
        const page = await browser.newPage({ viewport: { width, height: 900 } });
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.setContent(fs.readFileSync(file, 'utf8').replace('THEME', theme));
        if (shots) {
          fs.mkdirSync(shots, { recursive: true });
          await page.screenshot({ path: path.join(shots, `${theme}-${width}.png`), fullPage: true });
        }
        const m = await page.evaluate(() => {
          const bordered = (el) => {
            const cs = getComputedStyle(el);
            return parseFloat(cs.borderTopWidth) > 0 || parseFloat(cs.borderLeftWidth) > 0 || /inset/.test(cs.boxShadow);
          };
          return [...document.querySelectorAll('.msg.user .bub')].map((bub) => {
            const capsules = bub.querySelectorAll('.selection-bundle');
            const cap = capsules[0];
            const inner = cap ? [...cap.querySelectorAll('*')].filter(bordered).filter((el) => !el.matches('.selection-comment')).length : -1;
            const quotes = cap ? [...cap.querySelectorAll('.selection-quote')].map((q) => {
              const tag = q.querySelector('.selection-topic'), text = q.querySelector('.anote');
              if (!tag || !text) return null;
              const tagRects = tag.getClientRects(), textRects = text.getClientRects();
              const tr = tagRects[tagRects.length - 1], xr = textRects[0];
              return {
                tagInline: getComputedStyle(tag).display === 'inline' && getComputedStyle(text).display === 'inline',
                // the quote's first line box starts on the tag's last line
                sameLine: Math.abs(tr.top - xr.top) < 2 && xr.left >= tr.right - 1,
                tagWrapsAsText: getComputedStyle(tag).whiteSpace !== 'nowrap',
              };
            }) : [];
            const comment = cap && cap.querySelector('.selection-comment');
            const shown = comment && getComputedStyle(comment).display !== 'none' && comment.textContent !== '';
            let divider = null;
            const lastQuote = cap && [...cap.querySelectorAll('.selection-quote')].pop();
            if (shown && lastQuote) {
              const cs = getComputedStyle(comment), cr = comment.getBoundingClientRect(), capR = cap.getBoundingClientRect();
              const last = lastQuote.getBoundingClientRect();
              divider = {
                line: parseFloat(cs.borderTopWidth) >= 1 && cs.borderTopStyle !== 'none',
                below: cr.top >= last.bottom - 1,
                inside: cr.bottom <= capR.bottom + 0.5 && cr.left >= capR.left - 0.5 && cr.right <= capR.right + 0.5,
                spans: Math.abs(cr.left - capR.left) <= 4 && Math.abs(cr.right - capR.right) <= 1.5,
              };
            }
            return { capsules: capsules.length, inner, quotes, shown: !!shown, hasComment: !!(comment && comment.textContent), divider, overflow: bub.scrollWidth > bub.clientWidth + 1 };
          });
        });
        const where = `${theme}@${width}`;
        try {
          assert.deepEqual(errors, [], where + ': no page errors');
          assert.equal(m.length, 4, where + ': four selection messages');
          assert.deepEqual(m.map((x) => x.quotes.length), [1, 1, 2, 1], where + ': one quote paragraph per mention');
          m.forEach((x, i) => {
            assert.equal(x.capsules, 1, `${where} #${i}: exactly one capsule`);
            assert.equal(x.inner, 0, `${where} #${i}: no nested bordered capsule`);
            assert.equal(x.overflow, false, `${where} #${i}: nothing overflows the bubble`);
            x.quotes.forEach((q) => {
              assert.ok(q, `${where} #${i}: tag and quote present`);
              assert.ok(q.tagInline, `${where} #${i}: tag is inline text`);
              assert.ok(q.sameLine, `${where} #${i}: quote starts on the tag's line box`);
              assert.ok(q.tagWrapsAsText, `${where} #${i}: tag wraps like text`);
            });
            assert.equal(x.shown, x.hasComment, `${where} #${i}: comment section shows exactly when there is a comment`);
            if (x.hasComment) assert.deepEqual(x.divider, { line: true, below: true, inside: true, spans: true }, `${where} #${i}: comment below a divider inside the capsule`);
          });
        } catch (e) {
          failures.push(e.message);
        }
        await page.close();
      }
    }
    if (failures.length) throw new Error(failures.join('\n'));
    console.log('selection capsule ok');
  } finally {
    await browser.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
