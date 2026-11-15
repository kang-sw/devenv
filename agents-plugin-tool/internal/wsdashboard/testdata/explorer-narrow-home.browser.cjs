// Run only through TestExplorerNarrowTreeHomeAndPanelWidthBrowser's disposable fixture.
// Env: WS_NARROW_SHOTS=<dir> saves screenshots and metrics.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, route] = process.argv.slice(2);
const shots = process.env.WS_NARROW_SHOTS;

async function treeLayout(card) {
  return card.evaluate(el => {
    const r = x => x.getBoundingClientRect();
    const tree = el.querySelector('.ftree'), list = el.querySelector('.fdirectory');
    return { card: r(el).width, cardLeft: r(el).left, treeDisplay: getComputedStyle(tree).display, treeWidth: r(tree).width,
      listLeft: r(list).left, listWidth: r(list).width };
  });
}
async function setCardWidth(page, card, px) {
  await card.evaluate((el, w) => { const host = el.closest('.bcard'); host.style.width = w + 'px'; host.style.maxWidth = w + 'px'; }, px);
  await page.waitForTimeout(150);
}
// The left column of each page: the worktree page's Files panel, the session page's agents tree.
async function leftPanel(frame, selector) {
  return frame.locator(selector).evaluate(el => el.getBoundingClientRect().width);
}

(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const results = {};
    for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'ko-KR' });
      await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
      const errors = [], tab = await context.newPage(), sessionTab = await context.newPage();
      for (const p of [tab, sessionTab]) p.on('pageerror', e => errors.push(e.message));
      await tab.goto(base + route);
      const wt = tab.frameLocator('iframe.sframe.on');
      await wt.locator('#files .fentry[data-entry="worktree.txt"]').waitFor();
      await sessionTab.goto(base + '/s/livesession');
      const session = sessionTab.frameLocator('iframe.sframe.on');
      await session.locator('#convo').waitFor();
      const shot = (page, name) => shots ? page.screenshot({ path: path.join(shots, `${name}-${theme}.png`) }) : null;

      // 3. The worktree Files panel is exactly as wide as the session's left panel.
      const widths = {};
      for (const vw of [1440, 1100]) {
        await tab.setViewportSize({ width: vw, height: 1000 }); await sessionTab.setViewportSize({ width: vw, height: 1000 });
        await tab.waitForTimeout(200); await sessionTab.waitForTimeout(200);
        const worktree = await leftPanel(wt, '#files'), sessionPanel = await leftPanel(session, '#convo');
        widths[vw] = { worktree, session: sessionPanel };
        assert(Math.abs(worktree - sessionPanel) <= 1, `${vw}px: worktree Files ${worktree} vs session panel ${sessionPanel}`);
        if (vw === 1440) { await shot(tab, 'worktree-1440'); await shot(sessionTab, 'session-1440'); }
      }
      await tab.setViewportSize({ width: 1440, height: 1000 });
      await tab.waitForTimeout(200);
      const ownedFiles = await treeLayout(wt.locator('#files'));

      // 1. An explorer card narrower than 640px drops the folder tree.
      await wt.locator('.board-explorer').click();
      const card = wt.locator('.bcard[data-kind="explorer"] .fcard');
      await card.locator('.fentry[data-entry="worktree.txt"]').waitFor();
      await setCardWidth(tab, card, 1000);
      const wide = await treeLayout(card);
      assert.equal(wide.treeDisplay, 'block', 'wide card keeps the tree');
      assert(wide.treeWidth > 150 && wide.listLeft >= wide.cardLeft + wide.treeWidth - 1, `wide card tree beside the list: ${JSON.stringify(wide)}`);
      if (shots) {
        // The board column is narrower than 1000px at any width; unclip it only for the picture.
        await tab.setViewportSize({ width: 1800, height: 1000 });
        await card.evaluate(el => { el.closest('.bcol').style.overflow = 'visible'; });
        await tab.waitForTimeout(150);
        await card.screenshot({ path: path.join(shots, `wide-1000-${theme}.png`) });
        await card.evaluate(el => { el.closest('.bcol').style.overflow = ''; });
        await tab.setViewportSize({ width: 1440, height: 1000 });
        await tab.waitForTimeout(150);
      }
      await setCardWidth(tab, card, 520);
      const narrow = await treeLayout(card);
      assert.equal(narrow.treeDisplay, 'none', 'narrow card hides the tree');
      assert.equal(narrow.treeWidth, 0);
      assert(Math.abs(narrow.listLeft - narrow.cardLeft) <= 1 && Math.abs(narrow.listWidth - narrow.card) <= 1, `narrow list spans the card: ${JSON.stringify(narrow)}`);
      if (shots) await card.screenshot({ path: path.join(shots, `narrow-520-${theme}.png`) });
      await setCardWidth(tab, card, 1000);
      const restored = await treeLayout(card);
      assert.equal(restored.treeDisplay, 'block', 'widening restores the tree');
      // Back to the card's natural width, so no part of it hides under a scrolled board column.
      await card.evaluate(el => { const host = el.closest('.bcard'); host.style.width = host.style.maxWidth = ''; el.closest('.bcol').scrollLeft = 0; });

      // 2. Home sits right after up, and returns a nested folder to the root.
      const nav = await card.evaluate(el => {
        const home = el.querySelector('.fhome'), up = home.previousElementSibling;
        const box = b => { const r = b.getBoundingClientRect(); return [r.width, r.height]; };
        return { afterUp: /chevron-up/.test(up.innerHTML) && /house/.test(home.innerHTML),
          title: home.title, label: home.getAttribute('aria-label'), disabled: home.disabled, homeBox: box(home), upBox: box(up),
          homeIcon: box(home.querySelector('svg')), upIcon: box(up.querySelector('svg')) };
      });
      assert.equal(nav.afterUp, true, 'home follows up');
      assert(nav.title && nav.title === nav.label, 'home has a title and aria-label');
      assert.equal(nav.disabled, true, 'home disabled at the root');
      assert.deepEqual(nav.homeBox, nav.upBox, 'same button size as up');
      assert.deepEqual(nav.homeIcon, nav.upIcon, 'same icon size as up');
      await card.locator('.fentry[data-entry="alpha"]').dblclick();
      await card.locator('.fentry[data-entry="alpha/beta"]').dblclick();
      await card.locator('.fentry[data-entry="alpha/beta/deep.txt"]').waitFor();
      assert.deepEqual(await card.locator('.fcrumb-trail .fcrumb').allTextContents(), ['alpha', 'beta']);
      assert.equal(await card.locator('.fhome').isDisabled(), false, 'home enabled below the root');
      if (shots && theme === 'dark') {
        const a = await card.locator('.fback').boundingBox();
        await tab.screenshot({ path: path.join(shots, 'home-button-crop.png'), clip: { x: a.x - 8, y: a.y - 6, width: 340, height: a.height + 12 } });
      }
      await card.locator('.fhome').click();
      await card.locator('.fentry[data-entry="alpha"]').waitFor();
      assert.deepEqual(await card.locator('.fcrumb-trail .fcrumb').allTextContents(), [], 'breadcrumb shows only the root');
      assert.equal(await card.locator('.fcrumb-root').isVisible(), true);
      assert.equal(await card.locator('.fhome').isDisabled(), true, 'home disabled again at the root');

      assert.deepEqual(errors, []);
      results[theme] = { widths, ownedFiles, wide, narrow, restored, nav };
      await context.close();
    }
    const json = JSON.stringify(results, null, 2);
    if (shots) fs.writeFileSync(path.join(shots, 'metrics.json'), json + '\n');
    console.log(json);
    console.log('PASS: narrow card drops the tree, home returns to the root, worktree Files matches the session panel width.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
