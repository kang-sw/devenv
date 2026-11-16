package wsdashboard

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func whiteboardBridgeFunctions(t *testing.T, js string, names ...string) string {
	t.Helper()
	var out strings.Builder
	for _, name := range names {
		block := regexp.MustCompile(`\n  function ` + name + `\([^)]*\) \{[^\n]*\}\n`).FindString(js)
		if block == "" {
			block = regexp.MustCompile(`(?s)\n  function ` + name + `\([^)]*\) \{\n.*?\n  \}\n`).FindString(js)
		}
		if block == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		out.WriteString(block)
	}
	return out.String()
}

func TestBoardEntryFrameBridgeStaysNarrow(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	src := whiteboardBridgeFunctions(t, js, "boardJump", "indexFrames", "docFrame", "postDoc", "str", "num", "mentionOf", "selEl", "entryAt", "frameMention")
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n    if \(!f \|\| !m \|\| typeof m !== "object"\) return;[\s\S]*?\n  \}\);\n`).FindString(js)
	if listener == "" {
		t.Fatal("no source-gated frame listener")
	}
	runNode(t, "entry frame bridge", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var calls = [], jumps = [], reveals = [], pending = null, mpend = null, cchips = {}, handler, menus = [];
// node defines a getter-only global navigator that a top-level var cannot replace.
Object.defineProperty(globalThis, "navigator", { value: { userActivation: { isActive: false } }, configurable: true, writable: true });
function frameLink(m) { return m.href ? { href: m.href } : null; }
function webLink() { return false; }
function openLinkMenu(a, x, y) { menus.push({ a: a, x: x, y: y }); }
var desktopPreviews = [];
function desktopPreview(a) { desktopPreviews.push(a.href); return true; }
var board = { name: "board" }, pins = { name: "pins" }, targets = {};
function target(id, owned) { return targets[id] = { id: id, owned: owned, folded: true, tagName: "DIV", parentNode: null, closest: function () { return this.history ? board : null; }, scrollIntoView: function () { check("owner reveals before fallback scroll " + id, this.folded, false); jumps.push(id); } }; }
var fold = { tagName: "DETAILS", owned: true, open: false, parentNode: null };
target("item-3", true).parentNode = fold;
target("p1", true).parentNode = fold;
target("item-4", false); target("p404", false);
function boardBoxes() { return [board, pins]; }
function inBoard(n) { return !!n && n.owned === true; }
function $(id) { return targets[id] || null; }
var window = { WsCards: { unfoldTopic: function (n) { if (n.owned) { n.folded = false; reveals.push(n.id); } } }, wbHistory: { loadTarget: function (root, id, beforeGeometry) {
  calls.push(root.name + ":" + id);
  if (id.indexOf("item-") === 0) return null;
  var n = id === "p9" && root === pins ? targets.p9 || target("p9", true) : root === board && targets[id] || null;
  if (n && n.owned) {
    check("history receives the named owner", beforeGeometry === window.WsCards.unfoldTopic, true);
    if (beforeGeometry) beforeGeometry(n);
    check("owner reveals before loader geometry " + id, n.folded, false);
  }
  return n;
} }, addEventListener: function (kind, fn) { if (kind === "message") handler = fn; } };
var topic = { getAttribute: function (k) { return k === "data-topic" ? "trusted-topic" : null; } };
var wrapper = { nodeType: 1, getAttribute: function (k) { return k === "data-entry" ? "e7" : k === "data-entry-item" ? "3" : null; }, closest: function (s) { return s === "[data-entry]" ? this : topic; } };
function frame(owned, entry) {
  return { nodeType: 1, owned: owned, parentNode: entry ? wrapper : topic, contentWindow: {},
    getAttribute: function (k) { return k === "data-entry" ? entry : k === "data-doc-key" || k === "data-doc" ? "entry:" + entry : null; },
    closest: function (s) { return s === "[data-entry]" ? (entry ? this : null) : topic; },
    getBoundingClientRect: function () { return { left: 10, top: 20 }; } };
}
var own = frame(true, "e7"), foreign = frame(false, "e999"), frames = [own, foreign];
function q() { return frames; }
function boardQ() { return frames; }
var frameWins = new Map();
function placeMention(p) { pending = p; mpend = p; }
function hideMention() { pending = null; mpend = null; }
`+src+listener+`
boardJump("p1"); boardJump("p9"); boardJump("item-3");
check("existing panel/item targets scroll, including deferred pinned history", jumps, ["p1", "p9", "item-3"]);
targets.p9.history = true; targets.p9.folded = true; boardJump("p9");
check("history navigation delegates positioning to shared keep", jumps, ["p1", "p9", "item-3"]);
check("containing disclosures open before navigation", fold.open, true);
var count = calls.length, revealCount = reveals.length;
["#p1", "e07", "p0", "p01", "p1 div", "item-3,body", "item-3[x]", "https://evil/p1", "../p1", "__proto__", null, 7].forEach(boardJump);
check("non-allowlisted IDs never enter the history loader", calls.length, count);
check("non-allowlisted IDs never unfold a topic", reveals.length, revealCount);
boardJump("item-4"); boardJump("p404"); boardJump("p777");
check("off-board and missing targets never scroll", jumps, ["p1", "p9", "item-3"]);
check("off-board and missing targets never unfold a topic", reveals.length, revealCount);
handler({ source: own.contentWindow, data: { wbDoc: "sel", docKey: "entry:e7", text: "selected HTML", entry: "e999", id: "99", topic: "forged", x: 4, y: 5 } });
check("claimed locator/topic/id are ignored", pending.m, { topic: "trusted-topic", text: "selected HTML", id: "3", entry: "e7" });
check("selection coordinates remain data", [pending.x, pending.y], [4, 5]);
pending = null;
handler({ source: foreign.contentWindow, data: { wbDoc: "sel", docKey: "entry:e7", text: "foreign" } });
handler({ source: {}, data: { wbDoc: "sel", docKey: "entry:e7", text: "unknown source" } });
handler({ source: own.contentWindow, data: { wbDoc: "sel", docKey: "entry:e999", text: "wrong frame identity" } });
handler({ source: own.contentWindow, data: { wbDoc: "sel", text: "missing frame identity" } });
check("only this board's frame with matching identity may mention", pending, null);
handler({ source: foreign.contentWindow, data: { wbDoc: "jump", docKey: "entry:e7", id: "p1" } });
handler({ source: own.contentWindow, data: { wbDoc: "jump", docKey: "entry:e999", id: "p1" } });
check("foreign source or wrong identity cannot jump", jumps, ["p1", "p9", "item-3"]);
handler({ source: own.contentWindow, data: { wbDoc: "jump", docKey: "entry:e7", id: "p1" } });
check("own matching frame can jump to a validated target", jumps, ["p1", "p9", "item-3", "p1"]);
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md", topic: "forged", origin: "forged", x: 4, y: 5 } });
check("frame link derives exact origin from validated frame, never message identity", [menus.length, menus[0].a.wsPreviewOrigin === topic, menus[0].x, menus[0].y], [1, true, 14, 25]);
handler({ source: foreign.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md" } });
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:wrong", href: "docs/a.md" } });
handler({ source: {}, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md" } });
frames = [foreign]; own.owned = false;
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md" } });
check("unknown, wrong-key, off-board and disappeared frames cannot open menus", menus.length, 1);
frames = [own, foreign]; own.owned = true;
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md", mod: true } });
check("a posted modified click without user activation opens the menu, never a preview", [menus.length, desktopPreviews], [2, []]);
navigator.userActivation.isActive = true;
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md", mod: "yes" } });
check("only mod === true asks for the desktop preview", [menus.length, desktopPreviews], [3, []]);
handler({ source: own.contentWindow, data: { wbDoc: "link", docKey: "entry:e7", href: "docs/a.md", mod: true } });
check("an active modified click asks for the desktop preview", [menus.length, desktopPreviews], [3, ["docs/a.md"]]);
navigator.userActivation.isActive = false;
var posted = [];
own.contentWindow.postMessage = function (m) { posted.push(m); };
postDoc(own, { wbDoc: "size" });
check("host replies carry the same frame identity", posted, [{ wbDoc: "size", docKey: "entry:e7" }]);
process.stdout.write(bad.join("\n"));`)
}

func TestBoardEntryDocFrameKeyEnvelope(t *testing.T) {
	data, err := os.ReadFile("../wswhiteboard/assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	if !strings.Contains(js, `d.querySelector('meta[name="wb-doc-key"]')`) {
		t.Fatal("frame identity is not read from its rendered metadata")
	}
	post := regexp.MustCompile(`function post\(m\) \{[^\n]*\}`).FindString(js)
	listener := regexp.MustCompile(`(?s)window.addEventListener\("message", function \(e\) \{.*?\n  \}\);`).FindString(js)
	if post == "" || listener == "" {
		t.Fatal("missing frame bridge")
	}
	runNode(t, "frame identity envelope", `
var bad = [], posted = [], handler, sizes = 0, relays = 0, last = 0;
var docKey = "entry:e7", parent = { postMessage: function (m) { posted.push(m); } };
var window = { parent: parent, addEventListener: function (type, fn) { handler = fn; } };
function size() { sizes++; } function askRelays() { relays++; }
function theme() {} function relayed() {}
`+post+listener+`
post({ wbDoc: "sel", docKey: "forged", text: "selected" });
if (posted[0].docKey !== "entry:e7") bad.push("post failed to bind rendered document identity");
handler({ source: {}, data: { wbDoc: "size", docKey: "entry:e7" } });
handler({ source: parent, data: { wbDoc: "size", docKey: "entry:e8" } });
handler({ source: parent, data: { wbDoc: "size" } });
if (sizes || relays) bad.push("foreign/mismatched host message accepted");
handler({ source: parent, data: { wbDoc: "size", docKey: "entry:e7" } });
if (sizes !== 1 || relays !== 1) bad.push("matching parent message rejected");
process.stdout.write(bad.join("\n"));`)
}

func TestBoardEntryFrameHeightsAreIndependent(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := whiteboardBridgeFunctions(t, string(data), "frameHeights", "setFrameHeights")
	runNode(t, "entry frame heights", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function frame(key, height) { return { getAttribute: function () { return key; }, style: { height: height } }; }
var old = [frame("topic:topic/e7", "100px"), frame("entry:e7", "200px"), frame("entry:e9", "300px")];
var fresh = [frame("entry:e9", ""), frame("topic:topic/e7", ""), frame("entry:e7", ""), frame("entry:e10", "")];
function q(sel, root) { return root; }
`+src+`
setFrameHeights(fresh, frameHeights(old));
check("body and panel versions keep distinct heights across a swap", fresh.map(function (f) { return f.style.height; }), ["300px", "100px", "200px", ""]);
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistorySnapshotIncludesPinnedTopics(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := whiteboardBridgeFunctions(t, string(data), "captureBoardHistory")
	runNode(t, "history snapshots", `
var bad = [];
var window = { wbHistory: { capture: function (root) { return [{ key: root, first: "e7" }]; } } };
function boardBoxes() { return ["board", "pins"]; }
`+src+`
var got = captureBoardHistory();
if (!Array.isArray(got) || got.length !== 2 || got[0].key !== "board" || got[1].key !== "pins") bad.push("history snapshots must stay arrays across all board boxes: " + JSON.stringify(got));
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistoryClosedPagingSkipsMaterializedTopics(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	src := whiteboardBridgeFunctions(t, js, "boardTopicPresent")
	loader := whiteboardBridgeFunctions(t, js, "loadClosed")
	// A topic not yet on the board stays in the page (its clone also feeds the
	// viewed staging cache); a materialized one becomes a count-preserving slot.
	if !regexp.MustCompile(`if \(!boardTopicPresent\(name\)\) \{[^\n}]*return; \}`).MatchString(loader) || !strings.Contains(loader, `s.setAttribute("data-slot", name)`) {
		t.Fatal("closed paging does not replace already materialized topics with count-preserving slots")
	}
	runNode(t, "closed topic deduplication", `
var bad = [];
var window = { WsCards: { pinnedTopic: function (name) { return name === "pinned"; } } };
function boardQ() { return [{ getAttribute: function () { return "jump-loaded"; } }]; }
`+src+`
if (!boardTopicPresent("jump-loaded") || !boardTopicPresent("pinned") || boardTopicPresent("not-loaded")) bad.push("closed paging would duplicate a navigation-loaded or pinned topic");
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistoryHooksSurroundSSESwap(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := whiteboardBridgeFunctions(t, string(data), "swap")
	capture, morph := strings.Index(js, "captureBoardHistory()"), strings.Index(js, "morphBoard(el, html")
	if capture < 0 || morph <= capture {
		t.Fatal("SSE swaps must capture history before the board morph")
	}
	// Tree rows also restore disclosures. Scope this ordering check to the
	// board branch rather than mistaking a tree restoration for the board's.
	board := js[morph:]
	materialize, disclosure := strings.Index(board, "window.wbHistory.restore(n, history)"), strings.Index(board, "restoreOpen(n)")
	outerScroll := strings.Index(board, "restoreScroll(scroll)")
	if materialize < 0 || materialize >= disclosure || outerScroll <= disclosure || strings.Count(js, "window.wbHistory.restore(") != 1 {
		t.Fatal("SSE swaps must restore history once before board disclosures and outer scrolling")
	}

	// Execute the actual dispatcher too: each inserted board root must receive
	// its captured pages before disclosures/frames and the final owner apply.
	// Tree deltas and full fallbacks must use their own lifecycle, never history.
	runNode(t, "board and tree swap ordering", animationFrameHarness+`
var bad = [], log = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var main = {scrollTop:400, wbScrollOwner:true}, pins = {scrollTop:500, wbScrollOwner:true}, outer = {scrollTop:40}, treePane = {scrollTop:80};
var oldMain = {name:"old-main", scroller:main}, oldPin = {name:"old-pin", scroller:pins};
var boxes = [{name:"board", firstElementChild:oldMain}, {name:"pins", firstElementChild:oldPin}];
var topic = {name:"topic"}, pinTopic = {name:"pin-topic"}, row = {name:"row"};
function box(name) { return {name:name, getAttribute:function(){return "old";}, setAttribute:function(){log.push("hash:"+name);}}; }
var boardEl = box("board"), treeEl = box("tree");
Object.defineProperty(treeEl, "innerHTML", {set:function(){log.push("tree-replace");}});
var focus = {isConnected:true, focus:function(opt){check("tree focus avoids scrolling",opt,{preventScroll:true});doc.activeElement=focus;log.push("focus");}};
var doc = {activeElement:focus, scrollingElement:outer};
var window = {wbHistory:{
  capture:function(b){log.push("capture:"+b.name);return [{key:b.name,first:"e7"}];},
  keep:function(n,opt){
    if(opt && opt.capture===false)return {scroller:n.scroller};
    var top=n.scroller.scrollTop;log.push("keep:"+n.name);
    return {apply:function(){
      check("numeric restoration leaves the owner alone "+n.name,n.scroller.scrollTop,0);
      check("outer scroll restoration remains deferred during owner apply",outer.scrollTop,0);
      n.scroller.scrollTop=top;log.push("apply:"+n.name);
    }};
  },
  sweep:function(){log.push("sweep");},
  init:function(n){log.push("init:"+n.name);},
  restore:function(n,h){check("both board boxes supply history",h,[{key:"board",first:"e7"},{key:"pins",first:"e7"}]);log.push("history:"+n.name);}
}};
function boardBoxes(){return boxes;}
function region(name){return name==="board"?boardEl:treeEl;}
function $(){return main;}
function q(){return [];}
var boardPending={}, boardFrozen=[], boardPagedEdits=["paged"], boardLatest=null, boardRendered=null, boardOffered=null;
var boardHomeWrites=new WeakSet(), ctext=null, mpend=null, live=false, TL_REGIONS={}, convo=null, stick=false;
var treeRendered={html:"previous"}, mergeRows=true;
function editing(){return false;}
function reducedMotion(){return true;}
function boardInputFull(html){return html;}
function boardStageInput(html){return html;}
function readAnswers(){return "answers";}
function frameHeights(){return "heights";}
function boardRead(fn){return fn();}
function scrollState(){return [[main,main.scrollTop],[pins,pins.scrollTop],[outer,outer.scrollTop],[treePane,treePane.scrollTop]];}
function morphBoard(el,html,frozen,paged){
  check("morph uses the staged input and pending pages",[el===boardEl,html,frozen,paged],[true,"board-html",[],["paged"]]);
  main.scrollTop=pins.scrollTop=outer.scrollTop=0;log.push("morph");return [topic,pinTopic];
}
function watchBoard(n){log.push("watch:"+n.name);}
function bandBoard(n){log.push("band:"+n.name);}
function restoreOpen(n){log.push("open:"+n.name);}
function fmtTimes(n){log.push("times:"+n.name);}
function sweepOutImgs(n){log.push("images:"+n.name);}
function setFrameHeights(n,h){check("captured heights",h,"heights");log.push("heights:"+n.name);}
function mountFrames(n){log.push("frames:"+n.name);}
function writeAnswers(n,a){check("captured answers",a,"answers");log.push("answers:"+n.name);}
function indexFrames(){} function bindCapsuleNotes(){} function mermaid(){} function syncChips(){} function applySent(){}
function boardRecordPending(){log.push("pending");}
function boardPendingBadge(){} function boardHoldIndicators(){}
function treeInputFull(html){log.push("tree-full");return {html:html};}
function treeInputHTML(input){return input.html;}
function mergeTreeRows(el,previous,next){
  check("tree merge uses its canonical baseline",previous,treeRendered);
  doc.activeElement=null;treePane.scrollTop=0;log.push("tree-merge");return mergeRows?[row]:null;
}
function drawerWait(){log.push("wait");} function agSwapped(){log.push("agents");}
`+whiteboardBridgeFunctions(t, string(data), "swap", "captureBoardHistory", "boardKeep")+dashboardJSBlock(t, "\n  var scrollRestore =", "\n  [\"wheel\"")+`
var numericRestore=restoreScroll;
restoreScroll=function(s){log.push("scroll");numericRestore(s);};
swap("board","new-board","board-html",true);
var prepared=function(name){return ["init:","history:","watch:","open:","times:","images:","heights:","frames:","answers:"].map(function(s){return s+name;});};
check("board lifecycle ordering and exactly-once owner corrections",log,
  ["capture:board","capture:pins","keep:old-main","keep:old-pin","morph","sweep","band:topic","band:pin-topic"]
  .concat(prepared("topic"),prepared("pin-topic"),["hash:board","scroll","apply:old-main","apply:old-pin","pending"]));
check("owner captures apply before the batched numeric restore",[main.scrollTop,pins.scrollTop,outer.scrollTop],[400,500,0]);
flushAnimationFrames();
check("both reader owners survive numeric restoration",[main.scrollTop,pins.scrollTop,outer.scrollTop],[400,500,40]);
check("paged edits are consumed once",boardPagedEdits,[]);
log=[];swap("tree","row-delta",{html:"delta"});
check("tree row lifecycle never captures board history",log,["tree-merge","open:row","times:row","images:row","hash:tree","focus","scroll","wait","agents"]);
flushAnimationFrames();
check("tree pane and retained focus survive the row swap",[treePane.scrollTop,doc.activeElement===focus],[80,true]);
log=[];mergeRows=false;swap("tree","full-fallback","full-html");
check("full tree fallback restores disclosures before scrolling without board history",log,["tree-full","tree-merge","tree-replace","open:tree","times:tree","images:tree","hash:tree","focus","scroll","wait","agents"]);
flushAnimationFrames();
check("full fallback keeps canonical input, pane and focus",[treeRendered.html,treePane.scrollTop,doc.activeElement===focus],["full-html",80,true]);
process.stdout.write(bad.join("\n"));`)
}
