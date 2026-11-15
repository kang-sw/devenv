// args: long.html short.html. Asserts the 1000px initial window and chunked loads.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  const b = await chromium.launch();
  try {
    const meas = (p) => p.evaluate(() => {
      const rows = [...document.querySelectorAll('.history-live > .timeline-row')];
      return { rows: rows.length, gaps: document.querySelectorAll('.history-live > .history-gap').length, h: rows.reduce((s, r) => s + r.getBoundingClientRect().height, 0) };
    });
    const long = await b.newPage({ viewport: { width: 1600, height: 900 } });
    await long.goto('file://' + process.argv[2]); await long.waitForTimeout(600);
    const m0 = await meas(long);
    assert(m0.h >= 1000 && m0.gaps === 1, 'initial window >= 1000px with one fold: ' + JSON.stringify(m0));
    await long.click('.history-gap [data-load="up"]'); await long.waitForTimeout(400);
    const m1 = await meas(long);
    assert(m1.h - m0.h >= 1000, 'one click adds >= 1000px: ' + JSON.stringify([m0, m1]));
    const short = await b.newPage({ viewport: { width: 1600, height: 900 } });
    await short.goto('file://' + process.argv[3]); await short.waitForTimeout(600);
    const s = await meas(short);
    assert(s.h < 1000 && s.gaps === 0, 'short history shows without a fold: ' + JSON.stringify(s));
    console.log(JSON.stringify({ initial: m0, afterClick: m1, short: s }));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
