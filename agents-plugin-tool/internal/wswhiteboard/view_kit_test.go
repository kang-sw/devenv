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
El.prototype.fire = function (t) { (this.on[t] || []).slice().forEach(function (f) { f(); }); };
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
function winFire(t) { (winOn[t] || []).slice().forEach(function (f) { f(); }); }
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
// only, sweep of removed nodes, one observer per margin, seen). The cases
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
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("view kit cases failed:\n%s", s)
	}
}
