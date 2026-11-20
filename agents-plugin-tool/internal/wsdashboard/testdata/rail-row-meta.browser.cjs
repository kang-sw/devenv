// Run by TestRailRowMetaBrowser against disposable fixture sessions.
// argv: the fixture daemon's base URL and an optional absolute directory
// for the dark and light screenshots.
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

const row = id => `#r-rail .rs[data-rid="${id}"]`;

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    if (shots) fs.mkdirSync(shots, { recursive: true });
    const errors = [];
    // Dark animates; light asks for reduced motion, which stops the pulse.
    for (const [scheme, motion] of [['dark', 'no-preference'], ['light', 'reduce']]) {
      // From 1600px the rail is docked: no overlay, no scrim.
      const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 }, deviceScaleFactor: 2, locale: 'en-US', colorScheme: scheme, reducedMotion: motion });
      const page = await ctx.newPage();
      page.on('pageerror', e => errors.push(scheme + ': ' + e.message));
      await page.goto(base + '/');
      await page.locator(row('dotrow')).waitFor();
      const left = (sel) => page.locator(sel).first().evaluate(e => e.getBoundingClientRect().left);
      const height = (id) => page.locator(row(id)).evaluate(e => e.getBoundingClientRect().height);

      // Line 1: no robot badge and no policy capsule in any row.
      assert.equal(await page.locator('#r-rail .rs .bots, #r-rail .rs .chip.policy').count(), 0, scheme + ': no robot badge or capsule in a row');
      // The subagent count rides on the icon, hidden at zero.
      assert.equal(await page.locator(row('metaparent') + ' .sbn').textContent(), '1', scheme + ': parent count');
      assert.equal(await page.locator(row('dotrow') + ' .sbn').textContent(), '12', scheme + ': two-digit count');
      assert.equal(await page.locator(row('plainrow') + ' .sbn').count(), 0, scheme + ': no count at zero');
      // The question dot carries its count to a reader.
      assert.equal(await page.locator(row('metaparent') + ' .qdot').getAttribute('aria-label'), '2 items to answer', scheme + ': dot label');
      assert.equal(await page.locator(row('kidone') + ' .qdot').getAttribute('aria-label'), '1 item to answer', scheme + ': child dot label');
      assert.equal(await page.locator(row('plainrow') + ' .qdot').count(), 0, scheme + ': no dot without questions');
      // No count reserves no space: the mockup's name-to-icon gap is 6px.
      // A count flows 2.5px after the icon and pushes the name by its width.
      const gap = id => page.locator(row(id)).evaluate(r => r.querySelector('.rid').getBoundingClientRect().left - r.querySelector('.hb').getBoundingClientRect().right);
      assert.equal(await gap('plainrow'), 6, scheme + ': no-count gap matches the mockup');
      assert.equal(await gap('idleparent'), 6, scheme + ': no reserved count slot');
      for (const id of ['agentsrow', 'dotrow', 'metaparent', 'kidone']) {
        const width = await page.locator(row(id) + ' .sbn').evaluate(e => e.getBoundingClientRect().width);
        assert.ok(Math.abs(await gap(id) - (8.5 + width)) < 0.1, scheme + ': ' + id + ' count pushes name');
      }
      const iconX = await left(row('plainrow') + ' .hb');
      assert.ok(Math.abs(await left(row('dotrow') + ' .hb') - iconX) < 0.5, scheme + ': the dot keeps the icon x');
      const kidX = await left(row('kidfour') + ' .rid');
      assert.ok(await left(row('kidone') + ' .rid') > kidX, scheme + ': child count pushes name');
      assert.equal(await gap('kidfour'), 6, scheme + ': child no-count gap');
      // Child rows stay one line, badges or not.
      const kidH = await height('kidfour');
      for (const id of ['kidone', 'kidtwo', 'kidthree', 'kidfive']) {
        assert.ok(Math.abs(await height(id) - kidH) < 0.5, scheme + ': ' + id + ' one line');
      }
      // The parent's second line: toggle (chevron, person, count), working
      // subscript, then the message.
      const meta = row('metaparent') + ' .drmeta';
      assert.equal(await page.locator(meta + ' .kidtog .kn').textContent(), '5', scheme + ': child count');
      assert.equal(await page.locator(meta + ' .drkwork').textContent(), '3', scheme + ': working count');
      assert.equal(await page.locator(meta + ' .drkwork').evaluate(b => b.previousElementSibling.className), 'kidtog', scheme + ': subscript after the toggle');
      assert.ok((await page.locator(meta + ' .drmsg .s2').textContent()).startsWith('Whiteboard:'), scheme + ': message after the meta');
      assert.equal(await page.locator(row('idleparent') + ' .drkwork').count(), 0, scheme + ': no working count at zero');
      assert.equal(await page.locator(row('agentsrow') + ' .drmeta').count(), 0, scheme + ': no meta line without children');
      // Pulses honour reduced motion.
      for (const sel of [row('metaparent') + ' .sbn', meta + ' .drkwork']) {
        const anim = await page.locator(sel).evaluate(b => getComputedStyle(b).animationName);
        assert.equal(anim, motion === 'reduce' ? 'none' : 'rail-blink', scheme + ': ' + sel + ' pulse');
      }
      await page.addStyleTag({ content: '*{animation-play-state:paused !important; animation-delay:0s !important; animation:none !important}' });
      // idleparent folded keeps the shot to the six states.
      await page.locator(row('idleparent') + ' .kidtog').click();
      await page.waitForTimeout(150);
      const shoot = async (name) => {
        if (!shots) return;
        // Off the rows: no hover card, no hover buttons in the shot.
        await page.mouse.move(1660, 990);
        await page.evaluate(() => document.activeElement && document.activeElement.blur());
        await page.waitForFunction(() => { const p = document.querySelector('.rpop.shp'); return !p || p.hidden; });
        await page.waitForTimeout(200);
        await page.screenshot({ path: path.join(shots, `${name}-${scheme}.png`), scale: 'css' });
        await page.locator('#r-rail .rsessions').first().screenshot({ path: path.join(shots, `${name}-${scheme}-crop.png`), scale: 'device' });
      };
      await shoot('real-expanded');
      // The toggle on the second line folds and unfolds the parent.
      await page.locator(meta + ' .kidtog').click();
      await page.waitForTimeout(150);
      assert.equal(await page.locator(row('metaparent')).evaluate(e => e.classList.contains('kcol')), true, scheme + ': folded');
      assert.equal(await page.locator(meta + ' .kidtog').getAttribute('aria-expanded'), 'false', scheme + ': aria folded');
      assert.equal(await page.locator(row('kidone')).isVisible(), false, scheme + ': children hidden');
      assert.equal(await page.locator(row('metaparent') + ' .kidorb').isVisible(), true, scheme + ': working roll-up on the icon');
      await shoot('real-collapsed');
      await page.locator(meta + ' .kidtog').click();
      await page.waitForTimeout(150);
      assert.equal(await page.locator(row('kidone')).isVisible(), true, scheme + ': unfolded');
      await ctx.close();
    }
    assert.deepEqual(errors, []);
    console.log('PASS: rail rows carry the subagent count and question dot on the icon, the toggle on the second line, one-line children');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
