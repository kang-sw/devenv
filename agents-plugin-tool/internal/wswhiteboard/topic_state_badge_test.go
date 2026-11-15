package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// A deliberately small DOM: the timer must discover current document nodes on
// every tick, never retain a topic list or create a timer for an adopted root.
const stateBadgeDOM = `
const assert = require('node:assert/strict');
class E {
 constructor(cls) { this.className=cls||''; this.children=[]; this.parentNode=null; this.attrs={}; this._text='';
 this.classList={contains:c=>this.className.split(' ').includes(c),toggle:(c,on)=>{this.className=this.className.split(' ').filter(x=>x!==c).concat(on?[c]:[]).join(' ');}}; }
 get textContent(){return this._text+this.children.map(n=>n.textContent).join('');}
 set textContent(v){this._text=String(v);this.children.forEach(n=>n.parentNode=null);this.children=[];}
 appendChild(n){this.children.push(n);n.parentNode=this;return n;}
 insertBefore(n,b){if(!b)return this.appendChild(n);this.children.splice(this.children.indexOf(b),0,n);n.parentNode=this;return n;}
 removeChild(n){this.children.splice(this.children.indexOf(n),1);n.parentNode=null;}
 getAttribute(k){return Object.hasOwn(this.attrs,k)?this.attrs[k]:null;}
 setAttribute(k,v){this.attrs[k]=String(v);}
 hasAttribute(k){return Object.hasOwn(this.attrs,k);}
 matches(s){return s==='section.topic[data-topic]'?this.classList.contains('topic'):s[0]==='.'&&this.classList.contains(s.slice(1));}
 querySelectorAll(s){return this.children.flatMap(n=>(n.matches(s)?[n]:[]).concat(n.querySelectorAll(s)));}
 querySelector(s){return this.querySelectorAll(s)[0]||null;}
}
var document=new E(),window={wsT:true,addEventListener:()=>{}},now=Date.parse('2026-11-15T00:00:00Z'),timers=[];
document.createElement=()=>new E();
Date.now=()=>now;
function setInterval(fn,ms){timers.push({fn,ms});return timers.length;}
function wsT(k,...a){return ({'js.wb.state.awaiting-confirmation':'확인 대기','js.wb.state.waiting':'대기','js.wb.state.discussing':'논의 중','js.wb.state.implementing':'구현 중','js.wb.state.eta':'ETA {0}분 · {1}분 경과'}[k]||k).replace(/\{(\d+)\}/g,(_,i)=>a[i]);}
const start=now;
function topic(state='implementing'){
 const n=new E('topic');n.setAttribute('data-topic','work');n.setAttribute('data-state',state);
 Object.entries({'owner':'<img src=x onerror=attack()>','eta-minutes':'30','started-at':new Date(start).toISOString(),'deadline-at':new Date(start+30*60000).toISOString(),'run':'e1'}).forEach(([k,v])=>n.setAttribute('data-state-'+k,v));
 const bar=n.appendChild(new E('tbar'));bar.appendChild(new E('tlead'));document.appendChild(n);return n;
}
var n=topic();
function eta(){return n.querySelector('.eta');}
function tick(at){now=at;timers[0].fn();}
function expected(minutes,over){assert.equal(eta().textContent,'ETA 30분 · '+minutes+'분 경과'+(over?' (+'+over+')':''));assert.equal(eta().classList.contains('wb-state-over'),!!over);}
`

func TestTopicStateBadgeClockAndReconciliation(t *testing.T) {
	data, err := embeddedAsset("ago.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node required for badge clock contract")
	}
	script := stateBadgeDOM + `
const deps=n.appendChild(new E('topic-deps'));deps.appendChild(new E('dep')).textContent='topic:blocker';
` + string(data) + `
expected(0,0);
assert.equal(n.querySelector('.own').textContent,'<img src=x onerror=attack()>');
assert.equal(n.querySelector('.own').children.length,0,'owner is text, not markup');
for(const [delta,min,over] of [[-1,0,0],[0,0,0],[30*60000-1,29,0],[30*60000,30,0],[31*60000-1,30,0],[31*60000,31,1],[47*60000,47,17]]) {tick(start+delta);expected(min,over);}
const run=n.getAttribute('data-state-run'),started=n.getAttribute('data-state-started-at');
n.setAttribute('data-state-eta-minutes','60');n.setAttribute('data-state-deadline-at',new Date(start+60*60000).toISOString());window.wbAgo(n);
assert.equal(eta().textContent,'ETA 60분 · 47분 경과');assert.equal(eta().classList.contains('wb-state-over'),false);
assert.equal(n.getAttribute('data-state-run'),run);assert.equal(n.getAttribute('data-state-started-at'),started);
for(const state of ['awaiting-confirmation','waiting','discussing']) {n.setAttribute('data-state',state);window.wbAgo(document);assert.equal(n.querySelector('.stb').textContent,wsT('js.wb.state.'+state));assert.equal(eta(),null);assert.equal(n.querySelector('.own'),null);}
assert.equal(n.querySelector('.topic-deps'),deps,'badge transitions preserve dependency row');
assert.equal(deps.textContent,'topic:blocker');
n.setAttribute('data-state','implementing');n.setAttribute('data-state-run','e2');n.setAttribute('data-state-started-at',new Date(now).toISOString());n.setAttribute('data-state-deadline-at',new Date(now+60*60000).toISOString());window.wbAgo(n);assert.equal(eta().textContent,'ETA 60분 · 0분 경과');
for(const [key,bad] of [['started-at',''],['started-at','not-a-date'],['started-at','2026-02-30T00:00:00Z'],['deadline-at',''],['deadline-at','bad'],['deadline-at',new Date(now+60000).toISOString()],['eta-minutes','NaN'],['eta-minutes','0'],['eta-minutes','1.5'],['run',''],['owner','']]) {const k='data-state-'+key,old=n.getAttribute(k);n.setAttribute(k,bad);window.wbAgo(n);assert.equal(eta(),null,'invalid '+key+' '+bad);n.setAttribute(k,old);window.wbAgo(n);assert(eta());}
const removed=n;document.removeChild(n);const stale=removed.querySelector('.eta').textContent;n=topic();tick(start+60000);expected(1,0);assert.equal(removed.querySelector('.eta').textContent,stale,'detached node is never ticked');
n.classList.toggle('closed',true);window.wbAgo(n);assert.equal(n.querySelector('.topic-state'),null);
n.classList.toggle('closed',false);n.setAttribute('data-state-diagnostic','malformed');window.wbAgo(n);assert.equal(n.querySelector('.topic-state'),null);
delete n.attrs['data-state-diagnostic'];delete n.attrs['data-state'];window.wbAgo(n);assert.equal(n.querySelector('.topic-state'),null,'legacy absence is not inferred');
` + string(data) + `
for(let i=0;i<10;i++)window.wbAgo(document);
assert.equal(timers.length,1,'script reload, refresh and adoption share the document timer');
assert(timers[0].ms<=1000,'elapsed advances promptly at run-relative minute boundaries');
console.log('PASS state badge clock, boundaries, metadata, transitions, detached nodes and singleton timer');`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("badge node: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS state badge") {
		t.Fatalf("missing receipt: %s", out)
	}
	t.Log(string(out))
}
