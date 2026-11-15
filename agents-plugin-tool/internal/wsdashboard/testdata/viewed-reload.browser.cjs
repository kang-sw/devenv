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
   if(self.origin!=='null')window.__reloadEntry=sessionStorage.getItem('ws.dashboard.board-reload./s/reloadtest');
   window.__sockets=[];
   window.WebSocket=class {constructor(url){this.url=url;window.__sockets.push(this);setTimeout(()=>this.onopen?.(),0);}send(){}close(){}};
   if(self.origin!=='null' && 'serviceWorker' in navigator) navigator.serviceWorker.register=async()=>{throw Error('no service workers');};
  });
  const page=await context.newPage(), errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  const key='ws.dashboard.board-reload./s/reloadtest';
  async function settle(){await page.waitForTimeout(450);}
  async function mount(){await page.goto(origin+'/s/reloadtest');await page.waitForFunction(()=>window.wbHistory && window.__sockets.length);await settle();}
  async function position(){
   for(let i=0;i<4;i++) {await page.evaluate(()=>{const n=document.getElementById('p10'),h=wbHistory.keep(n,{capture:false});h.scroller.scrollTop+=n.getBoundingClientRect().top-h.viewport().top-85;});await settle();}
   return geometry();
  }
  async function geometry(){return page.evaluate(()=>{const n=document.getElementById('p10'),h=wbHistory.keep(n,{capture:false}),s=h.scroller;return {top:n.getBoundingClientRect().top,line:h.viewport().top+1,scroll:s.scrollTop,above:document.querySelector('[data-topic="Topic00"]').getBoundingClientRect().height,entry:window.__reloadEntry,keeping:!!wbHistory.keeping(s)};});}
  async function reloadAsset(){
   await Promise.all([page.waitForEvent('domcontentloaded'),page.evaluate(()=>{const s=window.__sockets.find(s=>s.url.includes('/s/reloadtest/events'));s.onmessage({data:JSON.stringify({event:'assets',data:{id:'test-'+Date.now()}})});})]);
   await page.waitForFunction(()=>window.wbHistory && window.__sockets.length);await settle();
  }
  await mount();
  await page.evaluate(()=>wbHistory.loadTarget(document.getElementById('r-board'),'p10',window.WsCards.unfoldTopic));await settle();
  const before=await position();assert(before.scroll>2000,JSON.stringify(before));
  await reloadAsset();const after=await geometry();
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
