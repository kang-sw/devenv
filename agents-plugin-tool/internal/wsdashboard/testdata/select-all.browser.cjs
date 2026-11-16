// Run by TestScopedSelectAllBrowser against disposable, production-rendered pages.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, standalone, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: 'light', reducedMotion: 'reduce' });
    const tab = await context.newPage(), errors = [];
    tab.setDefaultTimeout(10000);
    tab.on('pageerror', e => errors.push(e.message));
    const clear = page => page.evaluate(() => { document.activeElement.blur(); getSelection().removeAllRanges(); });
    const selected = page => page.evaluate(() => getSelection().toString());
    const key = mod => tab.keyboard.press(mod + '+a');
    async function assertScope(page, selector, exact = true) {
      const result = await page.evaluate(selector => {
        const el = document.querySelector(selector), s = getSelection(), r = s.getRangeAt(0);
        return { text: s.toString(), expected: el.innerText, exact: r.startContainer === el && r.startOffset === 0 && r.endContainer === el && r.endOffset === el.childNodes.length };
      }, selector);
      assert.equal(result.exact, true, selector + ': range covers only the chosen block');
      // Chromium serializes paragraph breaks around option buttons differently
      // for Selection and innerText; the range endpoints above remain exact.
      const comparable = text => exact ? text : text.trim().replace(/\s+/g, ' ');
      assert.equal(comparable(result.text), comparable(result.expected), selector + ': text excludes chrome and neighboring blocks');
    }
    async function clickSelect(page, selector, mod = 'Control', exact = true) {
      await clear(page);
      await page.locator(selector).first().click({ position: { x: 12, y: 12 } });
      await key(mod);
      await assertScope(page, selector, exact);
    }
    async function uncanceled(page, selector, attributes = {}) {
      assert.equal(await page.locator(selector).evaluate((el, attributes) => {
        const ev = new KeyboardEvent('keydown', { key: 'a', ctrlKey: true, bubbles: true, cancelable: true, ...attributes });
        el.dispatchEvent(ev);
        return ev.defaultPrevented;
      }, attributes), false, selector + ': retain native handling');
    }
    async function shot(name) {
      if (!shots) return;
      fs.mkdirSync(shots, { recursive: true });
      await tab.screenshot({ path: path.join(shots, name + '.png') });
    }

    // Standalone board: topic body, per-entry log, policy body and side panel.
    await tab.goto(standalone);
    await clickSelect(tab, '.doc-body');
    await shot('card-select-all-light-1280');
    await key('Control');
    await assertScope(tab, '.doc-body');
    await key('Meta');
    await assertScope(tab, '.doc-body');
    await tab.locator('.entry-text').hover();
    await key('Control');
    await assertScope(tab, '.doc-body'); // selection wins over pointer
    await clear(tab);
    await key('Control');
    await assertScope(tab, '.doc-body'); // last click wins over pointer
    await clickSelect(tab, '.entry-text');
    await clickSelect(tab, '.cbody', 'Control', false);
    await clickSelect(tab, '.panel > .body');

    // A caret in a different block wins over the last clicked block.
    await tab.evaluate(() => {
      const el = document.querySelector('.doc-body p');
      getSelection().collapse(el.firstChild, 2);
    });
    await key('Control');
    await assertScope(tab, '.doc-body');
    // Clicking chrome clears the remembered block; hovering alone is enough.
    await tab.locator('.top h1').click();
    await clear(tab);
    await tab.locator('.doc-body').hover();
    await key('Control');
    await assertScope(tab, '.doc-body');
    await tab.locator('.top h1').click();
    await key('Control');
    assert.equal(await selected(tab), '');
    await key('Meta');
    assert.equal(await selected(tab), '');
    assert.equal(await tab.locator('.top h1').evaluate(el => getComputedStyle(el).userSelect), 'none');
    assert.notEqual(await tab.locator('.cbar .id').evaluate(el => getComputedStyle(el).userSelect), 'none', 'copyable item IDs stay selectable');
    await uncanceled(tab, '#reply-text');

    // The dashboard is a real rail shell plus the normal session frame.
    await tab.goto(base + '/s/livesession');
    const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
    await frame.waitForFunction(() => !!window.WsCards?.toolbar());
    await clickSelect(frame, '.msg.user .bub');
    await key('Meta');
    await assertScope(frame, '.msg.user .bub');
    await clickSelect(frame, '.cbody', 'Control', false);
    await frame.locator('#chead').click({ position: { x: 5, y: 5 } });
    await key('Control');
    assert.equal(await selected(frame), '');
    await tab.locator('#railbtn').click();
    await tab.locator('.rhd').click({ position: { x: 5, y: 5 } });
    await key('Control');
    assert.equal(await selected(tab), '', 'outer rail never selects the page');
    await tab.keyboard.press('Escape');

    // Both the backing textarea and the rich composer retain native selection.
    await uncanceled(frame, '#ctext');
    const editor = frame.locator('#composer .wsc-editor');
    await editor.fill('Only the composer text');
    await uncanceled(frame, '#composer .wsc-editor');
    await key(process.platform === 'darwin' ? 'Meta' : 'Control');
    assert.equal(await selected(frame), 'Only the composer text');
    await frame.evaluate(() => {
      const ta = document.querySelector('#ctext'); ta.classList.remove('wsc-backing');
      ta.style.cssText = 'display:block;position:fixed;top:80px;left:300px;width:250px;height:50px;z-index:9999';
      ta.value = 'Textarea selection stays native'; ta.focus();
    });
    await key(process.platform === 'darwin' ? 'Meta' : 'Control');
    assert.deepEqual(await frame.locator('#ctext').evaluate(el => [el.selectionStart, el.selectionEnd]), [0, 'Textarea selection stays native'.length]);
    await frame.locator('#ctext').evaluate(el => el.style.cssText = '');

    // Preview content is the actual guarded file route and maximized card sheet.
    await clear(frame);
    await frame.evaluate(() => window.WsPreview.open('selection-scope.txt'));
    const preview = '.bcard[data-kind="preview"]';
    await frame.locator(preview + ' .pv-txt').waitFor();
    await clickSelect(frame, preview + ' .bc-body');
    await frame.locator(preview + ' .bc-max').click();
    await frame.locator(preview + '[data-max]').waitFor();
    await key('Control'); // toolbar focus must choose the pane, not the background
    await assertScope(frame, preview + ' .bc-body');
    await shot('file-preview-select-all-light-1280');
    await key('Meta');
    await assertScope(frame, preview + ' .bc-body');
    // Even a stale/programmatic selection behind an open sheet is rejected.
    await frame.evaluate(() => {
      const r = document.createRange(); r.selectNodeContents(document.querySelector('.msg.user .bub'));
      getSelection().removeAllRanges(); getSelection().addRange(r);
    });
    await key('Control');
    await assertScope(frame, preview + ' .bc-body');
    assert.notEqual(await frame.locator(preview + ' .bc-sub').evaluate(el => getComputedStyle(el).userSelect), 'none', 'file paths stay selectable');
    await frame.locator(preview + ' .bc-min').click();
    await frame.locator(preview + ' .bc-close').click();

    // Sandboxed topic documents keep their own select-all; no handler injected.
    const bodyFrame = await (await frame.locator('iframe.doc-frame').first().elementHandle()).contentFrame();
    await bodyFrame.locator('.doc-body').click();
    await uncanceled(bodyFrame, '.doc-body');
    await key(process.platform === 'darwin' ? 'Meta' : 'Control');
    assert.equal((await selected(bodyFrame)).trim(), await bodyFrame.locator('.doc-body').innerText());
    assert.equal(await bodyFrame.locator('script').evaluateAll(els => els.some(el => el.textContent.includes('var dialogs ='))), false);

    // Transient fixtures cover terminal ownership, removed/hidden blocks,
    // native top-layer dialogs and contentless custom modals.
    await frame.evaluate(() => {
      const box = document.createElement('div'); box.id = 'select-all-fixtures';
      box.style.cssText = 'position:fixed;top:100px;left:300px;z-index:10000;background:white';
      box.innerHTML = '<div class="bcard" data-kind="terminal"><div class="bc-body"><div class="tm-host" tabindex="0">Terminal text</div></div></div>' +
        '<input aria-label="Rename fixture" value="Rename field text"><div class="memo-text">Memo body only</div>';
      document.body.appendChild(box);
    });
    await uncanceled(frame, '.tm-host');
    await uncanceled(frame, '#select-all-fixtures input');
    await clickSelect(frame, '.memo-text');
    await frame.locator('.memo-text').evaluate(el => el.hidden = true);
    await clear(frame);
    await tab.mouse.move(1278, 898);
    await key('Control');
    assert.equal(await selected(frame), '', 'hidden stale click is ignored');
    await frame.locator('.memo-text').evaluate(el => el.remove());
    await key('Control');
    assert.equal(await selected(frame), '', 'detached stale click is ignored');
    await frame.evaluate(() => {
      const dlg = document.createElement('dialog'); dlg.id = 'selection-dialog';
      dlg.innerHTML = '<header>Dialog chrome</header><div class="doc-body">Dialog content only</div><button>Close</button>';
      document.body.appendChild(dlg); dlg.showModal();
    });
    await key('Control');
    await assertScope(frame, '#selection-dialog .doc-body');
    await frame.locator('#selection-dialog button').click();
    await key('Meta');
    await assertScope(frame, '#selection-dialog .doc-body');
    await frame.locator('#selection-dialog').evaluate(el => el.remove());
    await frame.evaluate(() => {
      const modal = document.createElement('div'); modal.id = 'empty-modal';
      modal.setAttribute('role', 'dialog'); modal.setAttribute('aria-modal', 'true');
      modal.style.cssText = 'position:fixed;inset:100px;background:white;z-index:10001';
      modal.textContent = 'No readable pane'; document.body.appendChild(modal);
    });
    await key('Control');
    assert.equal(await selected(frame), '', 'contentless modal never selects the background');
    await frame.locator('#empty-modal').evaluate(el => el.remove());
    await frame.locator('#select-all-fixtures').evaluate(el => el.remove());
    assert.deepEqual(errors, []);
    console.log('scoped select-all: standalone body/log/policy/panel; dashboard bubble/composer/preview/shell; repeat and priority; dialogs; stale targets; terminal and sandbox exclusions: PASS');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
