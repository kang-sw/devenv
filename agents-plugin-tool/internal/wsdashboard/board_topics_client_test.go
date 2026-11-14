package wsdashboard

import "testing"

// Parse only the small section fixture vocabulary; accumulation, cloning,
// validation, morphing, stream dispatch and reconnect are production functions.
const boardTopicsClientDOM = `
var boardSeen = {}, openState = {};
function E(key, hash, text) {
 this.nodeType = 1; this.tagName = key ? "SECTION" : "DIV";
 this.attrs = key ? {"data-topic":key,"data-th":hash} : {};
 this.text = text; this.children = []; this.parentNode = null;
 this.classList = { contains: function(c) { return c === "topic" && !!key; } };
}
E.prototype.getAttribute = function(k) { return this.attrs[k] || null; };
E.prototype.hasAttribute = function(k) { return k in this.attrs; };
E.prototype.matches = function() { return false; };
E.prototype.querySelectorAll = function() { return []; };
E.prototype.cloneNode = function() { return new E(this.getAttribute("data-topic"),this.getAttribute("data-th"),this.text); };
function section(k,h,text) { return '<section data-topic="'+k+'" data-th="'+h+'">'+text+'</section>'; }
function frag(html) {
 var nodes = [], re = /<section data-topic="([^"]+)" data-th="([^"]+)">([^<]*)<\/section>|<div>([^<]*)<\/div>/g, m;
 while ((m = re.exec(html))) nodes.push(new E(m[1] || null, m[2], m[3] || m[4]));
 return {childNodes:nodes, firstElementChild:nodes[0] || null};
}
function delta(k,h,text,order) { return {hash:h,order:order,topics:[{key:k,hash:h,html:section(k,h,text)}]}; }
function texts(input) { return boardNodes(input).map(function(n) { return n.text; }); }
function isPeer() { return false; }
function q(sel, el) { return el ? el.querySelectorAll(sel) : [indicator]; }
var pinBox = new E(null,null,"pins");
window.WsCards = {
 pinList:function() { return pinBox; },
 weave:function(want) { return {pins:want.filter(function(n) { return topicKey(n)==="B"; }),board:want.filter(function(n) { return topicKey(n)!=="B"; })}; },
 settle:function(el,p) { el.children=p.board; pinBox.children=p.pins; p.board.forEach(function(n) { n.parentNode=el; }); p.pins.forEach(function(n) { n.parentNode=pinBox; }); }
};
`

func TestBoardTopicsClientSnapshots(t *testing.T) {
	src := dashboardFuncs(t, "topicKey", "topicSeen", "boardInputFull", "boardInputPatch", "boardNodes", "morphBoard", "forgetOpen", "openKey", "frameKeep", "frameKept", "frameChain", "frameGraftable", "frameGrafts", "settleMoves", "frameGraft")
	runNode(t, "board incremental snapshots", streamPageHarness+boardTopicsClientDOM+src+`
var base = boardInputFull(section("A","a1","A1")+section("B","b1","B1")+"<div>reply</div>");
var one = boardInputPatch(base,delta("A","a2","A2",["A","B"]));
var two = boardInputPatch(one,delta("B","b2","B2",["A","B"]));
check("deferred updates accumulate before morph",texts(two),["A2","B2","reply"]);
check("earlier canonical inputs stay immutable",[texts(base),texts(one)],[["A1","B1","reply"],["A2","B1","reply"]]);
var inserted = boardInputPatch(two,delta("C","c1","C1",["C","B","A"]));
var deleted = boardInputPatch(inserted,{hash:"delete",order:["B","C"],topics:[]});
check("order insertion and deletion",[texts(inserted),texts(deleted)],[["C1","B2","A2","reply"],["B2","C1","reply"]]);
check("unknown order key rejects event",boardInputPatch(two,{hash:"bad",order:["missing"],topics:[]}),null);
var board = new E(null,null,"board");
morphBoard(board,base);
var a = board.children[0], b = pinBox.children[0];
a.text="live DOM edit"; b.text="pinned DOM edit";
var aUpdate=boardInputPatch(base,delta("A","a2","A2",["B","A"]));
morphBoard(board,aUpdate);
check("unchanged pinned topic keeps identity through real morphBoard",pinBox.children[0]===b,true);
check("changed topic replaced",board.children[0]===a,false);
check("canonical snapshots never absorb live DOM edits",texts(base),["A1","B1","reply"]);
var liveA=board.children[0];
morphBoard(board,boardInputPatch(aUpdate,{hash:"reorder",order:["A","B"],topics:[]}));
check("unchanged board and pinned topic identities survive order-only patch",[board.children[0]===liveA,pinBox.children[0]===b],[true,true]);
process.stdout.write(bad.join("\n"));
`)
}

func TestBoardTopicsClientHiddenAndRecovery(t *testing.T) {
	src := dashboardFuncs(t, "topicKey", "topicSeen", "boardInputFull", "boardInputPatch", "boardNodes", "openStream", "connected", "connect", "showRegions", "held")
	runNode(t, "board hidden patches and recovery", streamPageHarness+boardTopicsClientDOM+`
var mode="session",rid="rid1",hiddenRegions={},modeHash="m1",liveMode=false,sseOn=true;
var regions=[{getAttribute:function(k) { return k==="data-region"?"board":"old-board"; }},{getAttribute:function(k) { return k==="data-region"?"head":"head-hash"; }}];
function q(sel) { return sel==="[data-region]"?regions:[indicator]; }
function pageHidden() { return doc.hidden; }
function trackerLost() {} function trackerNudge() {} function syncBadge() {} function modeChanged() {} function banner() {}
var tlHeld = {};
function tlCount() {} function tlShown() {}
var applied=[],heldInputs=[];
function swap(n,h,input) { applied.push({hash:h,input:input}); }
function tlHold(n,h,input) { heldInputs.push(input); }
`+src+`
connect();
var s=sockets[0];
doc.hidden=true;
s.msg("region",{name:"board",hash:"full",html:section("A","a1","A1")+section("B","b1","B1")});
s.msg("board-topics",delta("A","a2","A2",["A","B"]));
s.msg("board-topics",delta("B","b2","B2",["B","A"]));
check("hidden tab applies nothing",applied.length,0);
check("hidden input accumulates every earlier patch",texts(hiddenRegions.board.html),["B2","A2"]);
check("held earlier input remains canonical",texts(heldInputs[0]),["A1","B1"]);
doc.hidden=false; showRegions();
check("show applies only the newest complete snapshot",[applied.length,texts(applied[0].input)],[1,["B2","A2"]]);
s.msg("board-topics",{hash:"invalid",order:["unknown"],topics:[]});
check("invalid patch closes old socket",s.closed,true);
advance(0);
check("recovery omits board hash but keeps head and mode and opt-in",sockets[1].url,"ws://localhost:1234/s/rid1/events?h="+encodeURIComponent("head.head-hash,mode.m1")+"&topics=1");
check("invalid patch did not reach swap",applied.length,1);
process.stdout.write(bad.join("\n"));
`)
}
