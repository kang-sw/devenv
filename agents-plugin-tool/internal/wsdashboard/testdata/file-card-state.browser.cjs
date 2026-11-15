// Invoked only by TestFileCardThreeStateBrowser's disposable HTTP fixture.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, root] = process.argv.slice(2);
const evidence = process.env.WS_FILECARD_EVIDENCE_DIR || path.join(require('node:os').tmpdir(), 'ws-file-card-state');
const route = '/w/' + root.replace(/^\//, '').split('/').map(encodeURIComponent).join('/') + '?frame=1';
(async () => {
  fs.mkdirSync(evidence, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1700, height: 1000 }, locale: 'en-US', reducedMotion: 'reduce' });
    const tab = await context.newPage(), errors = [];
    tab.on('pageerror', e => errors.push(e.message));
    let page;
    async function active() {
      await tab.locator('iframe.sframe.on').waitFor();
      page = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
      await page.waitForFunction(() => !!window.WsCards?.toolbar());
    }
    await tab.goto(base + route);
    await active();
    const row = name => page.locator('#files .fentry[data-entry=' + JSON.stringify(name) + ']');
    const card = id => page.locator('.bcard[data-card=' + JSON.stringify(id) + ']');
    const list = () => page.evaluate(() => window.WsCards.list('preview').map(c => ({ id: c.id, data: c.data, state: c.el.getAttribute('data-file-state') })));
    const wait = async (file, state) => {
      await page.waitForFunction(({ file, state }) => window.WsCards?.list('preview').some(c => c.data.path === file && c.el.getAttribute('data-file-state') === state && c.body.querySelector('.pv-txt, .pv-md')), { file, state });
      return (await list()).find(c => c.data.path === file);
    };
    await row('worktree_page.go').waitFor().catch(async e => {
      await tab.screenshot({ path: path.join(evidence, 'failure.png') });
      fs.writeFileSync(path.join(evidence, 'failure.html'), await page.content());
      console.error('page errors:', errors); throw e;
    });
    await row('worktree_page.go').click();
    const first = await wait('worktree_page.go', 'preview');
    await row('worktree_page_test.go').click();
    assert.equal((await wait('worktree_page_test.go', 'preview')).id, first.id, 'one preview slot replaces in place');
    assert.equal((await list()).length, 1);
    assert.equal(await row('worktree_page.go').locator('.fstate').isHidden(), true, 'replaced file loses marker');
    assert.equal(await row('worktree_page_test.go').locator('.fstate').textContent(), 'Preview');
    await row('worktree_page_test.go').dblclick();
    assert.equal((await wait('worktree_page_test.go', 'open')).id, first.id, 'double click promotes in place');
    await row('worktree_page.go').click();
    const second = await wait('worktree_page.go', 'preview');
    assert.equal((await list()).length, 2, 'open survives next preview');
    await card(second.id).locator('.bc-nm').dblclick();
    assert.equal((await wait('worktree_page.go', 'open')).id, second.id, 'header double click promotes');
    await row('WORKFLOW.md').click();
    const pinned = await wait('WORKFLOW.md', 'preview');
    await card(pinned.id).locator('.bc-pin').click();
    await wait('WORKFLOW.md', 'pinned');
    assert.equal(await card(pinned.id).evaluate(n => n.closest('.bpcol')?.id), 'bpcol');
    assert.equal(await row('WORKFLOW.md').locator('.fstate').textContent(), 'Pinned');
    await row('another.txt').click();
    const next = await wait('another.txt', 'preview');
    assert.notEqual(next.id, pinned.id, 'pin vacates preview slot');
    await card(pinned.id).locator('.bc-pin').click();
    await wait('WORKFLOW.md', 'open');
    assert.equal((await wait('another.txt', 'preview')).id, next.id, 'unpin does not evict preview');
    await card(pinned.id).locator('.bc-pin').click();
    await row('worktree_page.go').click();
    await wait('worktree_page.go', 'open');
    assert.equal((await wait('another.txt', 'preview')).id, next.id, 'clicking open card does not demote it');
    // Both directory layouts move focus only on arrows, never preview/open.
    await row('worktree_page.go').focus();
    const beforeKeys = await list();
    await row('worktree_page.go').press('ArrowDown');
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-entry')), 'worktree_page_test.go');
    assert.deepEqual(await list(), beforeKeys);
    await row('another.txt').focus(); await row('another.txt').press('Enter');
    assert.equal((await wait('another.txt', 'open')).id, next.id, 'Enter promotes in place');
    assert.equal(await row('another.txt').locator('.fstate').textContent(), 'Open');
    await page.locator('#files .fview-modes button').nth(4).click();
    await row('worktree_page.go').focus(); await row('worktree_page.go').press('ArrowUp');
    assert.deepEqual(await list(), beforeKeys.map(c => c.id === next.id ? { ...c, state: 'open' } : c));
    await page.locator('#files .fview-modes button').nth(0).click();
    // A preview is saved with the old storage shape but restored as open.
    await card(next.id).locator('.bc-close').click();
    assert.equal(await row('another.txt').locator('.fstate').isHidden(), true, 'closing clears marker');
    await row('another.txt').click(); await wait('another.txt', 'preview');
    const stored = await page.evaluate(root => JSON.parse(localStorage.getItem('ws.dashboard.cards.wt:' + root)), root);
    for (const c of stored.cards) {
      assert.deepEqual(Object.keys(c).sort(), ['at', 'data', 'fold', 'h', 'id', 'kind', 'pinned']);
      assert.deepEqual(Object.keys(c.data).sort(), ['path', 'root']);
    }
    await tab.reload(); await active(); await row('another.txt').waitFor(); await wait('another.txt', 'open');
    assert(!(await list()).some(c => c.state === 'preview'), 'reload never restores preview');
    await wait('WORKFLOW.md', 'pinned');
    const restoredAnother = (await list()).find(c => c.data.path === 'another.txt');
    await card(restoredAnother.id).locator('.bc-close').click();
    await page.locator('#files .fq').fill('another');
    await page.locator('#files .hit').waitFor();
    await page.locator('#files .fq').press('Enter');
    await wait('another.txt', 'open');
    await page.locator('#files .fq').press('Escape');
    // Open-card count is unbounded by a preview eviction policy.
    await page.evaluate(root => { for (let i = 0; i < 30; i++) window.WsPreview.open('worktree_page.go', { root: root + '/count-' + i, keep: true }); }, root);
    assert.equal((await list()).length, 34);
    await page.evaluate(() => window.WsCards.list('preview').filter(c => /count-/.test(c.data.root)).forEach(c => c.close()));
    // Arrange the visual fixture like the owner's mockup: open, preview, pin.
    await page.evaluate(() => window.WsCards.list('preview').filter(c => /another|_test/.test(c.data.path)).forEach(c => c.close()));
    await row('worktree_page_test.go').click(); await wait('worktree_page_test.go', 'preview');
    const openCard = (await list()).find(c => c.data.path === 'worktree_page.go');
    await card(openCard.id).locator('.bc-raise').click();
    await page.waitForTimeout(1100); // allow the normal card-show flash to expire
    async function capture(theme, width, filename) {
      await tab.setViewportSize({ width, height: width < 800 ? 1000 : 820 });
      await tab.evaluate(theme => document.documentElement.setAttribute('data-theme', theme), theme);
      await page.evaluate(theme => document.documentElement.setAttribute('data-theme', theme), theme);
      await page.evaluate(() => { document.querySelector('#bcol').scrollTop = 0; });
      await page.waitForTimeout(100);
      await tab.screenshot({ path: path.join(evidence, filename) });
    }
    await capture('dark', 1700, 'after-dark.png');
    await capture('light', 1700, 'after-light.png');
    await capture('dark', 700, 'after-narrow.png');
    const placement = await page.evaluate(() => {
      const cards = window.WsCards.list('preview'), preview = cards.find(c => c.preview), board = document.querySelector('[data-region="board"]');
      return { first: [...board.children].find(n => n.matches('.bcard, .bpins')) === preview.el, noPinColumn: !document.documentElement.hasAttribute('data-pincol'), italic: getComputedStyle(preview.name).fontStyle, outline: getComputedStyle(preview.el.querySelector('.bc-in')).outlineStyle };
    });
    assert.deepEqual(placement, { first: true, noPinColumn: true, italic: 'italic', outline: 'dashed' });
    fs.writeFileSync(path.join(evidence, 'placement.json'), JSON.stringify(placement, null, 2));
    for (const width of [1599, 1600]) {
      await tab.setViewportSize({ width, height: 1000 });
      await page.waitForFunction(wide => document.documentElement.hasAttribute('data-pincol') === wide, width >= 1600);
    }
    // Both existing link entry points keep their default preview semantics.
    await tab.setViewportSize({ width: 1700, height: 1000 });
    await tab.goto(base + '/s/livesession');
    await active();
    await page.evaluate(() => {
      const a = document.createElement('a'); a.textContent = 'Board file link'; a.href = '/s/livesession/fs/view?path=worktree_page.go';
      a.setAttribute('data-wl', 'file'); a.setAttribute('data-rel', 'worktree_page.go');
      a.id = 'fixture-board-link'; document.querySelector('[data-region="board"]').appendChild(a);
    });
    await page.locator('#fixture-board-link').click();
    const boardPreview = await wait('worktree_page.go', 'preview');
    await page.locator('.board-search').click();
    await page.locator('#tkqin').fill('worktree_page_test');
    const result = page.locator('[data-qs="files"][data-qi]').filter({ hasText: 'worktree_page_test.go' });
    await result.first().click().catch(async e => {
      await tab.screenshot({ path: path.join(evidence, 'tracker-failure.png') });
      fs.writeFileSync(path.join(evidence, 'tracker-failure.html'), await page.content());
      console.error('page errors:', errors); throw e;
    });
    assert.equal((await wait('worktree_page_test.go', 'preview')).id, boardPreview.id, 'tracker file link reuses preview slot');
    assert.equal((await list()).length, 1);
    assert.deepEqual(errors, []);
    console.log('PASS: preview replacement, explorer/header promotion, pin/unpin slot, open retention/no cap, Enter/arrows, board/tracker preview, unchanged storage/reload, narrow leading placement, state markers, dark/light/narrow captures.');
    await context.close();
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
