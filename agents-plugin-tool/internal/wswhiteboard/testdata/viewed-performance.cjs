'use strict';

// Full production assets, real layout, no daemon or network. --source is only
// for a negative-control run against an extracted historical stream.js.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_VIEWED_PERF_PLAYWRIGHT);
const assets = path.join(__dirname, '../assets');
assert(process.argv.length === 2 || (process.argv.length === 4 && process.argv[2] === '--source'));
let source = fs.readFileSync(process.argv[3] || path.join(assets, 'stream.js'), 'utf8');
function instrument(needle, replacement) {
  assert.equal(source.split(needle).length, 2, `unique instrumentation site: ${needle}`);
  source = source.replace(needle, replacement);
}
instrument('function viewport(scroller) {', 'function viewport(scroller) { window.work.viewport++;');
instrument('function reading(scroller) {', `function reading(scroller) {
  var before = Object.assign({}, window.work);
  window.work.reading++;
  try { return measuredReading(scroller); }
  finally {
    ['viewport', 'rect', 'style'].forEach(function (key) {
      window.work.max[key] = Math.max(window.work.max[key], window.work[key] - before[key]);
    });
  }
}
function measuredReading(scroller) {`);

// 100 topics, 1200 chronological entries, 300 open panels, rendered as the
// server renders a timeline (15e44e235): whole rows, half of each topic
// behind an inert gap the initial window fill opens. Keep the original large
// single-column board: a tiny mock hides the repeated sticky-head scan. One
// gap row holds an open panel whose frame has no height yet, so its window
// stays pending until "late frame height" sizes it.
const LATE_TOPIC = 50;
let topics = '';
for (let i = 0; i < 100; i++) {
  const row = (id, late) => {
    const body = late
      ? `<article class="wb-entry panel" id="e${id}" data-panel="p${id}"><details open class="entry-panel" id="p${id}"><summary>Panel ${id}</summary><div class="doc-frame" id="late-frame"></div></details></article>`
      : id % 4 === 1
        ? `<article class="wb-entry panel" id="e${id}" data-panel="p${id}"><details open class="entry-panel" id="p${id}"><summary>Panel ${id}</summary><div class="payload" style="height:180px">Reading material ${id}</div></details></article>`
        : `<article class="wb-entry" id="e${id}" data-entry="e${id}">Log ${id}: stable review context and longer wrapping text.</article>`;
    return `<div class="timeline-row" data-row="r${id}" data-count="1" data-from="" data-to="">${body}</div>`;
  };
  let older = '', live = '';
  for (let j = 0; j < 6; j++) older += row(i * 12 + j + 1, i === LATE_TOPIC && j === 4);
  for (let j = 6; j < 12; j++) live += row(i * 12 + j + 1, false);
  topics += `<section class="topic" data-topic="Topic ${i}"><header class="tophead">Topic ${i}</header><div class="wb-history" data-history="Topic ${i}"><div class="history-live"><div class="history-gap" data-count="6" data-from="" data-to=""><span class="gap-label"></span><span class="gap-range"></span><span class="gap-actions"><button type="button" data-load="up">Older</button><button type="button" data-load="down" hidden>Newer</button></span><template data-history-gap>${older}</template></div>${live}</div></div></section>`;
}
const boot = `
window.resetWork = () => window.work = {reading:0, viewport:0, rect:0, style:0, raf:0, ro:0, max:{viewport:0, rect:0, style:0}};
resetWork();
const rect = Element.prototype.getBoundingClientRect;
Element.prototype.getBoundingClientRect = function() { work.rect++; return rect.call(this); };
const style = window.getComputedStyle;
window.getComputedStyle = function(...args) { work.style++; return style.apply(window, args); };
const raf = window.requestAnimationFrame.bind(window);
// Settlement frames use the native function, so they do not count as product work.
window.frames = async n => { for (let i = 0; i < n; i++) await new Promise(resolve => raf(resolve)); };
window.requestAnimationFrame = cb => raf(t => { work.raf++; cb(t); });
const RO = window.ResizeObserver;
window.ResizeObserver = class extends RO {
  constructor(cb) { super((...args) => { work.ro++; cb(...args); }); }
};`;

(async () => {
  const browser = await chromium.launch({
    headless: true, executablePath: process.env.WS_VIEWED_PERF_CHROMIUM,
    args: ['--disable-background-networking', '--no-proxy-server'],
  });
  try {
    const context = await browser.newContext({viewport: {width: 1400, height: 900}, serviceWorkers: 'block'});
    await context.route('**/*', route => route.abort());
    const page = await context.newPage();
    const violations = [];
    page.on('pageerror', error => violations.push(`page error: ${error.message}`));
    const css = fs.readFileSync(path.join(assets, 'page.css'), 'utf8');
    const reports = [];
    async function settle(stage, exercised) {
      // RO delivery -> rAF clip write -> resize delivery -> quiet frame.
      await page.evaluate(() => window.frames(4));
      const settled = await page.evaluate(() => structuredClone(work));
      await page.evaluate(() => window.frames(3));
      const quiet = await page.evaluate(() => structuredClone(work));
      reports.push({stage, settled, quiet});
      function check(ok, message) { if (!ok) violations.push(`${stage}: ${message}`); }
      // A stimulus that reaches the history window must run the RO -> rAF ->
      // capture path; settled membership never changes on resize, so one
      // that does not must cost no capture at all.
      if (exercised) check(settled.reading > 0 && settled.ro > 0 && settled.raf > 0, 'must exercise reading, RO delivery and rAF work');
      else check(settled.reading === 0 && settled.raf === 0, `settled history did capture work (${settled.reading} captures, ${settled.raf} frames)`);
      // One capture per scroller per batch (69c7e50fa): initialization plus
      // one resume frame, never one per history.
      check(settled.reading <= 2, `${settled.reading} captures > 2`);
      check(settled.max.viewport <= 1, `viewport/capture ${settled.max.viewport} > 1`);
      check(settled.max.rect <= 2000, `rect/capture ${settled.max.rect} > 2000`);
      check(settled.max.style <= 2000, `style/capture ${settled.max.style} > 2000`);
      check(JSON.stringify(settled) === JSON.stringify(quiet), 'work continued during three quiet frames');
    }
    await page.setContent(`<style>${css}\nbody{margin:0}#bcol{height:calc(100vh - 50px);overflow:auto}.topic{margin:12px}.tophead{position:sticky;top:0;height:32px;background:white}</style><div id="bcol">${topics}</div><script>${boot}</script><script>${source}</script>`, {timeout: 60000});
    // Rows a pending window has not opened yet still sit in their gap template.
    assert.deepEqual(await page.evaluate(() => {
      const count = sel => document.querySelectorAll(sel).length + Array.from(document.querySelectorAll('template[data-history-gap]')).reduce((n, t) => n + t.content.querySelectorAll(sel).length, 0);
      return [document.querySelectorAll('.topic').length, count('.wb-entry'), count('.entry-panel')];
    }), [100, 1200, 300]);
    await settle('initial load', true);
    await page.evaluate(() => resetWork());
    await page.setViewportSize({width: 1000, height: 760});
    await settle('viewport resize', true);
    await page.evaluate(() => { resetWork(); document.getElementById('late-frame').style.height = '120px'; });
    await settle('late frame height', true);
    await page.evaluate(() => { resetWork(); document.querySelector('.payload').style.height = '260px'; });
    await settle('panel resize', false);
    // Report every stimulus before asserting: the negative control must show
    // both resize costs, rather than stopping at its first expensive capture.
    console.log(JSON.stringify({settlementFrames: 4, quietFrames: 3, reports, violations}, null, 2));
    assert.equal(violations.length, 0, violations.join('\n'));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
