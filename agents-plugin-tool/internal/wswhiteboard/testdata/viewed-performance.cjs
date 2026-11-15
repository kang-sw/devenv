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

// 100 topics, 1200 chronological entries, 300 open panels. Keep the original
// large single-column board: a tiny mock hides the repeated sticky-head scan.
let topics = '';
for (let i = 0; i < 100; i++) {
  let entries = '';
  for (let j = 0; j < 12; j++) {
    const id = i * 12 + j + 1;
    entries += j % 4 === 0
      ? `<article class="wb-entry panel" id="e${id}" data-panel="p${id}"><details open class="entry-panel" id="p${id}"><summary>Panel ${id}</summary><div class="payload" style="height:180px">Reading material ${id}</div></details></article>`
      : `<article class="wb-entry" id="e${id}" data-entry="e${id}">Log ${id}: stable review context and longer wrapping text.</article>`;
  }
  topics += `<section class="topic" data-topic="Topic ${i}"><header class="tophead">Topic ${i}</header><div class="wb-history" data-history="Topic ${i}"><button hidden class="history-older">Older</button><div class="history-clip"><div class="history-live">${entries}</div></div></div></section>`;
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
    async function settle(stage) {
      // RO delivery -> rAF clip write -> resize delivery -> quiet frame.
      await page.evaluate(() => window.frames(4));
      const settled = await page.evaluate(() => structuredClone(work));
      await page.evaluate(() => window.frames(3));
      const quiet = await page.evaluate(() => structuredClone(work));
      reports.push({stage, settled, quiet});
      function check(ok, message) { if (!ok) violations.push(`${stage}: ${message}`); }
      check(settled.reading > 0 && settled.ro > 0 && settled.raf > 0, 'must exercise reading, RO delivery and rAF work');
      check(settled.max.viewport <= 1, `viewport/capture ${settled.max.viewport} > 1`);
      check(settled.max.rect <= 2000, `rect/capture ${settled.max.rect} > 2000`);
      check(settled.max.style <= 2000, `style/capture ${settled.max.style} > 2000`);
      check(JSON.stringify(settled) === JSON.stringify(quiet), 'work continued during three quiet frames');
    }
    await page.setContent(`<style>${css}\nbody{margin:0}#bcol{height:calc(100vh - 50px);overflow:auto}.topic{margin:12px}.tophead{position:sticky;top:0;height:32px;background:white}</style><div id="bcol">${topics}</div><script>${boot}</script><script>${source}</script>`, {timeout: 60000});
    assert.deepEqual(await page.evaluate(() => [document.querySelectorAll('.topic').length, document.querySelectorAll('.wb-entry').length, document.querySelectorAll('.entry-panel').length]), [100, 1200, 300]);
    await settle('initial load');
    await page.evaluate(() => resetWork());
    await page.setViewportSize({width: 1000, height: 760});
    await settle('viewport resize');
    await page.evaluate(() => { resetWork(); document.querySelector('.payload').style.height = '260px'; });
    await settle('panel resize');
    // Report every stimulus before asserting: the negative control must show
    // both resize costs, rather than stopping at its first expensive capture.
    console.log(JSON.stringify({settlementFrames: 4, quietFrames: 3, reports, violations}, null, 2));
    assert.equal(violations.length, 0, violations.join('\n'));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
