// Run by TestDesktopUpdateBrowser against a disposable fixture daemon. A
// browser tab (no werkschaftDesktop) shows no update row and no toast; a page
// with a stub of the desktop shell's preload bridge shows the settings row and
// the bottom-right toasts and calls back into the bridge. A current shell's
// bridge adds the canary pill and, while a packaged canary runs, Back to
// stable; an older shell's bridge shows neither. With SHOTS set it also
// writes the review screenshots there (CANARY_SHOTS: the channel captures).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base] = process.argv.slice(2);
const shots = process.env.SHOTS || '';
const tauriShots = process.env.TAURI_SHOTS || '';
const tauriBridge = fs.readFileSync(path.resolve(__dirname, '../../../desktop/src/update-bridge.js'), 'utf8');

// The stub mirrors an older shell's dashboard-preload.js: check/download/
// skip/restart and onStatus only.
function stub() {
  window.__calls = [];
  window.__emit = s => window.__listener && window.__listener(s);
  window.werkschaftDesktop = { update: {
    check() { window.__calls.push('check'); },
    download() { window.__calls.push('download'); },
    skip(v) { window.__calls.push('skip:' + v); },
    restart() { window.__calls.push('restart'); },
    onStatus(cb) { window.__listener = cb; window.__calls.push('subscribe'); },
  } };
}
// The current shell adds the payload-free canary and stable-return actions and
// the running channel; running is what main's update-channel handler returns.
function channelStub(running) {
  window.__calls = [];
  window.__emit = s => window.__listener && window.__listener(s);
  window.werkschaftDesktop = { update: {
    check() { window.__calls.push('check'); },
    download() { window.__calls.push('download'); },
    skip(v) { window.__calls.push('skip:' + v); },
    restart() { window.__calls.push('restart'); },
    checkCanary(...args) { window.__calls.push('checkCanary' + (args.length ? ':args' : '')); },
    backToStable(...args) { window.__calls.push('backToStable' + (args.length ? ':args' : '')); },
    channel() { window.__calls.push('channel'); return running === 'reject' ? Promise.reject(new Error('x')) : Promise.resolve(running); },
    onStatus(cb) { window.__listener = cb; window.__calls.push('subscribe'); },
  } };
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const plain = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale: 'en-US' });
    const errors = [];
    plain.on('pageerror', e => errors.push(e.message));
    await plain.goto(base + '/');
    await plain.locator('#gear').click();
    await plain.locator('#gmenu').waitFor({ state: 'visible' });
    assert.equal(await plain.locator('#dupd-check').count(), 0, 'a browser tab has no update row');
    assert.equal(await plain.locator('#dupd-canary, #dupd-stable').count(), 0, 'a browser tab has no channel controls');
    assert.equal(await plain.locator('.wstoast').count(), 0, 'a browser tab has no update toast');
    if (shots) await plain.screenshot({ path: path.join(shots, 'settings-browser-tab-light.png') });
    await plain.close();

    for (const [scheme, locale] of [['dark', 'ko-KR'], ['light', 'ko-KR'], ['dark', 'en-US']]) {
      const page = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale, colorScheme: scheme });
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(stub);
      await page.goto(base + '/');
      await page.waitForFunction(() => window.__calls.includes('subscribe'));
      const shot = async name => { if (shots && locale === 'ko-KR') await page.screenshot({ path: path.join(shots, name) }); };
      const calls = () => page.evaluate(() => window.__calls.slice());
      const emit = s => page.evaluate(s => window.__emit(s), s);

      // Settings: the row shows, with the standing phase beside the button.
      await emit({ state: 'phase', phase: { phase: 'none', version: null, skipped: false } });
      await page.locator('#gear').click();
      await page.locator('#dupd-check').waitFor({ state: 'visible' });
      assert.equal(await page.locator('#dupd-restart').isVisible(), false, 'no restart before an update is ready');
      assert.equal(await page.locator('#dupd-canary, #dupd-stable, .dupd-row').count(), 0, 'an older shell keeps the original row');
      const label = await page.locator('#dupd-check .gs').textContent();
      assert.ok(label.length > 0, 'the none phase has a line');
      await shot(`settings-check-button-${scheme}.png`);
      await page.locator('#dupd-check').click();
      assert.ok((await calls()).includes('check'));
      await shot(`settings-state-none-${scheme}.png`);
      await emit({ state: 'phase', phase: { phase: 'available', version: '0.51.0', skipped: false } });
      assert.match(await page.locator('#dupd-check .gs').textContent(), /0\.51\.0/);
      await shot(`settings-state-available-${scheme}.png`);
      await emit({ state: 'phase', phase: { phase: 'ready', version: '0.51.0', skipped: false } });
      await page.locator('#dupd-restart').waitFor({ state: 'visible' });
      await shot(`settings-state-ready-${scheme}.png`);
      await page.locator('#dupd-restart').click();
      assert.equal((await calls()).at(-1), 'restart');
      await page.keyboard.press('Escape');
      await page.locator('body').click({ position: { x: 1000, y: 500 } });

      // Available: text, [Update], [Skip this version], and a dismiss button.
      await emit({ state: 'available', version: '0.51.0', current: '0.50.0', skipped: false,
        phase: { phase: 'available', version: '0.51.0', skipped: false } });
      const toast = page.locator('.wstoast').first();
      await toast.waitFor({ state: 'visible' });
      assert.match(await toast.locator('.wst-body').textContent(), /v0\.51\.0/);
      assert.equal(await toast.locator('.wst-act').count(), 2);
      assert.equal(await toast.locator('.wst-x').count(), 1);
      const box = await toast.boundingBox();
      assert.ok(box.x + box.width > 1800 - 40 && box.y + box.height > 1000 - 60, 'the toast sits bottom-right');
      await shot(`toast-new-${scheme}.png`);
      await page.waitForTimeout(5000);
      assert.equal(await page.locator('.wstoast').count(), 1, 'the update toast stays until acted on');
      await toast.locator('.wst-act').nth(1).click();
      assert.equal((await calls()).at(-1), 'skip:0.51.0');
      assert.equal(await page.locator('.wstoast').count(), 0);
      await emit({ state: 'available', version: '0.51.0', skipped: true, phase: { phase: 'available', version: '0.51.0', skipped: true } });
      await page.locator('.wstoast .wst-act').first().click();
      assert.equal((await calls()).at(-1), 'download');

      // Ready: the restart button, and the full-quit/reboot note in the text.
      await emit({ state: 'downloading', version: '0.51.0', phase: { phase: 'available', version: '0.51.0', skipped: false } });
      await emit({ state: 'ready', version: '0.51.0', phase: { phase: 'ready', version: '0.51.0', skipped: false } });
      assert.equal(await page.locator('.wstoast').count(), 1, 'a new status replaces the previous update toast');
      assert.equal(await page.locator('.wstoast .wst-act').count(), 1);
      await shot(`toast-ready-${scheme}.png`);
      await page.locator('.wstoast .wst-act').click();
      assert.equal((await calls()).at(-1), 'restart');

      await emit({ state: 'current', version: '0.50.0', manual: true, phase: { phase: 'none', version: null, skipped: false } });
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /v0\.50\.0/);
      await shot(`toast-uptodate-${scheme}.png`);
      await emit({ state: 'error', manual: true, phase: { phase: 'none', version: null, skipped: false } });
      assert.equal(await page.locator('.wstoast.warn').count(), 1);
      await shot(`toast-error-${scheme}.png`);
      await page.close();
    }
    // The download page: a shell that does not update itself (Windows) sends
    // downloadPage on available, which replaces [Update]; a failed download
    // or apply carries it too. The page opens through window.open, which the
    // shell's window-open handler hands to the system browser.
    const PAGE = 'https://github.com/kang-sw/werkschaft/releases/latest';
    const notice = { state: 'available', version: '0.51.0', current: '0.50.0', skipped: false, manual: false, downloadPage: PAGE };
    const manual = [
      ['win-ready', 'dark', 1440, notice],
      ['win-ready', 'light', 1440, notice],
      ['win-ready', 'dark', 700, notice],
      ['mac-ready', 'dark', 1440, { state: 'ready', version: '0.51.0' }],
      ['mac-applyfailed', 'dark', 1440, { state: 'apply-error', version: '0.51.0', downloadPage: PAGE,
        message: 'the app folder /Applications/Werkschaft.app is not writable' }],
      ['mac-downloadfailed', 'dark', 1440, { state: 'download-error', version: '0.51.0', downloadPage: PAGE,
        message: 'checksum mismatch for Werkschaft-darwin-arm64.zip' }],
    ];
    for (const [name, scheme, width, status] of manual) {
      const page = await browser.newPage({ viewport: { width, height: 900 }, locale: 'ko-KR', colorScheme: scheme });
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(stub);
      await page.addInitScript(() => { window.open = (url, target) => { window.__calls.push('open:' + url + ' ' + target); return null; }; });
      await page.goto(base + '/');
      await page.waitForFunction(() => window.__calls.includes('subscribe'));
      const phase = status.state === 'ready' ? 'ready' : 'available';
      await page.evaluate(s => window.__emit(s), { ...status, phase: { phase, version: '0.51.0', skipped: false } });
      const toast = page.locator('.wstoast').first();
      await toast.waitFor({ state: 'visible' });
      const labels = await toast.locator('.wst-act').allTextContents();
      if (status.state === 'available') assert.deepEqual(labels, ['다운로드 페이지 열기', '이번 버전 건너뛰기'], name);
      else if (status.state === 'ready') assert.deepEqual(labels, ['지금 다시 시작'], name);
      else assert.deepEqual(labels, ['다운로드 페이지 열기'], name);
      const box = await toast.boundingBox();
      assert.ok(box.x >= 0 && box.x + box.width <= width && box.y + box.height <= 900, `${name}: the toast fits the viewport`);
      if (shots) await page.screenshot({ path: path.join(shots, `${name}-${scheme}-${width}.png`) });
      await toast.locator('.wst-act').first().click();
      const last = (await page.evaluate(() => window.__calls.slice())).at(-1);
      assert.equal(last, status.state === 'ready' ? 'restart' : `open:${PAGE} _blank`, name);
      if (status.state === 'available') {
        assert.equal(await page.locator('#dupd-restart').evaluate(e => e.hidden), true, `${name}: no restart in settings`);
      }
      await page.close();
    }
    // An older shell sends no downloadPage: the toasts stay as they were.
    {
      const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: 'ko-KR' });
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(stub);
      await page.goto(base + '/');
      await page.waitForFunction(() => window.__calls.includes('subscribe'));
      await page.evaluate(() => window.__emit({ state: 'apply-error', version: '0.51.0', message: 'x' }));
      await page.locator('.wstoast').first().waitFor({ state: 'visible' });
      assert.equal(await page.locator('.wstoast .wst-act').count(), 0);
      await page.evaluate(() => window.__emit({ state: 'available', version: '0.51.0', downloadPage: 'http://example.test/' }));
      assert.deepEqual(await page.locator('.wstoast .wst-act').allTextContents(), ['업데이트', '이번 버전 건너뛰기'], 'a non-https page is ignored');
      await page.close();
    }
    // Canary channel controls (current shell). The settings shots use the
    // same 1800x1000 ko-KR viewport and none phase as the before captures.
    const CANARY_SHOTS = process.env.CANARY_SHOTS || '';
    const STABLE = { canary: false, version: '0.51.0' };
    const CANARY = { canary: true, version: '0.52.0-canary.1' };
    const openShell = async (running, { scheme = 'light', locale = 'ko-KR', width = 1800, height = 1000 } = {}) => {
      const page = await browser.newPage({ viewport: { width, height }, locale, colorScheme: scheme });
      page.on('pageerror', e => errors.push(e.message));
      await page.addInitScript(channelStub, running);
      await page.goto(base + '/');
      await page.waitForFunction(() => window.__calls.includes('subscribe') && window.__calls.includes('channel'));
      await page.evaluate(() => window.__emit({ state: 'phase', phase: { phase: 'none', version: null, skipped: false } }));
      // A narrow window folds the rail; its button reveals the settings toggle.
      if (!await page.locator('#gear').isVisible()) await page.locator('#railbtn').click();
      await page.locator('#gear').click();
      await page.locator('#dupd-check').waitFor({ state: 'visible' });
      return page;
    };
    const calls = page => page.evaluate(() => window.__calls.slice());
    const emit = (page, s) => page.evaluate(s => window.__emit(s), s);
    const canaryShot = async (page, name) => { if (CANARY_SHOTS) await page.screenshot({ path: path.join(CANARY_SHOTS, name) }); };
    // The pill sits in the check row, beside the button, inside the card.
    async function assertBeside(page, label) {
      const check = await page.locator('#dupd-check').boundingBox();
      const pill = await page.locator('#dupd-canary').boundingBox();
      const card = await page.locator('#gmenu .gmenu').first().boundingBox();
      assert.equal(await page.locator('.dupd-row > #dupd-check + #dupd-canary').count(), 1, `${label}: pill follows the check button in one row`);
      assert.ok(pill.x >= check.x + check.width - 1 && pill.y < check.y + check.height && pill.y + pill.height > check.y,
        `${label}: the pill is beside the check button`);
      assert.ok(pill.x + pill.width <= card.x + card.width + 0.5 && pill.x >= card.x, `${label}: the pill stays inside the card`);
      assert.ok(pill.height <= check.height && pill.width < check.width, `${label}: the pill is smaller than the check row`);
      const overflow = await page.locator('.dupd-row').evaluate(e => e.scrollWidth - e.clientWidth);
      assert.ok(overflow <= 0, `${label}: the update row does not overflow (${overflow}px)`);
    }

    for (const scheme of ['light', 'dark']) {
      // Stable running (also the state after a stable return relaunch):
      // the pill shows, Back to stable does not.
      const page = await openShell(STABLE, { scheme });
      assert.equal(await page.locator('#dupd-canary').textContent(), 'Canary 빌드로 업데이트');
      assert.ok((await page.locator('#dupd-canary').getAttribute('title')).length > 0, 'the pill explains itself');
      assert.equal(await page.locator('#dupd-stable').isVisible(), false, 'a stable app has no Back to stable');
      await assertBeside(page, `stable-${scheme}`);
      const look = await page.locator('#dupd-canary').evaluate(e => {
        const c = getComputedStyle(e), k = getComputedStyle(document.getElementById('dupd-check'));
        return { style: c.borderTopStyle, color: c.color, checkColor: k.color, checkBorder: k.borderTopStyle };
      });
      assert.equal(look.style, 'dashed', 'the pill is visually distinct (dashed)');
      assert.notEqual(look.color, look.checkColor, 'the pill uses the warn ink, not the row text color');
      assert.equal(look.checkBorder, 'none', 'the stable check row keeps its look');
      await canaryShot(page, `after-stable-running-${scheme}.png`);
      // Keyboard: Tab from the check button reaches the pill, which shows a
      // focus ring and activates on Enter and Space.
      await page.locator('#dupd-check').focus();
      await page.keyboard.press('Tab');
      assert.equal(await page.evaluate(() => document.activeElement.id), 'dupd-canary');
      const ring = await page.locator('#dupd-canary').evaluate(e => {
        const c = getComputedStyle(e); return { style: c.outlineStyle, width: parseFloat(c.outlineWidth) };
      });
      assert.ok(ring.style !== 'none' && ring.width > 0, 'the focused pill shows a focus ring');
      await canaryShot(page, `after-stable-running-focus-${scheme}.png`);
      await page.keyboard.press('Enter');
      await page.keyboard.press('Space');
      assert.deepEqual((await calls(page)).filter(c => c.startsWith('checkCanary')), ['checkCanary', 'checkCanary']);
      assert.equal((await calls(page)).includes('check'), false, 'the pill never runs the stable check');

      // Canary statuses: none, cap-limited none, failure, available, ready.
      await emit(page, { state: 'current', version: '0.51.0', manual: true, action: 'canary', limited: false,
        phase: { phase: 'none', version: null, skipped: false } });
      const noneText = await page.locator('.wstoast .wst-body').textContent();
      assert.match(noneText, /canary/i);
      await emit(page, { state: 'current', version: '0.51.0', manual: true, action: 'canary', limited: true,
        phase: { phase: 'none', version: null, skipped: false } });
      const limitedText = await page.locator('.wstoast .wst-body').textContent();
      assert.notEqual(limitedText, noneText, 'a cap-limited miss reads differently from a full-list miss');
      await emit(page, { state: 'error', manual: true, action: 'canary', phase: { phase: 'none', version: null, skipped: false } });
      assert.equal(await page.locator('.wstoast.warn').count(), 1);
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /Canary/);
      await emit(page, { state: 'available', version: '0.52.0-canary.1', current: '0.51.0', manual: true, skipped: false, action: 'canary',
        phase: { phase: 'available', version: '0.52.0-canary.1', skipped: false, action: 'canary' } });
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /Canary.*v0\.52\.0-canary\.1/);
      assert.match(await page.locator('#dupd-check .gs').textContent(), /Canary v0\.52\.0-canary\.1/);
      assert.deepEqual(await page.locator('.wstoast .wst-act').allTextContents(), ['업데이트', '이번 버전 건너뛰기']);
      await canaryShot(page, `after-canary-available-${scheme}.png`);
      await page.locator('.wstoast .wst-act').first().click();
      assert.equal((await calls(page)).at(-1), 'download');
      await page.close();
    }

    for (const scheme of ['light', 'dark']) {
      // Canary running: Back to stable appears with the running version.
      const page = await openShell(CANARY, { scheme });
      const stable = page.locator('#dupd-stable');
      await stable.waitFor({ state: 'visible' });
      assert.match(await stable.locator('.dupd-label').textContent(), /안정 버전으로 되돌리기/);
      assert.match(await stable.locator('.gs').textContent(), /v0\.52\.0-canary\.1.*다운그레이드/);
      assert.equal(await page.locator('#dupd-canary').isVisible(), true, 'a canary app can still check for a newer canary');
      await assertBeside(page, `canary-${scheme}`);
      await canaryShot(page, `after-canary-running-${scheme}.png`);
      await stable.focus();
      await page.keyboard.press('Enter');
      assert.equal((await calls(page)).at(-1), 'backToStable');
      // The selection names the stable target and the downgrade before
      // anything installs; only an explicit action downloads it.
      await emit(page, { state: 'available', version: '0.51.0', current: '0.52.0-canary.1', manual: true, skipped: false,
        action: 'stable-return', phase: { phase: 'available', version: '0.51.0', skipped: false, action: 'stable-return' } });
      const body = await page.locator('.wstoast .wst-body').textContent();
      assert.match(body, /v0\.51\.0/);
      assert.match(body, /v0\.52\.0-canary\.1/);
      assert.match(body, /다운그레이드/);
      assert.deepEqual(await page.locator('.wstoast .wst-act').allTextContents(), ['다운그레이드 (v0.51.0)'], 'no skip on an explicit return');
      assert.equal(await page.locator('.wstoast .wst-x').count(), 1, 'the return can be dismissed');
      assert.match(await page.locator('#dupd-check .gs').textContent(), /v0\.51\.0.*다운그레이드/);
      // A previously skipped stable is still an explicit return: no skipped suffix.
      await emit(page, { state: 'phase', phase: { phase: 'available', version: '0.51.0', skipped: true, action: 'stable-return' } });
      assert.doesNotMatch(await page.locator('#dupd-check .gs').textContent(), /건너뜀/);
      assert.equal((await calls(page)).includes('download'), false, 'nothing installs before the explicit action');
      await canaryShot(page, `after-stable-return-confirm-${scheme}.png`);
      await page.locator('.wstoast .wst-act').click();
      assert.equal((await calls(page)).at(-1), 'download');
      await emit(page, { state: 'downloading', version: '0.51.0', action: 'stable-return',
        phase: { phase: 'available', version: '0.51.0', skipped: false, action: 'stable-return' } });
      await emit(page, { state: 'ready', version: '0.51.0', action: 'stable-return',
        phase: { phase: 'ready', version: '0.51.0', skipped: false, action: 'stable-return' } });
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /v0\.51\.0.*canary.*데몬/);
      // The toast click closed the settings card; the restart row is armed.
      assert.equal(await page.locator('#dupd-restart').evaluate(e => e.hidden), false);
      await canaryShot(page, `after-stable-return-ready-${scheme}.png`);
      await page.locator('.wstoast .wst-act').click();
      assert.equal((await calls(page)).at(-1), 'restart');
      // A stable-return failure or no-op reads as its own message.
      await emit(page, { state: 'error', manual: true, action: 'stable-return', phase: { phase: 'none', version: null, skipped: false } });
      assert.match(await page.locator('.wstoast.warn .wst-body').textContent(), /안정 버전/);
      await emit(page, { state: 'current', version: '0.51.0', manual: true, action: 'stable-return', limited: false,
        phase: { phase: 'none', version: null, skipped: false } });
      assert.match(await page.locator('.wstoast .wst-body').textContent(), /안정/);
      // A windows shell that does not update itself offers only the page.
      await emit(page, { state: 'available', version: '0.51.0', current: '0.52.0-canary.1', action: 'stable-return',
        downloadPage: 'https://github.com/kang-sw/werkschaft/releases/latest',
        phase: { phase: 'available', version: '0.51.0', skipped: false, action: 'stable-return' } });
      assert.deepEqual(await page.locator('.wstoast .wst-act').allTextContents(), ['다운로드 페이지 열기']);
      await page.close();
    }

    // Narrow windows and the longer English label: no overflow, still in the card.
    // Every wsi18n table's labels, at the shell's minimum window width.
    const narrow = [['ko-KR', 'light'], ['ko-KR', 'dark'], ['en-US', 'dark'], ['en-US', 'light'], ['ja-JP', 'light'], ['de-DE', 'light'],
      ['es-ES', 'light'], ['fr-FR', 'light'], ['it-IT', 'light'], ['pt-BR', 'light'], ['ru-RU', 'light'], ['zh-CN', 'light'], ['zh-TW', 'light']];
    const pills = new Set();
    for (const [locale, scheme, width = 760] of narrow) {
      const page = await openShell(CANARY, { scheme, locale, width, height: 900 });
      await page.locator('#dupd-stable').waitFor({ state: 'visible' });
      await assertBeside(page, `narrow-${locale}`);
      const stableOverflow = await page.locator('#dupd-stable').evaluate(e => e.scrollWidth - e.clientWidth);
      assert.ok(stableOverflow <= 0, `narrow-${locale}: Back to stable does not overflow`);
      if (locale === 'en-US') {
        assert.equal(await page.locator('#dupd-canary').textContent(), 'Update to canary');
        assert.match(await page.locator('#dupd-stable .dupd-label').textContent(), /Back to stable/);
      }
      pills.add(await page.locator('#dupd-canary').textContent());
      if (['ko-KR', 'en-US'].includes(locale)) await canaryShot(page, `after-narrow-${locale}-${scheme}-${width}.png`);
      await page.close();
    }

    assert.equal(pills.size, 11, 'every table translates the canary label');
    // A shell whose channel query fails never shows Back to stable.
    {
      const page = await openShell('reject');
      await page.waitForTimeout(100);
      assert.equal(await page.locator('#dupd-stable').isVisible(), false);
      assert.equal(await page.locator('#dupd-canary').isVisible(), true);
      await page.close();
    }
    // The Tauri shell injects its real bridge after the verified document
    // finishes. Use a native-transport stub so Chromium can check both paths.
    for (const scheme of ['light', 'dark']) {
      for (const channel of [STABLE, CANARY]) {
        const page = await browser.newPage({ viewport: { width: 1800, height: 1000 }, locale: 'ko-KR', colorScheme: scheme });
        page.on('pageerror', e => errors.push(e.message));
        await page.goto(base + '/');
        await page.evaluate(transport => {
          window.__calls = [];
          const postMessage = value => window.__calls.push(value);
          if (transport === 'light') window.webkit = { messageHandlers: { werkschaftUpdate: { postMessage } } };
          else (window.chrome ||= {}).webview = { postMessage };
        }, scheme);
        await page.addScriptTag({ content: tauriBridge });
        await page.evaluate(c => installWerkschaftUpdateBridge('http://127.0.0.1:1', c, {}), channel);
        assert.equal(await page.evaluate(() => typeof window.werkschaftDesktop), 'undefined');
        await page.evaluate(c => {
          installWerkschaftUpdateBridge(location.origin, c, { phase: 'none' });
          window.dispatchEvent(new Event('werkschaft-update-ready'));
        }, channel);
        await page.locator('#gear').click();
        await page.locator('#dupd-check').waitFor({ state: 'visible' });
        assert.equal(await page.locator('#dupd-check').count(), 1, 'late and repeated readiness mounts one row');
        await assertBeside(page, `tauri-${scheme}`);
        assert.equal(await page.locator('#dupd-stable').isVisible(), channel.canary);
        const shot = async name => {
          if (tauriShots) await page.screenshot({ path: path.join(tauriShots, `${name}-${scheme}.png`) });
        };
        await shot(channel.canary ? 'after-canary-running' : 'after-stable-running');
        await page.locator('#dupd-check').click();
        assert.equal((await calls(page)).at(-1), 'werkschaft:update:check');

        if (channel.canary) {
          await page.locator('#dupd-stable').click();
          assert.equal((await calls(page)).at(-1), 'werkschaft:update:stable-return');
          await page.evaluate(() => window.__werkschaftUpdateStatus({
            state: 'available', version: '0.51.0', current: '0.52.0-canary.1', action: 'stable-return',
            phase: { phase: 'available', version: '0.51.0', action: 'stable-return' }
          }));
          assert.deepEqual(await page.locator('.wstoast .wst-act').allTextContents(), ['다운그레이드 (v0.51.0)']);
          await shot('after-stable-return-confirm');
          await page.locator('.wstoast .wst-act').click();
          assert.equal((await calls(page)).at(-1), 'werkschaft:update:download');
          await page.evaluate(() => window.__werkschaftUpdateStatus({
            state: 'ready', version: '0.51.0', action: 'stable-return',
            phase: { phase: 'ready', version: '0.51.0', action: 'stable-return' }
          }));
          await shot('after-stable-return-ready');
          await page.locator('.wstoast .wst-act').click();
          assert.equal((await calls(page)).at(-1), 'werkschaft:update:restart');
        } else {
          await page.locator('#dupd-canary').click();
          assert.equal((await calls(page)).at(-1), 'werkschaft:update:canary');
          await page.evaluate(() => window.__werkschaftUpdateStatus({
            state: 'available', version: '0.52.0-canary.1', action: 'canary',
            phase: { phase: 'available', version: '0.52.0-canary.1', action: 'canary' }
          }));
          await shot('after-canary-available');
          await page.locator('.wstoast .wst-act').nth(1).click();
          assert.equal((await calls(page)).at(-1), 'werkschaft:update:skip', 'skip never sends a renderer version');
          assert.equal(await page.evaluate(source => {
            const iframe = document.createElement('iframe');
            document.body.append(iframe);
            iframe.contentWindow.eval(source);
            iframe.contentWindow.installWerkschaftUpdateBridge(location.origin, {}, {});
            const installed = Boolean(iframe.contentWindow.werkschaftDesktop);
            iframe.remove();
            return installed;
          }, tauriBridge), false, 'the bridge does not install in child frames');
        }
        await page.close();
      }
    }
    assert.deepEqual(errors, []);
    console.log('desktop update browser acceptance ok');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exit(1); });
