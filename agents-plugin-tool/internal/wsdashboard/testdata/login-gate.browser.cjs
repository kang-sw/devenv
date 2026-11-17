// TestLoginGateBrowser: real daemon auth responses, disposable native clipboard.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const cfg = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
fs.mkdirSync(cfg.shots, { recursive: true });
let clock = 0, stage = 'startup';
function advance(seconds) {
  clock += seconds;
  fs.writeFileSync(cfg.clock + '.tmp', String(clock));
  fs.renameSync(cfg.clock + '.tmp', cfg.clock);
}
function progress(text) { stage = text; console.log(text); }
function deferred() { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; }
const pasteShortcut = process.platform === 'darwin' ? 'Meta+v' : 'Control+v';
function nativeClipboardContents() { return fs.existsSync(cfg.clipboard) ? fs.readFileSync(cfg.clipboard, 'utf8') : null; }
async function mint(next = '/s/livesession') {
  const r = await fetch(cfg.base + '/auth/link', { method: 'POST', headers: { Authorization: 'Bearer ' + cfg.token, 'Content-Type': 'application/json' }, body: JSON.stringify({ path: next }) });
  assert.equal(r.status, 200);
  return (await r.json()).url;
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  const contexts = [], errors = [], captures = [];
  let active;
  async function fixture(theme, permissions = true, pinnedTheme = true) {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme, locale: 'ko-KR' });
    contexts.push(context);
    context.setDefaultTimeout(10000);
    if (permissions) await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: cfg.base });
    await context.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: cfg.base }, ...(pinnedTheme ? [{ name: 'ws_dashboard_theme', value: theme, url: cfg.base }] : [])]);
    const calls = { readText: 0, writeText: 0, execCommand: 0 };
    await context.exposeBinding('__recordClipboard', (_, name) => { calls[name]++; });
    await context.addInitScript(() => {
      // Playwright's serviceWorkers:block script throws in opaque topic frames.
      // Block registration only where that API may safely be read.
      if (self.origin !== 'null') {
        const workers = navigator.serviceWorker;
        if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('Service workers forbidden in login fixture'); };
      }
      // Test seeding bypasses counters; production code always hits the wrappers.
      const clip = navigator.clipboard;
      if (clip) {
        const readText = clip.readText.bind(clip), writeText = clip.writeText.bind(clip);
        window.__seedClipboard = writeText;
        for (const [name, fn] of [['readText', readText], ['writeText', writeText]]) {
          Object.defineProperty(clip, name, { value: (...args) => { window.__recordClipboard(name); return fn(...args); } });
        }
      }
      const exec = document.execCommand.bind(document);
      document.execCommand = (...args) => { window.__recordClipboard('execCommand'); return exec(...args); };
      window.__testSockets = [];
      window.WebSocket = new Proxy(window.WebSocket, { construct(Target, args) {
        const socket = new Target(...args); window.__testSockets.push(socket); return socket;
      } });
    });
    await context.route('**/*', route => {
      const u = new URL(route.request().url());
      if (u.origin === cfg.base || u.hostname === 'localhost') return route.continue();
      return route.abort();
    });
    const page = await context.newPage(); active = page;
    const nativeResponses = [];
    await page.route('**/auth/clipboard', async route => {
      // The gate checks only r.ok; inspect the real server body here rather
      // than waiting for Chromium to consume an intentionally unread fetch.
      const response = await route.fetch();
      nativeResponses.push({ status: response.status(), body: await response.json() });
      await route.fulfill({ response });
    });
    page.on('pageerror', e => errors.push(stage + ': ' + e.message));
    return { context, page, calls, nativeResponses };
  }
  async function idleClipboard(f, expected = { readText: 0, writeText: 0, execCommand: 0 }) {
    await f.page.evaluate(() => Promise.resolve());
    assert.deepEqual(f.calls, expected, stage + ': no implicit clipboard calls');
  }
  async function gate(page, next) {
    const target = cfg.base + next;
    const r = page.url() === target ? await page.reload() : await page.goto(target);
    assert.equal(r.status(), 401);
    assert.equal(r.headers()['cache-control'], 'no-store');
    await page.locator('html[data-login-gate]').waitFor();
    await page.waitForFunction(() => !!window.wsLogin);
  }
  async function seed(page, text) { await page.evaluate(text => window.__seedClipboard(text), text); }
  async function paste(page, text) {
    await seed(page, text);
    await page.evaluate(() => document.activeElement?.blur());
    assert.equal(await page.evaluate(() => document.activeElement === document.body), true, 'paste needs no input focus');
    await page.keyboard.press(pasteShortcut);
  }
  async function capture(page, theme, state, selector = '.lg-card') {
    if (state === 'A' || state === 'A3') {
      const footer = page.locator('.lg-where');
      assert.deepEqual(await footer.locator('b').allTextContents(), ['클립보드에 복사', '설정', '연결']);
      const lines = await footer.evaluate(el => {
        const css = getComputedStyle(el);
        return (el.getBoundingClientRect().height - parseFloat(css.paddingTop) - parseFloat(css.borderTopWidth)) / parseFloat(css.lineHeight);
      });
      assert.ok(lines <= 2.1, `short source footer must fit two lines, got ${lines}`);
    }
    await page.screenshot({ path: path.join(cfg.shots, `${theme}-${state}.png`) });
    const card = `${theme}-${state}-detail.png`;
    await page.locator(selector).screenshot({ path: path.join(cfg.shots, card) });
    captures.push({ theme, state, card });
  }
  async function loggedIn(page, next) {
    active = page;
    await page.waitForURL(cfg.base + next);
    await page.locator('html[data-login-gate]').waitFor({ state: 'detached' });
    await page.locator('#gear').waitFor({ state: 'attached' });
  }
  try {
    for (const theme of ['light', 'dark']) {
      advance(120);
      const f = await fixture(theme), { page, context } = f;
      const next = '/s/livesession?acceptance=' + theme + '#/login-fragment';
      progress(theme + ': A cookieless load, unavailable writer hidden, Cprime clipboard warning');
      assert.equal((await context.cookies()).some(c => c.name === 'ws_dashboard_localhost'), false);
      const nativeBefore = nativeClipboardContents();
      await gate(page, next);
      assert.equal(nativeClipboardContents(), nativeBefore, 'gate load never invokes native clipboard writer');
      await page.locator('#lg-copy').waitFor();
      assert.equal(await page.getAttribute('html', 'data-theme'), theme);
      await page.emulateMedia({ colorScheme: theme === 'dark' ? 'light' : 'dark' });
      assert.equal(await page.getAttribute('html', 'data-theme'), theme, 'explicit theme cookie overrides OS');
      await idleClipboard(f);
      await capture(page, theme, 'A');
      fs.writeFileSync(cfg.clipboard + '.unavailable', '1');
      await gate(page, next);
      assert.equal(await page.locator('#lg-copy').count(), 0, 'unavailable writer hides native copy at render');
      await capture(page, theme, 'unavailable');
      fs.unlinkSync(cfg.clipboard + '.unavailable');
      await gate(page, next);
      await idleClipboard(f);
      await seed(page, '');
      await page.locator('#lg-clip').click();
      await page.locator('#lg-msg.warn').waitFor({ state: 'visible' });
      await capture(page, theme, 'Cprime');
      await idleClipboard(f, { readText: 1, writeText: 0, execCommand: 0 });
      // Capture native failures independently of the earlier empty-read warning.
      await gate(page, next);
      assert.equal(nativeClipboardContents(), nativeBefore, 'gate reload never invokes native writer');

      progress(theme + ': A3 native writer failure remains retryable; A4 actual native throttle');
      fs.writeFileSync(cfg.clipboard + '.fail', '1');
      const failed = page.waitForResponse(r => r.url() === cfg.base + '/auth/clipboard');
      await page.locator('#lg-copy').click();
      assert.equal((await failed).status(), 503);
      await page.locator('#lg-native-msg.note').waitFor({ state: 'visible' });
      assert.equal(await page.locator('#lg-copy').isEnabled(), true, 'runtime failure permits retry');
      await capture(page, theme, 'A3');
      fs.unlinkSync(cfg.clipboard + '.fail');
      const limited = page.waitForResponse(r => r.url() === cfg.base + '/auth/clipboard');
      await page.locator('#lg-copy').click();
      const limitedResponse = await limited;
      assert.equal(limitedResponse.status(), 429);
      assert.ok(Number(limitedResponse.headers()['retry-after']) > 0);
      await page.locator('#lg-native-msg.warn').waitFor({ state: 'visible' });
      await capture(page, theme, 'A4');

      progress(theme + ': A2 real native route returns no credential; real Ctrl+V; B ignores second paste; D');
      advance(61);
      const copied = page.waitForResponse(r => r.url() === cfg.base + '/auth/clipboard');
      await page.locator('#lg-copy').click();
      const copiedResponse = await copied;
      assert.equal(copiedResponse.status(), 200);
      assert.deepEqual(f.nativeResponses.at(-1), { status: 200, body: { ok: true } }, 'native route must not return the link/code');
      await page.locator('#lg-hint.go').waitFor({ state: 'visible' });
      await page.locator('#lg-again').waitFor({ state: 'visible' });
      const firstLink = fs.readFileSync(cfg.clipboard, 'utf8');
      assert.ok(new URL(firstLink).searchParams.get('login'));
      await capture(page, theme, 'A2');
      console.log('  A2 captured');
      advance(3);
      const recopied = page.waitForResponse(r => r.url() === cfg.base + '/auth/clipboard');
      await page.locator('#lg-again').click();
      assert.equal((await recopied).status(), 200);
      const link = fs.readFileSync(cfg.clipboard, 'utf8');
      assert.notEqual(link, firstLink, 'recopy mints a new one-use code');
      const responseReady = deferred(), release = deferred();
      let exchanges = 0;
      await page.route('**/auth/exchange', async route => {
        exchanges++;
        // Hold a real server response so checking is observable, never fake auth.
        const response = await route.fetch();
        assert.equal(response.status(), 200);
        responseReady.resolve();
        await release.promise;
        await route.fulfill({ response });
      });
      console.log('  submitting native paste via ' + pasteShortcut);
      await paste(page, link);
      console.log('  shortcut dispatched');
      await Promise.race([responseReady.promise, new Promise((_, reject) => setTimeout(() => reject(new Error('native paste did not exchange within 10 seconds')), 10000))]);
      await page.locator('#lg-hint.busy').waitFor({ state: 'visible' });
      assert.equal(await page.locator('#lg-input').isDisabled(), true);
      assert.equal(await page.locator('#lg-clip').isDisabled(), true);
      assert.equal(await page.locator('#lg-go').isDisabled(), true);
      assert.equal(await page.locator('#lg-input').inputValue(), '…/?login=••••••••');
      await capture(page, theme, 'B');
      await seed(page, 'ignored-during-checking');
      await page.keyboard.press(pasteShortcut);
      await page.waitForTimeout(100);
      assert.equal(exchanges, 1, 'paste while checking must not create another exchange');
      release.resolve();
      await page.locator('#lg-done').waitFor({ state: 'visible' });
      await capture(page, theme, 'D');
      await loggedIn(page, next);
      assert.equal(nativeClipboardContents(), link, 'login and dashboard load never invoke native writer');
      await page.unroute('**/auth/exchange');
      await idleClipboard(f, { readText: 1, writeText: 0, execCommand: 0 });
      const cookie = (await context.cookies()).find(c => c.name === 'ws_dashboard_localhost');
      assert.ok(cookie?.httpOnly);
      assert.equal(cookie.sameSite, 'Strict');
      assert.equal(cookie.path, '/');
      assert.ok(cookie.expires > Date.now() / 1000 + 399 * 86400);

      progress(theme + ': E explicit Settings copy and toast, clipboard button login, single-use rejection');
      if (!await page.locator('#gear').isVisible()) await page.locator('#railbtn').click();
      await page.locator('#gear').click();
      await page.locator('#login-copy').waitFor({ state: 'visible' });
      assert.match(await page.locator('.loginbox .gs').textContent(), /클립보드/);
      await idleClipboard(f, { readText: 1, writeText: 0, execCommand: 0 });
      await page.locator('#login-copy').click();
      await page.locator('.wstoast').filter({ hasText: '로그인 링크' }).waitFor({ state: 'visible' });
      await capture(page, theme, 'E', '#gmenu');
      await idleClipboard(f, { readText: 1, writeText: 1, execCommand: 0 });
      const settingsLink = await page.evaluate(() => navigator.clipboard.readText());
      assert.ok(new URL(settingsLink).searchParams.get('login'));
      const second = await fixture(theme);
      await gate(second.page, '/s/livesession#/clipboard-button');
      await idleClipboard(second);
      // Settings wrote the actual Chromium clipboard; no test seed for this path.
      await second.page.locator('#lg-clip').click();
      await loggedIn(second.page, '/s/livesession#/clipboard-button');
      await idleClipboard(second, { readText: 1, writeText: 0, execCommand: 0 });
      const used = await fixture(theme);
      await gate(used.page, '/s/livesession');
      await paste(used.page, settingsLink);
      await used.page.locator('#lg-msg.bad').waitFor({ state: 'visible' });
      assert.equal((await used.context.cookies()).some(c => c.name === 'ws_dashboard_localhost'), false);
      await used.context.close();
      await second.context.close();

      progress(theme + ': C injected expiry with real exchange; fallback manual code; exchange rate limit');
      const expiredLink = await mint();
      advance(601);
      const expired = await fixture(theme);
      await gate(expired.page, '/s/livesession');
      await paste(expired.page, expiredLink);
      await expired.page.locator('#lg-msg.bad').waitFor({ state: 'visible' });
      assert.equal(await expired.page.locator('#lg-input').getAttribute('aria-invalid'), 'true');
      const selected = await expired.page.locator('#lg-input').evaluate(el => el.selectionEnd - el.selectionStart === el.value.length);
      assert.equal(selected, true, 'rejected masked value remains selected');
      await capture(expired.page, theme, 'C');
      for (let i = 0; i < 9; i++) {
        assert.equal(await expired.page.evaluate(async () => (await fetch('/auth/exchange', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code: 'invalid', next: '/' }) })).status), 401);
      }
      await expired.page.locator('#lg-input').fill('invalid');
      await expired.page.locator('#lg-go').click();
      await expired.page.locator('#lg-msg.warn').waitFor({ state: 'visible' });
      await capture(expired.page, theme, 'exchange-rate');
      advance(61);
      const manualLink = await mint();
      await expired.page.locator('#lg-input').fill(new URL(manualLink).searchParams.get('login'));
      await expired.page.locator('#lg-input').press('Enter');
      await loggedIn(expired.page, '/s/livesession');
      await expired.context.close();

      progress(theme + ': cross-site Strict reentry via localhost preserves fragment');
      const before = { ...f.calls }, nativeBeforeReentry = nativeClipboardContents();
      const reentry = '/s/livesession?reentry=' + theme + '#/strict-fragment';
      const cross = http.createServer((_, res) => {
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(`<a id="enter" href="${cfg.base + reentry}">Dashboard</a>`);
      });
      await new Promise(resolve => cross.listen(0, resolve));
      try {
        const documents = [], probes = [];
        const listener = response => {
          if (response.url().startsWith(cfg.base + '/s/livesession?reentry=')) documents.push(response);
          if (response.url() === cfg.base + '/auth/status') probes.push(response);
        };
        page.on('response', listener);
        await page.goto('http://localhost:' + cross.address().port);
        await page.locator('#enter').click();
        await loggedIn(page, reentry);
        page.off('response', listener);
        assert.ok(documents.some(r => r.status() === 401), 'cross-site navigation really meets Strict gate');
        const first = documents.find(r => r.status() === 401);
        assert.notEqual((await first.request().allHeaders()).cookie?.includes('ws_dashboard_localhost='), true, 'Strict cookie omitted on cross-site navigation');
        assert.ok(probes.some(r => r.status() === 200), 'same-origin status probe receives existing cookie');
        await idleClipboard(f, before);
        assert.equal(nativeClipboardContents(), nativeBeforeReentry, 'Strict reentry never invokes native writer');
      } finally { await new Promise(resolve => cross.close(resolve)); }

      progress(theme + ': secret deletion + next real POST reloads top into gate');
      fs.unlinkSync(cfg.secret);
      const denied = page.waitForResponse(r => r.url() === cfg.base + '/s/livesession/message' && r.request().method() === 'POST');
      await page.evaluate(() => { fetch('/s/livesession/message', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }).catch(() => {}); });
      assert.equal((await denied).status(), 401);
      await page.locator('html[data-login-gate]').waitFor({ state: 'attached' });
      assert.equal(new URL(page.url()).hash, '#/strict-fragment');
      await idleClipboard(f, before);
      await capture(page, theme, 'rotated');

      progress(theme + ': live WebSocket drop after secret rotation probes and reloads top gate');
      const streamNext = '/s/livesession?stream=' + theme + '#/stream-fragment';
      await page.goto(await mint(streamNext));
      await loggedIn(page, streamNext);
      await page.waitForFunction(() => window.__testSockets.some(socket => socket.readyState === WebSocket.OPEN));
      fs.unlinkSync(cfg.secret);
      const probe = page.waitForResponse(r => r.url() === cfg.base + '/auth/status');
      const closedSockets = await page.evaluate(() => {
        const sockets = window.__testSockets.filter(socket => socket.readyState === WebSocket.OPEN);
        sockets.forEach(socket => socket.close());
        return sockets.length;
      });
      assert.ok(closedSockets > 0, 'actually close live production stream sockets');
      assert.equal((await probe).status(), 401);
      await page.locator('html[data-login-gate]').waitFor({ state: 'attached' });
      assert.equal(new URL(page.url()).hash, '#/stream-fragment');
      await idleClipboard(f, before);
      await capture(page, theme, 'stream-rotated');
      assert.equal(nativeClipboardContents(), nativeBeforeReentry, 'POST and stream recovery never invoke native writer');
      await context.close();
    }

    progress('OS theme fallback: no theme cookie follows dark then light media changes');
    const automatic = await fixture('dark', true, false);
    const nativeBeforeAuto = nativeClipboardContents();
    await gate(automatic.page, '/');
    await automatic.page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
    await automatic.page.emulateMedia({ colorScheme: 'light' });
    await automatic.page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
    await idleClipboard(automatic);
    assert.equal(nativeClipboardContents(), nativeBeforeAuto, 'OS theme gate load never writes native clipboard');
    await automatic.context.close();

    progress('mockup comparisons: A/A2/A3/A4/B/C/Cprime/D/E, light and dark');
    const mock = await browser.newPage({ viewport: { width: 1280, height: 1800 } });
    const compare = await browser.newPage({ viewport: { width: 920, height: 1100 } });
    await mock.goto(pathToFileURL(cfg.mockup).href);
    for (const { theme, state, card } of captures.filter(c => ['A', 'A2', 'A3', 'A4', 'B', 'C', 'Cprime', 'D', 'E'].includes(c.state))) {
      await mock.evaluate(theme => document.documentElement.setAttribute('data-theme', theme), theme);
      const label = state === 'Cprime' ? 'C′' : state;
      const section = mock.locator('.mk-lbl').filter({ has: mock.locator('.pill').getByText(label, { exact: true }) }).locator('..');
      const target = section.locator(state === 'E' ? '.settings-pop' : '.lg-card').first();
      const mockFile = `${theme}-${state}-mockup.png`;
      await target.screenshot({ path: path.join(cfg.shots, mockFile) });
      const image = filename => 'data:image/png;base64,' + fs.readFileSync(path.join(cfg.shots, filename)).toString('base64');
      await compare.setContent(`<style>body{margin:0;padding:18px;font:14px system-ui;background:${theme === 'dark' ? '#141518' : '#f3f3f0'};color:${theme === 'dark' ? '#e7e7e4' : '#1b1d21'}}main{display:flex;gap:24px;align-items:start}section{width:420px}img{max-width:100%}p{margin:0 0 12px}</style><h3>${theme} — ${state}</h3><main><section><p>Approved mockup</p><img src="${image(mockFile)}"></section><section><p>Real daemon / Chromium</p><img src="${image(card)}"></section></main>`);
      await compare.screenshot({ path: path.join(cfg.shots, `${theme}-${state}-comparison.png`), fullPage: true });
    }
    await compare.setViewportSize({ width: 1440, height: 1380 });
    for (const theme of ['light', 'dark']) {
      const tiles = ['A', 'A2', 'A3', 'A4', 'B', 'C', 'Cprime', 'D', 'E'].map(state => {
        const data = fs.readFileSync(path.join(cfg.shots, `${theme}-${state}-comparison.png`)).toString('base64');
        return `<section><img src="data:image/png;base64,${data}"></section>`;
      }).join('');
      await compare.setContent(`<style>body{margin:0;background:${theme === 'dark' ? '#141518' : '#f3f3f0'}}main{display:grid;grid-template-columns:repeat(3,480px)}section{height:450px;overflow:hidden}img{width:480px}</style><main>${tiles}</main>`);
      await compare.screenshot({ path: path.join(cfg.shots, `${theme}-comparison-overview.png`), fullPage: true });
    }
    await mock.close(); await compare.close();
    assert.deepEqual(errors, [], 'no browser JavaScript exceptions');
    for (const stale of ['failure.png', 'native-copy-debug.png']) {
      const filename = path.join(cfg.shots, stale);
      if (fs.existsSync(filename)) fs.unlinkSync(filename);
    }
    fs.writeFileSync(path.join(cfg.shots, 'acceptance.json'), JSON.stringify({ shortcut: pasteShortcut, fixtureVersion: '0.50.2-test', states: captures.map(({ theme, state }) => `${theme}-${state}`), errors, checks: 'real Ctrl+V, clipboard button, native round trip, one-use, expiry, throttles, checking ignores paste, Settings explicit copy/toast, Strict reentry, fragment retention, secret rotation POST and real WebSocket-drop recovery; no implicit clipboard access' }, null, 2));
    console.log('PASS: login gate acceptance and side-by-side captures: ' + cfg.shots);
  } catch (e) {
    console.error('FAILED STAGE: ' + stage);
    if (active && !active.isClosed()) await active.screenshot({ path: path.join(cfg.shots, 'failure.png') }).catch(() => {});
    throw e;
  } finally {
    for (const context of contexts) await context.close().catch(() => {});
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
