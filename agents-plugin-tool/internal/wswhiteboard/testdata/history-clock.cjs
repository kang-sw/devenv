// args: page.html [screenshot.png]. Every history clock (lead, owner, check ask)
// shows relative time, not the server HH:MM.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  const b = await chromium.launch();
  try {
    const p = await b.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' });
    await p.goto('file://' + process.argv[2]); await p.waitForTimeout(1500);
    if (process.argv[3]) await p.screenshot({ path: process.argv[3] });
    const clocks = await p.evaluate(() => [...document.querySelectorAll('.history-live time.entry-at')].map(t => ({
      text: t.textContent.trim(), at: t.getAttribute('data-at') || '', ask: t.classList.contains('wb-htime'),
      owner: !!t.closest('.owner-input'),
    })).filter(c => c.text));
    console.log(JSON.stringify(clocks));
    assert(clocks.some(c => c.ask), 'a check-ask clock is rendered');
    assert(clocks.some(c => c.owner), 'an owner-input clock is rendered');
    for (const c of clocks) {
      assert(!/^\d\d:\d\d$/.test(c.text), 'clock shows relative time, not HH:MM: ' + JSON.stringify(c));
    }
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
