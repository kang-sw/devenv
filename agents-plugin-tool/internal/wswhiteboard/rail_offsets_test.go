package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// nodeScript runs script under node, fed on stdin rather than with -e:
// Windows caps a command line at 32767 characters, and the scripts embed
// whole assets.
func nodeScript(node, script string) *exec.Cmd {
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(script)
	return cmd
}

// railPrelude is a page for rail.js under node: a mini DOM whose elements
// answer the compound selectors the rail uses (tag, classes, attributes,
// descendant and child combinators, :scope), a board column that scrolls,
// and topics laid out by a top offset the cases move. Every style write is
// logged so the cases can tell which element carries which offset.
const railPrelude = `
var bad = [], writes = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function E(tag, cls, attrs) { this.tag = tag; this.children = []; this.parent = null; this.attrs = attrs || {}; this.cls = {}; this.hidden = false; this.h = 0; this.y = 0; this.textContent = "";
  var me = this; (cls || "").split(" ").forEach(function (c) { if (c) me.cls[c] = true; });
  this.classList = { contains: function (c) { return !!me.cls[c]; }, add: function (c) { me.cls[c] = true; }, remove: function (c) { delete me.cls[c]; }, toggle: function (c, on) { if (on) me.cls[c] = true; else delete me.cls[c]; } };
  this.props = {};
  this.style = { setProperty: function (k, v) { writes.push([me.name || me.tag, k, v]); me.props[k] = v; } }; }
Object.defineProperty(E.prototype, "className", { set: function (v) { var me = this; me.cls = {}; v.split(" ").forEach(function (c) { if (c) me.cls[c] = true; }); } });
Object.defineProperty(E.prototype, "innerHTML", { set: function (v) { var me = this; me.children = []; v.replace(/<span class="([^"]+)"/g, function (_, c) { me.appendChild(new E("span", c)); return ""; }); }, get: function () { return ""; } });
Object.defineProperty(E.prototype, "firstChild", { get: function () { return this.children[0] || null; } });
Object.defineProperty(E.prototype, "offsetHeight", { get: function () { return this.hidden ? 0 : this.h; } });
Object.defineProperty(E.prototype, "isConnected", { get: function () { return true; } });
E.prototype.appendChild = function (c) { c.parent = this; this.children.push(c); return c; };
E.prototype.insertBefore = function (c) { c.parent = this; this.children.unshift(c); return c; };
E.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
E.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
E.prototype.hasAttribute = function (k) { return k in this.attrs; };
E.prototype.closest = function (s) { for (var e = this; e; e = e.parent) if (one(e, s)) return e; return null; };
E.prototype.addEventListener = function () {};
E.prototype.removeEventListener = function () {};
E.prototype.getBoundingClientRect = function () { var t = this.y - (this.scroller ? 0 : board.scrollTop); return { top: t, bottom: t + this.h, left: 0, width: 600, height: this.h }; };
function one(e, p) { var m = /^([a-z]*)((?:[.#][\w-]+|\[[\w-]+(?:="[^"]*")?\])*)$/.exec(p); if (!m) throw new Error("selector " + p);
  if (m[1] && e.tag !== m[1]) return false; var ok = true;
  m[2].replace(/\.([\w-]+)|\[([\w-]+)(?:="([^"]*)")?\]/g, function (_, c, a, v) { if (c && !e.cls[c]) ok = false; if (a && (!(a in e.attrs) || (v !== undefined && e.attrs[a] !== v))) ok = false; return ""; });
  return ok; }
function desc(e, out) { e.children.forEach(function (c) { out.push(c); desc(c, out); }); return out; }
function sel(root, s) { var out = [];
  s.split(",").forEach(function (alt) { var parts = alt.trim().split(/\s+/), set = [root], i = 0;
    if (parts[0] === ":scope") i = 1;
    for (; i < parts.length; i++) { var child = parts[i] === ">"; if (child) i++; var p = parts[i], next = [];
      set.forEach(function (e) { (child ? e.children : desc(e, [])).forEach(function (c) { if (one(c, p) && next.indexOf(c) < 0) next.push(c); }); }); set = next; }
    set.forEach(function (e) { if (e !== root && out.indexOf(e) < 0) out.push(e); }); });
  var all = desc(root, []); return out.sort(function (a, b) { return all.indexOf(a) - all.indexOf(b); }); }
E.prototype.querySelectorAll = function (s) { return sel(this, s); };
E.prototype.querySelector = function (s) { return sel(this, s)[0] || null; };
var board = new E("div", "bcol", { id: "bcol" }); board.name = "host"; board.h = 800; board.scrollTop = 0; board.scrollHeight = 4000; board.clientHeight = 800; board.scroller = true;
var oqTop = board.appendChild(new E("div", "oq-wrap top")), oqBot = board.appendChild(new E("div", "oq-wrap bot")); oqTop.name = "oq-top"; oqBot.name = "oq-bot";
function topic(key, y) { var t = board.appendChild(new E("section", "topic", { "data-topic": key })); t.y = y; t.h = 500; var head = t.appendChild(new E("div", "tophead")); head.name = "head-" + key; head.h = 30; head.y = y; var bar = head.appendChild(new E("div", "tbar")); bar.name = "bar-" + key; bar.h = 30; bar.y = y; bar.appendChild(new E("span", "tname")).textContent = key; return t; }
["a", "b", "c", "d", "e", "f"].forEach(function (k, i) { topic(k, i * 600); });
var queue = [], ro = null;
var document = { body: new E("body"), getElementById: function (id) { return id === "bcol" ? board : null; }, querySelector: function () { return null; }, createElement: function (t) { return new E(t); } };
var window = { requestAnimationFrame: function (f) { queue.push(f); return queue.length; }, innerHeight: 800, addEventListener: function () {}, removeEventListener: function () {}, ResizeObserver: function (cb) { ro = cb; this.observe = function () {}; this.disconnect = function () {}; } };
var requestAnimationFrame = window.requestAnimationFrame, cancelAnimationFrame = function () {}, ResizeObserver = window.ResizeObserver;
function getComputedStyle(e) { return { overflowY: e === board ? "auto" : "visible", paddingTop: "0", getPropertyValue: function () { return ""; } }; }
function frame() { var q = queue; queue = []; q.forEach(function (f) { f(); }); }
function hostWrites() { return writes.filter(function (w) { return w[0] === "host"; }); }
`

// TestRailOffsetsSkipTheHost pins where rail.js puts its offsets: each
// topic's head (.tophead: bar, note, and summary line, which stick as one)
// carries --wb-rail-top and a note none of its own, the open-question pill's wrappers carry
// --wb-rail-h and --wb-rail-bot, and the host carries none (a custom
// property on the host is inherited by every board node, so each write
// restyled the whole board). An unchanged value is not written again, and a
// bar the board gains on a re-query gets the current value even when the
// rail's draw does not change. The cases execute under node when it is
// installed.
func TestRailOffsetsSkipTheHost(t *testing.T) {
	data, err := assetFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; rail offset cases not executed")
	}
	script := railPrelude + string(data) + `
frame();
check("no host offset", hostWrites(), []);
check("first head at the line", board.querySelector(".tophead").props["--wb-rail-top"], "0px");
check("the bar inside the head carries none", board.querySelector(".tbar").props["--wb-rail-top"], undefined);
// Scroll into topic c: two capsules above (4 + 2 * 20 = 44), the current
// bar 30 tall, two topics below the bottom line.
board.scrollTop = 1250;
`
	// The rail schedules its run on scroll, which the stub does not fire;
	// a resize (the ResizeObserver callback) schedules it instead.
	script += `
writes = []; ro(); frame();
check("no host offset after scroll", hostWrites(), []);
check("every head sticks under two capsules", board.querySelectorAll("section.topic > .tophead").map(function (b) { return b.props["--wb-rail-top"]; }), ["44px", "44px", "44px", "44px", "44px", "44px"]);
check("pill top clears the stack and the head", oqTop.props["--wb-rail-h"], "74px");
check("pill bottom clears two capsules", oqBot.props["--wb-rail-bot"], (14 + 2 * 17 + 3 + 4) + "px");
writes = []; ro(); frame();
check("an unchanged draw writes nothing", writes.filter(function (w) { return /^--wb-rail/.test(w[1]); }), []);
// A collapsed topic (no height) is no rail item, so the draw stays the
// same, but its head is still the board's and gets the offset.
var g = topic("g", 3600); g.h = 0; writes = []; ro(); frame();
check("a new head gets the current offset", g.children[0].props["--wb-rail-top"], "44px");
check("only the new head is written", writes.filter(function (w) { return /^--wb-rail/.test(w[1]); }).map(function (w) { return w[0]; }), ["head-g"]);
// A note rides in its head, so it takes no offset of its own; the current
// head's whole height (a summary line under the bar) is what the pill
// clears.
var tc = board.querySelectorAll("section.topic")[2], nc = tc.children[0].appendChild(new E("textarea", "note tnote", { "data-topic": "c" })); nc.name = "note-c";
tc.children[0].h = 52; writes = []; ro(); frame();
check("a note gets no offset", nc.props["--wb-rail-note"], undefined);
check("a taller head moves the pill", oqTop.props["--wb-rail-h"], "96px");
check("only the pill is written", writes.filter(function (w) { return /^--wb-rail/.test(w[1]); }).map(function (w) { return w[0]; }), ["oq-top"]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("rail offset cases failed:\n%s", s)
	}
}
