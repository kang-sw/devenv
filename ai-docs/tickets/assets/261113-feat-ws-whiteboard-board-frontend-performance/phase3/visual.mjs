import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import readline from 'node:readline';
const out='/tmp/ws-board-topics-phase3.3pBANc';
const pkg=fs.readdirSync(path.join(os.homedir(),'.npm/_npx')).map(n=>path.join(os.homedir(),'.npm/_npx',n,'node_modules/playwright')).find(p=>fs.existsSync(path.join(p,'package.json')));
const {chromium}=createRequire(path.join(pkg,'package.json'))(pkg);
function start(bin) { return new Promise(resolve=>{ const p=spawn(bin,['-test.run','^TestBoardPerfServe$','-test.timeout','0'],{env:{...process.env,WS_BOARD_PERF_SERVE:'1'},stdio:['pipe','pipe','inherit']});const rl=readline.createInterface({input:p.stdout});const wait=[];rl.on('line',s=>{if(s.startsWith('BOARD-PERF READY '))resolve({url:s.slice(17),log:()=>new Promise(r=>{wait.push(r);p.stdin.write('log 1\n');}),stop:()=>new Promise(r=>{p.on('exit',r);p.stdin.write('quit\n');})});else if(s.startsWith('BOARD-PERF OK'))wait.shift()?.();});});}
const browser=await chromium.launch({headless:true});
const results={};
try { for(const [name,bin] of [['before','/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-aQ37QY/mcp.test'],['after','/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-IBUQ4g/mcp.test']]) {
 const server=await start(bin), context=await browser.newContext({viewport:{width:1600,height:1000},colorScheme:'light'}), page=await context.newPage(), events=[];
 page.on('websocket',ws=>ws.on('framereceived',e=>{try { const v=JSON.parse(e.payload);if(v.event==='board-topics'||v.event==='region'&&v.data.name==='board')events.push({event:v.event,topics:v.data.topics?.map(t=>t.key),bytes:Buffer.byteLength(e.payload)}); }catch{}}));
 await page.goto(server.url,{waitUntil:'load'});const frame=()=>page.frames().find(f=>/[?&]frame=1/.test(f.url()));await page.waitForTimeout(2500);
 await frame().evaluate(()=>{window.__unchanged=document.querySelector('section.topic[data-topic="T01"]');});
 for(let i=0;i<20;i++){const h=await frame().evaluate(()=>document.getElementById('r-board').getAttribute('data-h'));await server.log();await frame().waitForFunction(h=>document.getElementById('r-board').getAttribute('data-h')!==h,h);await page.waitForTimeout(350);}
 const identity=await frame().evaluate(()=>window.__unchanged===document.querySelector('section.topic[data-topic="T01"]'));
 const dir=path.join(out,name+'-look');fs.mkdirSync(dir,{recursive:true});const layout=[];
 for(let i=0;await frame().evaluate(i=>{const c=document.getElementById('bcol');return i<Math.ceil(c.scrollHeight/c.clientHeight);},i);i++){
  await frame().evaluate(i=>{const c=document.getElementById('bcol');c.scrollTop=i*c.clientHeight;},i);await page.waitForTimeout(1200);
  await frame().locator('#bcol').screenshot({path:path.join(dir,`light-${String(i).padStart(2,'0')}.png`),animations:'disabled',caret:'hide'});
  layout.push(await frame().evaluate(()=>{const c=document.getElementById('bcol'),r=c.getBoundingClientRect();return Array.from(c.querySelectorAll('section.topic')).map(t=>{const b=t.getBoundingClientRect();return {key:t.getAttribute('data-topic'),top:b.top-r.top,height:b.height};});}));
 }
 results[name]={events,identity,layout};await context.close();await server.stop();
 }
 fs.writeFileSync(path.join(out,'visual.json'),JSON.stringify(results,null,2));console.log(JSON.stringify(Object.fromEntries(Object.entries(results).map(([k,v])=>[k,{events:v.events,unchangedIdentity:v.identity,captures:v.layout.length}]))));
} finally {await browser.close();}
