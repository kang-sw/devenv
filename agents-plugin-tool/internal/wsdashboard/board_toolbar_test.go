package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestBoardToolbarBackdrop(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	rule := regexp.MustCompile(`(?s)\.board-toolbar\[data-floating\] \.board-toolbar-face::before\s*\{([^}]+)\}`).FindStringSubmatch(css)
	if len(rule) != 2 {
		t.Fatal("floating toolbar needs an opaque top-gap backing attached to its compact face")
	}
	// The positioned face, not the taller reserved slot, bounds the backing's
	// zero bottom inset. Keep the upward mask without intercepting pointers.
	face := regexp.MustCompile(`(?s)\.board-toolbar-face\s*\{([^}]+)\}`).FindStringSubmatch(css)
	if len(face) != 2 || !strings.Contains(face[1], "position: relative") {
		t.Fatal("backing must be positioned relative to the compact face")
	}
	if regexp.MustCompile(`\.board-toolbar\[data-floating\]::before\s*\{`).MatchString(css) {
		t.Error("reserved toolbar slot must not paint a backing below the face")
	}
	for _, want := range []string{"position: absolute", "inset: calc(-12px - var(--toolbar-top, 0px)) 0 0", "background: var(--wb-bg)", "z-index: -1", "pointer-events: none"} {
		if !strings.Contains(rule[1], want) {
			t.Errorf("backdrop missing %q", want)
		}
	}
}

func TestBoardToolbarPlacementAndCounts(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "board toolbar", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var A, B, X, moreBtn, calls = [], options = { toolbar: {
  overview: function () { calls.push("overview"); }, download: function () { calls.push("download"); }, files: function () { calls.push("files"); }
} };
function served() {
  A = topic("a", 300); B = topic("b", 200); X = topic("x", 0, true);
  moreBtn = more(1); moreBtn.setAttribute("data-closed-more", "7");
  return [A, B, X, moreBtn, reply()];
}
boot({}, served, options);
var bar = page.W.toolbar(), gal = page.W.gallery(), moves = 0;
function first(what) { check(what, page.board.firstChild === bar, true); }
function count() { return toolbarCounts(); }
check("separate count removed", bar.querySelector(".board-count"), null);
first("toolbar first on a narrow board");
check("the gallery stands outside the board", [gal.parentNode === page.dbody, page.board.contains(gal)], [true, false]);
check("not a card or rail item", [page.W.is(bar), bar.hasAttribute("data-rail"), bar.querySelectorAll(".tpin, .bc-hd").length], [false, false, 0]);
check("button order and terminal slot", bar.querySelector(".board-tools").children.map(function (n) { return n.className; }), ["board-action board-overview", "board-term-slot", "board-web-slot", "board-action board-download", "board-action board-files", "board-state-filter"]);
check("overview starts unpressed", bar.querySelector(".board-overview").getAttribute("aria-pressed"), "false");
["overview", "download", "files"].forEach(function (name) { bar.querySelector(".board-" + name).fire("click", {}); });
check("overview and download use mount callbacks; Files without an explorer is inert", calls, ["overview", "download"]);
check("open topics plus closed shown and remainder", count(), "open 2 · closed 8");
var insert = page.board.insertBefore, remove = page.board.removeChild;
page.board.insertBefore = function (n, ref) { if (n === bar) moves++; return insert.call(this, n, ref); };
page.board.removeChild = function (n) { if (n === bar) moves++; return remove.call(this, n); };
clickPin(B); first("pin keeps toolbar first");
check("a pin is counted once", count(), "open 2 · closed 8");
page.width = 1800; page.W.relayout(); first("rail-only width keeps toolbar first");
check("the gallery stays paged until its column fits", [page.layouts, page.states[page.states.length - 1].narrow], [[], true]);
page.width = 2560; page.W.relayout(); first("column width crossing keeps toolbar first");
check("the gallery shows as a column", [page.layouts, gal.parentNode === page.dbody], [[true], true]);
check("column topics count", count(), "open 2 · closed 8");
var layoutBefore = page.layouts.slice();
page.W.overview(true); first("reserved overview placement keeps toolbar first");
check("overview leaves the gallery where it stands, the layout unchanged", [gal.parentNode === page.dbody, page.W.isDocked(B), page.layouts], [true, true, layoutBefore]);
page.W.relayout();
var c = page.W.open("k", { name: "C" });
check("arrange in overview keeps the docked topic docked", page.W.isDocked(B), true);
page.width = 1000; page.W.relayout(); page.width = 2560; page.W.relayout();
check("a wide crossing changes only the layout", [page.W.isDocked(B), page.layouts], [true, [true, false, true]]);
page.W.overview(false);
check("exit keeps the current layout", [page.W.isDocked(B), page.layouts], [true, [true, false, true]]);
c.pin(true); c.pin(false); c.close(); first("card pin/unpin/close keep toolbar first");
clickPin(B); first("unpin keeps toolbar first");
check("the last undock leaves the wide empty slot", [gal.hidden, page.states[page.states.length - 1].empty, page.layouts], [false, true, [true, false, true, false]]);
var nextMore = more(2); nextMore.setAttribute("data-closed-more", "4");
B = topic("b", 400); X = topic("x", 0, true);
swap([B, X, topic("y", 0, true), nextMore, reply()]); first("weave and settle keep toolbar first");
check("landed swap updates count", count(), "open 1 · closed 6");
var closedPage = mk("div.closed-page", {}, [topic("z", 0, true), more(3)]);
closedPage.children[1].setAttribute("data-closed-more", "3");
page.board.insertBefore(closedPage, nextMore); page.board.removeChild(nextMore); page.W.decorate();
check("closed load decorates and refreshes count", count(), "open 1 · closed 6");
page.W.move(B, { pin: true, index: 0 }); page.W.move(B, { pin: false, after: null, before: null });
first("drop placement keeps toolbar first");
check("toolbar never moved during any operation", moves, 0);
check("toolbar and overview placement never persist", /toolbar|overview|board-term-slot/.test(page.store.cards || ""), false);

// A real pointer drag: hovering over the toolbar is not a flow drop. A
// drop below it interpolates only between topics, not the infrastructure.
boot({}, function () { A = topic("a", 300); B = topic("b", 200); return [A, B]; });
bar = page.W.toolbar();
function rect(n, top, height) {
  n.getClientRects = function () { return [1]; };
  n.getBoundingClientRect = function () { return { top: top, bottom: top + height, height: height, left: 0, right: 500, width: 500 }; };
}
rect(page.board, 0, 500); rect(bar, 0, 40); rect(A, 60, 40); rect(B, 120, 40);
var head = B.querySelector(".tbar"), e = { target: head, isPrimary: true, button: 0, pointerId: 1, pointerType: "mouse", clientX: 50, clientY: 130 };
page.board.fire("pointerdown", e);
page.doc.listeners.pointermove[0]({ pointerId: 1, pointerType: "mouse", buttons: 1, clientX: 50, clientY: 20, preventDefault: function () {} });
page.doc.listeners.pointerup[0]({ pointerId: 1, type: "pointerup" });
check("toolbar rejects drops", names(page.board), ["TOOLBAR", "a", "b"]);
page.board.fire("pointerdown", e);
page.doc.listeners.pointermove[0]({ pointerId: 1, pointerType: "mouse", buttons: 1, clientX: 50, clientY: 50, preventDefault: function () {} });
page.doc.listeners.pointerup[0]({ pointerId: 1, type: "pointerup" });
check("the toolbar is never a flow peer", names(page.board), ["TOOLBAR", "b", "a"]);
var saved = JSON.parse(page.store.cards).topics.filter(function (r) { return r.n === "b"; })[0];
check("head drop uses real topic time, not toolbar time", saved.a > 300, true);

boot({}, function () { return [mk("div.empty")]; });
check("empty board keeps the toolbar before the placeholder", names(page.board), ["TOOLBAR", "DIV"]);
check("Files is absent without a pane callback", page.W.toolbar().querySelector(".board-files"), null);
process.stdout.write(bad.join("\n"));
`)
}

func TestToolbarTerminalAvailability(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	term, _ := json.Marshal(mustAsset(t, "terminal.js"))
	runNode(t, "toolbar terminal availability", "var CARDS = "+string(cards)+", TERM = "+string(term)+";\n"+cardsDOM+`
function start(on) {
  boot({}, function () { return []; });
  page.doc.documentElement.setAttribute("data-rid", "session");
  if (on) page.doc.documentElement.setAttribute("data-term", "xterm.js");
  function fetch() { return Promise.resolve({ ok: true, json: function () { return { terminals: [], folders: { worktree: "/work", root: "/repo" } }; } }); }
  function MutationObserver() { this.observe = function () {}; }
  new Function("document", "window", "wsT", "fetch", "MutationObserver", TERM)(page.doc, page.win, wsT, fetch, MutationObserver);
  if (page.win.WsTerm) page.win.WsTerm.mount(page.board);
}
start(false);
check("no terminal without data-term", [!!page.win.WsTerm, page.W.toolbar().querySelectorAll(".tnew").length], [false, 0]);
start(true);
// Let the initial list settle before opening: both root and worktree
// choices must survive the entry point move.
setImmediate(function () {
var slot = page.doc.getElementById("board-term-slot"), btn = slot.querySelector(".tnew"), pop = slot.querySelector(".tpop");
check("one terminal entry in toolbar", [page.doc.body.querySelectorAll(".tnew").length, !!btn, pop.hidden], [1, true, true]);
check("terminal has visible and accessible labels", [btn.querySelector(".board-label").textContent, btn.getAttribute("aria-label"), btn.getAttribute("aria-expanded")], ["js.term.new", "js.term.new", "false"]);
btn.fire("click", {});
check("opens dialog below toolbar", [pop.hidden, btn.getAttribute("aria-expanded"), pop.querySelectorAll(".topt").length], [false, "true", 2]);
var prevented = false, stopped = false;
pop.fire("keydown", { key: "Escape", preventDefault: function () { prevented = true; }, stopPropagation: function () { stopped = true; } });
check("Escape closes only the popover", [pop.hidden, btn.getAttribute("aria-expanded"), prevented, stopped], [true, "false", true, true]);
var next = page.W.weave([topic("new", 2000)]); page.W.settle(page.board, next);
check("board swap preserves button and popover", [page.doc.getElementById("board-term-slot") === slot, slot.querySelector(".tnew") === btn, slot.querySelector(".tpop") === pop], [true, true, true]);
var requests = [], refusal = true, shown = false;
page.W.open = function (kind, data) { requests.push([kind, data]); return { fold: function () {}, show: function () { shown = true; } }; };
page.win.WsTerm.setup({ post: function (route, body, done) {
  requests.push([route, body.folder, body.path]);
  if (refusal) done(false, "not_found"); else done(true, { id: "T1", cwd: "/work" });
} });
btn.fire("click", {});
var input = pop.querySelector("input"), go = pop.querySelector(".pr");
input.value = "/bad"; go.fire("click", {});
check("typed path keeps refusal and dialog", [requests[0], pop.querySelector(".ferr").textContent, pop.hidden], [["terminals", "path", "/bad"], "js.term.err.missing", false]);
refusal = false; pop.querySelector(".topt").fire("click", {});
check("worktree choice opens terminal card and closes dialog", [requests[1], requests[2], shown, pop.hidden], [["terminals", "worktree", ""], ["terminal", { tid: "T1", cwd: "/work" }], true, true]);
process.stdout.write(bad.join("\n"));
});
`)
}

func TestToolbarPopoverFitsViewport(t *testing.T) {
	js := mustAsset(t, "terminal.js")
	place := regexp.MustCompile(`(?s)\n  function placePop\(\) \{.*?\n  \}\n`).FindString(js)
	if place == "" {
		t.Fatal("missing placePop")
	}
	runNode(t, "toolbar popover placement", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": " + JSON.stringify(got)); }
var window = { innerWidth: 420, innerHeight: 600 }, r;
var ui = { btn: { offsetHeight: 30 }, pop: { style: {}, getBoundingClientRect: function () { return r; } } };
`+place+`
r = { left: 200, right: 540, top: 100 }; placePop();
check("right edge clamped", ui.pop.style.left, "-128px");
check("below button with available space", [ui.pop.style.top, ui.pop.style.maxHeight], ["38px", "492px"]);
r = { left: -20, right: 320, top: 580 }; placePop();
check("left edge clamped", ui.pop.style.left, "28px");
check("near bottom keeps options reachable in scroll box", [ui.pop.style.top, ui.pop.style.maxHeight], ["-22px", "72px"]);
window.innerWidth = 1400; window.innerHeight = 900;
ui.col = { getClientRects: function () { return [1]; }, getBoundingClientRect: function () { return { left: 300, right: 700, top: 50, bottom: 800 }; } };
r = { left: 500, right: 840, top: 120 }; placePop();
check("popover also fits the clipping board column", [ui.pop.style.left, ui.pop.style.maxWidth, ui.pop.style.maxHeight], ["-148px", "384px", "672px"]);
process.stdout.write(bad.join("\n"));
`)
}

func TestToolbarPageActionHooks(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	for _, want := range []string{
		// The download saves the session board's zip; worktree pages have no session board.
		`download: worktree ? null : function () {`,
		`a.href = "/s/" + rid + "/board.zip"; a.download = "";`,
		// Worktree pages carry their own Files pane, so their toolbar omits the files sheet (d384c5136).
		`files: worktree ? null : filesSheet`,
		`filesTab = createFiles(fpane, chead && chead.querySelector(".fsum"), false)`,
		`var sheetInstance = null`,
		`if (!sheetInstance) sheetInstance = createFiles(sheet, null, true)`,
		`if (sheetInstance) { sheetInstance.cancelPreview(); sheetInstance.hidden(); }`,
		`load("ws.dashboard.toolbar." + k)`,
		`save("ws.dashboard.toolbar." + k, v)`,
		`if (fs) filesTab.shown()`,
		`else filesTab.hidden()`,
		`local: function () { return root.hasAttribute("data-sysopen"); }`,
		`if (b) { setCtab(b.getAttribute("data-ctab")); return; }`,
		`tail.setAttribute("data-closed-more", String(Math.max(0, (+more.getAttribute("data-closed-more") || 0) - count)))`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing toolbar page hook %s", want)
		}
	}
}

// Exercise the production observer with controlled rectangles, independent of
// Chromium's font metrics. Browser acceptance covers the actual CSS geometry.
func TestToolbarFloatingGeometry(t *testing.T) {
	js := mustAsset(t, "cards.js")
	floating := regexp.MustCompile(`(?s)\n  function floatToolbar\(\) \{.*?\n  \}\n`).FindString(js)
	if floating == "" {
		t.Fatal("missing floatToolbar")
	}
	runNode(t, "toolbar floating geometry", animationFrameHarness+`
var bad=[];
function check(name,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(name+": "+JSON.stringify(got));}
function styles(){var p={};return {getPropertyValue:function(k){return p[k]||"";},setProperty:function(k,v){p[k]=v;},removeProperty:function(k){delete p[k];}};}
var listeners={}, observed=[], resize, boardTop=0, compact=false, faceHeight=46, handles=[];
function action(y){return {getBoundingClientRect:function(){var h=compact?24:30;return {top:y-h/2,bottom:y+h/2,height:h};}};}
var row={children:[action(23),action(23),action(23)]}, face={firstChild:row,getClientRects:function(){return [1];},getBoundingClientRect:function(){return {width:700,height:faceHeight,bottom:faceHeight};}};
var toolbar={firstChild:face,style:styles(),attrs:{},hasAttribute:function(k){return k in this.attrs;},toggleAttribute:function(k,on){if(on)this.attrs[k]="";else delete this.attrs[k];}};
var board={getBoundingClientRect:function(){return {top:boardTop};}}, col={style:styles(),clientTop:0,getBoundingClientRect:function(){return {top:0};}}, documentScroller={style:styles()}, active=col;
var doc={scrollingElement:documentScroller,querySelectorAll:function(){return handles;},getElementById:function(){return col;},addEventListener:function(k,fn){listeners[k]=fn;}}, window={addEventListener:function(k,fn){listeners[k]=fn;},ResizeObserver:function(fn){resize=fn;this.observe=function(n){observed.push(n);};}};
var maxed=null;
function scroller(){return active;}
`+floating+`
floatToolbar();
check("initial layout is deferred",toolbar.style.getPropertyValue("--toolbar-reserved"),"");
flushAnimationFrames();
check("observe the face and board without extra targets",observed.length===2 && observed[0]===face && observed[1]===board,true);
check("top reserves original",[toolbar.hasAttribute("data-floating"),toolbar.style.getPropertyValue("--toolbar-reserved")],[false,"46px"]);
boardTop=-100;compact=true;faceHeight=28;row.children=[action(14),action(14),action(14)];listeners.scroll();
check("scroll coalesces layout",animationFrames.size,1);
flushAnimationFrames();
check("compact keeps original slot",[toolbar.hasAttribute("data-floating"),toolbar.style.getPropertyValue("--toolbar-reserved")],[true,"46px"]);
check("face drives rail and arrivals",[col.style.getPropertyValue("--wb-rail-inset"),col.style.getPropertyValue("scroll-padding-top")],["34px","38px"]);
// The filter is a direct row child just like an action. A filter-only wrap
// reserves a second full original row even while the live face is compact.
row.children=[action(14),action(14),action(46)];faceHeight=60;resize();flushAnimationFrames();
check("filter-only wrap",[toolbar.hasAttribute("data-wrapped-actions"),toolbar.style.getPropertyValue("--toolbar-reserved")],[true,"82px"]);
boardTop=0;compact=false;faceHeight=82;resize();flushAnimationFrames();
boardTop=-200;compact=true;faceHeight=60;resize();flushAnimationFrames();
check("original filter row survives floating",toolbar.style.getPropertyValue("--toolbar-reserved"),"82px");
row.children=[action(14),action(46),action(46)];resize();flushAnimationFrames();
check("wrapped actions reserve two original action rows",[toolbar.hasAttribute("data-wrapped-actions"),toolbar.style.getPropertyValue("--toolbar-reserved")],[true,"82px"]);
active=documentScroller;resize();flushAnimationFrames();
check("scroller handoff clears old padding",[col.style.getPropertyValue("scroll-padding-top"),documentScroller.style.getPropertyValue("scroll-padding-top")],["","70px"]);
maxed={files:true,el:{style:styles()}};resize();flushAnimationFrames();
check("Files clears the live face",maxed.el.style.getPropertyValue("--files-top"),"68px");
handles=[{getClientRects:function(){return [1];},getBoundingClientRect:function(){return {top:0,bottom:34};}}];resize();flushAnimationFrames();
check("document face clears fixed handles",toolbar.style.getPropertyValue("--toolbar-top"),"38px");
check("document rail and arrivals include handle clearance",[col.style.getPropertyValue("--wb-rail-inset"),documentScroller.style.getPropertyValue("scroll-padding-top")],["104px","108px"]);
handles=[];
boardTop=0;compact=false;faceHeight=82;resize();flushAnimationFrames();
check("return restores original slot",[toolbar.hasAttribute("data-floating"),toolbar.style.getPropertyValue("--toolbar-reserved")],[false,"82px"]);
process.stdout.write(bad.join("\n"));
`)
}
