package wsdashboard

import (
	"os"
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
// changes nothing. A frame in no topic
// mounts at once, and without scroll margins everything mounts as before.
// The cases execute under node when it is installed.
func TestBoardFrameBands(t *testing.T) {
	src := dashboardFuncs(t, "boardBands", "frameNear", "bandBoard", "bandTopic", "topicBand", "unmountFrames", "unmountFrame",
		"frameShown", "mountFrame", "mountFrames", "mountFramesPaced", "mountSlice", "frameToggle")
	runNode(t, "viewport bands", frameDOM+src+`
var mountList = [], mountTask = false, NEAR = "100% 0px", FAR = "400% 0px", topicNear = {}, topicRO = null, frameSizes = new Map(), mpend = null;
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
// leaves the observer. The pinned column scrolls its own topics where it
// scrolls, else the board column does. The cases execute under node when
// it is installed.
func TestBoardTopicResizeKeepsTheView(t *testing.T) {
	src := dashboardFuncs(t, "topicResized", "topicSettling", "topicScroller")
	runNode(t, "topic resize", `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var unobserved = [], keeping = null;
var topicRO = { unobserve: function (t) { unobserved.push(t.name); } };
var window = { wbHistory: { keeping: function (s) { return s === keeping ? {} : null; } } };
function Scroller(name, top) { this.name = name; this.top = top; this.clientTop = 0; this.scrollTop = 1000; this.scrollHeight = 5000; this.clientHeight = 800; this.kids = []; }
Scroller.prototype.contains = function (t) { return this.kids.indexOf(t) >= 0; };
Scroller.prototype.getBoundingClientRect = function () { return { top: this.top }; };
var bcol = new Scroller("bcol", 50), pins = new Scroller("pins", 60), html = new Scroller("html", 0);
var doc = { scrollingElement: html };
function $(id) { return id === "bcol" ? bcol : null; }
function getComputedStyle(s) { return { overflowY: s.oy || "auto" }; }
// A topic's rect is its layout top less its scroller's scroll offset.
function Topic(name, sc, y, h, inPins) {
  this.name = name; this.sc = sc; this.y = y; this.h = h; this.isConnected = true; this.attrs = {}; this.inPins = inPins;
  bcol.kids.push(this); if (inPins) pins.kids.push(this);
}
Topic.prototype.hasAttribute = function (k) { return k in this.attrs; };
Topic.prototype.closest = function (sel) { return sel === ".bpins" && this.inPins ? pins : null; };
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
// The pinned column scrolls its own topics where it scrolls.
var pinned = new Topic("pinned", pins, 0, 200, true);
pins.scrollTop = 500;
grow(pinned, 200); grow(pinned, 260);
check("a pinned topic above the pinned column's view scrolls that column", pins.scrollTop === 560 && bcol.scrollTop === 900);
pins.scrollHeight = 100;
var inner = new Topic("inner", bcol, 0, 100, true);
grow(inner, 100); grow(inner, 150);
check("a pin list that does not scroll leaves it to the board column", bcol.scrollTop === 950 && pins.scrollTop === 560);
if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardTopicsDoNotSkipRendering pins owner (627)=a: board topics no
// longer take content-visibility, so no stylesheet rule may skip a topic's
// rendering. Overview's `content-visibility: visible` overrides
// (261114-feat-ws-dashboard-board-toolbar-overview) are harmless and stay.
func TestBoardTopicsDoNotSkipRendering(t *testing.T) {
	css, err := os.ReadFile("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	for _, banned := range []string{"content-visibility: auto", "content-visibility: hidden", "data-wbcv"} {
		if strings.Contains(s, banned) {
			t.Errorf("dashboard.css still holds %q; board topics must not skip rendering (627)", banned)
		}
	}
}
