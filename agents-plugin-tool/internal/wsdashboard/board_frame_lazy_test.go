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
// the few selectors the functions under test use.
const frameDOM = `
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
var queue = [];
function tick() { while (queue.length) queue.shift()(); }
function El(tag, attrs, kids) {
  this.tagName = tag.toUpperCase(); this.nodeType = 1; this.parentNode = null; this.childNodes = [];
  this.attrs = {}; this._open = false; this.listeners = {};
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
  setAttribute: function (k, v) {
    this.attrs[k] = String(v);
    if (k === "srcdoc" && this.tagName === "IFRAME") this.loads = (this.loads || 0) + (this.isConnected ? 1 : 0);
  },
  removeAttribute: function (k) { delete this.attrs[k]; },
  addEventListener: function (type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); },
  removeChild: function (c) { this.childNodes.splice(this.childNodes.indexOf(c), 1); c.parentNode = null; return c; },
  insertBefore: function (c, ref) {
    if (c.parentNode) c.parentNode.removeChild(c);
    var i = ref ? this.childNodes.indexOf(ref) : this.childNodes.length;
    this.childNodes.splice(i, 0, c); c.parentNode = this;
    // A frame put on the page with no document loads about:blank at once.
    if (c.tagName === "IFRAME" && c.isConnected && !c.hasAttribute("srcdoc")) (c.listeners.load || []).forEach(function (fn) { fn(); });
    return c;
  },
  appendChild: function (c) { return this.insertBefore(c, null); },
  scrollIntoView: function () {}
};
function walk(root, out) { root.childNodes.forEach(function (c) { out.push(c); walk(c, out); }); return out; }
var SEL = {
  "iframe.doc-frame[data-srcdoc]:not([srcdoc])": function (n) { return n.tagName === "IFRAME" && n.classList.contains("doc-frame") && n.hasAttribute("data-srcdoc") && !n.hasAttribute("srcdoc"); },
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
doc.addEventListener = function (type, fn, capture) { if (capture) (doc.capture[type] = doc.capture[type] || []).push(fn); };
doc.dispatch = function (type, target) { (doc.capture[type] || []).forEach(function (fn) { fn({ target: target, type: type }); }); };
function frame(key, text) { return new El("iframe", { "class": "doc-frame", "data-doc": key, "data-doc-key": key, "data-srcdoc": text || "<!DOCTYPE html><html>" + key }); }
function mounted(f) { return f.getAttribute("srcdoc") === f.getAttribute("data-srcdoc") && f.hasAttribute("data-srcdoc"); }
var window = {};
function setTimeout() {}
function requestAnimationFrame() {}
`

// TestBoardFramesMountLazily pins the lazy frame mount (wswhiteboard
// framedDoc's data-srcdoc, dashboard.js mountFrames): a frame loads its
// document only once every details around it is open, on the page's first
// pass (a pinned topic's frame in the pin list included), on a toggle open
// whether the user's or a programmatic one (restoreOpen after a swap,
// boardJump), and for what a swap inserts; a frame under a closed fold or
// off the page stays unmounted, and data-srcdoc stays for comparisons. A
// changed topic's next body frame (frameGraft) gets its document before it
// goes in when the old frame had one, so it never shows blank. The cases
// execute under node when it is installed.
func TestBoardFramesMountLazily(t *testing.T) {
	src := dashboardFuncs(t, "frameShown", "mountFrames", "frameToggle", "isPeer", "openKey", "restoreOpenOne", "restoreOpen", "boardJump", "frameGraft")
	runNode(t, "lazy frame mount", frameDOM+src+`
var openState = {}, PEER_KEY = "p.";
function load() { return null; }
doc.addEventListener("toggle", frameToggle, true);

var A = frame("topic:x"), B = frame("entry:e1"), C = frame("entry:e2"), D = frame("entry:e3"), P = frame("topic:pinned");
var tdoc = new El("details", { "class": "tdoc", "data-key": "doc:x" }, [new El("div", {}, [A])]);
var versions = new El("details", { "class": "panel-versions", "data-key": "versions:e1" }, [new El("article", { id: "e2" }, [new El("details", { open: "" }, [C])])]);
var stream = new El("details", { "class": "topic-stream", open: "" }, [new El("article", {}, [new El("details", { "class": "entry-panel", open: "" }, [B, versions])])]);
var board = new El("div", { id: "r-board" }, [new El("section", { "class": "topic", "data-topic": "x" }, [tdoc, stream])]);
var pins = new El("div", { "class": "pins" }, [new El("section", { "class": "topic", "data-topic": "pinned" }, [P])]);
var inert = new El("template", {}, [D]);
doc.appendChild(board); doc.appendChild(pins);

// Before any pass nothing is mounted: the served frames carry data-srcdoc only.
check("served frames start unmounted", [A, B, C, D, P].every(function (f) { return !f.hasAttribute("srcdoc"); }));
mountFrames(doc);
check("an open chain mounts", mounted(B));
check("a pinned topic's frame mounts in the pin list", mounted(P));
check("a frame under a closed tdoc stays unmounted", !A.hasAttribute("srcdoc"));
check("a frame under a closed versions fold stays unmounted", !C.hasAttribute("srcdoc"));
check("a frame off the page stays unmounted", !D.hasAttribute("srcdoc"));
check("data-srcdoc stays on a mounted frame", B.getAttribute("data-srcdoc") === "<!DOCTYPE html><html>entry:e1");

// A user's toggle open.
tdoc.open = true; tick();
check("a toggle open mounts", mounted(A));
var loadsA = A.loads;
tdoc.open = false; tick(); tdoc.open = true; tick();
check("a mounted frame is not mounted again", A.loads === loadsA);

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
check("boardJump refuses an id it cannot find", hit === undefined && !C.hasAttribute("srcdoc"));
$ = function (id) { return id === "p2" ? Object.assign(versions.childNodes[0], { id: "p2" }) : null; };
boardJump("p2"); tick();
check("boardJump's opens mount", mounted(C));

// A swap's inserted section mounts what it shows.
var N1 = frame("entry:e5"), N2 = frame("entry:e6");
var n = new El("section", { "class": "topic", "data-topic": "z" }, [new El("details", { open: "" }, [N1]), new El("details", {}, [N2])]);
mountFrames(n);
check("a section off the page mounts nothing", !N1.hasAttribute("srcdoc"));
board.appendChild(n); mountFrames(n);
check("an inserted section mounts its open frames", mounted(N1) && !N2.hasAttribute("srcdoc"));

// frameGraft: the next body frame of a mounted frame gets its document
// before it goes in; the next frame of an unmounted one stays unmounted.
function graft(oldMounted) {
  var of = frame("topic:g", "<!DOCTYPE html><html>old"), nf = frame("topic:g", "<!DOCTYPE html><html>new");
  if (oldMounted) of.setAttribute("srcdoc", of.getAttribute("data-srcdoc"));
  var od = new El("div", { "class": "doc-body framed" }, [of]), nd = new El("div", { "class": "doc-body framed" }, [nf]);
  var old = new El("section", { "class": "topic", "data-topic": "g" }, [od]), nsec = new El("section", { "class": "topic", "data-topic": "g" }, [nd]);
  board.appendChild(old);
  var blank = false;
  frameShow = function (f) { if (!f.hasAttribute("srcdoc")) blank = true; };
  frameGraft({ old: old, n: nsec, oc: [of, od, old], nc: [nf, nd, nsec] });
  return { nf: nf, blank: blank, placed: od.childNodes.indexOf(nf) === 1 };
}
var frameShow, frameRevealMs = 1500;
function frameReveal() {}
var g1 = graft(true);
check("the next frame of a mounted frame goes in mounted", g1.placed && mounted(g1.nf) && g1.nf.getAttribute("srcdoc") === "<!DOCTYPE html><html>new");
check("the next frame of a mounted frame never loads blank", !g1.blank);
var g2 = graft(false);
check("the next frame of an unmounted frame stays unmounted", g2.placed && !g2.nf.hasAttribute("srcdoc"));

if (bad.length) console.log(bad.join("\n"));
`)
}

// TestBoardFramesMountAtEveryInsert pins where the page runs the mount
// pass: once after the cards have moved the pinned topics (so no frame
// loads and then moves), for every node a board swap inserts after its
// frame heights are restored, for an adopted closed page, and for a
// history load; the toggle listener is a capture listener.
func TestBoardFramesMountAtEveryInsert(t *testing.T) {
	raw, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	board := string(raw)
	mount, first := strings.Index(board, "window.WsCards.mount(cardBoard"), strings.Index(board, "\n  mountFrames(doc);\n")
	if mount < 0 || first < 0 || first < mount {
		t.Errorf("the first mount pass must follow WsCards.mount (cards %d, pass %d)", mount, first)
	}
	for what, pattern := range map[string]string{
		"swap insert":    `setFrameHeights\(n, frameH\);\n\s+mountFrames\(n\);`,
		"adopted page":   `adopted: function \(n\) \{\n[^}]*restoreOpen\(n\);\n\s+mountFrames\(n\);`,
		"history load":   `"wbhistoryload", function \(e\) \{\n[^}]*restoreOpen\(el\);\n\s+mountFrames\(el\);`,
		"toggle capture": `doc\.addEventListener\("toggle", frameToggle, true\);`,
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
// pending next frame (frameGraft) keeps its height. The cases execute
// under node when it is installed.
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
if (bad.length) console.log(bad.join("\n"));
`)
}
