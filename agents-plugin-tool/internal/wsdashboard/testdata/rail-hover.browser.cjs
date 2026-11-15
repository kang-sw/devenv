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
    await page.goto(base + '/');
    const row = page.locator('#r-rail .rs[data-rid="livesession"]');
    await row.locator('.rrun').waitFor();
    await page.waitForTimeout(300);
    await row.locator('.rl').focus();
    await page.locator('.shp').waitFor({ state: 'visible' });
    await page.evaluate(() => {
      const rail = document.querySelector('#r-rail');
      window.railBefore = {
        row: rail.querySelector('.rs[data-rid="livesession"]'),
        spinner: rail.querySelector('.rs[data-rid="livesession"] .rrun'),
        list: rail.querySelector('#rlist'), focus: document.activeElement,
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
        r.querySelector('.rs[data-rid="livesession"] .rrun') === b.spinner,
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
    assert.equal(await row.locator('.rrun').count(), 0, 'real row update removes working state');
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="livesession"]') === window.railBefore.row), false);
    await update();
    await row.locator('.rrun').waitFor();
    await page.evaluate(() => { window.nextSpinner = document.querySelector('#r-rail .rs[data-rid="livesession"] .rrun'); });
    await update();
    assert.equal(await page.evaluate(() => document.querySelector('#r-rail .rs[data-rid="livesession"] .rrun') === window.nextSpinner), true);

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
    assert.deepEqual(errors, []);
    console.log('PASS: hover-only stream keeps row/spinner identity and fresh open/reopened cards; real changes still swap');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
