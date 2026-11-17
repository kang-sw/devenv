package wswhiteboard

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// Drive the real hydration and navigation paths with independently controlled
// name/target responses. This fixture does not share the eager-history mock.
func TestHistoryLazyTargetAfterConcurrentPrefetchUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	asset = []byte(strings.Replace(string(asset), "var kit = viewKit(window);", "var kit = window.historyKit;", 1))
	encoded, _ := json.Marshal(string(asset))
	script := `var BOARD = '', BODY_CLASS = '';` + formPrelude + `
Object.defineProperties(E.prototype, {
  id: {get:function(){return this.attrs.id || '';}},
  tagName: {get:function(){return this.tag.toUpperCase();}},
  nodeType: {get:function(){return this.tag === '#fragment' ? 11 : 1;}},
  parentElement: {get:function(){return this.parent;}},
  firstChild: {get:function(){return this.children[0] || null;}},
  nextElementSibling: {get:function(){return this.parent && this.parent.children[this.parent.children.indexOf(this)+1] || null;}},
  isConnected: {get:function(){return root.contains(this);}},
  open: {get:function(){return this.hasAttribute('open');},set:function(v){if(v)this.setAttribute('open','');else this.removeAttribute('open');}}
});
E.prototype.matches = function(s){return s.split(',').some(p => one(this,p.trim()));};
var closest = E.prototype.closest;
E.prototype.closest = function(s){return closest.call(this, s[0] === '#' ? '[id="'+s.slice(1)+'"]' : s);};
E.prototype.remove = function(){if(this.parent){this.parent.children.splice(this.parent.children.indexOf(this),1);this.parent=null;}};
E.prototype.insertBefore = function(n,b){var nodes=n.tag==='#fragment'?n.children.slice():[n];var i=b?this.children.indexOf(b):this.children.length;nodes.forEach(c=>c.remove());this.children.splice(i,0,...nodes);nodes.forEach(c=>c.parent=this);};
E.prototype.appendChild = function(n){this.insertBefore(n,null);return n;};
E.prototype.cloneNode = function(deep){var n=new E(this.tag,Object.assign({},this.attrs));if(deep)this.children.forEach(c=>n.appendChild(c.cloneNode(true)));if(this.content)n.content=this.content.cloneNode(deep);return n;};
E.prototype.getBoundingClientRect = function(){return {top:0,bottom:600,height:600};};
function templates(n){n.querySelectorAll('template').reverse().forEach(t=>{t.content=new E('#fragment');t.children.slice().forEach(c=>t.content.appendChild(c));});return n;}
document.createElement = function(tag){var n=new E(tag);if(tag==='template'){n.content=new E('#fragment');Object.defineProperty(n,'innerHTML',{set:function(html){n.content=templates(parse(html));}});}return n;};
var watches=[], jumps=[];
window.requestAnimationFrame = function(){};
window.getComputedStyle = function(){return {position:'static'};};
window.historyKit = {
  keep:function(){return {apply:function(){},scroller:root};}, pin:function(){return {apply:function(){}};},
  watch:function(n,opt){var w={n:n,opt:opt};watches.push(w);return w;},unwatch:function(){},
  jump:function(n){jumps.push(n.id);}
};
var requests=[];
window.wbHistoryFetch = function(query){return new Promise(resolve=>requests.push({query:query,resolve:resolve}));};
require('vm').runInThisContext(` + string(encoded) + `, {filename:'stream.js'});
function gap(lazy, rows){return '<div class="history-gap" '+(lazy?'data-lazy ':'')+'data-count="3" data-from="a" data-to="b"><span class="gap-label"></span><span class="gap-range"></span><button data-load="down"></button><template data-history-gap>'+rows+'</template></div>';}
function row(i){return '<div class="timeline-row" data-row="e'+i+'"><article class="wb-entry agent" data-entry="e'+i+'" id="e'+i+'"></article></div>';}
function topic(lazy,rows){return '<section class="topic" data-topic="t"><div class="wb-history" data-history="t"><div class="history-live">'+gap(lazy,rows)+row(4)+'</div></div></section>';}
async function drain(){for(var i=0;i<12;i++)await Promise.resolve();}
(async function(){
  for(var claimed of [false,true]){
    root=templates(parse(topic(true,'')));watches=[];requests=[];jumps=[];
    window.wbHistory.init(root);
    var near=watches.find(w=>w.opt.margin==='800px');
    if(!near)throw Error('near-view prefetch was not armed');
    near.opt.onChange({visible:true});
    check('lazy target initially misses',window.wbHistory.loadTarget(root,'e1'),null);
    var wait=claimed && window.wbHistory.loading('e1');
    await drain();
    check('independent name and target requests',requests.map(r=>r.query),['name=t','target=e1']);
    var full=[{name:'t',html:topic(false,row(1)+row(2)+row(3))}];
    requests[0].resolve(full);await drain();
    check('prefetch leaves old target inert',!!root.querySelector('[id="e1"]'),false);
    requests[1].resolve(full);await drain();
    if(claimed){
      check('claimed wait resolves when prefetch already hydrated',await wait,true);
      check('caller retries its landing',window.wbHistory.loadTarget(root,'e1').id,'e1');
    }
    check('target materialized despite zero new fills',!!root.querySelector('[id="e1"]'),true);
    check('exactly one navigation',jumps,['e1']);
  }
  // A nonzero fill is not proof that the requested target exists.
  root=templates(parse(topic(true,'')));requests=[];jumps=[];
  window.wbHistory.loadTarget(root,'e99');var missing=window.wbHistory.loading('e99');
  await drain();requests[0].resolve([{name:'t',html:topic(false,row(1)+row(2)+row(3))}]);
  check('unknown target is not a successful wait',await missing,null);
  check('unknown target does not navigate',jumps,[]);
  if(bad.length)throw Error(bad.join('\n'));
})().catch(e=>{console.error(e);process.exitCode=1;});
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("lazy target race: %v\n%s", err, out)
	}
}
