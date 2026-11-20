// Source-only, fake-data browser verification and UI capture of the plan-usage
// header handle and popover showing both agy model groups alongside Claude and
// Codex, including footer age, disabled bucket dimming, and per-group history.
// PLAYWRIGHT_BROWSERS_PATH=... WS_PLAYWRIGHT_MODULE=... node <this-file> <shots-dir>
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [output] = process.argv.slice(2);
assert.ok(output, 'pass output directory for PNG captures');

const { readAssetSource } = require('./asset-source.cjs');
const css = [
  readAssetSource('wswhiteboard', 'page.css'),
  readAssetSource('wsdashboard', 'dashboard.css'),
  readAssetSource('wsdashboard', 'usage.css'),
].join('\n');
const scripts = ['rail.js', 'usage.js', 'dashboard.js'].map(p => readAssetSource('wsdashboard', p));
const enTable = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'wsi18n', 'tables', 'en.json'), 'utf8'));

const at = Date.UTC(2026, 10, 19, 12, 0);
const agyObserved = at - 12 * 60000;
const w = (kind, percent, elapsed, asOf, disabled) => ({
  kind,
  percent,
  asOf,
  resetsAt: asOf + (1 - elapsed) * (kind === 'five_hour' ? 5 * 3600000 : 7 * 86400000),
  ...(disabled ? { disabled: true } : {}),
});

const gem5h = w('five_hour', 42, 0.45, agyObserved);
const gemWk = w('seven_day', 58, 0.52, agyObserved);
const cg5h = w('five_hour', 84, 0.60, agyObserved, true);
const cgWk = w('seven_day', 29, 0.40, agyObserved);
const cl5h = w('five_hour', 22, 0.35, at);
const clWk = w('seven_day', 51, 0.64, at);
const cxWk = w('seven_day', 37, 0.45, at);

const payload = {
  warnPercent: 80,
  critPercent: 95,
  entries: [
    { harness: 'claude', plan: 'max', source: 'Claude push', asOf: at, windows: [cl5h, clWk] },
    { harness: 'codex', plan: 'pro', source: 'via Codex CLI', asOf: at, windows: [cxWk] },
    { harness: 'agy', group: 'gemini', label: 'Gemini Models', source: 'via agy CLI', asOf: agyObserved, windows: [gem5h, gemWk] },
    { harness: 'agy', group: 'claude_gpt', label: 'Claude and GPT models', source: 'via agy CLI', asOf: agyObserved, windows: [cg5h, cgWk] },
  ],
  history: {
    schema_version: 1,
    status: 'ok',
    samples: [
      { harness: 'claude', kind: 'five_hour', source: 'Claude push', resetsAt: cl5h.resetsAt, asOf: at - 60 * 60000, percent: 10 },
      { harness: 'claude', kind: 'five_hour', source: 'Claude push', resetsAt: cl5h.resetsAt, asOf: at, percent: 22 },
      { harness: 'codex', kind: 'seven_day', source: 'via Codex CLI', resetsAt: cxWk.resetsAt, asOf: at - 24 * 3600000, percent: 24 },
      { harness: 'codex', kind: 'seven_day', source: 'via Codex CLI', resetsAt: cxWk.resetsAt, asOf: at, percent: 37 },
    ],
  },
  agyHistory: {
    schema_version: 1,
    status: 'ok',
    samples: [
      { harness: 'agy', group: 'gemini', kind: 'five_hour', source: 'via agy CLI', resetsAt: gem5h.resetsAt, asOf: agyObserved - 60 * 60000, percent: 18 },
      { harness: 'agy', group: 'gemini', kind: 'five_hour', source: 'via agy CLI', resetsAt: gem5h.resetsAt, asOf: agyObserved, percent: 42 },
      { harness: 'agy', group: 'gemini', kind: 'seven_day', source: 'via agy CLI', resetsAt: gemWk.resetsAt, asOf: agyObserved - 48 * 3600000, percent: 30 },
      { harness: 'agy', group: 'gemini', kind: 'seven_day', source: 'via agy CLI', resetsAt: gemWk.resetsAt, asOf: agyObserved, percent: 58 },
      { harness: 'agy', group: 'claude_gpt', kind: 'seven_day', source: 'via agy CLI', resetsAt: cgWk.resetsAt, asOf: agyObserved - 48 * 3600000, percent: 12 },
      { harness: 'agy', group: 'claude_gpt', kind: 'seven_day', source: 'via agy CLI', resetsAt: cgWk.resetsAt, asOf: agyObserved, percent: 29 },
    ],
  },
};

(async () => {
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 2200 }, colorScheme: theme, serviceWorkers: 'block' });
      const requests = [];
      await context.route(/^https?:/, route => { requests.push(route.request().url()); return route.abort(); });
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.setContent(`<!doctype html><html data-theme="${theme}" data-mode="index" data-shell><head><style>${css}</style><style>
        body { margin: 0; padding: 12px; background: var(--wb-bg); color: var(--wb-fg); }
        .dwrap { padding: 0; }
      </style></head><body><div class="dwrap"><div class="top"><span class="tcen"><button class="usage-toggle" type="button" aria-label="Plan usage" aria-expanded="false"></button><span class="talert" id="talert" hidden></span></span></div></div><aside class="rail" id="rail"><div id="r-rail"><div class="rfoot"></div></div></aside></body></html>`);
      await page.evaluate(({ at, payload, enTable }) => {
        Date.now = () => at;
        window.wsI18n = { lang: 'en', t: enTable };
        window.wsT = (key, ...args) => (enTable[key] || key).replace(/\{(\d+)\}/g, (m, i) => i < args.length ? String(args[i]) : m);
        window.wsTn = key => key;
        window.WebSocket = undefined;
        window.fetch = url => {
          if (!String(url).startsWith('/rail/usage')) throw new Error('Unexpected fetch: ' + url);
          return Promise.resolve({ ok: true, json: () => Promise.resolve(payload) });
        };
      }, { at, payload, enTable });
      for (const content of scripts) await page.addScriptTag({ content });
      assert.deepEqual(errors, [], `${theme}: init errors`);

      await page.locator('.usage-group-age').first().waitFor();
      // Header handle checks: single 34px row, 4 harness groups (Claude, Codex, Gemini, Claude/GPT),
      // disabled 5h bucket omitted in footer (2 + 1 + 2 + 1 = 6 columns).
      const handle = await page.locator('.usage-toggle').boundingBox();
      assert.equal(handle.height, 34, `${theme}: single 34px handle row`);
      assert.equal(await page.locator('.usage-harness').count(), 4);
      assert.equal(await page.locator('.usage-column').count(), 6);
      assert.equal(await page.locator('.usage-group-age').count(), 2);
      await page.locator('.usage-toggle').screenshot({ path: path.join(output, `agy-gutter-${theme}.png`) });

      // Open popover and verify both agy groups, dimmed disabled card, and per-group charts.
      await page.locator('.usage-toggle').click();
      const pop = page.locator('.usage-pop');
      await pop.waitFor({ state: 'visible' });
      assert.equal(await pop.locator('.usage-account').count(), 4);
      assert.equal(await pop.locator('.usage-window-card.usage-disabled').count(), 1);
      const disabledOpacity = await pop.locator('.usage-window-card.usage-disabled').evaluate(e => parseFloat(getComputedStyle(e).opacity));
      assert.ok(disabledOpacity < 0.7, `${theme}: disabled card dimmed (${disabledOpacity})`);
      assert.equal(await pop.locator('.usage-window-empty').count(), 1);
      assert.equal(await pop.locator('.usage-window-empty').textContent(), 'no 5-hour window');
      const wideLayout = await pop.evaluate(el => ({
        width: el.getBoundingClientRect().width,
        rows: getComputedStyle(el.querySelector('.usage-row-label')).display,
        columns: getComputedStyle(el.querySelector('.usage-col-head')).display,
      }));
      assert.ok(wideLayout.width <= 988 && wideLayout.width > 980, `${theme}: wide popover width ${wideLayout.width}`);
      assert.notEqual(wideLayout.rows, 'none', `${theme}: wide B row labels visible`);
      assert.equal(wideLayout.columns, 'none', `${theme}: wide B column labels hidden`);
      await pop.screenshot({ path: path.join(output, `agy-popover-${theme}.png`) });

      await page.setViewportSize({ width: 420, height: 2200 });
      await page.evaluate(() => window.dispatchEvent(new Event('resize')));
      const narrowLayout = await pop.evaluate(el => {
        const percentRows = [...el.querySelectorAll('.usage-window')];
        return {
          width: el.getBoundingClientRect().width,
          rows: getComputedStyle(el.querySelector('.usage-row-label')).display,
          columns: getComputedStyle(el.querySelector('.usage-col-head')).display,
          vendorColumn: getComputedStyle(el.querySelector('.usage-vhead')).gridColumn,
          overflow: percentRows.some(n => n.scrollWidth > n.clientWidth),
          pageWidth: document.documentElement.scrollWidth,
          viewportWidth: window.innerWidth,
        };
      });
      assert.ok(narrowLayout.width <= 404, `${theme}: narrow popover width ${narrowLayout.width}`);
      assert.equal(narrowLayout.rows, 'none', `${theme}: narrow B row labels hidden`);
      assert.notEqual(narrowLayout.columns, 'none', `${theme}: narrow C column labels visible`);
      assert.equal(narrowLayout.vendorColumn, '1 / -1', `${theme}: narrow vendor head above cards`);
      assert.equal(narrowLayout.overflow, false, `${theme}: percent rows stay inside their cards`);
      assert.ok(narrowLayout.pageWidth <= narrowLayout.viewportWidth, `${theme}: no horizontal page scroll`);
      await pop.screenshot({ path: path.join(output, `agy-popover-narrow-${theme}.png`) });

      assert.deepEqual(requests, [], `${theme}: no network requests`);
      assert.deepEqual(errors, [], `${theme}: runtime errors`);
      await context.close();
    }
    console.log('agy usage UI capture passed; PNGs written to ' + path.resolve(output));
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
