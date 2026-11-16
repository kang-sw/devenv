const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const html = fs.readFileSync(process.argv[2], 'utf8');
const shots = process.argv[3];
const now = Number(process.argv[4]);
const scripts = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map(m => m[1]);
const withoutScripts = html.replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '');
const topic = name => `.topic[data-topic="${name}"]`;
const badge = name => `${topic(name)} .sb`;
const tip = name => `${topic(name)} .sb-tip`;
(async () => {
  fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const p = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
    await p.setContent(withoutScripts);
    await p.evaluate(n => { window.__stateNow = n; Date.now = () => window.__stateNow; }, now);
    const markup = () => p.locator('.topic-state').evaluateAll(nodes => nodes.map(n => n.innerHTML));
    const server = await markup();
    assert.equal(server.length, 8);
    for (const content of scripts) await p.addScriptTag({ content });
    assert.deepEqual(await markup(), server, 'server markup equals the first tick');
    await p.evaluate(() => {
      document.querySelectorAll('.topic-state').forEach(box => { box.textContent = ''; delete box.wbStateKey; });
      window.wbAgo(document);
    });
    assert.deepEqual(await markup(), server, 'independent client construction equals server markup');
    assert.equal(await p.locator(badge('wait')).getAttribute('class'), 'sb wait solo');
    assert.equal(await p.locator(`${badge('wait')} use`).getAttribute('href'), '#i-st-wait');
    assert.equal(await p.locator(badge('research')).innerText(), '~30m');
    assert.equal(await p.locator(`${topic('over')} .ext`).innerText(), '연장 1회 · 최초 45분');
    assert.equal(await p.locator(`${topic('extended')} .ext`).innerText(), '연장 2회 · 최초 120분');
    assert.equal(await p.locator(`${badge('over')} b`).innerText(), '72');
    assert((await p.locator(badge('over')).getAttribute('class')).includes('over'));
    assert.equal(await p.locator('.eta, .stb, .state-estimate, .wb-state-dot, [aria-live]').count(), 0);
    await p.evaluate(() => {
      window.__pill = document.querySelector('[data-topic="impl"] .sb');
      window.__number = window.__pill.querySelector('b');
      window.__icon = window.__pill.querySelector('svg');
      window.__stateNow += 56 * 60000;
      window.wbAgo(document);
    });
    assert.equal(await p.locator(`${badge('impl')} b`).innerText(), '61');
    assert.equal(await p.locator(badge('impl')).getAttribute('aria-label'), '구현 중 · pi 작업자 5 · ETA 60분 중 61분 경과 · 1분 초과');
    assert((await p.locator(badge('impl')).getAttribute('class')).includes('over'));
    assert(await p.evaluate(() => window.__pill === document.querySelector('[data-topic="impl"] .sb') && window.__number === window.__pill.querySelector('b') && window.__icon === window.__pill.querySelector('svg')));
    await p.evaluate(n => { window.__stateNow = n; window.wbAgo(document); }, now);
    await p.locator(badge('over')).hover();
    await p.waitForFunction(sel => document.querySelector(sel).matches(':popover-open'), tip('over'));
    assert.equal(await p.locator(tip('over')).evaluate(n => getComputedStyle(n).position), 'fixed');
    const r = await p.locator(tip('over')).boundingBox();
    assert(r.x >= 8 && r.x + r.width <= 1272 && r.y >= 8 && r.y + r.height <= 992);
    assert((await p.locator(tip('over')).innerText()).includes('12분 초과'));
    await p.mouse.move(1278, 998);
    await p.waitForFunction(sel => document.querySelector(sel).hidden, tip('over'));
    await p.locator(badge('over')).focus();
    await p.waitForFunction(sel => document.querySelector(sel).matches(':popover-open'), tip('over'));
    await p.keyboard.press('Escape');
    await p.waitForFunction(sel => document.querySelector(sel).hidden, tip('over'));
    assert(await p.locator(badge('over')).evaluate(n => n === document.activeElement));
    await p.locator(badge('research')).focus();
    await p.waitForFunction(sel => document.querySelector(sel).matches(':popover-open'), tip('research'));
    await p.locator(badge('research')).evaluate(n => n.blur());
    await p.waitForFunction(sel => document.querySelector(sel).hidden, tip('research'));
    // A dependency still uses click-to-pin and its original dialog semantics.
    // Scroll closes popovers by design; settle Playwright's automatic scroll
    // before testing the click, rather than racing its deferred scroll event.
    await p.locator(`${topic('narrow')} .dbtn`).first().scrollIntoViewIfNeeded();
    await p.waitForTimeout(200);
    await p.locator(`${topic('narrow')} .dbtn`).first().click();
    assert.equal(await p.locator(`${topic('narrow')} .dbtn`).first().getAttribute('aria-expanded'), 'true');
    await p.keyboard.press('Escape');
    await p.locator(`${topic('narrow')} .dbtn`).first().evaluate(n => n.blur());
    // Capture a fresh page: interaction checks scrolled the standalone rail,
    // whose pinned headers are intentionally tied to the previous layout.
    const capture = await browser.newPage({ viewport: { width: 1280, height: 1600 } });
    await capture.setContent(withoutScripts);
    await capture.evaluate(n => {
      window.__stateNow = n; Date.now = () => window.__stateNow;
      document.querySelector('[data-topic="narrow"]').style.width = '420px';
    }, now);
    for (const content of scripts) await capture.addScriptTag({ content });
    for (const theme of ['light', 'dark']) {
      await capture.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme);
      const fit = await capture.locator(topic('narrow')).evaluate(n => {
        const bar = n.querySelector('.tbar'), title = bar.querySelector('.tname');
        const items = [title, ...bar.querySelectorAll('.tlead > .dw, .sb, .tago, .tnote-toggle')];
        const centers = items.map(n => { const r = n.getBoundingClientRect(); return r.y + r.height / 2; });
        return { height: bar.getBoundingClientRect().height, delta: Math.max(...centers) - Math.min(...centers), ellipsis: title.scrollWidth > title.clientWidth, title: title.title, text: title.textContent };
      });
      assert(fit.height <= 34 && fit.delta <= 3, JSON.stringify(fit));
      assert(fit.ellipsis && fit.title === fit.text);
      await capture.screenshot({ path: path.join(shots, `topic-state-${theme}.png`), fullPage: true });
      await capture.waitForTimeout(200);
      await capture.locator(badge('over')).focus();
      await capture.waitForFunction(sel => document.querySelector(sel).matches(':popover-open'), tip('over'));
      // A full-page capture can resize/scroll the viewport, which correctly
      // dismisses popovers. The open-tip capture keeps the viewport unchanged.
      await capture.screenshot({ path: path.join(shots, `topic-state-tip-${theme}.png`) });
      assert(await capture.locator(tip('over')).evaluate(n => n.matches(':popover-open')));
      await capture.keyboard.press('Escape');
      await capture.locator(badge('over')).evaluate(n => n.blur());
    }
    // The former 600px row-break rule must not return on narrow viewports.
    await capture.setViewportSize({ width: 480, height: 900 });
    assert(await capture.locator(topic('narrow')).evaluate(n => n.querySelector('.tbar').getBoundingClientRect().height <= 34));
    console.log('PASS server/client parity, clock identity, overdue/extension/estimate text, all five glyphs, hover/focus/leave/blur/Esc, dependency pin and 420px single-row ellipsis');
    console.log('Screenshots: ' + shots);
  } finally { await browser.close(); }
})().catch(err => { console.error(err); process.exitCode = 1; });
