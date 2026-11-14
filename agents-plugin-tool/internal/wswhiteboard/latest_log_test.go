package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestLatestLogHighlightMarkersAndMotion(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="current"><wb:log>old</wb:log><wb:log>latest</wb:log></wb:topic>`)
	for _, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out, `class="trecap" data-entry="e2" data-kind="agent"`)
		if layout == LayoutFull {
			wantAll(t, out, `class="wb-entry agent current" id="e2"`, `class="wb-entry agent" id="e1"`)
		}
	}
	css, err := embeddedAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css),
		`.wb-entry.agent.current { background: var(--wb-latest-log); }`,
		`--wb-latest-log: rgb(110 200 130 / .08);`,
		`animation: wb-log-arrival 1s ease-in-out 3`,
		`@media (prefers-reduced-motion: reduce)`, `.wb-entry.agent.current.wb-log-blink { animation: none; }`)
	if strings.Contains(string(css), `.trecap[data-kind="agent"]`) {
		t.Fatal("latest-log highlight must not target recap copies")
	}
}

func TestLatestLogViewportArrivalUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	_, arrival, ok := strings.Cut(string(asset), "/* Latest agent arrival:")
	if !ok {
		t.Fatal("missing latest-log script")
	}
	arrival = "/* Latest agent arrival:" + arrival
	script := `
var now=Date.now(), reduced=false, timers=[], candidates=[], all=[], instances=[], binds=0;
function check(label, got, want) { if(JSON.stringify(got)!==JSON.stringify(want)) throw Error(label+': '+JSON.stringify(got)); }
function row(topic,id,at) {
  var classes=new Set(), parent={tagName:'SECTION',parentElement:null,getAttribute:function(){return topic;}};
  return {isConnected:true,parentElement:parent,
    closest:function(){return parent;},getAttribute:function(){return id;},
    querySelector:function(){return {getAttribute:function(){return new Date(at||now-1000).toISOString();}};},
    classList:{add:function(c){classes.add(c);},remove:function(c){classes.delete(c);},contains:function(c){return classes.has(c);}}};
}
function blinking(n){return n.classList.contains('wb-log-blink');}
var first=row('t','e1');candidates=[first];all=[first];
var document={body:{},querySelectorAll:function(s){return s==='template'?[]:s==='.wb-entry.agent.current'?candidates:all;}};
var window={matchMedia:function(){return {matches:reduced};},setTimeout:function(fn,ms){check('duration',ms,3000);timers.push(fn);},wbBindBoard:function(){binds++;},
  IntersectionObserver:function(fn){this.fn=fn;this.nodes=new Set();this.observe=function(n){this.nodes.add(n);};this.unobserve=function(n){this.nodes.delete(n);};instances.push(this);}};
function enter(n,visible){instances[0].fn([{target:n,isIntersecting:visible,intersectionRatio:visible?1:0}]);}
` + arrival + `
check('initial latest never observed',instances[0].nodes.size,0);
var header=row('t','e2');all.push(header);window.wbLatestLog.init();
check('header recap never observed',instances[0].nodes.has(header),false);
check('header recap never blinks',blinking(header),false);
var next=row('t','e2');candidates=[next];window.wbBindBoard(document);
check('bind delegates',binds,1);
check('new latest awaits viewport',instances[0].nodes.has(next),true);
enter(next,false);check('offscreen has no blink',blinking(next),false);
enter(next,true);check('first sight blinks',blinking(next),true);
check('one timer',timers.length,1);timers.shift()();check('blink ends',blinking(next),false);
window.wbLatestLog.init();check('consumed never reobserved',instances[0].nodes.size,0);
var replacement=row('t','e2');candidates=[replacement];window.wbLatestLog.init();
check('replacement of seen line does not blink',instances[0].nodes.size,0);
var folded=row('t','e3');folded.parentElement.tagName='DETAILS';folded.parentElement.open=false;
candidates=[folded];window.wbLatestLog.init();enter(folded,true);
check('folded line waits',blinking(folded),false);check('folded stays observed',instances[0].nodes.has(folded),true);
folded.parentElement.open=true;enter(folded,true);check('unfolded first sight blinks',blinking(folded),true);timers.shift()();
var superseded=row('t','e4'), latest=row('t','e5');candidates=[superseded];window.wbLatestLog.init();
candidates=[latest];window.wbLatestLog.init();check('superseded unobserved',instances[0].nodes.has(superseded),false);
enter(superseded,true);check('stale observer delivery ignored',blinking(superseded),false);
reduced=true;enter(latest,true);check('reduced motion never blinks',blinking(latest),false);check('no reduced timer',timers.length,0);
reduced=false;window.wbLatestLog.init();check('reduced arrival consumed',instances[0].nodes.size,0);
var oldClosed=row('closed','e6');candidates=[oldClosed];window.wbLatestLog.init();
check('fetched old closed topic is baseline',instances[0].nodes.size,0);
var freshTopic=row('new','e7',now+60000);candidates=[freshTopic];window.wbLatestLog.init();
check('new topic awaits sight',instances[0].nodes.has(freshTopic),true);enter(freshTopic,true);timers.shift()();
// A replacement of the same lower log never starts another animation.
var duplicate=row('new','e7',now+60000);candidates=[duplicate];window.wbLatestLog.init();
check('duplicate consumed by entry identity',instances[0].nodes.size,0);
// Re-evaluating the shared asset on a board swap retains the seen set.
` + arrival + `
check('one observer across script reruns',instances.length,1);
check('rerun does not blink',blinking(duplicate),false);
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("latest log: %v\n%s", err, out)
	}
}
