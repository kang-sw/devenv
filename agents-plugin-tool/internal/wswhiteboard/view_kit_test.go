package wswhiteboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// viewKitPrelude is a fake window for stream.js's viewKit: a virtual clock,
// IntersectionObserver and ResizeObserver doubles the cases drive by hand,
// and elements laid out in their scroller's content (y), each one's rect
// read from its scroller's top, scroll offset, and the transforms between.
const viewKitPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var now = 0, timers = [], tid = 0;
function setTimeout(fn, ms) { var id = ++tid; timers.push({ id: id, at: now + (ms || 0), fn: fn }); return id; }
function clearTimeout(id) { timers = timers.filter(function (t) { return t.id !== id; }); }
function advance(ms) {
  var end = now + ms;
  for (;;) {
    timers.sort(function (a, b) { return a.at - b.at || a.id - b.id; });
    var t = timers[0];
    if (!t || t.at > end) break;
    timers.shift(); now = t.at; t.fn();
  }
  now = end;
}
function El(name) { this.name = name; this.children = []; this.parentElement = null; this.style = {}; this.oy = "visible"; this.ty = 0; this.y = 0; this.top = 0;
  this.scrollTop = 0; this.scrollHeight = 0; this.clientHeight = 0; this.clientTop = 0; this.connected = true; this.on = {}; }
Object.defineProperty(El.prototype, "isConnected", { get: function () { return this.connected; } });
El.prototype.add = function (c) { c.parentElement = this; this.children.push(c); return c; };
El.prototype.addEventListener = function (t, f) { (this.on[t] = this.on[t] || []).push(f); };
El.prototype.removeEventListener = function (t, f) { var l = this.on[t] || [], i = l.indexOf(f); if (i >= 0) l.splice(i, 1); };
El.prototype.fire = function (t, e) { (this.on[t] || []).slice().forEach(function (f) { f(e || {}); }); };
// A scroller's rect top is its own top (0 for the document's); any other
// element's is its scroller's content top plus its y, less the scroll,
// plus every transform from it up to the scroller.
El.prototype.getBoundingClientRect = function () {
  if (!this.sc) return { top: this.top };
  var y = (this.sc === html ? 0 : this.sc.top) + this.y - this.sc.scrollTop;
  for (var p = this; p && p !== this.sc; p = p.parentElement) y += p.ty;
  return { top: y };
};
var html = new El("html"), body = html.add(new El("body"));
var document = { documentElement: html, body: body, scrollingElement: html };
var winOn = {};
var ios = [];
function IO(cb, opt) { this.cb = cb; this.opt = opt; this.obs = []; this.log = []; ios.push(this); }
IO.prototype.observe = function (n) { this.obs.push(n); this.log.push("observe:" + n.name); };
IO.prototype.unobserve = function (n) { this.obs.splice(this.obs.indexOf(n), 1); this.log.push("unobserve:" + n.name); };
IO.prototype.report = function (n, ratio) { this.cb([{ target: n, intersectionRatio: ratio, boundingClientRect: { top: ratio } }]); };
var ros = [];
function RO(cb) { this.cb = cb; this.obs = []; this.off = false; ros.push(this); }
RO.prototype.observe = function (n) { this.obs.push(n.name); };
RO.prototype.disconnect = function () { this.off = true; };
RO.prototype.fire = function () { if (!this.off) this.cb([]); };
var win = {
  document: document, IntersectionObserver: IO, ResizeObserver: RO, setTimeout: setTimeout, clearTimeout: clearTimeout,
  getComputedStyle: function (el) { return { overflowY: el.oy, transform: el.ty ? "matrix(1, 0, 0, 1, 0, " + el.ty + ")" : "none" }; },
  addEventListener: function (t, f) { (winOn[t] = winOn[t] || []).push(f); },
  removeEventListener: function (t, f) { var l = winOn[t] || [], i = l.indexOf(f); if (i >= 0) l.splice(i, 1); }
};
function winFire(t, e) { (winOn[t] || []).slice().forEach(function (f) { f(e || {}); }); }
// The board column scrolls (overflow auto, content taller than it); the
// pinned column scrolls on its own; a plain column and a column whose
// content fits leave the scroll to the document.
var bcol = body.add(new El("bcol")); bcol.oy = "auto"; bcol.scrollHeight = 4000; bcol.clientHeight = 500; bcol.top = 50;
var board = bcol.add(new El("board"));
var bpins = body.add(new El("bpins")); bpins.oy = "auto"; bpins.scrollHeight = 2000; bpins.clientHeight = 400; bpins.top = 60;
var plain = body.add(new El("plain"));
var fits = plain.add(new El("fits")); fits.oy = "auto"; fits.scrollHeight = 100; fits.clientHeight = 200;
html.scrollHeight = 5000; html.clientHeight = 800;
function topic(parent, name, sc, y) { var n = parent.add(new El(name)); n.sc = sc; n.y = y; return n; }
`

// TestViewKitUnderNode runs stream.js's viewKit (the shared visibility source
// and keep-in-place helper on wbHistory) against the fake window: the
// nearest scroller (a scrolling column, the pinned column, else the
// document), the layout-term measure, the session's end rules (500 ms
// quiet, 5 s cap, a scroll it did not write), one session per scroller
// with re-anchoring, overflow-anchor during a session, and the watch
// bookkeeping (subscriptions per node, onOut on a ratio > 0 to 0 change
// only, sweep of removed nodes, one observer per margin, seen), and the
// scroll margin each observer takes where the browser has one (bands). The cases
// execute under node when it is installed.
func TestViewKitUnderNode(t *testing.T) {
	data, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	kit := regexp.MustCompile(`(?s)\n  function viewKit\(win\) \{.*?\n  \}\n`).FindString(string(data))
	if kit == "" {
		t.Fatal("stream.js holds no viewKit")
	}
	if !strings.Contains(string(data), "watch: kit.watch, unwatch: kit.unwatch, sweep: kit.sweep, seen: kit.seen, keep: kit.keep, keeping: kit.keeping") {
		t.Fatal("wbHistory does not carry the kit's helpers")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; view kit cases not executed")
	}
	script := viewKitPrelude + kit + `
var k = viewKit(win), ends = [];
function onEnd(tag) { return function (r) { ends.push(tag + ":" + r); }; }

// --- nearest scroller ---
var a = topic(board, "a", bcol, 1000), p = topic(bpins, "p", bpins, 300), d = topic(plain, "d", html, 900), f = topic(fits, "f", html, 50);
[[a, bcol], [p, bpins], [d, html], [f, html]].forEach(function (c) {
  var h = k.keep(c[0]);
  check("scroller of " + c[0].name, h.scroller.name, c[1].name);
  h.end();
});
check("no session left", [k.keeping(bcol), k.keeping(bpins), k.keeping(html)], [null, null, null]);

// --- layout-term measure and apply ---
bcol.scrollTop = 900; a.ty = -40;
var h = k.keep(a, { onEnd: onEnd("a") });
check("keeping returns the session", k.keeping(bcol) === h, true);
check("scrolling column takes overflow-anchor none", bcol.style.overflowAnchor, "none");
a.ty = 0; a.y = 1300; h.apply();
check("apply keeps the anchor's layout offset (100), its slide not counted", bcol.scrollTop, 1200);
check("session observes the scroller's content", ros[ros.length - 1].obs, ["board"]);

// --- quiet: 500 ms after the last layout change ---
advance(400); a.y = 1350; ros[ros.length - 1].fire();
check("a layout change re-applies", bcol.scrollTop, 1250);
advance(400);
check("a layout change restarts the quiet window", k.keeping(bcol) === h, true);
advance(100);
check("500 ms with no layout change ends it", [k.keeping(bcol), ends], [null, ["a:quiet"]]);
check("overflow-anchor goes back", bcol.style.overflowAnchor, undefined);
check("an ended session's observer is off", ros[ros.length - 1].off, true);

// --- cap: 5 s whatever changes ---
ends = [];
h = k.keep(a, { onEnd: onEnd("a") });
for (var i = 0; i < 13; i++) { advance(400); ros[ros.length - 1].fire(); }
check("the cap ends a session that never quiets", ends, ["a:cap"]);

// --- user scroll ---
ends = [];
h = k.keep(a, { onEnd: onEnd("a") });
a.y = 1500; h.apply(); bcol.fire("scroll");
check("the session's own write is no user scroll", [k.keeping(bcol) === h, ends], [true, []]);
bcol.scrollTop += 80; bcol.fire("scroll");
check("a scroll the session did not write ends it", [k.keeping(bcol), ends], [null, ["a:user"]]);

// --- one session per scroller: a second keep re-anchors the first ---
ends = []; var nRO = ros.length;
var b = topic(board, "b", bcol, 2000), extra = board.add(new El("extra"));
var h1 = k.keep(a, { onEnd: onEnd("a") });
advance(400);
var h2 = k.keep(b, { offset: 20, observe: [extra], onEnd: onEnd("b") });
check("the second keep returns the running session", [h2 === h1, ros.length - nRO], [true, 1]);
check("its offset puts the new anchor there at once", bcol.scrollTop, 2000 - 20 - 0 + 0);
check("its observed nodes join the session", ros[ros.length - 1].obs, ["board", "extra"]);
bcol.fire("scroll");
check("neither call's write is a user scroll", k.keeping(bcol) === h1, true);
advance(400);
check("the quiet window restarted from the second call", k.keeping(bcol) === h1, true);
b.y = 2100; ros[ros.length - 1].fire();
check("the session holds the new anchor", bcol.scrollTop, 2100 - 20);
bcol.scrollTop = 10; bcol.fire("scroll");
check("an outside write still ends it, for both callers", ends, ["a:user", "b:user"]);

// --- the document scroller ---
ends = [];
html.scrollTop = 300;
h = k.keep(d, { onEnd: onEnd("d") });
check("document session", [h.scroller === html, html.style.overflowAnchor, ros[ros.length - 1].obs], [true, "none", ["body"]]);
d.y = 1100; h.apply();
check("document compensation", html.scrollTop, 500);
winFire("scroll");
check("own write on the document is no user scroll", k.keeping(html) === h, true);
html.scrollTop = 0; winFire("scroll");
check("a user scroll of the document ends it and restores anchoring", [ends, html.style.overflowAnchor], [["d:user"], undefined]);

// --- an anchor taken off the page ends the session ---
ends = [];
h = k.keep(a, { onEnd: onEnd("a") });
a.connected = false; h.apply();
check("a removed anchor ends the session", ends, ["a:ended"]);
a.connected = true;

// --- watch ---
var reports = [], outs = [];
var w1 = k.watch(a, { onChange: function (r) { reports.push("1:" + r.ratio + ":" + r.visible); }, onOut: function (r) { outs.push("1:" + r.node.name); } });
var io = ios[0];
check("one observer, root null, threshold 0", [ios.length, io.opt.root, io.opt.rootMargin, io.opt.threshold], [1, null, "0px", [0]]);
io.report(a, 0);
check("a first report of 0 is no onOut", [reports, outs], [["1:0:false"], []]);
var w2 = k.watch(a, { onOut: function (r) { outs.push("2:" + r.node.name); } });
check("a second subscription re-observes the node for its first report", io.log.slice(-3), ["observe:a", "unobserve:a", "observe:a"]);
io.report(a, 0.5);
io.report(a, 0);
check("both subscriptions see the node leave view, once", outs, ["1:a", "2:a"]);
io.report(a, 0);
check("no second onOut without coming back", outs.length, 2);
check("seen reads the last report", [k.seen(a).ratio, k.seen(a).visible, k.seen(a).node === a], [0, false, true]);
check("seen of an unwatched node", k.seen(b), null);
k.unwatch(w1);
io.report(a, 0.3);
check("an unwatched subscription hears nothing", reports.length, 4);
check("the node stays observed for the other one", io.obs.indexOf(a) >= 0, true);
w2.unwatch();
check("the last subscription leaving unobserves the node", io.obs.indexOf(a), -1);
var gone = topic(board, "gone", bcol, 0), kept = topic(board, "kept", bcol, 0), wide = 0;
k.watch(gone); k.watch(kept); k.watch(gone, { margin: "200px", onChange: function () { wide++; } });
check("one observer per margin", [ios.length, ios[1].opt.rootMargin], [2, "200px"]);
gone.connected = false; k.sweep();
check("sweep drops the removed node from every observer", [io.obs.map(function (n) { return n.name; }), ios[1].obs.length], [["kept"], 0]);
ios[1].report(gone, 1);
check("a swept subscription hears nothing", wide, 0);
// A subscription dropped by another one's callback in the same report
// hears nothing more.
var x = topic(board, "x", bcol, 0), heard = [], wy;
k.watch(x, { onChange: function () { heard.push("first"); wy.unwatch(); } });
wy = k.watch(x, { onChange: function () { heard.push("second"); } });
io.report(x, 1);
check("a subscription dropped mid-report is not called", heard, ["first"]);

// --- scroll margins ---
// A browser without scroll margins gets no bands; one with them gets each
// margin as both the root and the scroll margin, so a margin past the view
// reaches through an inner scroller's clip.
check("no scroll margin, no bands", [k.bands, "scrollMargin" in io.opt], [false, false]);
function SIO(cb, opt) { IO.call(this, cb, opt); }
SIO.prototype = Object.create(IO.prototype); SIO.prototype.scrollMargin = "";
win.IntersectionObserver = SIO;
var kb = viewKit(win), y = topic(board, "y", bcol, 0);
kb.watch(y, { margin: "100% 0px" });
var sio = ios[ios.length - 1];
check("scroll margins give bands", kb.bands, true);
check("the margin is both root and scroll margin", [sio.opt.root, sio.opt.rootMargin, sio.opt.scrollMargin], [null, "100% 0px", "100% 0px"]);
win.IntersectionObserver = IO;
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("view kit cases failed:\n%s", s)
	}
}

// TestViewKitJumpFollowUnderNode drives the real jump helper with layout
// shifts, a smooth-scroll double and virtual animation frames. It authors
// the bounded follow contract without requiring a browser or live daemon.
func TestViewKitJumpFollowUnderNode(t *testing.T) {
	data, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	kit := regexp.MustCompile(`(?s)\n  function viewKit\(win\) \{.*?\n  \}\n`).FindString(string(data))
	if kit == "" || !strings.Contains(string(data), "jump: kit.jump") {
		t.Fatal("stream.js does not expose the jump helper")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; jump follow cases not executed")
	}
	script := viewKitPrelude + kit + `
win.requestAnimationFrame = function (fn) { return setTimeout(function () { fn(now); }, 16); };
win.cancelAnimationFrame = clearTimeout;
var reduced = false, animate = false, jumps = [], stopped = 0;
win.matchMedia = function () { return { matches: reduced }; };
var rect = El.prototype.getBoundingClientRect;
El.prototype.getBoundingClientRect = function () { var r = rect.call(this); r.height = this.height || 40; return r; };
El.prototype.scrollIntoView = function (opt) {
  jumps.push([this.name, opt.block, opt.behavior]);
  if (animate && opt.behavior === "smooth") return;
  var s = this.sc, offset = opt.block === "center" ? (s.clientHeight - (this.height || 40)) / 2 : 0;
  var top = this.getBoundingClientRect().top - (s === html ? 0 : s.top) + s.scrollTop;
  s.scrollTop = Math.max(0, Math.min(s.scrollHeight - s.clientHeight, top - offset));
};
El.prototype.scrollTo = function (opt) { stopped++; this.scrollTop = opt.top; };
var k = viewKit(win), a = topic(board, "a", bcol, 1000), b = topic(board, "b", bcol, 2200), ends = [];
var h = k.keep(a, { onEnd: function (r) { ends.push(r); } });
k.jump(a, { block: "center", behavior: "smooth" });
check("initial jump retains smooth", jumps, [["a", "center", "smooth"]]);
check("jump centers in the column", bcol.scrollTop, 770);
check("a swap cannot re-anchor the active jump", k.keep(b, { offset: 12 }) === h, true);
h.end(); // A board swap finding no surviving viewed panel must not end it.
advance(80); a.y += 200; ros[ros.length - 1].fire();
check("late layout is corrected instantly, not by the old viewed panel", [bcol.scrollTop, jumps[jumps.length - 1]], [970, ["a", "center", "instant"]]);
advance(149);
check("no early settlement before 150ms quiet", k.keeping(bcol) === h, true);
advance(100);
check("settled jump restores anchoring and stops its work", [k.keeping(bcol), bcol.style.overflowAnchor, ends, timers.length, (winOn.keydown || []).length], [null, undefined, ["settled"], 0, 0]);
a.y += 100; advance(300);
check("layout after settlement is not followed", bcol.scrollTop, 970);
h = k.keep(b, { offset: 20 }); b.y += 30; h.apply();
check("ordinary viewed-panel anchoring resumes unchanged", bcol.scrollTop, 2210);
h.end();

// Smooth motion alone must not look like layout movement or settle early.
animate = true; jumps = [];
k.jump(a, { behavior: "smooth" });
for (var i = 0; i < 8; i++) { bcol.scrollTop -= 20; bcol.fire("scroll"); advance(16); }
check("native smooth motion is neither corrected nor cancelled", [jumps.length, !!k.keeping(bcol)], [1, true]);
a.y += 60; advance(16);
check("layout during smooth motion gets an instant correction", jumps[jumps.length - 1], ["a", "center", "instant"]);
advance(250); animate = false;
check("smooth jump also settles", k.keeping(bcol), null);

// Continuous movement cannot extend the cap, even without ResizeObserver.
win.ResizeObserver = null;
k.jump(a);
for (var i = 0; i < 24; i++) { a.y += 10; advance(80); }
a.y += 10; advance(79);
check("still following just before 2s", !!k.keeping(bcol), true);
advance(1);
check("hard 2s cap and cleanup", [k.keeping(bcol), timers.length, (bcol.on.wheel || []).length], [null, 0, 0]);

// Every intent cancels before a scroll event; a non-scrolling key does not.
["wheel", "touchstart", "touchmove", "pointerdown"].forEach(function (type) {
  k.jump(a, { behavior: "smooth" });
  bcol.fire(type);
  var before = bcol.scrollTop;
  a.y += 100; advance(250);
  check(type + " wins immediately", [k.keeping(bcol), bcol.scrollTop, timers.length], [null, before, 0]);
});
["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "PageUp", "PageDown", "Home", "End", " ", "Spacebar"].forEach(function (key) {
  k.jump(a); winFire("keydown", { key: "x" });
  check("non-scroll key leaves navigation alone", !!k.keeping(bcol), true);
  winFire("keydown", { key: key });
  check(key + " cancels", [k.keeping(bcol), timers.length], [null, 0]);
});
check("intent also interrupts native smooth scrolling", stopped, 14);

// Reduced motion, boundary clamping, replacement/removal, document root,
// pinned root and a second jump all use the same bounded lifecycle.
reduced = true; k.jump(a, { behavior: "smooth" });
check("reduced motion starts instantly", jumps[jumps.length - 1][2], "instant");
bcol.fire("wheel"); reduced = false;
a.y = 10; k.jump(a); advance(250);
check("a clamped edge settles instead of fighting", [bcol.scrollTop, k.keeping(bcol)], [0, null]);
a.y = 1000; a.ty = -40; k.jump(a); a.ty = 0; advance(16);
check("a rearrangement slide is followed in visible, not just layout, coordinates", bcol.scrollTop, 770);
bcol.fire("wheel");
k.jump(a); advance(80);
var replacement = topic(board, "replacement", bcol, 1300);
a.id = "item-1"; a.connected = false;
document.getElementById = function (id) { return id === "item-1" ? replacement : null; };
advance(16);
check("a morph's same-ID replacement is followed", bcol.scrollTop, 1070);
replacement.connected = false; advance(16);
check("a removed target ends cleanly", [k.keeping(bcol), timers.length], [null, 0]);
var d = topic(plain, "document", html, 900), p = topic(bpins, "pin", bpins, 700);
k.jump(d); d.y += 100; advance(16);
check("document scroller correction", html.scrollTop, 620);
winFire("touchstart");
k.jump(p, { block: "start" }); p.y += 80; advance(16);
check("pinned start alignment", bpins.scrollTop, 780);
bpins.fire("pointerdown");
a.connected = true; k.jump(a); advance(80); k.jump(b); advance(150);
check("a second jump owns a fresh settle window", !!k.keeping(bcol), true);
advance(100);
check("second jump leaves no timers or input hooks", [k.keeping(bcol), timers.length, (winOn.keydown || []).length], [null, 0, 0]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("jump follow cases failed:\n%s", s)
	}
}
