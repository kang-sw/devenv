// Run only through TestExplorerTabRowAndFloatWidthBrowser's disposable fixture.
// Env: WS_TABROW_SHOTS=<dir> saves screenshots and metrics.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, route] = process.argv.slice(2);
const shots = process.env.WS_TABROW_SHOTS;

// Bar geometry of one explorer instance (pane = its .fpane / .fcard root).
async function bar(pane) {
  return pane.evaluate(el => {
    const r = x => { const b = x.getBoundingClientRect(); return { left: b.left, right: b.right, top: b.top, bottom: b.bottom, width: b.width, height: b.height }; };
    const q = s => el.querySelector('.fbar ' + s);
    const chips = [...el.querySelectorAll('.fbar .ftab')];
    return {
      bar: r(el.querySelector('.fbar')), plus: r(q('.ftab-new')), back: r(q('.fback')), ctl: r(q('.fbar-ctl')), location: r(q('.flocation')),
      chips: chips.map(r), chipsShown: chips.filter(c => getComputedStyle(c).display !== 'none').length,
      plusLabel: q('.ftab-new').getAttribute('aria-label'), tablist: q('.ftabs').getAttribute('role'),
    };
  });
}
function oneTab(m, name) {
  assert.equal(m.chips.length, 1, `${name}: one tab`);
  assert.equal(m.chipsShown, 0, `${name}: the lone tab shows no chip`);
  assert(m.plus.left - m.bar.left <= 8, `${name}: + at the far left (${m.plus.left - m.bar.left})`);
  assert(m.plus.right <= m.back.left, `${name}: + before the nav buttons`);
  const mid = m.plus.top + m.plus.height / 2;
  assert(mid > m.ctl.top && mid < m.ctl.bottom, `${name}: + and nav share one row (plus mid ${mid}, ctl ${m.ctl.top}-${m.ctl.bottom})`);
  assert(m.bar.height <= 31, `${name}: single-row bar (${m.bar.height})`);
  assert(m.plusLabel, `${name}: + keeps its aria-label`);
}
function multiTab(m, name, count) {
  assert.equal(m.chipsShown, count, `${name}: ${count} chips shown`);
  const tabsBottom = Math.max(...m.chips.map(c => c.bottom));
  assert(Math.max(...m.chips.map(c => c.top)) < m.ctl.top, `${name}: tabs row above the address row`);
  assert(m.ctl.top >= tabsBottom - 1, `${name}: address row below every tab (ctl ${m.ctl.top}, tabs ${tabsBottom})`);
  assert(m.plus.bottom <= m.ctl.top + 1, `${name}: + stays on the tab row`);
  assert(m.back.left - m.bar.left <= 8, `${name}: address row starts at the left edge (${m.back.left - m.bar.left})`);
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
      const res = results[theme] = {};

      // 1. Worktree page Files panel: one tab, two tabs, back to one.
      await tab.goto(base + route);
      const wt = tab.frameLocator('iframe.sframe.on');
      const panel = wt.locator('#files');
      await panel.locator('.fentry[data-entry="worktree.txt"]').waitFor();
      res.panelOne = await bar(panel); oneTab(res.panelOne, 'worktree panel');
      if (shots && theme === 'dark') await panel.screenshot({ path: path.join(shots, 'onetab-dark.png') });
      await panel.locator('.ftab-new').click();
      await panel.locator('.ftab').nth(1).waitFor();
      res.panelTwo = await bar(panel); multiTab(res.panelTwo, 'worktree panel', 2);
      if (shots) await panel.screenshot({ path: path.join(shots, `twotabs-${theme}.png`) });
      await panel.locator('.ftab-close').nth(1).click();
      res.panelBack = await bar(panel); oneTab(res.panelBack, 'worktree panel after close');

      // 2. Explorer card: same rule, also at 520 px with two tabs.
      await wt.locator('.board-explorer').click();
      const card = wt.locator('.bcard[data-kind="explorer"] .fcard');
      await card.locator('.fentry[data-entry="worktree.txt"]').waitFor();
      oneTab(await bar(card), 'explorer card');
      await card.locator('.ftab-new').click();
      await card.locator('.ftab').nth(1).waitFor();
      await card.evaluate(el => { const host = el.closest('.bcard'); host.style.width = '520px'; host.style.maxWidth = '520px'; });
      await tab.waitForTimeout(150);
      res.card520 = await bar(card); multiTab(res.card520, 'explorer card 520', 2);
      if (shots && theme === 'dark') await card.screenshot({ path: path.join(shots, 'narrow-520-twotabs-dark.png') });

      // 3. Session page: the Files tab and the docked-left toolbar explorer.
      await sessionTab.goto(base + '/s/livesession');
      const session = sessionTab.frameLocator('iframe.sframe.on');
      await session.locator('#convo').waitFor();
      await session.locator('#ctabs button[data-ctab="files"]').click();
      await session.locator('#files .fbar .ftab-new').waitFor();
      oneTab(await bar(session.locator('#files')), 'session Files tab');
      res.float = {};
      for (const vw of [1440, 1100]) {
        await sessionTab.setViewportSize({ width: vw, height: 1000 });
        await sessionTab.waitForTimeout(200);
        const toggle = session.locator('.board-files');
        if (vw === 1440) await toggle.click();
        const sheet = session.locator('.fpane[data-max]');
        await sheet.locator('.fbar .ftab-new').waitFor();
        await sessionTab.waitForTimeout(200);
        const m = await sheet.evaluate(el => {
          const col = document.getElementById('chead').getBoundingClientRect(), grid = document.getElementById('grid').getBoundingClientRect(), s = el.getBoundingClientRect();
          return { docked: el.hasAttribute('data-docked'), sheet: s.width, column: col.width, sheetLeft: s.left, columnLeft: col.left, top: s.top, columnTop: grid.top, height: s.height, columnHeight: grid.height };
        });
        res.float[vw] = m;
        assert(m.docked, `${vw}px: toolbar explorer is docked`);
        assert(Math.abs(m.sheet - m.column) <= 1, `${vw}px: docked explorer ${m.sheet} vs left column ${m.column}`);
        assert(Math.abs(m.sheetLeft - m.columnLeft) <= 1, `${vw}px: docked explorer sits over the left column (${m.sheetLeft} vs ${m.columnLeft})`);
        assert(Math.abs(m.top - m.columnTop) <= 1 && Math.abs(m.height - m.columnHeight) <= 1, `${vw}px: docked explorer covers the complete left column`);
        if (vw === 1440) oneTab(await bar(sheet), 'floating explorer');
        if (shots && (vw === 1440 ? theme === 'dark' : theme === 'light')) await sessionTab.screenshot({ path: path.join(shots, `float-${vw}-${theme}.png`) });
      }
      // Docking fixes the sheet to the left column; only non-docked sheets extend.
      assert.equal(await session.locator('.board-toolbar .sheet-width').isVisible(), false, 'docked explorer hides the width toggle');
      // Opening at 1100 measures the left column there, not a stale snapshot.
      await session.locator('.board-files').click();
      await session.locator('.board-files').click();
      const reopened = await session.locator('.fpane[data-max]').evaluate(el => {
        const sheet = el.getBoundingClientRect(), column = document.getElementById('chead').getBoundingClientRect();
        return { docked: el.hasAttribute('data-docked'), width: sheet.width, columnWidth: column.width, left: sheet.left, columnLeft: column.left };
      });
      assert(reopened.docked && Math.abs(reopened.width - reopened.columnWidth) <= 1 && Math.abs(reopened.left - reopened.columnLeft) <= 1, `reopened at 1100: ${JSON.stringify(reopened)}`);

      assert.deepEqual(errors, []);
      await context.close();
    }
    const json = JSON.stringify(results, null, 2);
    if (shots) fs.writeFileSync(path.join(shots, 'metrics.json'), json + '\n');
    console.log(json);
    console.log('PASS: one tab leads with +, two tabs own a row above the address row, toolbar explorer docks over the complete left column.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
