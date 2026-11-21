package wsdashboard

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestHiddenFrameHoldsItsUpdatesUntilShown pins a framed page's side of the
// shell's visibility message (dashboard.js shFrameInit, connect,
// pageHidden, showRegions, tickTimes): browser-hidden tabs hold the newest
// regions while retaining light tracker events; a cached frame stops its
// stream and clock, suspends author frames, and ignores late socket events.
// Showing it drains held regions, reconnects once and catches the clock up.
// A message from any window but
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
	search := block("tkSearch", `(?s)\n  var tkWantSearch = false;\n  function tkSearch\(\) \{.*?\n  \}\n`)

	runNode(t, "hidden frame", "(function () {\n"+animationFrameHarness+streamPageHarness(t)+`
framed = true;
var parentWin = { got: [], hashes: [], layouts: [], postMessage: function (m, o) { parentWin.got.push([m.wsShell, o]); if (m.wsShell === "hash") parentWin.hashes.push(m.hash); if (m.wsShell === "layout") parentWin.layouts.push(m); } }, otherWin = {};
var listeners = {};
window.addEventListener = function (t, fn) { (listeners[t] = listeners[t] || []).push(fn); };
window.parent = parentWin;
window.scrollY = 0;
window.innerWidth = 1200;
location.origin = "http://localhost:1234";
location.hash = "";
var history = { pushState: function (s, t, u) { location.hash = u; }, replaceState: function (s, t, u) { location.hash = u; } };
doc.querySelector = function () { return null; };
doc.title = "sess - Werkschaft (alpha)";
var rootAttrs = {}, bodyWatch = [];
doc.body = {};
var MutationObserver = window.MutationObserver = function () { var seen = new Map(); this.observe = function (node, opts) { if (node === doc.body) { if (seen.has(node)) bodyWatch[seen.get(node)] = opts; else { seen.set(node, bodyWatch.length); bodyWatch.push(opts); } } }; };
var root = { toggleAttribute: function (k, on) { if (on) rootAttrs[k] = true; else delete rootAttrs[k]; }, setAttribute: function (k) { rootAttrs[k] = true; }, removeAttribute: function (k) { delete rootAttrs[k]; }, hasAttribute: function (k) { return !!rootAttrs[k]; }, clientWidth: 1200 };
function CustomEvent(type) { this.type = type; }
var suspended = 0, mounted = 0;
function suspendDocFrames() { suspended++; }
function mountFramesPaced() { mounted++; }
function region() { return null; }
function $() { return null; }
var searchOpens = 0, tkApi = { openSearch: function () { searchOpens++; } };
var mode = "session", rid = "rid1", worktree = "", hiddenRegions = {}, modeHash = "", liveMode = false, tlHeld = {};
function held() { return ""; }
function boardInputFull() { return null; }
function trackerLost() {}
function trackerNudge(rev) { log.push("nudge:" + rev); }
function swap(name, hash, html) { log.push("swap:" + name + "." + hash); }
// The board's shell-return window is pinned by testdata/board-home-ack.test.cjs.
function boardReturnShown() {}
function boardReturnEnd() {}
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
var settings = [];
function setLayout(v) { settings.push(["layout", v]); }
function setInput(v) { settings.push(["input", v]); }
var times = 0, tickSets = null;
function fmtTime() { times++; }
function pace() {}
Date.now = function () { return 0; };
var linkMenus = [], railPops = [];
q = function (sel) {
  if (/time\[data-ms\]/.test(sel)) return [{}];
  if (sel === "#sse") return [indicator];
  if (sel === ".rpop:not([hidden])") return railPops;
  if (sel === ".rpop:not([hidden]), .lmenu") return railPops.concat(linkMenus);
  return [];
};
`+mustAsset(t, "shell.js")+dashboardFuncs(t, "syncPageVisibility")+open+connect+show+tick+post+search+frame+`
connect();
shFrameInit();
flushAnimationFrames();
var s = sockets[0];
check("the body is watched for appended popovers and for hidden/open toggles, neither observe() overwriting the other",
  [bodyWatch.some(function (o) { return o.childList && !o.subtree; }), bodyWatch.some(function (o) { return o.subtree && o.attributes && o.attributeFilter.indexOf("hidden") >= 0; })], [true, true]);
check("a framed session page opts into topic and tree rows without the rail", s.url, "ws://localhost:1234/s/rid1/events?h=&frame=1&topics=1&treerows=1&entryrows=1&panelrows=1&display=1");
s.onopen({});
function fromShell(m, src) { m.wsShell = m.wsShell || "shown"; listeners.message.forEach(function (fn) { fn({ source: src || parentWin, origin: location.origin, data: m }); }); flushAnimationFrames(); }
fromShell({ wsShell: "settings", layout: "tabs", input: "mod" }, otherWin);
check("another window cannot change settings", settings, []);
fromShell({ wsShell: "settings", layout: "tabs", input: "mod" });
check("shell settings apply without consulting storage", settings, [["layout", "tabs"], ["input", "mod"]]);
fromShell({ wsShell: "search" }, otherWin);
check("another window cannot open search", searchOpens, 0);
fromShell({ wsShell: "search" });
check("the shell opens the palette without a hidden header trigger", searchOpens, 1);
s.msg("region", { name: "board", hash: "1", html: "" });
tickTimes();
check("a shown frame applies regions and ticks the clock", [log, times], [["swap:board.1"], 1]);
log = [];
fromShell({ shown: false }, otherWin);
s.msg("region", { name: "board", hash: "2", html: "" });
check("a message from another window is ignored", log, ["swap:board.2"]);
log = [];
doc.hidden = true; syncPageVisibility();
s.msg("region", { name: "board", hash: "3", html: "" });
s.msg("region", { name: "convlive", hash: "4", html: "" });
s.msg("region", { name: "board", hash: "5", html: "" });
s.msg("tracker", { rev: 7 });
s.msg("tracker", { rev: 8 });
tickTimes();
check("a browser-hidden frame holds regions and skips the clock while light tracker events act",
  [log, Object.keys(hiddenRegions), hiddenRegions.board.hash, times],
  [["hold:board", "hold:convlive", "hold:board", "nudge:7", "nudge:8"], ["convlive", "board"], "5", 1]);
doc.hidden = false;
log = [];
fromShell({ shown: false });
check("caching closes the stream and suspends author documents", [s.closed, suspended, root.hasAttribute("data-animation-paused")], [true, 1, true]);
s.msg("region", { name: "board", hash: "late", html: "" });
s.msg("tracker", { rev: 99 });
s.fail(); advance(10000); tickTimes();
check("cached frames ignore late events and never retry or tick", [log, sockets.length, times, hiddenRegions.board.hash], [[], 1, 1, "5"]);
fromShell({ shown: true });
check("shown, it applies each held region once in arrival order and resumes rendering",
  [log, Object.keys(hiddenRegions), shTrackerRev, times, mounted, sockets.length, root.hasAttribute("data-animation-paused")],
  [["swap:convlive.4", "swap:board.5"], [], null, 2, 1, 2, false]);
check("resumed stream requests display work", sockets[1].url, s.url);
log = [];
s = sockets[1]; s.onopen({});
s.msg("region", { name: "board", hash: "6", html: "" });
s.msg("tracker", { rev: 9 });
check("and then acts on updates as they come", log, ["swap:board.6", "nudge:9"]);
history.pushState(null, "", "#/store");
check("the frame posted its title, deferred layout, and hash route to its parent only", parentWin.got.map(function (x) { return x.join(" "); }), ["title http://localhost:1234", "layout http://localhost:1234", "hash http://localhost:1234"]);
history.replaceState(null, "", "#/store/item");
location.hash = "#/back";
listeners.popstate.forEach(function (fn) { fn({}); });
location.hash = "#/typed";
listeners.hashchange.forEach(function (fn) { fn({}); });
check("each way a hash route moves posts its hash", parentWin.hashes, ["#/store", "#/store/item", "#/back", "#/typed"]);
function popover() { fromShell({ wsShell: "sync" }); return parentWin.layouts[parentWin.layouts.length - 1].popover; }
var visible = { getClientRects: function () { return [{}]; } };
var hidden = { getClientRects: function () { return []; } };
linkMenus = [visible];
check("a visible link menu lifts its frame above the shell header", popover(), true);
check("a lifted page drops its background so the shell header and rail show beneath", "data-lifted" in rootAttrs, true);
linkMenus = [hidden];
check("a menu with no layout does not lift the frame", popover(), false);
linkMenus = [visible]; railPops = [visible];
check("a link menu and rail popover share the frame layer", popover(), true);
linkMenus = [];
check("closing the link menu preserves another visible popover", popover(), true);
railPops = [];
check("closing the last popover restores the shell layer", popover(), false);
check("closing the last popover restores the page background", "data-lifted" in rootAttrs, false);
var hoverCard = { classList: { contains: function (c) { return c === "shp"; } }, getClientRects: function () { return [{}]; } };
railPops = [hoverCard];
check("a visible read-only hover card does not lift the frame over the shell header and rail", popover(), false);
check("a hover card keeps the page background", "data-lifted" in rootAttrs, false);
railPops = [hoverCard, visible];
check("an interactive popover beside a hover card still lifts the frame", popover(), true);
railPops = [];
process.stdout.write(bad.join("\n"));
})();
`)
}

// TestFramesOfOneSessionShareOneRender pins the render fan-out under the
// shell: two frames of one session (two shells, or a shell and the page
// standing alone) take one session render per change, and two shells' rail
// streams share one rail render when its visible content changes.
func TestFramesOfOneSessionShareOneRender(t *testing.T) {
	f := startStoredRailFixture(t)
	a := openStream(t, f.base+"/s/livesession/events?frame=1")
	b := openStream(t, f.base+"/s/livesession/events?frame=1")
	railA := openStream(t, f.base+railEventsRoute)
	railB := openStream(t, f.base+railEventsRoute)
	for _, s := range []<-chan sseEvent{a, b, railA, railB} {
		quiet(t, s)
	}
	renders, rails := f.d.stats.SessionRenders.Load(), f.d.stats.RailRenders.Load()
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{
		{Kind: wslive.RingReply, Text: "shared rail change"},
	}); err != nil {
		t.Fatal(err)
	}
	f.change(t, "livesession", f.project, "shared change", 4)
	for _, s := range []<-chan sseEvent{a, b} {
		waitRegionHolding(t, s, regionBoard, "shared change")
	}
	for _, s := range []<-chan sseEvent{railA, railB} {
		waitRegionHolding(t, s, regionRail, "shared rail change")
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
  this.saved = 0;
  this.contentWindow = { location: { hash: "", href: ORIGIN + this._src, replace: function (href) { if (!self.cancelLeave) self.pending = href; } }, wsSaveSessionDraft: function () { self.saved++; }, focus: function () {}, postMessage: function (m, o) { if (o === ORIGIN) self.got.push(m.wsShell + (m.wsShell === "shown" ? ":" + m.shown : "")); }, wsHoldsDraft: function () { return !!self.draft; } };
  this.contentDocument = { readyState: "loading" };
}
Object.defineProperty(Frame.prototype, "src", { get: function () { return this._src; }, set: function (v) { this._src = v; this.contentDocument.readyState = "loading"; } });
Frame.prototype.addEventListener = function (t, fn) { if (t === "load") this.onload.push(fn); };
Frame.prototype.load = function () { this.contentDocument.readyState = "complete"; this.contentWindow.location.href = this.pending || ORIGIN + this._src; this.pending = null; this.onload.forEach(function (fn) { fn(); }); };
Frame.prototype.remove = function () { this.removed = true; };
Frame.prototype.focus = function () {};
var first = new Frame("/s/a?frame=1");
first.className = "sframe on";
var top = { own: false, style: { setProperty: function (k, v) { top.style[k] = v; } }, offsetHeight: 40, classList: { toggle: function (c, on) { if (c === "sh-own") top.own = on; } }, addEventListener: function () {} };
var framesBox = { kids: [first], appendChild: function (f) { this.kids.push(f); } };
var doc = { title: "a - Werkschaft (alpha)",
  querySelector: function (sel) { return sel === "#frames > iframe" ? first : sel === ".dwrap > .top" ? top : null; },
  createElement: function () { return new Frame(); },
  addEventListener: function () {} };
function $(id) { return id === "frames" ? framesBox : null; }
var attrs = {};
var root = { setAttribute: function (k, v) { attrs[k] = v; }, removeAttribute: function (k) { delete attrs[k]; }, hasAttribute: function (k) { return k in attrs; }, style: { setProperty: function () {} } };
var shell = true, framed = false, rid = "a", headHeld = null, log = [], layoutMode = "tabs", inputMode = "mod";
function region() { return null; }
function labelEditing() { return false; }
function labelEnd() {}
function swap(name, hash, html) { log.push("swap:" + name + ":" + html); }
function railMarks() { log.push("marks:" + rid); }
function railOpen() {}
function railShown() { return false; }
function railKey() { log.push("railKey"); }
var renameChecks = 0;
function railRenameNew() { renameChecks++; }
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
// page once its fresh frame loaded (pushState on a pick, none on popstate),
// saves drafts before a destructive swap, retains the current page on
// cancellation, and ignores removed frames. The same cases run directly with
// node --test, without a daemon or a Go-built fixture.
func TestShellSwitchesFramesThroughItsGlue(t *testing.T) {
	runNode(t, "shell lifecycle", `require("./testdata/shell-lifecycle.test.cjs")().then(() => {}, error => { throw error; });`)
}

// TestShellHotkeyForwarderLeavesSearchKeysToTerminals pins the shell
// document's keydown forwarder (dashboard.js shShellInit): Ctrl+K, Ctrl+Space
// and Cmd+K open the shown frame's palette except from a terminal target
// (tracker.js qShortcut's rule), and `/` skips any typing target. Runs under node when
// it is installed.
func TestShellHotkeyForwarderLeavesSearchKeysToTerminals(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	fwd := regexp.MustCompile(`(?s)stay with a terminal by qShortcut's target rule\.\n(    doc\.addEventListener\("keydown", function \(e\) \{.*?\n    \}\);\n)`).FindStringSubmatch(js)
	if fwd == nil {
		t.Fatal("dashboard.js holds no shell keydown forwarder")
	}
	runNode(t, "shell hotkey forwarder", "(function () {\nvar bad = [], searched = 0, handler;\n"+`
function shSearch() { searched++; }
var doc = { addEventListener: function (t, fn) { handler = fn; } };
`+fwd[1]+`
function target(sel) { return { closest: function (s) { return s.split(", ").some(function (p) { return sel.indexOf(p) >= 0 || (p.indexOf("[contenteditable") === 0 && sel === "ce"); }) ? this : null; } }; }
function fire(e, sel) { searched = 0; e.preventDefault = function () {}; e.target = target(sel); handler(e); return searched; }
function check(what, got, want) { if (got !== want) bad.push(what + ": got " + got + ", want " + want); }
check("Ctrl+K on the body opens", fire({ ctrlKey: true, key: "k" }, "body"), 1);
check("Ctrl+Space on the body opens", fire({ ctrlKey: true, key: " ", code: "Space" }, "body"), 1);
check("Cmd+K on the body opens", fire({ metaKey: true, key: "k" }, "body"), 1);
check("Cmd+Space is left to the system", fire({ metaKey: true, key: " ", code: "Space" }, "body"), 0);
check("Ctrl+K in a textarea opens", fire({ ctrlKey: true, key: "k" }, "textarea"), 1);
check("Ctrl+Space in a textarea opens", fire({ ctrlKey: true, key: " ", code: "Space" }, "textarea"), 1);
check("Cmd+K in an input opens", fire({ metaKey: true, key: "k" }, "input"), 1);
check("Ctrl+K in a contenteditable opens", fire({ ctrlKey: true, key: "k" }, "ce"), 1);
check("Ctrl+K in the terminal is left to it", fire({ ctrlKey: true, key: "k" }, ".xterm"), 0);
check("Cmd+K in the terminal is left to it", fire({ metaKey: true, key: "k" }, ".xterm"), 0);
check("Cmd+/ does nothing", fire({ metaKey: true, key: "/" }, "body"), 0);
check("/ on the body opens", fire({ key: "/" }, "body"), 1);
check("/ in a textarea types", fire({ key: "/" }, "textarea"), 0);
check("/ in an input types", fire({ key: "/" }, "input"), 0);
process.stdout.write(bad.join("\n"));
})();
`)
}

// TestFramePageAndItsStreamShareOneRead pins the session-switch hand-off: a
// frame page request renders through the shared page and keeps it, so the
// stream the frame opens next sends its first frame from that read instead
// of reading the session again; a change after the page still re-renders.
func TestFramePageAndItsStreamShareOneRead(t *testing.T) {
	f := startPageFixture(t)
	reads, renders := f.src.readCount("livesession"), f.d.stats.SessionRenders.Load()
	if code, body := get(t, f.base+"/s/livesession?frame=1"); code != http.StatusOK || !strings.Contains(body, "live board text") {
		t.Fatalf("frame page = %d", code)
	}
	events := openStream(t, f.base+"/s/livesession/events?frame=1")
	if got := quiet(t, events); len(got) == 0 {
		t.Fatal("the stream sent no first frame")
	}
	if got := f.src.readCount("livesession") - reads; got != 1 {
		t.Errorf("page open plus stream subscribe read the session %d times, want 1", got)
	}
	if got := f.d.stats.SessionRenders.Load() - renders; got != 1 {
		t.Errorf("page open plus stream subscribe rendered %d times, want 1", got)
	}
	f.change(t, "livesession", f.project, "changed after the page", 4)
	waitRegionHolding(t, events, regionBoard, "changed after the page")
}
