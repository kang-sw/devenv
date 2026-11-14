// Opt-in, standalone acceptance; no installed daemon or application server.
// WS_PLAYWRIGHT_MODULE=/absolute/path/to/playwright node internal/wswhiteboard/testdata/latest-log.browser.cjs
// WS_RECAP_CAPTURE_DIR selects the screenshot/measurement directory.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE || 'playwright');
const assets = path.join(__dirname, '../assets');
const css = fs.readFileSync(path.join(assets, 'page.css'), 'utf8');
const stream = fs.readFileSync(path.join(assets, 'stream.js'), 'utf8');
const arrival = stream.slice(stream.indexOf('/* Latest agent arrival:'));
const captures = path.resolve(process.env.WS_RECAP_CAPTURE_DIR || '.artifacts/recap-max4');
function recap(id) {
  return `<div class="tophead"><div class="tbar">Topic</div><div class="tsub"><div class="trecap" data-kind="agent" data-entry="${id}"><time datetime="${new Date().toISOString()}">now</time> — <div class="rtext">Log ${id}</div></div></div></div>`;
}
function log(id) {
  return `<article class="wb-entry agent current" data-kind="agent" data-entry="${id}"><div class="entry-line"><time datetime="${new Date().toISOString()}">now</time> — Log ${id}</div></article>`;
}
function content(id) { return recap(id) + log(id); }
function board() {
  return `<style>${css}</style><section class="topic" data-topic="top">${content('e1')}</section><div style="height:2200px"></div><section class="topic" data-topic="bottom">${content('e2')}</section>`;
}
async function recapGeometry(browser) {
  fs.mkdirSync(captures, { recursive: true });
  const results = [];
  for (const width of [900, 1900]) for (const theme of ['light', 'dark']) {
    const page = await browser.newPage({ viewport: { width, height: 900 } });
    try {
      await page.setContent(`<style>${css}</style><div class="fixture wb-rail" style="height:500px;overflow:auto;width:380px;margin:30px"><section class="topic" data-topic="geometry">${recap('e1')}<div style="height:700px">${log('e1')}</div></section></div>`);
      await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
      for (const lines of [1, 6]) {
        await page.locator('.rtext').evaluate((node, lines) => {
          node.innerHTML = Array.from({ length: lines }, (_, i) => 'Recap line ' + (i + 1)).join('<br>');
        }, lines);
        const measures = [];
        for (const stuck of [false, true]) {
          await page.evaluate(stuck => {
            document.querySelector('.fixture').scrollTop = stuck ? 120 : 0;
            document.querySelector('.tophead').classList.toggle('wb-stuck', stuck);
          }, stuck);
          await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
          const m = await page.evaluate(() => {
            const head = document.querySelector('.tophead'), recap = document.querySelector('.trecap');
            const s = getComputedStyle(recap), entry = document.querySelector('.wb-entry');
            return { header: head.getBoundingClientRect().height, recap: recap.getBoundingClientRect().height,
              line: parseFloat(s.lineHeight), clamp: s.webkitLineClamp, headerFill: s.backgroundColor,
              logFill: getComputedStyle(entry).backgroundColor,
              stuck: head.getBoundingClientRect().top > head.parentElement.getBoundingClientRect().top + 1 };
          });
          assert.equal(m.stuck, stuck, 'exercises the actual sticky transition');
          assert.equal(m.clamp, '4', 'ellipsis clamp is independent of sticking');
          assert.ok(Math.abs(m.recap - Math.min(lines, 4) * m.line) <= 1, 'one line shrinks; six lines clamp to four');
          assert.equal(m.headerFill, 'rgba(0, 0, 0, 0)', 'header recap has no highlight');
          assert.equal(m.logFill, 'rgba(110, 200, 130, 0.08)', 'lower latest log uses subtle tint');
          measures.push(m);
          await page.screenshot({ path: path.join(captures, `${width}-${theme}-${lines}-lines-${stuck ? 'stuck' : 'unstuck'}.png`) });
        }
        const delta = Math.abs(measures[1].header - measures[0].header);
        assert.ok(delta <= 1, 'same content has equal stuck/unstuck height');
        results.push({ width, theme, lines, delta, measures });
      }
    } finally { await page.close(); }
  }
  fs.writeFileSync(path.join(captures, 'measurements.json'), JSON.stringify(results, null, 2));
  console.log('recap max-four geometry: ' + JSON.stringify(results.map(({ width, theme, lines, delta, measures }) => ({ width, theme, lines, delta, header: measures[0].header, recap: measures[0].recap }))));
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 900, height: 600 } });
    const row = id => page.locator(`.wb-entry[data-entry="${id}"]`);
    async function load() {
      await page.goto('about:blank');
      await page.setContent(board());
      await page.addScriptTag({ content: arrival });
    }
    async function replace(topic, html) {
      await page.locator(`[data-topic="${topic}"]`).evaluate((node, text) => { node.innerHTML = text; }, html);
    }
    async function animation(id) { return row(id).evaluate(node => getComputedStyle(node).animationName); }
    async function waitBlink(id) {
      await row(id).scrollIntoViewIfNeeded();
      await page.waitForFunction(id => [...document.querySelectorAll(`.wb-entry[data-entry="${id}"]`)].some(node => getComputedStyle(node).animationName === 'wb-log-arrival'), id);
      const headerAnimation = await page.locator(`.trecap[data-entry="${id}"]`).evaluate(node => getComputedStyle(node).animationName);
      assert.equal(headerAnimation, 'none', 'header recap never blinks');
    }
    await load();
    assert.equal(await animation('e1'), 'none', 'initial load never blinks');
    await replace('bottom', content('e3'));
    await page.waitForTimeout(150);
    assert.equal(await animation('e3'), 'none', 'off-screen arrival waits');
    await row('e3').scrollIntoViewIfNeeded();
    await waitBlink('e3');
    const timing = await row('e3').evaluate(node => {
      const s = getComputedStyle(node); return [s.animationDuration, s.animationIterationCount];
    });
    assert.deepEqual(timing, ['1s', '3']);
    await page.waitForTimeout(3200);
    assert.equal(await animation('e3'), 'none', 'animation ends at about three seconds');
    const fill = await row('e3').evaluate(node => getComputedStyle(node).backgroundColor);
    await replace('bottom', `<article class="wb-entry agent" data-entry="e3">Previous latest</article>${content('e4')}`);
    await waitBlink('e4');
    assert.notEqual(await row('e3').evaluate(node => getComputedStyle(node).backgroundColor), fill, 'previous latest loses its fill');
    await page.waitForTimeout(3200);
    assert.equal(await row('e4').evaluate(node => getComputedStyle(node).backgroundColor), fill, 'new latest keeps tint');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await replace('bottom', content('e5'));
    await page.waitForTimeout(150);
    assert.equal(await animation('e5'), 'none', 'reduced motion has no blink');
    assert.equal(await row('e5').evaluate(node => getComputedStyle(node).backgroundColor), fill);
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    await replace('bottom', `${recap('e6')}<details><summary>Closed history</summary>${log('e6')}</details>`);
    await page.waitForTimeout(150);
    assert.equal(await animation('e6'), 'none', 'visible header does not consume folded log arrival');
    await page.locator('[data-topic="bottom"] summary').click();
    await waitBlink('e6');
    await page.waitForTimeout(3200);
    await load(); // Fresh page-script lifetime, as with reload; no persistent blink state.
    assert.equal(await animation('e1'), 'none', 'reload has no blink');
    await page.close();
    await recapGeometry(browser);
    console.log('latest log and max-four recap browser acceptance passed; screenshots: ' + captures);
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
