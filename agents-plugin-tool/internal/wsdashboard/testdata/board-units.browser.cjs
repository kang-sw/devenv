// Run through TestBoardTopicUnitsBrowser, against disposable fixture sessions.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'en-US' });
    const tab = await context.newPage(), events = [], errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    tab.on('websocket', ws => ws.on('framereceived', f => {
      try { events.push(JSON.parse(f.payload)); } catch {}
    }));
    const url = base + '/s/boardsession';
    async function sessionFrame(tab) {
      const host = tab.locator('iframe.sframe.on');
      await host.waitFor({ state: 'visible' });
      return (await host.elementHandle()).contentFrame();
    }
    await tab.goto(url);
    const page = await sessionFrame(tab);
    await page.locator('section.topic').nth(111).waitFor({ state: 'attached' });
    await page.waitForFunction(() => !!document.querySelector('section.topic[data-topic="topic-001"]'));
    await page.evaluate(() => { window.unchangedTopic = document.querySelector('section.topic[data-topic="topic-001"]'); });
    // Wait until the connection's first full board established its baseline.
    await new Promise((resolve, reject) => {
      const limit = Date.now() + 15000;
      const check = () => events.some(e => e.event === 'region' && e.data.name === 'board') ? resolve() : Date.now() > limit ? reject(new Error('no first full board')) : setTimeout(check, 20);
      check();
    });
    fs.mkdirSync('/tmp/exec5/btp1', { recursive: true });
    await tab.screenshot({ path: '/tmp/exec5/btp1/browser-before.png' });
    await fetch(updates);
    // The existing reader-anchor gate deliberately holds an exposed topic.
    // Release it through its UI instead of weakening the product's gate.
    const pending = page.locator('.board-viewed-pending');
    await pending.waitFor({ state: 'visible' });
    assert((await page.locator('section.topic[data-topic="topic-000"] .doc-frame').first().getAttribute('data-srcdoc')).includes('Original body 0'), 'visible body is held until release');
    await pending.click();
    await page.waitForFunction(() => {
      const doc = document.querySelector('section.topic[data-topic="topic-000"] .doc-frame');
      return (doc?.getAttribute('data-srcdoc') || doc?.getAttribute('srcdoc'))?.includes('Edited body marker');
    });
    assert.equal(await page.evaluate(() => window.unchangedTopic === document.querySelector('section.topic[data-topic="topic-001"]')), true, 'unchanged topic keeps its DOM node');
    assert(events.some(e => e.event === 'board-topics' && e.data.topics.length === 1), 'one-topic edit arrives as one unit');
    const signature = page => page.locator('section.topic').evaluateAll(nodes => nodes.map(n => ({
      key: n.dataset.topic, hash: n.dataset.th,
      text: [...n.querySelectorAll('.entry-text')].map(e => e.textContent),
      docs: [...n.querySelectorAll('.doc-frame')].map(e => e.getAttribute('data-srcdoc') || e.getAttribute('srcdoc'))
    })));
    const fresh = await context.newPage();
    await fresh.goto(url);
    const freshPage = await sessionFrame(fresh);
    await freshPage.locator('section.topic').nth(111).waitFor({ state: 'attached' });
    assert.deepEqual(await signature(page), await signature(freshPage), 'streamed board matches a fresh full render');
    await tab.screenshot({ path: '/tmp/exec5/btp1/browser-after.png' });
    await fresh.screenshot({ path: '/tmp/exec5/btp1/browser-fresh.png' });
    assert.deepEqual(errors, [], 'no page errors');
    console.log('112 topics: one-topic delta, unchanged node retained, streamed/fresh board parity, screenshots saved');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exit(1); });
