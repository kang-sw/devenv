import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import readline from 'node:readline';
const out='/tmp/ws-board-topics-phase3.3pBANc';
const pkg=fs.readdirSync(path.join(os.homedir(),'.npm/_npx')).map(n=>path.join(os.homedir(),'.npm/_npx',n,'node_modules/playwright')).find(p=>fs.existsSync(path.join(p,'package.json')));
const {chromium}=createRequire(path.join(pkg,'package.json'))(pkg);
function start(bin) { return new Promise(resolve=>{const p=spawn(bin,['-test.run','^TestBoardPerfServe$','-test.timeout','0'],{env:{...process.env,WS_BOARD_PERF_SERVE:'1'},stdio:['pipe','pipe','inherit']});const wait=[];readline.createInterface({input:p.stdout}).on('line',s=>{if(s.startsWith('BOARD-PERF READY '))resolve({url:s.slice(17),log:()=>new Promise(r=>{wait.push(r);p.stdin.write('log 1\n');}),stop:()=>new Promise(r=>{p.on('exit',r);p.stdin.write('quit\n');})});else if(s.startsWith('BOARD-PERF OK'))wait.shift()?.();});});}
const init=()=>{
 window.__historyTrace=[];
 window.__historyState=(reason)=>{const h=document.querySelector('.wb-history[data-history="T04"]');if(!h)return null;const c=h.querySelector('.history-clip'),l=h.querySelector('.history-live'),cr=c.getBoundingClientRect(),lr=l.getBoundingClientRect();return {at:performance.now(),reason,base:h._wbHistoryBase,snapped:h._wbHistorySnapped,style:h.style.getPropertyValue('--wb-history-height'),clip:cr.height,live:lr.height,cut:Array.from(l.children).map(n=>{const r=n.getBoundingClientRect();return {id:n.id||n.className,top:r.top-cr.top,bottom:r.bottom-cr.top,height:r.height};}).filter(n=>n.top< -1&&n.bottom>1),frames:Array.from(h.querySelectorAll('.doc-frame')).map(f=>({key:f.getAttribute('data-doc-key'),tag:f.tagName,height:f.style.height}))};};
 const set=CSSStyleDeclaration.prototype.setProperty;
 CSSStyleDeclaration.prototype.setProperty=function(k,v,p){const h=document.querySelector('.wb-history[data-history="T04"]');let before;if(k==='--wb-history-height'&&h&&h.style===this)before=window.__historyState('before style '+v);const result=set.call(this,k,v,p);if(before)window.__historyTrace.push({before,after:window.__historyState('after style '+v),stack:new Error().stack.split('\n').slice(1,5)});return result;};
 const RO=window.ResizeObserver;
 window.ResizeObserver=class extends RO {constructor(cb){super((entries,o)=>{const watched=entries.some(e=>e.target.matches?.('.wb-history[data-history="T04"] .history-live'));if(watched)window.__historyTrace.push(window.__historyState('before history resize'));cb(entries,o);if(watched)window.__historyTrace.push(window.__historyState('after history resize'));});}};
};
const browser=await chromium.launch({headless:true});
try {for(const [name,bin] of [['baseline','/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-aQ37QY/mcp.test'],['candidate','/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-IBUQ4g/mcp.test']]){
 const server=await start(bin),context=await browser.newContext({viewport:{width:1600,height:1000},colorScheme:'light'});await context.addInitScript(init);const page=await context.newPage();await page.goto(server.url,{waitUntil:'load'});const f=page.frames().find(f=>/[?&]frame=1/.test(f.url()));await page.waitForTimeout(2500);
 for(let i=0;i<20;i++){const h=await f.evaluate(()=>document.getElementById('r-board').getAttribute('data-h'));await server.log();await f.waitForFunction(h=>document.getElementById('r-board').getAttribute('data-h')!==h,h);await page.waitForTimeout(350);}
 const samples=[];
 for(let i=0;i<=6;i++){await f.evaluate(i=>{const c=document.getElementById('bcol');c.scrollTop=i*c.clientHeight;},i);await page.waitForTimeout(1200);samples.push(await f.evaluate(i=>window.__historyState('stop '+i),i));}
 // Observe, rather than reset, a settled history. No dates, history state,
 // frames, or production functions are changed to manufacture parity.
 await page.waitForTimeout(3000);const settled=await f.evaluate(()=>window.__historyState('settled 3s'));
 await f.locator('#bcol').screenshot({path:path.join(out,name+'-diagnostic.png'),animations:'disabled',caret:'hide'});
 const trace=await f.evaluate(()=>window.__historyTrace);fs.writeFileSync(path.join(out,name+'-trace.json'),JSON.stringify({samples,settled,trace},null,2));console.log(name,JSON.stringify({settled,traceCount:trace.length}));await context.close();await server.stop();
}}finally{await browser.close();}
