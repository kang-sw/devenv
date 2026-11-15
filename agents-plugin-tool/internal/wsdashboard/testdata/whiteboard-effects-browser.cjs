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
      const context = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US', serviceWorkers: 'allow', reducedMotion: 'reduce' });
      context.setDefaultTimeout(10000);
      const errors = [], consoleErrors = [], unexpected = [], history = [], requests = [];
      const serviceWorkerAttempts = [], serviceWorkerEvents = [];
      await context.exposeBinding('__wsEffectsServiceWorkerAttempt', ({ frame }, scriptURL) => {
        serviceWorkerAttempts.push({ frame: frame.url(), scriptURL });
      });
      context.on('serviceworker', worker => serviceWorkerEvents.push(worker.url()));
      // Playwright's blocker reads Navigator even in opaque sandboxed frames.
      // Keep their native restriction and block registrations in other frames.
      await context.addInitScript(() => {
        if (self.origin === 'null') return;
        const serviceWorkers = navigator.serviceWorker;
        if (!serviceWorkers) return;
        Object.getPrototypeOf(serviceWorkers).register = async function(scriptURL) {
          await globalThis.__wsEffectsServiceWorkerAttempt(typeof scriptURL === 'string' ? scriptURL : { type: typeof scriptURL });
          throw new Error('Service worker registration is forbidden in the effects fixture');
        };
      });
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
      async function effectsKeys() {
        return page.locator('.wbeffects[data-entry]').evaluateAll(nodes => nodes.map(e => e.getAttribute('data-entry')));
      }
      async function arrived(update, beforeKeys, expand) {
        assert(update.key && !beforeKeys.includes(update.key), 'append returns a genuinely new effects ring key');
        const fresh = page.locator(`.wbeffects[data-entry="${update.key}"]`);
        await fresh.waitFor({ state: 'attached' });
        assert.equal(await fresh.count(), 1, 'new carrier renders as its own update card');
        assert.equal(await fresh.locator('.wbeffect-values').count(), 1, 'log diagnostics remain inspectable; panel payload is not imported');
        assert(!(await fresh.textContent()).includes('PANEL-PAYLOAD-ONLY'), 'panel payload stays on the board');
        assert.equal(await fresh.locator('.wbeffect-fallback, script, img, iframe, strong').count(), 0);
        assert((await fresh.locator('pre').allTextContents()).includes(update.payload), 'newly arriving hostile log remains exact literal text');
        assert.equal(await page.evaluate(() => typeof window.effectsAttack), 'undefined', 'new effects payload never executes');
        if (expand) {
          await fresh.locator('.wbeffect-details > summary').click();
          assert.equal(await fresh.locator('.wbeffect-details').evaluate(e => e.open), true, 'change diagnostics expand independently of the card');
        }
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
        const effect = () => page.locator(`.wbeffects[data-entry="${keys.focus}"]`);
        const fallback = () => page.locator(`.wbeffects[data-entry="${keys.fallback}"]`);
        await effect().waitFor({ state: 'attached' });
        await effect().locator('.wbeffect-details > summary').click();
        await effect().evaluate(e => { e.effectsBrowserIdentity = 'original-effect-node'; });
        assert.equal(await effect().locator('.wbeffect-details').evaluate(e => e.open), true);
        const values = await effect().locator('pre').allTextContents();
        assert(values.some(s => s.includes('<img src=x onerror="window.effectsAttack=1">') && s.includes('**plain text**')), 'valid log stays literal, including HTML and Markdown syntax');
        assert.equal(await effect().locator('script, img, iframe, strong').count(), 0, 'effects never execute/render panel HTML or Markdown');
        assert.equal(await page.evaluate(() => typeof window.effectsAttack), 'undefined');
        assert((await effect().textContent()).includes('Panel recap only'), 'panel has a recap');
        assert(!(await effect().textContent()).includes('PANEL-PAYLOAD-ONLY'), 'panel body has no in-chat expansion');
        assert((await fallback().textContent()).includes('Details unavailable or unsupported'));
        assert((await fallback().locator('.wbeffect-fallback').textContent()).includes('"v":99'));
        assert.equal(await fallback().locator('img, script, iframe, .wbeffect-values').count(), 0, 'unsupported envelope is escaped fallback, not invented effects');
        await shot('saved-escaped-and-fallback');

        const savedKeys = await effectsKeys();
        const refresh = await action('append');
        await page.getByText(refresh.marker, { exact: true }).waitFor({ state: 'attached' });
        await arrived(refresh, savedKeys, true);
        assert.equal(await effect().evaluate(e => e.querySelector('.wbeffect-details').open && e.effectsBrowserIdentity === 'original-effect-node'), true, 'real stream refresh preserves expanded diagnostics and node');
        assert.equal(await page.getByText('old-history-marker', { exact: true }).count(), 0, 'old marker is outside initial ring window');
        const older = tab.waitForResponse(r => new URL(r.url()).pathname === '/s/effectsession/history' && r.status() === 200);
        await page.locator('#convo').evaluate(e => { e.scrollTop = 0; e.dispatchEvent(new Event('scroll')); });
        const historyResponse = await older;
        const historyBody = await historyResponse.json();
        assert(historyBody.html.includes('old-history-marker'), 'production history handler supplies older records');
        await page.getByText('old-history-marker', { exact: true }).waitFor({ state: 'attached' });
        assert(history.length > 0, 'real older-history paging occurred');
        assert.equal(await effect().evaluate(e => e.querySelector('.wbeffect-details').open && e.effectsBrowserIdentity === 'original-effect-node'), true, 'paging retains diagnostics expansion and existing entry identity');
        await effect().scrollIntoViewIfNeeded();
        await shot('expanded-after-refresh-and-history');

        await action('live');
        await active('live');
        await page.locator('#ctext').waitFor();
        const draft = `Preserve the effects draft at ${width}px`;
        await page.locator('#ctext').fill(draft);
        await waitStored(draft);
        const liveKeys = await effectsKeys();
        const liveRefresh = await action('append');
        await page.getByText(liveRefresh.marker, { exact: true }).waitFor({ state: 'attached' });
        await arrived(liveRefresh, liveKeys, true);
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
        const target = page.locator('#p1');
        async function jumpGeometry() {
          const geometry = await target.evaluate(e => {
            const r = e.getBoundingClientRect();
            const board = e.closest('#bcol, #bpins');
            const b = board.getBoundingClientRect();
            const clip = { left: b.left + board.clientLeft, top: b.top + board.clientTop };
            clip.right = clip.left + board.clientWidth;
            clip.bottom = clip.top + board.clientHeight;
            const css = getComputedStyle(e);
            return {
              open: e.open, rendered: r.width > 0 && r.height > 0 && css.display !== 'none' && !['hidden', 'collapse'].includes(css.visibility),
              target: { left: r.left, top: r.top, right: r.right, bottom: r.bottom }, clip,
              offscreen: r.bottom <= clip.top || r.top >= clip.bottom || r.right <= clip.left || r.left >= clip.right,
              inBoard: r.left >= clip.left && r.right <= clip.right && r.top >= clip.top && r.bottom <= clip.bottom
            };
          });
          const bounds = await target.boundingBox();
          const viewport = tab.viewportSize();
          return { ...geometry, bounds, viewport, inBrowser: !!bounds && bounds.x >= 0 && bounds.y >= 0 && bounds.x + bounds.width <= viewport.width && bounds.y + bounds.height <= viewport.height };
        }
        const preJump = await jumpGeometry();
        fs.writeFileSync(path.join(output, `${width}-jump-before.json`), JSON.stringify(preJump, null, 2));
        assert(preJump.rendered && preJump.offscreen, 'jump target starts outside the board clipping viewport');
        await body.getByRole('link', { name: 'Jump to target', exact: true }).click();
        let postJump = null;
        try {
          await until(async () => {
            postJump = await jumpGeometry();
            return postJump.open && postJump.rendered && postJump.inBoard && postJump.inBrowser;
          }, 'production jump opens and reveals target inside the board and browser viewports');
        } finally {
          fs.writeFileSync(path.join(output, `${width}-jump-after.json`), JSON.stringify(postJump, null, 2));
        }
        await target.waitFor({ state: 'visible' });
        const before = await target.boundingBox();
        assert(before && before.y >= 0 && before.y < 900, 'production jump brings target into view');
        const jumpedKeys = await effectsKeys();
        const jumpedRefresh = await action('append');
        await page.getByText(jumpedRefresh.marker, { exact: true }).waitFor({ state: 'attached' });
        await arrived(jumpedRefresh, jumpedKeys, false);
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
        assert.deepEqual(serviceWorkerAttempts, [], 'no service worker registration attempts');
        assert.deepEqual(serviceWorkerEvents, [], 'no service worker creations');
        assert.deepEqual(context.serviceWorkers(), [], 'no remaining service workers');
        assert.deepEqual(errors, [], 'no production page errors');
        assert.deepEqual(consoleErrors, [], 'no browser console errors');
        assert.deepEqual(unexpected, [], 'no external network access');
        console.log(`PASS ${width}px: real escaped effects/fallback, refresh/history expansion, no/stale/live adapter transitions, saved-history controls, drafts, board jump-follow, modal completion/generation`);
      } catch (e) {
        try { await shot('failure'); } catch (_) {}
        throw e;
      } finally {
        fs.writeFileSync(path.join(output, `${width}-service-workers.json`), JSON.stringify({ attempts: serviceWorkerAttempts, events: serviceWorkerEvents, remaining: context.serviceWorkers().map(worker => worker.url()) }, null, 2));
        fs.writeFileSync(path.join(output, `${width}-console-and-requests.json`), JSON.stringify({ errors, consoleErrors, unexpected, history, requests }, null, 2));
        await context.close();
      }
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
