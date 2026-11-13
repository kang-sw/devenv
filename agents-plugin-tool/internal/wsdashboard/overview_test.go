package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

func TestOverviewPureContracts(t *testing.T) {
	js := mustAsset(t, "overview.js")
	var src strings.Builder
	for _, name := range []string{"anchor", "classify", "layer", "readState"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\([^\n]*\) \{.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("missing %s", name)
		}
		src.WriteString(f)
	}
	runNode(t, "overview contracts", `
var bad=[];
function check(name, got, want) { if (JSON.stringify(got)!==JSON.stringify(want)) bad.push(name+": "+JSON.stringify(got)); }
var o={board:{contains:function(t){return t.inBoard;}}}, ITEM="items", showing="";
var doc={querySelector:function(sel){return showing && sel.indexOf(showing)>=0 ? {} : null;}};
`+src.String()+`
function node(top,bottom) { return {getBoundingClientRect:function(){return {top:top,bottom:bottom};}}; }
var a=node(-40,20),b=node(20,80),v={top:10,bottom:100};
check("first visible",anchor([a,b],{top:-80,bottom:0},v)===a,true);
check("toolbar visible means no anchor",anchor([a,b],{top:0,bottom:40},v),null);
check("below viewport is not visible",anchor([node(101,150)],{top:-80,bottom:0},v),null);
function target(kind) { return {inBoard:kind==='items',closest:function(sel){return sel.indexOf(kind)>=0?{}:null;}}; }
check("toolbar click",classify(target('.board-toolbar')),"toolbar");
check("popover click",classify(target('.tpop')),"popover");
check("palette click",classify(target('#tkqpop')),"palette");
check("item click",classify(target('items')),"item");
check("rail placeholder",classify(target('.rail-ph')),"outside");
check("no layer",layer(),"overview"); showing='.tpop';check("popover first",layer(),"popover");
showing='dialog';check("palette first",layer(),"palette");
var state={on:true,key:{topic:'sample'},offset:-127.25,pinTop:100};
check("storage round trip",readState(JSON.stringify(state)),state);
['null','{}','{"on":false,"offset":0}','{"on":true,"offset":"bad"}','broken'].forEach(function(s){check("invalid storage "+s,readState(s),null);});
process.stdout.write(bad.join('\n'));
`)
}

func TestOverviewPageIntegration(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	for _, want := range []string{"window.WsOverview.holding()", "boardOverviewHeld = { hash: hash, html: html }", "window.WsOverview.mount({", "keep: window.wbHistory.keep", "window.WsOverview.pick(topic, o)", "doc.addEventListener(\"ws-overview\", queue)", "window.WsOverview.active() ? 0.72 : 0", "if (cardBoard) mountBoardOverview();"} {
		if !strings.Contains(js, want) {
			t.Errorf("missing overview integration %q", want)
		}
	}
	overview := mustAsset(t, "overview.js")
	if strings.Contains(overview, "scrollTop =") || strings.Contains(overview, "scrollIntoView(") || strings.Contains(overview, "localStorage") || strings.Contains(overview, ".fold(") {
		t.Error("overview bypasses view-only/shared scroll ownership")
	}
	for _, want := range []string{"o.keep(n,", "ws.dashboard.overview.", "spacer.style.height", "o.place(true)", "o.place(false)", "scroller(landed) === o.pins", "e.stopImmediatePropagation()", "n.focus({ preventScroll: true })"} {
		if !strings.Contains(overview, want) {
			t.Errorf("missing overview contract %q", want)
		}
	}
	cards := mustAsset(t, "cards.js")
	for _, want := range []string{"if (overviewOn || dg", "window.WsOverview.after(function () { maximize(c); })"} {
		if !strings.Contains(cards, want) {
			t.Errorf("missing card guard %q", want)
		}
	}
}
