'use strict';
// Integrated native layout, full server-rendered assets, actual websocket admission.
// Raw reports are deliberately outside the repository. No screenshot capture.
const fs = require('node:fs');
const assert = require('node:assert/strict');
const {chromium} = require(process.env.WS_VIEWED_PERF_PLAYWRIGHT);
const origin = process.argv[2];
const report = {N:12, rationale:'12 completed serial events expose repeated per-update cost without an overlapping flood', events:[], violations:[], limits:[], setup:false};
const output = process.env.WS_VIEWED_LIVE_REPORT || '/tmp/viewed-live-report.json';
const check = (ok,message) => { if (!ok) report.violations.push(message); };
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.WS_VIEWED_PERF_CHROMIUM,args:['--disable-background-networking','--no-proxy-server']});
 try {
  const context = await browser.newContext({viewport:{width:1400,height:900}});
  await context.route('**/*',route => { const u = route.request().url(); return u.startsWith(origin+'/') || u.startsWith('data:') || u==='about:blank' ? route.continue() : route.abort(); });
  await context.addInitScript(() => {
   // Playwright's serviceWorkers:'block' probes a throwing getter in opaque
   // srcdoc frames. Block registration only in the same-origin top document.
   if (window === window.top && 'serviceWorker' in navigator) navigator.serviceWorker.register = async () => { throw Error('fixture forbids service workers'); };
   const m = window.__viewed = {rect:0,style:0,calls:{},ms:{},rafMs:0,roMs:0,wheel:0};
   const rect=Element.prototype.getBoundingClientRect; Element.prototype.getBoundingClientRect=function(){m.rect++;return rect.call(this);};
   const style=window.getComputedStyle; window.getComputedStyle=function(...a){m.style++;return style.apply(this,a);};
   const raf=window.requestAnimationFrame.bind(window); window.__frames=async n=>{while(n--)await new Promise(raf);};
   window.requestAnimationFrame=cb=>raf(t=>{const a=performance.now();try{cb(t);}finally{m.rafMs+=performance.now()-a;}});
   const RO=window.ResizeObserver;window.ResizeObserver=class extends RO{constructor(cb){super((...a)=>{const t=performance.now();try{cb(...a);}finally{m.roMs+=performance.now()-t;}});}};
   addEventListener('wheel',()=>m.wheel++,{passive:true});
   // Same-process opaque frames share this event loop, so top-level long
   // tasks are the main-thread blocks a reader feels.
   if (window === window.top) {window.__lt=[];try{new PerformanceObserver(l=>{for(const e of l.getEntries())window.__lt.push([e.startTime,e.duration]);}).observe({type:'longtask'});}catch{}}
  });
  const page = await context.newPage();
  page.on('pageerror',e=>report.violations.push('page error: '+e.message));
  const sessions = new Map();
  async function targets(){
   for(const frame of page.frames()) {try{const c=await context.newCDPSession(frame);const {targetInfo}=await c.send('Target.getTargetInfo');if(sessions.has(targetInfo.targetId)){await c.detach();continue;}await c.send('Performance.enable');sessions.set(targetInfo.targetId,c);}catch{}}
  }
  async function metrics(){await targets();const values={};for(const [id,c] of sessions){try{const {metrics}=await c.send('Performance.getMetrics');values[id]=Object.fromEntries(metrics.map(m=>[m.name,m.value]));}catch{}}return values;}
  const delta=(a,b)=>Object.keys(b).reduce((n,k)=>n+Math.max(0,(b[k].TaskDuration||0)-(a[k]?.TaskDuration||0)),0)*1000;
  async function work(){return page.evaluate(()=>structuredClone(window.__viewed));}
  async function geometry(){return page.evaluate(()=>{
   const s=document.querySelector('#bcol'),t=document.querySelector('section.topic[data-topic="B"]'),h=t?.querySelector('.wb-history'),clip=h?.querySelector('.history-clip'),live=h?.querySelector('.history-live');
   const box=n=>{if(!n)return null;const r=n.getBoundingClientRect();return {top:r.top,bottom:r.bottom,height:r.height,width:r.width};};
   const sr=s.getBoundingClientRect();return {scroll:s.scrollTop,scroller:box(s),topic:box(t),clip:box(clip),clipScroll:clip?.scrollTop,transform:live&&getComputedStyle(live).transform,historyHeight:h?.style.getPropertyValue('--wb-history-height'),visible:Array.from(t?.querySelectorAll('.wb-entry')||[]).map(n=>({id:n.id,...box(n)})).filter(r=>r.height>0&&r.bottom>Math.max(0,sr.top)&&r.top<Math.min(innerHeight,sr.bottom))};
  });}
  async function settle(){await page.evaluate(()=>window.__frames(5));await page.waitForTimeout(100);}
  await targets();const initial=await metrics(),start=Date.now();
  await page.goto(origin+'/s/viewedlive',{waitUntil:'domcontentloaded',timeout:20000});
  // Wheel at the first native opportunity, rather than waiting for restoration.
  await page.mouse.move(800,450);await page.mouse.wheel(0,160);
  await settle();
  report.cold={elapsedMs:Date.now()-start,taskMs:delta(initial,await metrics()),work:await work(),paint:await page.evaluate(()=>performance.getEntriesByType('paint').map(p=>({name:p.name,startTime:p.startTime}))),wheel:await geometry()};
  assert.equal(await page.locator('section.topic').count(),100,'setup: all heavy topics rendered');
  assert.equal(await page.locator('.wb-entry').count(),1200,'setup: all chronological entries rendered');
  assert.equal(await page.locator('.entry-panel[open]').count(),300,'setup: all open image panels rendered');
  await page.waitForFunction(()=>window.wbHistory&&document.querySelector('[data-region="board"]'));
  // Position B using native layout, without calling any product anchoring API.
  await page.evaluate(()=>{const s=document.querySelector('#bcol'),b=document.querySelector('section.topic[data-topic="B"] .history-clip');s.scrollTop+=b.getBoundingClientRect().top-s.getBoundingClientRect().top-330;});
  await settle();report.before=await geometry();
  assert(report.before.scroller.height<=900 && report.before.scroller.height>0,'setup: #bcol must be the actual bounded native scroller');
  assert(report.before.scroll>0 && report.before.visible.length>0 && report.before.clip.height>0,'setup: B must have exposed entries after actual scrolling');
  assert(await page.locator('section.topic[data-topic="A"] iframe').count()>0,'setup: production frame bridge must hydrate A');report.setup=true;
  async function update(panels=false,wheel=false){
   const before=await geometry(),a=await metrics(),wa=await work(),t0=await page.evaluate(()=>performance.now());
   const receipt=await page.evaluate(async panels=>{const r=await fetch('/update'+(panels?'?panels=1':''),{method:'POST'});if(!r.ok)throw Error(await r.text());return r.json();},panels);
   if(wheel)await page.mouse.wheel(0,180);
   await page.waitForFunction(hash=>document.querySelector('[data-region="board"]').getAttribute('data-h')===hash,receipt.hash,{timeout:15000});
   await settle();const b=await metrics(),wb=await work(),after=await geometry(),longTasks=await page.evaluate(t0=>window.__lt.filter(e=>e[0]>=t0).map(e=>Math.round(e[1])),t0);
   const row={receipt,before,after,longTasks,taskMs:delta(a,b),rect:wb.rect-wa.rect,style:wb.style-wa.style,rafMs:wb.rafMs-wa.rafMs,roMs:wb.roMs-wa.roMs,methods:Object.fromEntries(Object.keys(wb.ms).map(k=>[k,wb.ms[k]-(wa.ms[k]||0)])),wheel};
   check(after.clip.height>0&&after.visible.length>0,`seq ${receipt.seq}: blank B/history clip collapse`);
   if(!wheel) for(const v of before.visible){const next=after.visible.find(n=>n.id===v.id);check(next&&Math.abs(next.top-v.top)<=1,`seq ${receipt.seq}: exposed B entry ${v.id} moved/disappeared (${v.top} -> ${next?.top})`);}
   else check(after.scroll>before.scroll,`seq ${receipt.seq}: downward user wheel restored/locked`);
   return row;
  }
  for(let i=0;i<report.N;i++){const row=await update();report.events.push(row);check(row.receipt.event==='board-topics',`event ${i}: did not use production split topic patch`);check(row.longTasks.every(ms=>ms<50),`event ${i}: main-thread long task ${Math.max(...row.longTasks)}ms >=50ms`);}
  report.panels=await update(true,true);
  // Real delayed intrinsic IMG size delivery in opaque frames, through the
  // production postMessage bridge. Never read the child document from parent.
  // Chromium throttles rendering in offscreen opaque-origin frames, so a
  // late image only reports its size from a frame in view. Bring the newest
  // A frame into view natively and let the user-scroll cooldown expire.
  await page.evaluate(()=>{const s=document.querySelector('#bcol'),f=Array.from(document.querySelectorAll('section.topic[data-topic="A"] iframe')).pop();s.scrollTop+=f.getBoundingClientRect().top-s.getBoundingClientRect().top-120;});
  await settle();await page.waitForTimeout(300);
  const lateBefore=await geometry(),a=await metrics();
  report.lateFrames=await page.evaluate(()=>{const frames=Array.from(document.querySelectorAll('section.topic[data-topic="A"] iframe'));for(const f of frames)f.contentWindow.postMessage({viewedLate:true},'*');return frames.map(f=>({id:f.id,height:f.getBoundingClientRect().height}));});
  await page.mouse.wheel(0,-140);await settle();
  const lateAfter=await geometry();report.late={before:lateBefore,after:lateAfter,taskMs:delta(a,await metrics()),frames:await page.locator('section.topic[data-topic="A"] iframe').evaluateAll(fs=>fs.map(f=>({id:f.id,height:f.getBoundingClientRect().height})))};
  check(lateAfter.scroll<lateBefore.scroll,'late images: upward wheel restored/locked');
  check(report.late.frames.some((f,i)=>f.height>report.lateFrames[i]?.height+50),'late images: no real delayed intrinsic geometry delivered through frame bridge');
  check(lateAfter.clip.height>0,'late images: B blank/history clip collapse');
  report.worstMs=Math.max(...report.events.map(e=>e.taskMs));report.targetCount=sessions.size;
  report.limits.push('Per-update gate: no top-level long task >=50ms from the update request through settle. taskMs (CDP TaskDuration) is reported, not gated: it sums every attached frame target over the settle window, so it is not one main-thread block.','CDP TaskDuration sums unique attached targets; detached targets and targets that attach during a stimulus have incomplete start samples.','Cold wheel is sent immediately after DOMContentLoaded; pre-DOMContentLoaded synchronous work cannot receive Playwright input.','Cold metrics cover first paint, history initialization and initial capture together; wrappers report callback cost, not mutually exclusive CPU attribution.');
 }catch(e){report.error={message:e.message,stack:e.stack};}
 finally {await browser.close();fs.writeFileSync(output,JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report,null,2));}
 if(report.error||report.violations.length)process.exitCode=1;
})().catch(e=>{console.error(e);process.exitCode=1;});
