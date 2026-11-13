package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestBoardToolbarPlacementAndCounts(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "board toolbar", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var A, B, X, moreBtn, calls = [], options = { toolbar: {
  overview: function () { calls.push("overview"); }, search: function () { calls.push("search"); }, files: function () { calls.push("files"); }
} };
function served() {
  A = topic("a", 300); B = topic("b", 200); X = topic("x", 0, true);
  moreBtn = more(1); moreBtn.setAttribute("data-closed-more", "7");
  return [A, B, X, moreBtn, reply()];
}
boot({}, served, options);
var bar = page.W.toolbar(), pins = page.W.pinList(), moves = 0;
function first(what) { check(what, page.board.firstChild === bar, true); }
function count() { return bar.querySelector(".board-count").textContent; }
first("toolbar first on a narrow board");
check("toolbar precedes pins", bar.nextSibling === pins, true);
check("not a card or rail item", [page.W.is(bar), bar.hasAttribute("data-rail"), bar.querySelectorAll(".tpin, .bc-hd").length], [false, false, 0]);
check("button order and terminal slot", bar.firstChild.children.map(function (n) { return n.className; }), ["board-action board-overview", "board-term-slot", "board-web-slot", "board-action board-search", "board-action board-files", "board-count"]);
check("overview starts unpressed", bar.querySelector(".board-overview").getAttribute("aria-pressed"), "false");
["overview", "search", "files"].forEach(function (name) { bar.querySelector(".board-" + name).fire("click", {}); });
check("actions use mount callbacks", calls, ["overview", "search", "files"]);
check("open topics plus closed shown and remainder", count(), "open 2 · closed 8");
var insert = page.board.insertBefore, remove = page.board.removeChild;
page.board.insertBefore = function (n, ref) { if (n === bar) moves++; return insert.call(this, n, ref); };
page.board.removeChild = function (n) { if (n === bar) moves++; return remove.call(this, n); };
clickPin(B); first("pin keeps toolbar first");
check("a pin is counted once", count(), "open 2 · closed 8");
options.wide = true; page.W.relayout(); first("width crossing keeps toolbar first");
check("pins moved to column", pins.parentNode === page.col, true);
check("column topics count", count(), "open 2 · closed 8");
var layoutBefore = page.layouts.slice();
page.W.overview(true); first("reserved overview placement keeps toolbar first");
check("overview moves one list immediately after toolbar without layout changes", [bar.nextSibling === pins, pins.parentNode === page.board, page.layouts], [true, true, layoutBefore]);
page.W.relayout();
var c = page.W.open("k", { name: "C" });
check("arrange holds overview pins in board", pins.parentNode === page.board, true);
options.wide = false; page.W.relayout(); options.wide = true; page.W.relayout();
check("a wide crossing holds the list", [pins.parentNode === page.board, bar.nextSibling === pins], [true, true]);
page.W.overview(false);
check("exit uses current column layout", pins.parentNode === page.col, true);
c.pin(true); c.pin(false); c.close(); first("card pin/unpin/close keep toolbar first");
clickPin(B); first("unpin keeps toolbar first");
check("last unpin returns list after toolbar", bar.nextSibling === pins, true);
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
bar = page.W.toolbar(); pins = page.W.pinList();
function rect(n, top, height) {
  n.getClientRects = function () { return [1]; };
  n.getBoundingClientRect = function () { return { top: top, bottom: top + height, height: height, left: 0, right: 500, width: 500 }; };
}
rect(page.board, 0, 500); rect(bar, 0, 40); rect(A, 60, 40); rect(B, 120, 40);
var head = B.querySelector(".tbar"), e = { target: head, isPrimary: true, button: 0, pointerId: 1, pointerType: "mouse", clientX: 50, clientY: 130 };
page.board.fire("pointerdown", e);
page.doc.listeners.pointermove[0]({ pointerId: 1, pointerType: "mouse", buttons: 1, clientX: 50, clientY: 20, preventDefault: function () {} });
page.doc.listeners.pointerup[0]({ pointerId: 1, type: "pointerup" });
check("toolbar rejects drops", names(page.board), ["TOOLBAR", "PINS", "a", "b"]);
page.board.fire("pointerdown", e);
page.doc.listeners.pointermove[0]({ pointerId: 1, pointerType: "mouse", buttons: 1, clientX: 50, clientY: 50, preventDefault: function () {} });
page.doc.listeners.pointerup[0]({ pointerId: 1, type: "pointerup" });
check("toolbar and pin list are never flow peers", names(page.board), ["TOOLBAR", "PINS", "b", "a"]);
var saved = JSON.parse(page.store.cards).topics.filter(function (r) { return r.n === "b"; })[0];
check("head drop uses real topic time, not toolbar time", saved.a > 300, true);

boot({}, function () { return [mk("div.empty")]; });
check("empty board keeps toolbar and empty pins before placeholder", names(page.board), ["TOOLBAR", "PINS", "DIV"]);
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
		`search: function (e) { var b = $("tkqs"); if (b) { e.stopPropagation(); b.click(); } }`,
		`files: fpane && window.WsFiles ? function ()`,
		`if (!tabShown("convo")) setTab("convo");`,
		`setCtab("files");`,
		`if (filter) filter.focus();`,
		`tail.setAttribute("data-closed-more", String(Math.max(0, (+more.getAttribute("data-closed-more") || 0) - count)))`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing toolbar page hook %s", want)
		}
	}
}
