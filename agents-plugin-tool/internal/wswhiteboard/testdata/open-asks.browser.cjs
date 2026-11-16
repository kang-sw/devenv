// args: page.html [shots-dir]. Open asks stacked after the history in
// creation order, no check chip in the head, and navigation into folded
// history (a card's jump, an item reference) that materializes only the
// target's region, rings it, and brings it into view. Dark, 806 and 420 px.
const assert = require('node:assert/strict');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const url = pathToFileURL(process.argv[2]).href, shots = process.argv[3] || '';
const topic = '.topic[data-topic="t"]';
async function shot(p, name) {
  if (!shots) return;
  await p.mouse.move(0, 0);
  await p.screenshot({ path: path.join(shots, name + '.png') });
}
// The live history as gaps ('|') and row keys, in order.
const live = p => p.evaluate(sel => [...document.querySelector(sel + ' .history-live').children].map(n =>
  n.matches('.history-gap') ? '|' : n.getAttribute('data-row')), topic);
const total = p => p.evaluate(sel => document.querySelectorAll(sel + ' .history-live > .timeline-row').length +
  [...document.querySelectorAll(sel + ' template[data-history-gap]')].reduce((s, t) => s + t.content.querySelectorAll('.timeline-row').length, 0), topic);
// The target after a navigation: a live row, ringed, in view, with folds
// left above and below it, and only its region materialized.
async function arrived(p, id, what, before) {
  await p.waitForTimeout(150);
  const at = await p.evaluate(id => {
    const n = document.getElementById(id);
    if (!n) return null;
    const r = n.getBoundingClientRect(), row = n.closest('.timeline-row');
    return { live: !!n.closest('.history-live'), flash: n.classList.contains('flash'),
      inView: r.height > 0 && r.top >= 0 && r.bottom <= innerHeight, row: row && row.getAttribute('data-row') };
  }, id);
  assert(at, `${what}: #${id} not in the document`);
  assert(at.live && at.flash && at.inView, `${what}: #${id} ${JSON.stringify(at)}`);
  const after = await live(p), i = after.indexOf(at.row);
  assert(i >= 0, `${what}: the target row is live`);
  assert(after.slice(0, i).includes('|') && after.slice(i + 1).includes('|'), `${what}: folds stay above and below the target: ${after.join(' ')}`);
  const added = after.filter(k => k !== '|' && !before.includes(k)).length, all = await total(p);
  assert(added > 0 && added < all / 2, `${what}: ${added} of ${all} rows materialized`);
  return { row: at.row, added, all };
}
(async () => {
  const b = await chromium.launch();
  const out = {};
  try {
    for (const width of [806, 420]) {
      const p = await b.newPage({ viewport: { width, height: 900 }, colorScheme: 'dark' });
      await p.goto(url);
      await p.waitForTimeout(500);
      // (a) the open asks stack after the history, in creation order.
      const stack = await p.evaluate(sel => {
        const t = document.querySelector(sel), h = t.querySelector('.wb-history'), asks = t.querySelector('.wb-asks');
        return { after: !!asks && !!(h.compareDocumentPosition(asks) & Node.DOCUMENT_POSITION_FOLLOWING) && !h.contains(asks),
          ids: asks ? [...asks.querySelectorAll('[id^="item-"]')].map(n => n.id) : [],
          inHistory: [...h.querySelectorAll('[id^="item-"]')].map(n => n.id),
          jumps: asks ? [...asks.querySelectorAll('.wb-askjump')].map(a => a.getAttribute('href')) : [] };
      }, topic);
      assert(stack.after, `${width}: the asks follow the history`);
      assert.deepEqual(stack.ids, ['item-2', 'item-3'], `${width}: creation order`);
      assert.equal(stack.jumps.length, 2, `${width}: every card jumps to its ask line`);
      assert(!stack.inHistory.includes('item-2') && !stack.inHistory.includes('item-3'), `${width}: an open card in the history`);
      // (b) no check-answer chip in the head.
      assert.equal(await p.locator('.ckpill, .tophead .checks, button.ckhide').count(), 0, `${width}: a check chip remains`);
      const initial = await live(p);
      assert.equal(initial[0], '|', `${width}: the history folds above the latest window`);
      await p.evaluate(sel => document.querySelector(sel + ' .wb-asks').scrollIntoView({ block: 'end' }), topic);
      await p.waitForTimeout(200);
      await shot(p, `${width}-1-bottom`);
      // (c) the policy card's jump reveals its folded ask line alone.
      await p.click(`${topic} #item-3 .wb-askjump`);
      const jump = await arrived(p, stack.jumps[1].slice(1), `${width} card jump`, initial);
      await shot(p, `${width}-2-askjump`);
      // (d) the item reference in the latest line reveals answered (1).
      const now = await live(p);
      await p.evaluate(sel => document.querySelector(sel + ' .history-live a[href="#item-1"]').scrollIntoView({ block: 'center' }), topic);
      await p.click(`${topic} .history-live a[href="#item-1"]`);
      const ref = await arrived(p, 'item-1', `${width} item reference`, now);
      await shot(p, `${width}-3-itemref`);
      out[width] = { stack, jump, ref };
      await p.close();
    }
    console.log(JSON.stringify(out));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
