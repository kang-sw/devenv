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

// RenderRegions emits inert frame placeholders. This is the dashboard's
// mounting contract, including the height message that lets the host size a
// frame to its rendered body.
async function mountFrames(page) {
  await page.evaluate(() => {
    addEventListener('message', event => {
      if (!event.data || event.data.wbDoc !== 'height') return;
      const frame = [...document.querySelectorAll('iframe.doc-frame')].find(f => f.contentWindow === event.source);
      if (frame) frame.style.height = event.data.height + 'px';
    });
    function mount() {
      document.querySelectorAll('div.doc-frame[data-srcdoc]').forEach(p => {
        const frame = document.createElement('iframe');
        for (const attr of p.attributes) if (attr.name !== 'data-sandbox') frame.setAttribute(attr.name, attr.value);
        frame.setAttribute('sandbox', p.dataset.sandbox);
        frame.srcdoc = p.dataset.srcdoc;
        frame.style.height = '1px';
        p.replaceWith(frame);
      });
    }
    new MutationObserver(mount).observe(document, { childList: true, subtree: true });
    mount();
  });
}

function frame(page, selector = latest) {
  return page.frameLocator(selector + ' iframe.doc-frame');
}

async function waitMarks(page, selector = latest) {
  await frame(page, selector).locator('.pdg-hit').first().waitFor({ state: 'attached' });
  await page.waitForTimeout(150);
}

async function compare(page, selector, on) {
  const chip = page.locator(selector + ' .pdg-chip');
  if ((await chip.getAttribute('aria-pressed')) === String(on)) return;
  await chip.click();
  assert.equal(await chip.getAttribute('aria-pressed'), String(on));
}

async function loadTarget(page, id) {
  await page.evaluate(id => window.wbHistory.loadTarget(document, id), id);
  await page.waitForTimeout(100);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['light', 'dark']) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto(url);
      await mountFrames(page);
      await page.evaluate(value => document.documentElement.setAttribute('data-theme', value), theme);
      await waitMarks(page);
      const body = frame(page);
      assert.equal(await body.locator('.pdg-hit').count(), 3);
      assert.equal(await frame(page, '.historical').locator('.pdg-hit, .pdg-chip').count(), 0);
      assert.equal(await page.locator(panel + ' .pdg-chip').innerText(), 'v2 대비\n+1\n~1\n−1');
      assert.equal(await body.locator('.pdg-gone').isVisible(), false);
      await body.locator('.pdg-hit.mod').hover();
      assert.equal(await page.locator('.pdg-pop, .pdg-tip').count(), 0, 'sandboxed frames do not host overlays');

      await compare(page, panel, true);
      await body.locator('.pdg-diffview .pdg-gone').waitFor();
      assert.equal(await body.locator('del.pdg-w').innerText(), '1시간');
      assert.equal(await body.locator('.pdg-gone').isVisible(), true);
      await shot(page, theme + '-compare');
      await page.locator(panel + ' .pdg-off').click();
      await body.locator('.pdg-diffview').waitFor({ state: 'detached' });
      assert.equal(await page.locator(panel + ' .pdg-chip').getAttribute('aria-pressed'), 'false');

      // A repeated shared-script load remains idempotent, and a streamed
      // topic replacement mounts the new production frame before binding it.
      await page.evaluate(source => { window.wbBindBoard = function () {}; (0, eval)(source); }, fs.readFileSync(path.join(__dirname, '../assets/panel-diff.js'), 'utf8'));
      await page.evaluate(async () => {
        const html = await (await fetch('/live')).text();
        const incoming = new DOMParser().parseFromString(html, 'text/html').querySelector('.topic');
        document.querySelector('.topic').replaceWith(document.importNode(incoming, true));
        window.wbHistory.init(document);
        window.wbBindBoard(document);
      });
      await waitMarks(page, '#e8');
      const liveBody = frame(page, '#e8');
      assert.equal(await liveBody.locator('.pdg-hit').count(), 1, 'v4 compares only against v3');
      assert.equal(await page.locator('#e8 .pdg-chip').innerText(), 'v3 대비\n+0\n~1\n−0');
      assert.equal(await liveBody.locator('.pdg-pop, .pdg-tip').count(), 0);
      assert.deepEqual(errors, []);
      await page.close();
    }

    // The edge fixture exercises current framed behavior for deletion-only,
    // list, code, nested, paragraph, and raw-HTML blocks.
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const frameErrors = [];
    page.on('pageerror', e => frameErrors.push(e.message));
    await page.goto(url + '/edge');
    await mountFrames(page);
    await page.evaluate(() => {
      document.querySelectorAll('.entry-panel').forEach(p => { p.open = true; });
      window.wbBindBoard(document);
    });
    await loadTarget(page, 'e2');
    await waitMarks(page, '#e2');
    await compare(page, '#p1', true);
    assert.equal(await frame(page, '#e2').locator('.pdg-gone').innerText(), 'all removed', 'deletion-only panel has a body');
    await loadTarget(page, 'e4');
    await compare(page, '#p2', true);
    assert.equal(await frame(page, '#e4').locator('.doc-body > ul > li.pdg-gone').count(), 2, 'trailing removed items remain inside their list');
    assert.equal(await frame(page, '#e4').locator('.doc-body > li').count(), 0);
    await loadTarget(page, 'e6');
    await frame(page, '#e6').locator('.pdg-hit.mod').waitFor();
    assert.equal(await frame(page, '#e6').locator('.pdg-ins').innerText(), '2');
    await loadTarget(page, 'e8');
    await frame(page, '#e8').locator('.pdg-hit.mod').hover();
    assert.equal(await frame(page, '#e8').locator('.pdg-pop, .pdg-tip').count(), 0);
    await compare(page, '#p4', true);
    assert.equal(await frame(page, '#e8').locator('.doc-body ul ul li').count(), 1);
    assert.equal(await frame(page, '#e8').locator('.pdg-ins').innerText(), 'changed');
    await loadTarget(page, 'e10');
    await compare(page, '#p5', true);
    assert.equal(await frame(page, '#e10').locator('.pdg-gone').innerText(), 'second');
    await loadTarget(page, 'e12');
    await frame(page, '#e12').locator('.pdg-hit.mod').waitFor();
    assert.equal(await frame(page, '#e12').locator('.pdg-ins').innerText(), 'new');
    assert.deepEqual(frameErrors, []);

    // The dedicated framed route remains sandboxed and uses the same host
    // chip to enter comparison mode in its opaque-origin document.
    await page.goto(url + '/framed');
    await mountFrames(page);
    await waitMarks(page);
    assert.equal(await page.locator(latest + ' iframe').getAttribute('sandbox'), 'allow-scripts allow-popups allow-popups-to-escape-sandbox');
    await compare(page, panel, true);
    await frame(page).locator('.pdg-diffview .pdg-gone').waitFor();
    await page.locator(panel + ' .pdg-off').click();
    await frame(page).locator('.pdg-diffview').waitFor({ state: 'detached' });
    console.log('PASS: framed marks, no cross-origin overlays, compare mode, live replacement, and edge cases');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
