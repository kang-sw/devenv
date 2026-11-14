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
const script = fs.readFileSync(path.join(assets, 'usage.js'), 'utf8');
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
    for (const width of [1280, 360]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: 'dark' });
      const reference = await context.newPage(), page = await context.newPage();
      const errors = []; page.on('pageerror', e => errors.push(e.message));
      await reference.goto(pathToFileURL(path.resolve(mockup)).href);
      for (const variant of ['normal', 'over', 'stale', 'weekly']) {
        await page.setContent(`<!doctype html><html data-theme="dark"><head><style>${css}</style><style>
          body { margin: 0; padding: 10px; }
          .rail { position: static !important; transform: none !important; visibility: visible !important; width: 340px !important; max-width: 100%; height: auto !important; }
        </style></head><body><aside class="rail"><div class="rfoot"><button class="usage-toggle" type="button" aria-label="Plan usage" aria-expanded="false"></button></div></aside></body></html>`);
        await page.evaluate(({ at, entries }) => {
          Date.now = () => at;
          window.fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve({ entries, warnPercent: 80, critPercent: 95 }) });
        }, { at, entries: entries(variant) });
        await page.addScriptTag({ content: script });
        await page.evaluate(() => {
          const rail = document.querySelector('.rail'), button = rail.querySelector('button');
          const mounted = WsUsage.mount(document, rail);
          const pop = mounted.makePopover(() => {}, () => { pop.close(); pop.el.hidden = true; button.focus(); });
          document.body.appendChild(pop.el);
          button.addEventListener('click', () => { pop.el.hidden = false; pop.open(); pop.focus(); });
        });
        await page.locator('.usage-number').first().waitFor();
        assert.equal(await page.locator('.usage-column').count(), variant === 'weekly' ? 1 : 3);
        assert.equal(await page.locator('.usage-clock').count(), variant === 'stale' ? 1 : 0);
        assert.equal(await page.locator('.usage-pace').first().textContent(), variant === 'over' ? '+31%' : variant === 'weekly' ? '−7%' : '−13%');
        const fit = await page.locator('.rfoot').evaluate(e => e.scrollWidth <= e.clientWidth);
        assert.ok(fit, `${width}/${variant}: footer fits`);
        if (variant === 'stale') {
          const tones = await page.locator('.usage-column').first().evaluate(e => ({ current: getComputedStyle(e).color, pace: getComputedStyle(e.querySelector('.usage-pace')).color, dotted: getComputedStyle(e.querySelector('.usage-number')).textDecorationStyle }));
          assert.equal(tones.current, tones.pace); assert.equal(tones.dotted, 'dotted');
        }
        const expected = await reference.locator(`[data-variant="${variant}"] .rfoot`).screenshot({ path: path.join(output, `${width}-${variant}-mockup.png`) });
        const actual = await page.locator('.rfoot').screenshot({ path: path.join(output, `${width}-${variant}-actual.png`) });
        // Preserve a side-by-side PNG at matching footer dimensions for review.
        const comparison = await context.newPage();
        await comparison.setViewportSize({ width: 720, height: 130 });
        await comparison.setContent(`<body style="margin:0;background:#141518;color:#e7e7e4;font:12px system-ui"><div style="display:flex;gap:16px;padding:10px"><div>Approved A<br><img src="data:image/png;base64,${expected.toString('base64')}"></div><div>Implementation<br><img src="data:image/png;base64,${actual.toString('base64')}"></div></div></body>`);
        await comparison.screenshot({ path: path.join(output, `${width}-${variant}-comparison.png`) });
        await comparison.close();
        // Exercise the existing exported popover lifecycle with a native keyboard click.
        await page.locator('.usage-toggle').focus(); await page.keyboard.press('Enter');
        await page.getByRole('dialog', { name: 'Plan usage' }).waitFor({ state: 'visible' });
        assert.ok((await page.locator('.usage-details').textContent()).includes(variant === 'weekly' ? 'via Pi' : 'Claude push'));
        await page.getByRole('button', { name: 'Close plan usage' }).click();
        assert.equal(await page.locator('.usage-toggle').evaluate(e => e === document.activeElement), true);
      }
      assert.deepEqual(errors, []);
      await context.close();
    }
    console.log('Usage footer fixture checks passed; inspect comparison PNGs in ' + path.resolve(output));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
