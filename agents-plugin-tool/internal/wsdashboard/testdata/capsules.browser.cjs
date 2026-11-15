// Run by TestInlineCapsulesBrowserWrapping; no network or dashboard runtime.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const assets = path.join(__dirname, '../assets');

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    await page.route('**/*', route => route.abort());
    // Equal font metrics make fragment-top equality a line-placement assertion,
    // rather than a comparison of the production text and badge font sizes.
    await page.setContent('<style>' + fs.readFileSync(path.join(assets, 'dashboard-conversation.css'), 'utf8') + '.composer textarea + .wsc-editor{font-size:12px;line-height:1.5}</style><div class="composer" style="width:150px"><textarea id="text"></textarea></div>');
    await page.evaluate(() => { window.wsT = key => key; });
    await page.addScriptTag({ content: fs.readFileSync(path.join(assets, 'mention.js'), 'utf8') });
    await page.addScriptTag({ content: fs.readFileSync(path.join(assets, 'capsules.js'), 'utf8') });
    await page.evaluate(() => {
      const field = document.getElementById('text');
      WsCapsules.topicPicker(field, done => done([{ id: 't1', label: 'A long topic label that wraps across several lines' }]));
      field.focus();
    });
    await page.keyboard.type('abc #long');
    await page.locator('.wsm-pop:not([hidden]) .wsm-opt').waitFor();
    await page.keyboard.press('Tab');
    await page.keyboard.type('after');
    const state = await page.evaluate(() => {
      const field = document.getElementById('text'), editor = field.wsc.editor;
      const capsule = editor.querySelector('.wsc-capsule');
      const before = document.createRange();
      before.setStart(editor.firstChild, 0); before.setEnd(editor.firstChild, 3);
      return {
        text: field.value, caret: field.selectionStart,
        prefixTop: before.getBoundingClientRect().top,
        capsuleTop: capsule.getClientRects()[0].top,
        fragments: capsule.getClientRects().length,
        blocks: editor.querySelectorAll('br, div, p').length,
        ranges: field.wsc.snapshot()
      };
    });
    assert.equal(state.capsuleTop, state.prefixTop, 'Tab inserts the first badge fragment on the preceding text line');
    assert(state.fragments > 1, 'a long badge wraps naturally within the narrow editor');
    assert.equal(state.text, 'abc [t1] after', 'visual wrapping never changes canonical serialization');
    assert.equal(state.caret, state.text.length, 'typing continues after the badge');
    assert.equal(state.blocks, 0, 'insertion creates no forced line break or block wrapper');
    assert.deepEqual(state.ranges, [{ start: 4, end: 8, kind: 'topic', id: 't1', label: 'A long topic label that wraps across several lines' }]);
    console.log('capsules: native Tab insertion wraps inline and preserves text, identity and caret');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
