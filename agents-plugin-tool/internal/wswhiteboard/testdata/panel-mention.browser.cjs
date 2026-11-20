const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const dash = path.resolve(__dirname, '../../wsdashboard');
const composer = fs.readFileSync(path.join(dash, 'assets/dashboard-composer.js'), 'utf8');
function fn(name) {
  const m = composer.match(new RegExp('\\n  function ' + name + '\\([^]*?\\n  \\}\\n'));
  assert(m, name); return m[0];
}
const shots = process.argv[3];
(async () => {
  const browser = await chromium.launch();
  try {
    for (const theme of ['dark', 'light']) {
      const page = await browser.newPage({ viewport: { width: 1180, height: 850 }, colorScheme: theme });
      await page.goto(pathToFileURL(process.argv[2]).href);
      await page.addStyleTag({ content: fs.readFileSync(path.join(dash, 'assets/dashboard-board.css'), 'utf8') });
      await page.addStyleTag({ content: fs.readFileSync(path.join(dash, 'assets/dashboard-conversation.css'), 'utf8') });
      await page.addStyleTag({ content: '.composer { width: min(860px, 90vw); margin: 24px auto; padding: 18px; border: 1px solid var(--wb-line); border-radius: 12px; background: var(--wb-card); } .composer .wsc-editor { min-height: 70px; }' });
      const button = page.locator('.topic[data-topic="auth-profile"] .wb-entry.panel:not(.historical) .pfmention').first();
      await button.scrollIntoViewIfNeeded();
      const details = button.locator('xpath=ancestor::details[1]');
      const standaloneOpen = await details.evaluate(el => el.open);
      await button.click();
      assert.equal(await details.evaluate(el => el.open), standaloneOpen, 'standalone mention click is inert and does not toggle panel');
      await page.addScriptTag({ content: fs.readFileSync(path.join(dash, 'assets/capsules.js'), 'utf8') });
      await page.addScriptTag({ content: `
        var ctext = document.getElementById('ctext');
        var bmentions = [], queueDraft = () => {}, renderChips = () => {};
        var inlineEditor = () => ctext.wsc;
        WsCapsules.attach(ctext);
        ${fn('addPanelMention')}
        ${fn('addTicketMention')}
        window.WsComposer = { addPanelMention, addTicketMention };
      ` });
      await page.evaluate(() => { ctext.value = 'Before after'; ctext.focus(); ctext.setSelectionRange(7, 7); });
      const open = await details.evaluate(el => el.open);
      await button.click();
      assert.equal(await details.evaluate(el => el.open), open, 'mention click must not toggle panel');
      let state = await page.evaluate(() => ({ text: ctext.value, rows: ctext.wsc.ordered() }));
      assert(state.text.startsWith('Before {{p1}} after'), state.text);
      assert.equal(state.rows[0].kind, 'mention');
      assert.deepEqual({ topic: state.rows[0].payload.mention.topic, panel: state.rows[0].payload.panel, entry: state.rows[0].payload.entry }, { topic: 'auth-profile', panel: 'p1', entry: state.rows[0].payload.entry });
      assert.match(state.rows[0].payload.entry, /^e[1-9]/);
      if (shots) {
        await button.hover(); await page.waitForTimeout(300);
        await button.locator('xpath=ancestor::summary[1]').screenshot({ path: path.join(shots, `panel-mention-hover-${theme}.png`) });
      }
      const copied = await page.evaluate(() => {
        const m = ctext.wsc.ordered()[0], data = new DataTransfer();
        ctext.focus(); ctext.setSelectionRange(m.start, m.end);
        ctext.wsc.editor.dispatchEvent(new ClipboardEvent('copy', { clipboardData: data, bubbles: true, cancelable: true }));
        ctext.setSelectionRange(m.end, m.end);
        ctext.wsc.editor.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
        return { wire: ctext.value, rich: data.getData('application/x-wsc-capsules+json'), count: ctext.wsc.ordered().length };
      });
      assert(copied.rich.includes('"panel":"p1"'));
      assert.equal(copied.count, 2, 'copy/paste preserves mention metadata');
      assert(copied.wire.includes('{{p1}}{{p1}}'), 'wire keeps legacy panel anchor');
      await page.evaluate(() => { const m = ctext.wsc.ordered()[0]; ctext.focus(); ctext.setSelectionRange(m.end, m.end); });
      await page.keyboard.press('Backspace');
      assert.equal(await page.evaluate(() => ctext.wsc.ordered().length), 1, 'Backspace deletes one whole panel capsule');
      await page.evaluate(() => {
        ctext.value = 'Discuss '; ctext.focus(); ctext.setSelectionRange(ctext.value.length, ctext.value.length);
        ctext.wsc.insertCapsule('answer', { label: '(51)=b', anchor: '(51)=b', plain: '(51)=b', answer: { id: '51', answer: 'b' } });
        addPanelMention({ topic: 'auth-profile', panel: 'p1', entry: 'e2', recap: 'Panel recap' });
        addTicketMention({ stem: '261120-feat-example', title: 'Example ticket', status: 'ready', path: 'ready/261120-feat-example.md' });
      });
      if (shots) { await page.locator('.composer').scrollIntoViewIfNeeded(); await page.waitForTimeout(300); await page.locator('.composer').screenshot({ path: path.join(shots, `composer-mixed-${theme}.png`) }); }
      await page.close();
    }
    console.log('panel mention: click, saved caret, no toggle, copy/paste, atomic Backspace, screenshots passed');
  } finally { await browser.close(); }
})().catch(err => { console.error(err); process.exitCode = 1; });
