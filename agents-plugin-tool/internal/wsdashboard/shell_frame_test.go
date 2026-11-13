package wsdashboard

import (
	"regexp"
	"testing"
	"time"
)

// TestHiddenFrameHoldsItsUpdatesUntilShown pins a framed page's side of the
// shell's visibility message (dashboard.js shFrameInit, connect,
// showRegions, tickTimes): a frame the shell hides gets no visibilitychange,
// so the shell tells it (shown); hidden, it keeps only each region's newest
// update and applies none, skips the clock, and defers the tracker reload,
// as a hidden tab does; shown, it applies the held updates once each, reads
// the tracker once, and catches the clock up. A message from any window but
// its parent is ignored. The cases execute under node when it is installed.
func TestHiddenFrameHoldsItsUpdatesUntilShown(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	block := func(what, pattern string) string {
		t.Helper()
		got := regexp.MustCompile(pattern).FindString(js)
		if got == "" {
			t.Fatalf("dashboard.js holds no %s", what)
		}
		return got
	}
	open := block("openStream", `(?s)\n  function openStream\(path, on\) \{.*?\n  \}\n`)
	connect := block("connected ... connect", `(?s)\n  var sseOn = true;\n.*?\n  // showRegions applies`)
	show := block("showRegions", `(?s)\n  function showRegions\(\) \{.*?\n  \}\n`)
	tick := block("tickTimes", `(?s)\n  function tickTimes\(\) \{.*?\n  \}\n`)
	frame := block("shFrameInit", `(?s)\n  function shFrameInit\(\) \{.*?\n  \}\n  // shShellInit`)
	post := block("shPost", `(?s)\n  function shPost\(kind, m\) \{.*?\n  \}\n`)

	runNode(t, "hidden frame", "(function () {\n"+streamPageHarness+`
framed = true;
var parentWin = { got: [], postMessage: function (m, o) { parentWin.got.push([m.wsShell, o]); } }, otherWin = {};
var listeners = {};
window.addEventListener = function (t, fn) { (listeners[t] = listeners[t] || []).push(fn); };
window.parent = parentWin;
window.scrollY = 0;
window.innerWidth = 1200;
location.origin = "http://localhost:1234";
doc.querySelector = function () { return null; };
doc.addEventListener = function () {};
doc.title = "sess - ws dashboard";
var root = { hasAttribute: function () { return false; }, clientWidth: 1200 };
function region() { return null; }
function $() { return null; }
var mode = "session", rid = "rid1", hiddenRegions = {}, modeHash = "", liveMode = false, tlHeld = {};
function held() { return ""; }
function trackerLost() {}
function trackerNudge(rev) { log.push("nudge:" + rev); }
function swap(name, hash, html) { log.push("swap:" + name + "." + hash); }
function tlHold(name) { log.push("hold:" + name); }
function tlCount() {}
function tlShown() {}
function modeChanged() {}
function syncBadge() {}
function banner() {}
function apScan() {}
function frag() {}
function railKey() {}
function railPopOpen() {}
function setTheme() {}
var times = 0;
function fmtTime() { times++; }
function pace() {}
Date.now = function () { return 0; };
q = function (sel) { return /time\[data-ms\]/.test(sel) ? [{}] : sel === "#sse" ? [indicator] : []; };
`+mustAsset(t, "shell.js")+open+connect+show+tick+post+frame+`
connect();
shFrameInit();
var s = sockets[0];
check("a framed session page streams without the rail", s.url, "ws://localhost:1234/s/rid1/events?h=&frame=1");
s.onopen({});
function fromShell(m, src) { m.wsShell = m.wsShell || "shown"; listeners.message.forEach(function (fn) { fn({ source: src || parentWin, origin: location.origin, data: m }); }); }
s.msg("region", { name: "board", hash: "1", html: "" });
tickTimes();
check("a shown frame applies regions and ticks the clock", [log, times], [["swap:board.1"], 1]);
log = [];
fromShell({ shown: false }, otherWin);
s.msg("region", { name: "board", hash: "2", html: "" });
check("a message from another window is ignored", log, ["swap:board.2"]);
log = [];
fromShell({ shown: false });
s.msg("region", { name: "board", hash: "3", html: "" });
s.msg("region", { name: "convlive", hash: "4", html: "" });
s.msg("region", { name: "board", hash: "5", html: "" });
s.msg("tracker", { rev: 7 });
s.msg("tracker", { rev: 8 });
tickTimes();
check("a hidden frame holds its regions, defers the tracker, and skips the clock",
  [log, Object.keys(hiddenRegions), hiddenRegions.board.hash, shTrackerRev, times],
  [["hold:board", "hold:convlive", "hold:board"], ["convlive", "board"], "5", 8, 1]);
log = [];
fromShell({ shown: true });
check("shown, it applies each held region once, in arrival order, and reads the tracker once",
  [log, Object.keys(hiddenRegions), shTrackerRev, times], [["swap:convlive.4", "swap:board.5", "nudge:8"], [], null, 2]);
log = [];
s.msg("region", { name: "board", hash: "6", html: "" });
s.msg("tracker", { rev: 9 });
check("and then acts on updates as they come", log, ["swap:board.6", "nudge:9"]);
check("the frame posted its layout and title to its parent only", parentWin.got.map(function (x) { return x.join(" "); }), ["layout http://localhost:1234", "title http://localhost:1234"]);
process.stdout.write(bad.join("\n"));
})();
`)
}

// TestFramesOfOneSessionShareOneRender pins the render fan-out under the
// shell: two frames of one session (two shells, or a shell and the page
// standing alone) take one session render per change, and two shells' rail
// streams one rail render.
func TestFramesOfOneSessionShareOneRender(t *testing.T) {
	f := startPageFixture(t)
	a := openStream(t, f.base+"/s/livesession/events?frame=1")
	b := openStream(t, f.base+"/s/livesession/events?frame=1")
	railA := openStream(t, f.base+railEventsRoute)
	railB := openStream(t, f.base+railEventsRoute)
	for _, s := range []<-chan sseEvent{a, b, railA, railB} {
		quiet(t, s)
	}
	renders, rails := f.d.stats.SessionRenders.Load(), f.d.stats.RailRenders.Load()
	f.change(t, "livesession", f.project, "shared change", 4)
	for _, s := range []<-chan sseEvent{a, b} {
		waitRegionHolding(t, s, regionBoard, "shared change")
	}
	time.Sleep(settle)
	if got := f.d.stats.SessionRenders.Load() - renders; got != 1 {
		t.Errorf("the change took %d session renders for two frames, want 1", got)
	}
	if got := f.d.stats.RailRenders.Load() - rails; got != 1 {
		t.Errorf("the change took %d rail renders for two shells, want 1", got)
	}
}

// shellGlueHarness is a minimal shell document for running dashboard.js's
// shell block under node: fake frames (load, src, postMessage, removal), a
// fake history and address, and the window listeners the block adds.
const shellGlueHarness = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var ORIGIN = "http://h:1";
var location = { origin: ORIGIN, pathname: "/s/a", hash: "", search: "?token=t", get href() { return ORIGIN + this.pathname + this.search + this.hash; } };
var pushed = [];
var history = { pushState: function (s, t, u) { pushed.push(u); var x = new URL(u, ORIGIN); location.pathname = x.pathname; location.hash = x.hash; location.search = ""; },
  replaceState: function (s, t, u) { var x = new URL(u, ORIGIN); location.pathname = x.pathname; location.hash = x.hash; } };
var listeners = {};
var window = { addEventListener: function (t, fn) { (listeners[t] = listeners[t] || []).push(fn); }, top: null, parent: null };
window.top = window.parent = window;
function Frame(src) {
  var self = this;
  this.className = "sframe"; this.title = ""; this.removed = false; this.onload = []; this.got = []; this._src = src || "";
  this.classList = {
    add: function (c) { if (!self.classList.contains(c)) self.className += " " + c; },
    remove: function (c) { self.className = self.className.split(" ").filter(function (x) { return x !== c; }).join(" "); },
    contains: function (c) { return self.className.split(" ").indexOf(c) >= 0; }
  };
  this.contentWindow = { location: { hash: "" }, focus: function () {}, postMessage: function (m, o) { if (o === ORIGIN) self.got.push(m.wsShell + (m.wsShell === "shown" ? ":" + m.shown : "")); }, wsHoldsDraft: function () { return !!self.draft; } };
  this.contentDocument = { readyState: "loading" };
}
Object.defineProperty(Frame.prototype, "src", { get: function () { return this._src; }, set: function (v) { this._src = v; this.contentDocument.readyState = "loading"; } });
Frame.prototype.addEventListener = function (t, fn) { if (t === "load") this.onload.push(fn); };
Frame.prototype.load = function () { this.contentDocument.readyState = "complete"; this.onload.forEach(function (fn) { fn(); }); };
Frame.prototype.remove = function () { this.removed = true; };
Frame.prototype.focus = function () {};
var first = new Frame("/s/a?frame=1");
first.className = "sframe on";
var top = { own: false, style: {}, offsetHeight: 40, classList: { toggle: function (c, on) { if (c === "sh-own") top.own = on; } }, addEventListener: function () {} };
var framesBox = { kids: [first], appendChild: function (f) { this.kids.push(f); } };
var doc = { title: "a - ws dashboard",
  querySelector: function (sel) { return sel === "#frames > iframe" ? first : sel === ".dwrap > .top" ? top : null; },
  createElement: function () { return new Frame(); },
  addEventListener: function () {} };
function $(id) { return id === "frames" ? framesBox : null; }
var attrs = {};
var root = { setAttribute: function (k, v) { attrs[k] = v; }, removeAttribute: function (k) { delete attrs[k]; }, hasAttribute: function (k) { return k in attrs; }, style: { setProperty: function () {} } };
var shell = true, framed = false, rid = "a", headHeld = null, log = [];
function region() { return null; }
function labelEditing() { return false; }
function labelEnd() {}
function swap(name, hash, html) { log.push("swap:" + name + ":" + html); }
function railMarks() { log.push("marks:" + rid); }
function railOpen() {}
function railShown() { return false; }
function railKey() { log.push("railKey"); }
function setTheme() {}
function from(win, m, origin) { listeners.message.forEach(function (fn) { fn({ source: win, origin: origin || ORIGIN, data: m }); }); }
function on() { return framesBox.kids.filter(function (f) { return f.classList.contains("on") && !f.removed; }).map(function (f) { return f.src; }); }
function live() { return framesBox.kids.filter(function (f) { return !f.removed; }).map(function (f) { return f.src.replace("?frame=1", ""); }); }
function frameOf(path) { return framesBox.kids.filter(function (f) { return f.src.indexOf(path + "?") === 0 && !f.removed; })[0]; }
function click(href, mods) {
  var e = { defaultPrevented: false, button: 0, preventDefault: function () { e.defaultPrevented = true; },
    target: { closest: function () { return { href: new URL(href, ORIGIN).href, target: "", hasAttribute: function () { return false; } }; } } };
  for (var k in mods || {}) e[k] = mods[k];
  listeners.click.forEach(function (fn) { fn(e); });
  return e.defaultPrevented;
}
`

// TestShellSwitchesFramesThroughItsGlue pins the shell's side of the frames
// (dashboard.js's shell block): it takes the frame it loaded with by its
// address's path and hash (a query such as a LAN token is not the page's),
// trusts a message only from one of its frames on its own origin, shows a
// page once its frame loaded (pushState on a pick, none on popstate), tells
// each frame on every load whether it shows, keeps the shown and the most
// recently opened frames to the limit while a frame holding unsent work
// stays, and takes plain clicks on the pages it frames. The cases execute
// under node when it is installed.
func TestShellSwitchesFramesThroughItsGlue(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	glue := regexp.MustCompile(`(?s)\n  var shFrames = \[\].*?\n  // shFrameInit wires`).FindString(js)
	init := regexp.MustCompile(`(?s)\n  function shShellInit\(\) \{.*?\n  \}\n`).FindString(js)
	if glue == "" || init == "" {
		t.Fatal("dashboard.js holds no shell block")
	}
	runNode(t, "shell glue", shellGlueHarness+mustAsset(t, "shell.js")+"\n(function () {\n"+glue+init+`
first.contentDocument.readyState = "complete";
shShellInit();
check("the first frame is taken by its path despite a query, loaded, and asked to sync", [shOn && shOn.el === first, shOn && shOn.loaded, first.got], [true, true, ["sync"]]);
from(first.contentWindow, { wsShell: "head", html: "<b>a</b>", qs: "", pad: 0, top: 40 });
check("the shown frame's head is shown", log, ["swap:head:<b>a</b>"]);
log = [];
from(first.contentWindow, { wsShell: "head", html: "<b>evil</b>", qs: "", pad: 0, top: 40 }, "http://evil:1");
from({}, { wsShell: "head", html: "<b>stray</b>", qs: "", pad: 0, top: 40 });
from(first.contentWindow, { wsShell: "head", html: 1, qs: "", pad: 0, top: 40 });
from(first.contentWindow, { wsShell: "nav", href: "/s/b" }, "http://evil:1");
check("another origin, a window that is no frame, and a malformed message are ignored", [log, pushed], [[], []]);

check("a plain click on a session link is taken", click("/s/b"), true);
var b = frameOf("/s/b");
check("a new frame loads hidden while the shown one stays", [pushed, live(), on(), attrs["data-rid"]], [["/s/b"], ["/s/a", "/s/b"], ["/s/a?frame=1"], "b"]);
b.load();
check("once loaded it shows; each frame is told", [on(), first.got, b.got], [["/s/b?frame=1"], ["sync", "shown:false"], ["shown:true"]]);
check("a modified click and another route are left to the browser", [click("/s/c", { ctrlKey: true }), click("/tracker/x"), click("https://example.com/")], [false, false, false]);

from(first.contentWindow, { wsShell: "nav", href: "/s/c" });
check("a hidden frame's nav is ignored", pushed, ["/s/b"]);
from(b.contentWindow, { wsShell: "nav", href: "/s/c" });
from(b.contentWindow, { wsShell: "nav", href: "/s/d" });
var c = frameOf("/s/c"), d = frameOf("/s/d");
c.load();
check("a frame superseded before it showed is told it is hidden", [c.got, on()], [["shown:false"], ["/s/b?frame=1"]]);
d.load();
check("the last pick shows", on(), ["/s/d?frame=1"]);
first.load();
check("a hidden frame's reload is told it is hidden", first.got.slice(-1), ["shown:false"]);

location.pathname = "/s/a"; location.hash = "";
listeners.popstate.forEach(function (fn) { fn({}); });
check("back shows a kept frame at once, pushing nothing", [on(), pushed], [["/s/a?frame=1"], ["/s/b", "/s/c", "/s/d"]]);

d.draft = true;
["/s/e", "/s/f", "/s/g", "/s/h"].forEach(function (p) { click(p); });
check("the shown frame stays and a frame holding a draft stays past the limit; the least recent go", live(), ["/s/a", "/s/d", "/s/e", "/s/f", "/s/g", "/s/h"]);
d.draft = false;
click("/s/i");
check("the draft frame goes once it holds none; a frame never shown goes before the shown one", live(), ["/s/a", "/s/f", "/s/g", "/s/h", "/s/i"]);
process.stdout.write(bad.join("\n"));
})();
`)
}
