// Run by TestRailNavigationBrowser over isolated fixture sessions.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const { destination } = require('../../../desktop-electron/policy');
const preload = fs.readFileSync(path.join(__dirname, '../../../desktop-electron/dashboard-preload.js'), 'utf8');
const [base, shots] = process.argv.slice(2);
const on = '.sframe.on';
const editor = '#ctext + .wsc-editor';

async function show(page, pathname) {
  await page.waitForFunction(p => {
    const frame = document.querySelector('.sframe.on');
    return location.pathname === p && frame && frame.contentWindow.location.pathname === p;
  }, pathname);
  await page.frameLocator(on).locator('#grid').waitFor();
}
async function openRail(page) {
  if (!await page.locator('html').evaluate(el => el.classList.contains('rail-open'))) await page.locator('#railbtn').click();
  await page.locator('#gear').waitFor({ state: 'visible' });
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const desktop of [false, true]) {
      for (const theme of ['light', 'dark']) {
        const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme, locale: 'ko-KR', reducedMotion: 'reduce' });
        context.setDefaultTimeout(10000);
        // Pin the server-rendered language: routed Chromium requests may omit
        // Accept-Language even when the browser context has a locale.
        await context.addCookies([{ name: 'ws_dashboard_lang', value: desktop ? 'ko' : 'en', url: base }]);
        await context.addInitScript(({ desktop, preload }) => {
          if (self.origin === 'null') return;
          const workers = navigator.serviceWorker;
          if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('Service workers forbidden in rail navigation fixture'); };
          if (desktop && self === top) {
            new Function('require', preload)(name => {
              if (name !== 'electron') throw new Error('Unexpected preload dependency');
              return {
                contextBridge: { exposeInMainWorld: (key, value) => { window[key] = value; } },
                ipcRenderer: { send() {}, on() {} }
              };
            });
          }
        }, { desktop, preload });
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
        await show(page, '/s/livesession');
        await page.frameLocator(on).locator(editor).waitFor();
        await openRail(page);
        assert.equal(await page.locator('html').getAttribute('data-lang'), desktop ? 'ko' : 'en');
        const label = desktop ? '홈' : 'Home';
        const home = page.getByRole('button', { name: label, exact: true });
        await home.waitFor({ state: 'visible' });
        assert.equal(await home.count(), 1);
        assert.equal(await home.getAttribute('title'), label);
        assert.equal(await home.evaluate(el => el.previousElementSibling.id), 'gear');
        assert.equal(await home.locator('use').getAttribute('href'), '#l-house');
        const gearBox = await page.locator('#gear').boundingBox(), homeBox = await home.boundingBox();
        assert.equal(homeBox.width, gearBox.width, 'Home has Settings width');
        assert.equal(homeBox.height, gearBox.height, 'Home has Settings height');
        assert.ok(homeBox.x >= gearBox.x + gearBox.width, 'Home is immediately right of Settings');
        assert.equal(await page.locator('.rfoot').evaluate(el => el.scrollWidth <= el.clientWidth), true);
        if (shots && !desktop) {
          fs.mkdirSync(shots, { recursive: true });
          await page.waitForTimeout(400);
          await page.screenshot({ path: path.join(shots, `after-${theme}-1280.png`) });
          await page.locator('.rfoot').screenshot({ path: path.join(shots, `after-${theme}-footer.png`) });
        }
        if (desktop) {
          assert.equal(await page.evaluate(() => Object.isFrozen(window.werkschaftDesktop.update)), true);
          for (const suffix of ['/', '/?frame=1', '/s/livesession', '/s/livesession?frame=1']) {
            assert.equal(destination(base + suffix, base, true), 'dashboard');
            assert.equal(destination(base + suffix, base, false), 'dashboard');
          }
        }
        await page.frameLocator(on).locator(editor).fill('Keep this draft on reselect');
        await page.frameLocator(on).locator('body').evaluate(() => { window.railNavigationMarker = 42; });
        const history = await page.evaluate(() => window.history.length);
        const selected = page.locator('.rs[data-rid="livesession"] .rl');
        await selected.click();
        await selected.click();
        await selected.dblclick();
        await selected.focus();
        await page.keyboard.press('Enter');
        await show(page, '/s/livesession');
        assert.equal(await page.locator('html').getAttribute('data-rid'), 'livesession');
        assert.equal(await page.locator('.rs.cur').getAttribute('data-rid'), 'livesession');
        assert.equal(await page.locator('html').evaluate(el => el.classList.contains('rail-open')), true, 'reselect leaves the rail open');
        assert.equal(await page.frameLocator(on).locator('body').evaluate(() => window.railNavigationMarker), 42, 'reselect does not reload or replace the frame');
        assert.equal(await page.frameLocator(on).locator('#ctext').inputValue(), 'Keep this draft on reselect');
        assert.equal(await page.evaluate(() => window.history.length), history, 'reselect adds no history');
        await home.click();
        await show(page, '/');
        await page.frameLocator(on).locator('.dlanding').waitFor();
        assert.equal(await page.locator('html').getAttribute('data-rid'), '');
        assert.equal(await page.locator('.rs.cur').count(), 0);
        await openRail(page);
        const homeHistory = await page.evaluate(() => window.history.length);
        await home.click();
        await show(page, '/');
        assert.equal(await page.evaluate(() => window.history.length), homeHistory, 'Home on home adds no history');
        await openRail(page);
        await selected.click();
        await show(page, '/s/livesession');
        await page.frameLocator(on).locator(editor).waitFor();
        assert.equal(await page.frameLocator(on).locator('#ctext').inputValue(), 'Keep this draft on reselect', 'Home preserves a session draft');
        await openRail(page);
        await page.locator('.rs[data-rid="boardsession"] .rl').click();
        await show(page, '/s/boardsession');
        await openRail(page);
        await page.keyboard.press('Escape');
        assert.equal(await page.locator('html').evaluate(el => el.classList.contains('rail-open')), false, 'Escape still closes the rail');
        await show(page, '/s/boardsession');
        await openRail(page);
        await home.focus();
        await page.keyboard.press('Enter');
        await show(page, '/');
        await page.frameLocator(on).locator('.dlanding').waitFor();
        assert.deepEqual(errors, []);
        assert.deepEqual(context.serviceWorkers(), []);
        await context.close();
        console.log(`${desktop ? 'Desktop preload' : 'Browser'} ${theme}: Home placement/labels/navigation/drafts, repeated click/double-click/Enter no-op, other session selection and Escape passed.`);
      }
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
