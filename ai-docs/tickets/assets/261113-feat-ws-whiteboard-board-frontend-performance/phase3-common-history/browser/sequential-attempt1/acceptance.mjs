import fs from 'node:fs';
import path from 'node:path';
import {spawn} from 'node:child_process';
import readline from 'node:readline';
import {createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';
const ROOT='/tmp/ws-board-topics-common-history.EWq6tq';
const OUT=path.join(ROOT,'browser');fs.mkdirSync(OUT,{recursive:true});
const manifest=JSON.parse(fs.readFileSync(path.join(ROOT,'manifest.json')));
const original=fs.readFileSync(path.join(ROOT,'exports/candidate/scripts/board-perf.mjs'),'utf8');
const library=original.slice(0,original.indexOf('\nconst args = parseArgs(process.argv.slice(2));'))+'\nexport {loadPlaywright,coldLoad,metrics,probe,scenario,endMetrics,sample,push,table,compare};\n';
fs.writeFileSync(path.join(ROOT,'harness-library.mjs'),library);
const H=await import(pathToFileURL(path.join(ROOT,'harness-library.mjs')));
const {chromium}=H.loadPlaywright();
const sha=b=>createHash('sha256').update(b).digest('hex');
const env={...process.env};for(const k of Object.keys(env))if(k.startsWith('WS_'))delete env[k];
Object.assign(env,{WS_CACHE_HOME:path.join(ROOT,'browser-state/cache'),WS_CONFIG_HOME:path.join(ROOT,'browser-state/config')});
const servers=new Set();
async function start(side){
 const bin=path.join(ROOT,'bin',side+'.test');
 if(sha(fs.readFileSync(bin))!==manifest.sources[side].binary_sha256)throw Error('binary identity mismatch '+side);
 return new Promise((resolve,reject)=>{
  const p=spawn(bin,['-test.run','^TestBoardPerfServe$','-test.timeout','0'],{cwd:manifest.sources[side].root,env:{...env,WS_BOARD_PERF_SERVE:'1'},stdio:['pipe','pipe','pipe']});
  servers.add(p);let ready=false;const waiters=[];const log=fs.createWriteStream(path.join(OUT,`${side}-servers.log`),{flags:'a'});
  p.stderr.on('data',b=>log.write(b));readline.createInterface({input:p.stdout}).on('line',s=>{log.write(s+'\n');if(s.startsWith('BOARD-PERF READY ')){ready=true;resolve({url:s.slice(17),pid:p.pid,log:()=>new Promise(r=>{waiters.push(r);p.stdin.write('log 1\n');}),stop:()=>new Promise(r=>{p.once('exit',r);p.stdin.write('quit\n');})});}else if(s.startsWith('BOARD-PERF OK'))waiters.shift()?.();});
  p.on('exit',code=>{servers.delete(p);log.end();if(!ready)reject(Error('serve exited before ready '+side+' '+code));});
 });
}
function observe(){
 window.__accept={heights:[],errors:[]};
 window.addEventListener('error',e=>{if(String(e.message).includes('ResizeObserver'))window.__accept.errors.push(e.message);});
 window.addEventListener('message',e=>{const m=e.data;if(m&&m.wbDoc==='height')window.__accept.heights.push({at:performance.now(),key:m.docKey,height:m.height});});
}
function state(){
 const c=document.getElementById('bcol'),r=c.getBoundingClientRect();
 const rect=n=>{const b=n.getBoundingClientRect();return {top:b.top-r.top,left:b.left-r.left,width:b.width,height:b.height};};
 return {scrollTop:c.scrollTop,viewport:c.clientHeight,total:c.scrollHeight,
  topics:Array.from(c.querySelectorAll('section.topic')).map(t=>({key:t.dataset.topic,...rect(t),near:!!t.wbNear,
   histories:Array.from(t.querySelectorAll('.wb-history')).map(h=>{const live=h.querySelector('.history-live'),clip=h.querySelector('.history-clip'),id=h._wbHistoryChunks?.at(-1),entry=id&&h.querySelector('[id="'+id+'"]'),lr=live.getBoundingClientRect(),cr=clip.getBoundingClientRect(),er=entry&&entry.getBoundingClientRect();return {key:h.dataset.history,chunks:h._wbHistoryChunks,pending:!!h._wbHistoryPending,queued:!!h._wbHistoryFrame,style:h.style.getPropertyValue('--wb-history-height'),clip:cr.height,live:lr.height,edge:er?er.top-cr.top:null,bottomGap:lr.bottom-cr.bottom,steps:h._wbHistorySteps};}),
   frames:Array.from(t.querySelectorAll('.doc-frame')).map(f=>({key:f.dataset.docKey,tag:f.tagName,style:f.style.height,...rect(f)}))})),
  animations:document.getAnimations().filter(a=>a.playState==='running').length,errors:window.__accept?.errors||[]};
}
function stableKey(s){return JSON.stringify({...s,errors:[]});}
async function settle(page,frame){let last='',since=Date.now();const start=Date.now();while(Date.now()-start<15000){const s=await frame.evaluate(state),key=stableKey(s);if(key!==last){last=key;since=Date.now();}const ready=!s.animations&&!s.topics.some(t=>t.near&&t.histories.some(h=>h.pending||h.queued))&&!s.topics.some(t=>t.frames.some(f=>f.tag==='IFRAME'&&f.width>0&&!parseFloat(f.style)));if(ready&&Date.now()-since>=750)return s;await page.waitForTimeout(100);}throw Error('geometry did not settle: '+JSON.stringify(await frame.evaluate(state)));}
function compareState(a,b){const diffs=[];function visit(x,y,p){if(typeof x==='number'&&typeof y==='number'){if(Math.abs(x-y)>1)diffs.push({path:p,baseline:x,candidate:y});return;}if(x===null||y===null||typeof x!=='object'||typeof y!=='object'){if(x!==y)diffs.push({path:p,baseline:x,candidate:y});return;}if(Array.isArray(x)!==Array.isArray(y)){diffs.push({path:p,baseline:x,candidate:y});return;}const ks=new Set([...Object.keys(x),...Object.keys(y)]);for(const k of ks)visit(x[k],y[k],p+'.'+k);}visit(a,b,'state');return diffs;}
async function open(browser,side,{routes=false}={}){
 const server=await start(side),context=await browser.newContext({viewport:{width:1600,height:1000},colorScheme:'light'});await context.addInitScript(observe);const page=await context.newPage(),events=[];
 page.on('websocket',ws=>ws.on('framereceived',e=>{try{const v=JSON.parse(e.payload);if(v.event==='board-topics'||v.event==='region'&&v.data.name==='board')events.push({event:v.event,topics:v.data.topics?.map(t=>t.key),bytes:Buffer.byteLength(e.payload)});}catch{}}));
 let source;
 page.on('response',async response=>{if(/[?&]frame=1/.test(response.url())&&response.request().resourceType()==='document'){try{source=await response.text();}catch{}}});
 return {server,context,page,events,getSource:()=>source,close:async()=>{await context.close();await server.stop();}};
}
async function look(browser){
 const pairs={};const results={sources:{},stops:[],status:'running'};fs.writeFileSync(path.join(OUT,'look.json'),JSON.stringify(results,null,2));
 try{
  for(const side of ['baseline','candidate']){
   const p=pairs[side]=await open(browser,side);await p.page.goto(p.server.url,{waitUntil:'load'});p.frame=p.page.frames().find(f=>/[?&]frame=1/.test(f.url()));await p.page.waitForTimeout(2500);
   const html=p.getSource()||await(await fetch(p.server.url+'?frame=1')).text();const stream=fs.readFileSync(path.join(manifest.sources[side].root,'agents-plugin-tool/internal/wswhiteboard/assets/stream.js'),'utf8');const dashboard=fs.readFileSync(path.join(manifest.sources[side].root,'agents-plugin-tool/internal/wsdashboard/assets/dashboard.js'),'utf8');if(!html.includes(stream)||!html.includes(dashboard))throw Error('served bytes differ '+side);
   fs.writeFileSync(path.join(OUT,side+'-served.html'),html);results.sources[side]={revision:manifest.sources[side].revision,binary:manifest.sources[side].binary_sha256,port:new URL(p.server.url).port,pid:p.server.pid,servedHTML:sha(html),stream:sha(stream),dashboard:sha(dashboard),streamExact:true,dashboardExact:true};
   await p.frame.evaluate(()=>{window.__unchanged=document.querySelector('section.topic[data-topic="T01"]');});
  }
  for(let i=0;i<20;i++){for(const side of ['baseline','candidate']){const p=pairs[side],h=await p.frame.evaluate(()=>document.getElementById('r-board').getAttribute('data-h'));await p.server.log();await p.frame.waitForFunction(h=>document.getElementById('r-board').getAttribute('data-h')!==h,h);await p.page.waitForTimeout(350);}}
  for(let i=0;;i++){
   const counts=await Promise.all(Object.values(pairs).map(p=>p.frame.evaluate(()=>{const c=document.getElementById('bcol');return Math.ceil(c.scrollHeight/c.clientHeight);})));if(counts[0]!==counts[1])throw Error('scroll stop counts differ '+counts);if(i>=counts[0])break;
   for(const p of Object.values(pairs)){await p.frame.evaluate(i=>{const c=document.getElementById('bcol');c.scrollTop=i*c.clientHeight;},i);await p.page.waitForTimeout(1200);}
   const a=await settle(pairs.baseline.page,pairs.baseline.frame),b=await settle(pairs.candidate.page,pairs.candidate.frame),diffs=compareState(a,b);
   const entry={stop:i,baseline:a,candidate:b,diffs};results.stops.push(entry);fs.writeFileSync(path.join(OUT,'look.json'),JSON.stringify(results,null,2));
   for(const side of ['baseline','candidate']){const dir=path.join(OUT,side+'-look');fs.mkdirSync(dir,{recursive:true});await pairs[side].frame.locator('#bcol').screenshot({path:path.join(dir,`light-${String(i).padStart(2,'0')}.png`),animations:'disabled',caret:'hide'});}
   console.log('LOOK stop',i,'geometry diffs',diffs.length);if(diffs.length)throw Error('matched-state gate failed at stop '+i+': '+JSON.stringify(diffs.slice(0,10)));
  }
  results.status='geometry matched';for(const [side,p] of Object.entries(pairs)){results[side]={events:p.events,unchangedIdentity:await p.frame.evaluate(()=>window.__unchanged===document.querySelector('section.topic[data-topic="T01"]')),trace:await p.frame.evaluate(()=>window.__accept)};}
 }catch(e){results.status='failed';results.failure=String(e);throw e;}finally{for(const p of Object.values(pairs))await p.close();fs.writeFileSync(path.join(OUT,'look.json'),JSON.stringify(results,null,2));}
}
async function perf(browser){
 const results={};for(const side of ['baseline','candidate'])results[side]={label:side,tree:manifest.sources[side].root,revision:manifest.sources[side].revision,binary:manifest.sources[side].binary_sha256,shape:'8,6,4,2,60',rates:[4,6],runs:3,idle:60,gc:true,alternated:true,version:browser.version(),samples:{}};
 for(const rate of [4,6])for(let r=0;r<3;r++)for(const side of (r%2?['candidate','baseline']:['baseline','candidate'])){const server=await start(side);let context;try{const p=await H.coldLoad(browser,server.url,rate,{gc:true});context=p.context;const m0=await H.metrics(p.cdp),p0=await H.probe(p.page);await H.scenario('c',p.page,p.cdp,server,{});H.push(results[side],'c',rate,H.sample(m0,await H.endMetrics(p.cdp,{gc:true}),p0,await H.probe(p.page)));console.log('PERF',side,rate,r+1);}finally{if(context)await context.close();await server.stop();}}
 for(const [side,r] of Object.entries(results)){fs.writeFileSync(path.join(OUT,side+'-perf.json'),JSON.stringify(r,null,2));console.log(H.table(r));}
 H.compare({_:['compare',path.join(OUT,'baseline-perf.json'),path.join(OUT,'candidate-perf.json')],targets:'c:ScriptDuration'});
}
async function sequential(browser){
 const all={status:'running',sides:{}};
 try{for(const side of ['baseline','candidate']){
  const p=await open(browser,side),held=new Map(),released=new Set(),routeTrace=[],keys=['entry:e723','entry:e725','entry:e727'];
  const r=all.sides[side]={routeTrace,states:[]};
  try{
   await p.context.route('**/wb-frame-*.js',async route=>{
    let key=null;try{key=await(await route.request().frame().frameElement()).getAttribute('data-doc-key');}catch{}
    if(keys.includes(key)&&!released.has(key)){
     routeTrace.push({action:'held',key,url:route.request().url(),at:Date.now()});
     await new Promise(resolve=>held.set(key,resolve));
    }
    await route.continue();
   });
   await p.page.goto(p.server.url,{waitUntil:'load'});p.frame=p.page.frames().find(f=>/[?&]frame=1/.test(f.url()));await p.page.waitForTimeout(2500);
   for(let i=0;i<20&&held.size<3;i++){await p.frame.evaluate(i=>{const c=document.getElementById('bcol');c.scrollTop=i*c.clientHeight;},i);await p.page.waitForTimeout(400);}
   if(held.size!==3)throw Error('could not observe exactly three T04 script requests '+JSON.stringify([...held.keys()]));
   const save=async label=>{const s=await p.frame.evaluate(state),h=s.topics.find(t=>t.key==='T04');r.states.push({label,topic:h});return h;};
   await save('three actual frame documents awaiting unchanged script responses');
   function release(key){released.add(key);routeTrace.push({action:'released',key,at:Date.now()});held.get(key)();}
   release(keys[0]);release(keys[1]);
   await p.frame.waitForFunction(()=>['entry:e723','entry:e725'].every(k=>document.querySelector('.doc-frame[data-doc-key="'+k+'"]').style.height==='219px'),null,{timeout:10000});
   await p.frame.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
   const two=await save('first two native 219px heights applied; third response still held');
   if(!two.histories[0].pending)throw Error('boundary finalized before third unknown height');
   release(keys[2]);await settle(p.page,p.frame);const final=await save('third native 219px height applied; boundary finalized');
   if(final.histories[0].pending||final.histories[0].edge< -1||Math.abs(final.histories[0].bottomGap)>1)throw Error('final boundary does not align with complete entry');
   const chunks=final.histories[0].chunks.slice(),cache=await p.frame.evaluate(()=>({...document.querySelector('.wb-history[data-history="T04"]')._wbHistoryHeights}));
   const first=chunks.at(-1);
   // Native disclosure interaction causes another real frame to render late.
   // No body, style height, date, boundary or source bytes are overwritten.
   await p.frame.locator('#'+first+' .panel-versions > summary').first().click();
   await p.frame.waitForFunction(id=>{const h=document.querySelector('.wb-history[data-history="T04"]');return Array.from(h.querySelector('[id="'+id+'"] .panel-versions').querySelectorAll('iframe.doc-frame')).some(f=>parseFloat(f.style.height)>0);},first,{timeout:10000});
   await settle(p.page,p.frame);const late=await save('native version disclosure produced late height; finalized boundary retained');
   const lateCache=await p.frame.evaluate(()=>({...document.querySelector('.wb-history[data-history="T04"]')._wbHistoryHeights}));
   if(JSON.stringify(late.histories[0].chunks)!==JSON.stringify(chunks)||JSON.stringify(lateCache)!==JSON.stringify(cache)||late.histories[0].clip<=final.histories[0].clip||late.histories[0].edge< -1||Math.abs(late.histories[0].bottomGap)>1)throw Error('late native growth recut boundary/cache or broke alignment');
   r.trace=await p.frame.evaluate(()=>window.__accept);r.cache=cache;r.lateCache=lateCache;
   for(const key of keys)if(!r.trace.heights.some(m=>m.key===key&&m.height===219))throw Error('missing captured native 219px height '+key);
   if(r.trace.errors.length)throw Error('observer errors '+r.trace.errors);
   console.log('SEQUENTIAL',side,JSON.stringify({keys,initial:final.histories[0],late:late.histories[0],heightMessages:r.trace.heights.filter(m=>keys.includes(m.key))}));
  }finally{for(const [key,resolve] of held)if(!released.has(key))resolve();await p.close();fs.writeFileSync(path.join(OUT,'sequential.json'),JSON.stringify(all,null,2));}
 }
 const a=all.sides.baseline.states,b=all.sides.candidate.states;
 for(let i=0;i<a.length;i++){const diffs=compareState(a[i].topic,b[i].topic);if(diffs.length)throw Error('sequential matched-geometry gate differs '+JSON.stringify(diffs));}
 all.status='matched native two-then-one 219px heights and late growth';
 }catch(e){all.status='failed';all.failure=String(e);throw e;}finally{fs.writeFileSync(path.join(OUT,'sequential.json'),JSON.stringify(all,null,2));}
}
const command=process.argv[2];const browser=await chromium.launch({headless:true});try{if(command==='look')await look(browser);else if(command==='perf')await perf(browser);else if(command==='sequential')await sequential(browser);else throw Error('unknown command '+command);}finally{await browser.close();for(const p of servers){p.stdin.end();}}
