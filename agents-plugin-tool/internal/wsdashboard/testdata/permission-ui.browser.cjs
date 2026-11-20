// Run by TestPermissionUIBrowser against an isolated page fixture.
// argv: <base> <shots dir|""> <approved mockup dir|"">
// Only /support/choices and the viewed session's /agents routes are
// answered here (the hosted session's runner is not part of the fixture);
// the page HTML is the daemon's own, marked data-hosted as a hosted
// session's page is. Each view is checked, then captured in both themes at
// 1440 and 420, and set beside the approved mockup when one is given.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots, refs] = process.argv.slice(2);

const LEVELS = ['read-only', 'workdir-write', 'full'];
const MODES = ['auto', 'default', 'acceptEdits', 'plan', 'dontAsk', 'bypassPermissions'];
function choices(levels) {
  const extra = levels ? { permission_levels: LEVELS } : {};
  return { harnesses: [
    { harness: 'claude', lead: true, available: true, models: [{ id: 'opus-4.7' }, { id: 'sonnet-4.6' }], efforts: ['low', 'medium', 'high', 'max'], permission_modes: MODES, ...extra },
    { harness: 'pi', lead: true, available: true, models: [{ id: 'gpt-5.5' }], efforts: ['off', 'low', 'medium', 'high'], permission_modes: [], ...extra }
  ], last: {} };
}
const lead = extra => ({ id: 'livesession', lead: true, backend: 'claude', can_control: true, status: 'idle', model: 'opus-4.7', effort: 'high', held: 0, page: '/s/livesession', created_at: '2026-11-21T00:00:00Z', ...extra });
const STATES = {
  derived: lead({ process: true, permission_mode: 'auto', active_mode: 'auto', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' }),
  picked: lead({ process: true, permission_mode: 'plan', permission_level: 'read-only', permission_source: 'explicit', permission_enforcement: 'advisory' }),
  A: lead({ process: true, permission_mode: 'plan', active_mode: 'plan', permission_level: 'read-only', permission_source: 'explicit', permission_enforcement: 'partial' }),
  B: lead({ process: true, permission_mode: 'auto', active_mode: 'auto', permission_level: 'workdir-write', permission_source: 'legacy-derived', permission_enforcement: 'advisory' }),
  C: lead({ backend: 'pi', process: true, permission_level: 'workdir-write', permission_source: 'explicit', permission_enforcement: 'advisory' }),
  D: lead({ process: true, permission_mode: 'plan', active_mode: 'acceptEdits', permission_level: 'read-only', permission_source: 'explicit', permission_enforcement: 'advisory' }),
  E: lead({ process: false, permission_mode: 'bypassPermissions', permission_level: 'full', permission_source: 'explicit', permission_enforcement: 'advisory' }),
  chipFull: lead({ backend: 'pi', process: true, permission_level: 'full', permission_source: 'explicit', permission_enforcement: 'advisory' })
};
const SAVED = { harness: 'claude', harnesses: { claude: { model: 'opus-4.7', effort: 'high', mode: 'auto', iso: 'default' }, pi: { model: 'gpt-5.5', effort: 'medium', iso: 'isolated' } } };

function out(name) { return shots ? path.join(shots, name) : undefined; }

(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const failures = [];
  try {
    for (const theme of ['dark', 'light']) {
      for (const width of [1440, 420]) {
        if (process.env.PMUI_ONLY && process.env.PMUI_ONLY !== theme + ':' + width) continue;
        const S = { choices: choices(true), state: STATES.derived, posts: [], answer: () => ({ status: 404, body: 'not found' }) };
        const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: theme, locale: 'ko-KR', serviceWorkers: 'block' });
        await context.addInitScript(saved => {
          try { localStorage.setItem('ws.dashboard.newSession', JSON.stringify(saved)); } catch (_) {}
        }, SAVED);
        await context.route('**/*', async route => {
          const req = route.request(), url = new URL(req.url());
          if (url.origin !== base) return route.abort();
          if (url.pathname === '/rail/usage') return route.fulfill({ json: { entries: [], warnPercent: 80, critPercent: 95 } });
          if (url.pathname === '/support/choices') return route.fulfill({ json: S.choices, headers: { 'Cache-Control': 'no-store' } });
          if (url.pathname.startsWith('/agents/') || url.pathname === '/agents') {
            if (req.method() === 'GET' && url.pathname === '/agents/livesession') return route.fulfill({ json: S.state });
            if (req.method() === 'POST') {
              const body = req.postDataJSON();
              S.posts.push({ path: url.pathname, body });
              const a = S.answer(url.pathname, body);
              return a.json ? route.fulfill({ status: a.status, json: a.json }) : route.fulfill({ status: a.status, body: a.body });
            }
            return route.fulfill({ status: 404, body: 'not found' });
          }
          if (url.pathname === '/s/livesession' && req.resourceType() === 'document') {
            const r = await route.fetch();
            const html = (await r.text()).replace('<html ', '<html data-hosted="1" ');
            return route.fulfill({ response: r, body: html });
          }
          return route.continue();
        });
        const page = await context.newPage();
        const errors = [];
        // Sandboxed frames (web cards) cannot read navigator.serviceWorker under
        // the blocked-worker context; that is fixture noise, not this UI.
        page.on('pageerror', e => { if (!/serviceWorker/.test(e.message)) errors.push(e.stack || e.message); });
        const frame = () => page.frameLocator('.sframe.on');
        async function load(state, ch) {
          S.state = state; S.choices = ch || choices(true); S.posts = [];
          await page.goto(base + '/s/livesession');
          await frame().locator('#grid').waitFor();
          await page.locator('.dwrap > .top .pmhead').waitFor();
          await page.waitForTimeout(250);
        }
        async function railOpen() {
          if (await page.locator('#railbtn').isVisible() && !await page.locator('html').evaluate(el => el.classList.contains('rail-open'))) {
            await page.locator('#railbtn').click();
            await page.waitForTimeout(250);
          }
        }
        async function create() {
          await railOpen();
          if (!await page.locator('#rail .rnew').count()) {
            const dump = await page.evaluate(() => { const r = document.querySelector('#rail'); return (r ? r.outerHTML : 'no #rail').replace(/<svg[\s\S]*?<\/svg>/g, '').slice(0, 2500) + ' | sysopen=' + document.documentElement.hasAttribute('data-sysopen'); });
            throw new Error('no new-session button: ' + dump);
          }
          await page.locator('#rail .rnew').first().click();
          const pop = page.locator('.rpop:not([hidden])').filter({ has: page.locator('.pmsec') });
          await pop.waitFor();
          await page.waitForTimeout(200);
          return pop;
        }
        async function menu() {
          await page.locator('.dwrap > .top .pmhead').click();
          const pop = frame().locator('.rpop:not([hidden])').filter({ has: frame().locator('.pmstate') });
          await pop.waitFor();
          await page.waitForTimeout(200);
          return pop;
        }
        async function closePop() { await page.keyboard.press('Escape'); await page.waitForTimeout(150); }
        const name = n => `${n}-${theme}-${width}.png`;
        const step = async (label, fn) => {
          try { await fn(); } catch (e) { const m = `${theme}/${width} ${label}: ${e.message.split('\n').slice(0, 6).join(' | ')}`; failures.push(m); console.error(m); }
        };

        // create: default first and checked; the native mode under Advanced.
        await step('create', async () => {
          await load(STATES.derived);
          const pop = await create();
          assert.equal(await pop.locator('.pmseg button[aria-checked="true"]').textContent(), '기본(기존 동작)');
          assert.equal(await pop.locator('.pmseg button').count(), 4);
          assert.equal(await pop.locator('.pmadv').evaluate(d => d.open), false, 'Advanced starts collapsed');
          assert.equal(await pop.locator('.pmadv select').inputValue(), 'auto');
          await page.screenshot({ path: out(name('create')) });
          if (width === 1440) {
            await pop.locator('.spseg [data-harness="pi"]').click();
            await pop.locator('.pmseg [data-level="workdir-write"]').click();
            assert.equal(await pop.locator('.pmadv').isVisible(), false, 'Pi has no native disclosure');
            await page.screenshot({ path: out(name('create-pi')) });
          }
          await closePop();
        });

        // full-warning: Start opens the warning, unticked, grant disabled.
        await step('full-warning', async () => {
          const pop = await create();
          await pop.locator('.pmseg [data-level="full"]').click();
          assert.equal(await pop.locator('.pmadv select').inputValue(), 'bypassPermissions');
          await pop.locator('.sprow .pbtn.go').click();
          const dlg = page.locator('dialog.pmdlg[open]');
          await dlg.waitFor();
          assert.equal(await dlg.locator('.pbtn.danger').isDisabled(), true);
          assert.equal(await dlg.locator('input[type=checkbox]').isChecked(), false);
          assert.equal(S.posts.length, 0, 'opening the warning sends nothing');
          await page.waitForTimeout(150);
          await page.screenshot({ path: out(name('full-warning')) });
          // Cancel keeps the popover and its full pick, and sends nothing.
          await dlg.locator('.rdrow .pbtn:not(.danger)').click();
          assert.equal(await pop.isVisible(), true, 'cancel keeps the popover');
          assert.equal(await pop.locator('.pmseg button[aria-checked="true"]').getAttribute('data-level'), 'full');
          // Confirm: tick, grant; the request carries the acknowledgement.
          S.answer = () => ({ status: 500, body: 'fixture: no runner' });
          await pop.locator('.sprow .pbtn.go').click();
          await dlg.locator('label.pmack').click();
          await dlg.locator('.pbtn.danger').click();
          await page.waitForFunction(() => true);
          await page.waitForTimeout(400);
          assert.deepEqual(S.posts.map(p => [p.path, p.body.permission_level, p.body.permission_full_ack, 'permission_mode' in p.body]), [['/agents/leads/permission', 'full', true, false]]);
          await closePop();
        });

        // settings (legacy-derived) and settings-picked (explicit read-only).
        await step('settings', async () => {
          await load(STATES.derived);
          const pop = await menu();
          assert.equal(await pop.locator('.pmseg button[aria-checked="true"]').count(), 0);
          assert.equal(await pop.locator('.pmseg button.derived').getAttribute('data-level'), 'workdir-write');
          assert.equal(await pop.locator('.pmadv select').isDisabled(), false);
          await page.screenshot({ path: out(name('settings')) });
          await closePop();
          await load(STATES.picked);
          const p2 = await menu();
          assert.equal(await p2.locator('.pmseg button[aria-checked="true"]').getAttribute('data-level'), 'read-only');
          assert.equal(await p2.locator('.pmadv select').isDisabled(), true);
          assert.equal(await p2.locator('.pmstate .pmq').textContent(), '관측 대기');
          await page.screenshot({ path: out(name('settings-picked')) });
          await closePop();
        });

        // state: the five cases' state blocks and three head chips.
        const tiles = [];
        await step('state', async () => {
          for (const k of ['A', 'B', 'C', 'D', 'E']) {
            await load(STATES[k]);
            const pop = await menu();
            tiles.push({ k, png: await pop.locator('.pmstate').screenshot() });
            await closePop();
          }
          for (const k of ['A', 'B', 'chipFull']) {
            await load(STATES[k]);
            tiles.push({ k: 'chip-' + k, png: await page.locator('.dwrap > .top .tph-hs').screenshot() });
          }
        });

        // unsupported: no levels; refused (404) then re-check; menu without levels.
        const utiles = [];
        await step('unsupported', async () => {
          await load(STATES.derived, choices(false));
          let pop = await create();
          assert.equal(await pop.locator('.pmseg').isVisible(), false, 'no level without positive support');
          assert.equal(await pop.locator('.pmoff').isVisible(), true);
          utiles.push({ k: 'A', png: await pop.screenshot() });
          await closePop();
          await load(STATES.derived, choices(true));
          pop = await create();
          await pop.locator('.pmseg [data-level="read-only"]').click();
          S.answer = () => ({ status: 404, body: '404 page not found' });
          await pop.locator('.sprow .pbtn.go').click();
          await pop.locator('.pmerr:not([hidden])').waitFor();
          assert.deepEqual(S.posts.map(p => p.path), ['/agents/leads/permission'], 'a refusal is never resent on the legacy route');
          utiles.push({ k: 'B', png: await pop.screenshot() });
          S.choices = choices(false);
          await pop.locator('.sprow .pbtn.go').click();
          await pop.locator('.pmoff:not([hidden])').waitFor();
          assert.equal(S.posts.length, 1, 're-check sends nothing');
          await pop.locator('select').filter({ has: page.locator('option[value="plan"]') }).first().selectOption('plan');
          utiles.push({ k: 'B2', png: await pop.screenshot() });
          await closePop();
          await load(STATES.derived, choices(false));
          pop = await menu();
          assert.equal(await pop.locator('.pmchg').isVisible(), false);
          utiles.push({ k: 'C', png: await pop.screenshot() });
          await closePop();
        });

        if (errors.length) { const m = `${theme}/${width} page errors:\n${[...new Set(errors)].join('\n')}`; failures.push(m); console.error(m); }
        // Galleries in the mockup's arrangement, from the captured parts.
        const gal = async (file, title, items) => {
          if (!shots || !items.length) return;
          const g = await context.newPage();
          const bg = theme === 'dark' ? '#141518' : '#f3f3f0', fg = theme === 'dark' ? '#e7e7e4' : '#1b1d21';
          await g.setContent(`<body style="margin:0;padding:22px;background:${bg};color:${fg};font:13px system-ui"><div style="text-align:center;font-weight:800;font-size:14px;margin-bottom:14px">${title}</div><div style="display:flex;flex-wrap:wrap;gap:18px;justify-content:center;align-items:flex-start">${items.map(t =>
            `<div style="width:330px;max-width:100%;display:grid;gap:6px"><div style="font-size:12px;font-weight:800">${t.cap}</div><img style="max-width:100%" src="data:image/png;base64,${t.png.toString('base64')}"></div>`).join('')}</div></body>`);
          await g.setViewportSize({ width, height: 900 });
          await g.screenshot({ path: out(file), fullPage: true });
          await g.close();
        };
        const CAP = { A: 'A. Claude, 명시적 읽기 전용', B: 'B. Claude, 선택 없음 (기존 세션)', C: 'C. Pi, 명시적 작업 폴더 쓰기', D: 'D. Claude, 요청과 다른 모드 보고', E: 'E. Claude, 아직 관측 없음', 'chip-A': '헤드 칩: 읽기 전용', 'chip-B': '헤드 칩: 추정', 'chip-chipFull': '헤드 칩: 전체' };
        await gal(name('state'), '세션 권한 상태: 의도 · 네이티브 모드 · 근거', tiles.map(t => ({ ...t, cap: CAP[t.k] })));
        const UCAP = { A: 'A. 이 데몬이 권한 수준을 알리지 않음', B: 'B. 선택이 거부됨 (404)', B2: 'B2. 다시 확인해도 지원 없음', C: 'C. 세션 메뉴, 지원 없음' };
        await gal(name('unsupported'), '지원 없음 · 거부됨 (기존 경로로 대체하지 않음)', utiles.map(t => ({ ...t, cap: UCAP[t.k] })));
        await context.close();
      }
    }
    // Side by side with the approved mockup: mockup left, capture right.
    if (shots && refs) {
      const ctx = await browser.newContext({ viewport: { width: 2000, height: 1000 } });
      const p = await ctx.newPage();
      for (const file of fs.readdirSync(shots).filter(f => /\.png$/.test(f) && !f.startsWith('compare-')).sort()) {
        const ref = path.join(refs, file);
        if (!fs.existsSync(ref)) continue;
        const b64 = f => fs.readFileSync(f).toString('base64');
        await p.setContent(`<body style="margin:0;padding:12px;background:#808080;font:12px system-ui"><div id="c" style="display:inline-flex;gap:16px;align-items:flex-start"><div><div style="font-weight:700;margin-bottom:4px">approved mockup: ${file}</div><img src="data:image/png;base64,${b64(ref)}"></div><div><div style="font-weight:700;margin-bottom:4px">implementation: ${file}</div><img src="data:image/png;base64,${b64(path.join(shots, file))}"></div></div></body>`);
        await p.locator('#c').screenshot({ path: path.join(shots, 'compare-' + file) });
      }
      await ctx.close();
    }
  } finally {
    await browser.close();
  }
  if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
  console.log('permission UI captures done');
})().catch(e => { console.error(e); process.exit(1); });
