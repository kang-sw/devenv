'use strict';
const assert = require('node:assert/strict');
const {execFileSync} = require('node:child_process');
const {chromium} = require(process.env.WS_VIEWED_PERF_PLAYWRIGHT);
const origin = process.argv[2];
(async () => {
 const server = await chromium.launchServer({headless:true, executablePath:process.env.WS_VIEWED_PERF_CHROMIUM,args:['--disable-background-networking','--no-proxy-server']});
 const rootPID=server.process().pid;
 const processes=()=>execFileSync('ps',['-axo','pid=,ppid=,command='],{encoding:'utf8'}).trim().split('\n').map(row=>{const m=row.trim().match(/^(\d+)\s+(\d+)\s+(.*)$/);return {pid:+m[1],ppid:+m[2],command:m[3]};});
 let browser;
 try {
  browser = await chromium.connect(server.wsEndpoint());
  const context = await browser.newContext({viewport:{width:1400,height:900},reducedMotion:'reduce'});
  await context.route('**/*',r=>r.request().url().startsWith(origin+'/') ? r.continue() : r.abort());
  await context.addInitScript(() => {
   if(self.origin!=='null') {
    window.__reloadEntry=sessionStorage.getItem('ws.dashboard.board-reload./s/reloadtest');
    // Model a reload without a retained draft/tab preference.
    if(sessionStorage.getItem('reload-hide-board'))localStorage.removeItem('ws.dashboard.draft.reloadtest');
   }
   window.__sockets=[];
   window.WebSocket=class {constructor(url){this.url=url;window.__sockets.push(this);setTimeout(()=>this.onopen?.(),0);}send(){}close(){}};
   if(self.origin!=='null' && 'serviceWorker' in navigator) navigator.serviceWorker.register=async()=>{throw Error('no service workers');};
  });
  const page=await context.newPage(), errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  const key='ws.dashboard.board-reload./s/reloadtest';
  async function settle(){await page.waitForTimeout(450);}
  async function mount(){await page.goto(origin+'/s/reloadtest');await page.waitForFunction(()=>window.wbHistory && window.__sockets.length);await settle();}
  async function position(id='p10'){
   for(let i=0;i<4;i++) {await page.evaluate(id=>{const n=document.getElementById(id),h=wbHistory.keep(n,{capture:false});h.scroller.scrollTop+=n.getBoundingClientRect().top-h.viewport().top-85;},id);await settle();}
   return geometry(id);
  }
  async function geometry(id='p10'){return page.evaluate(id=>{const n=document.getElementById(id),h=wbHistory.keep(n||document.querySelector('[data-topic="Topic09"]'),{capture:false}),s=h.scroller;return {top:n?n.getBoundingClientRect().top:null,line:h.viewport().top+1,scroll:s.scrollTop,above:document.querySelector('[data-topic="Topic00"]').getBoundingClientRect().height,entry:window.__reloadEntry,keeping:!!wbHistory.keeping(s)};},id);}
  async function reloadAsset(){
   await Promise.all([page.waitForEvent('domcontentloaded'),page.evaluate(()=>{const s=window.__sockets.find(s=>s.url.includes('/s/reloadtest/events'));s.onmessage({data:JSON.stringify({event:'assets',data:{id:'test-'+Date.now()}})});})]);
   await page.waitForFunction(()=>window.wbHistory && window.__sockets.length);await settle();
  }
  await mount();
  await page.evaluate(()=>wbHistory.loadTarget(document.getElementById('r-board'),'p10',window.WsCards.unfoldTopic));await settle();
  const before=await position();assert(before.scroll>2000,JSON.stringify(before));
  await reloadAsset();const after=await geometry();
  assert(before.top!==null && after.top!==null,'viewed panel restored from history');
  assert(Math.abs((after.top-after.line)-(before.top-before.line))<=3,JSON.stringify({before,after}));
  assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),null,'entry consumed');
  console.log('PASS asset-triggered reload preserves viewed panel',JSON.stringify({before,after}));
  // Content-only log growth on a fully offscreen topic above the reference.
  await settle();const b=await geometry();
  await page.evaluate(async()=>{const html=await (await fetch('/board-update')).text();window.__sockets[0].onmessage({data:JSON.stringify({event:'region',data:{name:'board',hash:'content-only',html}})});});
  await settle();const a=await geometry();
  assert(a.above>b.above,'topic above must really grow: '+JSON.stringify({b,a}));
  assert(Math.abs(a.top-b.top)<=3,'content-only growth escaped owner: '+JSON.stringify({b,a}));
  console.log('PASS content-only growth above reader',JSON.stringify({b,a}));
  // A closed topic past the live-topic cap must be rematerialized by the
  // history owner, not replaced with a numeric scroll fallback.
  await mount();
  assert(await page.evaluate(()=>!!document.querySelector('template[data-history-topic="Closed00"]')),'closed topic starts inert');
  await page.evaluate(()=>wbHistory.loadTarget(document.getElementById('r-board'),'p200',window.WsCards.unfoldTopic));await settle();
  const closedBefore=await position('p200');
  await reloadAsset();const closedAfter=await geometry('p200');
  assert(closedAfter.top!==null,'reload must materialize the inert closed topic: '+JSON.stringify({closedBefore,closedAfter}));
  assert(Math.abs((closedAfter.top-closedAfter.line)-(closedBefore.top-closedBefore.line))<=3,JSON.stringify({closedBefore,closedAfter}));
  console.log('PASS reload restores inert closed-topic panel',JSON.stringify({closedBefore,closedAfter}));
  // Tabs/narrow layouts initially show Conversation after reload. Keyboard
  // activation and pointer/touch input revealing Board must retain its anchor.
  await page.evaluate(()=>sessionStorage.setItem('reload-hide-board','1'));
  for(const layout of ['tabs','narrow']) {
   await page.setViewportSize({width:layout==='narrow'?750:1400,height:900});
   await page.evaluate(layout=>localStorage.setItem('ws.dashboard.layout',layout==='tabs'?'tabs':'auto'),layout);
   await mount();await page.click('#tabbar [data-tab="board"]');
   await page.evaluate(()=>wbHistory.loadTarget(document.getElementById('r-board'),'p10',window.WsCards.unfoldTopic));await settle();
   const hiddenBefore=await position();await reloadAsset();
   assert.equal(await page.locator('#bcol').isVisible(),false,'board initially hidden: '+await page.evaluate(()=>JSON.stringify({grid:document.getElementById('grid').outerHTML.slice(0,220),layout:localStorage.getItem('ws.dashboard.layout'),entry:window.__reloadEntry})));
   const pending=await page.evaluate(k=>sessionStorage.getItem(k),key);
   assert.notEqual(pending,null,'hidden board keeps its anchor pending');
   if(layout==='tabs') {
    await reloadAsset();
    assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),pending,'a second reload carries the original pending anchor and TTL');
   }
   if(layout==='tabs') {
    await page.focus('#tabbar [data-tab="board"]');await page.keyboard.press('Space');
   } else {
    await page.dispatchEvent('#tabbar [data-tab="board"]','touchstart');
    await page.click('#tabbar [data-tab="board"]');
   }
   await settle();const hiddenAfter=await geometry();
   assert(Math.abs((hiddenAfter.top-hiddenAfter.line)-(hiddenBefore.top-hiddenBefore.line))<=3,JSON.stringify({layout,hiddenBefore,hiddenAfter}));
   assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),null,'visible board consumes anchor once');
   await page.evaluate(()=>{const s=document.getElementById('bcol');s.scrollTop=150;s.dispatchEvent(new Event('scroll'));});await settle();
   await page.click('#tabbar [data-tab="convo"]');await page.click('#tabbar [data-tab="board"]');await settle();
   assert(Math.abs((await geometry()).scroll-150)<=3,'reopening does not replay the consumed anchor');
   console.log('PASS hidden board restores once on reveal',layout,JSON.stringify({hiddenBefore,hiddenAfter}));
  }
  // Input and expiry while the board is still hidden must retire the
  // pending record before it can replay on a later reveal.
  for(const kind of ['wheel','scroll','expiry']) {
   await page.evaluate(({key,kind})=>sessionStorage.setItem(key,JSON.stringify({at:Date.now()-(kind==='expiry'?58000:0),page:location.pathname,records:[{scroller:'bcol',top:4000,key:'p10',topic:'Topic09',offset:39}]})),{key,kind});
   await page.reload();await settle();
   assert.notEqual(await page.evaluate(k=>sessionStorage.getItem(k),key),null,'anchor initially pending for '+kind);
   if(kind==='expiry')await page.waitForFunction(k=>sessionStorage.getItem(k)===null,key,{timeout:4000});
   else await page.evaluate(kind=>{
    if(kind==='scroll')document.getElementById('bcol').dispatchEvent(new Event('scroll'));
    else document.dispatchEvent(new WheelEvent('wheel',{deltaY:100,bubbles:true}));
   },kind);
   assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),null,kind+' retires hidden pending anchor');
   await page.click('#tabbar [data-tab="board"]');await settle();
   assert((await geometry()).scroll<100,kind+' must not restore after reveal');
  }
  console.log('PASS hidden pending anchor cancels on input and expires');
  await page.setViewportSize({width:1400,height:900});
  await page.evaluate(()=>{localStorage.setItem('ws.dashboard.layout','auto');sessionStorage.removeItem('reload-hide-board');});await mount();
  // Expired and wrong-page entries must never restore.
  for(const [label,patch] of [['expired',{at:Date.now()-120000}],['other-page',{page:'/s/another'}]]) {
   await page.evaluate(({key,patch})=>sessionStorage.setItem(key,JSON.stringify(Object.assign({at:Date.now(),page:location.pathname,records:[{scroller:'bcol',top:4000,key:'p10',topic:'Topic09',offset:85}]},patch))),{key,patch});
   await page.reload();await settle();assert((await geometry()).scroll<100,'ignored '+label);assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),null);
  }
  // A disappeared panel uses its topic; a disappeared topic uses scrollTop.
  for(const record of [
   {scroller:'bcol',top:2100,key:'missing-panel',topic:'Topic09',offset:85},
   {scroller:'bcol',top:2100,key:'missing-panel',topic:'missing-topic',offset:85}
  ]) {
   await page.evaluate(({key,record})=>sessionStorage.setItem(key,JSON.stringify({at:Date.now(),page:location.pathname,records:[record]})),{key,record});
   await page.reload();await settle();const fallback=await geometry();assert(fallback.scroll>500,'identity fallback restores a nonzero position: '+JSON.stringify({record,fallback}));
  }
  console.log('PASS missing-panel/topic fallback through existing owner');
  // Arrange input before the scheduled restore callback (not after landing).
  await page.addInitScript(()=>{
   if(self.origin==='null')return;
   const raf=window.requestAnimationFrame.bind(window);
   window.requestAnimationFrame=cb=>raf(t=>{if(window.wbHistory&&!window.__cancelReload){
    window.__cancelReload=true;
    if(sessionStorage.getItem('cancel-kind')==='scroll') {
     const s=document.getElementById('bcol');s.scrollTop=45;s.dispatchEvent(new Event('scroll'));
    } else document.dispatchEvent(new WheelEvent('wheel',{deltaY:100,bubbles:true}));
   }cb(t);});
  });
  for(const kind of ['wheel','scroll']) {
   await page.evaluate(({key,kind})=>{sessionStorage.setItem('cancel-kind',kind);sessionStorage.setItem(key,JSON.stringify({at:Date.now(),page:location.pathname,records:[{scroller:'bcol',top:4000,key:'p10',topic:'Topic09',offset:85}]}));},{key,kind});
   await page.reload();await settle();assert((await geometry()).scroll<100,kind+' cancels pending restore');
   assert.equal(await page.evaluate(k=>sessionStorage.getItem(k),key),null,'canceled entry consumed');
  }
  assert.deepEqual(errors,[],'page script errors');
  console.log('PASS stale/page guards and pre-restore user-input cancellation');
  await context.close();
 } finally {
  const before=processes(), ids=new Set([rootPID]);
  for(let i=0;i<8;i++)for(const p of before)if(ids.has(p.ppid))ids.add(p.pid);
  if(browser)await browser.close();await server.close();
  const remaining=processes().filter(p=>ids.has(p.pid));
  console.log('OWNED_PROCESS_SCAN',JSON.stringify({rootPID,tracked:[...ids],remaining}));
  assert.deepEqual(remaining,[],'owned browser processes must exit');
 }
})().catch(e=>{console.error(e);process.exitCode=1;});
