const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, controlURL] = process.argv.slice(2);
const shots = process.env.WS_CHAT_TERM_SHOTS;

async function until(fn, what, ms = 10000) {
  const limit = Date.now() + ms;
  for (;;) {
    try { const v = await fn(); if (v) return v; } catch (e) {}
    if (Date.now() > limit) throw new Error('timed out: ' + what);
    await new Promise(r => setTimeout(r, 40));
  }
}

(async () => {
  if (shots) fs.mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'ko-KR' });
    await context.addCookies([
      { name: 'ws_dashboard_theme', value: 'dark', url: base },
      { name: 'ws_dashboard_lang', value: 'ko', url: base }
    ]);
    const tab = await context.newPage(), errors = [], termPosts = [], termCloses = [], wsOpens = [];
    async function capture(name) { if (shots) await tab.screenshot({ path: shots + '/' + name }); }
    async function setTheme(theme) {
      await tab.evaluate(theme => {
        const button = document.querySelector('[data-theme-set="' + theme + '"]');
        if (!button) throw new Error('missing theme toggle: ' + theme);
        button.click();
      }, theme);
      await until(() => tab.evaluate(theme => document.documentElement.getAttribute('data-theme') === theme, theme), theme + ' shell theme');
      await until(() => page.evaluate(theme => document.documentElement.getAttribute('data-theme') === theme, theme), theme + ' session theme');
    }
    tab.on('pageerror', e => { errors.push(e.message); console.error('pageerror:', e.message); });
    tab.on('request', r => {
      const u = new URL(r.url());
      if (u.pathname === '/s/livesession/terminals' && r.method() === 'POST') termPosts.push(r.postDataJSON());
      if (/^\/s\/livesession\/terminals\/[^/]+\/close$/.test(u.pathname) && r.method() === 'POST') termCloses.push(u.pathname);
    });
    tab.on('websocket', ws => {
      if (ws.url().includes('/terminals/')) wsOpens.push(ws.url());
    });

    await tab.goto(base + '/s/livesession');
    const frameHandle = await tab.locator('iframe.sframe.on').elementHandle();
    const page = await frameHandle.contentFrame();
    await page.locator('#ctabs [data-ctab-add-term]').waitFor({ state: 'visible' });

    const initialTabs = await page.evaluate(() => [...document.querySelectorAll('#ctabs > *')].map(el => el.getAttribute('data-ctab') || (el.hasAttribute('data-ctab-add-term') ? 'add-term' : el.className)));
    assert.deepEqual(initialTabs, ['convo', 'files', 'add-term']);
    assert.equal(await page.evaluate(() => window.wsCanCacheSession()), true);
    assert.equal(await page.locator('#agisland').isVisible(), true);
    assert.equal(await page.locator('#chead .csum').isVisible(), true);
    assert.equal(await page.locator('.bcard[data-kind="terminal"]').count(), 0);
    await capture('default-ctabs.png');
    await setTheme('light');
    await capture('default-ctabs-light.png');
    await setTheme('dark');

    // 1. One-click + opens Terminal 1 in grid-area: convo without duplicating a .bcard
    await page.locator('#ctabs [data-ctab-add-term]').click();
    await page.locator('#ctabs .d-ctab-term').waitFor({ state: 'visible' });
    assert.equal(termPosts.length, 1);
    assert.equal(termPosts[0].folder, 'worktree');
    assert.equal(await page.locator('#grid').getAttribute('data-ctab'), 'term');
    assert.equal(await page.locator('#chead .csum').isVisible(), false);
    assert.equal(await page.locator('#chead .fsum').isVisible(), false);
    assert.equal(await page.locator('#agisland').isVisible(), false);
    assert.equal(await page.evaluate(() => window.wsCanCacheSession()), false);
    assert.equal(await page.locator('.bcard[data-kind="terminal"]').count(), 0);

    await until(() => page.evaluate(() => {
      const pane = document.querySelector('#cterm-panes .d-tpane[data-active]');
      return !!(pane && pane.querySelector('.xterm-helper-textarea'));
    }), 'tab 1 xterm mount');
    const tid1 = await page.locator('#ctabs .d-ctab-term').first().getAttribute('data-ctab-tid');
    await page.evaluate(() => {
      const pane = document.querySelector('#cterm-panes .d-tpane[data-active]');
      window.__xtermEl1 = pane.querySelector('.xterm');
    });

    const input1 = page.locator('#cterm-panes .d-tpane[data-active] textarea.xterm-helper-textarea');
    await input1.focus();
    await input1.pressSequentially('echo TAB1-OK', { delay: 10 });
    await input1.press('Enter');
    await until(() => page.evaluate(() => (document.querySelector('#cterm-panes .d-tpane[data-active]')?.textContent || '').includes('TAB1-OK')), 'TAB1-OK output');

    // 2. Switch to 대화 -> verify xterm DOM/WS preserved + tdot lights up on incoming entry while on terminal tab
    await page.locator('#ctabs [data-ctab="convo"]').click();
    assert.equal(await page.locator('#grid').getAttribute('data-ctab'), 'convo');
    assert.equal(await page.locator('#convo').isVisible(), true);
    assert.equal(await page.locator('#cterm-panes').isVisible(), false);
    assert.equal(await page.locator('#agisland').isVisible(), true);
    assert.equal(await page.evaluate(() => document.querySelector('#cterm-panes .d-tpane .xterm') === window.__xtermEl1), true);

    await page.locator('#ctabs [data-ctab-term]').first().click();
    assert.equal(await page.locator('#grid').getAttribute('data-ctab'), 'term');
    assert.equal(await page.evaluate(() => document.querySelector('#cterm-panes .d-tpane[data-active] .xterm') === window.__xtermEl1), true);
    assert.equal(wsOpens.length, 1, 'switching back must reuse the existing WebSocket');
    assert.equal(await page.evaluate(() => document.activeElement === document.querySelector('#cterm-panes .d-tpane[data-active] textarea.xterm-helper-textarea')), true, 're-activating terminal tab focuses xterm');

    await fetch(controlURL, { method: 'POST' });
    await until(() => page.evaluate(() => !!document.querySelector('#ctabs [data-ctab="convo"] .tdot.on')), 'convo tdot lights up while terminal tab active');

    // 3. Add Terminal 2 + capture active-term-tabs.png
    const storeKey = await page.evaluate(() => Object.keys(localStorage).find(k => k.startsWith('ws.dashboard.cterm.')));
    assert.ok(storeKey, 'chat terminal tab placement is saved');
    await page.evaluate(key => {
      const tabs = JSON.parse(localStorage.getItem(key));
      tabs.push({ id: 'other-window-tab', num: 1 });
      localStorage.setItem(key, JSON.stringify(tabs));
    }, storeKey);
    await page.locator('#ctabs [data-ctab-add-term]').click();
    await until(() => page.evaluate(() => document.querySelectorAll('#ctabs .d-ctab-term').length === 2), 'tab 2 added');
    assert.equal(await page.evaluate(key => JSON.parse(localStorage.getItem(key)).some(x => x.id === 'other-window-tab'), storeKey), true, 'adding a tab preserves another window\'s saved tab');
    await page.evaluate(key => localStorage.setItem(key, JSON.stringify(JSON.parse(localStorage.getItem(key)).filter(x => x.id !== 'other-window-tab'))), storeKey);
    assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('#ctabs .d-ctab-btn .d-ctab-label')].map(n => n.textContent)), ['터미널 1', '터미널 2']);
    assert.equal(await page.locator('.bcard[data-kind="terminal"]').count(), 0);
    await until(() => page.evaluate(() => {
      const pane = document.querySelector('#cterm-panes .d-tpane[data-active]');
      return !!(pane && pane.querySelector('.xterm-helper-textarea'));
    }), 'tab 2 xterm mount');
    const input2 = page.locator('#cterm-panes .d-tpane[data-active] textarea.xterm-helper-textarea');
    await input2.focus();
    await input2.pressSequentially('echo TAB2-OK', { delay: 10 });
    await input2.press('Enter');
    await until(() => page.evaluate(() => (document.querySelector('#cterm-panes .d-tpane[data-active]')?.textContent || '').includes('TAB2-OK')), 'TAB2-OK output');
    await capture('active-term-tabs.png');
    await setTheme('light');
    await capture('active-term-tabs-light.png');
    await setTheme('dark');

    // 4. Coexistence with board terminal card + reload persistence
    await page.locator('#board-term-slot .tnew').click();
    await page.locator('#board-term-slot .tpop .topt').first().click();
    await until(() => page.evaluate(() => document.querySelectorAll('.bcard[data-kind="terminal"]').length === 1), 'board terminal card created');
    assert.equal(await page.locator('#ctabs .d-ctab-term').count(), 2);

    await tab.reload();
    const page2 = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await until(() => page2.evaluate(() => document.querySelectorAll('#ctabs .d-ctab-term').length === 2 && document.querySelectorAll('.bcard[data-kind="terminal"]').length === 1), 'reload restores 2 tabs and 1 card');
    await page2.locator(`#ctabs [data-ctab-term="${tid1}"]`).click();
    await until(() => page2.evaluate(() => (document.querySelector('#cterm-panes .d-tpane[data-active]')?.textContent || '').includes('TAB1-OK')), 'reloaded tab 1 replays scrollback');

    // 5. Close tabs via × (tab 2 -> switches to tab 1, then tab 1 -> switches to convo)
    const dialog = page2.locator('dialog.ws-confirm');
    const cancelClose = dialog.getByRole('button', { name: '취소', exact: true });
    const acceptClose = dialog.getByRole('button', { name: '확인', exact: true });
    const closeSecond = page2.locator('#ctabs .d-ctab-close').nth(1);
    await closeSecond.click();
    await cancelClose.waitFor({ state: 'visible' });
    assert.equal(await cancelClose.evaluate(b => b === document.activeElement), true);
    await cancelClose.press('Escape');
    await dialog.waitFor({ state: 'detached' });
    assert.equal(await closeSecond.evaluate(b => b === document.activeElement), true);
    assert.equal(termCloses.length, 0);
    await closeSecond.click();
    await cancelClose.click();
    await dialog.waitFor({ state: 'detached' });
    assert.equal(await page2.locator('#ctabs .d-ctab-term').count(), 2, 'cancelled close keeps running tab');
    assert.equal(termCloses.length, 0);
    await closeSecond.click();
    await closeSecond.evaluate(b => b.click());
    assert.equal(await dialog.count(), 1);
    await acceptClose.press('Enter');
    await until(() => page2.evaluate(() => document.querySelectorAll('#ctabs .d-ctab-term').length === 1), 'tab 2 closed');
    assert.equal(await page2.locator('#grid').getAttribute('data-ctab'), 'term');
    await page2.locator('#ctabs .d-ctab-close').first().click();
    await acceptClose.click();
    await until(() => page2.evaluate(() => document.querySelectorAll('#ctabs .d-ctab-term').length === 0), 'tab 1 closed');
    assert.equal(await page2.locator('#grid').getAttribute('data-ctab'), 'convo');
    assert.equal(await page2.evaluate(() => window.WsTerm.cacheSafe()), true);
    assert.equal(await page2.evaluate(() => window.wsCanCacheSession()), false, 'board terminal card still keeps session uncached');
    await page2.locator('.bcard[data-kind="terminal"] .bc-close').click();
    await cancelClose.press('Escape');
    await dialog.waitFor({ state: 'detached' });
    assert.equal(await page2.locator('.bcard[data-kind="terminal"]').count(), 1);
    await page2.locator('.bcard[data-kind="terminal"] .bc-close').click();
    await acceptClose.click();
    await until(() => page2.evaluate(() => document.querySelectorAll('.bcard[data-kind="terminal"]').length === 0), 'board terminal card closed');
    assert.equal(await page2.evaluate(() => window.wsCanCacheSession()), true);
    assert.equal(termCloses.length, 3);
    assert.equal(errors.length, 0, 'unexpected pageerrors: ' + errors.join('; '));
  } finally {
    await browser.close();
  }
})();
