package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// The picker must expose the focused header, not merely keep it somewhere in
// the physical viewport behind a sticky face. Run the actual key handler.
func TestOverviewKeyboardExposedViewport(t *testing.T) {
	js := mustAsset(t, "overview.js")
	var src strings.Builder
	for _, name := range []string{"clearance", "key"} {
		pattern := `(?s)\n  function ` + name + `\([^\n]*\) \{.*?\n  \}\n`
		f := regexp.MustCompile(pattern).FindString(js)
		if f == "" {
			t.Fatalf("missing %s", name)
		}
		src.WriteString(f)
	}
	runNode(t, "overview exposed keyboard viewport", `
var bad = [], on = true, busy = false, pad = "76px", kept = [], viewport = {top:0,bottom:900,height:900};
function check(name, got, want) { if (JSON.stringify(got)!==JSON.stringify(want)) bad.push(name+": "+JSON.stringify(got)); }
function node(name, top) { return {name:name,top:top,hasAttribute:function(){return false;},querySelector:function(){return null;},getBoundingClientRect:function(){return {top:this.top,bottom:this.top+32,height:32};}}; }
var list=[node("previous",-20),node("next",200)], doc={activeElement:list[1]};
function items(){return list;}
function layer(){return "overview";}
function classify(){return "item";}
function swallow(){}
function rove(n){doc.activeElement=n;}
function scroller(){return {};}
function view(){return viewport;}
function getComputedStyle(){return {scrollPaddingTop:pad};}
function keep(n, offset){kept.push([n.name,offset]);}
`+src.String()+`
function up(top) { list[0].top=top;doc.activeElement=list[1];kept=[];key({key:"ArrowUp",target:{}}); }
up(-20);check("above viewport clears face and fixed host",kept,[["previous",76]]);
check("roving identity retained",doc.activeElement===list[0],true);
up(40);check("inside physical viewport but behind face is revealed",kept,[["previous",76]]);
up(76);check("exact exposed boundary stays put",kept,[]);
up(200);check("already exposed row stays put",kept,[]);
list[1].top=890;doc.activeElement=list[0];kept=[];key({key:"ArrowDown",target:{}});
check("lower edge still uses nearest exposed position",kept,[["next",868]]);
viewport={top:50,bottom:888,height:838};pad="38px";up(80);
check("column padding is relative to column top",kept,[["previous",38]]);
pad="auto";viewport={top:0,bottom:900,height:900};up(-20);
check("unconfigured host retains zero clearance",kept,[["previous",0]]);
process.stdout.write(bad.join("\n"));
`)
}

func TestOverviewEntryPreservesReturnOffset(t *testing.T) {
	js := mustAsset(t, "overview.js")
	for _, want := range []string{
		`entry = state || { key: identity(n), offset: (n || o.toolbar).getBoundingClientRect().top - v.top };`,
		`var shown = n ? Math.max(entry.offset, clearance(n)) : entry.offset;`,
		`motion(before, flights, true, n || o.toolbar, entry.offset, shown, complete)`,
		`keep(n || o.toolbar, shown)`,
		`boardTarget && restore === boardTarget ? entry.offset`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing exposure/return contract %q", want)
		}
	}
}
