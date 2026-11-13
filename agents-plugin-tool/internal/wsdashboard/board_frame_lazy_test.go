package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// dashboardFuncs returns the named top-level functions of dashboard.js, in
// order, for a node harness.
func dashboardFuncs(t *testing.T, names ...string) string {
	t.Helper()
	board, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := ""
	for _, name := range names {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(string(board))
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	return src
}

// frameDOM is a minimal DOM for the lazy frame cases: elements with
// attributes, children, an open property on details that queues a toggle
// event the way a browser does (asynchronously, at the details), and q over
// the few selectors the functions under test use. An iframe records whether
// it was connected when its sandbox and srcdoc were set, and one connected
// with no srcdoc commits about:blank at once (blank).
const frameDOM = `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var queue = [], SANDBOX = "allow-scripts allow-popups allow-popups-to-escape-sandbox";
function tick() { while (queue.length) queue.shift()(); }
function El(tag, attrs, kids) {
  this.tagName = tag.toUpperCase(); this.nodeType = 1; this.parentNode = null; this.childNodes = [];
  this.attrs = {}; this._open = false; this.listeners = {}; this.setLive = {}; this.style = {};
  var self = this;
  Object.keys(attrs || {}).forEach(function (k) { self.attrs[k] = attrs[k]; });
  if (this.tagName === "DETAILS") this._open = this.attrs.open !== undefined;
  (kids || []).forEach(function (k) { self.appendChild(k); });
}
El.prototype = {
  get children() { return this.childNodes; },
  get attributes() { var a = this.attrs; return Object.keys(a).map(function (k) { return { name: k, value: a[k] }; }); },
  get isConnected() { for (var n = this; n; n = n.parentNode) if (n === doc) return true; return false; },
  get open() { return this._open; },
  set open(v) {
    v = !!v;
    if (v === this._open) return;
    this._open = v;
    var d = this;
    queue.push(function () { doc.dispatch("toggle", d); });
  },
  get classList() {
    var self = this;
    function list() { return (self.attrs["class"] || "").split(/\s+/).filter(Boolean); }
    return {
      contains: function (c) { return list().indexOf(c) >= 0; },
      add: function (c) { var l = list(); if (l.indexOf(c) < 0) l.push(c); self.attrs["class"] = l.join(" "); },
      remove: function (c) { self.attrs["class"] = list().filter(function (x) { return x !== c; }).join(" "); }
    };
  },
  getAttribute: function (k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; },
  hasAttribute: function (k) { return Object.prototype.hasOwnProperty.call(this.attrs, k); },
  setAttribute: function (k, v) { this.attrs[k] = String(v); this.setLive[k] = this.isConnected; },
  removeAttribute: function (k) { delete this.attrs[k]; },
  addEventListener: function (type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); },
  removeChild: function (c) { this.childNodes.splice(this.childNodes.indexOf(c), 1); c.parentNode = null; return c; },
  insertBefore: function (c, ref) {
    if (c.parentNode) c.parentNode.removeChild(c);
    var i = ref ? this.childNodes.indexOf(ref) : this.childNodes.length;
    this.childNodes.splice(i, 0, c); c.parentNode = this;
    // A frame put on the page with no document loads about:blank at once.
    if (c.tagName === "IFRAME" && c.isConnected && !c.hasAttribute("srcdoc")) { c.blank = true; (c.listeners.load || []).forEach(function (fn) { fn(); }); }
    return c;
  },
  replaceChild: function (c, old) { this.insertBefore(c, old); return this.removeChild(old); },
  appendChild: function (c) { return this.insertBefore(c, null); },
  scrollIntoView: function () {},
  // closest models a single class selector (boardJump's ".wb-history")
  // and the selectors of SEL.
  closest: function (sel) {
    for (var n = this; n && n.nodeType === 1; n = n.parentNode) if (n.matches(sel)) return n;
    return null;
  },
  matches: function (sel) {
    if (SEL[sel]) return SEL[sel](this);
    if (!/^\.[\w-]+$/.test(sel)) throw new Error("selector not modelled: " + sel);
    return this.classList.contains(sel.slice(1));
  },
  dispatchEvent: function (e) { (this.listeners[e.type] || []).forEach(function (fn) { fn(e); }); }
};
function walk(root, out) { root.childNodes.forEach(function (c) { out.push(c); walk(c, out); }); return out; }
var SEL = {
  "div.doc-frame[data-srcdoc]": function (n) { return n.tagName === "DIV" && n.classList.contains("doc-frame") && n.hasAttribute("data-srcdoc"); },
  "iframe.doc-frame[data-srcdoc]": function (n) { return n.tagName === "IFRAME" && n.classList.contains("doc-frame") && n.hasAttribute("data-srcdoc"); },
  "section.topic[data-topic]": function (n) { return n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic"); },
  "details": function (n) { return n.tagName === "DETAILS"; },
  "details[data-key]": function (n) { return n.tagName === "DETAILS" && n.hasAttribute("data-key"); }
};
function q(sel, root) {
  var m = SEL[sel];
  if (!m) throw new Error("selector not modelled: " + sel);
  return walk(root || doc, []).filter(m);
}
var doc = new El("html");
doc.nodeType = 9;
doc.capture = {};
doc.createElement = function (tag) { return new El(tag); };
doc.addEventListener = function (type, fn, capture) { if (capture) (doc.capture[type] = doc.capture[type] || []).push(fn); };
doc.dispatch = function (type, target) { (doc.capture[type] || []).forEach(function (fn) { fn({ target: target, type: type }); }); };
// frame is a served placeholder (wswhiteboard framedDoc).
function frame(key, text) { return new El("div", { "class": "doc-frame", title: key, "data-doc": key, "data-doc-key": key, "data-sandbox": SANDBOX, "data-srcdoc": text || "<!DOCTYPE html><html>" + key }); }
// el is what stands for placeholder p now: p itself, or the frame mounted
// in its place (found by data-doc in p's old parent).
function el(p) {
  if (p.parentNode || p.tagName === "IFRAME") return p;
  var all = walk(doc, []).filter(function (n) { return n.tagName === "IFRAME" && n.getAttribute("data-doc") === p.getAttribute("data-doc"); });
  return all[all.length - 1] || p;
}
// mounted: an iframe stands in p's place, sandboxed and given its document
// before it was connected, never committing about:blank, data-srcdoc kept.
function mounted(p) {
  var f = el(p);
  return f.tagName === "IFRAME" && f.getAttribute("srcdoc") === p.getAttribute("data-srcdoc") && f.getAttribute("data-srcdoc") === p.getAttribute("data-srcdoc") &&
    f.getAttribute("sandbox") === SANDBOX && !f.hasAttribute("data-sandbox") && f.setLive.sandbox === false && f.setLive.srcdoc === false && !f.blank;
}
function placeholder(p) { return el(p) === p && p.tagName === "DIV" && !p.hasAttribute("srcdoc"); }
var window = {};
function setTimeout() {}
function requestAnimationFrame() {}
`

// TestBoardFramesMountLazily pins the lazy frame mount (wswhiteboard
// framedDoc's placeholder, dashboard.js mountFrame and mountFrames): a
// placeholder becomes its frame only once every details around it is open,
// on the page's first pass (a pinned topic's frame in the pin list
// included), on a toggle open whether the user's or a programmatic one
// (restoreOpen after a swap, boardJump), and for what a swap inserts; one
// under a closed fold or off the page stays a placeholder. The mounted frame
// takes the placeholder's attributes, the exact sandbox token set and its
// srcdoc before it is connected (it never commits about:blank), and keeps
// data-srcdoc for comparisons. A changed topic's next body frame (frameGraft)
// is mounted before it goes in when the old frame was mounted, so it never
// shows blank; the next of an unmounted one, or of one in a topic skipping
// its rendering (content-visibility), takes its place as a placeholder;
// the kept section keeps its content-visibility mark (data-wbcv).
// Under the viewport bands a jump keeps its target in place (wbHistory.keep).
// The cases execute under node when it is installed.
func TestBoardFramesMountLazily(t *testing.T) {
	src := dashboardFuncs(t, "boardBands", "frameNear", "frameShown", "mountFrame", "mountFrames", "frameToggle", "isPeer", "isPanel", "openKey", "restoreOpenOne", "restoreOpen", "boardJump", "frameGraft", "frameNext")
	runNode(t, "lazy frame mount", frameDOM+src+`
var openState = {}, PEER_KEY = "p.";
function load() { return null; }
function shutId() { return ""; }
function shutHas() { return false; }
doc.addEventListener("toggle", frameToggle, true);

var A = frame("topic:x"), B = frame("entry:e1"), C = frame("entry:e2"), D = frame("entry:e3"), P = frame("topic:pinned");
B.setAttribute("data-entry", "e1"); B.setAttribute("data-relay", "/s/r/out?p=a"); B.setAttribute("style", "height: 40px");
var tdoc = new El("details", { "class": "tdoc", "data-key": "doc:x" }, [new El("div", {}, [A])]);
var versions = new El("details", { "class": "panel-versions", "data-key": "versions:e1" }, [new El("article", { id: "e2" }, [new El("details", { open: "" }, [C])])]);
var stream = new El("details", { "class": "topic-stream", open: "" }, [new El("article", {}, [new El("details", { "class": "entry-panel", open: "" }, [B, versions])])]);
var board = new El("div", { id: "r-board" }, [new El("section", { "class": "topic", "data-topic": "x" }, [tdoc, stream])]);
var pins = new El("div", { "class": "pins" }, [new El("section", { "class": "topic", "data-topic": "pinned" }, [P])]);
var inert = new El("template", {}, [D]);
doc.appendChild(board); doc.appendChild(pins);

check("served frames start as placeholders", [A, B, C, D, P].every(placeholder));
mountFrames(doc);
check("an open chain mounts", mounted(B));
var fb = el(B);
check("the frame takes the placeholder's attributes: " + JSON.stringify(fb.attrs), fb.getAttribute("class") === "doc-frame" && fb.getAttribute("title") === "entry:e1" && fb.getAttribute("data-doc-key") === "entry:e1" &&
  fb.getAttribute("data-entry") === "e1" && fb.getAttribute("data-relay") === "/s/r/out?p=a" && fb.getAttribute("style") === "height: 40px");
check("the frame stands where its placeholder stood", fb.parentNode && fb.parentNode.childNodes[0] === fb);
check("a pinned topic's frame mounts in the pin list", mounted(P));
check("a frame under a closed tdoc stays a placeholder", placeholder(A));
check("a frame under a closed versions fold stays a placeholder", placeholder(C));
check("a frame off the page stays a placeholder", placeholder(D));

// A user's toggle open.
tdoc.open = true; tick();
check("a toggle open mounts", mounted(A));
var fa = el(A);
tdoc.open = false; tick(); tdoc.open = true; tick();
check("a mounted frame is not mounted again", el(A) === fa);

// restoreOpen after a swap opens a fold programmatically: its toggle mounts.
var R = frame("entry:e4"), rfold = new El("details", { "data-key": "run:e4" }, [R]);
board.appendChild(new El("section", { "class": "topic", "data-topic": "y" }, [rfold]));
openState["k:run:e4"] = true;
restoreOpen(board); tick();
check("restoreOpen's open mounts", mounted(R));

// boardJump opens every fold up to its target: its toggles mount.
window.wbHistory = null;
function $(id) { return id === "e2" ? versions.childNodes[0] : null; }
function inBoard(n) { for (; n; n = n.parentNode) if (n === board) return true; return false; }
function boardBoxes() { return [board]; }
var hit = boardJump("p1");
check("boardJump refuses an id it cannot find", hit === undefined && placeholder(C));
$ = function (id) { return id === "p2" ? Object.assign(versions.childNodes[0], { id: "p2" }) : null; };
boardJump("p2"); tick();
check("boardJump's opens mount", mounted(C));
// Under the viewport bands a jump keeps its target where it landed while
// the frames around it mount; without them (no bands) it starts no session.
var kept = [];
window.wbHistory = { loadTarget: function () { return null; }, keep: function (n) { kept.push(n); } };
boardJump("p2");
check("a jump without bands keeps nothing", kept.length === 0);
window.wbHistory.bands = true;
boardJump("p2");
check("a jump under the bands keeps its target", kept.length === 1 && kept[0] === versions.childNodes[0]);
window.wbHistory = null;

// A swap's inserted section mounts what it shows.
var N1 = frame("entry:e5"), N2 = frame("entry:e6");
var n = new El("section", { "class": "topic", "data-topic": "z" }, [new El("details", { open: "" }, [N1]), new El("details", {}, [N2])]);
mountFrames(n);
check("a section off the page mounts nothing", placeholder(N1));
board.appendChild(n); mountFrames(n);
check("an inserted section mounts its open frames", mounted(N1) && placeholder(N2));

// frameGraft: the next body frame of a mounted frame is mounted before it
// goes in; the next frame of an unmounted one takes its place unmounted.
function graft(oldMounted, skipped) {
  var of = frame("topic:g", "<!DOCTYPE html><html>old"), nf = frame("topic:g", "<!DOCTYPE html><html>new");
  if (oldMounted) of = mountFrame(of);
  var od = new El("div", { "class": "doc-body framed" }, [of]), nd = new El("div", { "class": "doc-body framed" }, [nf]);
  var old = new El("section", { "class": "topic", "data-topic": "g", "data-wbcv": "", "data-old": "" }, [od]), nsec = new El("section", { "class": "topic", "data-topic": "g" }, [nd]);
  old.wbSkipped = !!skipped;
  board.appendChild(old);
  var g = { old: old, n: nsec, oc: [of, od, old], nc: [nf, nd, nsec] };
  frameGraft(g);
  return { nf: nf, f: g.nc[0], od: od, of: of, old: old };
}
var frameShow = function () {}, frameRevealMs = 1500;
function frameReveal() {}
var g1 = graft(true);
check("the next frame of a mounted frame goes in mounted, beside the old", mounted(g1.nf) && g1.od.childNodes[0] === g1.of && g1.od.childNodes[1] === g1.f);
check("the next frame is pending, and the graft holds the frame, not the placeholder", g1.f.wbNext === true && g1.f.wbOld === g1.of && g1.of.wbOut === true && (g1.f.listeners.load || []).length === 1 && g1.nf.wbNext === undefined);
check("the kept section keeps its content-visibility mark and drops other old attributes", g1.old.hasAttribute("data-wbcv") && !g1.old.hasAttribute("data-old"));
var g2 = graft(false);
check("the next frame of an unmounted frame takes its place as a placeholder", placeholder(g2.nf) && g2.od.childNodes.length === 1 && g2.od.childNodes[0] === g2.nf && !g2.nf.wbNext);
// In a topic skipping its rendering a new frame would load at no size: the
// next frame of a mounted one there takes its place as a placeholder too.
var g3 = graft(true, true);
check("the next frame in a skipped topic takes the mounted frame's place as a placeholder", placeholder(g3.nf) && g3.od.childNodes.length === 1 && g3.od.childNodes[0] === g3.nf && !g3.nf.wbNext && !g3.of.parentNode);

if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardFramesMountAtEveryInsert pins where the page runs the mount
// pass: once after the cards have moved the pinned topics (so no frame
// loads and then moves) and the board's script set up wbHistory (wbForm,
// whose visibility source the bands use), for every node a board swap inserts after its
// frame heights are restored, for an adopted closed page, and for a
// history load; the toggle listener is a capture listener. Every place that
// puts topics under the shared visibility source puts them under the
// viewport bands too (bandBoard).
func TestBoardFramesMountAtEveryInsert(t *testing.T) {
	raw, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	board := string(raw)
	mount, first := strings.Index(board, "window.WsCards.mount(cardBoard"), strings.Index(board, "\n  mountFramesPaced(doc);\n")
	if mount < 0 || first < 0 || first < mount {
		t.Errorf("the first mount pass must follow WsCards.mount (cards %d, pass %d)", mount, first)
	}
	for what, pattern := range map[string]string{
		"swap insert":    `setFrameHeights\(n, frameH\);\n\s+mountFrames\(n\);`,
		"adopted page":   `adopted: function \(n\) \{\n[^}]*restoreOpen\(n\);\n\s+mountFrames\(n\);`,
		"history load":   `"wbhistoryload", function \(e\) \{\n[^}]*restoreOpen\(el\);\n\s+mountFrames\(el\);`,
		"toggle capture": `doc\.addEventListener\("toggle", frameToggle, true\);`,
		"bands at load":  `if \(window\.wbForm\) window\.wbForm\(\);\n(\s+//[^\n]*\n)*\s+boardBoxes\(\)\.forEach\(bandBoard\);\n\s+mountFramesPaced\(doc\);`,
		"bands at swap":  `ins\.forEach\(watchBoard\); \}\n\s+ins\.forEach\(bandBoard\);`,
		"bands adopted":  `adopted: function \(n\) \{\n[^}]*watchBoard\(n\);\n\s+bandBoard\(n\);`,
		"bands closed":   `watchBoard\(page\);\n\s+bandBoard\(page\);`,
	} {
		if !regexp.MustCompile(pattern).MatchString(board) {
			t.Errorf("dashboard.js: no mount pass at the %s (%s)", what, pattern)
		}
	}
}

// TestBoardFrameHeightsBatch pins the frame height path (dashboard.js's
// frame message listener, applyFrameHeights, docFrame): N height messages from the
// board's frames resolve their frames through one index, not a document
// query each, and land in one frame callback that never reads layout
// (getBoundingClientRect); a frame with no set height eases from 0, as the
// stylesheet starts it, and one with a height eases from that height; a
// pending next frame (frameGraft) keeps its height; a frame the viewport
// bands mounted (wbFirst) marks its first height as landed (wbLanded,
// topicResized) only when that height changes it. The cases execute under
// node when it is installed.
func TestBoardFrameHeightsBatch(t *testing.T) {
	src := dashboardFuncs(t, "indexFrames", "docFrame", "queueFrameHeight", "applyFrameHeights")
	raw, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n[\s\S]*?\n  \}\);\n`).FindString(string(raw))
	if listener == "" {
		t.Fatal("dashboard.js holds no frame message listener")
	}
	runNode(t, "frame height batch", `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var raf = [], queries = 0, themes = 0;
function requestAnimationFrame(fn) { raf.push(fn); return raf.length; }
function num(v) { return typeof v === "number" && isFinite(v) ? v : 0; }
function str(v) { return typeof v === "string" ? v : ""; }
function postTheme() { themes++; }
var frames = [];
function Frame(key) {
  this.contentWindow = { id: key };
  this.attrs = { "data-doc": key, "data-doc-key": key };
  this.style = { height: "" };
  this.anims = [];
}
Frame.prototype.getAttribute = function (k) { return this.attrs[k] || null; };
Frame.prototype.getBoundingClientRect = function () { bad.push("a height message read layout"); return { height: 0 }; };
Frame.prototype.animate = function (keys) { this.anims.push(keys.map(function (k) { return k.height; }).join(">")); };
function boardQ(sel) { if (sel === "iframe.doc-frame") queries++; return frames.slice(); }
function inBoard(f) { return frames.indexOf(f) >= 0; }
var frameWins = new Map(), frameSizes = new Map(), frameSizeFrame = 0, frameMessage;
var board = { contains: function (f) { return frames.indexOf(f) >= 0; } }, keeps = [], applied = 0;
function region(name) { return name === "board" ? board : null; }
function boardKept() { return null; }
function boardKeep(b) { keeps.push(frames.slice(7, 10).map(function (f) { return f.style.height; }).join()); return { h: { apply: function () { applied++; } } }; }
var window = { addEventListener: function (t, fn) { if (t === "message") frameMessage = fn; } };
`+src+listener+`
for (var i = 0; i < 20; i++) frames.push(new Frame("entry:e" + i));
frames[0].wbGrow = true;
frames[1].wbGrow = true; frames[1].style.height = "40px";
for (var i = 0; i < 20; i++) frameMessage({ source: frames[i].contentWindow, data: { wbDoc: "height", docKey: "entry:e" + i, height: 100 + i + 0.5 } });
// A second report from a frame before the batch lands: the last one wins.
frameMessage({ source: frames[2].contentWindow, data: { wbDoc: "height", docKey: "entry:e2", height: 300 } });
check("20 frames resolve through one index, got " + queries + " queries", queries === 1);
check("21 messages schedule one frame callback, got " + raf.length, raf.length === 1);
check("no height lands before the frame callback", frames.every(function (f) { return f.style.height === "" || f === frames[1]; }));
raf.shift()();
check("every height lands in the one callback", frames.every(function (f, i) { return f.style.height === (i === 2 ? "300px" : Math.ceil(100 + i + 0.5) + "px"); }));
check("a frame with no set height eases from 0: " + frames[0].anims, frames[0].anims.join() === "0px>101px");
check("a frame with a set height eases from it: " + frames[1].anims, frames[1].anims.join() === "40px>102px");
check("only a moving swap's frames ease", frames.slice(2).every(function (f) { return !f.anims.length; }) && !frames[0].wbGrow);
check("every frame takes the theme with its height", themes === 20);
// A later message schedules a new callback.
frameMessage({ source: frames[3].contentWindow, data: { wbDoc: "height", docKey: "entry:e3", height: 50 } });
check("a later message schedules its own callback", raf.length === 1);
// A pending next frame keeps the old frame's height.
frames[4].wbNext = true;
frameMessage({ source: frames[4].contentWindow, data: { wbDoc: "height", docKey: "entry:e4", height: 999 } });
raf.shift()();
check("a pending frame keeps its height", frames[4].style.height === "105px" && frames[3].style.height === "50px");
// A frame that took a new window (moved without moveBefore) is found again.
frames[5].contentWindow = { id: "moved" };
frameMessage({ source: frames[5].contentWindow, data: { wbDoc: "height", docKey: "entry:e5", height: 77 } });
raf.shift()();
check("a moved frame is indexed again on a miss", frames[5].style.height === "77px" && queries === 2);
// A window that is no board frame finds nothing.
frameMessage({ source: { id: "stranger" }, data: { wbDoc: "height", docKey: "entry:e6", height: 1 } });
check("a stranger's message changes nothing", raf.length === 0 && frames[6].style.height === "107px");
// Frames a board swap brought (wbKeep) start one keep session for the
// batch, before any height lands, applied once after; a wbKeep frame whose
// height does not change keeps its mark for a later report.
frames[7].wbKeep = frames[8].wbKeep = frames[9].wbKeep = true;
[7, 8].forEach(function (i) { frameMessage({ source: frames[i].contentWindow, data: { wbDoc: "height", docKey: "entry:e" + i, height: 500 } }); });
frameMessage({ source: frames[9].contentWindow, data: { wbDoc: "height", docKey: "entry:e9", height: 110 } });
raf.shift()();
check("one keep session for the batch, started before the heights landed: " + keeps, keeps.length === 1 && keeps[0] === "108px,109px,110px");
check("the session applies once, after the heights: " + applied, applied === 1 && frames[7].style.height === "500px");
check("only the frames that changed drop the mark", !frames[7].wbKeep && !frames[8].wbKeep && frames[9].wbKeep);
frames[10].wbFirst = frames[11].wbFirst = true;
frameMessage({ source: frames[10].contentWindow, data: { wbDoc: "height", docKey: "entry:e10", height: 400 } });
frameMessage({ source: frames[11].contentWindow, data: { wbDoc: "height", docKey: "entry:e11", height: 111.5 } });
raf.shift()();
check("a bands-mounted frame's first height that changes it lands once", frames[10].wbLanded === true && !frames[10].wbFirst);
check("one landing at the height it stood at marks nothing", frames[11].style.height === "112px" && !frames[11].wbLanded && !frames[11].wbFirst);
if (bad.length) console.log(bad.join("\n"));
`)
}

// TestTickTimesKeepsItsSetsUntilThePageChanges pins the per-second tick
// (dashboard.js tickTimes, tickDirty): it collects the relative times and
// paces once and keeps them across ticks while the page is unchanged, and a
// time inserted anywhere (here, outside any board swap) is collected and
// formatted on the next tick, since any insertion, removal, or change to a
// selected attribute drops the sets; the tick's own text writes inside a
// time or a pace do not. The observer watches the whole document. The cases
// execute under node when it is installed.
func TestTickTimesKeepsItsSetsUntilThePageChanges(t *testing.T) {
	src := dashboardFuncs(t, "tickDirty", "tickTimes")
	raw, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `new MutationObserver(tickDirty).observe(doc, { childList: true, subtree: true, attributes: true, attributeFilter: ["data-ms", "data-f", "data-reset"] });`) {
		t.Error("tickDirty does not observe the whole document for insertions, removals, and the selected attributes")
	}
	runNode(t, "tick sets", `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var queries = 0, times = [{ id: "t1" }], paces = [{ id: "p1" }], formatted = [];
var doc = { hidden: false }, shHidden = false;
var window = { MutationObserver: function () {} };
function q(sel) { queries++; return sel.indexOf("time") === 0 ? times.slice() : paces.slice(); }
function fmtTime(t) { formatted.push(t.id); }
function pace(e) { formatted.push(e.id); }
function el(tag, cls) { return { tagName: tag, classList: { contains: function (c) { return c === cls; } } }; }
var tickSets = null;
`+src+`
tickTimes(); tickTimes(); tickTimes();
check("three ticks collect once, got " + queries + " queries", queries === 2);
check("each tick formats every kept node", formatted.join() === "t1,p1,t1,p1,t1,p1");
// The tick's own text writes inside a time and a pace keep the sets.
tickDirty([{ type: "childList", target: el("TIME") }, { type: "childList", target: el("EM", "u-pace") }]);
tickTimes();
check("own text writes keep the sets", queries === 2);
// A conversation entry carrying a relative time lands outside the board.
times.push({ id: "t2" });
tickDirty([{ type: "childList", target: el("DIV") }]);
formatted = [];
tickTimes();
check("an insertion anywhere is collected on the next tick", queries === 4 && formatted.join() === "t1,t2,p1");
// A time whose format attribute changes is collected again too.
tickDirty([{ type: "attributes", target: el("TIME") }]);
tickTimes();
check("a selected attribute change drops the sets", queries === 6);
doc.hidden = true; formatted = [];
tickTimes();
check("a hidden tab skips the tick", formatted.length === 0);
if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardAnswersFlushTheTypedReply pins that the dashboard's reader of
// the board's reply text flushes a reply build still waiting for its frame
// (form.js buildSoon), so a keystroke typed just before a send is in it.
func TestBoardAnswersFlushTheTypedReply(t *testing.T) {
	src := dashboardFuncs(t, "boardAnswers")
	flush, read := strings.Index(src, "window.wbFlushReply()"), strings.Index(src, `querySelector("#reply-text")`)
	if flush < 0 || read < 0 || flush > read {
		t.Errorf("boardAnswers reads #reply-text without flushing the waiting build first:\n%s", src)
	}
}

// TestBoardFramesMountPaced pins the page load's paced mount
// (mountFramesPaced): the first slice mounts now and stops once about 8 ms
// have passed, each later slice runs in its own task (setTimeout) until every
// open placeholder is mounted; a placeholder under a closed fold stays one,
// and one a swap removed between slices is skipped. The cases execute under
// node when it is installed.
func TestBoardFramesMountPaced(t *testing.T) {
	src := dashboardFuncs(t, "boardBands", "frameNear", "frameShown", "mountFrame", "mountFrames", "mountFramesPaced", "mountSlice")
	runNode(t, "paced frame mount", frameDOM+src+`
var mountList = [], mountTask = false;
var clock = 0, timers = [];
var performance = { now: function () { return clock; } };
setTimeout = function (fn) { timers.push(fn); };
// Each mount costs 3 ms of the slice.
var rawMount = mountFrame;
mountFrame = function (p) { clock += 3; return rawMount(p); };
var open = [], closed = frame("entry:c"), gone = frame("entry:g");
for (var i = 0; i < 7; i++) open.push(frame("entry:o" + i));
var stream = new El("details", { open: "" }, open.slice(0, 4).concat([gone]).concat(open.slice(4)));
var board = new El("div", { id: "r-board" }, [stream, new El("details", {}, [closed])]);
doc.appendChild(board);
mountFramesPaced(doc);
check("the first slice mounts until about 8 ms have passed: " + open.map(mounted), mounted(open[0]) && mounted(open[1]) && mounted(open[2]) && placeholder(open[3]));
check("the rest waits for its own task", timers.length === 1);
stream.removeChild(gone);
var n = 0;
while (timers.length && n++ < 10) timers.shift()();
check("every open placeholder mounts in later slices", open.every(mounted));
check("a placeholder under a closed fold stays one", placeholder(closed));
check("one removed between slices is skipped", placeholder(gone) && !gone.parentNode);
check("the pacing ends", timers.length === 0 && n === 2);
if (bad.length) console.log(bad.join("\n"));
`)
}
