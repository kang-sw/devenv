const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
function func(source, name) {
  const m = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  assert.ok(m, name); return m[0];
}
test('hide completes an in-flight docframe replacement before parking its placeholder', () => {
  const root = { children: [], removeChild(n) { this.children.splice(this.children.indexOf(n),1); n.parentNode = null; }, replaceChild(n, old) { const i=this.children.indexOf(old); this.children[i]=n; n.parentNode=this; old.parentNode=null; } };
  class Element {
    constructor(tag, text) {
      this.tagName=tag.toUpperCase(); this.style={}; this.attrs={class:'doc-frame', 'data-srcdoc':text || ''}; this.listeners={}; this.parentNode=null;
      this.classList={contains:c=>(this.attrs.class||'').split(' ').includes(c), add:c=>{if(!this.classList.contains(c))this.attrs.class+=' '+c;}, remove:c=>{this.attrs.class=(this.attrs.class||'').split(' ').filter(x=>x!==c).join(' ');}};
    }
    get isConnected() { return this.parentNode===root; }
    get attributes() { return Object.entries({...this.attrs,...(this.style.height ? {style:'height:'+this.style.height} : {})}).map(([name,value])=>({name,value})); }
    getAttribute(k) { return this.attrs[k] ?? null; }
    setAttribute(k,v) { this.attrs[k]=v; if(k==='style')this.style.height=v.split(':')[1]; }
    removeAttribute(k) { delete this.attrs[k]; }
    addEventListener(k,fn) { this.listeners[k]=fn; }
  }
  const old=new Element('iframe','OLD'), next=new Element('iframe','NEW');
  old.style.height=next.style.height='400px'; root.children=[old,next]; old.parentNode=next.parentNode=root;
  const timers=[], context={ shHidden:false, frameSizes:new Map(), frameSizeFrame:1, mountTimer:1, mountList:[old,next], mountTask:true, frameRevealMs:2000,
    window:{wbHistory:{keep(){return {apply(){}};}}}, doc:{createElement:tag=>new Element(tag)},
    q:()=>root.children.filter(n=>n.tagName==='IFRAME'), postDoc(){},
    setTimeout(fn){timers.push(fn);return timers.length;}, clearTimeout(){}, cancelAnimationFrame(){}, requestAnimationFrame(fn){return 1;}
  };
  vm.createContext(context);
  const board=readAssetSource('wsdashboard','dashboard-board.js'), frames=readAssetSource('wsdashboard','dashboard-frames.js');
  vm.runInContext(['frameNext','frameReveal'].map(n=>func(board,n)).join('')+['mountFrame','unmountFrame','suspendDocFrames'].map(n=>func(frames,n)).join(''),context);
  context.frameNext(next,old); context.shHidden=true; context.suspendDocFrames();
  assert.equal(root.children.length,1); const placeholder=root.children[0];
  assert.equal(placeholder.tagName,'DIV'); assert.equal(placeholder.getAttribute('data-srcdoc'),'NEW');
  assert.equal(placeholder.classList.contains('doc-frame-next'),false); assert.equal(placeholder.getAttribute('aria-hidden'),null);
  context.shHidden=false; const resumed=context.mountFrame(placeholder);
  assert.equal(root.children.length,1); assert.equal(resumed.tagName,'IFRAME'); assert.equal(resumed.getAttribute('srcdoc'),'NEW');
  assert.equal(resumed.classList.contains('doc-frame-next'),false); assert.equal(resumed.getAttribute('aria-hidden'),null); assert.equal(resumed.style.height,'400px');
  timers.forEach(fn=>fn()); assert.equal(root.children.length,1,'stale fallback cannot revive OLD or hide NEW');
});

test('terminal cache safety rejects in-flight list and create callbacks', () => {
  const source=readAssetSource('wsdashboard','terminal.js'), callbacks=[];
  const gate=source.match(/cacheSafe: (function \(\) \{[^\n]+\})/)[1];
  const context={ listWait:null, creating:0, worktree:'', ROWS:24, guessCols:()=>80, hooks:{post(route,body,done){callbacks.push(done);}}, show(){}, REFUSAL:{}, wsT:k=>k };
  vm.createContext(context); vm.runInContext('var safe='+gate+';'+func(source,'create'),context);
  assert.equal(context.safe(),true); context.listWait=[]; assert.equal(context.safe(),false); context.listWait=null;
  context.create({},()=>{}); assert.equal(context.safe(),false,'a pending create may introduce a runtime after hide');
  callbacks.shift()(false,'unavailable'); assert.equal(context.safe(),true);
});
