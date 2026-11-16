package wsdashboard

import "testing"

// Dispatch runs the real reconnect and hidden-region owner here. Chromium runs
// the production parser and merger, without replacing them with these inputs.
func TestEntryRowsClientRecovery(t *testing.T) {
	src := dashboardFuncs(t, "openStream", "connected", "connect", "showRegions")
	runNode(t, "entry delta dispatch and hidden recovery", streamPageHarness+`
var mode="session",rid="rid1",worktree="",hiddenRegions={},modeHash="m1",liveMode=false,sseOn=true;
function held() { return "entries.old,head.kept"; }
function pageHidden() { return doc.hidden || shHidden; }
function trackerLost() {} function trackerNudge() {} function syncBadge() {} function modeChanged() {} function banner() {}
function tlHold() {} function tlCount() {} function tlShown() {} var tlHeld={};
var applied=[], bases=[];
function swap(name,hash,input) { applied.push([name,hash,input.value]); }
function entryInputFull(html) { return html==="invalid-full" ? null : {value:html}; }
function entryInputPatch(base,ev) { bases.push(base && base.value); return base && ev && ev.valid ? {value:base.value+ev.add} : null; }
`+src+`
connect(); var s=sockets[0];
check("session opts in",s.url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("entries.old,head.kept")+"&topics=1&railrows=1&treerows=1&entryrows=1&panelrows=1");
s.msg("region",{name:"entries",hash:"old",html:"A"});
check("first full with held hash still reaches owner",applied,[["entries","old","A"]]);
doc.hidden=true;
s.msg("entry-rows",{hash:"one",valid:true,add:"B"});
s.msg("entry-rows",{hash:"two",valid:true,add:"C"});
check("hidden changes accumulate immutable full inputs",[applied.length,hiddenRegions.entries.html.value,bases],[1,"ABC",["A","AB"]]);
doc.hidden=false;showRegions();
check("only newest hidden input applies",applied[1],["entries","two","ABC"]);
s.msg("entry-rows",{hash:"bad",valid:false});
check("invalid event closes without applying",[s.closed,applied.length],[true,2]);
advance(0);
check("reconnect opts in with held hashes",sockets[1].url,s.url);
sockets[1].msg("entry-rows",{hash:"premature",valid:true,add:"bad"});
check("new connection cannot inherit baseline",[sockets[1].closed,applied.length],[true,2]);
advance(0);
sockets[2].msg("region",{name:"entries",hash:"fresh",html:"D"});
sockets[2].msg("entry-rows",{hash:"next",valid:true,add:"E"});
check("old daemon full events repair baseline",applied[3],["entries","next","DE"]);
shHidden=true;
sockets[2].msg("entry-rows",{hash:"held",valid:true,add:"F"});
sockets[2].fail();advance(3000);
sockets[3].msg("region",{name:"entries",hash:"reconnected-full",html:"G"});
sockets[3].msg("entry-rows",{hash:"reconnected-delta",valid:true,add:"H"});
check("hidden reconnect replaces old canonical inputs",[applied.length,hiddenRegions.entries.html.value],[4,"GH"]);
sockets[2].msg("entry-rows",{hash:"stale",valid:true,add:"bad"});
check("closed socket cannot contaminate pending input",hiddenRegions.entries.html.value,"GH");
shHidden=false;showRegions();
check("show applies reconnected latest",applied[4],["entries","reconnected-delta","GH"]);
framed=true;connect();
check("frame opts in without rail rows",sockets[4].url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("entries.old,head.kept")+"&frame=1&topics=1&treerows=1&entryrows=1&panelrows=1");
shell=true;connect();
check("shell excludes entry rows",sockets[5].url,"ws://localhost:1234/rail/events?h="+encodeURIComponent("entries.old,head.kept")+"&frame=1&railrows=1");
shell=false;framed=false;mode="index";connect();
check("index excludes entry rows",sockets[6].url,"ws://localhost:1234/events?h="+encodeURIComponent("entries.old,head.kept"));
mode="session";worktree="/fixture";connect();
check("worktree excludes entry rows",sockets[7].url,"ws://localhost:1234/events?h="+encodeURIComponent("entries.old,head.kept")+"&topics=1&railrows=1");
process.stdout.write(bad.join("\n"));
`)
}
