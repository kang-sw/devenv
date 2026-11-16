// args: fold.html labels.json [shotdir]. labels maps data-row -> server class
// (log, closed-ask, answer, input, latest-input, open-ask).
// Asserts the bottom-up initial cut, one top fold holding closed Q/A, the old
// input and the open question's ask line, the open card stacked after the
// history, and anchor delta 0 with the same cut on expand, in dark and light.
// With shotdir, saves {initial,expanded}-{dark,light}.png.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, meta, shots] = process.argv.slice(2);
const labels = JSON.parse(fs.readFileSync(meta, 'utf8'));
const WINDOW = 1000;

// The live history as a list of gaps and rows (key, label, height).
const parts = (p) => p.evaluate((labels) => [...document.querySelector('.history-live').children].map(n => n.matches('.history-gap')
  ? { gap: true, count: +n.getAttribute('data-count') }
  : { key: n.getAttribute('data-row'), label: labels[n.getAttribute('data-row')] || '?', h: n.getBoundingClientRect().height }), labels);
const sum = (rows) => rows.reduce((s, r) => s + r.h, 0);

// Whole rows up to >= WINDOW, the crossing row included and nothing more
// (unless the history above ran out first).
function cut(rows, exhausted, what) {
  const h = sum(rows);
  if (!exhausted) assert(h >= WINDOW, `${what}: ${Math.round(h)}px < ${WINDOW}px`);
  assert(h - rows[0].h < WINDOW, `${what}: ${Math.round(h)}px keeps a row above the crossing row (${Math.round(rows[0].h)}px)`);
  return Math.round(h);
}

async function expand(p, index, what) {
  // Put the fold a third down the viewport and remember the row under it.
  const before = await p.evaluate((index) => {
    const g = document.querySelectorAll('.history-live > .history-gap')[index];
    let s = g.parentElement; while (s && !(s.scrollHeight > s.clientHeight && /auto|scroll/.test(getComputedStyle(s).overflowY))) s = s.parentElement;
    s = s || document.scrollingElement;
    s.scrollTop += g.getBoundingClientRect().top - innerHeight / 3;
    g.setAttribute('data-test-gap', '1');
    window.__a = g.nextElementSibling;
    return { key: window.__a.getAttribute('data-row') };
  }, index);
  await p.waitForTimeout(300);
  const y0 = await p.evaluate(() => window.__a.getBoundingClientRect().top);
  const rows0 = (await parts(p)).filter(r => !r.gap).map(r => r.key);
  await p.click('[data-test-gap] [data-load="up"]');
  await p.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  const d0 = await p.evaluate((y0) => window.__a.getBoundingClientRect().top - y0, y0);
  await p.waitForTimeout(1700); // past the settle window
  const d1 = await p.evaluate((y0) => window.__a.getBoundingClientRect().top - y0, y0);
  assert(Math.abs(d0) <= 1 && Math.abs(d1) <= 1, `${what}: anchor ${before.key} moved ${d0}px / ${d1}px`);
  const after = await parts(p), seen = new Set(rows0);
  const at = after.findIndex(r => r.key === before.key);
  const added = [];
  for (let i = at - 1; i >= 0 && !after[i].gap && !seen.has(after[i].key); i--) added.unshift(after[i]);
  assert.equal(after.filter(r => !r.gap).length - rows0.length, added.length, `${what}: loaded rows sit directly above the anchor`);
  const exhausted = at - added.length - 1 < 0 || !after[at - added.length - 1].gap;
  return { anchor: before.key, d0, d1, added: added.length, px: cut(added, exhausted, what + ' cut'), labels: [...new Set(added.map(r => r.label))] };
}

(async () => {
  const b = await chromium.launch();
  const out = {};
  try {
    for (const scheme of ['dark', 'light']) {
      const ctx = await b.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: scheme });
      const p = await ctx.newPage();
      await p.goto('file://' + file); await p.waitForTimeout(600);
      const all = await parts(p);
      const last = all.map(r => !!r.gap).lastIndexOf(true);
      assert(last >= 0, scheme + ': the history folds');
      const bottom = all.slice(last + 1), above = all.slice(0, last);
      const px = cut(bottom, false, scheme + ' initial cut');
      // Closed questions, answers, the older owner input and the open
      // question's ask line all fold like logs, in one fold above the cut.
      const shown = all.filter(r => !r.gap);
      ['closed-ask', 'answer', 'input', 'open-ask'].forEach(l => assert(!shown.some(r => r.label === l), `${scheme}: a ${l} row is shown: ${JSON.stringify(shown.filter(r => r.label === l))}`));
      assert(bottom.some(r => r.label === 'latest-input'), scheme + ': the latest owner input stays in the bottom window');
      const lifted = above.filter(r => !r.gap);
      assert.equal(lifted.length, 0, scheme + ': nothing is lifted out above the cut');
      assert.equal(last, 0, scheme + ': one fold, at the top');
      // The open question's card stacks after the history.
      assert(await p.evaluate(() => {
        const card = document.querySelector('.wb-asks [id^="item-"]'), history = document.querySelector('.wb-history');
        return !!card && !!(history.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING) && !history.contains(card);
      }), scheme + ': the open card stacks after the history');
      if (shots) await p.screenshot({ path: `${shots}/initial-${scheme}.png`, fullPage: true });
      // Expand the fold twice: same cut, anchor in place, folded Q/A rows
      // come back as logs.
      const gaps = all.filter(r => r.gap).length;
      const first = await expand(p, 0, scheme + ' expand the fold');
      const second = await expand(p, 0, scheme + ' expand the fold again');
      assert(first.labels.includes('answer') || first.labels.includes('closed-ask'), `${scheme}: expanding shows folded Q/A rows: ${first.labels}`);
      if (shots) {
        await p.evaluate(() => { document.activeElement && document.activeElement.blur(); });
        await p.mouse.move(0, 0);
        await p.evaluate(() => window.scrollTo(0, 0)); await p.waitForTimeout(200);
        await p.screenshot({ path: `${shots}/expanded-${scheme}.png`, fullPage: true });
      }
      out[scheme] = { initial: { px, rows: bottom.length, lifted: lifted.map(r => r.label), gaps }, first, second };
      await ctx.close();
    }
    console.log(JSON.stringify(out));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
