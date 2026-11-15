// Run by TestConversationRowGrammarBrowser against a static render of every row kind.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, shots] = process.argv.slice(2);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) {
      const page = await browser.newPage({ viewport: { width: 700, height: 900 } });
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.setContent(fs.readFileSync(file, 'utf8').replace('THEME', theme));
      const m = await page.evaluate(() => {
        const box = (el) => { const r = el.getBoundingClientRect(); return { l: r.left, r: r.right, t: r.top, b: r.bottom }; };
        const frame = (row) => ({
          time: row.querySelector('.row-time') && box(row.querySelector('.row-time')),
          chev: row.querySelector('.row-chevron .i') ? box(row.querySelector('.row-chevron')) : null,
          name: row.querySelector('.mpeer') && getComputedStyle(row.querySelector('.mpeer')).fontSize + '/' + getComputedStyle(row.querySelector('.mpeer')).fontWeight,
          preview: row.querySelector('.mfirst') && getComputedStyle(row.querySelector('.mfirst')).textOverflow,
          icon: !!row.querySelector('.row-icon .i'),
          h: row.getBoundingClientRect().height,
        });
        const mail = frame(document.querySelector('details.mailline.recv:not(.child-report):not(.agent-card) > summary'));
        const report = frame(document.querySelector('.child-report > summary'));
        const note = frame(document.querySelector('.agent-note > .conv-row'));
        const tools = document.querySelector('.toolsline > summary');
        return {
          mail, report, note, toolsH: tools.getBoundingClientRect().height,
          toolIcon: tools.querySelector('.row-icon use').getAttribute('href'),
          wbDetails: document.querySelectorAll('.wbeffects details, details.wbeffects').length,
          wbArticles: document.querySelectorAll('article.wbeffects.wbupdate').length,
          wbBody: document.querySelector('article.wbupdate .wbupdate-text')?.getBoundingClientRect().height,
        };
      });
      for (const row of [m.report, m.note]) {
        assert.equal(Math.round(row.time.r), Math.round(m.mail.time.r), theme + ': agent time column is the mail time column');
        assert.equal(Math.round(row.time.l), Math.round(m.mail.time.l));
        assert.equal(row.name, m.mail.name, theme + ': agent name font matches mail name');
        assert.equal(row.preview, 'ellipsis');
        assert(row.icon, 'agent row has an icon');
      }
      assert(m.report.chev, 'child report keeps the chevron');
      assert.equal(m.toolIcon, '#i-hammer');
      assert(m.toolsH <= 44, 'tool summary is compact: ' + m.toolsH);
      assert.equal(m.wbDetails, 0, 'whiteboard updates are never folded');
      assert(m.wbArticles >= 1 && m.wbBody > 0, 'whiteboard update card is expanded');
      assert.deepEqual(errors, []);
      if (shots) await page.screenshot({ path: shots + '/after-' + theme + '.png', fullPage: true });
      await page.close();
    }
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
