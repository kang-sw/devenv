package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestBoardFrameBands pins the viewport bands (dashboard.js frameNear,
// bandBoard, bandTopic, topicBand, unmountFrames, unmountFrame): where the
// view kit has scroll margins, each board topic is watched in a NEAR band
// (100%) and a FAR band (400%) through wbHistory.watch, once each. A
// topic's shown frames mount, paced, when it reports NEAR, and not before
// its first report (unless a topic of its name was last near, as a changed
// topic's new section is). Leaving FAR turns its frames back into
// placeholders that keep their attributes and height (a pending height
// included) and a swap's first-height marks (wbKeep, wbGrow), except the frames the page holds (a graft's pair, the focused
// frame, a held selection, the mention button's frame); coming NEAR again
// remounts them; a frame mounted with no height yet, or mounted again, is
// marked for its first height (wbFirst, topicResized). A report with no box
// changes nothing. While the narrow view's Main Board stands off the view
// behind a docked page (data-gal-away) leaving FAR keeps the frames. A frame in no topic
// mounts at once, and without scroll margins everything mounts as before.
// The cases execute under node when it is installed.
func TestBoardFrameBands(t *testing.T) {
	src := dashboardFuncs(t, "boardBands", "frameNear", "bandBoard", "bandTopic", "topicBand", "unmountFrames", "unmountFrame",
		"frameShown", "mountFrame", "mountFrames", "mountFramesPaced", "mountSlice", "frameToggle", "cvWatch")
	runNode(t, "viewport bands", frameDOM+src+`
var root = { away: false, hasAttribute: function (k) { return k === "data-gal-away" && root.away; } };
var mountList = [], mountTask = false, NEAR = "100% 0px", FAR = "400% 0px", topicNear = {}, topicRO = null, frameSizes = new Map(), mpend = null, cvRO = null;
var clock = 0, timers = [];
var performance = { now: function () { return clock; } };
setTimeout = function (fn) { timers.push(fn); };
function drain() { while (timers.length) timers.shift()(); }
var subs = [];
window.wbHistory = { bands: true, watch: function (n, o) { var s = { n: n, margin: o.margin, onChange: o.onChange }; subs.push(s); return s; } };
function report(t, margin, visible, box) {
  var r = box === false ? { width: 0, height: 0 } : { width: 100, height: 50 };
  subs.forEach(function (s) { if (s.n === t && s.margin === margin) s.onChange({ node: t, ratio: visible ? 1 : 0, visible: visible, rect: r }); });
  drain();
}
doc.addEventListener("toggle", frameToggle, true);

function topic(name, kids) { return new El("section", { "class": "topic", "data-topic": name }, kids); }
var A1 = frame("topic:a"), A2 = frame("entry:a1"), B1 = frame("topic:b"), C1 = frame("topic:c"), K1 = frame("card:k");
var cfold = new El("details", { "class": "tdoc" }, [C1]);
var A = topic("a", [A1, new El("details", { open: "" }, [A2])]), B = topic("b", [B1]), C = topic("c", [cfold]);
var board = new El("div", { id: "r-board" }, [A, B, C, new El("div", { "data-card": "k" }, [K1])]);
doc.appendChild(board);

bandBoard(board);
check("each topic is watched once in each band: " + subs.map(function (s) { return s.n.getAttribute("data-topic") + "@" + s.margin; }),
  subs.length === 6 && [A, B, C].every(function (t) { return subs.filter(function (s) { return s.n === t; }).map(function (s) { return s.margin; }).join() === NEAR + "," + FAR; }));
bandBoard(board);
check("a topic is banded once", subs.length === 6);

mountFramesPaced(doc); drain();
check("a topic not reported yet mounts nothing", [A1, A2, B1, C1].every(placeholder));
check("a frame in no topic mounts at once", mounted(K1));

report(B, NEAR, true, false);
check("a report with no box mounts nothing", placeholder(B1));
report(A, NEAR, true);
check("a topic coming near mounts its shown frames", mounted(A1) && mounted(A2));
report(C, NEAR, true);
check("a frame under a closed fold stays a placeholder", placeholder(C1));
report(B, NEAR, false); report(B, FAR, true);
check("a topic outside NEAR mounts nothing", placeholder(B1));
cfold.open = true; tick();
check("a toggle open in a near topic mounts", mounted(C1));

// Leaving FAR unmounts, the height and attributes kept.
var fa1 = el(A1), fa2 = el(A2);
fa1.setAttribute("style", "height: 120px"); fa1.setAttribute("data-relay", "/s/r/x");
frameSizes.set(fa2, 77.2);
fa1.wbKeep = true; fa2.wbGrow = true;
report(A, NEAR, false);
check("between the bands a topic keeps its frames", el(A1) === fa1 && el(A2) === fa2);
report(A, FAR, false);
var pa1 = fa1.parentNode ? null : A.childNodes[0], pa2 = A.childNodes[1].childNodes[0];
check("leaving FAR turns the frames back into placeholders", pa1 && pa1.tagName === "DIV" && pa2.tagName === "DIV" && !fa1.parentNode && !fa2.parentNode);
check("the placeholder keeps the frame's attributes: " + JSON.stringify(pa1.attrs), pa1.getAttribute("style") === "height: 120px" && pa1.getAttribute("data-relay") === "/s/r/x" &&
  pa1.getAttribute("data-srcdoc") === A1.getAttribute("data-srcdoc") && pa1.getAttribute("data-sandbox") === SANDBOX && pa1.getAttribute("data-doc-key") === "topic:a" &&
  !pa1.hasAttribute("srcdoc") && !pa1.hasAttribute("sandbox"));
check("a height not landed yet goes to the placeholder", pa2.style.height === "78px" && !frameSizes.has(fa2));
check("a swap's first-height marks go to the placeholder", pa1.wbKeep === true && pa2.wbGrow === true && !pa1.wbGrow && !pa2.wbKeep);
// The narrow view's Main Board off the view behind a docked page keeps
// its frames: switching back to it reloads nothing.
report(A, NEAR, true);
var away1 = el(pa1);
root.away = true; report(A, NEAR, false); report(A, FAR, false); root.away = false;
check("the Main Board off the view keeps its frames", el(pa1) === away1 && away1.parentNode);
report(A, NEAR, true);
check("coming near again remounts them", mounted(pa1) && mounted(pa2) && el(pa1).getAttribute("style") === "height: 120px");
check("a frame mounted with no height yet is marked for its first height", fa1.wbFirst === true);
check("a frame mounted again is marked for its first height", pa1.wbUnmounted === true && el(pa1).wbFirst === true && el(pa2).wbFirst === true);
var P1 = frame("topic:p"); P1.style.height = "50px";
check("a frame mounted at a height a swap gave it is not", !mountFrame(P1).wbFirst);
check("the frame mounted again takes the swap's first-height marks", el(pa1).wbKeep === true && el(pa2).wbGrow === true);

// Frames the page holds stay mounted.
var held = [el(pa1), el(pa2), el(C1)];
el(pa1).wbOut = true; el(pa2).wbHoldSelection = true;
doc.activeElement = el(C1);
report(A, FAR, false); report(C, FAR, false);
check("held frames stay mounted", held.every(function (f) { return f.parentNode; }));
doc.activeElement = null; mpend = { frame: el(C1) };
report(C, FAR, false);
check("the mention button's frame stays mounted", held[2].parentNode);
mpend = null;
report(C, FAR, false);
check("a frame no longer held unmounts", !held[2].parentNode);
var gn = mountFrame(frame("topic:c")); gn.wbNext = true; C.appendChild(gn);
report(C, FAR, false);
check("a graft's pending frame stays mounted", gn.parentNode === C);

// A changed topic's new section takes the last report for its name.
var A3 = frame("topic:a"), An = topic("a", [A3]), E1 = frame("topic:e"), En = topic("e", [E1]);
board.appendChild(An); board.appendChild(En);
mountFrames(An); mountFrames(En);
check("a new section of a near topic mounts at once", mounted(A3));
check("a new topic waits for its first report", placeholder(E1));
check("frameNear bands a topic it meets", subs.filter(function (s) { return s.n === En; }).length === 2);
report(B, NEAR, false);
var B2 = frame("topic:b"), Bn = topic("b", [B2]);
board.appendChild(Bn); mountFrames(Bn);
check("a new section of a topic last outside NEAR waits", placeholder(B2));

// Without scroll margins everything mounts as before.
window.wbHistory.bands = false;
var F1 = frame("topic:f"), F = topic("f", [F1]), n0 = subs.length;
board.appendChild(F); bandBoard(F); mountFrames(F);
check("without bands a shown frame mounts at once, unbanded", mounted(F1) && subs.length === n0);
if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardTopicResizeKeepsTheView pins the above-view compensation
// (dashboard.js topicResized, topicSettling, topicScroller): a topic
// standing wholly above its scroller's view whose size the bands changed (a
// frame mounted with no height whose first height landed, counted once) scrolls the
// scroller by the change outside a keep session; any other change (a frame
// already shown reporting a new height), a topic in view or below, a
// maximized topic, a topic's first size, and any change while a keep
// session runs on the scroller move nothing; a topic taken off the page
// leaves the observer. A docked topic (cards.js) scrolls in its pane's own
// body (.gal-pbody), which it fills, so its size changes move no scroller;
// a board-only page compensates the document scroller. The cases execute under node when
// it is installed.
func TestBoardTopicResizeKeepsTheView(t *testing.T) {
	src := dashboardFuncs(t, "topicResized", "topicSettling", "topicScroller")
	runNode(t, "topic resize", `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var unobserved = [], keeping = null, shHidden = false;
var topicRO = { unobserve: function (t) { unobserved.push(t.name); } };
var window = { wbHistory: { keeping: function (s) { return s === keeping ? {} : null; } } };
function Scroller(name, top) { this.name = name; this.top = top; this.clientTop = 0; this.scrollTop = 1000; this.scrollHeight = 5000; this.clientHeight = 800; this.kids = []; }
Scroller.prototype.contains = function (t) { return this.kids.indexOf(t) >= 0; };
Scroller.prototype.getBoundingClientRect = function () { return { top: this.top }; };
var bcol = new Scroller("bcol", 50), html = new Scroller("html", 0);
var doc = { scrollingElement: html };
function $(id) { return id === "bcol" ? bcol : null; }
function getComputedStyle(s) { return { overflowY: s.oy || "auto" }; }
// A topic's rect is its layout top less its scroller's scroll offset.
function Topic(name, sc, y, h) {
  this.name = name; this.sc = sc; this.y = y; this.h = h; this.isConnected = true; this.attrs = {}; this.children = [];
  bcol.kids.push(this);
}
Topic.prototype.hasAttribute = function (k) { return k in this.attrs; };
Topic.prototype.getBoundingClientRect = function () {
  var top = (this.sc === html ? 0 : this.sc.top) + this.y - this.sc.scrollTop;
  return { top: top, bottom: top + this.h, height: this.h };
};
function q(sel, t) { if (sel !== "iframe.doc-frame") throw new Error("selector not modelled: " + sel); return t.frames || []; }
// grow resizes topic t as the bands do: a frame they mounted whose first
// height has landed as a change (wbLanded, applyFrameHeights); resize as
// anything else does.
function resize(t, h) { t.h = h; topicResized([{ target: t, borderBoxSize: [{ blockSize: h }] }]); }
function grow(t, h) { t.frames = (t.frames || []).concat([{ wbLanded: true }]); resize(t, h); }
`+src+`
var above = new Topic("above", bcol, 100, 300), inview = new Topic("inview", bcol, 900, 400), below = new Topic("below", bcol, 3000, 300);
[above, inview, below].forEach(function (t) { grow(t, t.h); });
check("a first size moves nothing", bcol.scrollTop === 1000);
grow(above, 500);
check("a topic wholly above the view scrolls by its change: " + bcol.scrollTop, bcol.scrollTop === 1200);
check("a first height counts once", above.frames.every(function (f) { return !f.wbLanded; }));
resize(above, 600);
check("a frame already shown growing above moves the view as before", bcol.scrollTop === 1200);
var pending = { wbFirst: true };
above.frames.push(pending); resize(above, 500);
check("a frame whose first height has not landed counts nothing", bcol.scrollTop === 1200 && pending.wbFirst && !pending.wbLanded);
above.frames.pop();
grow(inview, 600); grow(below, 900);
check("a topic in view or below moves nothing", bcol.scrollTop === 1200);
// After the compensation the above topic sits at 100 - 1200 + 50: still above.
grow(above, 200);
check("a shrinking topic above the view scrolls back: " + bcol.scrollTop, bcol.scrollTop === 900);
keeping = bcol;
grow(above, 400);
check("a keep session compensates by itself", bcol.scrollTop === 900);
keeping = null;
above.attrs["data-max"] = "";
grow(above, 100);
check("a maximized topic moves nothing", bcol.scrollTop === 900);
delete above.attrs["data-max"];
above.isConnected = false;
grow(above, 0);
check("a removed topic leaves the observer", unobserved.join() === "above" && bcol.scrollTop === 900);
// A docked topic scrolls in its own body, which it fills: its size
// changes move neither that body nor the board column.
window.WsCards = { isDocked: function (t) { return !!t.docked; } };
var pane = new Scroller("pane", 60), body = new Scroller("pbody", 90);
pane.scrollTop = 0; body.scrollTop = 500;
body.classList = { contains: function (c) { return c === "gal-pbody"; } };
var docked = new Topic("docked", pane, 0, 200);
docked.docked = true; docked.children = [body];
grow(docked, 200); grow(docked, 260);
check("a docked topic's resize moves neither its body nor the board column", body.scrollTop === 500 && bcol.scrollTop === 900);
window.WsCards.isDocked = function () { return false; };
var inner = new Topic("inner", bcol, 0, 100);
grow(inner, 100); grow(inner, 150);
check("a topic not docked is the board column's", bcol.scrollTop === 950 && body.scrollTop === 500);
// Board-only layout: bcol is not an independent scroller. Its document
// must receive exactly one height delta (native anchoring is off in CSS).
bcol.oy = "visible";
var docAbove = new Topic("doc-above", html, 100, 200);
grow(docAbove, 200); grow(docAbove, 673);
check("an above-view mount compensates the document by one frame height", html.scrollTop === 1473 && bcol.scrollTop === 950);
if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardTopicSkipKeepsHeadsAndSheets pins where the board may skip
// rendering. Owner (627)=a dropped content-visibility on whole topics; the
// owner's 2026-11-17 visible-only directive brings it back on topic parts
// only: a topic's head (sticky, carries the stuck shadow) and a card's head
// never skip, nor does a maximized topic or a card holding focus, and
// overview renders everything.
func TestBoardTopicSkipKeepsHeadsAndSheets(t *testing.T) {
	css, err := dashboardTestAsset("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	if strings.Contains(s, "content-visibility: hidden") {
		t.Error("dashboard.css holds content-visibility: hidden")
	}
	rules := regexp.MustCompile(`\}([^{}]+)\{[^{}]*content-visibility: auto[^{}]*\}`).FindAllStringSubmatch(s, -1)
	if len(rules) == 0 {
		t.Fatal("board topic parts take no content-visibility: auto")
	}
	for _, r := range rules {
		for _, sel := range strings.Split(r[1], ",") {
			sel = strings.TrimSpace(sel)
			if !strings.Contains(sel, ":not([data-max])") {
				t.Errorf("%q skips a maximized topic or card", sel)
			}
			// A topic part takes no focus guard: a focused part is relevant to
			// the user and renders anyway, and the guard on the topic flipped
			// every part at once as focus entered it (a suspected Edge renderer crash).
			card := strings.HasSuffix(sel, "> .bc-body")
			if card != strings.Contains(sel, ":not(:focus-within)") {
				t.Errorf("%q: only a card body takes the focus guard", sel)
			}
			if !strings.Contains(sel, ":not(.tophead)") && !strings.HasSuffix(sel, "> .bc-body") {
				t.Errorf("%q may skip a head", sel)
			}
		}
	}
	for _, want := range []string{
		"section.topic[data-topic] > :not(.tophead) { contain-intrinsic-block-size: auto",
		":root[data-overview] .r-board :is(section.topic > :not(.tophead), .bcard .bc-body) { content-visibility: visible !important; }",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}
