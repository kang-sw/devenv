// args: top.html inner.html shotdir. The row under a clicked fold keeps its
// viewport position while the loaded rows appear above it.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  const b = await chromium.launch();
  const out = {};
  try {
    for (const [name, file] of [['top', process.argv[2]], ['inner', process.argv[3]]]) {
      for (const nested of [false, true]) {
        const p = await b.newPage({ viewport: { width: 1200, height: 800 } });
        await p.goto('file://' + file); await p.waitForTimeout(600);
        if (nested) await p.evaluate(() => {
          const box = document.createElement('div');
          box.id = 'nest'; box.style.cssText = 'height:700px;overflow:auto;border:1px solid red';
          while (document.body.firstChild) box.appendChild(document.body.firstChild);
          document.body.appendChild(box);
        });
        // The fold to click: the last gap with a load-above button (inner gap
        // for "inner", the top one for "top").
        const sel = await p.evaluate((name) => {
          const gaps = [...document.querySelectorAll('.history-live > .history-gap')].filter(g => g.querySelector('[data-load="up"]') && g.nextElementSibling && g.nextElementSibling.matches('.timeline-row'));
          const g = name === 'inner' ? gaps[gaps.length - 1] : gaps[0];
          if (!g) return null;
          g.setAttribute('data-test-gap', '1'); return true;
        }, name);
        assert(sel, name + ': a fold exists');
        const deltas = [];
        for (let i = 0; i < 3; i++) {
          const gap = p.locator('[data-test-gap] [data-load="up"]');
          await gap.scrollIntoViewIfNeeded();
          // Reader scrolled so the fold sits mid-viewport.
          await p.evaluate(() => {
            const g = document.querySelector('[data-test-gap]'); const r = g.getBoundingClientRect();
            let s = g.parentElement; while (s && !(s.scrollHeight > s.clientHeight && /auto|scroll/.test(getComputedStyle(s).overflowY))) s = s.parentElement;
            (s || document.scrollingElement).scrollTop += r.top - innerHeight / 3;
          });
          await p.waitForTimeout(350);
          const before = await p.evaluate(() => { const g = document.querySelector('[data-test-gap]'); const a = g.nextElementSibling; window.__a = a; return { top: a.getBoundingClientRect().top, rows: document.querySelectorAll('.history-live > .timeline-row').length }; });
          if (i === 0) await p.screenshot({ path: `${process.argv[4]}/${name}${nested ? '-nested' : ''}-before.png` });
          await gap.click(); await p.waitForTimeout(500);
          const after = await p.evaluate(() => ({ top: window.__a.getBoundingClientRect().top, rows: document.querySelectorAll('.history-live > .timeline-row').length }));
          if (i === 0) await p.screenshot({ path: `${process.argv[4]}/${name}${nested ? '-nested' : ''}-after.png` });
          assert(after.rows > before.rows, 'click loaded rows');
          deltas.push(after.top - before.top);
          if (!(await p.locator('[data-test-gap]').count())) break;
        }
        out[name + (nested ? '-nested' : '')] = deltas;
        deltas.forEach(d => assert(Math.abs(d) <= 1, `${name} nested=${nested} anchor moved ${d}px: ${deltas}`));
        await p.close();
      }
    }
    console.log(JSON.stringify(out));
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
