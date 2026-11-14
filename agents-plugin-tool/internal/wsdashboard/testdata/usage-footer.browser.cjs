// Source-only, fake-data LOOK check. No daemon, build, credentials or network.
// WS_PLAYWRIGHT_MODULE=/absolute/path/to/playwright node <this-file> <mockup.html> <shots-dir>
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [mockup, output] = process.argv.slice(2);
assert.ok(mockup && output, 'pass the approved mockup HTML and output directory');
const assets = path.resolve(__dirname, '../assets');
const css = [path.resolve(__dirname, '../../wswhiteboard/assets/page.css'), path.join(assets, 'dashboard.css'), path.join(assets, 'usage.css')].map(p => fs.readFileSync(p, 'utf8')).join('\n');
const scripts = ['rail.js', 'usage.js', 'dashboard.js'].map(p => fs.readFileSync(path.join(assets, p), 'utf8'));
const at = Date.UTC(2026, 10, 15);
function entries(variant) {
  const observed = variant === 'stale' ? at - 42 * 60000 : at;
  const w = (kind, percent, elapsed, asOf) => ({ kind, percent, asOf, resetsAt: asOf + (1 - elapsed) * (kind === 'five_hour' ? 5 * 3600000 : 7 * 86400000) });
  const gpt = { harness: 'codex', source: 'via Pi', windows: [w('seven_day', 38, .45, at)] };
  if (variant === 'weekly') return [gpt];
  return [{ harness: 'claude', source: 'Claude push', windows: [w('five_hour', variant === 'over' ? 64 : 20, 1 / 3, observed), w('seven_day', 53, .66, observed)] }, gpt];
}
(async () => {
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) for (const width of [1280, 360]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: theme, serviceWorkers: 'block' });
      const requests = [];
      await context.route(/^https?:/, route => { requests.push(route.request().url()); return route.abort(); });
      const reference = await context.newPage();
      await reference.goto(pathToFileURL(path.resolve(mockup)).href);
      for (const variant of ['normal', 'over', 'stale', 'weekly']) {
        const page = await context.newPage(), errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.setContent(`<!doctype html><html data-theme="${theme}" data-mode="index" data-shell><head><style>${css}</style><style>
          body { margin: 0; padding: 10px; }
          .rail { position: static !important; transform: none !important; visibility: visible !important; width: 340px !important; max-width: 100%; height: auto !important; }
        </style></head><body><aside class="rail" id="rail"><div id="r-rail"><div aria-hidden="true" style="height:48px"></div><div class="rfoot"><button class="usage-toggle" type="button" aria-label="Plan usage" aria-expanded="false"></button></div></div></aside><button id="outside" style="position:fixed;bottom:10px;right:10px">Outside</button></body></html>`);
        await page.evaluate(({ at, entries }) => {
          Date.now = () => at;
          window.wsT = key => key; window.wsTn = key => key;
          // Disable the stream transport; only usage fetches may occur, and
          // these resolve locally. Load the real dashboard event handlers.
          window.WebSocket = undefined;
          window.fetch = url => {
            if (!String(url).startsWith('/rail/usage')) throw new Error('Unexpected fixture fetch: ' + url);
            return Promise.resolve({ ok: true, json: () => Promise.resolve({ entries, warnPercent: 80, critPercent: 95 }) });
          };
        }, { at, entries: entries(variant) });
        for (const content of scripts) await page.addScriptTag({ content });
        assert.deepEqual(errors, [], `${theme}/${width}/${variant}: initialization`);
        await page.locator('.usage-number').first().waitFor();
        assert.equal(await page.locator('.usage-column').count(), variant === 'weekly' ? 1 : 3);
        assert.equal(await page.locator('.usage-clock').count(), variant === 'stale' ? 1 : 0);
        assert.equal(await page.locator('.usage-pace').first().textContent(), variant === 'over' ? '+31%' : variant === 'weekly' ? '−7%' : '−13%');
        assert.ok(await page.locator('.rfoot').evaluate(e => e.scrollWidth <= e.clientWidth), `${theme}/${width}/${variant}: footer fits`);
        if (variant === 'stale') {
          const tones = await page.locator('.usage-column').first().evaluate(e => ({ current: getComputedStyle(e).color, pace: getComputedStyle(e.querySelector('.usage-pace')).color, dotted: getComputedStyle(e.querySelector('.usage-number')).textDecorationStyle }));
          assert.equal(tones.current, tones.pace); assert.equal(tones.dotted, 'dotted');
        }
        // The approved reference is dark-only. For light review keep its
        // geometry intact and adapt only its palette to production tokens;
        // never edit the canonical mockup file.
        if (theme === 'light' && variant === 'normal') {
          const palette = await page.evaluate(() => {
            const style = getComputedStyle(document.documentElement);
            return ':root { color-scheme: light; ' + Array.from(style).filter(k => k.startsWith('--wb-')).map(k => k + ':' + style.getPropertyValue(k) + ';').join('') + '}';
          });
          await reference.addStyleTag({ content: palette });
        }
        const prefix = `${theme}-${width}-${variant}`;
        const expected = await reference.locator(`[data-variant="${variant}"] .rfoot`).screenshot({ path: path.join(output, `${prefix}-mockup.png`) });
        const actual = await page.locator('.rfoot').screenshot({ path: path.join(output, `${prefix}-actual.png`) });
        const expectedSize = await reference.locator(`[data-variant="${variant}"] .rfoot`).boundingBox();
        const actualSize = await page.locator('.rfoot').boundingBox();
        assert.ok(Math.abs(expectedSize.width - actualSize.width) < 1 && Math.abs(expectedSize.height - actualSize.height) < 1, `${prefix}: matching reference footer dimensions`);
        const comparison = await context.newPage();
        await comparison.setViewportSize({ width: 720, height: 130 });
        await comparison.setContent(`<body style="margin:0;background:${theme === 'dark' ? '#141518' : '#fff'};color:${theme === 'dark' ? '#e7e7e4' : '#222'};font:12px system-ui"><div style="display:flex;gap:16px;padding:10px"><div>Approved A${theme === 'light' ? ' (light palette)' : ''}<br><img src="data:image/png;base64,${expected.toString('base64')}"></div><div>Implementation<br><img src="data:image/png;base64,${actual.toString('base64')}"></div></div></body>`);
        await comparison.screenshot({ path: path.join(output, `${prefix}-comparison.png`) });
        await comparison.close();
        // These are the production dashboard.js open, Escape, outside-click,
        // close-button, aria-expanded and focus-restoration paths, not mocks.
        const button = page.locator('.usage-toggle'), dialog = page.getByRole('dialog', { name: 'Plan usage' });
        await button.focus(); await page.keyboard.press('Enter');
        await dialog.waitFor({ state: 'visible' });
        assert.equal(await button.getAttribute('aria-expanded'), 'true');
        assert.ok((await page.locator('.usage-details').textContent()).includes(variant === 'weekly' ? 'via Pi' : 'Claude push'));
        await page.keyboard.press('Escape'); await dialog.waitFor({ state: 'hidden' });
        assert.equal(await button.getAttribute('aria-expanded'), 'false');
        assert.equal(await button.evaluate(e => e === document.activeElement), true);
        await button.click(); await dialog.waitFor({ state: 'visible' });
        await page.locator('#outside').click(); await dialog.waitFor({ state: 'hidden' });
        assert.equal(await button.getAttribute('aria-expanded'), 'false');
        await button.click(); await dialog.waitFor({ state: 'visible' });
        await page.getByRole('button', { name: 'Close plan usage' }).click();
        await dialog.waitFor({ state: 'hidden' });
        assert.equal(await button.evaluate(e => e === document.activeElement), true);
        assert.deepEqual(errors, []);
        console.log(`${prefix}: geometry, state, keyboard, Escape and outside-click passed`);
        await page.close();
      }
      assert.deepEqual(requests, [], 'no network requests');
      await context.close();
    }
    console.log('Usage footer checks passed; inspect comparison PNGs in ' + path.resolve(output));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
