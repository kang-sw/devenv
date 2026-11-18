// Run by TestSessionSwitchHeaderBrowser with real pages and disposable stores.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  const failures = [];
  try {
    for (const width of [1280, 1920, 390]) {
      for (const theme of ['light', 'dark']) {
        const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: theme });
        await context.addInitScript(() => {
          if (self !== top) return;
          // A frame's load event can precede its rAF/postMessage header.
          // Hold only that message to make the intermediate paint deterministic.
          window.headerGate = null;
          addEventListener('message', e => {
            const gate = window.headerGate;
            if (!gate || !e.data || e.data.wsShell !== 'head' || e.origin !== location.origin) return;
            if (e.source.location.pathname !== gate.path) return;
            e.stopImmediatePropagation();
            gate.messages.push({ data: e.data, source: e.source, origin: e.origin });
          }, true);
          const workers = navigator.serviceWorker;
          if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('No service workers in fixture'); };
        });
        await context.route('**/*', route => {
          const url = new URL(route.request().url());
          if (url.origin !== base) return route.abort();
          if (url.pathname === '/rail/usage') return route.fulfill({ json: { entries: [], warnPercent: 80, critPercent: 95 } });
          return route.continue();
        });
        const page = await context.newPage();
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.goto(base + '/s/livesession');
        await page.waitForFunction(() => document.querySelector('.top').style.minHeight !== '');
        await page.frameLocator('.sframe.on').locator('#grid').waitFor();
        await page.waitForTimeout(300);
        // Cold load, then cached return. Sample the shell and the visible
        // document's actual content top every animation frame, not just endpoints.
        for (const [rid, cold] of [['boardsession', true], ['livesession', false]]) {
          if (await page.locator('#railbtn').isVisible() && !await page.locator('html').evaluate(el => el.classList.contains('rail-open'))) await page.locator('#railbtn').click();
          await page.evaluate(({ rid, cold }) => {
            if (cold) window.headerGate = { path: '/s/' + rid, messages: [] };
            window.headerSamples = [];
            window.headerSampling = true;
            function sample() {
              const header = document.querySelector('.dwrap > .top').getBoundingClientRect();
              const rail = document.querySelector('#rail').getBoundingClientRect();
              const frame = document.querySelector('.sframe.on');
              const grid = frame.contentDocument.querySelector('#grid');
              window.headerSamples.push({ height: header.height, railTop: rail.top,
                mainTop: grid ? frame.getBoundingClientRect().top + grid.getBoundingClientRect().top : null });
              if (window.headerSampling) requestAnimationFrame(sample);
            }
            sample();
          }, { rid, cold });
          await page.locator(`.rs[data-rid="${rid}"] .rl`).click();
          if (cold) await page.waitForFunction(() => window.headerGate.messages.length > 0);
          await page.waitForTimeout(150);
          if (shots && cold) {
            fs.mkdirSync(shots, { recursive: true });
            await page.screenshot({ path: path.join(shots, `mid-switch-${theme}-${width}.png`) });
          }
          await page.evaluate(() => {
            const gate = window.headerGate;
            window.headerGate = null;
            if (gate) gate.messages.forEach(m => dispatchEvent(new MessageEvent('message', m)));
          });
          await page.waitForFunction(rid => {
            const frame = document.querySelector('.sframe.on');
            return frame && frame.contentWindow.location.pathname === '/s/' + rid;
          }, rid);
          await page.waitForTimeout(150);
          const samples = await page.evaluate(() => { window.headerSampling = false; return window.headerSamples; });
          const ranges = {};
          for (const key of ['height', 'railTop', 'mainTop']) {
            const values = samples.map(s => s[key]);
            assert.ok(values.every(Number.isFinite), `${key} must exist on every frame`);
            ranges[key] = [Math.min(...values), Math.max(...values)];
            if (ranges[key][1] !== ranges[key][0]) failures.push(`${width}/${theme}/${cold ? 'cold' : 'cached'} ${key}: ${ranges[key].join('..')}`);
          }
          console.log(JSON.stringify({ width, theme, navigation: cold ? 'cold' : 'cached', frames: samples.length, ranges }));
        }
        assert.deepEqual(errors, []);
        await context.close();
      }
    }
    assert.deepEqual(failures, [], 'Header height and main content top must not shift during session navigation');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
