// Run only through TestExplorerCardFramelessTabWrapBrowser's disposable fixture.
// Env: WS_TABWRAP_SHOTS=<dir> saves screenshots, WS_TABWRAP_PREFIX=before|after names them,
// WS_TABWRAP_SOFT=1 reports violations instead of failing (used to capture the "before" state).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, route] = process.argv.slice(2);
const shots = process.env.WS_TABWRAP_SHOTS, prefix = process.env.WS_TABWRAP_PREFIX || 'after', soft = !!process.env.WS_TABWRAP_SOFT;
const problems = [];
function ok(cond, msg) { if (cond) return; if (soft) problems.push(msg); else assert.fail(msg); }

async function metrics(card) {
  return card.evaluate(el => {
    const r = x => { const b = x.getBoundingClientRect(); return { left: b.left, right: b.right, top: b.top, bottom: b.bottom, width: b.width, height: b.height }; };
    const ws = el.querySelector('.fworkspace'), bar = el.querySelector('.fbar');
    const wcs = getComputedStyle(ws), cr = r(el);
    const hit = x => { const b = x.getBoundingClientRect(); return x.contains(document.elementFromPoint(b.left + b.width / 2, b.top + b.height / 2)); };
    const tabs = [...el.querySelectorAll('.ftab')].map(x => ({ ...r(x), close: hit(x.querySelector('.ftab-close')), closeVisible: getComputedStyle(x.querySelector('.ftab-close')).visibility !== 'hidden', select: hit(x.querySelector('.ftab-select')) }));
    const plus = el.querySelector('.ftab-new'), ctl = el.querySelector('.fbar-ctl') || el.querySelector('.flocation');
    const ctlButtons = ctl ? [...el.querySelectorAll('.fbar button:not(.ftab-new):not(.ftab-select):not(.ftab-close)')].filter(b => !b.hidden && b.offsetParent).map(b => ({ title: b.title || b.className, hit: hit(b) })) : [];
    return {
      card: cr, ws: r(ws), wsLeftGap: ws.getBoundingClientRect().left - cr.left, wsRightGap: cr.right - ws.getBoundingClientRect().right,
      border: wcs.borderTopWidth + '/' + wcs.borderRightWidth + '/' + wcs.borderBottomWidth + '/' + wcs.borderLeftWidth, radius: wcs.borderTopLeftRadius, margin: wcs.margin,
      barLeftGap: bar.getBoundingClientRect().left - cr.left, bar: r(bar), tabs, plus: r(plus), plusHit: hit(plus),
      ctl: ctl ? r(ctl) : null, ctlButtons,
    };
  });
}

(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const results = {};
    for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'ko-KR' });
      await context.addCookies([{ name: 'ws_dashboard_theme', value: theme, url: base }]);
      const errors = [], tab = await context.newPage();
      tab.on('pageerror', e => errors.push(e.message));
      await tab.goto(base + route);
      const wt = tab.frameLocator('iframe.sframe.on');
      await wt.locator('.board-explorer').click();
      const card = wt.locator('.bcard[data-kind="explorer"] .fcard');
      await card.locator('.fentry[data-entry="worktree.txt"]').waitFor();
      const shot = name => shots ? card.screenshot({ path: path.join(shots, `${prefix}-${name}-${theme}.png`) }) : null;

      // 1 tab at 1440: edge-to-edge panes, no frame, single unchanged tab row.
      const one = await metrics(card);
      await shot('margins');
      ok(one.wsLeftGap <= 1 && one.wsRightGap <= 1, `panes edge to edge: left ${one.wsLeftGap}, right ${one.wsRightGap}`);
      ok(one.border === '0px/0px/0px/0px', `pane container border ${one.border}`);
      ok(one.radius === '0px', `pane container radius ${one.radius}`);
      ok(one.margin === '0px', `pane container margin ${one.margin}`);
      ok(one.barLeftGap <= 4.5, `tab bar has no card inset: ${one.barLeftGap}`);
      // A lone tab shows no chip: + leads the single row (its chip box collapses to 0).
      ok(one.tabs.length === 1 && one.tabs[0].width === 0 && one.bar.height <= 31 && one.ctl && one.ctl.top < one.plus.bottom, `single row with 1 tab: bar ${one.bar.height}`);

      await card.locator('.fentry[data-entry="devenv-explorer-workspace"]').dblclick();
      await card.locator('.ftab-select .fn', { hasText: 'devenv-explorer-workspace' }).first().waitFor({ state: 'attached' }); // a lone tab's chip is hidden
      // Narrower card: add tabs until 12 exist, then force the card to 800 px (the 1440 board column is 826 px).
      for (let i = 1; i < 12; i++) await card.locator('.ftab-new').click();
      await wt.locator('.bcard[data-kind="explorer"] .ftab').nth(11).waitFor();
      const w800 = await card.evaluate(el => { const host = el.closest('.bcard'); host.style.width = '800px'; host.style.maxWidth = '800px'; return el.getBoundingClientRect().width; });
      const w = {};
      for (const [name, width] of [['tabs12', 800], ['tabs12-420', 420]]) {
        await card.evaluate((el, px) => { const host = el.closest('.bcard'); host.style.width = px + 'px'; host.style.maxWidth = px + 'px'; }, width);
        await tab.waitForTimeout(150);
        const m = await metrics(card);
        w[name] = m;
        await shot(name);
        const inside = m.tabs.every(t => t.left >= m.card.left - 0.5 && t.right <= m.card.right + 0.5);
        const rows = new Set(m.tabs.map(t => Math.round(t.top))).size;
        ok(inside, `${name}: every tab inside the card`);
        ok(rows >= 2, `${name}: tabs occupy ${rows} rows`);
        ok(m.tabs.every(t => t.close && t.closeVisible && t.select), `${name}: every tab and close button reachable`);
        ok(m.plusHit && m.plus.right <= m.card.right + 0.5, `${name}: plus reachable`);
        ok(m.ctlButtons.length >= 7 && m.ctlButtons.every(b => b.hit), `${name}: right-side buttons clickable (${m.ctlButtons.length})`);
        const tabsBottom = Math.max(...m.tabs.map(t => t.bottom));
        ok(m.ctl.top >= tabsBottom - 1, `${name}: button array moved below the tab rows (ctl.top ${m.ctl.top}, tabs bottom ${tabsBottom})`);
        ok(m.ctl.right <= m.card.right + 0.5, `${name}: button array inside the card`);
      }
      assert.deepEqual(errors, []);
      results[theme] = { one, w800, tabs12: w['tabs12'], tabs12_420: w['tabs12-420'] };
      await context.close();
    }
    const json = JSON.stringify(results, null, 2);
    if (shots) fs.writeFileSync(path.join(shots, `${prefix}-metrics.json`), json + '\n');
    console.log(json);
    if (problems.length) console.log('PROBLEMS:\n' + problems.join('\n'));
    else console.log('PASS: frameless edge-to-edge panes, wrapped tabs, button array on its own row.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
