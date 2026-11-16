package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// The dashboard consumes short owner transactions without selecting another
// reference after the morph, and never numerically restores an owned scroller.
func TestBoardKeepWiring(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	swap := dashboardFuncs(t, "swap")
	order := []string{
		"html = boardStageInput(boardLatest.html || html);",
		"var kept = board ? boardKeep() : null",
		"ins = morphBoard(el, html, boardFrozen, boardPagedEdits);",
		"boardPagedEdits = [];",
		"window.wbHistory.sweep();",
		"window.wbHistory.restore(n, history);",
		"watchBoard(n);",
		"restoreScroll(scroll);",
		"if (kept) kept.h.apply();",
	}
	at := 0
	for _, s := range order {
		i := strings.Index(swap[at:], s)
		if i < 0 {
			t.Fatalf("swap lacks %q after offset %d", s, at)
		}
		at += i + len(s)
	}
	if strings.Count(swap, "kept.h.apply()") != 1 {
		t.Error("swap must apply its owner captures exactly once, outside animation setup")
	}
	for _, retired := range []string{"function boardAnchor(", "function boardKeepSettle(", "function boardView(", "function boardKept("} {
		if strings.Contains(js, retired) {
			t.Errorf("retired ordinary target selection remains: %s", retired)
		}
	}
	if !strings.Contains(js, "\n  boardBoxes().forEach(watchBoard);\n") || !strings.Contains(js, "btn.parentNode.replaceChild(page, btn);\n      watchBoard(page);") {
		t.Error("initial and fetched board sections must remain watched")
	}
	for _, re := range []string{
		`(?s)\n  var modeHash = "", hiddenRegions = \{\}, treeRendered = null, entryRendered = null, panelRendered = null;\n.*?\n  function pageHidden\(\) \{ return doc\.hidden \|\| shHidden; \}\n`,
		`function receiveRegion\(ev\) \{[^}]*if \(pageHidden\(\)\) \{\n        delete hiddenRegions\[ev\.name\];`,
		`function showRegions\(\) \{\n    if \(pageHidden\(\)\) return;`,
	} {
		if !regexp.MustCompile(re).MatchString(js) {
			t.Errorf("dashboard.js does not match %s", re)
		}
	}
	if !strings.Contains(dashboardFuncs(t, "topicResized"), "s.wbScrollOwner") {
		t.Error("resize observer remains a competing writer on owned scrollers")
	}
}

func TestBoardKeepUnderNode(t *testing.T) {
	src := dashboardFuncs(t, "boardKeep", "watchBoard", "boardViewedExposure", "restoreScroll")
	runNode(t, "board owner captures", `
var bad = [], log = [], owned = null;
function check(what, ok) { if (!ok) bad.push(what); }
var main = {scrollTop:400, wbScrollOwner:true}, pins = {scrollTop:500, wbScrollOwner:true};
var a = {name:"a", scroller:main}, b = {name:"b", scroller:pins}, c = {name:"c", scroller:main};
var window = {wbHistory:{
 keep:function(n,opt) {
   if (opt && opt.capture === false) return {scroller:n.scroller};
   log.push("capture:"+n.name);
   return {apply:function(){log.push("apply:"+n.name);},end:function(){log.push("end:"+n.name);}};
 },
 watch:function(n){log.push("watch:"+n.name);return {};},
 keeping:function(n){return n===owned ? {} : null;}
}};
function boardBoxes(){return [{firstElementChild:a},{firstElementChild:b}];}
function q(sel,root){return root.kids || [];}
`+src+`
var kept = boardKeep([a,b,c]);
check("pre-capture every actual scroller once",log.join() === "capture:a,capture:b");
kept.h.apply();
check("both independent captures apply once",log.join() === "capture:a,capture:b,apply:a,apply:b");
log=[]; boardKeep().h.end();
check("all boardBoxes participate",log.join() === "capture:a,capture:b,end:a,end:b");
var section={name:"topic",nodeType:1,matches:function(){return true;}};
log=[];watchBoard(section);watchBoard(section);
check("watch once",log.join() === "watch:topic");
var other={scrollTop:5}, transient={scrollTop:60}; owned=transient;
restoreScroll([[main,100],[pins,100],[other,50],[transient,0]]);
check("permanent owner and active jump are not numerically restored",main.scrollTop===400 && pins.scrollTop===500 && other.scrollTop===50 && transient.scrollTop===60);
owned=null;restoreScroll([[main,0],[transient,0]]);
check("ownership persists between transactions",main.scrollTop===400 && transient.scrollTop===0);
process.stdout.write(bad.join("\n"));`)
}
