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
E.prototype.getBoundingClientRect=function(){
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
E.prototype.dispatchEvent=function(e){e.target=this;for(var p=this;p;p=e.bubbles?p.parent:null){(p.on[e.type]||[]).slice().forEach(function(fn){fn(e);});}if(e.type==='wbhistoryload'){loads++;check('load root',e.detail.root===this,true);}return true;};
fire=function(e,type){e.dispatchEvent(new CustomEvent(type,{bubbles:true}));};
window.historyKit={watch:function(n,opt){return {node:n,opt:opt};},unwatch:function(w){if(w)w.gone=true;},keep:function(n,opt){var k={node:n,opt:opt,applied:0,apply:function(){this.applied++;}};keeps.push(k);return k;}};
function visible(h,ratio){var w=h._wbHistoryWatch,was=w.ratio;w.ratio=ratio;w.opt.onChange({visible:ratio>0});if(was>0&&!ratio)w.opt.onOut();}
function ResizeObserver(fn){this.fn=fn;}
ResizeObserver.prototype.observe=function(){};ResizeObserver.prototype.disconnect=function(){};
function resize(h){h._wbHistoryObserver.fn();flush();}
function panel(i,height,unknown){return '<article class="wb-entry panel" id="e'+i+'" data-entry="e'+i+'" data-panel="p'+i+'" data-history-anchor="e'+i+'" data-height="'+height+'"><details class="entry-panel" id="p'+i+'" data-key="entry:e'+i+'" open><summary>panel</summary><iframe class="doc-frame"'+(unknown?'':' data-known="1"')+'></iframe></details></article>';}
function logs(from,to){var out='';for(var i=from;i<=to;i++)out+='<article class="wb-entry agent" id="e'+i+'" data-entry="e'+i+'" data-history-anchor="e'+i+'">line</article>';return out;}
function run(from,inner,open){return '<details class="run-fold" data-key="run:e'+from+'" data-history-anchor="e'+from+'"'+(open?' open':'')+'><summary><span class="lbl">logs</span></summary>'+inner+'</details>';}
function board(html,pages){
  root=parse('<section class="topic"><div class="topic-stream"><div class="wb-history" data-history="t"><header class="wb-history-head"><button class="history-older">older</button></header><div class="history-clip"><div class="history-live">'+html+'</div></div>'+(pages||[]).map(function(p){return '<template data-history-page>'+p+'</template>';}).join('')+'</div></div></section>');
  root.querySelectorAll('template').reverse().forEach(function(t){t.content=new E('#fragment');t.children.slice().forEach(function(c){t.content.appendChild(c);});});
  root.querySelectorAll('[data-known]').forEach(function(f){f.style.height='100px';});
  var h=root.querySelector('.wb-history');h.style.setProperty=function(k,v){this[k]=v;};return h;
}
function start(html,pages){var h=board(html,pages);window.wbHistory.init(document);flush();return h;}
function first(h){return h._wbHistoryChunks.at(-1);}
var bound=0;window.wbBindBoard=function(){bound++;};
var h=board(panel(1,300)+logs(2,3)+panel(4,300)+logs(5,6)+panel(7,300));
` + string(asset) + `
check('three panels fit including intervening logs',first(h),'e1');
check('three whole panels natural height',h._wbHistoryHeight,980);
h.querySelector('[id="e2"]').setAttribute('data-height','140');resize(h);
check('inline timestamp wrapping never recuts finalized boundary',[first(h),h._wbHistoryHeight],['e1',1100]);
h=start(panel(1,320)+logs(2,3)+panel(4,320)+panel(5,320));
check('exactly one thousand pixels retains three',[first(h),h._wbHistoryHeight],['e1',1000]);
h=start(logs(1,2)+panel(3,400)+logs(4,5)+panel(6,400)+logs(7,8)+panel(9,400)+logs(10,11));
check('three shrinks to two',first(h),'e6');
check('boundary immediately above whole panel not older logs',h._wbHistoryHeight,880);
check('excluded older panel has no rendered box',h.querySelector('[id="e3"]').getBoundingClientRect().height,0);
check('selected boundary panel is whole',h.querySelector('[id="e6"]').getBoundingClientRect().height,400);
var oldWatch=h._wbHistoryWatch;click(h.querySelector('.history-older'));flush();
check('next chunk includes prior older logs and panel',h._wbHistoryChunks,['e6','e3']);
check('expansion re-arms and uses shared keep',[h._wbHistorySteps,oldWatch.gone,keeps.at(-1).applied],[2,true,1]);
check('logs before oldest panel are still older',h.querySelector('.history-older').hidden,false);
click(h.querySelector('.history-older'));flush();
check('remaining log-only prefix is next chunk',first(h),'e1');
check('exhausted control hides',h.querySelector('.history-older').hidden,true);
h=start(panel(1,600)+logs(2,3)+panel(4,600)+logs(5,6)+panel(7,600));
check('two shrinks to one',first(h),'e7');
h=start(panel(1,1200)+panel(2,1500));
check('oversized singleton always whole',[first(h),h._wbHistoryHeight],['e2',1500]);
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
// A log-only history counts entries, including rows behind a closed run.
h=start(run(1,logs(1,97),false)+logs(98,100));
check('log-only chunk is thirty entries',first(h),'e71');
check('run is split at exact boundary',h.querySelectorAll('.run-fold').map(function(d){return [d.getAttribute('data-history-anchor'),d.querySelectorAll('.wb-entry').length,d.open];}),[['e1',70,false],['e71',27,false]]);
h.querySelectorAll('.run-fold')[1].open=true;
var logSnapshot=window.wbHistory.capture(document);
var restoredLogs=board(run(1,logs(1,97),false)+logs(98,100));window.wbHistory.restore(document,logSnapshot);flush();
check('swap retains split fold choices',restoredLogs.querySelectorAll('.run-fold').map(function(d){return [d.getAttribute('data-history-anchor'),d.open];}),[['e1',false],['e71',true]]);
h=restoredLogs;
click(h.querySelector('.history-older'));flush();check('second log chunk is thirty',first(h),'e41');
var appendedSnapshot=window.wbHistory.capture(document), appendedHistory=board(run(1,logs(1,98),false)+logs(99,101));
window.wbHistory.restore(document,appendedSnapshot);flush();
check('append swap retains expanded boundaries',appendedHistory._wbHistoryChunks,['e71','e41']);
check('append swap preserves actually open rows',[71,72,97].map(function(i){return appendedHistory.querySelector('[id="e'+i+'"]').closest('.run-fold').open;}),[true,true,true]);
h=appendedHistory;
click(h.querySelector('.history-older'));flush();check('third log chunk is thirty',first(h),'e11');
click(h.querySelector('.history-older'));flush();check('last log chunk is remainder',[first(h),h.querySelector('.history-older').hidden],['e1',true]);
h=start(logs(51,100),[logs(1,50)]);
check('initial thirty needs no page',[first(h),loads],['e71',0]);
click(h.querySelector('.history-older'));flush();check('cross-page log chunk',[first(h),h.querySelectorAll('.wb-entry').length],['e41',100]);
// A closed run crossing a page joins without swallowing or losing the cut.
h=start(run(51,logs(51,97),false)+logs(98,100),[run(1,logs(1,50),false)]);
click(h.querySelector('.history-older'));flush();
check('joined closed run retains thirty-line boundary',first(h),'e41');
check('joined rows remain chronological',[h.querySelector('.wb-entry').id,h.querySelectorAll('.wb-entry').at(-1).id],['e1','e100']);
// Fewer than three live panels fetches only candidate pages, one per frame.
h=start(panel(101,200),[panel(51,200),panel(1,200)]);
check('candidate loading reaches three panels',[first(h),h.querySelectorAll('.wb-entry').length],['e1',3]);
check('all history fits',h.querySelector('.history-older').hidden,true);
// Navigation keeps its original allowlist, fold opening and keep lifetime.
h=start(logs(51,100),[panel(1,200)+logs(2,50)]);
check('unknown target refused',window.wbHistory.loadTarget(document,'p999'),null);
check('selector injection refused',window.wbHistory.loadTarget(document,'p1"] script'),null);
var target=window.wbHistory.loadTarget(document,'p1');flush();
check('known panel opens and becomes visible',[target.id,target.open,first(h)],['p1',true,'e1']);
check('navigation uses shared keep offset',keeps.at(-1).opt.offset,12);
keeps.at(-1).opt.onEnd();check('keep end clears target',h._wbHistoryTarget,null);
// Excluded whole rows cannot be native tab stops. Selected fold summaries
// still navigate by their real first-entry anchor if an overlay covers them.
['run-fold','system-run'].forEach(function(kind){
  var html=run(1,logs(1,97),false).replace('run-fold',kind)+logs(98,100);
  h=start(html);
  var folds=h.querySelectorAll('.'+kind), summary=folds[1].querySelector('summary');
  check(kind+' excluded prefix has no rendered box',folds[0].getBoundingClientRect().height,0);
  document.elementFromPoint=function(){return new E('div',{'class':'oq-pill'});};
  fire(summary,'focusin');flush();
  check(kind+' selected but occluded summary uses keep',[first(h),keeps.at(-1).node===summary],['e71',true]);
  keeps.at(-1).opt.onEnd();
  document.elementFromPoint=null;
  var oldestTarget=window.wbHistory.loadTarget(document,'e1');flush();
  check(kind+' deep link reveals excluded whole entry',[first(h),oldestTarget.id],['e1','e1']);
  keeps.at(-1).opt.onEnd();
});
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
