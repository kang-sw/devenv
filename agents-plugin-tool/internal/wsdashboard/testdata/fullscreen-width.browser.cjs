// Offline acceptance for the shared sheet. Run from the repository root:
// WS_PLAYWRIGHT=<cached module> WS_FULLSCREEN_EVIDENCE=<dir> node <this file>
// Only this process's ephemeral HTTP server and Chromium are started.
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT);
const evidence = process.env.WS_FULLSCREEN_EVIDENCE;
assert(evidence, 'an explicit evidence directory is required');
fs.mkdirSync(evidence, { recursive: true });
const base = 'agents-plugin-tool/internal/wsdashboard/assets/';
const read = name => fs.readFileSync(base + name, 'utf8');
const sprite = fs.readdirSync(base + 'lucide/icons').filter(n => n.endsWith('.svg')).map(n => {
  return `<symbol id="l-${n.slice(0, -4)}" viewBox="0 0 24 24">${read('lucide/icons/' + n).match(/<svg\b[^>]*>([\s\S]*)<\/svg>/)[1]}</symbol>`;
}).join('');
const translations = fs.readFileSync('agents-plugin-tool/internal/wsi18n/tables/en.json', 'utf8');
const html = `<!doctype html><html data-rid="fixture" data-theme="light"><head><meta charset="utf-8">
<link rel="stylesheet" href="/page.css"><link rel="stylesheet" href="/dashboard.css"></head>
<body class="dash live"><svg width="0" height="0" style="position:absolute">${sprite}</svg>
<div class="dwrap"><div class="top"><h1>Shared fullscreen width — isolated fixture</h1></div><div class="dbody">
<main class="dgrid live" id="grid" data-tab="board"><div class="r-tree">Session / fixture</div><div class="thead chead">Conversation / Files</div>
<div class="convo" id="convo"><p>Owning conversation column</p></div><div class="composer">Fixture — no daemon, terminal or provider</div>
<div class="bcol" id="bcol"><div class="r-board" id="r-board"><section class="topic" data-topic="width"><div class="tophead"><div class="tbar"><b>Topic with mixed content</b></div></div>
<div class="tdoc"><p>Topic owns its category; inner code or media never overrides it.</p></div>
<article class="wb-entry panel" data-kind="panel"><details class="entry-panel" data-key="entry:p1"><summary><span class="entry-recap">Framed entry panel</span><span class="pmeta">p1 · 2 versions</span><button class="ibtn d-max">↗</button><button class="ibtn d-min">↙</button></summary>
<div class="doc-body entry-body framed"><iframe title="stable entry frame" sandbox="allow-scripts" srcdoc="<p>Stable frame</p>"></iframe><details class="inner-disclosure"><summary>Inner disclosure</summary>Preserved body</details>${'<p>Scrollable panel content</p>'.repeat(50)}</div>
<details class="panel-versions"><summary>Versions</summary>Independent version state</details></details></article></section></div></div></main></div></div>
<aside class="bpins" id="bpins"></aside><section class="fpane" id="toolbar-files" hidden><div class="fhd">Files / fixture</div><input class="fq" aria-label="Search files"><div class="ftree">docs/ · src/ · README.md</div></section>
<script>const translations=${translations}; function wsT(k,a,b){return k==='js.board.toolbar.count'?'1 open topic':translations[k]||k;}</script>
<script src="/cards.js"></script><script src="/preview.js"></script><script>
var W=window.WsCards, board=document.getElementById('r-board'), col=document.getElementById('bpins');
var files=document.getElementById('toolbar-files'), counters={shown:0,hidden:0,sized:0};
W.register('terminal',{open:function(c){c.title({icon:'square-terminal',name:'Terminal / measured board column'});c.body.innerHTML='<pre>fixture@workspace % go test -run TestFullscreenPanelWidth\\nPASS\\n\\nWidth changes preserve the same content node.\\nNormal width comes from this terminal’s owning column.\\nUse the adjacent button to extend and restore.</pre>';},sized:function(){counters.sized++;}});
W.register('web',{open:function(c){c.title({icon:'globe',name:'Web card'});c.body.innerHTML='<iframe title="stable web" srcdoc="<h2>Stable web content</h2>"></iframe>';}});
['approval','system','other'].forEach(function(k){W.register(k,{open:function(c){c.title({name:k});}});});
var overviewHeld=false, overviewAfter=null;
window.WsOverview={holding:function(){return overviewHeld;},after:function(f){overviewAfter=f;}};
W.mount(board,{key:'fixture-cards',load:function(k){return sessionStorage.getItem(k);},save:function(k,v){if(v===null)sessionStorage.removeItem(k);else sessionStorage.setItem(k,v);},column:col,
 shows:function(n){return innerWidth>=1600&&n>0;},layout:function(on){document.documentElement.toggleAttribute('data-pincol',on);},
 toolbar:{files:{el:files,shown:function(){counters.shown++;},hidden:function(){counters.hidden++;}}}});
window.fixtureReady=true;
</script></body></html>`;
const server = http.createServer((req, res) => {
  const u = new URL(req.url, 'http://fixture');
  if (u.pathname === '/') { res.setHeader('Content-Type', 'text/html'); res.end(html); return; }
  if (u.pathname === '/s/fixture/fs/preview') {
    const kind = u.searchParams.get('path').split('.')[0];
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ kind: kind === 'code' ? 'text' : kind, source: kind === 'code', text: 'const width = "semantic category";\n'.repeat(12), html: '<h1>Markdown preview</h1><p>Prose keeps its category width.</p>', diff: [], frame: '/frame' })); return;
  }
  const assets = { '/cards.js': read('cards.js'), '/preview.js': read('preview.js'), '/dashboard.css': read('dashboard.css'), '/page.css': fs.readFileSync('agents-plugin-tool/internal/wswhiteboard/assets/page.css', 'utf8') };
  if (assets[u.pathname]) { res.setHeader('Content-Type', u.pathname.endsWith('.css') ? 'text/css' : 'text/javascript'); res.end(assets[u.pathname]); return; }
  if (u.pathname === '/frame') { res.end('<p>Sandboxed preview</p>'); return; }
  res.writeHead(404); res.end();
});
let browser;
const observations = [], screenshots = [];
async function rect(page, selector) { return page.locator(selector).evaluate(n => { const r = n.getBoundingClientRect(); return {left:r.left,right:r.right,width:r.width,top:r.top,bottom:r.bottom}; }); }
function near(actual, expected, name) { assert(Math.abs(actual - expected) <= 1, `${name}: ${actual} != ${expected}`); }
async function geometry(page, selector, width, expected, name) {
  const r = await rect(page, selector); near(r.width, expected, name); near(r.left, (width - expected) / 2, name + ' centered');
  observations.push({name,viewport:width,...r});
}
async function preview(page, kind) {
  await page.evaluate(kind => { window.activeCard = WsPreview.open(kind + '.fixture'); }, kind);
  await page.waitForFunction(() => activeCard.el.hasAttribute('data-preview-kind'));
  await page.evaluate(() => activeCard.maxBtn.click());
}
async function shot(page, name) {
  const file = path.join(evidence, name + '.png'); await page.screenshot({path:file}); screenshots.push(file);
}
(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  assert.notEqual(server.address().port, 29417);
  browser = await chromium.launch({headless:true});
  const context = await browser.newContext({viewport:{width:2400,height:900}});
  await context.route('**/*', route => route.request().url().startsWith(origin) ? route.continue() : route.abort());
  const page = await context.newPage(); page.setDefaultTimeout(4000);
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.goto(origin); await page.waitForFunction(() => window.fixtureReady);
  // Actual preview renderer responses supply identity, including source-vs-prose.
  for (const [kind, expected] of [['text',1280],['markdown',1280],['code',1440],['diff',1760],['frame',1600],['image',1600],['video',1600],['other',1920]]) {
    await preview(page,kind); await geometry(page,'.bcard[data-max] .bc-in',2400,expected,'preview '+kind);
    await page.locator('.bcard[data-max] .bc-min').click();
  }
  for (const [kind, expected] of [['web',1600],['approval',1024],['system',1024],['other',1920]]) {
    await page.evaluate(kind => { window.testCard=W.open(kind,{}); testCard.maxBtn.click(); },kind);
    await geometry(page,'.bcard[data-max] .bc-in',2400,expected,kind);
    if(kind==='web') {
      await page.evaluate(() => { window.keptWeb=testCard.body.firstChild; });
      await page.locator('.bcard[data-max] .sheet-width').click();
      await geometry(page,'.bcard[data-max] .bc-in',2400,2368,'extension beyond cap');
      assert(await page.evaluate(() => keptWeb===testCard.body.firstChild));
    }
    await page.locator('.bcard[data-max] .bc-min').click();
  }
  // Retire the prior cases' cards: a growing synthetic flow is not the page's
  // normal preview-slot lifecycle and would bury the following topic controls.
  await page.evaluate(() => { W.list().slice().forEach(c => c.close()); document.getElementById('bcol').scrollTop=0; });
  // Topic and entry-panel identity wins even when an inner frame contains media.
  await page.locator('section.topic > .tophead .d-max').click();
  await geometry(page,'section.topic[data-max]',2400,1440,'topic');
  await page.locator('section.topic > .tophead .d-min').click();
  await page.locator('.entry-panel > summary > .d-max').click();
  await geometry(page,'.entry-panel[data-max]',2400,1440,'framed panel');
  const panelButtons = await page.locator('.entry-panel[data-max] > summary').evaluate(n => {
    return ['.sheet-width','.d-min'].map(s => {const r=n.querySelector(s).getBoundingClientRect();return [r.left,r.right,r.top];});
  });
  assert(panelButtons[0][1] <= panelButtons[1][0], 'entry-panel controls do not overlap');
  await page.evaluate(() => { window.keptEntry=document.querySelector('.entry-panel iframe'); document.querySelector('.inner-disclosure').open=true; document.querySelector('.panel-versions').open=true; document.querySelector('.entry-panel').scrollTop=180; });
  await page.locator('.entry-panel[data-max] .sheet-width').click();
  assert(await page.evaluate(() => keptEntry===document.querySelector('.entry-panel iframe') && document.querySelector('.inner-disclosure').open && document.querySelector('.panel-versions').open && document.querySelector('.entry-panel').scrollTop===180));
  const panelRect=await rect(page,'.entry-panel[data-max]'), headRect=await rect(page,'.entry-panel[data-max] > summary');
  near(headRect.top,panelRect.top+1,'sticky panel header');
  await page.locator('.entry-panel[data-max] .d-min').click();
  // Files uses the conversation grid pane, not the board or hidden explorer.
  const convoWidth=(await rect(page,'#convo')).width;
  await page.locator('.board-files').click();
  await geometry(page,'.fpane[data-max]',2400,Math.min(1920,convoWidth),'Files owning column');
  await page.locator('.board-toolbar .sheet-width').click();
  await geometry(page,'.fpane[data-max]',2400,2368,'Files extended');
  await page.locator('.board-toolbar .sheet-width').click();
  await page.locator('.board-files').click();
  assert(await page.locator('.board-files').evaluate(n=>document.activeElement===n));
  // Terminal takes its original board/pinned column once, across viewport reflow.
  await page.evaluate(()=>{window.term=W.open('terminal',{});});
  const boardWidth=(await rect(page,'#r-board')).width;
  await page.evaluate(()=>term.maxBtn.click());
  await geometry(page,'.bcard[data-max] .bc-in',2400,Math.min(1920,boardWidth),'Terminal owning board');
  await page.setViewportSize({width:800,height:900});
  await geometry(page,'.bcard[data-max] .bc-in',800,Math.min(768,boardWidth,1920),'resize clamp');
  await page.setViewportSize({width:2400,height:900});
  await geometry(page,'.bcard[data-max] .bc-in',2400,Math.min(1920,boardWidth),'original snapshot after resize');
  await page.evaluate(()=>{term.minBtn.click();term.pin(true);});
  const pinnedWidth=(await rect(page,'#bpins')).width;
  await page.evaluate(()=>term.maxBtn.click());
  await geometry(page,'.bcard[data-max] .bc-in',2400,Math.min(1920,pinnedWidth),'Terminal owning pinned column');
  await page.evaluate(()=>{term.minBtn.click();term.pin(false);});
  // Overview hands off before maximizing rather than admitting a second owner.
  await page.evaluate(()=>{overviewHeld=true;term.maxBtn.click();});
  assert.equal(await page.locator('[data-max]').count(),0);
  await page.evaluate(()=>{overviewHeld=false;overviewAfter();});
  await page.keyboard.press('Escape'); assert.equal(await page.locator('[data-max]').count(),0);
  // Required theme/viewport captures show the same measured Terminal in both states.
  for(const width of [1280,1900]) for(const theme of ['light','dark']) {
    await page.setViewportSize({width,height:900});
    await page.evaluate(theme=>document.documentElement.setAttribute('data-theme',theme),theme);
    const host=(await rect(page,'#r-board')).width;
    await page.evaluate(()=>term.maxBtn.click());
    await geometry(page,'.bcard[data-max] .bc-in',width,Math.min(host,1920,width-32),'capture normal');
    await shot(page,`${width}-${theme}-normal`);
    const toggle=page.locator('.bcard[data-max] .sheet-width'); assert(await toggle.isVisible());
    assert.equal(await toggle.getAttribute('aria-pressed'),'false');
    await toggle.click(); await geometry(page,'.bcard[data-max] .bc-in',width,width-32,'capture extended');
    assert.equal(await toggle.getAttribute('aria-label'),'Restore normal width');
    assert.equal(await toggle.getAttribute('aria-pressed'),'true');
    await shot(page,`${width}-${theme}-extended`);
    await toggle.click(); await geometry(page,'.bcard[data-max] .bc-in',width,Math.min(host,1920,width-32),'restore in sheet');
    await page.locator('.bcard[data-max] .bc-min').click();
  }
  await page.setViewportSize({width:640,height:900});
  await page.evaluate(()=>term.maxBtn.click());
  await geometry(page,'.bcard[data-max] .bc-in',640,624,'mobile unchanged');
  near((await rect(page,'.bcard[data-max] .bc-in')).top,46,'mobile top');
  assert.equal(await page.locator('.bcard[data-max] .sheet-width').isVisible(),false);
  await shot(page,'640-dark-normal'); await page.keyboard.press('Escape');
  await page.setViewportSize({width:1280,height:900}); await preview(page,'markdown');
  assert.equal(await page.locator('.bcard[data-max] .sheet-width').isVisible(),false,'ineffective extension absent at wide clamped width');
  await page.keyboard.press('Escape');
  // Reopen and reload never remember extension; content is not remounted by width.
  await page.setViewportSize({width:2400,height:900});
  await page.evaluate(()=>term.maxBtn.click()); await page.locator('.bcard[data-max] .sheet-width').click();
  await page.locator('.bc-scrim').click({position:{x:1,y:1}});
  await page.evaluate(()=>term.maxBtn.click());
  assert.equal(await page.locator('.bcard[data-max] .sheet-width').getAttribute('aria-pressed'),'false');
  await page.locator('.bcard[data-max] .sheet-width').click(); await page.reload(); await page.waitForFunction(()=>window.fixtureReady);
  await page.evaluate(()=>{window.term=W.list('terminal')[0];term.maxBtn.click();});
  assert.equal(await page.locator('.bcard[data-max] .sheet-width').getAttribute('aria-pressed'),'false');
  assert.deepEqual(errors,[]);
  fs.writeFileSync(path.join(evidence,'browser-results.json'),JSON.stringify({observations,screenshots,errors},null,2));
  console.log(`PASS: ${observations.length} geometry assertions; category/lifecycle/focus/Overview/disclosures/sticky/frame/reset checks; ${screenshots.length} screenshots`);
})().catch(e=>{console.error(e.stack);process.exitCode=1;}).finally(async()=>{if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));});
