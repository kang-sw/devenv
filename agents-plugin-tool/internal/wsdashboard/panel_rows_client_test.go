package wsdashboard

import "testing"

func TestPanelRowsSwapLifecycle(t *testing.T) {
	src := dashboardFuncs(t, "swap")
	runNode(t, "panel swap lifecycle ordering", `
var bad=[],log=[];
function check(label,a,b){if(JSON.stringify(a)!==JSON.stringify(b))bad.push(label+": "+JSON.stringify(a));}
var live=false,TL_REGIONS={},panelRendered={value:"old"},merge=true;
var el={name:"panels",getAttribute:function(){return "old";},setAttribute:function(){log.push("hash");}};
Object.defineProperty(el,"innerHTML",{set:function(){log.push("replace");}});
var changed={name:"agenda"};
function region(){return el;}
function scrollState(){log.push("scroll-capture");return "panes";}
function panelReaderState(n){check("capture region",n===el,true);log.push("reader-capture");return "reader";}
function panelInputFull(html){log.push("full");return {value:html};}
function panelInputHTML(input){return input.value;}
function mergePanelRows(n,previous,next){check("canonical previous",previous,panelRendered);log.push("merge");return merge?[changed]:null;}
function restoreOpen(n){log.push("open:"+n.name);}
function fmtTimes(n){log.push("times:"+n.name);}
function sweepOutImgs(n){log.push("images:"+n.name);}
function syncSpinnerPhases(){}
function drawerSync(){log.push("drawer-sync");}
function restorePanelReader(n,state){check("reader snapshot",[n===el,state],[true,"reader"]);log.push("reader-restore");}
function restoreScroll(state){check("pane snapshot",state,"panes");log.push("scroll-restore");}
function drawerSwapped(){log.push("drawer-swapped");}
`+src+`
swap("panels","delta",{value:"next"});
check("changed cards lifecycle",log,["scroll-capture","reader-capture","merge","open:agenda","times:agenda","images:agenda","hash","drawer-sync","reader-restore","scroll-restore","drawer-swapped"]);
check("delta canonical remembered",panelRendered.value,"next");
log=[];merge=false;swap("panels","full","legacy");
check("full fallback lifecycle",log,["scroll-capture","reader-capture","full","merge","replace","open:panels","times:panels","images:panels","hash","drawer-sync","reader-restore","scroll-restore","drawer-swapped"]);
check("old daemon canonical remembered",panelRendered.value,"legacy");
log=[];swap("panels","old",{value:"first-full-current"});
check("same hash initializes without remount",[log,panelRendered.value],[[],"first-full-current"]);
process.stdout.write(bad.join("\\n"));
`)
}

// Chromium exercises real validation and DOM merging; this probe isolates the
// production stream owner's mandatory full recovery and hidden accumulation.
func TestPanelRowsClientRecovery(t *testing.T) {
	src := streamClientFuncs(t, "openStream", "connected", "connect", "showRegions")
	runNode(t, "panel delta dispatch and recovery", streamPageHarness(t)+`
var mode="session",rid="rid1",worktree="",hiddenRegions={},modeHash="m1",liveMode=false,sseOn=true;
function held(){return "panels.old,head.kept";}
function trackerLost(){} function trackerNudge(){} function syncBadge(){} function modeChanged(){} function banner(){}
function tlHold(){} function tlCount(){} function tlShown(){} var tlHeld={};
var applied=[],bases=[];
function swap(name,hash,input){applied.push([name,hash,input.value]);}
function panelInputFull(html){return html==="invalid-full"?null:{value:html};}
function panelInputPatch(base,ev){bases.push(base&&base.value);return base&&ev&&ev.valid?{value:base.value+ev.add}:null;}
`+src+`
connect();var s=sockets[0];
check("session opts in",s.url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("panels.old,head.kept")+"&topics=1&railrows=1&treerows=1&entryrows=1&panelrows=1&display=1");
s.msg("region",{name:"panels",hash:"old",html:"A"});
check("first full reaches same-hash owner",applied,[["panels","old","A"]]);
doc.hidden=true;
s.msg("panel-rows",{hash:"one",valid:true,add:"B"});
s.msg("panel-rows",{hash:"two",valid:true,add:"C"});
check("hidden inputs accumulate",[applied.length,hiddenRegions.panels.html.value,bases],[1,"ABC",["A","AB"]]);
doc.hidden=false;showRegions();
check("latest canonical input applies once",applied[1],["panels","two","ABC"]);
s.msg("panel-rows",{hash:"bad",valid:false});
check("invalid closes without applying",[s.closed,applied.length],[true,2]);
advance(0);
check("reconnect opts in with held hashes",sockets[1].url,s.url);
sockets[1].msg("panel-rows",{hash:"premature",valid:true,add:"bad"});
check("new socket cannot inherit canonical input",[sockets[1].closed,applied.length],[true,2]);
advance(0);
sockets[2].msg("region",{name:"panels",hash:"fresh",html:"D"});
sockets[2].msg("panel-rows",{hash:"next",valid:true,add:"E"});
check("old daemon full repairs baseline",applied[3],["panels","next","DE"]);
doc.hidden=true;
sockets[2].msg("panel-rows",{hash:"held",valid:true,add:"F"});
sockets[2].fail();advance(3000);
check("browser-hidden reconnect requests no display work",sockets[3].url,s.url.replace("display=1","display=0"));
sockets[3].msg("region",{name:"panels",hash:"reconnected-full",html:"G"});
sockets[3].msg("panel-rows",{hash:"reconnected-delta",valid:true,add:"H"});
check("hidden reconnect replaces stale input",[applied.length,hiddenRegions.panels.html.value],[4,"GH"]);
sockets[2].msg("panel-rows",{hash:"stale",valid:true,add:"bad"});
check("closed socket cannot contaminate hidden input",hiddenRegions.panels.html.value,"GH");
doc.hidden=false;showRegions();
check("show applies recovered latest",applied[4],["panels","reconnected-delta","GH"]);
framed=true;connect();
check("frame opts in without rail",sockets[4].url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("panels.old,head.kept")+"&frame=1&topics=1&treerows=1&entryrows=1&panelrows=1&display=1");
shell=true;connect();
check("shell excludes panels",sockets[5].url,"ws://localhost:1234/rail/events?h="+encodeURIComponent("panels.old,head.kept")+"&frame=1&railrows=1&display=1");
shell=false;framed=false;mode="index";connect();
check("index excludes panels",sockets[6].url,"ws://localhost:1234/events?h="+encodeURIComponent("panels.old,head.kept")+"&display=1");
mode="session";worktree="/fixture";connect();
check("worktree excludes panels",sockets[7].url,"ws://localhost:1234/events?h="+encodeURIComponent("panels.old,head.kept")+"&topics=1&railrows=1&display=1");
process.stdout.write(bad.join("\n"));
`)
}
