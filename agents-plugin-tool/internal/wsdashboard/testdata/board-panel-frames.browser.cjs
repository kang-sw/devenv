// Run by TestBoardPanelFrameReuseBrowser against the complete dashboard IIFE.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const tab = await browser.newPage({ viewport: { width: 1600, height: 1800 }, locale: 'en-US' });
    const errors = [], failures = [];
    tab.on('pageerror', e => errors.push(e.message));
    // Enter through the shell, then operate on its active session frame.
    await tab.goto(base + '/s/livesession');
    const active = tab.locator('iframe.sframe.on');
    await active.waitFor({ state: 'visible' });
    const page = await (await active.elementHandle()).contentFrame();
    assert(page, 'active session frame exists');
    await page.locator('#ctext').waitFor({ state: 'attached' });
    assert.equal(await page.evaluate(() => typeof Element.prototype.moveBefore), 'function', 'Chromium must support state-preserving moves');
    const panels = [
      { topic: 'BodyAndPanel', id: 'p1' },
      { topic: 'PanelOnly', id: 'p2' }
    ];
    function topic(p) { return page.locator(`section.topic[data-topic="${p.topic}"]`); }
    function frame(p) { return topic(p).locator(`details.entry-panel[id="${p.id}"] > .entry-body > iframe.doc-frame:not(.doc-frame-next)`); }
    async function sample(p) {
      const el = frame(p);
      await el.waitFor({ state: 'visible' });
      // Every tested topic stays inside the view; viewport eviction is not
      // script reuse, and must not explain either a pass or a failure.
      assert.equal(await topic(p).evaluate(t => {
        const r = t.getBoundingClientRect();
        return r.top >= 0 && r.bottom <= innerHeight;
      }), true, `${p.topic} remains in view`);
      assert.equal(await el.getAttribute('sandbox'), 'allow-scripts allow-popups allow-popups-to-escape-sandbox');
      const f = await (await el.elementHandle()).contentFrame();
      await f.waitForFunction(() => window.panelRun && window.panelRun.ticks >= 5);
      return f.evaluate(() => ({ ...window.panelRun }));
    }
    async function update(state, marker) {
      const response = await fetch(updates + '?state=' + state);
      assert.equal(response.status, 204);
      // Viewed protection may stage a change across these exposed panels.
      // Apply it through the real control, without moving either topic away.
      await page.waitForFunction(selector => document.querySelector(selector) ||
        [...document.querySelectorAll('.wb-oq-top .oq-updates')].some(b => !b.hidden), marker);
      const pending = page.locator('.wb-oq-top .oq-updates:visible');
      if (await pending.count()) await pending.click();
      await page.locator(marker).waitFor({ state: 'attached' });
      // Sample only after the morph, body double buffer and mount pass settle.
      await page.waitForFunction(() => !document.querySelector('.doc-frame-next'));
    }
    function record(p, change, before, after, restart = false) {
      const ok = restart ? after.load !== before.load && after.version === 'replacement'
        : after.load === before.load && after.ticks > before.ticks;
      console.log(JSON.stringify({ topic: p.topic, change, before, after, result: ok ? 'pass' : 'FAIL' }));
      if (!ok) failures.push(`${p.topic}: ${change}`);
    }
    for (const p of panels) {
      // Panels are initially open in the production renderer. Explicitly open
      // the whole-card fold if persisted defaults ever make it closed.
      await topic(p).evaluate(t => { t.querySelectorAll('details:not(.panel-versions)').forEach(d => { d.open = true; }); });
    }
    assert.equal(await topic(panels[0]).locator('.tdoc .doc-frame').count(), 1, 'first topic has a body frame');
    assert.equal(await topic(panels[1]).locator('.tdoc .doc-frame').count(), 0, 'second topic has panels only');
    async function bodyLoad() {
      const body = await (await topic(panels[0]).locator('.tdoc iframe.doc-frame').elementHandle()).contentFrame();
      await body.waitForFunction(() => document.readyState === 'complete');
      return body.evaluate(() => window.bodyLoad || (window.bodyLoad = Math.random().toString(36)));
    }
    const originalBodyLoad = await bodyLoad();
    let previous = await Promise.all(panels.map(sample));
    for (const [state, marker, owner] of [
      ['log-body', '#e10', 0], ['log-only', '#e11', 1]
    ]) {
      await update(state, marker);
      assert.equal(await bodyLoad(), originalBodyLoad, 'unchanged body retains its live document after ' + state);
      const current = await Promise.all(panels.map(sample));
      panels.forEach((p, i) => record(p, i === owner ? 'own-topic log' : 'other-topic log', previous[i], current[i]));
      previous = current;
    }
    for (const [state, marker, owner] of [
      ['overwrite-body', '#e20', 0], ['overwrite-only', '#e21', 1]
    ]) {
      await update(state, marker);
      assert.equal(await bodyLoad(), originalBodyLoad, 'unchanged body retains its live document after ' + state);
      const current = await Promise.all(panels.map(sample));
      panels.forEach((p, i) => record(p, i === owner ? 'own-panel overwrite' : 'other-panel overwrite', previous[i], current[i], i === owner));
      previous = current;
    }
    assert.deepEqual(errors, [], 'no browser script errors');
    assert.deepEqual(failures, [], 'unchanged panels retain live scripts; overwritten panels restart');
    console.log('PASS: both topic shapes keep counting across own/other-topic updates and restart on overwrite');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
