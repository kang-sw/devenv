// Run by TestSelectionNoteBrowser against disposable, source-built sessions.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, updates] = process.argv.slice(2);

async function select(page, start, end = start) {
  await page.evaluate(([a, b]) => {
    const first = document.querySelector(a).firstChild, last = document.querySelector(b).firstChild;
    const r = document.createRange();
    r.setStart(first, 0); r.setEnd(last, last.length);
    const s = getSelection(); s.removeAllRanges(); s.addRange(r);
  }, [start, end]);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of [1440, 420]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US' });
      const tab = await context.newPage(), sent = [], errors = [];
      let page;
      const view = tab.frameLocator('iframe.sframe.on');
      async function activePage() {
        const el = tab.locator('iframe.sframe.on');
        await el.waitFor({ state: 'visible' });
        page = await (await el.elementHandle()).contentFrame();
        await page.locator('#ctext + .wsc-editor').waitFor({ state: 'visible' });
      }
      async function goSession(id) { await tab.goto(base + '/s/' + id); await activePage(); }
      tab.on('pageerror', e => errors.push(e.message));
      tab.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/message')) sent.push(r.postDataJSON()); });
      await fetch(updates + '?reset=1');
      await goSession('livesession');
      await page.locator('#quote-first').waitFor({ state: 'attached' });
      // Use the existing board tab control at narrow widths.
      if (!(await page.locator('#quote-first').isVisible())) await page.locator('[data-tab="board"]').first().click();
      const pop = view.locator('[data-wbmention-pop]'), input = view.getByRole('textbox', { name: 'Note about this selection (optional)' });
      const val = l => l.evaluate(e => ('value' in e) ? e.value : e.textContent);
      const bundles = view.locator('#ctext + .wsc-editor .wsc-mention');
      const attach = view.locator('[data-wbmention]'), chips = bundles;
      const bundleData = i => bundles.nth(i).evaluate(el => JSON.parse(el.getAttribute('data-wsc-payload')).selection);
      const composer = view.locator('#ctext + .wsc-editor'), backing = view.locator('#ctext');
      // A native pointer selection, followed by input focus that collapses it.
      await page.locator('#quote-first').click({ clickCount: 3 });
      await input.waitFor({ state: 'visible' });
      await input.fill('first note');
      await input.press('Tab');
      assert.equal(await attach.evaluate(e => e === document.activeElement), true, 'action is keyboard reachable');
      await attach.press('Shift+Tab');
      assert.equal(await input.evaluate(e => e === document.activeElement), true);
      const rect = await pop.boundingBox();
      assert(rect.x >= 0 && rect.x + rect.width <= width && rect.y >= 0 && rect.y + rect.height <= 900, 'popover fits viewport');
      // IME Enter/Escape, Safari's post-composition 229, and a pointer click
      // during composition must neither attach nor send. OS candidate UI is
      // outside this synthetic browser event contract.
      await input.dispatchEvent('compositionstart', { data: '가' });
      await input.dispatchEvent('keydown', { key: 'Enter', isComposing: true, keyCode: 229 });
      await input.dispatchEvent('keydown', { key: 'Escape', isComposing: true });
      await attach.click();
      assert.equal(await chips.count(), 0);
      assert.equal(await pop.isVisible(), true);
      await input.dispatchEvent('compositionend', { data: '가' });
      await attach.focus();
      await attach.dispatchEvent('keydown', { key: 'Enter', keyCode: 229 });
      assert.equal(await chips.count(), 0);
      await input.focus();
      await input.press('End'); await input.press('Enter');
      assert.equal(await chips.count(), 0, 'note Enter only edits');
      await input.fill('first note');
      await select(page, '#quote-next');
      await fetch(updates);
      await page.locator('#quote-updated').waitFor({ state: 'visible' });
      assert.equal(await val(input), 'first note', 'source update keeps note');
      await attach.click();
      assert.equal(await chips.count(), 1);
      assert.equal(await bundles.count(), 1);
      assert.match(await backing.inputValue(), /\[mention:/, 'selection has an inline anchor');
      assert.equal((await bundleData(0)).mentions[0].entry, 'e7', 'source update and changed selection never retarget captured quote');
      assert.match(JSON.stringify(await bundleData(0)), /First immutable quote[\s\S]*first note/);
      assert.equal(sent.length, 0, 'attachment is not send');
      // Reset the source, then capture two immutable entries in DOM order.
      await fetch(updates + '?reset=1');
      await page.locator('#quote-first').waitFor({ state: 'visible' });
      await select(page, '#quote-first', '#quote-next');
      await input.fill('one note for both'); await attach.click();
      assert.equal(await chips.count(), 2);
      assert.equal(await bundles.count(), 2, 'one push, one bundle, even across two entries');
      assert.equal((await bundleData(1)).mentions.length, 2);
      assert.match((await bundleData(1)).comment, /one note for both/);
      assert.match(await backing.inputValue(), /\[mention:/);
      // The production sandbox bridge dispatches a real selection report.
      // Its untrusted payload may not claim different topic/entry provenance.
      const frameEl = page.locator('iframe.doc-frame').first();
      if (!(await frameEl.count())) {
        await page.locator('section.topic[data-topic="Plan"] summary').first().click();
      }
      await page.locator('section.topic[data-topic="Plan"]').scrollIntoViewIfNeeded();
      await page.locator('iframe.doc-frame').first().waitFor();
      const frameHandle = await page.locator('iframe.doc-frame').first().elementHandle();
      await frameHandle.evaluate(f => { f.setAttribute('data-entry', 'e9'); f.setAttribute('data-entry-item', '4'); });
      const frame = await frameHandle.contentFrame();
      await frame.locator('.doc-body').waitFor();
      await frame.evaluate(() => {
        const r = document.createRange(); r.selectNodeContents(document.querySelector('.doc-body'));
        const s = getSelection(); s.removeAllRanges(); s.addRange(r);
      });
      await input.fill('sandbox note');
      await frame.evaluate(() => parent.postMessage({wbDoc:'sel', docKey:'wrong', topic:'forged', entry:'e999', text:'forged selection'}, '*'));
      // Removing the source while the note is held must not discard it.
      await frameHandle.evaluate(f => f.remove());
      await tab.setViewportSize({ width: width - 10, height: 900 });
      await attach.click();
      assert.equal(await chips.count(), 3);
      assert.equal(await bundles.count(), 3);
      assert.match(JSON.stringify(await bundleData(2)), /Sandbox selection text[\s\S]*sandbox note/);
      assert.match(await backing.inputValue(), /\[mention:/);
      let text = '';
      // Keyboard selection and the legacy empty-note attachment.
      await page.locator('#quote-next').scrollIntoViewIfNeeded();
      // Establish a native document selection before extending it with keys;
      // programmatic collapse on a non-editable paragraph is not a caret.
      await page.locator('#quote-next').click({ clickCount: 2, position: { x: 10, y: 10 } });
      for (let i = 0; i < ' immutable quote'.length; i++) await tab.keyboard.press('Shift+ArrowRight');
      await input.waitFor({ state: 'visible' });
      assert.equal(await val(input), '');
      await attach.click();
      assert.equal(await chips.count(), 4);
      assert.equal(await bundles.count(), 4);
      assert.match(await backing.inputValue(), /\[mention:/, 'empty note still adds an inline source');
      // Repeated action cannot duplicate an attachment.
      await attach.dispatchEvent('click');
      assert.equal(await chips.count(), 4);
      // Session navigation saves only this session's draft, then restores all
      // contexts as editable text and all source locators independently.
      await goSession('boardsession');
      assert.equal(await page.locator('#ctext').inputValue(), '');
      assert.equal(await page.locator('#ctext + .wsc-editor .wsc-mention').count(), 0);
      await goSession('livesession');
      assert.match(await backing.inputValue(), /\[mention:/);
      assert.equal(await chips.count(), 4);
      assert.equal(await bundles.count(), 4);
      text = 'typed beside the groups';
      await composer.press('End'); await composer.type(text);
      await tab.reload(); await activePage();
      assert((await backing.inputValue()).includes(text), 'ordinary edits survive draft restore');
      assert.equal(await chips.count(), 4);
      assert.equal(sent.length, 0);
      await page.locator('#csend').click();
      await page.waitForFunction(() => document.querySelector('#ctext').value === '' && !document.querySelector('#ctext + .wsc-editor .wsc-mention'));
      assert.equal(sent.length, 1);
      assert.deepEqual(sent[0].mentions || [], [], 'sources travel inside their selection groups');
      const groups = [...sent[0].text.matchAll(/<wb:selection>\n([\s\S]*?)<\/wb:selection>/g)].map(m => m[1]);
      assert.equal(groups.length, 4, 'one selection element per push');
      assert.match(groups[0], /<wb:mention topic="review" id="3" entry="e7">First immutable quote<\/wb:mention>\n<wb:comment>first note<\/wb:comment>/);
      assert.match(groups[1], /entry="e7">First immutable quote<\/wb:mention>\n<wb:mention topic="review" entry="e8">Second immutable quote<\/wb:mention>\n<wb:comment>one note for both<\/wb:comment>/);
      assert.match(groups[2], /<wb:mention topic="Plan" id="4" entry="e9">Sandbox selection text<\/wb:mention>\n<wb:comment>sandbox note<\/wb:comment>/);
      assert.match(groups[3], /entry="e8">[^<]*<\/wb:mention>\n<wb:comment><\/wb:comment>/);
      assert.equal(sent[0].text.replace(/<wb:selection>[\s\S]*?<\/wb:selection>\s*/g, '').trim(), text, 'typed text stays outside the groups');
      const cancel = page.locator('#r-convlive [data-cancel]').last();
      await cancel.waitFor({ state: 'attached' });
      await cancel.evaluate(el => el.click());
      await page.waitForFunction(() => document.querySelectorAll('#ctext + .wsc-editor .wsc-mention').length === 4);
      assert((await backing.inputValue()).includes(text), 'cancel restores prose beside inline selection capsules');
      assert.equal((await bundleData(0)).mentions[0].entry, 'e7', 'cancel restores selection metadata');
      // Plain selection dismissal still follows selection collapse/Escape.
      await select(page, '#quote-next'); await input.waitFor({ state: 'visible' });
      await input.focus(); await input.press('Escape'); assert.equal(await pop.isVisible(), false);
      await select(page, '#quote-first'); await input.waitFor({ state: 'visible' });
      await page.evaluate(() => getSelection().removeAllRanges());
      await pop.waitFor({ state: 'hidden' });
      assert.deepEqual(errors, []);
      console.log(`PASS ${width}px: pointer/keyboard, focus, IME guards, ordered entries, source update/removal, sandbox bridge, editable draft/session isolation, separate send, bounds and dismissal`);
      await context.close();
    }
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
