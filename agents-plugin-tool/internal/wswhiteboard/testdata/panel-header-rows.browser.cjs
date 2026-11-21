const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);

const baseURL = process.argv[2];
const shotsDir = process.argv[3] || '';
if (shotsDir) fs.mkdirSync(shotsDir, { recursive: true });

async function mountFramesAndFoldSecondPanel(page) {
  await page.evaluate(() => {
    // Collapse p2 so captures & assertions cover both expanded (p1) and
    // collapsed (p2, historical p1 v1) panels simultaneously.
    const p2 = document.getElementById('p2');
    if (p2) p2.open = false;

    function mount(root) {
      root.querySelectorAll('div.doc-frame[data-srcdoc]').forEach(p => {
        for (let n = p.parentElement; n; n = n.parentElement) if (n.tagName === 'DETAILS' && !n.open) return;
        const f = document.createElement('iframe');
        for (const a of p.attributes) if (a.name !== 'data-sandbox') f.setAttribute(a.name, a.value);
        f.setAttribute('sandbox', p.dataset.sandbox);
        f.srcdoc = p.dataset.srcdoc;
        p.replaceWith(f);
      });
    }
    document.addEventListener('toggle', e => { if (e.target.open) mount(e.target); }, true);
    window.addEventListener('message', e => {
      if (!e.data || e.data.wbDoc !== 'height') return;
      const f = [...document.querySelectorAll('iframe.doc-frame')].find(f => f.contentWindow === e.source);
      if (f) f.style.height = e.data.height + 'px';
    });
    mount(document);
  });
  await page.waitForTimeout(350);
}

async function assertTwoRowPanels(page, label, expectDashboardControls) {
  const metrics = await page.evaluate((expectDash) => {
    const docOverflow = document.documentElement.scrollWidth - window.innerWidth;
    const panels = [...document.querySelectorAll('.wb-entry.panel')].map(article => {
      const details = article.querySelector('.entry-panel');
      const sum = details.querySelector(':scope > summary');
      const recap = sum.querySelector(':scope > .entry-recap');
      const pmeta = sum.querySelector(':scope > .pmeta');
      const pmark = sum.querySelector(':scope > .pmark');
      const pchev = sum.querySelector(':scope > .pchev');
      const dmax = sum.querySelector(':scope > .d-max');
      const rSum = sum.getBoundingClientRect();
      const rRecap = recap.getBoundingClientRect();
      const rMeta = pmeta.getBoundingClientRect();
      const rMark = pmark ? pmark.getBoundingClientRect() : null;
      const rChev = pchev ? pchev.getBoundingClientRect() : null;
      const rDmax = dmax ? dmax.getBoundingClientRect() : null;
      return {
        id: article.id,
        panelId: article.getAttribute('data-panel'),
        open: details.open,
        msg: details.classList.contains('msg'),
        historical: article.classList.contains('historical'),
        sumWidth: rSum.width,
        sumOverflow: sum.scrollWidth - sum.clientWidth,
        articleOverflow: article.scrollWidth - article.clientWidth,
        recap: { left: rRecap.left, right: rRecap.right, top: rRecap.top, bottom: rRecap.bottom, width: rRecap.width, height: rRecap.height },
        pmeta: { left: rMeta.left, right: rMeta.right, top: rMeta.top, bottom: rMeta.bottom, width: rMeta.width, height: rMeta.height },
        pmarkVisible: !!(rMark && rMark.width > 0),
        pchev: rChev ? { left: rChev.left, right: rChev.right, top: rChev.top, bottom: rChev.bottom, width: rChev.width } : null,
        dmax: rDmax ? { left: rDmax.left, right: rDmax.right, top: rDmax.top, bottom: rDmax.bottom, width: rDmax.width } : null,
        expectDash,
      };
    });
    return { docOverflow, panels };
  }, expectDashboardControls);

  assert(metrics.docOverflow <= 1, `${label}: document horizontal overflow ${metrics.docOverflow}`);
  assert(metrics.panels.length >= 3, `${label}: expected >= 3 panels, got ${metrics.panels.length}`);

  for (const p of metrics.panels) {
    assert(p.sumOverflow <= 1, `${label} ${p.id}: summary horizontal overflow ${p.sumOverflow}`);
    assert(p.articleOverflow <= 1, `${label} ${p.id}: article horizontal overflow ${p.articleOverflow}`);
    assert(p.pmeta.top >= p.recap.bottom - 1, `${label} ${p.id}: .pmeta (${p.pmeta.top}) must sit on row 2 below .entry-recap (${p.recap.bottom})`);
    assert(p.recap.width >= p.sumWidth * 0.62, `${label} ${p.id}: .entry-recap width (${p.recap.width}) squeezed relative to summary (${p.sumWidth})`);
    assert(p.pmeta.left >= p.recap.left - 1, `${label} ${p.id}: .pmeta left (${p.pmeta.left}) aligned under .entry-recap (${p.recap.left})`);
    if (!p.msg) {
      assert(p.pmarkVisible, `${label} ${p.id}: .pmark should be visible`);
      assert(p.pchev && p.pchev.width > 0, `${label} ${p.id}: .pchev should be visible`);
      assert(p.pchev.left >= p.recap.right - 1, `${label} ${p.id}: .pchev (${p.pchev.left}) must not overlap .entry-recap (${p.recap.right})`);
      assert(p.pchev.bottom <= p.pmeta.top + 2, `${label} ${p.id}: .pchev (${p.pchev.bottom}) stays on row 1 above .pmeta (${p.pmeta.top})`);
      if (expectDashboardControls) {
        assert(p.dmax && p.dmax.width > 0, `${label} ${p.id}: .d-max visible in dashboard pane`);
        assert(p.dmax.left >= p.recap.right - 1 && p.dmax.right <= p.pchev.left + 1, `${label} ${p.id}: .d-max sits between title and chevron on row 1`);
      } else if (p.dmax) {
        assert.equal(p.dmax.width, 0, `${label} ${p.id}: .d-max hidden in standalone page`);
      }
    }
  }
}

(async () => {
  const browser = await chromium.launch();
  const errors = [];
  try {
    const scenarios = [
      { name: 'wide-light-1280', route: '/standalone', width: 1280, height: 1100, theme: 'light', dash: false },
      { name: 'wide-dark-1280', route: '/standalone', width: 1280, height: 1100, theme: 'dark', dash: false },
      { name: 'narrow-light-420', route: '/standalone', width: 420, height: 1100, theme: 'light', dash: false },
      { name: 'narrow-dark-420', route: '/standalone', width: 420, height: 1100, theme: 'dark', dash: false },
      { name: 'docked-dashboard-light-420', route: '/docked', width: 420, height: 1100, theme: 'light', dash: true },
      { name: 'docked-dashboard-dark-420', route: '/docked', width: 420, height: 1100, theme: 'dark', dash: true },
      { name: 'docked-dashboard-light-360', route: '/docked', width: 360, height: 1100, theme: 'light', dash: true },
      { name: 'docked-dashboard-dark-360', route: '/docked', width: 360, height: 1100, theme: 'dark', dash: true },
    ];
    for (const sc of scenarios) {
      const page = await browser.newPage({ viewport: { width: sc.width, height: sc.height }, colorScheme: sc.theme });
      page.on('pageerror', e => errors.push(`${sc.name}: ${e.message}`));
      await page.goto(baseURL + sc.route);
      await page.evaluate(t => document.documentElement.setAttribute('data-theme', t), sc.theme);
      await mountFramesAndFoldSecondPanel(page);
      await assertTwoRowPanels(page, sc.name, sc.dash);
      if (sc.dash) {
        await page.locator('#p1 > summary').hover();
        const actions = await page.locator('#p1 > summary').evaluate(sum => {
          const cite = sum.querySelector('.pfmention').getBoundingClientRect();
          const link = sum.querySelector('.pflink').getBoundingClientRect();
          const meta = sum.querySelector('.pmeta').getBoundingClientRect();
          return { citeTop: cite.top, linkTop: link.top, citeRight: cite.right, linkLeft: link.left, linkRight: link.right, metaRight: meta.right };
        });
        assert(Math.abs(actions.citeTop - actions.linkTop) <= 2, `${sc.name}: actions must share a row: ${JSON.stringify(actions)}`);
        assert(actions.linkLeft >= actions.citeRight - 1 && actions.metaRight - actions.linkRight <= 2, `${sc.name}: actions must remain together at the right edge: ${JSON.stringify(actions)}`);
      }

      // Verify click targets: .pfmention and .pflink do not toggle panel open/close;
      // clicking .entry-recap toggles collapsed panel p2 open and back closed.
      const p2 = page.locator('#p2');
      assert.equal(await p2.evaluate(el => el.open), false, `${sc.name}: p2 starts collapsed`);
      await page.locator('#p2 .pflink').click();
      assert.equal(await p2.evaluate(el => el.open), false, `${sc.name}: link button does not toggle p2`);
      assert.equal(await page.locator('.pfmenu').count(), 1, `${sc.name}: link menu opens`);
      await page.keyboard.press('Escape');
      await page.locator('#p2 .pfmention').click({ force: true });
      assert.equal(await p2.evaluate(el => el.open), false, `${sc.name}: mention button does not toggle p2`);
      await page.locator('#p2 .entry-recap').click();
      assert.equal(await p2.evaluate(el => el.open), true, `${sc.name}: clicking recap opens p2`);
      await page.locator('#p2 .entry-recap').click();
      assert.equal(await p2.evaluate(el => el.open), false, `${sc.name}: clicking recap closes p2`);

      if (shotsDir) {
        await page.mouse.move(0, 0);
        await page.locator('.wb-history').screenshot({ path: path.join(shotsDir, sc.name + '.png') });
        if (sc.width === 420 && sc.dash) {
          await page.locator('#p1').hover();
          await page.waitForTimeout(200);
          await page.locator('#p1 > summary').screenshot({ path: path.join(shotsDir, sc.name + '-hover.png') });
        }
      }
      await page.close();
    }
    assert.deepEqual(errors, [], 'browser page errors');
    console.log('panel-header-rows browser acceptance passed');
  } finally {
    await browser.close();
  }
})().catch(e => {
  console.error(e);
  process.exit(1);
});
