// Offline visual acceptance using real assets and production page initialization.
// From the repository root: WS_PLAYWRIGHT=<cached module> WS_FOLDER_DOCK_EVIDENCE=<dir> node <this file>
// Starts only an owned ephemeral HTTP server and headless Chromium, never a daemon.
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT);
const { readAssetSource } = require('./asset-source.cjs');
const evidence = process.env.WS_FOLDER_DOCK_EVIDENCE;
assert(evidence, 'an explicit evidence directory is required');
fs.mkdirSync(evidence, { recursive: true });
const read = name => readAssetSource('wsdashboard', name);
const dashboard = read('dashboard-board.js');
const start = dashboard.indexOf('  var filePreview = null, filePreviewBack = null;');
const end = dashboard.indexOf('    if (worktree) {', start);
assert(start >= 0 && end > start, 'production files/sheet/preview initialization and board mount required');
// Close the enclosing if (cardBoard); omit unrelated worktree/overview startup.
const initialization = dashboard.slice(start, end) + '\n  }\n';
const sprite = fs.readdirSync('agents-plugin-tool/internal/wsdashboard/assets/lucide/icons').filter(n => n.endsWith('.svg')).map(n => {
  const body = read('lucide/icons/' + n).match(/<svg\b[^>]*>([\s\S]*)<\/svg>/)[1];
  return `<symbol id="l-${n.slice(0, -4)}" viewBox="0 0 24 24">${body}</symbol>`;
}).join('');
const translations = fs.readFileSync('agents-plugin-tool/internal/wsi18n/tables/en.json', 'utf8');
const topic = (id, title, text) => `<section id="${id}" class="topic" data-topic="${id}"><div class="tophead"><div class="tbar"><span class="tlead"><b class="tname">${title}</b></span></div></div><div class="tdoc"><p>${text}</p></div></section>`;
const html = `<!doctype html><html data-rid="fixture" data-theme="light"><head><meta charset="utf-8">
<link rel="icon" href="data:,"><link rel="stylesheet" href="/page.css"><link rel="stylesheet" href="/dashboard.css"></head>
<body class="dash live"><svg width="0" height="0" style="position:absolute" aria-hidden="true">${sprite}</svg>
<div class="dwrap"><header class="top"><h1>Folder sheet — isolated session fixture</h1><span class="meta">No daemon or hosted session</span></header>
<div class="dbody"><aside class="rail" id="rail"><div class="thead">Sessions</div><p>Local acceptance fixture</p></aside><div class="rscrim"></div>
<main id="grid" class="dgrid live" data-layout="split" data-tab="board" data-ctab="convo">
<div id="chead" class="thead chead"><nav class="ctabs"><button aria-pressed="true">Conversation</button><button aria-pressed="false">Files</button></nav><span class="csum"></span><span class="fsum"></span></div>
<section id="files" class="fpane" aria-label="Files"></section>
<section id="convo" class="convo"><article class="bubble"><p>Keep the toolbar explorer independent of the Files tab.</p></article><article class="bubble"><p>The sheet docks over this entire column. File previews use the reading pane on the right.</p></article></section>
<div id="composer" class="composer"><div class="crow"><textarea aria-label="Message" placeholder="Message the session…"></textarea></div></div>
<div id="bcol" class="bcol"><div id="r-board" class="r-board">${topic('current-topic', 'Current work', 'The board stays in place underneath the reading pane.')}${topic('pinned-topic', 'Pinned reference', 'This durable topic must return unchanged after dismissal.')}</div></div>
</main></div></div>
<script>const translations=${translations}; function wsT(k,a,b){var text=k==='js.board.toolbar.count'?a+' open · '+b+' closed':translations[k]||k;var args=[a,b];return text.replace(/\\{(\\d+)\\}/g,function(_,i){return args[+i];});} function wsTn(k,n){return wsT(k+(n===1?'.one':'.other'),n);}</script>
<script src="/gallery.js"></script><script src="/cards.js"></script><script src="/files.js"></script><script src="/preview.js"></script>
<script>(function(){
var doc=document,root=doc.documentElement,rid='fixture',cardIdentity=rid,live=true,worktree='',mode='live',layoutMode='split',LAYOUT_KEY='layout';
var grid=doc.getElementById('grid'),fpane=doc.getElementById('files'),chead=doc.getElementById('chead'),filesTab=null,filesSheet=null;
function $(id){return doc.getElementById(id);} function region(name){return $('r-'+name);} var store={};
function load(k){return store[k]||null;} function save(k,v){store[k]=v;}
function post(){throw new Error('host action is outside this offline fixture');}
function browsePost(){return post();} function toast(){} function copyText(){} function attachExplorerPath(){}
function gwRender(){throw new Error('unexpected diff renderer');} function railOpen(){}
function tabShown(tab){return grid.getAttribute('data-tab')===tab;} function setTab(tab){grid.setAttribute('data-tab',tab);}
function watchBoard(){} function bandBoard(){} function restoreOpen(){} function mountFrames(){} function mermaid(){}
${initialization}
// Seed an actual durable pin through the framework, retaining production breakpoints.
window.WsCards.move($('pinned-topic'),{pin:true,index:0});
window.folderFixture={setLayout:function(value){layoutMode=value;grid.setAttribute('data-layout',value);window.WsCards.relayout();syncFilePreview();},setFilesTab:function(on){grid.setAttribute('data-ctab',on?'files':'convo');if(on)filesTab.shown();else filesTab.hidden();},current:function(){return filePreview.current();}};
window.addEventListener('resize',syncFilePreview);
window.fixtureReady=true;
})();</script></body></html>`;
const assets = { '/gallery.js': read('gallery.js'), '/cards.js': read('cards.js'), '/files.js': read('files.js'), '/preview.js': read('preview.js'), '/dashboard.css': read('dashboard.css'), '/page.css': readAssetSource('wswhiteboard', 'page.css') };
const unexpected = [];
const server = http.createServer((req, res) => {
  const u = new URL(req.url, 'http://fixture');
  if (u.pathname === '/') { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end(html); return; }
  if (assets[u.pathname]) { res.setHeader('Content-Type', u.pathname.endsWith('.css') ? 'text/css' : 'text/javascript'); res.end(assets[u.pathname]); return; }
  if (u.pathname === '/s/fixture/fs/list') {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ root: '/project', name: 'project', session: true, abs: '/project', sep: '/', entries: [{ name: 'README.md' }, { name: 'notes.txt' }, { name: 'src', dir: true }] })); return;
  }
  if (u.pathname === '/s/fixture/fs/preview') {
    const rel = u.searchParams.get('path');
    if (!['README.md', 'notes.txt'].includes(rel)) { unexpected.push(req.url); res.writeHead(404); res.end(); return; }
    const text = rel === 'README.md' ? '# Docked toolbar explorer\n\nIndependent navigation, shared reading pane.\n' : 'Second file replaces the first preview.\nNo durable card is created by a single click.\n';
    const answer = { kind: rel.endsWith('.md') ? 'markdown' : 'text', rel, abs: '/project/' + rel, size: text.length, text, html: '<h1>Docked toolbar explorer</h1><p>Independent navigation, shared reading pane.</p><h2>Reading space</h2><p>The board and pinned reference remain unchanged underneath this memory-only preview.</p>' };
    res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(answer)); return;
  }
  unexpected.push(req.url); res.writeHead(404); res.end();
});
let browser;
const observations = [], screenshots = [], errors = [];
function check(condition, message) { assert(condition, message); console.log('PASS: ' + message); }
function near(actual, expected, name) { assert(Math.abs(actual - expected) <= 1, `${name}: ${actual} != ${expected}`); }
async function geometry(page) {
  return page.evaluate(() => {
    const rect = selector => { const n=document.querySelector(selector),r=n.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom,width:r.width,height:r.height,visible:getComputedStyle(n).display!=='none'}; };
    return { sheet:rect('#toolbar-files'),header:rect('#chead'),composer:rect('#composer'),board:rect('#bcol'),pins:rect('#bgal'),pane:rect('.file-preview-pane'),gutter:parseFloat(getComputedStyle(document.querySelector('.dwrap')).paddingRight),viewport:document.documentElement.clientWidth };
  });
}
async function settle(page) {
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
}
async function capture(page, size, state) {
  await page.mouse.move(size.width-1,size.height-1); await settle(page);
  const file=path.join(evidence,`${size.width}x${size.height}-${state}.png`);
  await page.screenshot({path:file,caret:'initial'}); screenshots.push(file);
}
async function closed(page, name) {
  await page.waitForFunction(() => document.getElementById('toolbar-files').hidden && document.querySelector('.file-preview-pane').hidden);
  check(await page.evaluate(() => !document.documentElement.hasAttribute('data-file-preview') && !folderFixture.current() && document.querySelector('.bc-scrim').hidden), name);
}
(async () => {
  await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
  const port=server.address().port; assert.notEqual(port,29417);
  const origin=`http://127.0.0.1:${port}`;
  console.log('Offline fixture: '+origin+' (ephemeral port, not 29417)');
  // Below 1600px and in tabs the gallery takes its narrow model: the seeded
  // pin's page, shown when it docks, covers the Main Board (Page 0), whose
  // toolbar holds the folder. Show Page 0 through its rail dot.
  async function showBoard(page){
    const home=page.locator('#bgal .gal-dot.main');
    if (await home.isVisible() && !(await home.evaluate(n=>n.classList.contains('active')))) await home.click();
  }
  browser=await chromium.launch({headless:true});
  for (const size of [{width:1920,height:1080},{width:1440,height:900}]) {
    const context=await browser.newContext({viewport:size,deviceScaleFactor:1,colorScheme:'light',reducedMotion:'reduce'});
    await context.route('**/*',route => {
      if (new URL(route.request().url()).origin === origin) return route.continue();
      errors.push('unexpected external request: '+route.request().url()); return route.abort();
    });
    const page=await context.newPage(); page.setDefaultTimeout(8000);
    page.on('pageerror',e => errors.push(e.stack));
    await page.goto(origin); await page.waitForFunction(() => window.fixtureReady);
    await showBoard(page);
    const label=`${size.width}x${size.height}`,folder=page.locator('.board-files'),sheet=page.locator('#toolbar-files');
    check(await sheet.isHidden() && await page.locator('.file-preview-pane').isHidden(),label+' initial state has both surfaces closed');
    await capture(page,size,'a-closed');
    // Hold a real left-explorer preflight until after docked sheet dismissal.
    for (const dismissal of ['Escape','empty-space click']) {
      await page.evaluate(() => folderFixture.setFilesTab(true));
      const leftFile=page.locator('#files [data-entry="README.md"]'); await leftFile.waitFor();
      let release, requested;
      const held=new Promise(resolve=>{release=resolve;}), started=new Promise(resolve=>{requested=resolve;});
      await page.route('**/fs/preview?**',async route=>{requested();await held;await route.continue();},{times:1});
      await leftFile.click(); await started;
      check(await page.locator('.file-preview-pane').isHidden(),label+' left preflight is pending without a pane');
      await folder.click(); await sheet.locator('[data-entry="README.md"]').waitFor(); await settle(page);
      if (dismissal === 'Escape') await page.keyboard.press('Escape');
      else {
        const bounds=await geometry(page),outside={x:(bounds.sheet.right+bounds.board.left)/2,y:bounds.board.top+100};
        check(await page.evaluate(({x,y})=>document.elementFromPoint(x,y)?.classList.contains('bc-scrim'),outside),label+' pending-preview empty-space click hits scrim');
        await page.mouse.click(outside.x,outside.y);
      }
      await closed(page,label+' '+dismissal+' dismisses sheet before left response');
      const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/s/fixture/fs/preview');
      release(); await (await response).finished(); await settle(page);
      check(await page.evaluate(() => document.getElementById('toolbar-files').hidden && document.querySelector('.file-preview-pane').hidden && !folderFixture.current() && !document.documentElement.hasAttribute('data-file-preview')),label+' '+dismissal+' ignores delayed left preflight: no preview reopens');
      await page.evaluate(() => folderFixture.setFilesTab(false));
    }
    await folder.click(); await sheet.locator('[data-entry="README.md"]').waitFor(); await settle(page);
    const docked=await geometry(page);
    for (const [key,expected] of [['left',docked.header.left],['right',docked.header.right],['top',docked.header.top],['bottom',docked.composer.bottom]]) near(docked.sheet[key],expected,label+' sheet '+key);
    check(await sheet.getAttribute('data-docked') === '' && await page.locator('#grid').getAttribute('data-ctab') === 'convo',label+' sheet equals chead-to-composer column within 1px; left tab unchanged');
    console.log('  sheet bounds: '+JSON.stringify(docked.sheet));
    await capture(page,size,'b-docked');
    await sheet.locator('[data-entry="README.md"]').click();
    await page.locator('.file-preview-pane h1').waitFor(); await settle(page);
    const preview=await geometry(page);
    near(preview.pane.left,preview.board.left,label+' reading pane left');
    near(preview.pane.top,preview.board.top,label+' reading pane top');
    near(preview.pane.bottom,preview.board.bottom,label+' reading pane bottom');
    near(preview.pane.right,preview.viewport-preview.gutter,label+' reading pane reaches viewport gutter');
    check(preview.pane.left >= preview.sheet.right && preview.pane.right >= preview.board.right-1 && (!preview.pins.visible || preview.pane.right >= preview.pins.right-1),label+' reading pane covers board + visible pin column without overlapping sheet');
    console.log('  reading bounds: '+JSON.stringify(preview.pane)+'; separate pin column: '+preview.pins.visible);
    // Real hit-testing ensures CSS stacking keeps pane content above the scrim.
    check(await page.locator('.file-preview-head').evaluate(n=>{const r=n.getBoundingClientRect(),hit=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2);return n===hit||n.contains(hit);}),label+' reading pane receives pointer input above sheet scrim');
    await capture(page,size,'c-preview');
    await page.locator('.file-preview-pane h1').click();
    check(await sheet.isVisible() && await page.locator('.file-preview-pane').isVisible(),label+' clicking inside reading pane retains both surfaces');
    await sheet.locator('[data-entry="notes.txt"]').click();
    await page.waitForFunction(() => folderFixture.current()?.path === 'notes.txt' && document.querySelector('.file-preview-body').textContent.includes('Second file replaces'));
    check(await page.locator('.file-preview-pane h1').count() === 0 && await page.locator('.file-preview-pane').count() === 1,label+' second file replaces the preview');
    await sheet.locator('.fq').focus(); await page.keyboard.press('Escape'); await closed(page,label+' dashboard-document Escape closes both');
    await folder.click(); await sheet.locator('[data-entry="README.md"]').click(); await page.locator('.file-preview-pane h1').waitFor(); await settle(page);
    const reopened=await geometry(page),outside={x:(reopened.sheet.right+reopened.pane.left)/2,y:reopened.pane.top+100};
    check(await page.evaluate(({x,y})=>document.elementFromPoint(x,y)?.classList.contains('bc-scrim'),outside),label+' empty column gutter outside sheet and reading pane hits scrim');
    await page.mouse.click(outside.x,outside.y); await closed(page,label+' outside empty-space click closes both');
    await page.evaluate(() => folderFixture.setLayout('tabs'));
    await showBoard(page);
    await folder.click(); await sheet.locator('[data-entry="README.md"]').waitFor(); await settle(page);
    const tabs=await geometry(page);
    check(await sheet.getAttribute('data-docked') === null && tabs.sheet.top > tabs.board.top && tabs.sheet.bottom < size.height,label+' tabs retains non-docked dropdown sheet');
    observations.push({viewport:size,docked,preview,tabs,pinColumnVisible:preview.pins.visible});
    await page.keyboard.press('Escape'); await closed(page,label+' tabs Escape retains existing dismissal');
    await context.close();
  }
  assert.deepEqual(errors,[],'no browser errors or external requests'); assert.deepEqual(unexpected,[],'no unmocked HTTP routes');
  fs.writeFileSync(path.join(evidence,'browser-results.json'),JSON.stringify({observations,screenshots,errors,unexpected},null,2));
  console.log(`PASS: both light-theme viewports; ${screenshots.length} PNGs; no browser errors or unexpected requests`);
  console.log(screenshots.join('\n'));
})().catch(async e=>{
  console.error(e.stack); console.error(JSON.stringify({errors,unexpected},null,2)); process.exitCode=1;
  if(browser) for(const context of browser.contexts()) for(const page of context.pages()) {
    await page.screenshot({path:path.join(evidence,'failure.png')});
    fs.writeFileSync(path.join(evidence,'failure.html'),await page.content());
  }
}).finally(async()=>{
  if(browser) await browser.close();
  await new Promise(resolve=>server.close(resolve));
});
