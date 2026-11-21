// Run by TestCenterNavigationBrowser against rendered production assets.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);
(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const errors = [], failures = [];
  try {
    const tab = await browser.newPage({ viewport: { width: 1800, height: 800 }, reducedMotion: 'reduce' });
    tab.on('pageerror', e => errors.push(e.message));
    await tab.goto(base + '/fixture');
    const page = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await page.waitForFunction(() => window.WsCards && window.wbHistory);
    const shot = async name => { if (shots) await tab.screenshot({ path: path.join(shots, name + '.png') }); };
    const ids = { stream: 'item-15', 'first-pin': 'item-44', 'second-pin': 'item-73' };
    const target = name => page.locator('#' + ids[name]);
    const settle = () => page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
    async function check(name, label) {
      const node = target(name);
      await node.waitFor({ state: 'visible' });
      const geometry = await node.evaluate(el => {
        let scroller = el.parentElement;
        while (scroller && !(scroller.scrollHeight > scroller.clientHeight + 2 && /(auto|scroll)/.test(getComputedStyle(scroller).overflowY))) scroller = scroller.parentElement;
        const r = el.getBoundingClientRect(), s = scroller ? scroller.getBoundingClientRect() : { top: 0, bottom: innerHeight };
        const head = el.closest('.topic')?.querySelector('.tophead');
        const style = head && getComputedStyle(head);
        let top = Math.max(0, s.top);
        if (style?.position === 'sticky') top = Math.max(top, head.getBoundingClientRect().bottom);
        const host = el.closest('.topic')?.closest('.gal-pane, #bcol, .wrap');
        const wrap = host?.querySelector('.oq-wrap.wb-oq-top'), pill = wrap?.querySelector('.oq-pill');
        if (pill && !pill.hidden && pill.getBoundingClientRect().height) top = Math.max(top, pill.getBoundingClientRect().bottom);
        if (scroller) top = Math.max(top, s.top + (parseFloat(getComputedStyle(scroller).scrollPaddingTop) || 0));
        return { top: r.top, bottom: r.bottom, center: (r.top + r.bottom) / 2, viewCenter: (top + Math.min(innerHeight, s.bottom)) / 2, unobstructedTop: top,
          head: head?.getBoundingClientRect().toJSON(), pill: pill?.getBoundingClientRect().toJSON(), wrap: wrap?.getBoundingClientRect().toJSON(),
          scrollTop: scroller?.scrollTop, max: scroller && scroller.scrollHeight - scroller.clientHeight };
      });
      console.log(label, JSON.stringify(geometry));
      if (label.startsWith('pinned-gallery')) assert(geometry.scrollTop > 100 && geometry.max - geometry.scrollTop > 100, `${label}: fixture must allow scrolling on both sides`);
      await shot(label);
      if (Math.abs(geometry.center - geometry.viewCenter) >= 5) failures.push(`${label}: target should be centered: ${JSON.stringify(geometry)}`);
    }
    async function navigate(name, byElement) {
      const id = ids[name];
      assert(id, `${name} panel has an id`);
      const result = await page.evaluate(({id, byElement}) => {
        const found = window.wbHistory.navigate(byElement ? document.getElementById(id) : id);
        return found && found.id;
      }, { id, byElement });
      assert.equal(result, id, 'navigate returns the resolved target');
      await settle();
    }
    for (const theme of ['dark', 'light']) {
      if (!await tab.locator('#gear').isVisible()) await tab.locator('#railbtn').click();
      await tab.locator('#gear').click();
      await tab.locator(`#gmenu [data-theme-set="${theme}"]`).click();
      await tab.waitForFunction(theme => document.documentElement.dataset.theme === theme, theme);
      await page.evaluate(() => localStorage.clear());
      await page.goto(base + '/s/centernav');
      await page.waitForFunction(theme => document.documentElement.dataset.theme === theme, theme);
      await tab.keyboard.press('Escape');
      await page.locator('section.topic[data-topic="stream"] .trecap a[href="#item-15"]').click();
      await settle();
      await settle();
      await page.waitForTimeout(500);
      await check('stream', `stream-ref-after-${theme}`);
      // Bare (N) is not parsed by whiteboard markdown. Supply the equivalent
      // anchor explicitly to exercise the separately rendered link click.
      await page.evaluate(() => {
        const recap = document.querySelector('section.topic[data-topic="stream"] .trecap');
        const link = document.createElement('a'); link.href = '#item-15'; link.textContent = '(15)'; link.id = 'fixture-bare-reference'; recap.append(' ', link);
        document.scrollingElement.scrollTop = 0;
      });
      const bare = page.locator('#fixture-bare-reference');
      await bare.scrollIntoViewIfNeeded();
      const before = await target('stream').evaluate(el => { const r = el.getBoundingClientRect(); return (r.top + r.bottom) / 2; });
      await bare.click(); await page.waitForTimeout(500);
      const after = await target('stream').evaluate(el => { const r = el.getBoundingClientRect(); return (r.top + r.bottom) / 2; });
      const middle = await page.evaluate(() => { const p = document.querySelector('#bcol .oq-wrap.wb-oq-top .oq-pill:not([hidden])'); return ((p?.getBoundingClientRect().bottom || 0) + innerHeight) / 2; });
      assert(Math.abs(after - middle) < Math.abs(before - middle), `bare (15) link moves target closer to center: ${before} -> ${after}`);
      assert.equal(await page.evaluate(() => window.wbHistory.resolveTarget('check-15')?.id), 'item-15', 'check alias resolves to live item');
      {
        await page.evaluate(() => {
          for (const name of ['first-pin', 'second-pin']) window.WsCards.move(document.querySelector(`section.topic[data-topic="${name}"]`), { pin: true, index: 99 });
        });
        await settle();
      }
      const first = page.locator('section.topic[data-topic="first-pin"]');
      const second = page.locator('section.topic[data-topic="second-pin"]');
      assert(await first.evaluate(el => !!el.closest('#bgal .gal-page')), 'first topic is pinned');
      assert(await second.evaluate(el => !!el.closest('#bgal .gal-page')), 'second topic is pinned');
      await page.evaluate(() => window.WsCards.activate(document.querySelector('section.topic[data-topic="second-pin"]')));
      assert(await first.evaluate(el => !el.closest('.gal-page').hasAttribute('data-active')), 'target page starts inactive');
      const mainBefore = await page.locator('#bcol').evaluate(el => el.scrollTop);
      await navigate('first-pin', false);
      assert.equal(await page.locator('#bcol').evaluate(el => el.scrollTop), mainBefore, 'gallery navigation preserves main board scroll');
      assert(await first.evaluate(el => el.closest('.gal-page').hasAttribute('data-active')), 'navigation activates target page');
      await check('first-pin', `pinned-gallery-jump-${theme}`);
      await navigate('first-pin', true);
      assert(await first.evaluate(el => el.closest('.gal-page').hasAttribute('data-active')), 'active page remains active');
      await check('first-pin', `pinned-gallery-active-${theme}`);
    }
    assert.deepEqual(errors, [], 'no uncaught browser errors');
    assert.deepEqual(failures, [], 'centered navigation');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
