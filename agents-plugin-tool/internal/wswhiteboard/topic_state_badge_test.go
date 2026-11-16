package wswhiteboard

import (
	"os/exec"
	"testing"
)

// A small DOM checks authority changes and tick identity without a browser.
const stateBadgeDOM = `
const assert = require('node:assert/strict');
class E {
 constructor(cls='',tag='span') { this.className=cls;this.tag=tag;this.children=[];this.parentNode=null;this.attrs={};this._text='';this.style={};
 this.classList={contains:c=>this.className.split(' ').includes(c),toggle:(c,on)=>{this.className=this.className.split(' ').filter(x=>x!==c).concat(on?[c]:[]).join(' ');}}; }
 get textContent(){return this._text+this.children.map(n=>n.textContent).join('');}
 set textContent(v){this._text=String(v);this.children.forEach(n=>n.parentNode=null);this.children=[];}
 appendChild(n){this.children.push(n);n.parentNode=this;return n;}
 insertBefore(n,b){if(!b)return this.appendChild(n);this.children.splice(this.children.indexOf(b),0,n);n.parentNode=this;return n;}
 removeChild(n){this.children.splice(this.children.indexOf(n),1);n.parentNode=null;}
 getAttribute(k){return Object.hasOwn(this.attrs,k)?this.attrs[k]:null;}
 setAttribute(k,v){this.attrs[k]=String(v);if(k==='class')this.className=String(v);}
 hasAttribute(k){return Object.hasOwn(this.attrs,k);}
 matches(s){return s==='section.topic[data-topic]'?this.classList.contains('topic'):s[0]==='.'?this.classList.contains(s.slice(1)):s===this.tag;}
 querySelectorAll(s){return this.children.flatMap(n=>(n.matches(s)?[n]:[]).concat(n.querySelectorAll(s)));}
 querySelector(s){return this.querySelectorAll(s)[0]||null;}
}
var document=new E(),window={wsT:true,addEventListener:()=>{}},now=Date.parse('2026-11-15T00:00:00Z'),timers=[];
document.createElement=tag=>new E('',tag);
document.createElementNS=(_,tag)=>new E('',tag);
document.createTextNode=text=>{const n=new E();n.textContent=text;return n;};
document.documentElement=new E();document.addEventListener=()=>{};
Date.now=()=>now;
function setInterval(fn,ms){timers.push({fn,ms});return timers.length;}
function wsT(k,...a){return ({'js.wb.state.awaiting-confirmation':'확인 대기','js.wb.state.waiting':'대기','js.wb.state.discussing':'논의 중','js.wb.state.implementing':'구현 중','js.wb.state.researching':'연구 중','js.wb.state.estimate':'예상 {0}분','js.wb.state.eta.tip':'ETA {0}분 중 {1}분 경과','js.wb.state.over':'{0}분 초과','js.wb.state.extended':'연장 {0}회 · 최초 {1}분','js.wb.state.eta.max':'/{0}m','js.wb.state.estimate.short':'~{0}m'}[k]||k).replace(/\{(\d+)\}/g,(_,i)=>a[i]);}
const start=now;
function topic(state='implementing'){
 const n=new E('topic');n.setAttribute('data-topic','work');n.setAttribute('data-state',state);
 Object.entries({'owner':'<img src=x onerror=attack()>','eta-minutes':'30','initial-eta-minutes':'30','extensions':'0','started-at':new Date(start).toISOString(),'deadline-at':new Date(start+30*60000).toISOString(),'run':'e1'}).forEach(([k,v])=>n.setAttribute('data-state-'+k,v));
 const bar=n.appendChild(new E('tbar'));bar.appendChild(new E('tlead'));document.appendChild(n);return n;
}
var n=topic();
function pill(){return n.querySelector('.sb');}
function elapsed(){return pill().querySelector('b');}
function tick(at){now=at;timers[0].fn();}
function expected(minutes,over){assert.equal(elapsed().textContent,String(minutes));assert.equal(pill().classList.contains('over'),!!over);assert.equal(pill().getAttribute('aria-label'),'구현 중 · <img src=x onerror=attack()> · ETA 30분 중 '+minutes+'분 경과'+(over?' · '+over+'분 초과':''));}
`

func badgeNode(t *testing.T, body string) {
	t.Helper()
	data, err := embeddedAsset("ago.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node required for badge contract")
	}
	// Loading the asset twice must still leave one document-owned clock.
	out, err := nodeScript(node, stateBadgeDOM+string(data)+string(data)+body).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("badge node: %v", err)
	}
}

func TestTopicStateBadgeResearchingMetadata(t *testing.T) {
	badgeNode(t, `
tick(start+47*60000);expected(47,17);
n.setAttribute('data-state','researching');window.wbAgo(n);
assert.equal(pill().className,'sb research');
assert.equal(pill().querySelector('use').getAttribute('href'),'#i-st-research');
assert.equal(pill().textContent,'~30m');
assert.equal(n.querySelector('.who').textContent,'<img src=x onerror=attack()>');
assert.equal(n.querySelector('.who').children.length,0,'owner remains literal text');
for(const k of ['run','started-at','deadline-at'])delete n.attrs['data-state-'+k];
window.wbAgo(n);
const estimate=pill();tick(start+999999999);
assert.equal(pill(),estimate,'informational estimate is not rebuilt by ticks');
for(const [owner,minutes] of [['next owner','7'],['','7'],['owner only',''],['','']]) {
 if(owner)n.setAttribute('data-state-owner',owner);else delete n.attrs['data-state-owner'];
 if(minutes)n.setAttribute('data-state-eta-minutes',minutes);else delete n.attrs['data-state-eta-minutes'];
 window.wbAgo(n);
 assert.equal(pill().textContent,minutes?'~'+minutes+'m':'');
 assert.equal(pill().getAttribute('aria-label'),'연구 중'+(owner?' · '+owner:'')+(minutes?' · 예상 '+minutes+'분':''));
 assert.equal(pill().classList.contains('solo'),!minutes);
}
for(const bad of ['0','-1','1.5','NaN','9007199254740992']) {
 n.setAttribute('data-state-eta-minutes',bad);window.wbAgo(n);assert.equal(pill().querySelector('.t'),null);
}
n.setAttribute('data-state-owner','researcher');n.setAttribute('data-state-eta-minutes','7');window.wbAgo(n);
for(const state of ['waiting','discussing','awaiting-confirmation','implementing']) {
 n.setAttribute('data-state',state);window.wbAgo(n);
 assert(pill().classList.contains('solo'));assert.equal(n.querySelector('.who'),null);
}
for(const mode of ['closed','diagnostic','absence']) {
 n.setAttribute('data-state','researching');window.wbAgo(n);assert(n.querySelector('.topic-state'));
 if(mode==='closed')n.classList.toggle('closed',true);
 if(mode==='diagnostic')n.setAttribute('data-state-diagnostic','malformed');
 if(mode==='absence')delete n.attrs['data-state'];
 window.wbAgo(n);assert.equal(n.querySelector('.topic-state'),null);
 n.classList.toggle('closed',false);delete n.attrs['data-state-diagnostic'];
}
console.log('PASS research metadata, compact estimates, transitions and clearing');`)
}

func TestTopicStateBadgeClockAndReconciliation(t *testing.T) {
	badgeNode(t, `
expected(0,0);
const original=pill(),number=elapsed(),icon=pill().querySelector('svg');
assert.equal(pill().getAttribute('tabindex'),'0');assert.equal(pill().getAttribute('role'),'img');
assert.equal(pill().getAttribute('aria-live'),null);
for(const [delta,min,over] of [[-1,0,0],[0,0,0],[30*60000-1,29,0],[30*60000,30,0],[31*60000-1,30,0],[31*60000,31,1],[47*60000,47,17]]) {tick(start+delta);expected(min,over);assert.equal(pill(),original);assert.equal(elapsed(),number);assert.equal(pill().querySelector('svg'),icon);}
const run=n.getAttribute('data-state-run'),started=n.getAttribute('data-state-started-at');
n.setAttribute('data-state-eta-minutes','60');n.setAttribute('data-state-extensions','1');n.setAttribute('data-state-deadline-at',new Date(start+60*60000).toISOString());window.wbAgo(n);
assert.equal(pill().textContent,'47/60m');assert.equal(pill().classList.contains('over'),false);
assert(pill().getAttribute('aria-label').endsWith('연장 1회 · 최초 30분'));
assert.equal(n.querySelector('.ext').textContent,'연장 1회 · 최초 30분');
assert.equal(n.getAttribute('data-state-run'),run);assert.equal(n.getAttribute('data-state-started-at'),started);
const deps=n.querySelector('.tlead').appendChild(new E('dw'));
for(const [state,cls] of [['awaiting-confirmation','confirm'],['waiting','wait'],['discussing','discuss']]) {
 n.setAttribute('data-state',state);window.wbAgo(document);
 assert.equal(pill().className,'sb '+cls+' solo');assert.equal(pill().querySelector('use').getAttribute('href'),'#i-st-'+cls);
 assert.equal(pill().getAttribute('aria-label'),wsT('js.wb.state.'+state));
}
assert.equal(n.querySelector('.dw'),deps,'transitions preserve dependency wrapper');
n.setAttribute('data-state','implementing');window.wbAgo(n);
for(const [key,bad] of [['started-at',''],['started-at','not-a-date'],['started-at','2026-02-30T00:00:00Z'],['deadline-at',''],['deadline-at','bad'],['deadline-at',new Date(now+60000).toISOString()],['eta-minutes','NaN'],['eta-minutes','0'],['eta-minutes','1.5'],['run',''],['owner','']]) {
 const k='data-state-'+key,old=n.getAttribute(k);n.setAttribute(k,bad);window.wbAgo(n);assert.equal(elapsed(),null,'invalid '+key+' '+bad);n.setAttribute(k,old);window.wbAgo(n);assert(elapsed());
}
const removed=n;document.removeChild(n);const stale=elapsed().textContent;n=topic();tick(start+60000);expected(1,0);assert.equal(removed.querySelector('b').textContent,stale,'detached nodes are never ticked');
for(let i=0;i<10;i++)window.wbAgo(document);
assert.equal(timers.length,1);assert(timers[0].ms<=1000);
console.log('PASS badge clock, boundaries, node identity, extension, waiting class, validation and detached nodes');`)
}
