package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// pillSegPrelude is a page for pill.js under node whose elements answer the
// class selectors the pill uses: a board host, five items laid out above or
// below an 800px view, an IntersectionObserver whose callback the cases fire
// by hand, and a wsT that returns the key.
const pillSegPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function E(tag) { this.tag = tag; this.children = []; this.style = {}; this.attrs = {}; this.cls = {}; this.q = {}; this.parent = null; this.top = 0; this.hidden = false; this.textContent = ""; var me = this;
  this.classList = { contains: function (c) { return !!me.cls[c]; } }; }
Object.defineProperty(E.prototype, "className", { set: function (v) { var me = this; me.cls = {}; v.split(" ").forEach(function (c) { if (c) me.cls[c] = true; }); } });
E.prototype.all = function (out) { this.children.forEach(function (c) { out.push(c); c.all(out); }); return out; };
E.prototype.querySelectorAll = function (sel) { if (sel in this.q) return this.q[sel]; var c = sel.slice(1); return this.all([]).filter(function (e) { return e.cls[c]; }); };
E.prototype.querySelector = function (sel) { if (sel in this.q) return this.q[sel][0] || null; if (sel[0] !== ".") return null; return this.querySelectorAll(sel)[0] || null; };
E.prototype.closest = function (sel) { for (var e = this; e; e = e.parent) if (e.cls[sel.slice(1)]) return e; return null; };
E.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
E.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
E.prototype.hasAttribute = function (k) { return k in this.attrs; };
E.prototype.appendChild = function (c) { c.parent = this; this.children.push(c); return c; };
E.prototype.insertBefore = function (c) { c.parent = this; this.children.unshift(c); return c; };
Object.defineProperty(E.prototype, "firstChild", { get: function () { return this.children[0] || null; } });
E.prototype.addEventListener = function () {};
E.prototype.removeEventListener = function () {};
E.prototype.getBoundingClientRect = function () { return { top: this.top, bottom: this.top + 40, width: 100, height: 40 }; };
E.prototype.scrollIntoView = function () { jumped.push(this.attrs.id); };
var fire = null, jumped = [];
function IntersectionObserver(cb) { fire = cb; this.observe = function () {}; this.disconnect = function () {}; }
function getComputedStyle() { return { overflowY: "visible" }; }
function wsT(k) { return k; }
function item(id, top, state) { var e = new E("div"); e.attrs.id = id; e.top = top;
  if (state === "pick" || state === "note") e.q['.opt[data-opt][aria-pressed="true"]'] = [new E("button")];
  if (state === "note") { var n = new E("textarea"); n.value = "x"; e.q["textarea.note"] = [n]; }
  return e; }
// Above the view: open a1, answered with a note a2, open a3. Below: answered
// b1, open b2.
var items = [item("a1", -300, ""), item("a2", -200, "note"), item("a3", -100, ""), item("b1", 900, "pick"), item("b2", 1000, "")];
var host = new E("div"), html = new E("html");
var document = { body: new E("body"), documentElement: html, title: "Board", getElementById: function (id) { return id === "bcol" ? host : null; },
  querySelector: function () { return null; }, querySelectorAll: function () { return items; }, createElement: function (t) { return new E(t); } };
var window = { IntersectionObserver: IntersectionObserver, innerHeight: 800, addEventListener: function () {}, removeEventListener: function () {} };
function pill(i) { return host.children[i].children[0]; }
function shown(p) { return p.hidden ? null : p.querySelectorAll(".oq-seg").filter(function (b) { return !b.hidden; }).map(function (b) { return [b.getAttribute("data-k"), b.children[0].textContent, b.querySelector(".n").textContent]; }); }
function press(p, k) { var b = p.querySelectorAll(".oq-seg").filter(function (b) { return b.getAttribute("data-k") === k; })[0]; p.onclick({ target: b.children[1] }); }
`

// TestPillSegments pins the open-question pill's per-state segments: one
// button per state with a non-zero count (open, answered, noted, in that
// order), a label apart from its count, a per-segment tooltip, and a click on
// a segment jumping to the nearest item of that state at its edge. The cases
// execute under node when it is installed.
func TestPillSegments(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; pill segment cases not executed")
	}
	script := pillSegPrelude + string(data) + `
fire([]);
var top = pill(0), bot = pill(1);
check("top segments", shown(top), [["open", "js.pill.open", "2"], ["ans", "js.pill.answered", "1"], ["nt", "js.pill.noted", "1"]]);
check("bottom segments", shown(bot), [["open", "js.pill.open", "1"], ["ans", "js.pill.answered", "1"]]);
check("tooltips", top.querySelectorAll(".oq-seg").map(function (b) { return b.title; }), ["js.pill.up", "js.pill.upanswered", "js.pill.upnoted"]);
check("bottom tooltips", bot.querySelectorAll(".oq-seg").map(function (b) { return b.title; }), ["js.pill.down", "js.pill.downanswered", "js.pill.downnoted"]);
check("arrows", [top.children[0].textContent, bot.children[0].textContent], ["↑", "↓"]);
press(top, "open"); press(top, "ans"); press(top, "nt"); press(bot, "open"); press(bot, "ans");
check("jumps", jumped, ["a3", "a2", "a2", "b2", "b1"]);
fire(items.map(function (el) { return { isIntersecting: true, target: el }; }));
check("all in view hides both pills", [top.hidden, bot.hidden], [true, true]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("pill segment cases failed:\n%s", s)
	}
}
