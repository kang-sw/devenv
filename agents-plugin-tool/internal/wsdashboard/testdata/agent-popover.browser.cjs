// The session's agents island (agisland, top-right in the conversation
// cell; its full view is agpop) in Chromium, through the shell page.
// env: AGPOP_BASE (daemon), AGPOP_UPDATES (swaps the agents region with a new
// caption or state=zero/nonlive fixture), AGPOP_ROWS ({keys, peers}: every row the agents region renders),
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
// open is the full view's state; island and convo are the island's visible
// box and the conversation grid cell it overlays, after its size transition.
const state = async (frame) => {
  await frame.waitForTimeout(300);
  return frame.evaluate(() => {
    const pop = document.getElementById('agpop'), badge = document.getElementById('sbadge'), island = document.getElementById('agisland');
    const box = el => { const r = el.getBoundingClientRect(); return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width }; };
    return { open: !pop.hidden, mode: island.getAttribute('data-mode'), expanded: badge.getAttribute('aria-expanded'), pinned: document.getElementById('agpin').getAttribute('aria-pressed'),
      island: box(island), shown: island.getClientRects().length > 0, convo: box(document.getElementById('convo')), vw: innerWidth, vh: innerHeight, label: badge.textContent.trim() };
  });
};
const shot = async (tab, name) => {
  if (!shots) return;
  await tab.mouse.move(1, 1);
  await tab.screenshot({ path: `${shots}/${name}.png` });
};
const fits = (s, what) => {
  const i = s.island, c = s.convo;
  assert(s.shown && s.mode === 'full', `${what}: the island is shown in full ${JSON.stringify(s)}`);
  assert(i.left >= 0 && i.right <= s.vw && i.top >= 0 && i.bottom <= s.vh, `${what}: island leaves the viewport ${JSON.stringify(s)}`);
  assert(i.left >= c.left - 1 && i.right <= c.right + 1 && i.top >= c.top - 1, `${what}: island leaves the conversation cell ${JSON.stringify(s)}`);
  assert(c.right - i.right <= 12 && i.top - c.top <= 12, `${what}: island sits away from the cell's top-right corner ${JSON.stringify(s)}`);
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
      assert.equal(s.mode, 'compact', 'live running/failed agents start in compact HUD mode');
      const compactHud = await frame.evaluate(() => {
        const nodes = [...document.querySelectorAll('#agcompact .hud-node')];
        const svgPath = document.querySelector('#agcompact .hud-lines path')?.getAttribute('d') || '';
        return { count: nodes.length, hasPath: svgPath.includes('M') };
      });
      assert.deepEqual(compactHud, { count: 4, hasPath: true }, 'compact HUD renders visible forest, 완료 count, and SVG connectors');
      // Clicking the compact background opens the full popover.
      await frame.click('#agcompact', { position: { x: 2, y: 2 } });
      assert.equal((await state(frame)).open, true);
      assert.equal(await frame.getAttribute('#agcompact', 'aria-expanded'), 'true');
      await frame.click('#agclose');
      assert.match(s.label, /^2\(\+1\)\s*실행 중$/, 'badge shows the running count');
      await shot(tab, `closed-${scheme}-1440`);
      // Open: the island expands to full, top-right in the conversation cell.
      await frame.click('#sbadge');
      s = await state(frame);
      assert(s.open && s.expanded === 'true', 'badge opens the island');
      fits(s, scheme + ' 1440');
      // Every row the agents region renders is in the island's full view, and none outside.
      const loss = await frame.evaluate((rows) => ({
        missing: rows.keys.filter(k => !document.querySelector(`#agpop details[data-key="${k}"]`)),
        outside: [...document.querySelectorAll('details[data-key^="agent-"]')].filter(d => !d.closest('#agpop')).length,
        peers: document.querySelectorAll('#agpop .peer').length }), rows);
      assert.deepEqual(loss, { missing: [], outside: 0, peers: rows.peers }, 'no row lost');
      const statuses = await frame.evaluate(() => ['a3', 'a4', 'a5'].map(id => {
        const row = document.querySelector(`#agpop details[data-key="agent-${id}"]`).closest('.agent');
        return { legacyClass: row.classList.contains('done'), label: row.querySelector('.astatus')?.textContent || '',
          hover: row.querySelector('template.shp-source').content.querySelector('.shp-hd .chip').textContent };
      }));
      assert.deepEqual(statuses, [
        { legacyClass: true, label: '', hover: '완료' },
        { legacyClass: true, label: '휴면', hover: '휴면' },
        { legacyClass: true, label: '삭제됨', hover: '삭제됨' }
      ], 'localized Pi sleep/deleted labels leave native done and old CSS intact');
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
      // Header chips: running, done, failed, mail, background; the pin
      // control is a pushpin.
      const head = await frame.evaluate(() => ({ bg: document.querySelector('#agpop .thead .chip.bgt')?.textContent.trim(),
        chips: document.querySelectorAll('#agpop .thead .chip').length, pin: document.querySelector('#agpin use')?.getAttribute('href') }));
      assert.deepEqual(head, { bg: '백그라운드 2', chips: 5, pin: '#l-pin' }, 'header chips and pin icon');
      await shot(tab, `open-${scheme}-1440`);
      // Sticky header: only the list scrolls.
      const sticky = await frame.evaluate(() => {
        const t = document.getElementById('r-tree'); t.scrollTop = t.scrollHeight;
        const d = t.querySelector('.thead').getBoundingClientRect().top - t.getBoundingClientRect().top;
        const r = { scrolls: t.scrollHeight > t.clientHeight, d }; t.scrollTop = 0; return r;
      });
      assert(!sticky.scrolls || Math.abs(sticky.d) <= 1, 'header stays while the list scrolls: ' + JSON.stringify(sticky));
      // Row expansion: newest four batches, then the full output.
      const liveLine = () => frame.evaluate(() => [...document.querySelectorAll('#agpop .agent > details[data-key="agent-a1"] ~ .brows.live > li')].filter(li => li.offsetParent).length);
      assert.equal(await liveLine(), 1, 'a collapsed row shows its last call');
      await frame.click('#agpop details[data-key="agent-a1"] > summary');
      assert.equal(await liveLine(), 0, 'an open row does not repeat its box\'s last call below it');
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
      assert.equal((await state(frame)).open, true, 'island stays open across a swap');
      // Esc, outside click and the close button collapse it.
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
      // Pin: an outside click leaves it open, Esc still collapses it, and it
      // opens again on reload.
      await frame.click('#sbadge');
      await frame.click('#agpin');
      assert.notEqual(await frame.evaluate(() => getComputedStyle(document.querySelector('#agpin svg')).fill), 'none', 'a pinned pushpin is filled');
      await shot(tab, `pinned-${scheme}-1440`);
      await frame.click('#convo', { position: { x: 60, y: 120 } });
      s = await state(frame);
      assert(s.open && s.pinned === 'true', 'pinned island stays open on an outside click: ' + JSON.stringify(s));
      await tab.keyboard.press('Escape');
      s = await state(frame);
      assert(!s.open && s.pinned === 'true', 'Esc collapses a pinned island and keeps the pin: ' + JSON.stringify(s));
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
    // Narrow screens: the same island shrunk to the conversation cell. The
    // board tab keeps the badge, but in the one-column layout the island
    // overlays the conversation cell, which that tab hides, so it hides too.
    for (const [width, scheme] of [[700, 'dark'], [700, 'light'], [420, 'dark']]) {
      const { ctx, tab, frame } = await open(b, { width, scheme });
      await shot(tab, `closed-${scheme}-${width}`);
      await frame.click('#sbadge');
      let s = await state(frame);
      assert(s.open, `${width}: opens`);
      fits(s, `${scheme} ${width}`);
      assert(s.island.width <= s.vw - 16, `${width}: island narrower than the viewport ${JSON.stringify(s.island)}`);
      out[width + scheme] = { width: Math.round(s.island.width), vw: s.vw };
      // The title text node follows the header icon.
      assert.equal(await frame.evaluate(() => { const r = document.createRange(); r.selectNodeContents(document.querySelector('#agpop .thead').childNodes[1]); return r.getClientRects().length; }), 1, `${width}: the island title stays on one line`);
      await frame.click('#agpop details[data-key="agent-a1"] > summary');
      await shot(tab, `open-${scheme}-${width}`);
      await frame.click('#agclose');
      if (await frame.isVisible('#tabbar button[data-tab="board"]')) {
        await frame.click('#tabbar button[data-tab="board"]');
        assert(await frame.isVisible('#sbadge'), `${width}: board tab keeps the badge`);
        await frame.click('#sbadge');
        s = await state(frame);
        assert(s.open && !s.shown, `${width}: the board tab hides the island with the conversation cell ${JSON.stringify(s)}`);
        if (scheme === 'dark') await shot(tab, `board-open-${scheme}-${width}`);
      }
      await ctx.close();
    }
    // No running subagents: the badge remains a muted, usable island
    // trigger. A draft retains the same DOM when the adapter goes away,
    // exercising syncBadge with liveMode=false rather than reloading it.
    {
      const { ctx, tab, frame, errors } = await open(b, { width: 1440 });
      const draft = 'keep this draft while the adapter goes away';
      await frame.locator('#ctext + .wsc-editor').fill(draft);
      assert.equal((await fetch(process.env.AGPOP_UPDATES + '?state=zero')).status, 204);
      await frame.waitForFunction(() => document.querySelector('#sbadge .slabel').textContent === '0');
      await frame.locator('#ibadge:not([hidden])').waitFor();
      const checkIdle = async (what) => {
        const idle = await frame.evaluate(() => {
          const badge = document.getElementById('sbadge'), probe = document.createElement('span');
          probe.style.color = 'var(--wb-muted-fg)'; badge.appendChild(probe);
          const muted = getComputedStyle(probe).color; probe.remove();
          return { visible: !badge.hidden && badge.getClientRects().length > 0, idle: badge.classList.contains('idle'),
            count: badge.querySelector('.slabel').textContent, spinner: getComputedStyle(badge.querySelector('.spin')).display,
            color: getComputedStyle(badge).color, muted };
        });
        assert.deepEqual([idle.visible, idle.idle, idle.count, idle.spinner], [true, true, '0', 'none'], `${what}: zero badge stays visible and quiet`);
        assert.equal(idle.color, idle.muted, `${what}: badge uses the muted color`);
        assert.equal((await state(frame)).open, false, `${what}: starts closed`);
        assert.equal((await state(frame)).mode, 'idle', `${what}: empty HUD forest drops to idle pill`);
        assert.equal(await frame.locator('#agidle .agidle-label').count(), 0);
        await frame.click('#sbadge');
        const s = await state(frame);
        assert(s.open && s.expanded === 'true', `${what}: zero badge opens the island`);
        assert(await frame.isVisible('#agpop details[data-key="agent-ended"]'), `${what}: ended agent is accessible`);
        fits(s, what);
        await shot(tab, `${what}-1440`);
        await frame.click('#agclose');
      };
      await checkIdle('zero-running');
      assert.equal((await fetch(process.env.AGPOP_UPDATES + '?state=nonlive')).status, 204);
      await frame.locator('#dash-banner:not([hidden])').waitFor();
      // The lead was working in the zero-running fixture. Its interrupt
      // control now hides, proving the mode event made the page non-live.
      await frame.waitForFunction(() => document.getElementById('ibadge').hidden);
      assert.equal(await frame.inputValue('#ctext'), draft, 'non-live transition retains the draft');
      await checkIdle('non-live');
      assert.deepEqual(errors, [], 'idle and non-live fixtures have no page errors');
      await ctx.close();
      out.idle = 'zero-running and non-live ok';
    }
    console.log(JSON.stringify(out));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
