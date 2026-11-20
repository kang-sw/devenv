// Static large-board harness: real CSS, capsule editor and inline composer note.
// WS_NOTE_BASELINE optionally reads pre-fix dashboard-composer.js/dashboard-rail.css.
// WS_NOTE_TRACE_DIR optionally retains the invalidation trace; otherwise memory only.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const { readAssetSource } = require('./asset-source.cjs');
const assets = path.join(__dirname, '../assets');
const baseline = process.env.WS_NOTE_BASELINE;
const composer = fs.readFileSync(path.join(baseline || assets, 'dashboard-composer.js'), 'utf8');
function func(name) {
  const match = composer.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  assert(match, 'missing production function ' + name);
  return match[0];
}
(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.WS_CHROMIUM_EXECUTABLE ? { executablePath: process.env.WS_CHROMIUM_EXECUTABLE } : {}) });
  try {
    const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
    await page.route('**/*', route => route.abort());
    let css = readAssetSource('wswhiteboard', 'page.css') + readAssetSource('wsdashboard', 'dashboard.css');
    if (baseline) css = css.replace(fs.readFileSync(path.join(assets, 'dashboard-rail.css'), 'utf8'), fs.readFileSync(path.join(baseline, 'dashboard-rail.css'), 'utf8'));
    // Fixed fixture geometry only; selectors/containment remain production-owned.
    css += ' .tnote + .wsc-editor{min-height:32px;padding:4px} #bcol{display:block!important;height:900px;overflow:auto} #grid{height:1000px!important}';
    const history = '<div class="msg user"><div class="bub">History <span>text</span></div></div>'.repeat(1500);
    const board = Array.from({ length: 100 }, (_, i) => '<section class="topic" data-topic="t' + i + '"><header class="tophead">Topic ' + i + '</header>' + '<div class="md"><p>Board text <b>bold</b><span> line</span></p></div>'.repeat(25) + (i === 0 ? '<textarea class="tnote" data-topic="t0"></textarea><div class="card" data-id="1"><textarea class="note"></textarea></div>' : '') + '</section>').join('');
    await page.setContent('<!doctype html><html><head><style>' + css + '</style></head><body class="dash" data-dashboard-layout="split"><div class="dbody"><div id="grid" class="dgrid live" data-layout="split"><div class="convo">' + history + '</div><div id="bcol" class="bcol"><div id="r-board" class="r-board">' + board + '</div></div><div id="composer" class="composer"><div id="cchips" class="cchips"></div><textarea id="ctext"></textarea></div></div></div><div class="bpzone"></div></body></html>');
    await page.evaluate(() => { window.wsT = k => k; });
    await page.addScriptTag({ content: fs.readFileSync(path.join(assets, 'capsules.js'), 'utf8') });
    await page.addScriptTag({ content: `
      var doc=document,cimgs=null;
      function $(id){return doc.getElementById(id);}
      function q(s,el){return Array.from((el||doc).querySelectorAll(s));}
      function region(n){return $('r-'+n);}
      function boardQ(s){return q(s,region('board'));}
      function setCheckNo(){}
      ` + composer.slice(0, composer.indexOf('  function renderSelectionChips()')) + func('topicNotes') + `
      function renderSelectionChips(){}
      document.querySelectorAll('textarea.note,textarea.tnote').forEach(f=>WsCapsules.attach(f));
      document.addEventListener('input',e=>{if(e.target.closest('#r-board'))syncChips();});
    ` });
    await page.evaluate(() => document.querySelector('.tnote').focus());
    await page.keyboard.type('warmup');
    await page.waitForTimeout(100);
    assert(await page.evaluate(() => {
      const note = document.querySelector('#ctext').wsc.ordered().find(r => r.kind === 'topic-note' && r.key === 't0');
      return note?.payload.note === 'warmup';
    }), 'warmup note appears in inline composer');
    const cdp = await page.context().newCDPSession(page), events = [];
    cdp.on('Tracing.dataCollected', e => events.push(...e.value));
    await cdp.send('Tracing.start', { categories: 'toplevel,devtools.timeline,blink,disabled-by-default-devtools.timeline.invalidationTracking', transferMode: 'ReportEvents' });
    for (let i = 0; i < 50; i++) {
      await page.keyboard.type('x');
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    }
    const done = new Promise(resolve => cdp.once('Tracing.tracingComplete', resolve));
    await cdp.send('Tracing.end'); await done;
    if (process.env.WS_NOTE_TRACE_DIR) fs.writeFileSync(path.join(process.env.WS_NOTE_TRACE_DIR, baseline ? 'before.json' : 'after.json'), JSON.stringify({ traceEvents: events }));
    const style = events.filter(e => e.name === 'UpdateLayoutTree' && e.ph === 'X');
    const layout = events.filter(e => e.name === 'Layout' && e.ph === 'X');
    const tasks = events.filter(e => e.name === 'ThreadControllerImpl::RunTask' && e.ph === 'X' && e.tid === style[0].tid);
    const ms = list => list.reduce((total, e) => total + e.dur / 1000, 0);
    const counts = style.map(e => e.args.elementCount ?? e.args.data?.elementCount ?? 0);
    console.log(JSON.stringify({ mode: baseline ? 'before' : 'after', elements: await page.locator('*').count(), keys: 50,
      styleElements: [...new Set(counts)], styleTotalMs: ms(style), stylePerKeyMs: ms(style) / 50,
      layoutTotalMs: ms(layout), layoutPerKeyMs: ms(layout) / 50,
      mainTaskTotalMs: ms(tasks), mainTaskPerKeyMs: ms(tasks) / 50,
      mainTaskMaxMs: Math.max(...tasks.map(e => e.dur / 1000)) }, null, 2));
    assert.equal(await page.evaluate(() => document.querySelector('.tnote').value), 'warmup' + 'x'.repeat(50));
    if (!baseline) {
      assert(style.length > 0, 'trace contains measured style work');
      assert(Math.max(...counts) < 100, 'typing never restyles the whole document');
      assert(await page.evaluate(() => {
        const rows = document.querySelector('#ctext').wsc.ordered().filter(r => r.kind === 'topic-note' && r.key === 't0');
        return rows.length === 1 && rows[0].payload.note === document.querySelector('.tnote').value &&
          document.querySelectorAll('#ctext + .wsc-editor [data-wsc-kind="topic-note"][data-wsc-key="t0"]').length === 1;
      }), 'inline topic note tracks all 50 inputs');
    }
    // Native Korean IME composition follows the same board-input path.
    for (const text of ['ㅎ', '하', '한', '한ㄱ', '한그', '한글']) {
      await cdp.send('Input.imeSetComposition', { text, selectionStart: text.length, selectionEnd: text.length });
    }
    await cdp.send('Input.insertText', { text: '한글' });
    const state = await page.evaluate(() => {
      const field = document.querySelector('.tnote');
      const rows = document.querySelector('#ctext').wsc.ordered().filter(r => r.kind === 'topic-note' && r.key === 't0');
      return { value: field.value, inlineNotes: rows, label: document.querySelector('#ctext + .wsc-editor [data-wsc-kind="topic-note"][data-wsc-key="t0"]')?.textContent, capsules: field.wsc.snapshot() };
    });
    assert(state.value.endsWith('한글'));
    assert.equal(state.inlineNotes.length, 1);
    assert.equal(state.inlineNotes[0].payload.note, state.value.trim());
    assert(state.label.includes('한글'), 'inline topic note displays the composed text');
    assert.deepEqual(state.capsules, []);
    console.log('note typing: native input, backing value, inline composer note and Korean IME passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
