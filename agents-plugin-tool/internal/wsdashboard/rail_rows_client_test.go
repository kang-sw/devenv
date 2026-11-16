package wsdashboard

import "testing"

// Real dispatch/reconnect/hidden-region code, with a small typed input stub.
// Chromium exercises the real input parser and keyed DOM merger separately.
func TestRailRowsClientRecovery(t *testing.T) {
	src := dashboardFuncs(t, "openStream", "connected", "connect", "showRegions")
	runNode(t, "rail delta dispatch and recovery", streamPageHarness+`
var mode="session",rid="rid1",worktree="",hiddenRegions={},modeHash="m1",liveMode=false,sseOn=true;
function held() { return "rail.old,head.kept"; }
function pageHidden() { return doc.hidden; }
function trackerLost() {} function trackerNudge() {} function syncBadge() {} function modeChanged() {} function banner() {}
function tlHold() {} function tlCount() {} function tlShown() {} var tlHeld={};
function frag(html) { return html; } function apScan() {}
var applied=[], bases=[];
function swap(name,hash,input) { applied.push([name,hash,input]); }
var WsRail={
 rowInputFull:function(html) { return {value:html}; },
 rowInputHTML:function(input) { return input.value; },
 rowInputPatch:function(base,ev) { bases.push(base); return base && ev && ev.valid ? {value:base.value+ev.add} : null; }
};
`+src+`
connect(); var s=sockets[0];
s.msg("region",{name:"rail",hash:"full",html:"A"});
check("legacy full regions remain usable",applied,[["rail","full",{value:"A"}]]);
doc.hidden=true;
s.msg("rail-rows",{hash:"one",valid:true,add:"B"});
s.msg("rail-rows",{hash:"two",valid:true,add:"C"});
check("hidden patches use newest canonical baseline",[applied.length,hiddenRegions.rail.html.value,bases.map(function(b){return b.value;})],[1,"ABC",["A","AB"]]);
doc.hidden=false;showRegions();
check("only newest complete input applies",applied[1],["rail","two",{value:"ABC"}]);
s.msg("rail-rows",{hash:"invalid",valid:false});
check("untrusted delta closes stream without applying",[s.closed,applied.length],[true,2]);
advance(0);
check("reconnect retains other hashes and opts in",sockets[1].url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("rail.old,head.kept")+"&topics=1&railrows=1&treerows=1&entryrows=1&panelrows=1");
// The new connection cannot use the previous one's canonical row baseline.
sockets[1].msg("rail-rows",{hash:"premature",valid:true,add:"bad"});
check("delta before first full is rejected",[sockets[1].closed,applied.length],[true,2]);
advance(0);
sockets[2].msg("region",{name:"rail",hash:"fresh",html:"D"});
sockets[2].msg("rail-rows",{hash:"next",valid:true,add:"E"});
check("full region repairs baseline",applied[3],["rail","next",{value:"DE"}]);
process.stdout.write(bad.join("\n"));
`)
}
