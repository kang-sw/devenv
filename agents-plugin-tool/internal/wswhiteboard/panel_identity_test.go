package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPanelIdentityFoldAndUnreadBadgeUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(asset)
	start, end := strings.Index(source, "  function panelStateKit(win)"), strings.Index(source, "\n  var kit = viewKit(window);")
	if start < 0 || end <= start {
		t.Fatal("shared panel state factory missing")
	}
	script := `var BOARD = '';` + formPrelude + `
Object.defineProperty(E.prototype,'parentElement',{get:function(){return this.parent;}});
Object.defineProperty(E.prototype,'open',{get:function(){return this.hasAttribute('open');},set:function(v){if(v)this.setAttribute('open','');else this.removeAttribute('open');}});
var storage={};
window.document=document;
window.localStorage={getItem:function(k){return storage[k]||null;},setItem:function(k,v){storage[k]=v;}};
document.documentElement.setAttribute('data-wb-reply','session-a');
document.on={};document.addEventListener=function(k,fn){(document.on[k]||(document.on[k]=[])).push(fn);};
function fixture(panel,entry,older){
  // An earlier version is a sibling row written before the latest one.
  root=parse('<div class="wb-history">'+(older?'<article class="wb-entry panel historical" data-panel="'+panel+'" data-entry="'+older+'"><details class="entry-panel" data-key="entry:'+older+':history"><summary>old recap</summary></details></article><div class="wb-entry system" data-entry="e99"></div>':'')+'<article class="wb-entry panel" data-panel="'+panel+'" data-entry="'+entry+'"><details class="entry-panel" id="'+panel+'" data-key="entry:'+entry+'" open><summary>recap<span class="wb-panel-updated" hidden>UPDATED</span></summary></details></article></div>');
  return root.querySelector('.entry-panel[id]');
}
function pending(p){return !p.querySelector('.wb-panel-updated').hidden;}
function userOpen(kit,p){document.on.click.at(-1)({target:p.querySelector('summary')});p.open=true;return kit.toggle(p);}
` + source[start:end] + `
var kit=panelStateKit(window), p=fixture('p1','e1');
check('new pN retains default open',kit.restore(p)&&p.open&&!pending(p),true);
p.open=false;kit.toggle(p);
for(var i=0;i<5;i++){
  p=fixture('p1','e1');kit.restore(p);
  check('stable entry unrelated update '+i+' stays folded without false update',[p.open,pending(p)],[false,false]);
}
p=fixture('p1','e2','e1');kit.restore(p);
check('same pN new eN stays folded and visibly updated',[p.open,pending(p)],[false,true]);
p.open=true;check('transient graft open is not reader acknowledgement',kit.toggle(p),false);
kit.restore(p);check('transient graft toggle cannot clear unread mark',[p.open,pending(p)],[false,true]);
var snapshot=kit.capture(root);
kit=panelStateKit(window);p=fixture('p1','e2','e1');kit.restore(p);
check('reload preserves pending fold and badge',[p.open,pending(p)],[false,true]);
p=fixture('p1','e3','e2');kit.restore(p);
check('repeated republish remains pending until actual open',[p.open,pending(p)],[false,true]);
check('actual summary open is accepted',userOpen(kit,p),true);
check('actual open clears badge exactly once',[p.open,pending(p)],[true,false]);
kit=panelStateKit(window);p=fixture('p1','e3','e2');kit.restore(p);
check('acknowledgement survives reload',[p.open,pending(p)],[true,false]);
p.open=false;kit.toggle(p);
var fresh=fixture('p2','e4');kit.restore(fresh);
check('unrelated new pN keeps default, not another panel fold',[fresh.open,pending(fresh)],[true,false]);

storage['ws.dashboard.panelshut']=JSON.stringify(['session-a/entry:e5']);
var legacy=fixture('p3','e6','e5');kit.restore(legacy);
check('legacy latest-entry fold migrates through known historical metadata',[legacy.open,pending(legacy)],[false,true]);
var historical=root.querySelector('.historical .entry-panel');historical.open=true;
check('historical-version fold is not the current identity record',kit.restore(historical),false);
check('historical manual choice remains independent',historical.open,true);
kit.opened(legacy);
check('explicit deep-link open acknowledges the actual current panel',[legacy.open,pending(legacy)],[true,false]);

var oldSnapshotPanel=fixture('p4','e9','e8');
kit.snapshot(root,{folds:[['entry:e8',false]]});kit.restore(oldSnapshotPanel);
check('legacy standalone snapshot migrates old entry fold',[oldSnapshotPanel.open,pending(oldSnapshotPanel)],[false,true]);
// A historical-version-only choice must never migrate into latest pN state.
var independent=fixture('p5','e11','e10');
kit.snapshot(root,{folds:[['entry:e10:history',false]]});kit.restore(independent);
check('historical-version key is not a latest-panel legacy fold',[independent.open,pending(independent)],[true,false]);
// A newer persisted acknowledgement outranks an older captured snapshot.
p=fixture('p1','e3','e2');kit=panelStateKit(window);kit.snapshot(root,{panels:snapshot});kit.restore(p);
check('stale snapshot cannot resurrect an acknowledged badge',pending(p),false);
if(bad.length)throw Error(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("panel identity: %v\n%s", err, out)
	}
}

func TestPanelUpdatedMarkIsProminentAndLatestOnly(t *testing.T) {
	st := streamFixture()
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		out := markup(render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"})))
		if layout == LayoutFull {
			if strings.Count(out, `class="wb-panel-updated" hidden`) != 4 {
				t.Fatal("one initially hidden update mark per latest panel required")
			}
			wantAll(t, out, `data-wb-reply="r"`, "UPDATED")
		} else {
			wantNone(t, out, `class="wb-panel-updated"`)
		}
	}
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), `.wb-panel-updated {`, `border: 2px solid var(--wb-primary)`, `background: var(--wb-primary)`, `.wb-panel-pending > .entry-panel`)
}
