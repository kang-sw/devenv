'use strict';
const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);

const baseURL = process.argv[2];
const outDir = process.argv[3];

async function settle(page, ms = 120) {
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  if (ms > 0) await page.waitForTimeout(ms);
}

async function runTheme(browser, theme) {
  const context = await browser.newContext({ viewport: { width: 1320, height: 760 } });
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', err => { pageErrors.push(err); console.error(`[${theme} pageerror]`, err); });
  page.on('console', msg => { if (msg.type() === 'error') console.error(`[${theme} console.error]`, msg.text()); });
  const url = `${baseURL}/s/uxdemo?theme=${theme}`;
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.evaluate(() => {
    localStorage.clear();
    localStorage.setItem('ws.dashboard.layout', 'split');
  });
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  const debug = await page.evaluate(() => ({
    mode: document.documentElement.getAttribute('data-mode'),
    hasConvo: !!document.getElementById('convo'),
    hasBcol: !!document.getElementById('bcol'),
    userMsgs: document.querySelectorAll('#convo .msg.user').length,
    jumpBtn: !!document.querySelector('#convo .d-jump-btn')
  }));
  console.log(`[${theme} debug]`, JSON.stringify(debug));
  await page.waitForSelector('#convo .d-jump-btn', { state: 'attached', timeout: 5000 });
  await page.waitForSelector('#bcol .wb-hold-toggle', { state: 'visible' });
  // Wait for sandboxed panel iframe to mount and size.
  await page.waitForFunction(() => {
    const f = document.querySelector('#bcol details.entry-panel iframe.doc-frame');
    return !!(f && parseFloat(f.style.height) > 20);
  }, { timeout: 10000 });
  await settle(page, 180);

  // 1) Chat jump arrow: scroll #convo to bottom so latest user message is above viewport.
  await page.evaluate(() => {
    const c = document.getElementById('convo');
    c.scrollTop = c.scrollHeight;
  });
  await settle(page, 180);

  const jumpBefore = await page.evaluate(() => {
    const c = document.getElementById('convo');
    const cr = c.getBoundingClientRect();
    const btn = c.querySelector('.d-jump-btn');
    const br = btn.getBoundingClientRect();
    return {
      hidden: btn.hidden,
      dir: btn.getAttribute('data-dir'),
      leftOffset: br.left - cr.left,
      topOffset: br.top - cr.top,
      scrollTop: c.scrollTop
    };
  });
  assert.equal(jumpBefore.hidden, false, `${theme}: jump arrow visible when latest user msg is above`);
  assert.equal(jumpBefore.dir, 'up', `${theme}: jump arrow points up`);
  assert.ok(jumpBefore.leftOffset >= 0 && jumpBefore.leftOffset < 40, `${theme}: jump arrow sits at top-left (${jumpBefore.leftOffset})`);
  assert.ok(jumpBefore.topOffset >= 0 && jumpBefore.topOffset < 40, `${theme}: jump arrow sits at top-left (${jumpBefore.topOffset})`);

  await page.screenshot({ path: path.join(outDir, `chat-jump-before-${theme}.png`) });

  // Click jump arrow -> scrolls latest user message into view and hides arrow.
  await page.click('#convo .d-jump-btn');
  await page.waitForFunction(() => {
    const btn = document.querySelector('#convo .d-jump-btn');
    return btn && btn.hidden;
  }, { timeout: 5000 });
  await settle(page, 180);

  const jumpAfter = await page.evaluate(() => {
    const c = document.getElementById('convo');
    const cr = c.getBoundingClientRect();
    const msgs = Array.from(c.querySelectorAll('.msg.user'));
    const latest = msgs[msgs.length - 1];
    const lr = latest.getBoundingClientRect();
    return {
      hidden: c.querySelector('.d-jump-btn').hidden,
      latestVisible: lr.bottom > cr.top && lr.top < cr.bottom,
      scrollTop: c.scrollTop
    };
  });
  assert.equal(jumpAfter.hidden, true, `${theme}: jump arrow hides once latest user msg is visible`);
  assert.equal(jumpAfter.latestVisible, true, `${theme}: latest user msg is in viewport after click`);
  await page.screenshot({ path: path.join(outDir, `chat-jump-after-${theme}.png`) });

  // Verify scrolling to very top when latest user msg is below flips arrow down (if scrollable past it).
  await page.evaluate(() => {
    const c = document.getElementById('convo');
    c.scrollTop = 0;
  });
  await settle(page, 100);

  // 2) In-card Ctrl/Cmd+F find bar:
  // First confirm Ctrl/Cmd+F outside a card (in #convo) does NOT open .wb-find-bar.
  const convoBox = await page.locator('#convo').boundingBox();
  await page.mouse.move(convoBox.x + 60, convoBox.y + 80);
  await page.mouse.click(convoBox.x + 60, convoBox.y + 80);
  await page.keyboard.press('Control+f');
  await settle(page, 100);
  assert.equal(await page.locator('.wb-find-bar').count(), 0, `${theme}: Ctrl+F outside card must not open in-card find bar`);

  // Now scroll the open panel card into the middle of #bcol, move pointer over it, and press Ctrl+F.
  await page.evaluate(() => {
    const p = document.querySelector('#bcol details.entry-panel');
    if (p) p.scrollIntoView({ block: 'center', behavior: 'instant' });
  });
  await settle(page, 160);
  const panelBox = await page.locator('#bcol details.entry-panel').first().boundingBox();
  console.log(`[${theme} panelBox]`, JSON.stringify(panelBox));
  await page.mouse.move(panelBox.x + 80, panelBox.y + Math.min(48, panelBox.height / 2));
  await settle(page, 80);
  await page.keyboard.press('Control+f');
  await page.waitForSelector('#bcol details.entry-panel .wb-find-bar', { state: 'visible', timeout: 6000 });
  await page.fill('#bcol details.entry-panel .wb-find-input', 'cache');
  await page.waitForFunction(() => {
    const el = document.querySelector('#bcol details.entry-panel .wb-find-count');
    return el && /^1\/[4-9]$/.test(el.textContent.trim());
  }, { timeout: 5000 });
  // Step to next match with Enter so active highlight moves to 2/N.
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => {
    const el = document.querySelector('#bcol details.entry-panel .wb-find-count');
    return el && /^2\/[4-9]$/.test(el.textContent.trim());
  }, { timeout: 5000 });
  await settle(page, 180);

  const findCountText = await page.locator('#bcol details.entry-panel .wb-find-count').textContent();
  assert.match(findCountText.trim(), /^2\/[4-9]$/, `${theme}: find bar counts matches across header + sandboxed iframe`);
  await page.screenshot({ path: path.join(outDir, `card-find-open-${theme}.png`) });

  // Shift+Enter steps back to 1/N; Escape closes bar and cleans up.
  await page.keyboard.press('Shift+Enter');
  await page.waitForFunction(() => {
    const el = document.querySelector('#bcol details.entry-panel .wb-find-count');
    return el && /^1\/[4-9]$/.test(el.textContent.trim());
  }, { timeout: 5000 });
  await page.keyboard.press('Escape');
  await page.waitForSelector('.wb-find-bar', { state: 'detached' });

  // 3) Local hold + held pill category + auto-clear on content edit:
  // Topics render newest-first: follow-up-notes (#item-5, top), release-timing (#item-3, #item-4, middle), rollout-checklist (#item-1, #item-2, bottom).
  // Hold #item-5 (top topic, scrolls above viewport so top pill shows held 1) AND #item-3 (middle topic, visible in viewport in held state!).
  await page.locator('#bcol #item-5 .wb-hold-toggle').click();
  await page.locator('#bcol #item-3 .wb-hold-toggle').click();
  await settle(page, 160);

  // Scroll #bcol so #item-5 is above viewport (top pill shows held 1) and #item-3 (held) + #item-4 (open) are fully visible below the stuck header,
  // while #item-1 and #item-2 are below viewport (bottom pill shows open 2).
  await page.evaluate(() => {
    const b = document.getElementById('bcol');
    const item3 = document.getElementById('item-3');
    const br = b.getBoundingClientRect();
    const ir = item3.getBoundingClientRect();
    b.scrollTop += (ir.top - br.top) - 185;
  });
  await settle(page, 220);

  const heldState = await page.evaluate(() => {
    const topPill = document.querySelector('#bcol .oq-wrap.wb-oq-top .oq-pill');
    const segs = Array.from(topPill.querySelectorAll('.oq-seg'))
      .filter(s => !s.hidden)
      .map(s => [s.getAttribute('data-k'), s.querySelector('.n').textContent]);
    return {
      title: document.title,
      item5Held: document.getElementById('item-5').classList.contains('wb-held'),
      item3Held: document.getElementById('item-3').classList.contains('wb-held'),
      topPillHidden: topPill.hidden,
      segs
    };
  });
  assert.equal(heldState.item5Held, true, `${theme}: item-5 marked held`);
  assert.equal(heldState.item3Held, true, `${theme}: item-3 marked held`);
  assert.equal(heldState.topPillHidden, false, `${theme}: top pill visible`);
  assert.ok(heldState.segs.some(([k, n]) => k === 'held' && n === '1'), `${theme}: top pill shows held count 1 (${JSON.stringify(heldState.segs)})`);
  assert.ok(heldState.title.startsWith('(3) '), `${theme}: tab title excludes 2 held items from 5 total asks (got ${heldState.title})`);
  await page.screenshot({ path: path.join(outDir, `pill-held-${theme}.png`) });

  // Now simulate agent editing item-3's content (?edited=1) while keeping same board identity & localStorage!
  await page.goto(`${baseURL}/s/uxdemo?theme=${theme}&edited=1`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#bcol #item-3', { state: 'visible' });
  await page.evaluate(() => {
    const b = document.getElementById('bcol');
    const item3 = document.getElementById('item-3');
    const br = b.getBoundingClientRect();
    const ir = item3.getBoundingClientRect();
    b.scrollTop += (ir.top - br.top) - 185;
  });
  await settle(page, 220);

  const autoClearState = await page.evaluate(() => {
    const topPill = document.querySelector('#bcol .oq-wrap.wb-oq-top .oq-pill');
    const segs = Array.from(topPill.querySelectorAll('.oq-seg'))
      .filter(s => !s.hidden)
      .map(s => [s.getAttribute('data-k'), s.querySelector('.n').textContent]);
    return {
      title: document.title,
      item5Held: document.getElementById('item-5').classList.contains('wb-held'),
      item3Held: document.getElementById('item-3').classList.contains('wb-held'),
      item3Title: document.querySelector('#item-3 .title').textContent,
      segs
    };
  });
  assert.equal(autoClearState.item5Held, true, `${theme}: unedited item-5 keeps its hold across reload`);
  assert.equal(autoClearState.item3Held, false, `${theme}: edited item-3 hold auto-cleared`);
  assert.ok(autoClearState.title.startsWith('(4) '), `${theme}: tab title increments back to (4) after auto-clear (got ${autoClearState.title})`);
  await page.screenshot({ path: path.join(outDir, `pill-autoclear-${theme}.png`) });

  assert.equal(pageErrors.length, 0, `${theme}: expected zero page errors, got ${pageErrors.map(e => e.message).join('; ')}`);
  await context.close();
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    await runTheme(browser, 'light');
    await runTheme(browser, 'dark');
    console.log('board-chat-small-ux browser acceptance passed for light and dark themes');
  } finally {
    await browser.close();
  }
})().catch(err => {
  console.error(err);
  process.exit(1);
});
