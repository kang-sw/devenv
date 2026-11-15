package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHistoryScriptPanelChunksAndSwapUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	// The real shared kit has separate geometry/visibility/timer tests.
	asset = []byte(strings.Replace(string(asset), "var kit = viewKit(window);", "var kit = window.historyKit;", 1))
	script := `var BOARD = '', BODY_CLASS = '';` + formPrelude + `
var originalOne=one;
one=function(e,s){ return originalOne(e,s.replace(/#([\w-]+)/g,'[id="$1"]')); };
Object.defineProperty(E.prototype,'id',{get:function(){return this.attrs.id||'';}});
Object.defineProperty(E.prototype,'tagName',{get:function(){return this.tag.toUpperCase();}});
Object.defineProperty(E.prototype,'parentElement',{get:function(){return this.parent;}});
Object.defineProperty(E.prototype,'firstChild',{get:function(){return this.children[0]||null;}});
Object.defineProperty(E.prototype,'nextElementSibling',{get:function(){return this.parent && this.parent.children[this.parent.children.indexOf(this)+1]||null;}});
Object.defineProperty(E.prototype,'open',{get:function(){return 'open' in this.attrs;},set:function(v){if(v)this.attrs.open='';else delete this.attrs.open;}});
Object.defineProperty(E.prototype,'isConnected',{get:function(){for(var p=this;p;p=p.parent)if(p===root)return true;return false;}});
E.prototype.matches=function(s){var self=this;return s.split(',').some(function(p){return one(self,p.trim());});};
E.prototype.remove=function(){this.parent.children.splice(this.parent.children.indexOf(this),1);this.parent=null;};
E.prototype.cloneNode=function(deep){var n=new E(this.tag,Object.assign({},this.attrs));n.style=Object.assign({},this.style);n.textContent=this.textContent;if(deep)this.children.forEach(function(c){n.appendChild(c.cloneNode(true));});if(this.content)n.content=this.content.cloneNode(true);return n;};
E.prototype.insertBefore=function(n,before){var self=this,list=n.tag==='#fragment'?n.children.slice():[n];list.forEach(function(c){if(c.parent)c.remove();});var i=before?this.children.indexOf(before):this.children.length;this.children.splice.apply(this.children,[i,0].concat(list));list.forEach(function(c){c.parent=self;});};
E.prototype.appendChild=function(n){this.insertBefore(n,null);return n;};
function size(n){
  if(n.tag==='template'||n.hasAttribute('data-wb-history-before'))return 0;
  if(n.matches('.wb-entry'))return +(n.attrs['data-height']||20);
  if(n.tag==='summary')return 20;
  if(n.tag==='details'&&!n.open)return 20;
  return n.children.reduce(function(sum,c){return sum+size(c);},0);
}
function offset(n,live){if(n===live)return 0;var p=n.parent,at=offset(p,live);for(var c of p.children){if(c===n)break;at+=size(c);}return at;}
Object.defineProperty(E.prototype,'scrollHeight',{get:function(){return size(this);},set:function(){}});
var navigationGuard=false,navigationOrder=[];
function navigationGeometry(node,phase){
  if(!navigationGuard)return;
  var topic=node.closest&&node.closest('.topic');
  if(!topic||topic.getAttribute('data-topic')!=='guarded')return;
  check('whole-topic reveal precedes '+phase,topic.getAttribute('data-size'),'base');navigationOrder.push(phase);
}
Object.defineProperty(E.prototype,'offsetWidth',{get:function(){navigationGeometry(this,'flash');return 0;}});
E.prototype.getBoundingClientRect=function(){
  navigationGeometry(this,'rect');
  var h=this.closest('.wb-history');if(!h)return {top:0,bottom:100,height:100};
  var live=h.querySelector('.history-live'),height=size(live);
  if(this===h||this.matches('.history-clip'))return {top:0,bottom:height,height:height};
  for(var p=this;p&&p!==live;p=p.parentElement)if(p.hasAttribute('data-wb-history-before'))return {top:0,bottom:0,height:0};
  var top=offset(this,live),s=size(this);return {top:top,bottom:top+s,height:s};
};
var keeps=[],loads=0,frames=[],timers=[],motions=[];
window.requestAnimationFrame=function(fn){frames.push(fn);};
function frame(){var batch=frames.splice(0);batch.forEach(function(fn){fn();});}
function flush(){for(var i=0;frames.length&&i<20;i++)frame();if(frames.length)throw Error('frame loop');}
window.setTimeout=function(fn){timers.push(fn);};
E.prototype.getAnimations=function(){return motions;};
document.on={};document.addEventListener=function(type,fn){(document.on[type]||(document.on[type]=[])).push(fn);};
function CustomEvent(type,init){this.type=type;Object.assign(this,init||{});}
E.prototype.dispatchEvent=function(e){if(e.type==='wbhistoryload')navigationGeometry(this,'load');e.target=this;for(var p=this;p;p=e.bubbles?p.parent:null){(p.on[e.type]||[]).slice().forEach(function(fn){fn(e);});}if(e.type==='wbhistoryload'){loads++;check('load root',e.detail.root===this,true);}return true;};
fire=function(e,type){e.dispatchEvent(new CustomEvent(type,{bubbles:true}));};
function keepDouble(n,opt,phase){navigationGeometry(n,phase);var k={node:n,opt:opt,applied:0,apply:function(){this.applied++;}};keeps.push(k);return k;}
window.historyKit={watch:function(n,opt){return {node:n,opt:opt};},unwatch:function(w){if(w)w.gone=true;},keep:function(n,opt){return keepDouble(n,opt,'keep');},jump:function(n,opt){return keepDouble(n,opt,'jump');}};
function visible(h,ratio){var w=h._wbHistoryWatch,was=w.ratio;w.ratio=ratio;w.opt.onChange({visible:ratio>0});if(was>0&&!ratio)w.opt.onOut();}
function ResizeObserver(fn){this.fn=fn;}
ResizeObserver.prototype.observe=function(){};ResizeObserver.prototype.disconnect=function(){};
function resize(h){h._wbHistoryObserver.fn();flush();}
function panel(i,height,unknown){return '<article class="wb-entry panel" id="e'+i+'" data-entry="e'+i+'" data-panel="p'+i+'" data-history-anchor="e'+i+'" data-height="'+height+'"><details class="entry-panel" id="p'+i+'" data-key="entry:e'+i+'" open><summary>panel</summary><iframe class="doc-frame"'+(unknown?'':' data-known="1"')+'></iframe></details></article>';}
function logs(from,to,height){var out='';for(var i=from;i<=to;i++)out+='<article class="wb-entry agent" id="e'+i+'" data-entry="e'+i+'" data-history-anchor="e'+i+'"'+(height?' data-height="'+height+'"':'')+'>line</article>';return out;}
function chip(from,inner,open){return '<details class="system-run" data-key="system:e'+from+'" data-history-anchor="e'+from+'"'+(open?' open':'')+'><summary><span class="lbl">logs</span></summary>'+inner+'</details>';}
function board(html,pages){
  root=parse('<section class="topic"><div class="topic-stream"><div class="wb-history" data-history="t"><div class="mhead"><button class="history-older">older</button></div><div class="history-clip"><div class="history-live">'+html+'</div></div>'+(pages||[]).map(function(p){return '<template data-history-page>'+p+'</template>';}).join('')+'</div></div></section>');
  root.querySelectorAll('template').reverse().forEach(function(t){t.content=new E('#fragment');t.children.slice().forEach(function(c){t.content.appendChild(c);});});
  // Known frames have rendered, including those a page will materialize.
  function rendered(f){f.style.height='100px';}
  root.querySelectorAll('[data-known]').forEach(rendered);
  root.querySelectorAll('template').forEach(function(t){t.content.querySelectorAll('[data-known]').forEach(rendered);});
  var h=root.querySelector('.wb-history');h.style.setProperty=function(k,v){this[k]=v;};return h;
}
function start(html,pages){var h=board(html,pages);window.wbHistory.init(document);flush();return h;}
function first(h){return h._wbHistoryChunks.at(-1);}
var bound=0;window.wbBindBoard=function(node){if(node)navigationGeometry(node,'bind');bound++;};
var h=board(panel(1,300)+logs(2,3)+panel(4,300)+logs(5,6)+panel(7,300));
` + string(asset) + `
check('three panels fit including intervening logs',first(h),'e1');
check('three whole panels natural height',h._wbHistoryHeight,980);
h.querySelector('[id="e2"]').setAttribute('data-height','140');resize(h);
check('inline timestamp wrapping never recuts finalized boundary',[first(h),h._wbHistoryHeight],['e1',1100]);
h=start(panel(1,320)+logs(2,3)+panel(4,320)+panel(5,320));
check('exactly one thousand pixels retains three',[first(h),h._wbHistoryHeight],['e1',1000]);
h=start(logs(1,2)+panel(3,400)+logs(4,5)+panel(6,400)+logs(7,8)+panel(9,400)+logs(10,11));
check('logs above the second panel fill the pixel budget',first(h),'e4');
check('row-granular cut stops before the overflowing panel',h._wbHistoryHeight,920);
check('excluded older panel has no rendered box',h.querySelector('[id="e3"]').getBoundingClientRect().height,0);
check('selected boundary log is whole',h.querySelector('[id="e4"]').getBoundingClientRect().height,20);
check('selected panel is whole',h.querySelector('[id="e6"]').getBoundingClientRect().height,400);
var oldWatch=h._wbHistoryWatch;click(h.querySelector('.history-older'));flush();
check('next chunk is the remaining panel and logs',h._wbHistoryChunks,['e4','e1']);
check('expansion re-arms and uses shared keep',[h._wbHistorySteps,oldWatch.gone,keeps.at(-1).applied],[2,true,1]);
check('exhausted control hides',h.querySelector('.history-older').hidden,true);
h=start(panel(1,600)+logs(2,3)+panel(4,600)+logs(5,6)+panel(7,600));
check('pixel budget keeps the newest panel and the logs above it',[first(h),h._wbHistoryHeight],['e5',640]);
h=start(panel(1,1200)+panel(2,1500));
check('oversized singleton always whole',[first(h),h._wbHistoryHeight],['e2',1500]);
h=start(logs(1,5)+panel(6,1400));
check('single oversized panel is a whole one-row chunk',[first(h),h._wbHistoryHeight],['e6',1400]);
h=start(panel(1,1400)+logs(2,2));
check('newest log does not force an older oversized panel into its chunk',[first(h),h._wbHistoryHeight],['e2',20]);
click(h.querySelector('.history-older'));flush();
check('older oversized panel joins whole on the next click',[first(h),h._wbHistoryHeight],['e1',1420]);
// One chunk stops before whichever budget a row would exceed first:
// thirty log lines, three panels, or 1000px of rendered height.
h=start(logs(1,40,60));
check('tall log lines cut under thirty by pixels',[first(h),h._wbHistoryHeight],['e25',960]);
h=start(logs(1,100,5));
check('short log lines cap at thirty',[first(h),h._wbHistoryHeight],['e71',150]);
h=start(panel(1,300)+logs(2,51)+panel(52,300));
check('logs between panels cap at thirty lines',[first(h),h._wbHistoryHeight],['e22',900]);
h=start(panel(1,300)+logs(2,51,30)+panel(52,300));
check('logs between panels cut by pixels',[first(h),h._wbHistoryHeight],['e29',990]);
// First frame measurement settles once; later geometry never changes
// selected whole-entry membership. Cached heights survive swaps.
h=start(panel(1,400)+panel(2,400)+panel(3,400,true));
check('unknown height is provisional',[first(h),!!h._wbHistoryPending],['e1',true]);
h.querySelector('[id="e3"]').querySelector('.doc-frame').style.height='100px';resize(h);
check('first usable height finalizes two',[first(h),h._wbHistoryPending],['e2',null]);
var hiddenFixture=panel(1,400)+panel(2,400)+panel(3,400);
var hiddenHistory=start(hiddenFixture.replace('<iframe class="doc-frame" data-known="1"></iframe></details></article>', '<iframe class="doc-frame" data-known="1"></iframe><details class="panel-versions"><summary>versions</summary><iframe class="doc-frame"></iframe></details></details></article>'));
check('unsized hidden historical frame does not block visible measurement',[hiddenHistory._wbHistoryPending,hiddenHistory._wbHistoryHeights['e1:true']],[null,400]);
click(hiddenHistory.querySelector('.history-older'));flush();check('hidden historical frame does not block expansion',hiddenHistory._wbHistorySteps,2);
var zeroHistory=start(panel(1,400)+panel(2,400)+panel(3,0));
check('zero-height panel never enters cache',[!!zeroHistory._wbHistoryPending,zeroHistory._wbHistoryHeights['e3:true']],[true,undefined]);
zeroHistory.querySelector('[id="e3"]').setAttribute('data-height','400');resize(zeroHistory);check('first nonzero layout progresses',zeroHistory._wbHistoryPending,null);
root=h.closest('.topic').parentElement;
click(h.querySelector('.history-older'));flush();
var chunks=h._wbHistoryChunks.slice(),snap=window.wbHistory.capture(document);
h.querySelector('[id="e2"]').setAttribute('data-height','1800');resize(h);
check('late measurement never recuts expanded chunks',h._wbHistoryChunks,chunks);
check('whole selected content follows growth at saved boundary',h._wbHistoryHeight,2600);
var next=board(panel(1,1500)+panel(2,1500)+panel(3,1500));window.wbHistory.restore(document,snap);flush();
check('swap restores exact chunk IDs',next._wbHistoryChunks,chunks);
check('swap restores cached measured height',next._wbHistoryHeights['e2:true'],400);
check('swap is not recut by new height',next._wbHistoryHeight,4500);
next.querySelector('[id="p2"]').open=false;
var foldSnapshot=window.wbHistory.capture(document);foldSnapshot[0].open=false;
for(var update=0;update<5;update++){
  next=board(panel(1,1500)+panel(2,1500)+panel(3,1500)+logs(4,4+update));
  window.wbHistory.restore(document,foldSnapshot);flush();
  check('frequent update '+update+' preserves manual panel fold',next.querySelector('[id="p2"]').open,false);
  check('frequent update '+update+' ignores obsolete whole-history flag',root.querySelectorAll('details.topic-stream').length,0);
  foldSnapshot=window.wbHistory.capture(document);foldSnapshot[0].open=false;
}
window.wbHistory.init(document);check('one delegated click listener',next.on.click.length,1);
// A log-only history counts entries, including rows behind a closed system chip.
h=start(chip(1,logs(1,97),false)+logs(98,100));
check('log-only chunk is thirty entries',first(h),'e71');
check('chip is split at exact boundary',h.querySelectorAll('.system-run').map(function(d){return [d.getAttribute('data-history-anchor'),d.querySelectorAll('.wb-entry').length,d.open];}),[['e1',70,false],['e71',27,false]]);
h.querySelectorAll('.system-run')[1].open=true;
var logSnapshot=window.wbHistory.capture(document);
var restoredLogs=board(chip(1,logs(1,97),false)+logs(98,100));window.wbHistory.restore(document,logSnapshot);flush();
check('swap retains split fold choices',restoredLogs.querySelectorAll('.system-run').map(function(d){return [d.getAttribute('data-history-anchor'),d.open];}),[['e1',false],['e71',true]]);
h=restoredLogs;
click(h.querySelector('.history-older'));flush();check('second log chunk is thirty',first(h),'e41');
var appendedSnapshot=window.wbHistory.capture(document), appendedHistory=board(chip(1,logs(1,98),false)+logs(99,101));
window.wbHistory.restore(document,appendedSnapshot);flush();
check('append swap retains expanded boundaries',appendedHistory._wbHistoryChunks,['e71','e41']);
check('append swap preserves actually open rows',[71,72,97].map(function(i){return appendedHistory.querySelector('[id="e'+i+'"]').closest('.system-run').open;}),[true,true,true]);
h=appendedHistory;
click(h.querySelector('.history-older'));flush();check('third log chunk is thirty',first(h),'e11');
click(h.querySelector('.history-older'));flush();check('last log chunk is remainder',[first(h),h.querySelector('.history-older').hidden],['e1',true]);
h=start(logs(51,100),[logs(1,50)]);
check('initial thirty needs no page',[first(h),loads],['e71',0]);
click(h.querySelector('.history-older'));flush();check('cross-page log chunk',[first(h),h.querySelectorAll('.wb-entry').length],['e41',100]);
// A closed chip crossing a page joins without swallowing or losing the cut.
h=start(chip(51,logs(51,97),false)+logs(98,100),[chip(1,logs(1,50),false)]);
click(h.querySelector('.history-older'));flush();
check('joined closed chip retains thirty-line boundary',first(h),'e41');
check('joined rows remain chronological',[h.querySelector('.wb-entry').id,h.querySelectorAll('.wb-entry').at(-1).id],['e1','e100']);
// Fewer than three live panels fetches only candidate pages, one per frame.
h=start(panel(101,200),[panel(51,200),panel(1,200)]);
check('candidate loading reaches three panels',[first(h),h.querySelectorAll('.wb-entry').length],['e1',3]);
check('all history fits',h.querySelector('.history-older').hidden,true);
// A republished oldest selected panel moves inside a newer card. Restore
// exact finalized IDs, but clip and page at its original chronological cut.
function repost(oldID,newID){
  var historical=panel(oldID,400).replace('class="wb-entry panel"','class="wb-entry panel historical"').replace(' id="p'+oldID+'"','').replace(' open>','>');
  return panel(newID,400).replace('data-panel="p'+newID+'"','data-panel="p'+oldID+'"').replace('id="p'+newID+'"','id="p'+oldID+'"').replace('</details></article>','<details class="panel-versions"><summary>versions</summary>'+historical+'</details></details></article>');
}
// Row heights place the pixel cuts exactly at e7 (820+190 > 1000) and e3
// (190+400+5+400 = 995, and e2 would exceed).
h=start(panel(1,400)+logs(2,2)+panel(3,400)+logs(4,4,5)+panel(5,400)+logs(6,6,190)+panel(7,400)+logs(8,8)+panel(9,400));
click(h.querySelector('.history-older'));flush();
check('republish fixture has finalized expanded boundary',h._wbHistoryChunks,['e7','e3']);
var republishSnapshot=window.wbHistory.capture(document),republishChunks=h._wbHistoryChunks.slice();
function republishedBoard(){return board(chip(2,logs(2,2)+logs(4,4),false)+panel(5,400)+logs(6,6)+panel(7,400)+logs(8,8)+panel(9,400)+repost(3,10),[panel(1,400)]);}
h=republishedBoard();window.wbHistory.restore(document,republishSnapshot);flush();
check('republish restores exact finalized membership',h._wbHistoryChunks,republishChunks);
check('republished boundary lives only in Versions',!!h.querySelector('[id="e3"]').closest('.panel-versions'),true);
check('republish selects whole chronological successor',h._wbHistoryHeight,1660);
check('republish splits the system chip at successor',h.querySelectorAll('.system-run').map(function(d){return d.getAttribute('data-history-anchor');}),['e2','e4']);
resize(h);check('republish resize never recuts restored chunks',h._wbHistoryChunks,republishChunks);
click(h.querySelector('.history-older'));flush();
check('Older advances before historical cutoff',[h._wbHistoryChunks,h._wbHistoryPending,h.querySelector('.history-older').hidden],[['e7','e3','e1'],null,true]);
h=republishedBoard();window.wbHistory.restore(document,republishSnapshot);flush();window.wbHistory.loadTarget(document,'e2');flush();
check('navigation can reveal rows before historical cutoff',first(h),'e2');keeps.at(-1).opt.onEnd();
// When the saved first entry also became historical, its successor may
// still be inert. Restore the known cut's rows before measuring the clip.
h=start(panel(3,400)+logs(4,4,5)+panel(5,400)+logs(6,6,190)+panel(7,400)+logs(8,8)+panel(9,400),[logs(1,2)]);
click(h.querySelector('.history-older'));flush();var deferredSnapshot=window.wbHistory.capture(document);
// Here the page loads to learn that e2 overflows, so capture saves e1 as
// first. A chunk that fills its counts first leaves the page inert and saves
// its boundary e3; pin that reachable shape to exercise the held successor.
check('deferred fixture cuts at the historical boundary',[deferredSnapshot[0].first,h._wbHistoryChunks,h.querySelectorAll('template[data-history-page]').length],['e1',['e7','e3'],0]);
deferredSnapshot[0].first='e3';
h=board(panel(5,400)+logs(6,6)+panel(7,400)+logs(8,8)+panel(9,400)+repost(3,10),[chip(1,logs(1,2)+logs(4,4),false)]);
window.wbHistory.restore(document,deferredSnapshot);flush();
check('historical first restores inert successor and exact cut',[h._wbHistoryChunks,h._wbHistoryHeight,h.querySelectorAll('template[data-history-page]').length],[['e7','e3'],1660,0]);
click(h.querySelector('.history-older'));flush();check('Older reaches logs before deferred historical cut',[first(h),h._wbHistoryPending,h.querySelector('.history-older').hidden],['e1',null,true]);
h=start(panel(3,400)+chip(4,logs(4,4,5),true)+panel(5,400)+logs(6,6,150)+panel(7,400)+logs(8,8,70)+panel(9,400),[logs(1,1)+logs(2,2,100)]);
click(h.querySelector('.history-older'));flush();var disclosureSnapshot=window.wbHistory.capture(document);disclosureSnapshot[0].first='e3';
h=board(panel(5,400)+logs(6,6)+panel(7,400)+logs(8,8)+panel(9,400)+repost(3,10),[chip(1,logs(1,2)+logs(4,4),false)]);
window.wbHistory.restore(document,disclosureSnapshot);flush();
check('deferred successor retains saved open disclosure',[h._wbHistoryChunks,h.querySelector('[id="e4"]').closest('.system-run').open,h._wbHistoryHeight],[['e7','e3'],true,1680]);
// Even a saved in-flight Older operation resolves that same historical cut.
var inFlight=JSON.parse(JSON.stringify(republishSnapshot));inFlight[0].chunks.push('e3');inFlight[0].pending={before:'e3'};
h=republishedBoard();window.wbHistory.restore(document,inFlight);resize(h);
check('republish restores and finishes pending Older',[h._wbHistoryChunks,h._wbHistoryPending],[['e7','e3','e1'],null]);
// A truly absent ID must cancel its unfinished chunk, not wedge all clicks.
var unresolved=JSON.parse(JSON.stringify(republishSnapshot));unresolved[0].chunks[1]='e999';
h=republishedBoard();window.wbHistory.restore(document,unresolved);flush();
click(h.querySelector('.history-older'));flush();
check('unresolvable boundary cancels pending chunk',[h._wbHistoryChunks,h._wbHistorySteps,h._wbHistoryPending],[['e7','e999'],2,null]);
click(h.querySelector('.history-older'));resize(h);
check('unresolvable retries do not accumulate chunks',[h._wbHistoryChunks,h._wbHistorySteps,h._wbHistoryPending],[['e7','e999'],2,null]);
var unresolvedFlight=JSON.parse(JSON.stringify(unresolved));unresolvedFlight[0].chunks.push('e999');unresolvedFlight[0].pending={before:'e999'};
window.wbHistory.restore(document,unresolvedFlight);resize(h);
check('unresolvable restored pending clears on resize',[h._wbHistoryChunks,h._wbHistorySteps,h._wbHistoryPending],[['e7','e999'],2,null]);
window.wbHistory.restore(document,republishSnapshot);flush();click(h.querySelector('.history-older'));flush();
check('valid restoration recovers Older after missing boundary',[first(h),h._wbHistoryPending],['e1',null]);
// No older rows remain when the very first selected panel becomes historical.
h=start(panel(1,400)+panel(2,400)+panel(3,400)+panel(4,400));click(h.querySelector('.history-older'));flush();
var exhaustedSnapshot=window.wbHistory.capture(document);
h=board(panel(2,400)+panel(3,400)+panel(4,400)+repost(1,5));window.wbHistory.restore(document,exhaustedSnapshot);flush();
check('historical first boundary remains exhausted',[h._wbHistoryChunks,h._wbHistoryHeight,h.querySelector('.history-older').hidden],[['e3','e1'],1600,true]);
// Undated item anchors are a stable legacy prefix, not recorded eN events.
// A republished e1 boundary must resolve to e2, never to item-9 by string
// length. Only explicit Older reveals that earlier prefix, in stored order.
var legacyPrefix='<article class="wb-entry item" id="item-9" data-entry="item-9" data-history-item="9"><span class="wb-hunknown">unknown</span></article><article class="wb-entry item" id="item-2" data-entry="item-2" data-history-item="2"><span class="wb-hunknown">unknown</span></article>';
h=start(legacyPrefix+panel(1,500)+panel(2,500)+panel(3,500)+panel(4,500));
click(h.querySelector('.history-older'));flush();
check('legacy prefix fixture finalizes recorded boundaries',h._wbHistoryChunks,['e3','e1']);
var legacyBoundarySnapshot=window.wbHistory.capture(document);
function legacyRepost(){return board(legacyPrefix+panel(2,500)+panel(3,500)+panel(4,500)+repost(1,5));}
h=legacyRepost();window.wbHistory.restore(document,legacyBoundarySnapshot);flush();
check('legacy prefix never becomes a later recorded successor',[h._wbHistoryChunks,h.querySelector('[id="item-9"]').getBoundingClientRect().height,h.querySelector('[id="e2"]').getBoundingClientRect().height],[['e3','e1'],0,500]);
click(h.querySelector('.history-older'));flush();
function legacyOrder(){return h.querySelectorAll('[data-history-item]').map(function(e){return [e.id,e.getBoundingClientRect().height>0,!!e.querySelector('.wb-hunknown'),!!e.querySelector('time')];});}
check('Older reveals undated prefix in stored nonnumeric order',legacyOrder(),[['item-9',true,true,false],['item-2',true,true,false]]);
var legacyVisibleSnapshot=window.wbHistory.capture(document),legacyCuts=h._wbHistoryChunks.slice();
h=legacyRepost();window.wbHistory.restore(document,legacyVisibleSnapshot);flush();
check('reload preserves exact legacy cuts and visible order',[h._wbHistoryChunks,legacyOrder()],[legacyCuts,[['item-9',true,true,false],['item-2',true,true,false]]]);
// Navigation keeps its allowlist/fold opening and uses the explicit owner.
h=start(logs(51,100),[panel(1,200)+logs(2,50)]);
check('unknown target refused',window.wbHistory.loadTarget(document,'p999'),null);
check('selector injection refused',window.wbHistory.loadTarget(document,'p1"] script'),null);
var target=window.wbHistory.loadTarget(document,'p1');flush();
check('known panel opens and becomes visible',[target.id,target.open,first(h)],['p1',true,'e1']);
check('navigation uses explicit jump offset',keeps.at(-1).opt.offset,12);
keeps.at(-1).opt.onEnd();check('keep end clears target',h._wbHistoryTarget,null);
// A host reveals through its fold owner before the loader's first layout,
// including reentrant admission hooks and inert closed-topic initialization.
function unfoldForNavigation(node){
  var topic=node.closest('.topic');
  check('callback receives a live owning topic',!!topic&&topic.isConnected,true);
  topic.setAttribute('data-size','base');navigationOrder.push('owner');
}
function guardTopic(h){
  var topic=h.closest('.topic');topic.setAttribute('data-topic','guarded');topic.setAttribute('data-size','fold');
  topic.appendChild(new E('details',{'class':'unrelated'}));
  return topic;
}
['live','page'].forEach(function(kind){
  h=start(kind==='live'?panel(1,200)+panel(2,200)+panel(3,200):panel(51,200)+panel(52,200)+panel(53,200),kind==='page'?[panel(1,200)]:[]);
  var topic=guardTopic(h),sibling=new E('section',{'class':'topic','data-topic':'other','data-size':'fold'});
  root.appendChild(sibling);
  var pages=h.querySelectorAll('template[data-history-page]').length,loadCount=loads,bindCount=bound;
  check(kind+' fixture holds its target page inert',pages,kind==='page'?1:0);
  navigationOrder=[];navigationGuard=true;
  ['p999','p1"] script','p01','#p1','item-3'].forEach(function(id){check(kind+' rejects '+id,window.wbHistory.loadTarget(document,id,unfoldForNavigation),null);});
  check(kind+' rejected IDs never reveal, load or bind',[navigationOrder,loads,bound,h.querySelectorAll('template[data-history-page]').length],[[],loadCount,bindCount,pages]);
  var target=window.wbHistory.loadTarget(document,'p1',unfoldForNavigation);flush();
  check(kind+' reveals through callback before first geometry',navigationOrder[0],'owner');
  ['rect','flash','jump'].forEach(function(phase){check(kind+' exercises '+phase,navigationOrder.includes(phase),true);});
  if(kind==='page')['bind','load'].forEach(function(phase){check('page exercises admission '+phase,navigationOrder.includes(phase),true);});
  check(kind+' opens the known target without unrelated fold changes',[target.id,target.open,topic.querySelector('.unrelated').open,sibling.getAttribute('data-size')],['p1',true,false,'fold']);
  navigationGuard=false;keeps.at(-1).opt.onEnd();
});
// The sought panel can itself be inside a page of an inert topic. Revealing
// only after materialize returns would miss init -> reset -> choose/fit here.
h=board(panel(51,200)+panel(52,200)+panel(53,200),[panel(1,200)]);
var deferredTopic=guardTopic(h),deferred=new E('template',{'data-history-topic':'guarded'});
deferred.content=new E('#fragment');deferred.content.appendChild(deferredTopic);root.appendChild(deferred);
window.wbHistory.init(document);
navigationOrder=[];navigationGuard=true;
check('unknown inert target cannot reveal or admit',window.wbHistory.loadTarget(document,'p999',unfoldForNavigation),null);
check('unknown inert topic stays inert',[navigationOrder,!!root.querySelector('template[data-history-topic]')],[[],true]);
var deferredTarget=window.wbHistory.loadTarget(document,'p1',unfoldForNavigation);flush();
check('inert topic owner runs before init geometry',navigationOrder[0],'owner');
['rect','bind','load','flash','jump'].forEach(function(phase){check('inert topic exercises '+phase,navigationOrder.includes(phase),true);});
check('inert target becomes live and opens without unrelated disclosure',[deferredTarget.isConnected,deferredTarget.open,deferredTarget.closest('.topic').querySelector('.unrelated').open,!!root.querySelector('template[data-history-topic]')],[true,true,false,false]);
navigationGuard=false;keeps.at(-1).opt.onEnd();
var revealedSnapshot=window.wbHistory.capture(document),ownerCalls=navigationOrder.filter(function(p){return p==='owner';}).length;
h=board(panel(51,200)+panel(52,200)+panel(53,200),[panel(1,200)]);guardTopic(h);
window.wbHistory.restore(document,revealedSnapshot);flush();
check('callback-free restore does not unfold the whole topic',[h.closest('.topic').getAttribute('data-size'),navigationOrder.filter(function(p){return p==='owner';}).length],['fold',ownerCalls]);
// Excluded whole rows cannot be native tab stops. A selected chip summary
// still navigates by its first-entry anchor if an overlay covers it.
h=start(chip(1,logs(1,97),false)+logs(98,100));
var folds=h.querySelectorAll('.system-run'),summary=folds[1].querySelector('summary');
check('excluded chip prefix has no rendered box',folds[0].getBoundingClientRect().height,0);
document.elementFromPoint=function(){return new E('div',{'class':'oq-pill'});};
fire(summary,'focusin');flush();
check('selected but occluded summary uses keep',[first(h),keeps.at(-1).node===summary],['e71',true]);
keeps.at(-1).opt.onEnd();
document.elementFromPoint=null;
var oldestTarget=window.wbHistory.loadTarget(document,'e1');flush();
check('chip deep link reveals excluded whole entry',[first(h),oldestTarget.id],['e1','e1']);
keeps.at(-1).opt.onEnd();
// Navigation reveals an excluded whole row, never by scrolling an inner box.
h=start(logs(1,100));window.wbHistory.loadTarget(document,'e1');flush();
check('excluded target reveals exact whole row',[first(h),keeps.at(-1).node.id],['e1','e1']);
keeps.at(-1).opt.onEnd();
var steps=h._wbHistorySteps;resize(h);check('resize never resurrects navigation',[h._wbHistorySteps,h._wbHistoryTarget],[steps,null]);
// Collapse only after a visible->out transition and preserves loaded rows.
visible(h,0);check('first zero does not collapse',h._wbHistorySteps,2);
visible(h,1);visible(h,0);flush();check('off-screen collapse returns newest thirty',[first(h),h._wbHistorySteps,h.querySelectorAll('.wb-entry').length],['e71',1,100]);
click(h.querySelector('.history-older'));flush();
var input=new E('textarea');h.closest('.topic').appendChild(input);document.activeElement=input;
visible(h,1);visible(h,0);check('editing defers collapse',h._wbHistorySteps,2);
document.activeElement=null;document.on.focusout[0]();timers.splice(0).forEach(function(fn){fn();});flush();check('focusout retries collapse',h._wbHistorySteps,1);
click(h.querySelector('.history-older'));flush();h.closest('.topic').setAttribute('data-max','');visible(h,1);visible(h,0);check('maximized topic does not collapse',h._wbHistorySteps,2);
var before=bound;window.wbBindBoard(root);check('host hook retained',bound,before+1);
if(bad.length)throw Error(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("history script: %v\n%s", err, out)
	}
}

func TestHistoryScriptWiredOnlyIntoFullPagesAndSharedAssets(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PageAssets()
	if err != nil || !strings.Contains(string(shared.FormScript), a.stream) {
		t.Fatalf("shared history script missing: %v", err)
	}
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		p := newPageData(BuildView(streamFixture(), ViewInput{Layout: layout}), a, false)
		if got := strings.Contains(string(p.Script), a.stream); got != (layout == LayoutFull) {
			t.Errorf("layout %d history wiring = %v", layout, got)
		}
	}
	if strings.Contains(string(a.css), "--wb-history-height") || strings.Contains(string(a.css), ".history-more .history-clip::after") || !strings.Contains(string(a.css), ".history-live > [data-wb-history-before] { display: none") {
		t.Fatal("history must select whole entries without a pixel clip or fade")
	}
	if strings.Contains(string(a.css), ".topic-stream > summary") || !strings.Contains(string(a.css), "grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr)") {
		t.Fatal("history must stay open with a geometrically centered header control")
	}
	if strings.Contains(string(a.css), "grid-template-columns: minmax(0, 1fr) 320px") {
		t.Fatal("full page still reserves a side rail")
	}
}
