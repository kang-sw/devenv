package wsdashboard

import "testing"

// Dispatch uses the real WebSocket and hidden-region code. The real DOM input
// validator and merger run in TestTreeRowsBrowser, including malformed events.
func TestTreeRowsClientRecovery(t *testing.T) {
	src := dashboardFuncs(t, "openStream", "connected", "connect", "showRegions")
	runNode(t, "tree delta dispatch and hidden recovery", streamPageHarness+`
var mode="session",rid="rid1",worktree="",hiddenRegions={},modeHash="m1",liveMode=false,sseOn=true;
function held() { return "tree.old,head.kept"; }
function pageHidden() { return doc.hidden; }
function trackerLost() {} function trackerNudge() {} function syncBadge() {} function modeChanged() {} function banner() {}
function tlHold() {} function tlCount() {} function tlShown() {} var tlHeld={};
var applied=[], bases=[];
function swap(name,hash,input) { applied.push([name,hash,input.value]); }
function treeInputFull(html) { return {value:html}; }
function treeInputPatch(base,ev) { bases.push(base && base.value); return base && ev && ev.valid ? {value:base.value+ev.add} : null; }
`+src+`
connect(); var s=sockets[0];
s.msg("region",{name:"tree",hash:"full",html:"A"});
check("legacy full accepted",applied,[["tree","full","A"]]);
doc.hidden=true;
s.msg("tree-rows",{hash:"one",valid:true,add:"B"});
s.msg("tree-rows",{hash:"two",valid:true,add:"C"});
check("hidden updates accumulate immutable inputs",[applied.length,hiddenRegions.tree.html.value,bases],[1,"ABC",["A","AB"]]);
doc.hidden=false;showRegions();
check("only newest hidden input applies",applied[1],["tree","two","ABC"]);
s.msg("tree-rows",{hash:"bad",valid:false});
check("invalid event closes without applying",[s.closed,applied.length],[true,2]);
advance(0);
check("reconnect opts in and preserves held hashes",sockets[1].url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("tree.old,head.kept")+"&topics=1&railrows=1&treerows=1");
sockets[1].msg("tree-rows",{hash:"premature",valid:true,add:"bad"});
check("connection cannot inherit another baseline",[sockets[1].closed,applied.length],[true,2]);
advance(0);
sockets[2].msg("region",{name:"tree",hash:"fresh",html:"D"});
sockets[2].msg("tree-rows",{hash:"next",valid:true,add:"E"});
check("full repairs baseline",applied[3],["tree","next","DE"]);
doc.hidden=true;
sockets[2].msg("tree-rows",{hash:"held",valid:true,add:"F"});
sockets[2].fail(); advance(3000);
sockets[3].msg("region",{name:"tree",hash:"reconnected-full",html:"G"});
sockets[3].msg("tree-rows",{hash:"reconnected-delta",valid:true,add:"H"});
check("hidden reconnect replaces old connection inputs",[applied.length,hiddenRegions.tree.html.value],[4,"GH"]);
doc.hidden=false;showRegions();
check("show uses only reconnected latest",applied[4],["tree","reconnected-delta","GH"]);
process.stdout.write(bad.join("\n"));
`)
}
