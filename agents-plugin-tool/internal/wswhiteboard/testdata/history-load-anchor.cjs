// args: top.html inner.html shotdir. The row under a clicked fold keeps its
// viewport position while the loaded rows appear above it, also while late
// images in those rows arrive (100/300/600 ms after the insert). Owner input
// ends that hold, and the hold ends by itself, leaving no observer behind.
// WS_ANCHOR_REPORT=1 prints the deltas without failing (measurement tables).
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const report = !!process.env.WS_ANCHOR_REPORT;
const LATE = [[100, 240], [300, 360], [600, 300]]; // [delay ms, image height px]
const LABELS = ['0', '100', '300', '600', '1000', '1700'];
const svg = (h) => `<svg xmlns="http://www.w3.org/2000/svg" width="560" height="${h}"><rect x="1" y="1" width="558" height="${h - 2}" rx="8" fill="#e8eef7" stroke="#7a8aa6"/><text x="20" y="34" font-family="sans-serif" font-size="18" fill="#34445f">late mockup image, ${h}px</text></svg>`;

async function open(b, name, file, nested) {
  const p = await b.newPage({ viewport: { width: 1440, height: 800 } });
  // Track ResizeObservers that are still observing, by creation time.
  await p.addInitScript(() => {
    const RO = window.ResizeObserver; window.__ro = [];
    window.ResizeObserver = class extends RO {
      constructor(fn) { super(fn); this.at = performance.now(); this.live = false; window.__ro.push(this); }
      observe(...a) { this.live = true; return super.observe(...a); }
      disconnect() { this.live = false; return super.disconnect(); }
    };
  });
  // A late image: the response arrives d ms after the row requests it.
  await p.route('http://late.test/**', async (route) => {
    const u = new URL(route.request().url());
    await new Promise(r => setTimeout(r, +u.searchParams.get('d')));
    await route.fulfill({ contentType: 'image/svg+xml', body: svg(+u.searchParams.get('h')) });
  });
  await p.goto('file://' + file); await p.waitForTimeout(600);
  if (nested) await p.evaluate(() => {
    const box = document.createElement('div');
    box.id = 'nest'; box.style.cssText = 'height:700px;overflow:auto;border:1px solid red';
    while (document.body.firstChild) box.appendChild(document.body.firstChild);
    document.body.appendChild(box);
  });
  // The fold to click: the last gap with a load-above button (inner gap for
  // "inner", the top one for "top").
  const sel = await p.evaluate((name) => {
    const gaps = [...document.querySelectorAll('.history-live > .history-gap')].filter(g => g.querySelector('[data-load="up"]') && g.nextElementSibling && g.nextElementSibling.matches('.timeline-row'));
    const g = name === 'inner' ? gaps[gaps.length - 1] : gaps[0];
    if (!g) return null;
    g.setAttribute('data-test-gap', '1'); return true;
  }, name);
  assert(sel, name + ': a fold exists');
  return p;
}

// Seeds late images into the rows the click loads first, puts the fold a
// third down the viewport, and arms page-side sampling on the next click.
// Samples are taken after the frame that follows each image's arrival.
function arm(p, n) {
  return p.evaluate(({ LATE, n }) => {
    const g = document.querySelector('[data-test-gap]');
    const rows = g.querySelector('template[data-history-gap]').content.children;
    LATE.forEach(([d, h], i) => {
      const row = rows[rows.length - 1 - i];
      if (!row) return;
      const img = document.createElement('img');
      img.className = 'late-test'; img.style.cssText = 'display:block;max-width:100%;margin:6px 0';
      img.src = `http://late.test/m.svg?d=${d}&h=${h}&n=${n}`;
      row.appendChild(img);
    });
    let s = g.parentElement; while (s && !(s.scrollHeight > s.clientHeight && /auto|scroll/.test(getComputedStyle(s).overflowY))) s = s.parentElement;
    s = s || document.scrollingElement; window.__s = s;
    s.scrollTop += g.getBoundingClientRect().top - innerHeight / 3;
    const a = window.__a = g.nextElementSibling;
    const rec = window.__rec = { y0: null, samples: [], rows0: document.querySelectorAll('.history-live > .timeline-row').length };
    const pins = () => window.wbHistory.pinning ? window.wbHistory.pinning() : -1;
    const sample = (label) => rec.samples.push({ label, d: a.getBoundingClientRect().top - rec.y0, st: s.scrollTop, pins: pins() });
    const painted = (fn) => requestAnimationFrame(() => requestAnimationFrame(fn));
    document.addEventListener('click', () => {
      rec.t0 = performance.now();
      setTimeout(() => { // after the history's click handler loaded the rows
        rec.rows1 = document.querySelectorAll('.history-live > .timeline-row').length;
        painted(() => sample('0'));
        document.querySelectorAll('img.late-test').forEach(img => {
          if (img.complete) return;
          const d = new URL(img.src).searchParams.get('d');
          img.addEventListener('load', () => painted(() => sample(d)), { once: true });
        });
      }, 0);
      setTimeout(() => sample('1000'), 1000);
      setTimeout(() => {
        sample('1700');
        rec.ro = window.__ro.filter(o => o.at >= rec.t0 && o.live).length;
        rec.done = true;
      }, 1700);
    }, { capture: true, once: true });
  }, { LATE, n });
}

async function click(p) {
  const gap = p.locator('[data-test-gap] [data-load="up"]');
  await gap.scrollIntoViewIfNeeded();
  await arm(p, Math.random());
  await p.waitForTimeout(350);
  await p.evaluate(() => { window.__rec.y0 = window.__a.getBoundingClientRect().top; });
  await gap.click();
}

(async () => {
  const b = await chromium.launch();
  const out = {}, fail = [];
  const check = (ok, msg) => { if (!ok) fail.push(msg); };
  try {
    for (const [name, file] of [['top', process.argv[2]], ['inner', process.argv[3]]]) {
      for (const nested of [false, true]) {
        const key = name + (nested ? '-nested' : '');
        let p = await open(b, name, file, nested);
        const deltas = [];
        for (let i = 0; i < 3; i++) {
          await click(p);
          await p.waitForFunction(() => window.__rec.done, null, { timeout: 5000 });
          const rec = await p.evaluate(() => window.__rec);
          if (i === 0) await p.screenshot({ path: `${process.argv[4]}/after-${key}.png` });
          const by = Object.fromEntries(rec.samples.map(s => [s.label, s]));
          deltas.push(LABELS.map(l => by[l] ? Math.round(by[l].d * 10) / 10 : null));
          check(rec.rows1 > rec.rows0, `${key} click ${i}: loaded rows`);
          LABELS.forEach(l => check(by[l] && Math.abs(by[l].d) <= 1, `${key} click ${i}: anchor moved ${by[l] && by[l].d}px at ${l}ms`));
          // The hold ends by itself: no pin and no observer left from the click.
          check(by['1700'] && by['1700'].pins === 0, `${key} click ${i}: ${by['1700'] && by['1700'].pins} pin(s) active after settle`);
          check(rec.ro === 0, `${key} click ${i}: ${rec.ro} ResizeObserver(s) from the click still observing`);
          if (!(await p.locator('[data-test-gap]').count())) break;
        }
        out[key] = deltas;
        await p.close();
        // Owner wheel during the settle window ends the hold: the anchor moves
        // with the wheel and nothing writes the scroller afterwards.
        p = await open(b, name, file, nested);
        await click(p);
        await p.waitForTimeout(150);
        await p.mouse.wheel(0, -300);
        await p.waitForTimeout(250);
        const mid = await p.evaluate(() => ({ d: window.__a.getBoundingClientRect().top - window.__rec.y0, st: window.__s.scrollTop, pins: window.wbHistory.pinning ? window.wbHistory.pinning() : -1 }));
        await p.waitForFunction(() => window.__rec.done, null, { timeout: 5000 });
        const end = (await p.evaluate(() => window.__rec)).samples.find(s => s.label === '1700');
        out[key + '-wheel'] = { afterWheel: Math.round(mid.d), end: Math.round(end.d), scrollAfterWheel: Math.round(mid.st), scrollEnd: Math.round(end.st), pins: mid.pins };
        check(mid.pins === 0, `${key} wheel: ${mid.pins} pin(s) active after the wheel`);
        check(mid.d >= 250, `${key} wheel: anchor did not follow the wheel (${mid.d}px)`);
        check(Math.abs(end.st - mid.st) <= 1, `${key} wheel: scroller moved from ${mid.st} to ${end.st} after the wheel`);
        await p.close();
      }
    }
    console.log('samples (ms): ' + LABELS.join(','));
    Object.entries(out).forEach(([k, v]) => console.log(k + ' ' + JSON.stringify(v)));
    if (!report && fail.length) { console.error(fail.join('\n')); process.exit(1); }
  } finally { await b.close(); }
})().catch(e => { console.error(e); process.exit(1); });
