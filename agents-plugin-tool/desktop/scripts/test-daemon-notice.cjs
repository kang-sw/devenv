'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const { pathToFileURL } = require('node:url');

const desktop = path.resolve(__dirname, '..');
const assets = path.resolve(desktop, '../internal/wsdashboard/assets');
const notice = fs.readFileSync(path.join(desktop, 'src/daemon-notice.js'), 'utf8');
const shots = process.env.SHOTS;
const modulePath = process.env.WS_PLAYWRIGHT_MODULE;

test('replacement refusal reaches only the pinned dashboard and keeps its document', {
  skip: !modulePath && 'Set WS_PLAYWRIGHT_MODULE to run Chromium',
}, async () => {
  const { chromium } = require(modulePath);
  const browser = await chromium.launch({ headless: true });
  const origin = 'http://127.0.0.1:49152';
  const message = 'No healthy replacement was found. Keeping the current connection.';
  const invoke = (frame, pin, reason) =>
    frame.evaluate(`(${notice})(${JSON.stringify(pin)}, ${JSON.stringify(reason)})`);
  const capture = async (page, name) => {
    if (shots) {
      fs.mkdirSync(shots, { recursive: true });
      await page.screenshot({ path: path.join(shots, name + '.png') });
    }
  };
  try {
    for (const colorScheme of ['light', 'dark']) {
      const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme });
      await page.route(origin + '/**', route => route.fulfill({
        contentType: 'text/html',
        body: route.request().isNavigationRequest() && route.request().url() === origin + '/'
          ? `<!doctype html><html data-theme="${colorScheme}"><body>
            <input id="draft" aria-label="Draft"><iframe name="child" src="/frame" hidden></iframe>`
          : '<!doctype html><body>',
      }));
      await page.goto(origin);
      await page.locator('#draft').fill('Retained draft');
      const child = page.frame({ name: 'child' });
      assert.equal(await invoke(page, origin, message), 0, 'legacy fallback');
      await page.addScriptTag({ path: path.join(assets, 'toast.js') });
      await child.addScriptTag({ path: path.join(assets, 'toast.js') });
      await page.addStyleTag({ path: path.join(assets, 'dashboard.css') });
      await page.addStyleTag({ path: path.join(assets, 'dashboard-menus.css') });
      assert.equal(await invoke(child, origin, message), 0, 'subframe denied');
      assert.equal(await invoke(page, 'http://127.0.0.1:49153', message), 0, 'other origin denied');
      const comparison = process.env.DAEMON_NOTICE_BASELINE;
      if (comparison && colorScheme === 'light') {
        const background = fs.readFileSync(comparison).toString('base64');
        await page.addStyleTag({ content: `html body { background: url(data:image/png;base64,${background}) no-repeat fixed; }
          #draft { opacity: 0; }` });
        assert.equal(await invoke(page, origin, 'The starter refused replacement. Retry after checking daemon health.'), 1);
        await capture(page, 'refusal-reference-copy-light');
      }
      assert.equal(await invoke(page, origin, message), 1);
      const toast = page.locator('#wstoasts .wstoast.warn');
      assert.equal(await toast.textContent(), message);
      assert.equal(await page.locator('#draft').inputValue(), 'Retained draft');
      assert.equal(page.url(), origin + '/');
      await capture(page, 'refusal-' + colorScheme);
      assert.equal(await invoke(page, origin, '<b>New warning</b>'), 1);
      assert.equal(await toast.count(), 1);
      assert.equal(await toast.textContent(), '<b>New warning</b>');
      assert.equal(await toast.locator('b').count(), 0);
      if (shots) {
        await page.setViewportSize({ width: 980, height: 650 });
        for (const [file, heading] of [['index', 'Connect to your dashboard'], ['python-missing', 'Python 3 is required']]) {
          await page.goto(pathToFileURL(path.join(desktop, 'ui', file + '.html')).href);
          assert.equal(await page.locator('h1').innerText(), heading);
          if (file === 'index') assert.match(await page.locator('main').textContent(), /Retry to start or reconnect/);
          await capture(page, file + '-' + colorScheme);
        }
      }
      await page.close();
    }
  } finally {
    await browser.close();
  }
});
