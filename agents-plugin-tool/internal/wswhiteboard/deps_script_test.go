package wswhiteboard

import (
	"os/exec"
	"testing"
)

func TestDependencyPopoverDelegatedInteractions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("deps.js")
	if err != nil {
		t.Fatal(err)
	}
	script := `
const assert = require('node:assert/strict');
let now=0, next=0, timers=new Map();
function setTimeout(fn,ms){const id=++next;timers.set(id,{fn,at:now+ms});return id;}
function clearTimeout(id){timers.delete(id);}
function tick(ms){now+=ms;for(const [id,t] of [...timers])if(t.at<=now){timers.delete(id);t.fn();}}
class E {
 constructor(cls,parent=null){this.cls=cls;this.parent=parent;this.children=[];if(parent)parent.children.push(this);this.attrs={};this.style={setProperty(k,v){this[k]=v;}};this.classes=new Set();this.classList={toggle:(k,on)=>on?this.classes.add(k):this.classes.delete(k)};this.hidden=cls==='pop';this.isConnected=true;this.rect={left:100,top:100,bottom:122,width:22,height:22};}
 contains(n){for(;n;n=n.parent)if(n===this)return true;return false;}
 closest(s){for(let n=this;n;n=n.parent){if(s==='.dw'&&n.cls==='dw')return n;if(s==='.dw .dbtn'&&n.cls==='dbtn'&&n.parent.cls==='dw')return n;if(s==='a'&&n.cls==='a')return n;}return null;}
 querySelector(s){return this.children.find(n=>'.'+n.cls===s)||null;}
 setAttribute(k,v){this.attrs[k]=v;}
 getBoundingClientRect(){this.measured=(this.measured||0)+1;return this.rect;}
 focus(){document.activeElement=this;fire('focusin',this);}
}
const listeners={}, winListeners={};
const captures={};
var document={activeElement:null,addEventListener(type,fn,capture){(listeners[type]||=[]).push(fn);captures[type]=!!capture;}};
var window={innerWidth:1024,innerHeight:768,addEventListener(type,fn){(winListeners[type]||=[]).push(fn);}};
function fire(type,target,extra={}){const e={target,preventDefault(){this.prevented=true;},stopPropagation(){this.stopped=true;},...extra};for(const fn of listeners[type]||[])fn(e);return e;}
function fixture(){const w=new E('dw'),b=new E('dbtn',w),p=new E('pop',w),a=new E('a',p);p.rect={width:318,height:220};return {w,b,p,a};}
function state(f,open){assert.equal(f.p.hidden,!open);assert.equal(f.b.attrs['aria-expanded']||'false',String(open));if(f.p.showPopover)assert.equal(f.p.nativeOpen,open);}
function nativeFixture(){
 const f=fixture();f.p.nativeOpen=false;f.p.shown=0;f.p.hiddenCalls=0;
 f.p.matches=s=>{assert.equal(s,':popover-open');if(f.p.unsupportedPseudo)throw new SyntaxError('unsupported pseudo-class');return f.p.nativeOpen;};
 f.p.showPopover=()=>{assert(!f.p.hidden);assert(!f.p.nativeOpen);f.p.nativeOpen=true;f.p.shown++;};
 f.p.hidePopover=()=>{f.p.nativeOpen=false;f.p.hiddenCalls++;};
 return f;
}
const f=fixture(), outside=new E('outside');
` + string(asset) + `
fire('mouseover',f.b);tick(119);state(f,false);tick(1);state(f,true);
assert.equal(f.p.style.left,'94px');assert.equal(f.p.style.top,'130px');
fire('mouseout',f.b,{relatedTarget:outside});tick(100);
fire('mouseover',f.p,{relatedTarget:outside});tick(100);state(f,true);
fire('mouseout',f.p,{relatedTarget:outside});tick(160);state(f,false);
fire('mouseover',f.b);fire('mouseout',f.b,{relatedTarget:outside});tick(200);state(f,false);
f.b.focus();state(f,true);
let e=fire('click',f.b);assert(e.prevented&&e.stopped);state(f,true);
document.activeElement=null;fire('mouseout',f.w,{relatedTarget:outside});tick(200);state(f,true);
fire('click',outside);state(f,false);
fire('click',f.b);state(f,true);fire('click',f.b);state(f,false);
f.b.focus();f.a.focus();e=fire('keydown',f.a,{key:'Escape'});assert(e.prevented&&e.stopped);state(f,false);assert.equal(document.activeElement,f.b);
fire('focusout',f.b,{relatedTarget:outside});document.activeElement=outside;tick(200);state(f,false);
f.b.focus();document.activeElement=f.a;fire('focusout',f.b,{relatedTarget:f.a});tick(200);state(f,true);
document.activeElement=outside;fire('focusout',f.a,{relatedTarget:outside});tick(160);state(f,false);
f.b.rect={left:990,top:690,bottom:712,width:22,height:22};fire('click',f.b);
assert.equal(f.p.style.left,'698px');assert.equal(f.p.style.top,'462px');assert(f.p.classes.has('above'));assert.equal(f.p.style['--wb-deps-caret'],'299px');
fire('scroll',f.p);state(f,true);fire('scroll',outside);state(f,false);
f.b.rect={left:-20,top:0,bottom:22,width:22,height:22};f.p.rect.height=900;fire('click',f.b);
assert.equal(f.p.style.left,'8px');assert.equal(f.p.style.top,'8px');
winListeners.resize[0]();state(f,false);
// A pinned topic can be replaced while open; its stale pin cannot block hover.
fire('click',f.b);state(f,true);f.w.isConnected=false;
const inserted=fixture();fire('mouseover',inserted.b);tick(120);state(inserted,true);state(f,false);
fire('click',inserted.a);state(inserted,false);
const detached=fixture();fire('mouseover',detached.b);detached.w.isConnected=false;tick(120);state(detached,false);
// Re-rendered nodes use the same delegated listeners. Reload is idempotent.
` + string(asset) + `
for(const list of Object.values(listeners))assert.equal(list.length,1);
assert.equal(winListeners.resize.length,1);
fire('click',inserted.b);state(inserted,true);fire('keydown',inserted.b,{key:'Escape'});state(inserted,false);
assert(f.b.measured>=5,'position is measured again on every open');
const native=nativeFixture();
native.b.focus();native.a.focus();assert.equal(native.p.shown,1,'already open top-layer popover is not shown twice');
fire('keydown',native.a,{key:'Escape'});assert.equal(native.p.hiddenCalls,1);state(native,false);
// Reparenting closes native visibility independently of hidden. Reopen even
// before the asynchronous toggle event has arrived, including when pinned.
fire('click',native.b);state(native,true);
native.p.nativeOpen=false;assert.equal(native.p.hidden,false);
fire('click',native.b);state(native,true);assert.equal(native.p.shown,3);
native.p.nativeOpen=false;
fire('mouseover',native.b,{relatedTarget:outside});tick(120);state(native,true);assert.equal(native.p.shown,4);
// Native toggle does not bubble: its capture listener clears ARIA and the pin.
assert.equal(captures.toggle,true);
fire('toggle',outside,{newState:'closed'});state(native,true);
fire('toggle',native.p,{newState:'open'});state(native,true);
native.p.nativeOpen=false;fire('toggle',native.p,{newState:'closed'});state(native,false);
const afterNativeClose=fixture();fire('mouseover',afterNativeClose.b);tick(120);state(afterNativeClose,true);
fire('click',outside);state(afterNativeClose,false);
fire('mouseover',native.b);tick(120);state(native,true);assert.equal(native.p.shown,5);
// A delayed close event from a previous open must not close a reopened popup.
fire('toggle',native.p,{newState:'closed'});state(native,true);
native.p.nativeOpen=false;fire('toggle',native.p,{newState:'closed'});state(native,false);
fire('click',native.b);state(native,true);assert.equal(native.p.shown,6);
fire('click',outside);state(native,false);
const legacy=nativeFixture();legacy.p.unsupportedPseudo=true;
legacy.b.focus();legacy.a.focus();assert.equal(legacy.p.shown,1);
legacy.p.nativeOpen=false;fire('toggle',legacy.p,{newState:'closed'});state(legacy,false);
fire('mouseover',legacy.b);tick(120);state(legacy,true);assert.equal(legacy.p.shown,2);
fire('click',outside);state(legacy,false);
console.log('PASS dependency hover grace, pin, outside click, focus, Escape, placement, scroll, resize, inserts, singleton listeners and native visibility reconciliation');
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("dependency node: %v\n%s", err, out)
	}
	t.Log(string(out))
}
