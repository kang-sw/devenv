package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// Exercise actual rail frames with connected-DOM writes and geometry/style
// reads instrumented. Detached animation copies do not dirty page layout.
func TestRailLayoutReadsBeforeWrites(t *testing.T) {
	data, err := assetFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := railPrelude + `
var listeners = {}, windowListeners = {}, mutation = null, disconnected = false;
var anims = [], insetReads = 0, checking = false, wrote = false, lateReads = [];
function connected(e) { for (; e; e = e.parent) if (e === board || e === document.body) return true; return false; }
function write(e) { if (checking && connected(e)) wrote = true; }
function read(what) { if (checking && wrote) lateReads.push(what); }
var OriginalE = E;
E = function (tag, cls, attrs) {
  OriginalE.call(this, tag, cls, attrs);
  instrument(this);
};
E.prototype = OriginalE.prototype;
// Instrument the already constructed board and topics as well.
function instrument(e) {
  ["hidden", "textContent"].forEach(function (key) {
    var value = e[key];
    Object.defineProperty(e, key, { get: function () { return value; }, set: function (v) { write(e); value = v; } });
  });
  ["add", "remove", "toggle"].forEach(function (key) {
    var f = e.classList[key]; e.classList[key] = function () { write(e); return f.apply(this, arguments); };
  });
  var style = e.style;
  e.style = new Proxy(style, {
    set: function (o, k, v) { write(e); o[k] = v; return true; },
    get: function (o, k) { return k === "setProperty" ? function () { write(e); return o[k].apply(o, arguments); } : o[k]; }
  });
  e.children.forEach(instrument);
}
instrument(board);
var rect = E.prototype.getBoundingClientRect;
E.prototype.getBoundingClientRect = function () { read("rect"); return rect.call(this); };
var height = Object.getOwnPropertyDescriptor(E.prototype, "offsetHeight").get;
// The original fixture's descriptor is non-configurable; use a derived
// prototype for both the existing elements and ones the rail creates.
var measured = Object.create(E.prototype);
Object.defineProperty(measured, "offsetHeight", { get: function () { read("height"); return height.call(this); } });
function measuredTree(e) { Object.setPrototypeOf(e, measured); e.children.forEach(measuredTree); }
measuredTree(board); E.prototype = measured;
["appendChild", "insertBefore", "setAttribute"].forEach(function (key) {
  var f = E.prototype[key]; E.prototype[key] = function () { write(this); return f.apply(this, arguments); };
});
E.prototype.animate = function () { write(this); anims.push(this); return {}; };
E.prototype.cloneNode = function () { var c = new E(this.tag); c.cls = Object.assign({}, this.cls); return c; };
E.prototype.removeAttribute = function (k) { write(this); delete this.attrs[k]; };
E.prototype.removeChild = function (c) { write(this); this.children.splice(this.children.indexOf(c), 1); c.parent = null; };
Object.defineProperty(E.prototype, "parentNode", { get: function () { return this.parent; } });
Object.defineProperty(E.prototype, "lastChild", { get: function () { return this.children[this.children.length - 1] || null; } });
document.body.animate = function () {};
board.addEventListener = function (t, f) { listeners[t] = f; };
window.addEventListener = function (t, f) { windowListeners[t] = f; };
window.MutationObserver = function (f) { mutation = f; this.observe = function () {}; this.disconnect = function () { disconnected = true; }; };
var style = getComputedStyle;
getComputedStyle = function (e) {
  read("style"); var cs = style(e), prop = cs.getPropertyValue;
  cs.getPropertyValue = function (k) { if (k === "--wb-rail-inset") insetReads++; return prop(k); };
  return cs;
};
var tick = frame;
frame = function () { wrote = false; checking = true; tick(); checking = false; };
function scroll(y) { board.scrollTop = y; listeners.scroll(); frame(); }
function ghosts() { return anims.filter(function (e) { return e.cls["wb-rail-ghost"]; }).length; }
` + string(data) + `
frame();
check("initial inset read", insetReads, 1);
scroll(10); frame();
scroll(20); frame();
check("scrolls reuse inset", insetReads, 1);
check("shadow toggles without late height read", lateReads, []);
anims = []; scroll(500);
check("redraw does not animate before layout", anims.length, 0);
frame();
check("both edge morphs survive deferral", ghosts(), 2);
check("destinations all read before animation writes", lateReads, []);
// Continuous scroll must consume pending motion in an unchanged draw,
// not cancel it every frame until scrolling stops.
scroll(0); ro(); frame(); anims = [];
scroll(500);
check("crossing defers morphs", ghosts(), 0);
scroll(510);
check("continuous scroll plays both morphs", ghosts(), 2);
scroll(520);
check("continuous scroll does not replay morphs", ghosts(), 2);
check("folded destination reads precede writes", lateReads, []);
// A genuinely different destination supersedes the pending measurement.
anims = []; scroll(0); var stale = queue[queue.length - 1].id;
scroll(500);
check("new run cancels stale destination", cancelled[stale], true);
check("new run waits for its own destination", anims.length, 0);
frame();
check("only latest destination starts", ghosts(), 2);
// Size and explicit host-input changes refresh the cached inset.
var cachedReads = insetReads;
railInset = "34px"; windowListeners.resize(); frame();
check("resize refreshes inset", insetReads, cachedReads + 1);
check("resize updates offset", oqTop.props["--wb-rail-h"], "58px");
railInset = "66px"; mutation(); frame();
check("late host style refreshes inset", insetReads, cachedReads + 2);
check("late host style updates offset", oqTop.props["--wb-rail-h"], "90px");
// The unchanged-key/fresh path must read a resized head before offsets.
ro(); frame();
check("all redraw paths read before writes", lateReads, []);
// Crossing into the next topic and then toggling only its stuck shadow
// must retain the morph, even though the full draw key changes.
railInset = "";
board.querySelectorAll("section.topic").forEach(function (t) { t.h = 588; });
board.scrollTop = 574; ro(); frame(); frame(); anims = [];
scroll(575);
check("boundary crossing defers morph", ghosts(), 0);
scroll(577);
check("shadow-only change retains pending morph", ghosts(), 1);
frame();
check("shadow-only change plays morph once", ghosts(), 1);
check("shadow-only destination reads precede writes", lateReads, []);
// Teardown must cancel a destination frame as well as a redraw frame.
scroll(0); anims = []; window.wbRail.off(); frame();
check("off cancels pending motion", anims.length, 0);
check("off disconnects inset observer", disconnected, true);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("rail layout cases failed:\n%s", s)
	}
}
