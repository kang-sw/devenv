// Run only through TestWorktreePageFileCardStyleBrowser's disposable fixture.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, route] = process.argv.slice(2);
const evidence = process.env.WS_STYLE_EVIDENCE_DIR;

async function headerMetrics(header) {
  return header.evaluate(el => {
    const s = getComputedStyle(el), icon = el.querySelector('svg'), i = getComputedStyle(icon);
    return { height: el.getBoundingClientRect().height, padding: s.padding,
      fontSize: s.fontSize, fontWeight: s.fontWeight, color: s.color,
      letterSpacing: s.letterSpacing, textTransform: s.textTransform,
      iconWidth: i.width, iconHeight: i.height, iconColor: i.color };
  });
}
async function coreMetrics(pane) {
  return pane.evaluate(el => {
    const css = sel => getComputedStyle(el.querySelector(sel));
    const ws = css('.fworkspace');
    return { barPadding: css('.fbar').padding, searchPadding: css('.fsrow').padding,
      footerPadding: css('.ffooter').padding, workspaceMargin: ws.margin,
      borderWidth: ws.borderTopWidth, borderStyle: ws.borderTopStyle,
      borderColor: ws.borderTopColor, radius: ws.borderTopLeftRadius };
  });
}
async function capture(page, filename, clip) {
  if (!evidence) return;
  await page.screenshot({ path: path.join(evidence, filename), ...(clip ? { clip } : {}) });
}
async function leftColumn(frame) {
  return frame.locator('#grid').evaluate(el => {
    const r = el.getBoundingClientRect();
    const left = (el.querySelector('#convo') || el.querySelector('#files')).getBoundingClientRect();
    const owner = window.frameElement.getBoundingClientRect();
    return { x: owner.x + r.x, y: owner.y + r.y, width: left.width, height: r.height };
  });
}

(async () => {
  if (evidence) fs.mkdirSync(evidence, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const results = {};
    for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({ viewport: { width: 1600, height: 1000 }, locale: 'ko-KR' });
      await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
      const errors = [], tab = await context.newPage(), sessionTab = await context.newPage();
      for (const p of [tab, sessionTab]) p.on('pageerror', e => errors.push(e.message));
      await tab.goto(base + route);
      const wt = tab.frameLocator('iframe.sframe.on');
      await wt.locator('#files .fentry[data-entry="worktree.txt"]').waitFor();
      await sessionTab.goto(base + '/s/livesession');
      const session = sessionTab.frameLocator('iframe.sframe.on');
      await session.locator('#ctext').waitFor({ state: 'attached' });
      const worktreeHeader = await headerMetrics(wt.locator('#chead'));
      // The agents header lives in the popover the badge opens.
      await session.locator('#sbadge').click();
      const sessionHeader = await headerMetrics(session.locator('#r-tree > .thead'));
      assert.deepEqual(worktreeHeader, sessionHeader, 'Files uses the shared session panel header metrics');
      assert.equal(worktreeHeader.height, 30);
      assert.equal(await wt.locator('#chead').getAttribute('class'), 'thead', 'no whiteboard chead class collision');
      const owned = await coreMetrics(wt.locator('#files'));
      assert.equal(owned.barPadding, '0px 6px 0px 4px');
      assert.equal(owned.searchPadding, '3px 6px');
      assert.equal(owned.footerPadding, '0px 6px 0px 10px');
      assert.equal(owned.workspaceMargin, '0px');
      assert.equal(owned.borderWidth, '0px', 'page-owned browser stays unframed');
      await capture(tab, `worktree-${theme}.png`);
      await capture(tab, `worktree-left-${theme}.png`, await leftColumn(wt));
      await capture(sessionTab, `session-left-${theme}.png`, await leftColumn(session));
      // The session's page-owned Files browser must also keep its flush core.
      await session.locator('[data-ctab="files"]').click();
      await session.locator('#files .fworkspace').waitFor();
      assert.deepEqual(await coreMetrics(session.locator('#files')), owned);
      await wt.locator('.board-explorer').click();
      const card = wt.locator('.bcard[data-kind="explorer"] .fcard');
      await card.locator('.fentry[data-entry="worktree.txt"]').waitFor();
      const framed = await coreMetrics(card);
      assert.deepEqual(framed, owned, 'explorer cards are frameless and edge to edge like page-owned browsers');
      if (evidence) await card.screenshot({ path: path.join(evidence, `filecard-${theme}.png`) });
      const sticky = await card.evaluate(async el => {
        const workspace = el.querySelector('.fworkspace').getBoundingClientRect(), result = {};
        for (const [selector, headSelector] of [['.ftree', '.ftree-head'], ['.fdirectory', '.fdetail-head']]) {
          const scroller = el.querySelector(selector), head = el.querySelector(headSelector);
          scroller.scrollTop = 220;
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
          const r = head.getBoundingClientRect(), s = scroller.getBoundingClientRect();
          result[selector] = { scrollTop: scroller.scrollTop, height: r.height, offset: r.top - s.top,
            withinFrame: r.left >= workspace.left - 0.5 && r.right <= workspace.right + 0.5 && r.bottom < workspace.bottom,
            hit: head.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)) };
        }
        return result;
      });
      for (const value of Object.values(sticky)) {
        assert(value.scrollTop > 0, 'fixture really scrolls');
        assert.equal(value.height, 28);
        assert.equal(value.offset, 0, 'sticky header stays at the scroller top');
        assert.equal(value.withinFrame, true, 'header is inside the workspace');
        assert.equal(value.hit, true, 'header is painted and reachable, not clipped');
      }
      if (evidence) await card.screenshot({ path: path.join(evidence, `filecard-sticky-${theme}.png`) });
      await tab.setViewportSize({ width: 446, height: 1000 });
      await card.locator('.fworkspace').waitFor();
      assert((await card.boundingBox()).width <= 480, 'exercise the narrow container breakpoint');
      assert.equal(await card.locator('.fworkspace').evaluate(el => getComputedStyle(el).margin), '0px');
      await card.evaluate(el => { el.querySelector('.ftree').scrollTop = 0; el.querySelector('.fdirectory').scrollTop = 0; });
      if (evidence) await card.screenshot({ path: path.join(evidence, `filecard-narrow-${theme}.png`) });
      assert.deepEqual(errors, []);
      results[theme] = { worktreeHeader, sessionHeader, owned, framed, sticky };
      await context.close();
    }
    const json = JSON.stringify(results, null, 2);
    if (evidence) fs.writeFileSync(path.join(evidence, 'metrics.json'), json + '\n');
    console.log(json);
    console.log('PASS: shared headers, frameless cards, both page-owned cores, sticky heads, dark/light and narrow.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
