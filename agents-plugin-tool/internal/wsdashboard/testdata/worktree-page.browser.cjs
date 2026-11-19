// Driven only by TestWorktreePageBrowser's disposable daemon and holders.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, pathsJSON] = process.argv.slice(2), paths = JSON.parse(pathsJSON);
const route = p => '/w/' + p.replace(/\\/g, '/').replace(/^\//, '').split('/').map(encodeURIComponent).join('/').replace(/^([A-Za-z])%3A/, '$1:');
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 2400, height: 1000 }, locale: 'en-US' });
    const tab = await context.newPage(), errors = [], terminalPosts = [], lists = [];
    tab.on('pageerror', e => { errors.push(e.message); console.error('page error:', e.message); });
    tab.on('request', r => {
      const u = new URL(r.url());
      if (u.pathname === '/s/host0/terminals') {
        if (r.method() === 'POST') terminalPosts.push(r.postDataJSON());
        else lists.push(u.searchParams.get('folder'));
      }
    });
    let page;
    async function active(path) {
      await tab.waitForURL(base + route(path));
      await tab.waitForFunction(p => document.querySelector('iframe.sframe.on')?.contentDocument?.documentElement.getAttribute('data-worktree') === p, path);
      page = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
      await page.locator('#files .fentry[data-entry="worktree.txt"]').waitFor();
    }
    function heading(path) { return tab.locator('.rwt[data-g=' + JSON.stringify(path) + '] .wthd'); }
    async function open(path, key = false) {
      if (key) { await heading(path).focus(); await heading(path).press('Enter'); }
      else await heading(path).click({ position: { x: 50, y: 12 } });
      await active(path);
    }
    async function cards(kind) { return page.evaluate(k => window.WsCards.list(k).map(c => ({ id: c.id, data: c.data, pinned: !!c.pinned })), kind); }
    async function makeTerminal(path) {
      await page.locator('.tnew').click();
      await page.waitForFunction(() => window.WsCards.list('terminal').length === 1 && !!window.WsCards.list('terminal')[0].t.term);
      const terminal = page.locator('.bcard[data-kind="terminal"] textarea.xterm-helper-textarea');
      await terminal.focus();
      await terminal.pressSequentially('echo WV-$((20+2)); pwd', { delay: 10 });
      await terminal.press('Enter');
      await page.waitForFunction(p => {
        const b = window.WsCards.list('terminal')[0].t.term.buffer.active;
        const text = Array.from({ length: b.length }, (_, i) => b.getLine(i).translateToString(true)).join('');
        return text.includes('WV-22') && text.includes(p);
      }, path);
      const list = await (await tab.request.get(base + '/s/host0/terminals?folder=' + encodeURIComponent(path))).json();
      assert.equal(list.terminals.length, 1);
      assert.equal(list.terminals[0].cwd, path);
      assert.equal(list.terminals[0].id, (await cards('terminal'))[0].data.tid);
      assert.equal(list.orphans?.length || 0, 0);
      return list.terminals[0].id;
    }
    await tab.goto(base + '/');
    await heading(paths[0]).waitFor();
    // Shared rail markup also reaches remote viewers; its local flag is
    // what disables both navigation gestures there (the server guard is
    // independently exercised by TestWorktreePageGuardsAndTemplate).
    await tab.evaluate(() => document.documentElement.removeAttribute('data-sysopen'));
    await heading(paths[0]).click({ position: { x: 50, y: 12 } });
    await heading(paths[0]).focus(); await heading(paths[0]).press('Enter');
    assert.equal(new URL(tab.url()).pathname, '/');
    await tab.evaluate(() => document.documentElement.setAttribute('data-sysopen', '1'));
    await open(paths[0]);
    fs.mkdirSync('/tmp/exec5/wv-impl', { recursive: true });
    await tab.screenshot({ path: '/tmp/exec5/wv-impl/first-open.png' });
    assert.equal(terminalPosts.length, 0, 'first visit never starts a shell');
    assert.equal((await cards('terminal')).length, 0);
    assert.equal(await page.locator('#convo, #composer, #tabbar, #r-tree, .topic, .board-overview, .board-state-filter, .board-web-slot button').count(), 0);
    assert.equal(await page.locator('.fattachment button:visible').count(), 1, 'copy only; send-to-session is hidden');
    await page.locator('#files .fentry[data-entry="worktree.txt"]').dblclick();
    await page.locator('.bcard[data-kind="preview"] .pv-txt').waitFor({ state: 'visible' });
    assert((await page.locator('.bcard[data-kind="preview"]').textContent()).includes(paths[0]), 'live preview reads this root');
    await page.locator('.board-explorer').click();
    const explorer = page.locator('.bcard[data-kind="explorer"]');
    await explorer.locator('.fentry[data-entry="worktree.txt"]').waitFor();
    await explorer.locator('.fview-modes button').nth(2).click();
    const explorerID = (await cards('explorer'))[0].id;
    const tid0 = await makeTerminal(paths[0]);
    // Search has index-page scope, not host0/session file or answer hooks.
    assert.equal(await page.locator('.board-download').count(), 0, 'worktree page has no board zip to download');
    assert.equal(await page.locator('#tkqs').count(), 0, 'worktree session page omits duplicate #tkqs button');
    assert.equal(await tab.locator('#tkqs').count(), 0, 'shell header mirrors no #tkqs button on worktree page');
    await page.locator('#shandle').click();
    await page.locator('#tkqpop').waitFor({ state: 'visible' });
    await page.locator('#tkqin').press('Escape');
    await page.locator('#tkqpop').waitFor({ state: 'hidden' });
    await tab.setViewportSize({ width: 1800, height: 1000 });
    await tab.screenshot({ path: '/tmp/exec5/wv-impl/worktree-page.png' });
    await tab.setViewportSize({ width: 2400, height: 1000 });
    await explorer.locator('.bc-pin').click();
    await page.locator('.bcard[data-kind="terminal"] .bc-pin').click();
    await tab.screenshot({ path: '/tmp/exec5/wv-impl/pinned-page.png' });
    // Drag uses the existing threshold and must not become a navigation.
    const hd = await heading(paths[1]).boundingBox(), before = tab.url();
    await tab.mouse.move(hd.x + 45, hd.y + hd.height / 2); await tab.mouse.down();
    await tab.mouse.move(hd.x + 80, hd.y + hd.height / 2 + 12, { steps: 5 }); await tab.mouse.up();
    await tab.waitForTimeout(150);
    assert.equal(tab.url(), before, 'drag does not navigate');
    await open(paths[1], true);
    assert.equal((await cards('explorer')).length, 0, 'second worktree does not restore the first explorer');
    assert.equal((await cards('terminal')).length, 0, 'filtered list does not import another worktree terminal');
    await page.locator('#files .fentry[data-entry="worktree.txt"]').dblclick();
    await page.locator('.bcard[data-kind="preview"] .pv-txt').waitFor({ state: 'visible' });
    assert((await page.locator('.bcard[data-kind="preview"]').textContent()).includes(paths[1]), 'second live preview uses its own root');
    const tid1 = await makeTerminal(paths[1]);
    assert.notEqual(tid0, tid1);
    await tab.reload(); await active(paths[1]);
    await page.waitForFunction(() => window.WsCards.list('terminal').length === 1);
    assert.equal((await cards('terminal'))[0].data.tid, tid1);
    assert.equal((await cards('explorer')).length, 0);
    await open(paths[0]);
    await page.waitForFunction(() => window.WsCards.list('terminal').length === 1 && window.WsCards.list('explorer').length === 1);
    assert.equal((await cards('terminal'))[0].data.tid, tid0);
    assert.equal((await cards('terminal'))[0].pinned, true);
    assert.equal((await cards('explorer'))[0].id, explorerID);
    assert.equal((await cards('explorer'))[0].pinned, true);
    assert.equal(await page.locator('.bcard[data-kind="explorer"] .fdirectory').getAttribute('data-view'), 'icon');
    await tab.reload(); await active(paths[0]);
    await page.waitForFunction(() => window.WsCards.list('terminal').length === 1 && window.WsCards.list('explorer').length === 1);
    assert.equal((await cards('terminal'))[0].data.tid, tid0);
    await page.locator('.bcard[data-kind="preview"] .pv-txt').waitFor({ state: 'visible' });
    assert((await page.locator('.bcard[data-kind="preview"]').textContent()).includes(paths[0]), 'restored preview remints through host0');
    const keys = await tab.evaluate(() => Object.keys(localStorage));
    for (const path of paths) assert(keys.includes('ws.dashboard.cards.wt:' + path));
    assert(keys.some(k => k.startsWith('ws.dashboard.explorer.wt:' + paths[0] + '.')));
    for (const path of paths) assert(keys.includes('ws.dashboard.files.tabs.wt:' + path));
    assert(!keys.some(k => /(?:cards|explorer).*host0/.test(k)), 'anchor never owns browser state');
    assert(lists.length >= 2 && lists.every(p => paths.includes(p)), 'every terminal reconciliation is folder-filtered');
    assert.equal(terminalPosts.length, 2, 'reload and revisit never auto-spawn');
    assert.deepEqual(terminalPosts.map(p => [p.folder, p.path]), paths.map(p => ['path', p]));
    // Closing a browser page does not close host-owned shells.
    await tab.close();
    const revisit = await context.newPage();
    await revisit.goto(base + route(paths[0]));
    const frame = await (await revisit.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await frame.waitForFunction(() => window.WsCards?.list('terminal').length === 1);
    assert.equal(await frame.evaluate(() => window.WsCards.list('terminal')[0].data.tid), tid0);
    // Session consumer remains separate from both worktrees.
    await revisit.goto(base + '/s/livesession');
    const session = await (await revisit.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await session.locator('#ctext').waitFor({ state: 'attached' });
    assert.equal(await session.getAttribute('html', 'data-worktree'), null);
    await session.waitForFunction(() => !!window.WsCards);
    assert.equal(await session.evaluate(() => window.WsCards.list('terminal').length), 0);
    assert.deepEqual(errors, []);
    console.log('PASS: two-root files, pwd, exact terminal lists, per-worktree cards/pins/reload, click/Enter/drag, search, lifetime and session isolation.');
    await context.close();
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
