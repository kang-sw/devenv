// Run only through TestWhiteboardEffectsBrowser after an explicit slot START.
// The Go fixture serves production pages/assets/history/streams on port 0.
// Only store mutations and the provider-free agent-host boundary are synthetic.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, control, output] = process.argv.slice(2);
assert(base && control && output, 'pass disposable page/control URLs and retained artifacts');
for (const address of [base, control]) {
  const u = new URL(address);
  assert(['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname), 'fixture is loopback only');
  assert(u.port && u.port !== '29417', 'never access the installed/shared daemon');
}

async function action(name) {
  const res = await fetch(control + '?action=' + encodeURIComponent(name), { method: 'POST' });
  assert.equal(res.status, 200, 'fixture action ' + name);
  return res.json();
}
async function until(check, message) {
  const end = Date.now() + 10000;
  while (Date.now() < end) {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, 40));
  }
  assert.fail(message);
}

(async () => {
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of [1280, 360]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US', serviceWorkers: 'block', reducedMotion: 'reduce' });
      context.setDefaultTimeout(10000);
      const errors = [], consoleErrors = [], unexpected = [], history = [], requests = [];
      const tab = await context.newPage();
      tab.on('pageerror', e => errors.push(e.message));
      tab.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text()); });
      tab.on('request', r => {
        requests.push({ method: r.method(), url: r.url() });
        if (new URL(r.url()).pathname === '/s/effectsession/history') history.push(r.url());
      });
      await context.route('**/*', route => {
        const u = new URL(route.request().url());
        if (u.pathname === '/favicon.ico' && u.origin === new URL(base).origin) return route.fulfill({ status: 204 });
        if (['http:', 'https:'].includes(u.protocol) && ![new URL(base).origin, new URL(control).origin].includes(u.origin)) {
          unexpected.push(u.href);
          return route.abort();
        }
        return route.continue();
      });
      let page;
      async function active(mode) {
        await until(async () => {
          const el = tab.locator('iframe.sframe.on');
          if (!(await el.count())) return false;
          const handle = await el.elementHandle();
          page = handle && await handle.contentFrame();
          return page && (!mode || await page.locator('html').getAttribute('data-mode') === mode);
        }, 'active production session frame reaches ' + mode);
      }
      async function shot(name) {
        await tab.screenshot({ path: path.join(output, `${width}-${name}.png`), fullPage: true });
      }
      async function showTab(name) {
        const pane = page.locator(name === 'board' ? '#bcol' : '#convo');
        if (!(await pane.isVisible())) await page.locator(`[data-tab="${name}"]`).first().click();
        await pane.waitFor({ state: 'visible' });
      }
      async function savedOnly() {
        assert.equal(await page.locator('html').getAttribute('data-mode'), 'history');
        assert.equal(await page.locator('#composer, #ctext, #apbox, #r-tree, #r-convlive, #r-ctx, #r-cmds, #r-sline').count(), 0, 'saved history contains no fabricated live controls/activity');
      }
      async function waitStored(draft) {
        await page.waitForFunction(text => Object.keys(localStorage).some(k => (localStorage.getItem(k) || '').includes(text)), draft);
      }
      async function openModal() {
        if (await tab.locator('#railbtn').isVisible() && !(await tab.locator('html').evaluate(e => e.classList.contains('rail-open')))) await tab.locator('#railbtn').click();
        await tab.locator('.rwt .wthd').first().hover();
        await tab.locator('.rnew').first().click();
        const modal = tab.getByRole('dialog', { name: 'New session', exact: true });
        await modal.waitFor({ state: 'visible' });
        const rect = await modal.boundingBox();
        assert(rect && rect.x >= 0 && rect.x + rect.width <= width + 1 && rect.y >= 0 && rect.y + rect.height <= 901, 'production new-session modal fits viewport');
        return modal;
      }
      try {
        const keys = await action('reset');
        await tab.goto(base + '/s/effectsession');
        await active('history');
        await savedOnly();
        await showTab('convo');
        const effect = () => page.locator(`details.wbeffects[data-entry="${keys.focus}"]`);
        const fallback = () => page.locator(`details.wbeffects[data-entry="${keys.fallback}"]`);
        await effect().waitFor({ state: 'attached' });
        await effect().locator('summary').click();
        await effect().evaluate(e => { e.effectsBrowserIdentity = 'original-effect-node'; });
        assert.equal(await effect().evaluate(e => e.open), true);
        const values = await effect().locator('pre').allTextContents();
        assert(values.some(s => s.includes('<img src=x onerror="window.effectsAttack=1">') && s.includes('**plain text**')), 'valid payload stays literal, including HTML and Markdown syntax');
        assert.equal(await effect().locator('script, img, iframe, strong').count(), 0, 'effects never execute/render panel HTML or Markdown');
        assert.equal(await page.evaluate(() => typeof window.effectsAttack), 'undefined');
        await fallback().locator('summary').click();
        assert((await fallback().textContent()).includes('Details unavailable or unsupported'));
        assert((await fallback().locator('.wbeffect-fallback').textContent()).includes('"v":99'));
        assert.equal(await fallback().locator('img, script, iframe, .wbeffect-values').count(), 0, 'unsupported envelope is escaped fallback, not invented effects');
        await shot('saved-escaped-and-fallback');

        const refresh = await action('append');
        await page.getByText(refresh.marker, { exact: true }).waitFor({ state: 'attached' });
        assert.equal(await effect().evaluate(e => e.open && e.effectsBrowserIdentity === 'original-effect-node'), true, 'real stream refresh preserves expanded node');
        assert.equal(await page.getByText('old-history-marker', { exact: true }).count(), 0, 'old marker is outside initial ring window');
        const older = tab.waitForResponse(r => new URL(r.url()).pathname === '/s/effectsession/history' && r.status() === 200);
        await page.locator('#convo').evaluate(e => { e.scrollTop = 0; e.dispatchEvent(new Event('scroll')); });
        const historyResponse = await older;
        const historyBody = await historyResponse.json();
        assert(historyBody.html.includes('old-history-marker'), 'production history handler supplies older records');
        await page.getByText('old-history-marker', { exact: true }).waitFor({ state: 'attached' });
        assert(history.length > 0, 'real older-history paging occurred');
        assert.equal(await effect().evaluate(e => e.open && e.effectsBrowserIdentity === 'original-effect-node'), true, 'paging retains expansion and existing entry identity');
        await effect().scrollIntoViewIfNeeded();
        await shot('expanded-after-refresh-and-history');

        await action('live');
        await active('live');
        await page.locator('#ctext').waitFor();
        const draft = `Preserve the effects draft at ${width}px`;
        await page.locator('#ctext').fill(draft);
        await waitStored(draft);
        const liveRefresh = await action('append');
        await page.getByText(liveRefresh.marker, { exact: true }).waitFor({ state: 'attached' });
        assert.equal(await page.locator('#ctext').inputValue(), draft, 'effects refresh never changes composer input');
        await action('stale');
        await page.locator('#dash-banner').waitFor({ state: 'visible' });
        assert((await page.locator('#dash-banner').textContent()).includes('went away'));
        assert.equal(await page.locator('#ctext').inputValue(), draft, 'adapter loss banners instead of dropping an in-memory draft');
        await tab.reload();
        await active('history');
        await savedOnly();
        await tab.reload();
        await active('history');
        await savedOnly();
        await action('live');
        await active('live');
        await page.locator('#ctext').waitFor();
        assert.equal(await page.locator('#ctext').inputValue(), draft, 'saved-history reloads do not overwrite stored composer draft');
        await action('absent');
        await page.locator('#dash-banner').waitFor({ state: 'visible' });
        assert.equal(await page.locator('#ctext').inputValue(), draft, 'no-adapter loss also preserves input');
        await tab.reload();
        await active('history');
        await savedOnly();
        await action('live');
        await active('live');
        await page.locator('#ctext').waitFor();
        assert.equal(await page.locator('#ctext').inputValue(), draft);
        await shot('live-restored-draft');

        // Follow a real link in the sandboxed, production-rendered board.
        // A conversation update must not drag the jumped-to board target.
        await showTab('board');
        await page.locator('section.topic[data-topic="navigation"]').scrollIntoViewIfNeeded();
        const bodyFrame = page.locator('section.topic[data-topic="navigation"] iframe.doc-frame').first();
        await bodyFrame.waitFor();
        const body = await (await bodyFrame.elementHandle()).contentFrame();
        await body.getByRole('link', { name: 'Jump to target', exact: true }).click();
        await page.waitForFunction(() => location.hash === '#p1' && document.querySelector('#p1') && document.querySelector('#p1').open);
        const target = page.locator('#p1');
        await target.waitFor({ state: 'visible' });
        const before = await target.boundingBox();
        assert(before && before.y >= 0 && before.y < 900, 'production jump brings target into view');
        const jumpedRefresh = await action('append');
        await page.getByText(jumpedRefresh.marker, { exact: true }).waitFor({ state: 'attached' });
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        const after = await target.boundingBox();
        assert(after && Math.abs(after.y - before.y) <= 16, 'effects refresh preserves the followed board target');
        assert.equal(await page.locator('#ctext').inputValue(), draft);
        await shot('jump-follow-after-effects-refresh');

        // Modal completion runs from the real HTTP response, not an SSE
        // attachment. Held synthetic responses make busy/generation observable.
        const initialRequests = (await action('requests')).count;
        let modal = await openModal();
        const start = modal.getByRole('button', { name: 'Start', exact: true });
        await start.click();
        await until(async () => (await action('requests')).count === initialRequests + 1, 'one creation request reaches synthetic host');
        assert.equal(await start.isDisabled(), true);
        assert((await modal.textContent()).includes('Starting…'));
        await start.dispatchEvent('click');
        assert.equal((await action('requests')).count, initialRequests + 1, 'busy blocks duplicate creation');
        await shot('modal-pending');
        await action('complete');
        await tab.waitForURL(base + '/s/createdsession');
        await active('board');
        assert.equal(await modal.isVisible(), false, 'HTTP 201 completes and closes active modal without attach events');
        await tab.goto(base + '/s/effectsession');
        await active('live');
        assert.equal(await page.locator('#ctext').inputValue(), draft, 'modal completion saves the originating session draft');
        modal = await openModal();
        await modal.getByRole('button', { name: 'Start', exact: true }).click();
        await until(async () => (await action('requests')).count === initialRequests + 2, 'second held creation request arrives');
        await modal.getByRole('button', { name: 'Cancel', exact: true }).click();
        modal = await openModal();
        await action('complete');
        await tab.waitForURL(base + '/s/createdsession');
        await active('board');
        assert.equal(await modal.isVisible(), true, 'late prior-generation success never closes a newer modal');
        assert.equal(await modal.getByRole('button', { name: 'Start', exact: true }).isDisabled(), false);
        await modal.getByRole('button', { name: 'Cancel', exact: true }).click();
        await shot('modal-generation-completed');
        assert.deepEqual(errors, [], 'no production page errors');
        assert.deepEqual(consoleErrors, [], 'no browser console errors');
        assert.deepEqual(unexpected, [], 'no external network access');
        console.log(`PASS ${width}px: real escaped effects/fallback, refresh/history expansion, no/stale/live adapter transitions, saved-history controls, drafts, board jump-follow, modal completion/generation`);
      } catch (e) {
        try { await shot('failure'); } catch (_) {}
        throw e;
      } finally {
        fs.writeFileSync(path.join(output, `${width}-console-and-requests.json`), JSON.stringify({ errors, consoleErrors, unexpected, history, requests }, null, 2));
        await context.close();
      }
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
