// args: board.html shotdir. Topic "a" loads history above its fold, the owner
// wheels until the whole card is above the viewport, and the loaded rows fold
// back into the gap while a visible line in topic "b" stays put (< 2 px).
// Runs in the page scroller and in a nested one, then checks the load
// control's capsule styling in both themes.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, shots] = process.argv.slice(2);

const state = (p) => p.evaluate(() => {
  const a = document.querySelector('.topic[data-topic="a"]');
  const gap = a.querySelector('.history-live > .history-gap');
  return { rows: a.querySelectorAll('.history-live > .timeline-row').length, gaps: a.querySelectorAll('.history-live > .history-gap').length, count: gap && +gap.getAttribute('data-count') };
});

async function open(b, nested, scheme, width) {
  const p = await b.newPage({ viewport: { width: width || 1280, height: 800 }, colorScheme: scheme || 'dark' });
  await p.goto('file://' + file); await p.waitForTimeout(600);
  if (nested) await p.evaluate(() => {
    const box = document.createElement('div');
    box.id = 'nest'; box.style.cssText = 'height:760px;overflow:auto';
    while (document.body.firstChild) box.appendChild(document.body.firstChild);
    document.body.appendChild(box);
  });
  await p.waitForTimeout(200);
  return p;
}

async function run(b, nested) {
  const key = nested ? 'nested' : 'page', fail = [];
  const p = await open(b, nested);
  const s0 = await state(p);
  assert(s0.gaps >= 1, key + ': topic a has a fold');
  const up = p.locator('.topic[data-topic="a"] .history-gap [data-load="up"]').first();
  await up.scrollIntoViewIfNeeded();
  await up.click();
  await p.waitForTimeout(1800); // the load-above pin settles
  const s1 = await state(p);
  if (!(s1.rows > s0.rows)) fail.push(`${key}: load above admitted no rows ${JSON.stringify([s0, s1])}`);
  if (!nested) {
    await up.scrollIntoViewIfNeeded(); await p.waitForTimeout(300);
    await p.screenshot({ path: `${shots}/before-expanded.png` });
  }
  // Pick the topic-b entry that lands mid-viewport once topic a's bottom is
  // 200 px above the view, and sample its y every frame.
  const plan = await p.evaluate(() => {
    let s = document.getElementById('nest') || document.scrollingElement;
    const a = document.querySelector('.topic[data-topic="a"]').getBoundingClientRect();
    const top = s === document.scrollingElement ? 0 : s.getBoundingClientRect().top;
    const by = Math.round(a.bottom - top + 200);
    const mid = innerHeight / 2 + by;
    const entries = [...document.querySelectorAll('.topic[data-topic="b"] .history-live .wb-entry')];
    const ref = entries.find(e => e.getBoundingClientRect().top >= mid) || entries[entries.length - 1];
    ref.setAttribute('data-test-ref', '1');
    const live = document.querySelector('.topic[data-topic="a"] .history-live');
    const rec = window.__rec = { samples: [], folded: null };
    const y = () => ref.getBoundingClientRect().top;
    (function tick() { rec.samples.push(y()); if (rec.samples.length > 3) rec.samples.shift(); requestAnimationFrame(tick); })();
    new MutationObserver(() => {
      if (rec.folded) return;
      // Callback runs after the fold and its compensation; the last frame
      // sample is the pre-fold position.
      rec.folded = { before: rec.samples[rec.samples.length - 1], sync: y(), at: performance.now() };
      requestAnimationFrame(() => requestAnimationFrame(() => { rec.folded.painted = y(); }));
      setTimeout(() => { rec.folded.later = y(); }, 500);
    }).observe(live, { childList: true });
    return { by };
  });
  const box = await p.evaluate(() => { const s = document.getElementById('nest'); const r = s ? s.getBoundingClientRect() : { left: 0, top: 0, width: innerWidth, height: innerHeight }; return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
  await p.mouse.move(box.x, box.y);
  // Owner wheel in steps, like a real scroll; the fold waits for it to end.
  for (let left = plan.by; left > 0; left -= 200) { await p.mouse.wheel(0, Math.min(200, left)); await p.waitForTimeout(16); }
  await p.waitForFunction(() => window.__rec.folded && window.__rec.folded.later !== undefined, null, { timeout: 5000 }).catch(() => {});
  const rec = await p.evaluate(() => window.__rec.folded);
  const out = await p.evaluate(() => document.querySelector('.topic[data-topic="a"]').getBoundingClientRect().bottom);
  const s2 = await state(p);
  if (!rec) fail.push(`${key}: topic a did not fold after leaving the view (bottom ${out}) ${JSON.stringify(s2)}`);
  else {
    const d = { sync: rec.sync - rec.before, painted: rec.painted - rec.before, later: rec.later - rec.before };
    console.log(`${key} fold anchor delta px ${JSON.stringify(d)}`);
    Object.entries(d).forEach(([k, v]) => { if (!(Math.abs(v) < 2)) fail.push(`${key}: visible line moved ${v}px (${k})`); });
    if (!(s2.rows === s0.rows && s2.count === s0.count && s2.gaps === s0.gaps)) fail.push(`${key}: fold did not restore the initial window ${JSON.stringify([s0, s1, s2])}`);
  }
  // Back in view: the fold row is the initial one again and loads again.
  await up.scrollIntoViewIfNeeded();
  await p.waitForTimeout(300);
  if (!nested) await p.screenshot({ path: `${shots}/after-folded.png` });
  await up.click(); await p.waitForTimeout(600);
  const s3 = await state(p);
  if (!(s3.rows === s1.rows)) fail.push(`${key}: reload after fold admitted ${s3.rows} rows, first load ${s1.rows}`);
  console.log(`${key} states ${JSON.stringify({ initial: s0, loaded: s1, folded: s2, reloaded: s3 })}`);
  await p.close();
  return fail;
}

// The load pill sits right after the gap label/time, not at the far right
// edge, and never overflows the gap row when it wraps at narrow widths.
async function inline(b, scheme, width) {
  const p = await open(b, false, scheme, width);
  const gap = p.locator('.topic[data-topic="a"] .history-gap').first();
  await gap.scrollIntoViewIfNeeded();
  const m = await gap.evaluate((g) => {
    const range = g.querySelector('.gap-range').getBoundingClientRect(), pill = g.querySelector('[data-load="up"]').getBoundingClientRect(), row = g.getBoundingClientRect();
    const em = parseFloat(getComputedStyle(g).fontSize);
    return { em, rangeRight: range.right, pillLeft: pill.left, pillRight: pill.right, rowRight: row.right, sameLine: Math.abs(pill.top - range.top) < 12 };
  });
  const r = await gap.boundingBox();
  await p.screenshot({ path: `${shots}/loadmore-inline-${scheme}-${width}.png`, clip: { x: Math.max(0, r.x - 40), y: Math.max(0, r.y - 60), width: Math.min(width, r.width + 80), height: r.height + 120 } });
  await p.close();
  const fail = [];
  if (m.pillRight > m.rowRight + 0.5) fail.push(`${scheme} ${width}: pill overflows row ${JSON.stringify(m)}`);
  if (width >= 1000 && !(m.sameLine && m.pillLeft - m.rangeRight >= 0 && m.pillLeft - m.rangeRight <= 1.5 * m.em)) fail.push(`${scheme} ${width}: pill not inline after label ${JSON.stringify(m)}`);
  console.log(`inline ${scheme} ${width} ${JSON.stringify(m)}`);
  return fail;
}

async function capsule(b, scheme) {
  const p = await open(b, false, scheme);
  const up = p.locator('.topic[data-topic="a"] .history-gap [data-load="up"]').first();
  await up.scrollIntoViewIfNeeded();
  const st = await up.evaluate((n) => {
    const c = getComputedStyle(n), r = n.getBoundingClientRect();
    return { bg: c.backgroundColor, bw: c.borderTopWidth, bs: c.borderTopStyle, radius: parseFloat(c.borderTopLeftRadius), h: r.height, color: c.color };
  });
  const gap = p.locator('.topic[data-topic="a"] .history-gap').first();
  const r = await gap.boundingBox();
  await p.screenshot({ path: `${shots}/loadmore-capsule-${scheme}.png`, clip: { x: Math.max(0, r.x - 40), y: Math.max(0, r.y - 60), width: Math.min(1280, r.width + 80), height: r.height + 120 } });
  await up.hover(); await p.waitForTimeout(100);
  const hover = await up.evaluate((n) => getComputedStyle(n).backgroundColor);
  await p.close();
  const fail = [];
  const alpha = (c) => { const m = /rgba?\(([^)]+)\)/.exec(c); if (!m) return 1; const v = m[1].split(/[ ,/]+/).filter(Boolean); return v.length > 3 ? +v[3] : 1; };
  if (st.bg === 'transparent' || !(alpha(st.bg) > 0)) fail.push(`${scheme}: capsule background ${st.bg}`);
  if (!(st.bw === '0px' || st.bs === 'none')) fail.push(`${scheme}: capsule border ${st.bw} ${st.bs}`);
  if (!(st.radius >= st.h / 2)) fail.push(`${scheme}: radius ${st.radius} < height/2 ${st.h / 2}`);
  if (hover === st.bg) fail.push(`${scheme}: no hover change (${hover})`);
  console.log(`capsule ${scheme} ${JSON.stringify(Object.assign(st, { hover }))}`);
  return fail;
}

(async () => {
  const b = await chromium.launch();
  let fail = [];
  try {
    fail = fail.concat(await run(b, false));
    fail = fail.concat(await run(b, true));
    fail = fail.concat(await capsule(b, 'dark'));
    fail = fail.concat(await capsule(b, 'light'));
    for (const scheme of ['dark', 'light']) for (const width of [1440, 700, 480]) fail = fail.concat(await inline(b, scheme, width));
  } finally { await b.close(); }
  if (fail.length) { console.error(fail.join('\n')); process.exit(1); }
})().catch(e => { console.error(e); process.exit(1); });
