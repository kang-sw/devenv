const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const url = process.argv[2], shots = process.argv[3];
const latest = '#e6', panel = latest + ' .entry-panel';
async function shot(page, name) {
  if (!shots) return;
  fs.mkdirSync(shots, { recursive: true });
  await page.screenshot({ path: path.join(shots, name + '.png'), fullPage: true });
}
async function waitMarks(page, selector = latest) {
  await page.locator(selector + ' .pdg-hit').first().waitFor();
  await page.waitForTimeout(150);
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['light', 'dark']) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(url);
      await page.evaluate(theme => document.documentElement.setAttribute('data-theme', theme), theme);
      await waitMarks(page);
      assert.equal(await page.locator('.pdg-hit').count(), 3);
      assert.equal(await page.locator('.historical .pdg-hit, .historical .pdg-chip').count(), 0);
      assert.equal(await page.locator(panel + ' .pdg-chip').innerText(), 'v2 대비\n+1\n~1\n−1');
      assert.equal(await page.locator(panel + ' .pdg-gone').isVisible(), false);
      const guide = await page.evaluate(sel => {
        const b = document.querySelector(sel + ' .entry-body').getBoundingClientRect();
        const bar = document.querySelector(sel + ' .pdg-hit.mod .pdg-bar').getBoundingClientRect();
        return { delta: Math.abs((bar.left + bar.width / 2) - b.left - .5), width: bar.width };
      }, latest);
      assert(guide.delta < 1 && guide.width === 3, JSON.stringify(guide));
      await shot(page, theme + '-rest');

      const modified = page.locator(latest + ' .pdg-hit.mod');
      await modified.hover();
      await page.locator('.pdg-pop.mod').waitFor();
      assert.equal(await page.locator('.pdg-pop .k').innerText(), '수정됨');
      assert.equal(await page.locator('.pdg-pop .v').innerText(), 'v2 09:55 → v3 10:20');
      assert.equal(await page.locator('.pdg-pop del').innerText(), '1시간');
      assert((await page.locator(panel + ' .pdg-ins').allTextContents()).join('').includes('30분'));
      await page.waitForFunction(sel => getComputedStyle(document.querySelector(sel + ' .pdg-hit.mod .pdg-bar')).width === '5px', latest);
      await shot(page, theme + '-modified-hover');
      // Pointer can cross from the gutter to the overlay without closing it.
      await page.locator('.pdg-pop .pin').hover();
      assert.equal(await page.locator('.pdg-pop').count(), 1);
      await page.locator('.pdg-pop .pin').click();
      assert.equal(await page.locator('.pdg-pop.pinned').count(), 1);
      await page.keyboard.press('Escape');
      assert.equal(await page.locator('.pdg-pop').count(), 0);
      assert.equal(await modified.evaluate(n => n === document.activeElement), true, 'Esc restores mark focus');

      const removed = page.locator(latest + ' .pdg-hit.del');
      await removed.click();
      assert.equal(await page.locator('.pdg-pop.del.pinned').count(), 1);
      assert((await page.locator('.pdg-pop .pdg-pb').innerText()).includes('이름순'));
      await page.mouse.move(1200, 800);
      await page.waitForTimeout(200);
      assert.equal(await page.locator('.pdg-pop.pinned').count(), 1);
      await shot(page, theme + '-removed-pinned');
      await page.locator('.pdg-pop .close').click();
      assert.equal(await page.locator('.pdg-pop').count(), 0);

      // Focus opens the dialog; Tab reaches its pin/close/link controls.
      await modified.focus();
      await page.keyboard.press('Tab');
      assert.equal(await page.locator('.pdg-pop .pin').evaluate(n => n === document.activeElement), true);
      await page.keyboard.press('Escape');
      const added = page.locator(latest + ' .pdg-hit.new');
      await added.hover();
      assert.equal(await page.locator('.pdg-tip').innerText(), '추가됨 · v3 10:20');
      await page.mouse.move(1200, 800);
      await page.waitForTimeout(200);
      assert.equal(await page.locator('.pdg-tip').count(), 0);

      const chip = page.locator(panel + ' .pdg-chip');
      await chip.click();
      assert.equal(await chip.getAttribute('aria-pressed'), 'true');
      assert.equal(await page.locator(panel).getAttribute('open'), '');
      assert.equal(await page.locator(panel + ' .pdg-note').isVisible(), true);
      assert.equal(await page.locator(panel + ' .pdg-gone').isVisible(), true);
      assert.equal(await page.locator(panel + ' del.pdg-w').innerText(), '1시간');
      await page.mouse.move(1200, 800);
      await shot(page, theme + '-compare');
      await page.locator(panel + ' .pdg-off').click();
      assert.equal(await chip.getAttribute('aria-pressed'), 'false');
      assert.equal(await page.locator(panel + ' .pdg-gone').isVisible(), false);
      // The full-version link admits the old row, opens it, and targets eN.
      await modified.click();
      await page.locator('.pdg-pop .pdg-pf a').click();
      await page.waitForTimeout(100);
      assert.equal(await page.locator('#e4 .entry-panel').getAttribute('open'), '');
      assert.equal(await page.locator('#e4 .pdg-hit').count(), 0);

      // A repeated shared-script load replaces the form binder before the
      // diff singleton reattaches, just as a dashboard board refresh can.
      await page.evaluate(source => { window.wbBindBoard = function () {}; (0, eval)(source); }, fs.readFileSync(path.join(__dirname, '../assets/panel-diff.js'), 'utf8'));
      // A streaming topic swap uses the actual new server-rendered section.
      await modified.click();
      await page.evaluate(async () => {
        const html = await (await fetch('/live')).text();
        const incoming = new DOMParser().parseFromString(html, 'text/html').querySelector('.topic');
        document.querySelector('.topic').replaceWith(document.importNode(incoming, true));
        window.wbHistory.init(document);
        window.wbBindBoard(document);
      });
      await waitMarks(page, '#e8');
      assert.equal(await page.locator('.pdg-pop').count(), 0, 'stale pinned overlay removed on swap');
      assert.equal(await page.locator('.pdg-hit').count(), 1, 'v4 compares only against v3');
      assert.equal(await page.locator('#e8 .pdg-chip').innerText(), 'v3 대비\n+0\n~1\n−0');
      assert.equal(await page.locator('#e6 .pdg-hit').count(), 0);
      await page.locator('#e8 .pdg-hit.mod').hover();
      assert.equal(await page.locator('.pdg-pop del').innerText(), '30분');
      assert.deepEqual(errors, []);
      await page.close();
    }
    // Sandboxed frames keep the existing isolation, draw marks without
    // popovers, and the host chip navigates the same frame into compare mode.
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const frameErrors = [];
    page.on('pageerror', e => frameErrors.push(e.message));
    await page.goto(url + '/framed');
    await page.evaluate(() => {
      addEventListener('message', e => {
        if (!e.data || e.data.wbDoc !== 'height') return;
        const f = [...document.querySelectorAll('iframe.doc-frame')].find(f => f.contentWindow === e.source);
        if (f) f.style.height = e.data.height + 'px';
      });
      document.querySelectorAll('.entry-panel[open] div.doc-frame').forEach(p => {
        const f = document.createElement('iframe');
        for (const a of p.attributes) if (a.name !== 'data-sandbox') f.setAttribute(a.name, a.value);
        f.setAttribute('sandbox', p.dataset.sandbox);
        f.srcdoc = p.dataset.srcdoc;
        p.replaceWith(f);
      });
    });
    const frame = page.frameLocator(latest + ' iframe');
    await frame.locator('.pdg-hit.mod').waitFor();
    assert.equal(await frame.locator('.pdg-hit').count(), 3);
    assert.equal(await page.locator(latest + ' iframe').getAttribute('sandbox'), 'allow-scripts allow-popups allow-popups-to-escape-sandbox');
    await frame.locator('.pdg-hit.mod').hover();
    assert.equal(await frame.locator('.pdg-pop').count(), 0);
    await page.locator(panel + ' .pdg-chip').click();
    await frame.locator('.pdg-diffview .pdg-gone').waitFor();
    assert.equal(await frame.locator('del.pdg-w').innerText(), '1시간');
    await page.locator(panel + ' .pdg-off').click();
    await frame.locator('.pdg-diffview').waitFor({ state: 'detached' });
    assert.deepEqual(frameErrors, []);
    await page.goto(url + '/edge');
    await page.evaluate(() => {
      document.querySelectorAll('.entry-panel').forEach(p => { p.open = true; });
      window.wbBindBoard(document);
    });
    await waitMarks(page, '#e2');
    await page.locator('#p1 .pdg-chip').click();
    assert.equal(await page.locator('#p1 .pdg-gone').innerText(), 'all removed', 'deletion-only panel has a body');
    await page.locator('#p2 .pdg-chip').click();
    assert.equal(await page.locator('#p2 .entry-body > ul > li.pdg-gone').count(), 2, 'trailing removed items remain inside their list');
    assert.equal(await page.locator('#p2 .entry-body > li').count(), 0);
    await page.locator('#e6 .pdg-hit.mod').hover();
    assert.equal(await page.locator('.pdg-pop del').innerText(), '1', 'code toolbar/line numbers are not diff text');
    assert.equal(await page.locator('#p3 .pdg-ins').innerText(), '2');
    await page.keyboard.press('Escape');
    await page.locator('#e8 .pdg-hit.mod').hover();
    assert(!(await page.locator('.pdg-pop .pdg-pb').innerText()).includes('child'), 'nested units are not duplicated in the parent');
    await page.locator('#p4 .pdg-chip').click();
    assert.equal(await page.locator('#p4 .entry-body ul ul li').count(), 1);
    assert.equal(await page.locator('#p4 .pdg-ins').innerText(), 'changed');
    assert.equal(await page.locator('#p5 .entry-body > p:not(.pdg-gone)').evaluate(n => getComputedStyle(n).marginBottom), '0px', 'hidden trailing deletion leaves current layout alone');
    await page.locator('#p5 .pdg-chip').click();
    assert.equal(await page.locator('#p5 .pdg-gone').innerText(), 'second');
    await page.locator('#e12 .pdg-hit.mod').hover();
    assert.equal(await page.locator('.pdg-pop del').innerText(), 'old', 'raw HTML lists remain part of an HTML block');
    assert.equal(await page.locator('#p6 .pdg-ins').innerText(), 'new');
    console.log('PASS: light/dark marks, hover, pin, keyboard, compare, prior link, live version, sandboxed frame, empty/list/code/nested/paragraph edge cases');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
