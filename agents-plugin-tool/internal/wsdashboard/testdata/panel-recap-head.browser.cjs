// Production RenderRegions + dashboard HTML; no mockup or daemon.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    if (shots) fs.mkdirSync(shots, { recursive: true });
    for (const theme of ['light', 'dark']) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 1000 }, colorScheme: theme, reducedMotion: 'reduce' });
      await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
      const page = await context.newPage(), errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(base + '/s/panelrecap?stage=panel');
      await page.evaluate(theme => document.documentElement.dataset.theme = theme, theme);
      const topic = name => page.locator(`section.topic[data-topic="${name}"]`);
      const recap = name => topic(name).locator('.tophead .trecap');
      async function check(name, kind, mark, text) {
        const r = recap(name);
        await r.waitFor();
        assert.equal(await r.count(), 1, 'exactly one head summary');
        assert.equal(await r.getAttribute('data-kind'), kind);
        assert.equal((await r.locator('.evi').innerText()).trim(), mark);
        assert((await r.locator('.rtext').innerText()).includes(text));
        assert(await r.getAttribute('data-entry'), 'summary points at an entry');
      }
      async function shot(name, target) {
        if (shots) {
          await page.mouse.move(0, 0);
          await target.screenshot({ path: path.join(shots, `${name}-${theme}.png`) });
        }
      }
      await check('panel-only', 'panel', 'P', '패널 recap은 최신');
      await check('mixed', 'panel', 'P', '수리 3/5 병합');
      await shot('panel-only', topic('panel-only'));
      await shot('log-then-later-panel', topic('mixed'));
      const target = await recap('mixed').getAttribute('data-entry');
      await recap('mixed').locator('.rat').click();
      await page.waitForFunction(id => document.getElementById(id)?.querySelector('.entry-panel.flash'), target);
      const row = page.locator('#' + target);
      assert.equal(await row.getAttribute('data-kind'), 'panel', 'recap jump reaches panel row, not earlier log');
      assert.equal(await row.locator('details.entry-panel').evaluate(d => d.open), true);
      assert(await row.evaluate(el => { const r = el.getBoundingClientRect(); return r.bottom > 0 && r.top < innerHeight; }), 'jumped panel is visible');
      const before = await recap('panel-only').locator('.rtext').innerText();
      await page.locator('.board-overview').focus();
      await page.keyboard.press('Control+q');
      await page.waitForFunction(() => document.documentElement.hasAttribute('data-overview'));
      await page.locator('.board-overview').evaluate(el => el.blur());
      await page.waitForTimeout(400); // production Overview size transition
      assert.equal(await recap('panel-only').locator('.rtext').innerText(), before, 'Overview preserves same full summary text');
      await check('mixed', 'panel', 'P', '수리 3/5 병합');
      const clamp = await recap('panel-only').locator('.rtext').evaluate(el => {
        const s = getComputedStyle(el);
        return { lines: s.webkitLineClamp, height: el.getBoundingClientRect().height, line: parseFloat(s.lineHeight), full: el.scrollHeight };
      });
      assert.equal(clamp.lines, '4');
      assert(clamp.height <= clamp.line * 4 + 2, JSON.stringify(clamp));
      assert(clamp.full > clamp.height, 'long recap really overflows and is clamped');
      if (shots) {
        // Capture only rendered board content, not the column's viewport-sized
        // empty tail. Move away from controls; Escape exits Overview itself,
        // so its persistent bottom-of-viewport help stays outside this crop.
        await page.mouse.move(0, 0);
        await page.waitForTimeout(700);
        const clip = await page.locator('#bcol').evaluate(el => {
          const board = el.getBoundingClientRect();
          const topics = [...el.querySelectorAll('section.topic')].map(t => t.getBoundingClientRect());
          const bottom = Math.max(...topics.map(r => r.bottom));
          return { x: board.left, y: board.top, width: board.width, height: Math.min(innerHeight - board.top, bottom - board.top + 20) };
        });
        await page.screenshot({ path: path.join(shots, `overview-${theme}.png`), clip });
      }
      await page.keyboard.press('Control+q');
      await page.waitForFunction(() => !document.documentElement.hasAttribute('data-overview'));
      await page.goto(base + '/s/panelrecap?stage=log');
      await check('mixed', 'agent', 'L', 'Later agent log');
      await check('panel-only', 'panel', 'P', '패널 recap은 최신');
      assert.deepEqual(errors, [], 'no browser script errors');
      console.log(`${theme}: panel-only P; log → panel P → later log L; panel jump; Ctrl+Q same summary + four-line clamp passed`);
      await context.close();
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
