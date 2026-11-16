package wsdashboard

import (
	"strings"
	"testing"
)

func TestKeyedRowsClientHelpers(t *testing.T) {
	src := dashboardFuncs(t, "keyedSame", "keyedInputPatch", "keyedInputHTML", "mergeEntriesRegion")
	runNode(t, "keyed row helpers", streamPageHarness(t)+`
var box = {rows:{a:"old A",b:"old B"}}, incoming = {rows:{a:"new A",b:"new B"}};
function entriesBox() { return box; }
function entryInputHTML(input) { return "full"; }
function frag() { return {querySelector:function(s) { return s === ".centries" ? incoming : null; }}; }
function gapped() { return false; }
function mergeEntries(existing, next, changed) {
  check("same-key differing full rows marked changed", [changed.a, changed.b], [true, true]);
  check("merge receives existing and incoming rows", [existing.rows.a, next.rows.a], ["old A", "new A"]);
  return [];
}
function mermaid() {} function sweepOutImgs() {}
`+src+`
mergeEntriesRegion({querySelector:function() { return null; }},
  {shell:"shell",order:["a","b"],rows:{a:{html:"new A"},b:{html:"new B"}}}, false, null);
function rowNode(html, key) { return html === "new " + key ? {outerHTML:html} : null; }
var tree = {shell:"tree",order:["parent","child"],parents:[-1,0],rows:{parent:{html:"old parent"},child:{html:"old child"}}};
var treeNext = keyedInputPatch(tree,{hash:"h",order:["parent","child"],rows:[{key:"child",hash:"c",html:"new child"}]},rowNode);
check("tree patch preserves parents",treeNext.parents,[-1,0]);
check("tree patch updates child without mutating baseline",[treeNext.rows.child.html,tree.rows.child.html,treeNext.rows.parent.html],["new child","old child","old parent"]);
var entry = {shell:"entry",order:["a"],rows:{a:{html:"old a"}}};
var entryNext = keyedInputPatch(entry,{hash:"h",order:["a"],rows:[{key:"a",hash:"a1",html:"new a"}]},rowNode);
check("entry patch updates row without mutating baseline",[entryNext.shell,entryNext.rows.a.html,entry.rows.a.html],["entry","new a","old a"]);
check("validator rejects row",keyedInputPatch(entry,{hash:"h",order:["a"],rows:[{key:"a",hash:"a1",html:"new a"}]},rowNode,function() { return false; }),null);
var entryHTML = {shell:"<main><!--ws-entry-row-0--></main>",order:["a"],rows:{a:{html:"<p>entry</p>"}}};
check("entry HTML fills slot",keyedInputHTML(entryHTML,/<!--ws-entry-row-(\d+)-->/g),"<main><p>entry</p></main>");
var treeHTML = {shell:"<main><!--ws-tree-row-0--></main>",order:["parent","child"],parents:[-1,0],rows:{parent:{html:"<div>parent<!--ws-tree-row-1--></div>"},child:{html:"<div>child</div>"}}};
check("tree HTML fills nested child slot",keyedInputHTML(treeHTML,/<!--ws-tree-row-(\d+)-->/g,true),"<main><div>parent<div>child</div></div></main>");
check("HTML string passes through",keyedInputHTML("raw",/unused/g),"raw");
process.stdout.write(bad.join("\n"));
`)
}

func TestDashboardFuncsOneLine(t *testing.T) {
	src := dashboardFuncs(t, "keyedSame")
	if strings.Count(src, "\n") != 2 || strings.Contains(src, "keyedInputPatch") {
		t.Fatalf("one-line extraction swallowed another declaration: %q", src)
	}
}
