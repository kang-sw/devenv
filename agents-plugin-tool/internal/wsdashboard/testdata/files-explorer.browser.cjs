// Offline appearance fixture: actual revision assets and dashboard initialization.
// Run from the repository root; requires an explicitly supplied cached Playwright.
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT);
const baseline = 'e86f22a82189ac6bb556e7cad84352b566a65b8b';
const prefix = 'agents-plugin-tool/internal/wsdashboard/assets/';
const evidence = process.env.WS_FILES_EVIDENCE;
assert(evidence, 'owned evidence directory required');
fs.mkdirSync(evidence, { recursive: true });
function source(rev, file) { return rev === 'result' ? fs.readFileSync(file, 'utf8') : execFileSync('git', ['show', rev + ':' + file], { encoding: 'utf8' }); }
function initFrom(src, result) {
  if (result) {
    const start = src.indexOf('  if (live && fpane && window.WsFiles) {');
    const end = src.indexOf('  var cardBoard', start);
    assert(start > 0 && end > start);
    return src.slice(start, end);
  }
  const start = src.indexOf('  if (live && fpane && window.WsFiles) {');
  const end = src.indexOf('  // morphBoard', start);
  assert(start > 0 && end > start);
  return src.slice(start, end);
}
function sprite(rev) {
  return '<svg width="0" height="0" style="position:absolute" aria-hidden="true">' + fs.readdirSync(prefix + 'lucide/icons').filter(n => n.endsWith('.svg')).map(n => {
    const body = source(rev, prefix + 'lucide/icons/' + n).match(/<svg\b[^>]*>([\s\S]*)<\/svg>/)[1];
    return `<symbol id="l-${n.slice(0, -4)}" viewBox="0 0 24 24">${body}</symbol>`;
  }).join('') + '</svg>';
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  const errors = [], captures = [];
  try {
    for (const rev of [baseline, 'result']) {
      const result = rev === 'result';
      const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1, colorScheme: 'light', reducedMotion: 'reduce' });
      page.on('pageerror', e => errors.push(rev + ': ' + e.stack));
      await page.route('**/*', route => { errors.push('unexpected network: ' + route.request().url()); return route.abort(); });
      const css = source(rev, 'agents-plugin-tool/internal/wswhiteboard/assets/page.css') + '\n' + source(rev, prefix + 'dashboard.css');
      await page.setContent(`<!doctype html><html data-theme="light" data-sysopen data-rid="fixture"><head><style>${css}</style></head><body class="dash live">${sprite(rev)}<div class="dwrap"><div class="top"><span class="meta">Project</span><span class="meta sname">Conversation fixture</span><span class="host">pi</span></div><div class="dbody"><main id="grid" class="dgrid live" data-layout="split" data-tab="convo" data-ctab="files"><section id="files" class="fpane" aria-label="Files"></section><div class="r-tree"><div class="thead">Agents</div><p>Session · active</p></div><div id="splitter" class="splitter"></div><div id="chead" class="thead chead"><nav id="ctabs" class="ctabs"><button data-ctab="convo" aria-pressed="false">Conversation</button><button data-ctab="files" aria-pressed="true">Files</button></nav><span class="csum"></span><span class="fsum"></span></div><div id="convo" class="convo">Conversation</div><div id="composer" class="composer"><div class="crow"><textarea placeholder="Message"></textarea></div></div><div id="bcol" class="bcol"><div id="r-board"><section class="topic" data-topic="fixture"><div class="tophead"><div class="tbar"><span class="tlead"><span class="tname">Current work</span></span><time class="tago" data-at="1"></time></div></div><p>Read-only dashboard fixture</p></section></div></div></main></div></div></body></html>`);
      await page.evaluate(() => {
        const labels = { 'js.board.toolbar.files': 'Files', 'js.board.toolbar.search': 'Search', 'js.board.toolbar.overview': 'Overview', 'js.files.mode.name': 'Name', 'js.files.mode.content': 'Content', 'js.files.find.name': 'Find files', 'js.files.find.content': 'Search content', 'js.files.view': 'View', 'js.files.view.ign': 'Show ignored files', 'js.files.view.dot': 'Hide dot files' };
        window.wsT = (k, a, b) => k === 'js.board.toolbar.count' ? `open ${a} · closed ${b}` : labels[k] || k;
        window.wsTn = window.wsT;
        window.fetch = async input => {
          const u = new URL(input, 'http://fixture.invalid');
          if (u.pathname !== '/s/fixture/fs/list') throw new Error('unmocked fetch ' + input);
          const dir = u.searchParams.get('dir') || '';
          return { ok: true, json: async () => ({ root: '/project', name: 'project', session: true, abs: '/project' + (dir ? '/' + dir : ''), sep: '/', entries: dir ? [{ name: 'main.go', st: 'M' }] : [{ name: 'src', dir: true, st: '*' }, { name: 'README.md' }, { name: '.gitignore' }, { name: 'go.mod' }] }) };
        };
      });
      await page.addScriptTag({ content: source(rev, prefix + 'cards.js') });
      await page.addScriptTag({ content: source(rev, prefix + 'files.js') });
      const dashboard = source(rev, prefix + 'dashboard.js');
      const initialization = initFrom(dashboard, result);
      fs.writeFileSync(path.join(evidence, (result ? 'result' : 'baseline') + '-dashboard-init.js'), initialization);
      const toolbar = dashboard.match(/files: (filesSheet|fpane && window\.WsFiles \? revealPanes : null)/);
      assert(toolbar, 'production toolbar owner wiring missing');
      await page.addScriptTag({ content: `(function(){
        var doc=document, root=doc.documentElement, rid='fixture', live=true;
        var fpane=doc.getElementById('files'), chead=doc.getElementById('chead'), filesTab=null, filesSheet=null;
        var store={}; function load(k){return store[k]||null;} function save(k,v){store[k]=v;}
        function post(){throw new Error('no host actions');} function toast(){} function copyText(){}
        function revealPanes(){if(fpane.getClientRects().length)window.WsFiles.shown();else window.WsFiles.hidden();}
        ${initialization}
        window.WsCards.mount(doc.getElementById('r-board'), {key:'cards',load:load,save:save,toolbar:{overview:function(){},search:function(){},${toolbar[0]}},column:null,shows:function(){return false;}});
        ${result ? 'filesTab.shown();' : 'window.WsFiles.shown();'}
      })();` });
      await page.locator('#files .fr').first().waitFor();
      async function capture(state) {
        await page.mouse.move(1439, 899);
        await page.evaluate(() => { window.captureFocus = document.activeElement; document.activeElement.blur(); });
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        const filename = (result ? 'result' : 'baseline') + '-' + state + '.png';
        await page.screenshot({ path: path.join(evidence, filename) });
        captures.push(filename);
        await page.evaluate(() => window.captureFocus.focus({ preventScroll: true }));
      }
      await page.locator('#files [data-path="src"]').click();
      await page.locator('#files [data-path="src/main.go"]').waitFor();
      await capture('tab-tree');
      await page.locator('#files .fhd button').nth(3).click();
      await capture('tab-view');
      await page.keyboard.press('Escape');
      await page.locator('#files [data-path="src"]').click();
      await page.locator('.board-files').click();
      const sheet = page.locator('.fpane[data-max]');
      await sheet.locator('.fr').first().waitFor();
      await sheet.locator('[data-path="src"]').click();
      await sheet.locator('[data-path="src/main.go"]').waitFor();
      await capture('toolbar-tree');
      await sheet.locator('.fhd button').nth(3).click();
      await capture('toolbar-view');
      await page.keyboard.press('Escape');
      assert.equal(await sheet.count(), 1, 'popover Escape must not dismiss sheet');
      await page.keyboard.press('Escape');
      assert.equal(await page.locator('.fpane[data-max]').count(), 0);
      assert.equal(await page.locator('#grid').getAttribute('data-ctab'), 'files');
      if (result) assert.equal(await page.locator('#chead .fsum .froot').count(), 1, 'tab label retained');
      await page.close();
    }
    assert.deepEqual(errors, []);
    const comparison = await browser.newPage({ viewport: { width: 2880, height: 950 }, deviceScaleFactor: 1 });
    await comparison.route('**/*', r => r.abort());
    const summaries = [];
    for (const state of ['tab-tree', 'tab-view', 'toolbar-tree', 'toolbar-view']) {
      const data = ['baseline', 'result'].map(rev => 'data:image/png;base64,' + fs.readFileSync(path.join(evidence, rev + '-' + state + '.png')).toString('base64'));
      await comparison.setContent(`<style>body{margin:0} img{width:1440px;height:900px} main{display:flex}</style><h3>${state}: baseline | result</h3><main><img src="${data[0]}"><img src="${data[1]}"></main>`);
      const diff = await comparison.evaluate(async () => {
        const images = [...document.images]; await Promise.all(images.map(i => i.decode()));
        const pixels = images.map(i => { const c = document.createElement('canvas'); c.width=i.naturalWidth;c.height=i.naturalHeight; const ctx=c.getContext('2d');ctx.drawImage(i,0,0);return ctx.getImageData(0,0,c.width,c.height).data; });
        let count=0; for(let i=0;i<pixels[0].length;i+=4) if(pixels[0].slice(i,i+4).some((v,j)=>v!==pixels[1][i+j]))count++;
        return { differentPixels:count,totalPixels:images[0].naturalWidth*images[0].naturalHeight };
      });
      summaries.push({state,...diff});
      await comparison.screenshot({path:path.join(evidence,'compare-'+state+'.png')});
    }
    fs.writeFileSync(path.join(evidence, 'comparison.html'), '<!doctype html><style>section{display:flex}img{width:49%}</style>' + summaries.map(s => `<h2>${s.state}: ${s.differentPixels}/${s.totalPixels} differing pixels</h2><section><img src="baseline-${s.state}.png"><img src="result-${s.state}.png"></section>`).join(''));
    fs.writeFileSync(path.join(evidence,'results.json'), JSON.stringify({baseline,result:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),summaries,captures},null,2));
    console.log(JSON.stringify({ evidence, summaries },null,2));
    assert(summaries.every(s => s.differentPixels === 0), 'appearance mismatch: inspect retained comparisons before product changes');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode=1; });
