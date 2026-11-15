// Run by TestRailKidBusyBrowser against disposable fixture sessions.
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    if (shots) fs.mkdirSync(shots, { recursive: true });
    const errors = [];
    // Dark animates; light asks for reduced motion, which stops the pulse.
    const pages = [];
    for (const [scheme, motion] of [['dark', 'no-preference'], ['light', 'reduce']]) {
      const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'en-US', colorScheme: scheme, reducedMotion: motion });
      const page = await ctx.newPage();
      page.on('pageerror', e => errors.push(scheme + ': ' + e.message));
      await page.goto(base + '/');
      if (await page.locator('#railbtn').isVisible() && !(await page.locator('html').evaluate(e => e.classList.contains('rail-open')))) await page.locator('#railbtn').click();
      pages.push({ scheme, page });
    }
    const row = page => page.locator('#r-rail .rs[data-rid="livesession"]');
    const busy = page => row(page).locator('.drkwork');
    async function expect(text) {
      for (const { page } of pages) {
        if (text === '') await busy(page).waitFor({ state: 'detached' });
        else await page.waitForFunction(t => { const b = document.querySelector('#r-rail .rs[data-rid="livesession"] .drkwork'); return b && b.textContent === t; }, text);
      }
    }
    async function shoot(name) {
      if (!shots) return;
      for (const { scheme, page } of pages) {
        await page.screenshot({ path: path.join(shots, name + '-' + scheme + '.png'), animations: 'disabled' });
        const box = await row(page).boundingBox();
        await page.screenshot({ path: path.join(shots, name + '-' + scheme + '-crop.png'), animations: 'disabled',
          clip: { x: Math.max(0, box.x - 8), y: Math.max(0, box.y - 8), width: box.width + 16, height: box.height + 16 } });
      }
    }
    const set = q => fetch(updates + '?' + q).then(r => r.json());

    // Three of five children work: thinking; idle with a subagent; replying
    // with a subagent (counted once).
    await expect('3');
    for (const { scheme, page } of pages) {
      assert.equal(await row(page).locator('.kidtog .kn').textContent(), '5', scheme + ': fold count');
      assert.equal(await busy(page).getAttribute('title'), '3 sessions under this one at work', scheme + ': title');
      const next = await busy(page).evaluate(b => b.previousElementSibling && b.previousElementSibling.className);
      assert.equal(next, 'kidtog', scheme + ': the count sits right of the fold toggle');
      const anim = await busy(page).evaluate(b => getComputedStyle(b).animationName);
      assert.equal(anim, scheme === 'dark' ? 'rail-blink' : 'none', scheme + ': pulse honours reduced motion');
    }
    await shoot('rail-busy-3');

    // An idle child starts thinking: the count follows over the stream.
    await set('kidfour=think');
    await expect('4');
    // Subagents end on one child, it stays idle: one fewer.
    await set('kidtwo=idle');
    await expect('3');
    // Every child idle: no working count at all, the fold count stays.
    await set('kidone=idle&kidthree=idle&kidfour=idle');
    await expect('');
    for (const { scheme, page } of pages) assert.equal(await row(page).locator('.kidtog .kn').textContent(), '5', scheme + ': fold count when idle');
    await shoot('rail-busy-0');

    assert.deepEqual(errors, []);
    console.log('PASS: parent row shows its working descendants as a subscript after its fold count and follows live state');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
