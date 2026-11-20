'use strict';
// Runs only against the temporary authenticated daemon owned by the Go fixture.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {chromium} = require(process.env.WS_PLAYWRIGHT_MODULE || 'playwright');
const [url, out] = process.argv.slice(2);
(async () => {
  const browser = await chromium.launch({headless: true});
  try {
    const context = await browser.newContext({viewport: {width: 1600, height: 1100}});
    await context.addCookies([JSON.parse(process.env.WS_WHITEBOARD_TEST_COOKIE)]);
    const tab = await context.newPage();
    const requests = [], errors = [];
    tab.on('request', r => requests.push(new URL(r.url()).pathname));
    tab.on('pageerror', e => errors.push(e.message));
    await tab.goto(url, {waitUntil: 'domcontentloaded'});
    await tab.locator('iframe.sframe.on').waitFor();
    const frameElement = await tab.locator('iframe.sframe.on').elementHandle();
    const page = await frameElement.contentFrame();
    await page.locator('.board-toolbar').waitFor();
    if (!await page.locator('#bcol').isVisible()) await page.locator('[data-tab="board"]:visible').first().click();
    await page.locator('section.topic[data-topic="Browser gate"]').waitFor();
    const choice = page.locator('#item-1 .opt[data-opt="a"]');
    await choice.click();
    assert.equal(await choice.getAttribute('aria-pressed'), 'true', 'answer selection remains interactive');
    const file = page.locator('#item-1 a[data-wl="file"]').first();
    assert.equal(await file.getAttribute('data-rel'), 'docs/plan.md');
    await file.click({button: 'right'});
    await page.locator('.lmenu').waitFor();
    await tab.keyboard.press('Escape');
    // Demand the old target through the actual daemon route and browser history
    // controller, then open its sandboxed panel.
    await page.evaluate(() => { location.hash = 'p1'; });
    await page.locator('#p1').waitFor({state: 'attached'});
    await page.locator('#p1 > summary').click();
    await page.waitForFunction(() => document.querySelector('#p1 iframe'));
    const frames = await page.locator('iframe[data-doc]').count();
    assert(frames > 0, 'topic/panel bodies still use sandboxed frames');
    assert(await page.locator('#p1 iframe').getAttribute('sandbox'));
    assert(requests.some(p => /\/topic$/.test(p)), 'lazy topic history reaches daemon');
    // Admit the image and Mermaid panel and verify their rendered frame output.
    const html = await page.content();
    assert(html.includes('shots') || html.includes('Image card'));
    assert(html.includes('SSE-panel-marker'));
    await tab.screenshot({path: path.join(out, 'served-board.png'), fullPage: true});
    await page.evaluate(() => { location.hash = 'p6'; });
    const picture = page.locator('#p6');
    await picture.waitFor({state: 'attached'});
    if (await picture.getAttribute('open') === null) await picture.locator(':scope > summary').click();
    await picture.locator('iframe').waitFor({state: 'attached'});
    const imageFrame = await (await picture.locator('iframe').elementHandle()).contentFrame();
    await imageFrame.waitForFunction(() => Array.from(document.images).some(i => i.naturalWidth === 1200));
    await tab.screenshot({path: path.join(out, 'served-image.png'), fullPage: true});
    const diagram = page.locator('details.entry-panel').filter({hasText: 'SSE-panel-marker'}).first();
    await diagram.scrollIntoViewIfNeeded();
    if (await diagram.getAttribute('open') === null) await diagram.locator(':scope > summary').click();
    await diagram.locator('iframe').waitFor({state: 'attached'});
    const diagramFrame = await (await diagram.locator('iframe').elementHandle()).contentFrame();
    await diagramFrame.locator('.mermaid svg').waitFor();
    await tab.screenshot({path: path.join(out, 'served-mermaid.png'), fullPage: true});
    await page.locator('#drhandle').click();
    await page.waitForFunction(() => {
      const clip = document.getElementById('drclip');
      return clip.getBoundingClientRect().height >= parseFloat(clip.style.height) - 1;
    });
    await page.locator('#p-todo').waitFor({state: 'visible'});
    assert((await page.locator('#p-todo').textContent()).includes('Auxiliary sections stay below the full-width board'));
    await tab.screenshot({path: path.join(out, 'served-panels.png'), fullPage: true, animations: 'disabled'});

    assert(!requests.some(p => /\/(index\.html|index\.stamp\.js)$/.test(p)), 'no retired file requests');
    assert.deepEqual(errors, [], 'page errors');
    fs.writeFileSync(path.join(out, 'browser.json'), JSON.stringify({requests, frames, errors, answer: true, fileActions: true, lazyHistory: true, image: true, mermaid: true, panels: true}, null, 2));
    console.log('PASS served board: answers, file actions, lazy history, sandboxed frames, images and Mermaid; no disk-page requests');
  } finally { await browser.close(); }
})().catch(err => { console.error(err); process.exitCode = 1; });
