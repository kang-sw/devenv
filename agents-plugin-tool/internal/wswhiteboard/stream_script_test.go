package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHistoryScriptWholeItemWindowsAndGapNavigationUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	asset = []byte(strings.Replace(string(asset), "var kit = viewKit(window);", "var kit = window.historyKit;", 1))
	// These cases drive gaps explicitly from hand-set layouts; the initial
	// 1000px fill has its own browser test (TestHistoryInitialWindowBrowser).
	asset = []byte(strings.Replace(string(asset), "h._wbHistoryFilling = true; fill(h);", "", 1))
	script := `var BOARD='',BODY_CLASS='';` + formPrelude + `
Object.defineProperty(E.prototype,'id',{get:function(){return this.attrs.id||'';}});
Object.defineProperty(E.prototype,'tagName',{get:function(){return this.tag.toUpperCase();}});
Object.defineProperty(E.prototype,'nodeType',{get:function(){return this.tag==='#fragment'?11:1;}});
Object.defineProperty(E.prototype,'parentElement',{get:function(){return this.parent;}});
Object.defineProperty(E.prototype,'nextElementSibling',{get:function(){return this.parent&&this.parent.children[this.parent.children.indexOf(this)+1]||null;}});
Object.defineProperty(E.prototype,'open',{get:function(){return 'open' in this.attrs;},set:function(v){if(v)this.attrs.open='';else delete this.attrs.open;}});
Object.defineProperty(E.prototype,'isConnected',{get:function(){for(var p=this;p;p=p.parent)if(p===root)return true;return false;}});
var baseClosest=E.prototype.closest;E.prototype.closest=function(s){return s[0]==='#'?baseClosest.call(this,'[id="'+s.slice(1)+'"]'):baseClosest.call(this,s);};
E.prototype.matches=function(s){var self=this;return s.split(',').some(function(p){return one(self,p.trim());});};
E.prototype.remove=function(){if(this.parent){this.parent.children.splice(this.parent.children.indexOf(this),1);this.parent=null;}};
E.prototype.cloneNode=function(deep){var n=new E(this.tag,Object.assign({},this.attrs));n.style=Object.assign({},this.style);n.textContent=this.textContent;if(deep)this.children.forEach(function(c){n.appendChild(c.cloneNode(true));});if(this.content)n.content=this.content.cloneNode(deep);return n;};
E.prototype.insertBefore=function(n,before){var self=this,list=n.tag==='#fragment'?n.children.slice():[n];list.forEach(function(c){c.remove();});var i=before?this.children.indexOf(before):this.children.length;this.children.splice.apply(this.children,[i,0].concat(list));list.forEach(function(c){c.parent=self;});};
E.prototype.appendChild=function(n){this.insertBefore(n,null);return n;};
function size(n){if(n.tag==='template')return 0;if(n.matches('.timeline-row'))return +(n.getAttribute('data-height')||400);return n.children.reduce(function(s,c){return s+size(c);},0);}
var geometry=[],guard=false;
E.prototype.getBoundingClientRect=function(){if(guard){var t=this.closest('.topic');check('owner unfolds before geometry',t&&t.getAttribute('data-size'),'base');geometry.push(this.id);}return {top:0,bottom:size(this),height:size(this)};};
var loads=0,keeps=[],jumps=[],frames=[];
window.requestAnimationFrame=function(fn){frames.push(fn);};
window.setTimeout=function(){};window.clearTimeout=function(){};
window.getComputedStyle=function(){return {position:'static'};};
function flush(){var batch=frames.splice(0);batch.forEach(function(fn){fn();});}
function CustomEvent(type,opt){this.type=type;Object.assign(this,opt||{});}
E.prototype.dispatchEvent=function(e){e.target=this;for(var p=this;p;p=e.bubbles?p.parent:null)(p.on[e.type]||[]).slice().forEach(function(fn){fn(e);});if(e.type==='wbhistoryload')loads++;};
fire=function(n,type){n.dispatchEvent(new CustomEvent(type,{bubbles:true}));};
var watches=[];window.historyKit={seen:function(){return null;},pin:function(){return {apply:function(){}};},watch:function(n,opt){watches.push([n,opt]);return {unwatch:function(){}};},idle:function(){return true;},keep:function(n,opt){if(opt&&opt.capture===false)return {scroller:n.closest('.test-scroller')||root};var k={applied:0,apply:function(){this.applied++;}};keeps.push(k);return k;},jump:function(n,opt){jumps.push([n.id,opt.offset]);}};
function ResizeObserver(fn){this.fn=fn;};ResizeObserver.prototype.observe=function(){};ResizeObserver.prototype.disconnect=function(){};
function row(i,height,body){return '<div class="timeline-row" data-row="e'+i+'" data-count="1" data-from="10:00" data-to="10:01" data-height="'+height+'">'+(body||'<article class="wb-entry agent" id="e'+i+'" data-entry="e'+i+'">line</article>')+'</div>';}
function rows(from,to,height){var s='';for(var i=from;i<=to;i++)s+=row(i,height);return s;}
function gap(html,top){return '<div class="history-gap"><span class="gap-label"></span><span class="gap-range"></span><span><button data-load="up">up</button><button data-load="down"'+(top?' hidden':'')+'>down</button></span><template data-history-gap>'+html+'</template></div>';}
function board(html){root=parse('<section class="topic" data-topic="t"><div class="wb-history" data-history="t"><div class="history-live">'+html+'</div></div></section>');root.querySelectorAll('template').reverse().forEach(function(t){t.content=new E('#fragment');t.children.slice().forEach(function(c){t.content.appendChild(c);});});return root.querySelector('.wb-history');}
function liveIDs(h){return h.querySelectorAll('.timeline-row').map(function(n){return n.getAttribute('data-row');});}
var bound=0;window.wbBindBoard=function(){bound++;};
var h=board(gap(rows(1,10,400),true)+row(11,50)+gap(rows(12,24,400),false)+row(25,50));
` + string(asset) + `
var top=h.querySelector('.history-gap');click(top.querySelector('[data-load="down"]'));check('top has no downward load',liveIDs(h),['e11','e25']);
click(top.querySelector('[data-load="up"]'));
check('up includes straddling third item',liveIDs(h),['e8','e9','e10','e11','e25']);
check('top remainder count and range',[top.getAttribute('data-count'),top.querySelector('.gap-range').textContent,top.querySelector('[data-load="down"]').hidden],['7','10:00–10:01',true]);
var middle=h.querySelectorAll('.history-gap')[1];click(middle.querySelector('[data-load="down"]'));
check('middle down admits only nearest edge',liveIDs(h),['e8','e9','e10','e11','e12','e13','e14','e25']);
click(middle.querySelector('[data-load="up"]'));
check('middle up admits opposite edge',liveIDs(h),['e8','e9','e10','e11','e12','e13','e14','e22','e23','e24','e25']);
check('insertion used shared owner',keeps.every(function(k){return k.applied===1;}),true);
var oldLoads=loads;check('unknown target inert',window.wbHistory.loadTarget(document,'e999'),null);check('injection inert',window.wbHistory.loadTarget(document,'e1"] script'),null);check('invalid cannot admit',loads,oldLoads);
h.closest('.topic').setAttribute('data-size','fold');guard=true;
function reveal(n){n.closest('.topic').setAttribute('data-size','base');}
var target=window.wbHistory.loadTarget(document,'e17',reveal);guard=false;
check('jump materializes target window not intervening prefix',[target.id,!!h.querySelector('[id="e16"]'),!!h.querySelector('[id="e20"]')],['e17',false,false]);
check('jump goes through scroll owner',jumps.at(-1),['e17',12]);
check('known target consumed minimum whole-row window',liveIDs(h).filter(function(id){return ['e17','e18','e19'].includes(id);}),['e17','e18','e19']);
var saved=window.wbHistory.capture(document),before=liveIDs(h);
h=board(gap(rows(1,10,400),true)+row(11,50)+gap(rows(12,24,400),false)+row(25,50)+row(26,50));
var pre=[loads,bound,keeps.length],rect=E.prototype.getBoundingClientRect;
E.prototype.getBoundingClientRect=function(){throw Error('prepare must not read layout');};
window.wbHistory.prepare(document,saved);
window.wbHistory.prepare(document,saved);
E.prototype.getBoundingClientRect=rect;
check('pre-morph preparation restores admitted rows from inert gaps exactly once',liveIDs(h),before.concat(['e26']));
check('preparation neither mounts nor binds nor captures scroll',[loads,bound,keeps.length],pre);
check('unadmitted history remains inert',!!h.querySelector('[id="e1"]'),false);
window.wbHistory.restore(document,saved);check('swap restores exact membership without eviction or extra window',liveIDs(h),before.concat(['e26']));
window.wbHistory.init(document);check('one listener after repeated init',h.on.click.length,1);
h=board(gap(rows(1,10,400),true)+row(11,20));window.wbHistory.init(document);window.wbHistory.loadTarget(document,'e10');
check('near-end target fills from the nearest prefix to reach 1000px',liveIDs(h),['e8','e9','e10','e11']);
h=board(gap(rows(1,100,5),true)+row(101,20));window.wbHistory.init(document);click(h.querySelector('[data-load="up"]'));check('no thirty-line ceiling',liveIDs(h).length,101);
var panels='';for(var i=1;i<=5;i++)panels+=row(i,200,'<article class="wb-entry panel" id="e'+i+'" data-entry="e'+i+'"><details class="entry-panel"><summary>panel</summary></details></article>');
h=board(gap(panels,true)+row(6,20));window.wbHistory.init(document);click(h.querySelector('[data-load="up"]'));check('exact 1000px has no three-panel ceiling',liveIDs(h),['e1','e2','e3','e4','e5','e6']);
// First usable frame height resumes a pending load. Late growth cannot recut.
var panel='<article class="wb-entry panel" id="e12" data-entry="e12" data-panel="p1"><details class="entry-panel" id="p1" open><summary>panel</summary><iframe class="doc-frame"></iframe></details></article>';
h=board(row(10,50)+gap(row(11,600)+row(12,500,panel)+row(13,100),false)+row(14,50));window.wbHistory.init(document);
click(h.querySelector('[data-load="down"]'));check('unknown panel waits after admission',[h._wbHistoryPending.length,liveIDs(h)],[1,['e10','e11','e12','e14']]);
var pendingSnapshot=window.wbHistory.capture(document);
h=board(row(10,50)+gap(row(11,600)+row(12,500,panel)+row(13,100),false)+row(14,50));window.wbHistory.restore(document,pendingSnapshot);
check('pending window survives a swap',h._wbHistoryPending.length,1);
h.querySelector('.doc-frame').style.height='500px';h._wbHistoryObserver.fn();flush();check('straddling rendered panel completes',[h._wbHistoryPending.length,liveIDs(h)],[0,['e10','e11','e12','e14']]);
h.querySelector('.doc-frame').style.height='1800px';h._wbHistoryObserver.fn();flush();check('late height never evicts or adds',liveIDs(h),['e10','e11','e12','e14']);
// An offscreen frame can stall the initial bottom window in the same gap
// as a navigation/reload target. Admit that target without canceling fill.
h=board(gap(rows(1,10,400)+row(12,500,panel),true)+row(14,50));window.wbHistory.init(document);
h._wbHistoryFilling=true;h._wbHistoryObserver.fn();flush();
check('initial fill waits for admitted frame',[h._wbHistoryPending.length,liveIDs(h)],[1,['e12','e14']]);
var initialWindow=h._wbHistoryPending[0];
var immediate=window.wbHistory.loadTarget(document,'e10');
check('navigation admits target despite pending initial fill',[immediate&&immediate.id,liveIDs(h)],['e10',['e10','e12','e14']]);
check('navigation retains unfinished initial window',h._wbHistoryPending[0]===initialWindow,true);
h.querySelector('.doc-frame').style.height='500px';h._wbHistoryObserver.fn();flush();
h._wbHistoryObserver.fn();flush();
check('initial fill still reaches bottom 1000px after navigation',[h._wbHistoryPending.length,h._wbHistoryFilling,liveIDs(h)],[0,false,['e9','e10','e12','e14']]);
// Panel IDs and item anchors share validation and unfolding before geometry.
h=board(gap(row(12,1400,panel),true)+row(14,50));window.wbHistory.init(document);var p=window.wbHistory.loadTarget(document,'p1');check('oversized panel is one whole window',[p.id,p.open,liveIDs(h)],['p1',true,['e12','e14']]);
h=board(row(1,20,'<article class="wb-entry system" id="e1" data-entry="e1"><div class="card" id="item-3"></div></article>'));window.wbHistory.init(document);check('question target uses same owner',window.wbHistory.loadTarget(document,'item-3').id,'item-3');check('question owner jump',jumps.at(-1),['item-3',12]);
// A system fold may absorb the former latest event after an append. Restore
// by durable contained entries, not just the fold's first-entry row key.
h=board(row(5,20,'<details class="system-run" open><article class="wb-entry system" id="e5" data-entry="e5"></article></details>'));window.wbHistory.init(document);var foldSnapshot=window.wbHistory.capture(document);
h=board(gap(row(1,20,'<details class="system-run"><article class="wb-entry system" id="e1" data-entry="e1"></article><article class="wb-entry system" id="e5" data-entry="e5"></article></details>'),true)+row(6,20));window.wbHistory.restore(document,foldSnapshot);
check('regrouped system event remains admitted and open',[!!h.querySelector('[id="e5"]'),h.querySelector('.system-run').open],[true,true]);
// Auto-fold: once the card has fully left the view, owner-loaded rows go
// back into gaps (an exhausted gap is rebuilt from the kept shell); server
// rows stay, and the rebuilt gaps load again.
h=board(gap(rows(1,2,400),true)+row(3,50)+gap(rows(4,8,400),false)+row(9,50));window.wbHistory.init(document);
var foldWatch=watches.filter(function(x){return x[0]===h.closest('.topic');}).at(-1)[1];
click(h.querySelector('.history-gap [data-load="up"]'));
var lower=h.querySelector('.history-gap');click(lower.querySelector('[data-load="up"]'));click(lower.querySelector('[data-load="down"]'));
check('loads exhaust both gaps',[liveIDs(h),h.querySelectorAll('.history-gap').length],[['e1','e2','e3','e4','e5','e6','e7','e8','e9'],0]);
foldWatch.onChange({visible:true});foldWatch.onOut({ratio:0});
var folded=h.querySelectorAll('.history-gap');
check('fold returns loaded rows to rebuilt gaps',[liveIDs(h),folded.map(function(g){return g.getAttribute('data-count');}),folded.map(function(g){return g.querySelector('[data-load="down"]').hidden;})],[['e3','e9'],['2','5'],[true,false]]);
click(folded[1].querySelector('[data-load="up"]'));check('rebuilt gap loads again',liveIDs(h),['e3','e6','e7','e8','e9']);
foldWatch.onChange({visible:true});foldWatch.onOut({ratio:0});check('second fold',liveIDs(h),['e3','e9']);
// The landed initializer still shares one capture per actual scroller.
var batchHistories=[];for(var i=0;i<8;i++)batchHistories.push(board(gap(rows(1,10,400),true)+row(11,20)));
root=new E('div');
for(var c=0;c<2;c++){var column=new E('div',{'class':'test-scroller'});root.appendChild(column);batchHistories.slice(c*4,c*4+4).forEach(function(h){column.appendChild(h.closest('.topic'));});}
var initialKeeps=keeps.length,initialReads=0,rect=E.prototype.getBoundingClientRect;
E.prototype.getBoundingClientRect=function(){initialReads++;return rect.call(this);};
window.wbHistory.init(document);E.prototype.getBoundingClientRect=rect;
check('eight histories share two reading captures',keeps.length-initialKeeps,2);
check('initial result geometry is one read per history',initialReads,8);
check('server anchors remain unchanged by batched init',batchHistories.map(liveIDs),Array.from({length:8},function(){return ['e11'];}));
window.wbHistory.init(document);check('bound histories never capture again',keeps.length-initialKeeps,2);
check('every actual capture applies once',keeps.every(function(k){return k.applied===1;}),true);
if(bad.length)throw Error(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("timeline script: %v\n%s", err, out)
	}
}

func TestHistoryScriptWiredIntoSharedAssets(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PageAssets()
	if err != nil || !strings.Contains(string(shared.FormScript), a.stream) {
		t.Fatalf("shared history script missing: %v", err)
	}
	// The viewport fold is back as a gap refold; the clip budget stays retired.
	for _, old := range []string{"PANELS =", "LINES =", "--wb-history-height"} {
		if strings.Contains(a.stream, old) {
			t.Errorf("retired clip budget path %s", old)
		}
	}
	if !strings.Contains(string(a.css), ".history-gap") || strings.Contains(string(a.css), "max-height: var(--wb-history-height") {
		t.Fatal("gaps replaced by clipped history")
	}
}

// TestHistoryReadyHeightAcceptsZeroHeightRows pins readyHeight in stream.js:
// a laid-out row whose content is all hidden (zero height, but a box) is
// ready at 0 and never blocks the initial window or "load above"; a row with
// no box yet (a host not laid out) still waits. The cases execute under node
// when it is installed.
func TestHistoryReadyHeightAcceptsZeroHeightRows(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; readyHeight cases not executed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(asset)
	start := strings.Index(src, "  function readyHeight(row) {")
	if start < 0 {
		t.Fatal("readyHeight not found in stream.js")
	}
	end := strings.Index(src[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("readyHeight has no end in stream.js")
	}
	script := src[start:start+end+4] + `
var bad = [];
function check(what, got, want) { if (got !== want) bad.push(what + ": got " + got + ", want " + want); }
function row(height, boxes) { return { querySelectorAll: function () { return []; }, getBoundingClientRect: function () { return { height: height }; }, getClientRects: function () { return { length: boxes }; } }; }
check("a visible row reads its height", readyHeight(row(24, 1)), 24);
check("a laid-out row with hidden content is ready at zero", readyHeight(row(0, 1)), 0);
check("a row with no box waits", readyHeight(row(0, 0)), null);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("readyHeight cases failed:\n%s", s)
	}
}
