// Source-only Playwright verification + side-by-side mockup captures for
// 261120 composer controls (Model + Effort only, decision (38)) and context meter.
// Run: WS_PLAYWRIGHT_MODULE=<abs playwright> node internal/wsdashboard/testdata/composer-controls-and-meter.browser.cjs <mockup.html> <shots-dir>
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const { assetBundle, meterMarkup, pillsMarkup } = require('./composer-meter-fixture.cjs');

const [mockup, output] = process.argv.slice(2);
assert.ok(mockup && output, 'pass mockup HTML path and output directory');

const ASSETS = assetBundle();
const AT = Date.UTC(2026, 10, 20, 12, 0);

const CHOICES = {
  harnesses: [
    { harness: 'claude', lead: true, available: true, models: [{ id: 'opus-4.6' }, { id: 'sonnet-4.6' }], efforts: ['low', 'medium', 'high', 'max'], permission_modes: ['auto', 'default'] },
    { harness: 'codex', lead: true, available: true, models: [{ id: 'gpt-5.4' }, { id: 'o4-mini' }], efforts: ['low', 'medium', 'high', 'xhigh'], permission_modes: ['workspace-write'] },
    { harness: 'agy', lead: true, available: true, models: [{ id: 'gemini-4-hydrogen' }], efforts: ['low', 'medium', 'high'], permission_modes: ['accept-edits'] },
    { harness: 'pi', lead: true, available: true, models: [{ id: 'pi-sonnet-4.6' }], efforts: ['off', 'medium', 'high'], permission_modes: [] },
  ],
};

function usagePayload() {
  const r5 = AT + 2.5 * 3600000, r7 = AT + 3.5 * 86400000;
  return {
    warnPercent: 80, critPercent: 95,
    entries: [{ harness: 'codex', plan: 'pro', source: 'via Codex CLI', asOf: AT, windows: [
      { kind: 'five_hour', percent: 24, resetsAt: r5, asOf: AT },
      { kind: 'seven_day', percent: 48, resetsAt: r7, asOf: AT },
    ] }],
    history: { schema_version: 1, status: 'ok', samples: [{ harness: 'codex', kind: 'seven_day', source: 'via Codex CLI', resetsAt: r7, asOf: AT - 86400000, percent: 30 }] },
    codexFiveHourHistory: { schema_version: 1, status: 'ok', samples: [{ harness: 'codex', kind: 'five_hour', source: 'via Codex CLI', resetsAt: r5, asOf: AT - 3600000, percent: 10 }, { harness: 'codex', kind: 'five_hour', source: 'via Codex CLI', resetsAt: r5, asOf: AT, percent: 24 }] },
  };
}

function composerHtml({ theme, width, harness, model, effort, isChild, canEdit, hosted, meter }) {
  const attrs = `data-theme="${theme}" data-mode="live" data-rid="sess1" data-lang="en"${hosted ? ' data-hosted="1"' : ''}${isChild ? ' data-child="1"' : ''}${canEdit ? ' data-can-edit-settings="1"' : ''}`;
  const pills = (hosted || isChild) ? pillsMarkup({ model, effort, isChild, canEdit }) : '';
  return `<!doctype html><html ${attrs}><head><style>${ASSETS.css}</style><style>
    body { margin: 0; padding: 16px; background: var(--wb-bg); color: var(--wb-fg); }
    .stage { width: ${width}px; max-width: 100%; }
  </style></head><body>${ASSETS.sprite}
  <div class="dwrap"><div class="top"><span id="r-head" class="region r-head" data-region="head"><span class="tph tph-hs"><span class="host sse"><span class="dot"></span>${harness}</span></span></span><span class="tcen"><button class="usage-toggle" type="button"></button><span id="talert" hidden></span></span></div><aside id="rail" hidden><div id="r-rail"><div class="rfoot"></div></div></aside></div>
  <div class="stage"><div id="composer" class="composer"><div id="r-ctx" class="region r-ctx" data-region="ctx">${meterMarkup(meter)}</div><div class="crow"><div class="cfield"><textarea id="ctext" rows="1" placeholder="Message..."></textarea></div><button type="button" class="csend" id="csend">Send</button></div><div class="cfoot">${pills}<div id="r-sline" class="region r-sline" data-region="sline"></div></div></div></div></body></html>`;
}

async function sideBySide(ctx, name, leftBuf, rightBuf, theme) {
  const p = await ctx.newPage({ viewport: { width: 1920, height: 1200 } });
  const bg = theme === 'dark' ? '#141518' : '#f3f3f0', fg = theme === 'dark' ? '#e7e7e4' : '#1b1d21';
  await p.setContent(`<body style="margin:0;padding:12px;background:${bg};color:${fg};font:12px system-ui"><div style="display:inline-flex;gap:16px;align-items:flex-start"><div><div style="margin-bottom:4px;font-weight:700">Mockup v2</div><img src="data:image/png;base64,${leftBuf.toString('base64')}"></div><div><div style="margin-bottom:4px;font-weight:700">Implementation (Model+Effort only, D38)</div><img src="data:image/png;base64,${rightBuf.toString('base64')}"></div></div></body>`);
  await p.locator('div').first().screenshot({ path: path.join(output, name) });
  await p.close();
}

(async () => {
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) {
      const ctx = await browser.newContext({ viewport: { width: 1280, height: 960 }, colorScheme: theme, serviceWorkers: 'block' });
      const ref = await ctx.newPage();
      await ref.goto(pathToFileURL(path.resolve(mockup)).href);
      await ref.locator(`[data-bind="theme"][data-val="${theme}"]`).click();

      for (const [wName, vw] of [['560', 560], ['320', 320]]) {
        await ref.locator(`[data-bind="width"][data-val="${wName === '560' ? 'normal' : 'narrow'}"]`).click();
        const mShot = await ref.locator('#primary-composer').screenshot({ path: path.join(output, `mockup-${theme}-${wName}.png`) });

        const page = await ctx.newPage(), errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.setContent(composerHtml({ theme, width: vw, harness: 'claude', model: 'opus-4.6', effort: 'high', isChild: false, canEdit: true, hosted: true, meter: { cur: 51293, soft: 110000, hard: 180000, native: 1000000 } }));
        await page.evaluate(({ en }) => { window.wsI18n = { lang: 'en', t: en }; window.wsT = k => en[k] || k; window.wsTn = k => k; window.WebSocket = undefined; }, { en: ASSETS.en });
        await page.addScriptTag({ content: ASSETS.dashJs });
        assert.deepEqual(errors, []);

        const cfoot = page.locator('.cfoot');
        assert.ok((await cfoot.boundingBox()).height >= 22);
        assert.equal(await page.locator('#composer-pills .fpill').count(), 2, 'Model + Effort only (decision 38)');
        assert.equal(await page.locator('[data-pop="perm"], .sb-dir, .sb-use, .sb-cost, .sb-flag, .t80, .t95').count(), 0);
        const natVis = await page.locator('.sb-native').evaluate(e => getComputedStyle(e).display !== 'none');
        assert.equal(natVis, wName === '560', `${theme}/${wName}: native parenthetical visibility`);
        const iShot = await page.locator('#composer').screenshot({ path: path.join(output, `impl-${theme}-${wName}.png`) });
        await sideBySide(ctx, `compare-${theme}-${wName}.png`, mShot, iShot, theme);
        await page.close();
      }
      await ref.close();
      await ctx.close();
    }

    // Part 2: 8-state meter gallery, rail-hover compact, 4 harnesses + roles,
    // SSE popover survival, and Codex 5h top-bar pace/history.
    const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 1200 }, colorScheme: 'dark', serviceWorkers: 'block' });
    const ref2 = await ctx2.newPage();
    await ref2.goto(pathToFileURL(path.resolve(mockup)).href);

    // 2a. 8-state meter gallery + .ctx-tip hover
    const states = [
      ['Normal', { cur: 51293, soft: 110000, hard: 180000, native: 1000000 }, '', null],
      ['Soft Crossed', { cur: 118800, soft: 110000, hard: 180000, native: 1000000 }, 'warn', '#l-info'],
      ['Nudge 1/3', { cur: 140400, soft: 110000, hard: 180000, native: 1000000 }, 'warn', '#l-bell'],
      ['Nudge 2/3', { cur: 163800, soft: 110000, hard: 180000, native: 1000000 }, 'd-hot', '#l-bell-ring'],
      ['Overflow', { cur: 189000, soft: 110000, hard: 180000, native: 1000000 }, 'crit', '#l-zap'],
      ['Hard == Native', { cur: 90000, soft: 120000, hard: 200000, native: 200000 }, '', null],
      ['Unknown', { cur: null, soft: 110000, hard: 180000, native: 1000000 }, 'd-unknown', null],
      ['Narrow 320px', { cur: 140400, soft: 110000, hard: 180000, native: 1000000, narrow: true }, 'warn', '#l-bell'],
    ];
    const gPage = await ctx2.newPage();
    await gPage.setContent(`<!doctype html><html data-theme="dark"><head><style>${ASSETS.css}</style><style>
      body { margin: 0; padding: 14px; background: var(--wb-bg); color: var(--wb-fg); }
      .glist { display: flex; flex-direction: column; gap: 8px; max-width: 660px; background: var(--wb-card); padding: 12px; border-radius: 12px; }
      .srow { display: grid; grid-template-columns: 140px minmax(0, 1fr); align-items: center; gap: 10px; }
      .srow-narrow { max-width: 300px; }
    </style></head><body>${ASSETS.sprite}<div class="glist">${states.map(([name, m]) =>
      `<div class="srow"><b style="font-size:12px">${name}</b><div class="r-ctx${m.narrow ? ' srow-narrow' : ''}">${meterMarkup(m)}</div></div>`
    ).join('')}</div>
    <div class="shp" style="margin-top:12px;width:264px;padding:10px;background:var(--wb-card);border-radius:10px">${meterMarkup(states[2][1])}</div></body></html>`);

    const srows = gPage.locator('.glist .srow');
    for (let i = 0; i < states.length; i++) {
      const [name, m, cls, icon] = states[i];
      const row = srows.nth(i);
      if (cls) assert.ok((await row.locator('.sb-ctx').getAttribute('class')).includes(cls), `${name} class ${cls}`);
      assert.equal(await row.locator('.sb-float').count(), icon ? 1 : 0, `${name} floating icon count`);
      if (icon) assert.equal(await row.locator(`.sb-float use`).getAttribute('href'), icon, `${name} icon ${icon}`);
      if (m.narrow) assert.equal(await row.locator('.sb-native').evaluate(e => getComputedStyle(e).display), 'none', 'Narrow 320px hides native');
      if (name === 'Hard == Native') assert.equal(await row.locator('.sb-native').count(), 0);
      if (name === 'Overflow') assert.equal(await row.locator('.sb-pct').textContent(), '105%');
      if (name === 'Unknown') assert.equal(await row.locator('.sb-pct').textContent(), '?');
    }
    assert.equal(await gPage.locator('.shp .sb-native').evaluate(e => getComputedStyle(e).display), 'none', 'rail-hover hides native');
    await gPage.locator('.srow').first().locator('.sb-ctx').hover();
    assert.equal(await gPage.locator('.srow').first().locator('.ctx-tip').evaluate(e => getComputedStyle(e).visibility), 'visible', 'hover reveals .ctx-tip');
    await gPage.mouse.move(0, 0);
    await gPage.locator('.shp').screenshot({ path: path.join(output, 'impl-rail-hover-dark.png') });
    const gImpl = await gPage.locator('.glist').screenshot({ path: path.join(output, 'impl-gallery-states-dark.png') });
    const gMock = await ref2.locator('#state-gallery').screenshot({ path: path.join(output, 'mockup-gallery-states-dark.png') });
    await sideBySide(ctx2, 'compare-gallery-states-dark.png', gMock, gImpl, 'dark');
    await gPage.close();

    // 2b. All 4 harnesses (2 controls only) + child lock + non-local tooltip + external omission
    const roles = [
      ['Claude Lead', { harness: 'claude', model: 'opus-4.6', effort: 'high', hosted: true, isChild: false, canEdit: true }],
      ['Codex Lead', { harness: 'codex', model: 'gpt-5.4', effort: 'high', hosted: true, isChild: false, canEdit: true }],
      ['AGY Lead', { harness: 'agy', model: 'gemini-4-hydrogen', effort: 'high', hosted: true, isChild: false, canEdit: true }],
      ['Pi Lead', { harness: 'pi', model: 'pi-sonnet-4.6', effort: 'medium', hosted: true, isChild: false, canEdit: true }],
      ['Child Lock', { harness: 'claude', model: 'sonnet', effort: 'medium', hosted: true, isChild: true, canEdit: false }],
      ['Non-local', { harness: 'claude', model: 'opus-4.6', effort: 'high', hosted: true, isChild: false, canEdit: false }],
      ['External', { harness: 'claude', model: '', effort: '', hosted: false, isChild: false, canEdit: false }],
    ];
    const rPage = await ctx2.newPage();
    await rPage.setContent(`<!doctype html><html data-theme="dark"><head><style>${ASSETS.css}</style><style>
      body { margin: 0; padding: 14px; background: var(--wb-bg); color: var(--wb-fg); }
      .flist { display: flex; flex-direction: column; gap: 6px; max-width: 580px; background: var(--wb-card); padding: 12px; border-radius: 12px; }
      .frow { display: grid; grid-template-columns: 120px minmax(0, 1fr); align-items: center; gap: 10px; }
    </style></head><body>${ASSETS.sprite}<div class="flist">${roles.map(([lbl, r]) =>
      `<div class="frow"><b style="font-size:12px">${lbl}</b><div class="cfoot">${(r.hosted || r.isChild) ? pillsMarkup(r) : ''}<div class="region r-sline"></div></div></div>`
    ).join('')}</div></body></html>`);
    const frows = rPage.locator('.frow');
    for (let i = 0; i < 6; i++) {
      assert.equal(await frows.nth(i).locator('.fpill').count(), 2, `${roles[i][0]} has 2 pills only`);
      assert.equal(await frows.nth(i).locator('[data-pop="perm"]').count(), 0);
    }
    assert.equal(await frows.nth(4).locator('use[href="#l-chevron-down"]').count(), 0, 'child shows no chevron');
    assert.equal(await frows.nth(4).locator('use[href="#l-lock"]').count(), 2, 'child shows 2 locks');
    assert.ok((await frows.nth(4).locator('.fpill').first().getAttribute('title')).includes('fixed at spawn'));
    assert.equal(await frows.nth(5).locator('use[href="#l-lock"]').count(), 0, 'non-local shows no lock');
    assert.ok((await frows.nth(5).locator('.fpill').first().getAttribute('title')).includes('host machine'));
    assert.equal(await frows.nth(6).locator('.cfoot').evaluate(e => getComputedStyle(e).display), 'none', 'external hides .cfoot');
    const rImpl = await rPage.locator('.flist').screenshot({ path: path.join(output, 'impl-harnesses-roles-dark.png') });
    const rMock = await ref2.locator('#foot-gallery').screenshot({ path: path.join(output, 'mockup-gallery-foot-dark.png') });
    await sideBySide(ctx2, 'compare-harnesses-roles-dark.png', rMock, rImpl, 'dark');
    await rPage.close();

    // 2c. Popover + POST /agents/<id>/settings + SSE region swap survival + Codex 5h usage
    const iPage = await ctx2.newPage(), posts = [], errs = [];
    iPage.on('pageerror', e => errs.push(e.message));
    await iPage.setContent(composerHtml({ theme: 'dark', width: 560, harness: 'claude', model: 'opus-4.6', effort: 'high', isChild: false, canEdit: true, hosted: true, meter: { cur: 140400, soft: 110000, hard: 180000, native: 1000000 } }));
    await iPage.evaluate(({ at, en, choices, usage }) => {
      Date.now = () => at;
      window.wsI18n = { lang: 'en', t: en }; window.wsT = k => en[k] || k; window.wsTn = k => k;
      window.WebSocket = undefined;
      window.__posts = [];
      window.fetch = (url, init) => {
        const u = String(url);
        const okJson = obj => Promise.resolve({ ok: true, status: 200, headers: { get: () => '' }, json: () => Promise.resolve(obj), text: () => Promise.resolve(JSON.stringify(obj)) });
        if (u.startsWith('/rail/usage')) return okJson(usage);
        if (u === '/support/choices') return okJson(choices);
        if (u === '/agents/sess1') return okJson({ id: 'sess1', backend: 'claude', model: 'opus-4.6', effort: 'high', can_control: true });
        if (u === '/agents/sess1/settings') {
          const b = JSON.parse(init.body);
          window.__posts.push(b);
          return okJson({ id: 'sess1', backend: 'claude', model: b.model || 'opus-4.6', effort: b.effort || 'high', can_control: true });
        }
        return okJson({});
      };
    }, { at: AT, en: ASSETS.en, choices: CHOICES, usage: usagePayload() });
    await iPage.addScriptTag({ content: ASSETS.railJs });
    await iPage.addScriptTag({ content: ASSETS.usageJs });
    await iPage.addScriptTag({ content: ASSETS.dashJs });

    // Codex 5h top-bar pace & history popover check
    await iPage.locator('.usage-toggle .usage-column').first().waitFor();
    assert.equal(await iPage.locator('.usage-toggle .usage-column').count(), 2, 'Codex 5h and 7d in top bar');
    await iPage.locator('.usage-toggle').click();
    await iPage.locator('.usage-pop .usage-window-card').first().waitFor();
    assert.equal(await iPage.locator('.usage-pop .usage-window-card').count(), 2, 'Codex 5h + 7d charts');
    await iPage.locator('.usage-pop').screenshot({ path: path.join(output, 'impl-codex-5h-usage-dark.png') });
    await iPage.keyboard.press('Escape');

    // Open Model popover, focus an option via ArrowDown, simulate SSE swaps of r-sline & r-ctx
    // (plus a live head-poll redraw), and verify popover and keyboard focus survive.
    const modelBtn = iPage.locator('#composer-pills [data-pop="model"]');
    await modelBtn.click();
    await iPage.locator('#fpop-portal .fpop-opt').first().waitFor();
    await iPage.keyboard.press('ArrowDown');
    await iPage.evaluate(() => {
      document.getElementById('r-sline').innerHTML = '';
      document.getElementById('r-ctx').dataset.h = 'swapped';
      window.dispatchEvent(new Event('resize'));
    });
    assert.equal(await iPage.locator('#fpop-portal').isVisible(), true, 'popover survives SSE swap');
    assert.equal(await modelBtn.getAttribute('aria-expanded'), 'true');
    assert.equal(await iPage.evaluate(() => document.activeElement && document.activeElement.classList.contains('fpop-opt')), true, 'focus survives resize/redraw');
    const cBox = await iPage.locator('#composer').boundingBox();
    const pBox = await iPage.locator('#fpop-portal').boundingBox();
    const topY = Math.max(0, Math.min(cBox.y, pBox.y) - 8);
    const botY = Math.max(cBox.y + cBox.height, pBox.y + pBox.height) + 8;
    await iPage.screenshot({ path: path.join(output, 'impl-popover-open-dark.png'), clip: { x: Math.max(0, cBox.x - 8), y: topY, width: cBox.width + 16, height: botY - topY } });
    await iPage.locator('#fpop-portal .fpop-opt[data-val="sonnet-4.6"]').click();
    await iPage.waitForFunction(() => document.querySelector('#composer-pills [data-pop="model"] .pv').textContent === 'sonnet-4.6');
    assert.deepEqual(await iPage.evaluate(() => window.__posts), [{ model: 'sonnet-4.6' }]);
    assert.deepEqual(errs, []);
    await iPage.close();
    await ref2.close();
    await ctx2.close();
    console.log('All Phase 2 Playwright checks passed; PNGs in ' + path.resolve(output));
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
