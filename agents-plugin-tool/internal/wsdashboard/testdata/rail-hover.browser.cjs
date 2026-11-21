// Run by TestRailHoverUpdatesBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale: 'en-US' });
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.addInitScript(() => {
      window.railWire = [];
      const Native = window.WebSocket;
      window.WebSocket = class extends Native {
        constructor(...args) {
          super(...args);
          this.addEventListener('message', e => {
            const m = JSON.parse(e.data);
            if (m.event === 'rail-rows' || (m.event === 'region' && m.data.name === 'rail')) window.railWire.push(m);
          });
        }
      };
    });
    await page.goto(base + '/');
    const row = page.locator('#r-rail .rs[data-rid="livesession"]');
    await row.locator('.hbr').waitFor();
    await page.waitForTimeout(300);
    await row.locator('.rl').focus();
    await page.locator('.shp').waitFor({ state: 'visible' });
    await page.evaluate(() => {
      const rail = document.querySelector('#r-rail');
      window.railBefore = {
        row: rail.querySelector('.rs[data-rid="livesession"]'),
        spinner: rail.querySelector('.rs[data-rid="livesession"] .hbr'),
        list: rail.querySelector('#rlist'), focus: document.activeElement,
        other: rail.querySelector('.rs[data-rid="boardsession"]'),
        hash: rail.getAttribute('data-h'), classMutations: 0
      };
      window.railObserver = new MutationObserver(ms => {
        window.railBefore.classMutations += ms.filter(m => m.type === 'attributes').length;
      });
      window.railObserver.observe(rail, { subtree: true, attributes: true, attributeFilter: ['class'] });
    });
    async function update(query = '') {
      const { at } = await (await fetch(updates + query)).json();
      await page.waitForFunction(at => {
        const src = document.querySelector('#r-rail .rs[data-rid="livesession"] template');
        return src && src.innerHTML.includes('data-ms="' + at + '"');
      }, at);
      return at;
    }
    const at = await update();
    await page.waitForFunction(at => document.querySelector('.shp').innerHTML.includes('data-ms="' + at + '"'), at);
    assert.deepEqual(await page.evaluate(() => {
      const r = document.querySelector('#r-rail'), b = window.railBefore;
      return [r.querySelector('.rs[data-rid="livesession"]') === b.row,
        r.querySelector('.rs[data-rid="livesession"] .hbr') === b.spinner,
        r.querySelector('#rlist') === b.list, document.activeElement === b.focus,
        r.getAttribute('data-h') !== b.hash, b.classMutations];
    }), [true, true, true, true, true, 0], 'hover-only push keeps rows, spinner, scroll container and focus without re-marking');
    await page.evaluate(() => window.railObserver.disconnect());
    // Opening after another push also reads current evidence, not a cached card.
    await page.locator('body').click({ position: { x: 1000, y: 800 } });
    const reopenedAt = await update();
    await row.locator('.rl').focus();
    await page.waitForFunction(at => !document.querySelector('.shp').hidden && document.querySelector('.shp').innerHTML.includes('data-ms="' + at + '"'), reopenedAt);
    // A genuine status change still lands, and becomes the next comparison baseline.
    await update('?idle=1');
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="boardsession"]') === window.railBefore.other), true, 'unchanged row survives a visible status delta');
    assert.equal(await row.locator('.hbr').count(), 0, 'real row update removes working state');
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="livesession"]') === window.railBefore.row), false);
    await update();
    await row.locator('.hbr').waitFor();
    await page.evaluate(() => { window.nextSpinner = document.querySelector('#r-rail .rs[data-rid="livesession"] .hbr'); });
    await update();
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="livesession"] .hbr') === window.nextSpinner), true);

    assert.ok(await page.evaluate(() => window.railWire.some(m => m.event === 'rail-rows' && m.data.rows.length === 1 && m.data.rows[0].key === 'livesession')), 'production connection opted into changed-row events');
    // Hidden tabs accumulate canonical inputs, not patches against stale DOM.
    await page.evaluate(() => {
      window.testHidden = true;
      Object.defineProperty(document, 'hidden', { configurable: true, get: () => window.testHidden });
    });
    let finalAt;
    for (const query of ['?idle=1', '', '?idle=1']) {
      const { at } = await (await fetch(updates + query)).json(); finalAt = at;
      await page.waitForFunction(at => window.railWire.some(m => m.event === 'rail-rows' && m.data.rows.some(r => r.html.includes('data-ms="' + at + '"'))), at);
    }
    assert.equal(await row.locator('.hbr').count(), 1, 'hidden deltas did not touch live DOM');
    await page.evaluate(() => { window.testHidden = false; document.dispatchEvent(new Event('visibilitychange')); });
    await page.waitForFunction(at => document.querySelector('#r-rail .rs[data-rid="livesession"] template').innerHTML.includes('data-ms="' + at + '"'), finalAt);
    assert.equal(await row.locator('.hbr').count(), 0, 'newest hidden status applied');
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="boardsession"]') === window.railBefore.other), true, 'hidden accumulation keeps unchanged rows');

    // Exercise the helper with escaped content, reordered sources, and changed
    // hover attributes, independently of the fixture's particular row shape.
    await page.evaluate(() => {
      const root = document.createElement('div');
      const html = (a, b, stale = '', label = 'A') => '<div class="rs" data-rid="a"><b>' + label + '</b><template class="shp-source"' + stale + '>' + a + '</template></div>' +
        '<div class="rs" data-rid="b"><template class="shp-source">' + b + '</template></div>';
      root.innerHTML = html('old A', 'old B');
      const patch = window.WsRail.hoverUpdates(root), a = root.children[0], b = root.children[1];
      a.classList.add('cur'); // client marks never become the comparison baseline
      root.insertBefore(b, a); // matching sources are keyed, not zipped in DOM order
      if (!patch(html('&lt;script&gt;A&lt;/script&gt;', 'new B', ' data-stale="1"'))) throw Error('hover update not handled');
      if (root.children[0] !== b || root.children[1] !== a || !a.classList.contains('cur')) throw Error('client state lost');
      if (a.querySelector('template').content.textContent !== '<script>A</script>' || !a.querySelector('template').hasAttribute('data-stale')) throw Error('hover evidence/attributes stale');
      if (b.querySelector('template').innerHTML !== 'new B') throw Error('source assigned to wrong row');
      if (patch(html('A', 'B', '', 'changed'))) throw Error('real markup change swallowed');
      root.innerHTML = html('A', 'B', '', 'changed');
      if (!patch(html('new A', 'new B', '', 'changed'))) throw Error('full swap did not reset baseline');
      if (patch(html('newer A', 'newer B', '', 'changed').replace(/<template class="shp-source">newer A<\/template>/, ''))) throw Error('removed server source did not fall back');
      root.querySelector('template').remove();
      if (patch(html('newer A', 'newer B', '', 'changed'))) throw Error('missing source did not fall back');
    });
    await page.evaluate(() => {
      const R = window.WsRail, doc = document;
      const row = (id, label, hover) => '<div class="rs" data-rid="' + id + '"><b>' + label + '</b><template class="shp-source">' + hover + '</template></div>';
      const raw = '<section>' + row('a', 'A', 'old') + row('b', 'B', 'old') + '</section>';
      const input = R.rowInputFull(raw, doc), root = doc.createElement('div'); root.innerHTML = raw;
      const a = root.querySelector('[data-rid="a"]'), b = root.querySelector('[data-rid="b"]');
      a.classList.add('cur');
      const next = R.rowInputPatch(input, { hash: 'next', order: ['a', 'b'], rows: [{ key: 'a', hash: 'a2', html: row('a', 'A', '&lt;script&gt;safe&lt;/script&gt;') }] }, doc);
      if (!next || R.mergeRows(root, input, next) !== false || root.querySelector('[data-rid="a"]') !== a || !a.classList.contains('cur') || root.querySelector('[data-rid="b"]') !== b) throw Error('hover delta lost client state');
      if (a.querySelector('template').content.textContent !== '<script>safe</script>' || root.querySelector('script')) throw Error('hover escaping lost');
      const reordered = R.rowInputPatch(next, { hash: 'order', order: ['b', 'a'], rows: [] }, doc);
      if (R.mergeRows(root, next, reordered) !== true || root.querySelector('section').firstChild !== b) throw Error('order-only delta lost keyed identity');
      if (input.rows.a.html.includes('safe') || next.order[0] !== 'a') throw Error('canonical input mutated');
      const bad = [null, { hash: 'x', order: ['a', 'a'], rows: [] }, { hash: 'x', order: ['a', 'missing'], rows: [] },
        { hash: 'x', order: ['a', 'b'], rows: [{ key: 'a', hash: 'h', html: row('b', 'wrong key', '') }] },
        { hash: 'x', order: ['a', 'b'], rows: [{ key: 'a', hash: 'h', html: row('a', 'A', '') + '<p>extra</p>' }] },
        { hash: 'x', order: ['a', 'b'], rows: [{ key: 'a', hash: 'h', html: row('a', 'A', '') }, { key: 'a', hash: 'h', html: row('a', 'A', '') }] }];
      if (bad.some(ev => R.rowInputPatch(input, ev, doc) !== null) || R.rowInputPatch(null, { hash: 'h', order: [], rows: [] }, doc) !== null) throw Error('uncertain baseline admitted');
      const changedShell = R.rowInputFull('<aside>' + row('a', 'A', '') + row('b', 'B', '') + '</aside>', doc);
      if (R.mergeRows(root, reordered, changedShell) !== null) throw Error('changed shell did not fall back');
      b.remove();
      if (R.mergeRows(root, reordered, next) !== null) throw Error('missing live row did not fall back');
      if (R.rowInputHTML(input) !== raw) throw Error('canonical full HTML not preserved');
    });
    assert.deepEqual(errors, []);
    console.log('PASS: keyed deltas preserve unchanged rows, order and hidden accumulation; hover-only updates keep row/spinner identity and fresh cards; real changes still swap');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
