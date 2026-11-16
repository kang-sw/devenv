// Run by TestBoardOverviewShortcutBrowser against disposable production pages.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: 'light', locale: 'ko-KR', reducedMotion: 'reduce' });
    context.setDefaultTimeout(10000);
    await context.addInitScript(() => {
      if (self.origin === 'null') return;
      const workers = navigator.serviceWorker;
      if (workers) Object.getPrototypeOf(workers).register = async () => { throw new Error('Service workers forbidden in shortcut fixture'); };
    });
    await context.route('**/*', route => {
      const u = new URL(route.request().url());
      if (u.origin !== base) return route.abort();
      if (u.pathname === '/favicon.ico') return route.fulfill({ status: 204 });
      return route.continue();
    });
    const tab = await context.newPage(), errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    async function active(rid) {
      await tab.waitForFunction(rid => {
        const f = document.querySelector('iframe.sframe.on');
        return f && f.contentDocument?.documentElement.dataset.rid === rid && !!f.contentWindow.WsOverview && !!f.contentDocument.querySelector('.board-overview');
      }, rid);
      return (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    }
    async function state(frame, on, rid = 'livesession') {
      await frame.waitForFunction(on => document.documentElement.hasAttribute('data-overview') === on, on);
      assert.deepEqual(await frame.evaluate(rid => ({
        pressed: document.querySelector('.board-overview').getAttribute('aria-pressed'),
        active: window.WsOverview.active(),
        stored: sessionStorage.getItem('ws.dashboard.overview.' + rid) !== null
      }), rid), { pressed: String(on), active: on, stored: on });
    }
    async function uncanceled(frame, selector, attributes = {}) {
      assert.equal(await frame.locator(selector).evaluate((el, attributes) => {
        const e = new KeyboardEvent('keydown', { key: 'q', code: 'KeyQ', ctrlKey: true, bubbles: true, cancelable: true, ...attributes });
        el.dispatchEvent(e);
        return e.defaultPrevented;
      }, attributes), false, selector + ': shortcut must pass through untouched ' + JSON.stringify(attributes));
    }
    async function focusBoard(frame) { await frame.locator('.board-overview').focus(); }
    for (const lang of ['ko', 'en']) {
      await context.addCookies([{ name: 'ws_dashboard_lang', value: lang, url: base }]);
      await tab.goto(base + '/s/livesession');
      let frame = await active('livesession');
      const label = lang === 'ko' ? '한 눈에 보기' : 'Overview';
      assert.equal(await frame.locator('.board-overview .board-label').textContent(), label);
      assert.equal(await frame.locator('.board-overview').getAttribute('aria-label'), label, 'visible/accessibility label is unchanged');
      assert.equal(await frame.locator('.board-overview').getAttribute('title'), label + ' (Ctrl+Q)');
      await state(frame, false);
      await focusBoard(frame);
      await tab.keyboard.press('Control+q');
      await state(frame, true);
      if (shots && lang === 'ko') {
        fs.mkdirSync(shots, { recursive: true });
        await tab.screenshot({ path: path.join(shots, 'overview-ctrl-q-light-1280.png') });
      }
      // Keyboard entry uses the toolbar's session-scoped persistence, too.
      await tab.reload();
      frame = await active('livesession');
      await state(frame, true);
      await focusBoard(frame);
      await tab.keyboard.press('Control+q');
      await state(frame, false);
      await tab.keyboard.press('Meta+q');
      await state(frame, false);

      // A non-editing focus in the chat column still controls the board.
      await frame.locator('#convo .bub').first().evaluate(el => { el.tabIndex = -1; el.focus(); });
      await tab.keyboard.press('Control+q');
      await state(frame, true);
      await tab.keyboard.press('Control+q');
      await state(frame, false);

      // Rail/header focus is in the outer document, not the session iframe.
      await tab.locator('#railbtn').focus();
      await tab.keyboard.press('Control+q');
      await state(frame, true);
      await tab.locator('#railbtn').focus();
      await tab.keyboard.press('Control+q');
      await state(frame, false);

      // Clicking a topic body focuses its opaque sandbox, one frame deeper.
      const bodyElement = frame.locator('iframe.doc-frame').first();
      await bodyElement.waitFor({ state: 'visible' });
      const bodyFrame = await (await bodyElement.elementHandle()).contentFrame();
      await bodyFrame.locator('.doc-body').click();
      await tab.keyboard.press('Meta+q');
      await state(frame, false);
      for (const attributes of [{ shiftKey: true }, { altKey: true }, { metaKey: true }, { isComposing: true }, { keyCode: 229 }]) {
        await uncanceled(bodyFrame, '.doc-body', attributes);
        await state(frame, false);
      }
      await bodyFrame.evaluate(() => {
        const input = document.createElement('input'); input.id = 'body-edit'; document.body.appendChild(input);
      });
      await uncanceled(bodyFrame, '#body-edit');
      await state(frame, false);
      await bodyFrame.locator('#body-edit').evaluate(el => el.remove());
      await bodyFrame.locator('.doc-body').click();
      await tab.keyboard.press('Control+q');
      await state(frame, true);
      await tab.keyboard.press('Control+q');
      await state(frame, false);

      // Both modifier and IME exclusions must remain uncanceled in either document.
      for (const [page, selector] of [[frame, '.board-overview'], [tab, '#railbtn']]) {
        for (const attributes of [{ ctrlKey: false }, { ctrlKey: false, metaKey: true }, { shiftKey: true }, { altKey: true }, { metaKey: true }, { isComposing: true }, { keyCode: 229 }]) {
          await uncanceled(page, selector, attributes);
          await state(frame, false);
        }
      }
      // The composer has both a backing textarea and a rich editable surface.
      await uncanceled(frame, '#ctext');
      await frame.locator('#ctext').evaluate(el => {
        el.classList.remove('wsc-backing');
        el.style.cssText = 'display:block;position:fixed;top:80px;left:300px;width:250px;height:50px;z-index:9999';
        el.value = 'Composer text stays native'; el.focus();
      });
      await tab.keyboard.press('Control+q');
      await state(frame, false);
      assert.equal(await frame.locator('#ctext').inputValue(), 'Composer text stays native');
      await frame.locator('#ctext').evaluate(el => { el.value = ''; el.style.cssText = ''; el.blur(); el.dispatchEvent(new Event('input', { bubbles: true })); });
      await uncanceled(frame, '#composer .wsc-editor');
      await frame.locator('#composer .wsc-editor').focus();
      await tab.keyboard.press('Control+q');
      await state(frame, false);

      // Synthetic boundary targets, not a real terminal process or a renamed session.
      for (const page of [frame, tab]) {
        await page.evaluate(() => {
          const fixture = document.createElement('div'); fixture.id = 'shortcut-targets';
          fixture.innerHTML = '<input class="rename"><div contenteditable="true"><span class="editable-child">Editable</span></div><div class="tm-host"><div class="xterm" tabindex="0">Terminal boundary</div></div>';
          document.body.appendChild(fixture);
        });
        for (const selector of ['#shortcut-targets input', '#shortcut-targets .editable-child', '#shortcut-targets .xterm', '#shortcut-targets .tm-host']) {
          await uncanceled(page, selector);
          await state(frame, false);
        }
        await page.locator('#shortcut-targets').evaluate(el => el.remove());
      }
      // Only the same-origin parent may forward a toggle to this frame.
      await frame.evaluate(() => {
        for (const [source, origin] of [[window, location.origin], [window.parent, 'https://invalid.example']]) {
          window.dispatchEvent(new MessageEvent('message', { data: { wsShell: 'overview' }, source, origin }));
        }
      });
      await state(frame, false);
      // A board hidden by the session layout must not consume the key in either document.
      await frame.locator('#bcol').evaluate(el => el.style.display = 'none');
      await uncanceled(frame, 'body');
      await uncanceled(tab, '#railbtn');
      await state(frame, false);
      await frame.locator('#bcol').evaluate(el => el.style.display = '');
      console.log(lang + ': Ctrl+Q on/off from board, sandboxed topic body, chat and shell; persistence, tooltip, native editing/terminal/modifier/IME exclusions and parent validation passed.');
    }
    // A different visible session is the recipient, not a previously selected board.
    await tab.locator('#railbtn').click();
    await tab.locator('.rs[data-rid="boardsession"] .rl').click();
    const board = await active('boardsession');
    await tab.locator('#railbtn').focus();
    await tab.keyboard.press('Control+q');
    await state(board, true, 'boardsession');
    assert.equal(await tab.evaluate(() => sessionStorage.getItem('ws.dashboard.overview.livesession')), null);
    await tab.locator('#railbtn').focus();
    await tab.keyboard.press('Control+q');
    await state(board, false, 'boardsession');

    // Home has no board: neither the shell nor its index frame claims Ctrl+Q.
    await tab.goto(base + '/');
    await tab.locator('iframe.sframe.on').waitFor();
    const home = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await home.locator('.dlanding').waitFor();
    await uncanceled(tab, '#railbtn');
    await uncanceled(home, 'body');
    assert.deepEqual(errors, []);
    await context.close();
    console.log('Visible-session targeting and no-board pass-through passed.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
