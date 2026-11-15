// The session's agents popover (agpop) in Chromium, through the shell page.
// env: AGPOP_BASE (daemon), AGPOP_UPDATES (swaps the agents region with a new
// caption), AGPOP_ROWS ({keys, peers}: every row the agents region renders),
// AGPOP_SHOTS (optional screenshot directory).
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.AGPOP_BASE, shots = process.env.AGPOP_SHOTS;
const rows = JSON.parse(process.env.AGPOP_ROWS);

async function open(b, { width, scheme = 'dark', height = 900, ctx }) {
  ctx = ctx || await b.newContext({ viewport: { width, height }, colorScheme: scheme, locale: 'ko-KR' });
  await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
  const tab = await ctx.newPage(), errors = [];
  tab.on('pageerror', e => errors.push(e.message));
  await tab.goto(base + '/s/livesession');
  const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
  await frame.locator('#sbadge:not([hidden])').waitFor();
  await tab.waitForTimeout(500);
  return { ctx, tab, frame, errors };
}
const state = (frame) => frame.evaluate(() => {
  const pop = document.getElementById('agpop'), badge = document.getElementById('sbadge');
  const r = pop.getBoundingClientRect(), br = badge.getBoundingClientRect();
  return { open: !pop.hidden, expanded: badge.getAttribute('aria-expanded'), pinned: document.getElementById('agpin').getAttribute('aria-pressed'),
    rect: { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width }, badgeTop: br.top, vw: innerWidth, vh: innerHeight, label: badge.textContent.trim() };
});
const shot = async (tab, name) => {
  if (!shots) return;
  await tab.mouse.move(1, 1);
  await tab.screenshot({ path: `${shots}/${name}.png` });
};
const fits = (s, what) => {
  assert(s.rect.left >= 0 && s.rect.right <= s.vw && s.rect.top >= 0, `${what}: popover leaves the viewport ${JSON.stringify(s)}`);
  assert(s.rect.bottom <= s.badgeTop, `${what}: popover covers the badge ${JSON.stringify(s)}`);
};

(async () => {
  const b = await chromium.launch();
  const out = {};
  try {
    for (const scheme of ['dark', 'light']) {
      const { ctx, tab, frame, errors } = await open(b, { width: 1440, scheme });
      // Closed: no agents panel in the grid; the badge reads the running count.
      const layout = await frame.evaluate(() => ({ inGrid: !!document.querySelector('#grid > .r-tree, #grid > #splitter'), splitter: !!document.getElementById('splitter'),
        convoTop: document.getElementById('chead').getBoundingClientRect().top - document.getElementById('grid').getBoundingClientRect().top }));
      assert.deepEqual([layout.inGrid, layout.splitter], [false, false], 'the agents panel and its splitter left the grid');
      assert(layout.convoTop < 4, 'the conversation starts at the top of its column: ' + layout.convoTop);
      let s = await state(frame);
      assert(!s.open && s.expanded === 'false', 'closed at load');
      assert.match(s.label, /^2\(\+1\)\s*실행 중$/, 'badge shows the running count');
      await shot(tab, `closed-${scheme}-1440`);
      // Open: popover above the badge, in the viewport.
      await frame.click('#sbadge');
      s = await state(frame);
      assert(s.open && s.expanded === 'true', 'badge opens the popover');
      fits(s, scheme + ' 1440');
      // Every row the agents region renders is in the popover, and none outside.
      const loss = await frame.evaluate((rows) => ({
        missing: rows.keys.filter(k => !document.querySelector(`#agpop details[data-key="${k}"]`)),
        outside: [...document.querySelectorAll('details[data-key^="agent-"]')].filter(d => !d.closest('#agpop')).length,
        peers: document.querySelectorAll('#agpop .peer').length }), rows);
      assert.deepEqual(loss, { missing: [], outside: 0, peers: rows.peers }, 'no row lost');
      // Order: lead, running, ended, background, peers.
      const order = await frame.evaluate(() => {
        const sel = ['details[data-key="agent-lead"]', '.tgroup.run', '.tgroup.ended', '.tgroup.bg', '.pgroup'];
        const els = sel.map(s => document.querySelector('#agpop ' + s));
        return { present: els.map(Boolean), ordered: els.every((e, i) => i === 0 || (els[i - 1].compareDocumentPosition(e) & Node.DOCUMENT_POSITION_FOLLOWING)),
          run: [...document.querySelectorAll('#agpop .tgroup.run > .agent > details')].map(d => d.getAttribute('data-key')),
          ended: [...document.querySelectorAll('#agpop .tgroup.ended > .agent > details')].map(d => d.getAttribute('data-key')),
          child: !!document.querySelector('#agpop .tgroup.run details[data-key="agent-a1"] ~ .kids details[data-key="agent-a7"]') };
      });
      assert.deepEqual(order, { present: [true, true, true, true, true], ordered: true, run: ['agent-a1', 'agent-a2'], ended: ['agent-a3', 'agent-a4', 'agent-a5', 'agent-a6'], child: true }, 'group order');
      await shot(tab, `open-${scheme}-1440`);
      // Sticky header: only the list scrolls.
      const sticky = await frame.evaluate(() => {
        const t = document.getElementById('r-tree'); t.scrollTop = t.scrollHeight;
        const d = t.querySelector('.thead').getBoundingClientRect().top - t.getBoundingClientRect().top;
        const r = { scrolls: t.scrollHeight > t.clientHeight, d }; t.scrollTop = 0; return r;
      });
      assert(!sticky.scrolls || Math.abs(sticky.d) <= 1, 'header stays while the list scrolls: ' + JSON.stringify(sticky));
      // Row expansion: newest four batches, then the full output.
      await frame.click('#agpop details[data-key="agent-a1"] > summary');
      const shown = () => frame.evaluate(() => [...document.querySelectorAll('#agpop details[data-key="agent-a1"] > .brows > li')].filter(li => li.offsetParent).length);
      assert.equal(await shown(), 4, 'expanded row shows its newest four batches');
      assert(await frame.isVisible('#agpop details[data-key="agent-a1"] > .afull'), 'full output action shows');
      await shot(tab, `expanded-${scheme}-1440`);
      await frame.click('#agpop details[data-key="agent-a1"] > .afull');
      assert.equal(await shown(), 7, 'full output shows every batch');
      assert(!(await frame.isVisible('#agpop details[data-key="agent-a1"] > .afull')), 'action hides once taken');
      // A region swap keeps the row open and full.
      await fetch(process.env.AGPOP_UPDATES + '?caption=' + scheme + '-swap');
      await frame.waitForFunction((c) => document.getElementById('r-tree').textContent.includes(c), scheme + '-swap');
      assert.equal(await frame.evaluate(() => document.querySelector('#agpop details[data-key="agent-a1"]').open), true, 'row stays open across a swap');
      assert.equal(await shown(), 7, 'full output survives a swap');
      assert.equal((await state(frame)).open, true, 'popover stays open across a swap');
      // Esc, outside click and the close button close it.
      await tab.keyboard.press('Escape');
      assert.equal((await state(frame)).open, false, 'Esc closes');
      await frame.click('#sbadge');
      await frame.click('#convo', { position: { x: 60, y: 120 } });
      assert.equal((await state(frame)).open, false, 'outside click closes');
      await frame.click('#sbadge');
      await frame.click('#sbadge');
      assert.equal((await state(frame)).open, false, 'badge toggles');
      await frame.click('#sbadge');
      await frame.click('#agclose');
      assert.equal((await state(frame)).open, false, 'close button closes');
      // Pin: Esc and outside clicks leave it open; it opens again on reload.
      await frame.click('#sbadge');
      await frame.click('#agpin');
      await tab.keyboard.press('Escape');
      await frame.click('#convo', { position: { x: 60, y: 120 } });
      s = await state(frame);
      assert(s.open && s.pinned === 'true', 'pinned popover stays open: ' + JSON.stringify(s));
      await tab.close();
      let again = await open(b, { width: 1440, scheme, ctx });
      s = await state(again.frame);
      assert(s.open && s.pinned === 'true' && s.expanded === 'true', 'pin survives a reload');
      await again.frame.click('#agclose');
      await again.tab.close();
      again = await open(b, { width: 1440, scheme, ctx });
      s = await state(again.frame);
      assert(!s.open && s.pinned === 'false', 'close unpins: closed after reload');
      assert.deepEqual(errors.concat(again.errors), [], 'no page errors');
      await ctx.close();
      out[scheme] = 'ok';
    }
    // Narrow screens: the same popover shrunk to the viewport; the board tab
    // keeps the badge and the popover.
    for (const [width, scheme] of [[700, 'dark'], [700, 'light'], [420, 'dark']]) {
      const { ctx, tab, frame } = await open(b, { width, scheme });
      await shot(tab, `closed-${scheme}-${width}`);
      await frame.click('#sbadge');
      let s = await state(frame);
      assert(s.open, `${width}: opens`);
      fits(s, `${scheme} ${width}`);
      assert(s.rect.width <= s.vw - 16, `${width}: popover narrower than the viewport ${JSON.stringify(s.rect)}`);
      await frame.click('#agpop details[data-key="agent-a1"] > summary');
      await shot(tab, `open-${scheme}-${width}`);
      await frame.click('#agclose');
      if (await frame.isVisible('#tabbar button[data-tab="board"]')) {
        await frame.click('#tabbar button[data-tab="board"]');
        assert(await frame.isVisible('#sbadge'), `${width}: board tab keeps the badge`);
        await frame.click('#sbadge');
        s = await state(frame);
        assert(s.open, `${width}: board tab opens the popover`);
        fits(s, `${scheme} ${width} board`);
        if (scheme === 'dark') await shot(tab, `board-open-${scheme}-${width}`);
      }
      out[width + scheme] = { width: Math.round(s.rect.width), vw: s.vw };
      await ctx.close();
    }
    console.log(JSON.stringify(out));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
