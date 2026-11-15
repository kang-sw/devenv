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
   const m = window.__viewed = {rect:0,style:0,calls:{},ms:{},rafMs:0,roMs:0,wheel:0,historyAdmission:[]};
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
   const s=document.querySelector('#bcol'),t=document.querySelector('section.topic[data-topic="B"]'),h=t?.querySelector('.wb-history'),live=h?.querySelector('.history-live');
   const box=n=>{if(!n)return null;const r=n.getBoundingClientRect();return {top:r.top,bottom:r.bottom,height:r.height,width:r.width};};
   const sr=s.getBoundingClientRect();return {scroll:s.scrollTop,scroller:box(s),topic:box(t),history:box(h),live:box(live),visible:Array.from(t?.querySelectorAll('.wb-entry')||[]).map(n=>({id:n.id,...box(n)})).filter(r=>r.height>0&&r.bottom>Math.max(0,sr.top)&&r.top<Math.min(innerHeight,sr.bottom))};
  });}
  async function settle(){
   await page.evaluate(()=>window.__frames(5));await page.waitForTimeout(100);
   // 19f8e848e intentionally adds a 3px/300ms held-topic nudge (AI Context:
   // restart it asynchronously). Compare reading geometry after that motion;
   // retain the <=1px anchor gate and include the animation in long-task timing.
   await page.evaluate(async()=>{
    await Promise.all(Array.from(document.querySelectorAll('.topic.board-hold-nudge')).flatMap(n=>n.getAnimations()).map(a=>a.finished.catch(()=>{})));
    await window.__frames(2);
   });
  }
  await targets();const initial=await metrics(),start=Date.now();
  await page.goto(origin+'/s/viewedlive',{waitUntil:'domcontentloaded',timeout:20000});
  // Wheel at the first native opportunity, rather than waiting for restoration.
  await page.mouse.move(800,450);await page.mouse.wheel(0,160);
  await settle();
  report.cold={elapsedMs:Date.now()-start,taskMs:delta(initial,await metrics()),work:await work(),paint:await page.evaluate(()=>performance.getEntriesByType('paint').map(p=>({name:p.name,startTime:p.startTime}))),wheel:await geometry()};
  assert.equal(await page.locator('section.topic').count(),100,'setup: all heavy topics rendered');
  // 15e44e235 (AI Context: inert gaps, latest-line/batch anchors and whole-item
  // 1000px windows) intentionally retired the all-1200-live/history-clip model.
  // timelineView keeps e12 of each legacy topic live; stream.js fill/progress
  // admits its preceding suffix until 1000px, pausing on an unmeasured frame.
  // Derive exact membership from the production admission-time measurements,
  // not a replacement fixed count. Late frame shrink/growth cannot recut a
  // settled window, so measuring only its final geometry would be incorrect.
  async function timeline(initial=false){return page.evaluate(initial=>{
   const topics=Array.from(document.querySelectorAll('section.topic')).map(t=>{
   const h=t.querySelector('.wb-history'),live=h.querySelector('.history-live');
   const entry=n=>({id:n.id,panel:n.getAttribute('data-panel'),open:!!n.querySelector('.entry-panel[open]')});
   const parts=Array.from(live.children).map(n=>{
    const gap=n.matches('.history-gap'),root=gap?n.querySelector('template[data-history-gap]').content:n;
    return {gap,count:gap?+n.dataset.count:1,entries:Array.from(root.querySelectorAll('.wb-entry')).map(entry)};
   });
   return {name:t.dataset.topic,parts};
   });
   if(!initial)return topics;
   const m=window.__viewed,windows=m.historyAdmission;m.historyAdmission=null;
   return {topics,windows};
  },initial);}
  function auditTimeline(topics,updates=0,panels=false){
   let live=0,held=0,open=0,panelCount=0;
   for(const t of topics){
    const i=t.name==='A'?0:t.name==='B'?1:Number(t.name.slice(6));
    const expected=Array.from({length:12},(_,j)=>'e'+(i*12+j+1));
    if(t.name==='B')for(let j=1;j<=updates;j++)expected.push('e'+(1200+j));
    // applyPanel appends the panel and its system receipt for each command.
    if(t.name==='A'&&panels)expected.push('e1213','e1214','e1215','e1216');
    assert.deepEqual(t.parts.flatMap(p=>p.entries.map(e=>e.id)),expected,`${t.name}: chronological identity/order across live rows and inert gaps`);
    const expectedPanels=Array.from({length:3},(_,j)=>({id:'e'+(i*12+j*4+1),panel:'p'+(i*3+j+1)}));
    if(t.name==='A'&&panels)expectedPanels.push({id:'e1213',panel:'p301'},{id:'e1215',panel:'p302'});
    assert.deepEqual(t.parts.flatMap(p=>p.entries).filter(e=>e.panel).map(({id,panel})=>({id,panel})),expectedPanels,`${t.name}: panel identities reach their chronological entries`);
    for(const p of t.parts){
     assert.equal(p.count,p.entries.length,`${t.name}: gap count matches retained entries`);
     if(p.gap)held+=p.count;else live+=p.count;
     panelCount+=p.entries.filter(e=>e.panel).length;
     open+=p.entries.filter(e=>e.panel&&e.open).length;
    }
   }
   assert.equal(live+held,1200+updates+(panels?4:0),'all entries accounted for, including inert gaps');
   assert.equal(panelCount,300+(panels?2:0),'all image panels retained, including inert gaps');
   // New panels may retain older live disclosures through snapshot restore;
   // only the untouched initial fixture has exactly three open per topic.
   if(!panels)assert.equal(open,300,'all initial open panels retained, including inert gaps');
   return {live,held,open,panelCount};
  }
  const {topics:initialTimeline,windows}=await timeline(true);
  report.membership=auditTimeline(initialTimeline);report.windows=windows;
  assert(report.membership.held>0,'setup: older history must remain inert, not mount all 1200 entries');
  for(const t of initialTimeline){
   const latest=t.parts.at(-1).entries.at(-1).id,expected=new Set([latest]);
   assert(!t.parts.at(-1).gap,`${t.name}: latest legacy entry cannot be inert`);
   assert(t.parts.filter(p=>p.gap).length<=1&&(!t.parts.some(p=>p.gap)||t.parts[0].gap),`${t.name}: initial admission is a chronological suffix`);
   for(const w of report.windows.filter(w=>w.topic===t.name)){
    let height=w.base;
    for(const [i,r] of w.rows.entries()){
     assert(height<1000,`${t.name}: no extra row after crossing the 1000px window`);
     expected.add(r.id);
     if(r.height===null){
      assert.equal(i,w.rows.length-1,`${t.name}: unmeasured frame pauses admission`);
      assert.equal(w.waiting,r.id,`${t.name}: pending whole row is retained`);
      assert(!w.done,`${t.name}: unmeasured window cannot be settled`);
     }else height+=r.height;
    }
    assert.equal(w.height,height,`${t.name}: window uses measured whole-row heights`);
    assert(!w.done||height>=1000||w.remaining===0,`${t.name}: settle only at 1000px or exhausted gap`);
   }
   const chronological=Array.from(expected).sort((a,b)=>Number(a.slice(1))-Number(b.slice(1)));
   assert.deepEqual(t.parts.filter(p=>!p.gap).flatMap(p=>p.entries.map(e=>e.id)),chronological,`${t.name}: exact live membership derived from anchor plus 1000px admissions`);
  }
  await page.waitForFunction(()=>window.wbHistory&&document.querySelector('[data-region="board"]'));
  // Position B using native layout, without calling any product anchoring API.
  await page.evaluate(()=>{const s=document.querySelector('#bcol'),b=document.querySelector('section.topic[data-topic="B"] .history-live');s.scrollTop+=b.getBoundingClientRect().top-s.getBoundingClientRect().top-330;});
  await settle();report.before=await geometry();
  assert(report.before.scroller.height<=900 && report.before.scroller.height>0,'setup: #bcol must be the actual bounded native scroller');
  assert(report.before.scroll>0 && report.before.visible.length>0 && report.before.live.height>0,'setup: B must have exposed entries after actual scrolling');
  assert(await page.locator('section.topic[data-topic="A"] iframe').count()>0,'setup: production frame bridge must hydrate A');report.setup=true;
  let renderedUpdates=0,renderedPanels=false;
  async function auditRendered(updates,panels){
   const held=await page.evaluate(()=>({B:document.querySelector('section.topic[data-topic="B"]').hasAttribute('data-board-held'),A:document.querySelector('section.topic[data-topic="A"]').hasAttribute('data-board-held'),pending:window.wbPill?.pending}));
   if(held.B||held.A)assert(held.pending>0,'held rendering advertises pending updates');
   if(!held.B)renderedUpdates=updates;
   if(!held.A)renderedPanels=panels;
   auditTimeline(await timeline(),renderedUpdates,renderedPanels);
   if(renderedUpdates){
    const newest=page.locator(`section.topic[data-topic="B"] .wb-entry#e${1200+renderedUpdates}`);
    assert.equal(await newest.count(),1,'latest admitted B entry is live, not inert');
    assert.match(await newest.innerText(),new RegExp(`Completed live reasoning event ${1199+renderedUpdates}:`),'B entry has the delivered text');
    assert(await newest.evaluate(n=>n.classList.contains('current')),'latest admitted B entry owns current state');
   }
   // 15e44e235 pauses the window at an unmeasured panel: p301 may still
   // be inert behind p302. auditTimeline verifies both identities either way.
   if(renderedPanels)assert.equal(await page.locator('section.topic[data-topic="A"] #e1215 #p302[open]').count(),1,'latest delivered A panel is live and open');
   return held;
  }
  async function update(panels=false,wheel=false){
   const before=await geometry(),a=await metrics(),wa=await work(),t0=await page.evaluate(()=>performance.now());
   const receipt=await page.evaluate(async panels=>{const r=await fetch('/update'+(panels?'?panels=1':''),{method:'POST'});if(!r.ok)throw Error(await r.text());return r.json();},panels);
   if(wheel)await page.mouse.wheel(0,180);
   await page.waitForFunction(hash=>document.querySelector('[data-region="board"]').getAttribute('data-h')===hash,receipt.hash,{timeout:15000});
   await settle();const b=await metrics(),wb=await work(),after=await geometry(),longTasks=await page.evaluate(t0=>window.__lt.filter(e=>e[0]>=t0).map(e=>Math.round(e[1])),t0);
   const row={receipt,before,after,longTasks,taskMs:delta(a,b),rect:wb.rect-wa.rect,style:wb.style-wa.style,rafMs:wb.rafMs-wa.rafMs,roMs:wb.roMs-wa.roMs,methods:Object.fromEntries(Object.keys(wb.ms).map(k=>[k,wb.ms[k]-(wa.ms[k]||0)])),wheel};
   check(after.live.height>0&&after.visible.length>0,`seq ${receipt.seq}: blank B/history window collapse`);
   const updates=report.events.length+(panels?0:1);
   // Viewed rendering may intentionally stay frozen until scroll/release;
   // the stream hash is a receipt, not proof that held topic bytes are live.
   row.held=await auditRendered(updates,panels);
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
  check(lateAfter.live.height>0,'late images: B blank/history window collapse');
  // A real owner click releases any still-viewed updates. Check the complete
  // delivered identities/content after release, not just receipt hashes.
  if(await page.evaluate(()=>window.wbPill.pending>0))await page.locator('.oq-wrap.wb-oq-top .oq-updates').click();
  await page.waitForFunction(()=>document.querySelector('section.topic[data-topic="B"] #e1212')&&document.querySelector('section.topic[data-topic="A"] #p302'));
  await settle();
  report.finalMembership=auditTimeline(await timeline(),report.N,true);
  await auditRendered(report.N,true);
  report.worstMs=Math.max(...report.events.map(e=>e.taskMs));report.targetCount=sessions.size;
  report.limits.push('Per-update gate: no top-level long task >=50ms from the update request through settle. taskMs (CDP TaskDuration) is reported, not gated: it sums every attached frame target over the settle window, so it is not one main-thread block.','CDP TaskDuration sums unique attached targets; detached targets and targets that attach during a stimulus have incomplete start samples.','Cold wheel is sent immediately after DOMContentLoaded; pre-DOMContentLoaded synchronous work cannot receive Playwright input.','Cold metrics cover first paint, history initialization and initial capture together; wrappers report callback cost, not mutually exclusive CPU attribution.');
 }catch(e){report.error={message:e.message,stack:e.stack};}
 finally {await browser.close();fs.writeFileSync(output,JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report,null,2));}
 if(report.error||report.violations.length)process.exitCode=1;
})().catch(e=>{console.error(e);process.exitCode=1;});
