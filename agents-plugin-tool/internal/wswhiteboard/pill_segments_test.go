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
var fire = null, jumped = [], observed = [];
function IntersectionObserver(cb) { fire = cb; this.observe = function (el) { observed.push(el); }; this.disconnect = function () {}; }
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
var followed = [];
window.wbHistory = { jump: function (el, opt) { followed.push([el.attrs.id, opt.block, opt.behavior]); } };
press(top, "open"); press(top, "ans"); press(top, "nt"); press(bot, "open"); press(bot, "ans");
check("every badge segment shares bounded jump following", followed, [["a3", "center", "smooth"], ["a2", "center", "smooth"], ["a2", "center", "smooth"], ["b2", "center", "smooth"], ["b1", "center", "smooth"]]);
check("delegated navigation does not scroll twice", jumped.length, 5);
window.wbHistory = null;
// A delayed visibility report cannot overrule the current positions.
fire(items.map(function (el) { return { isIntersecting: true, target: el }; }));
check("stale reports leave top counts alone", shown(top), [["open", "js.pill.open", "2"], ["ans", "js.pill.answered", "1"], ["nt", "js.pill.noted", "1"]]);
items.forEach(function (el) { el.top = 100; });
fire(items.map(function (el) { return { isIntersecting: false, target: el }; }));
check("current positions hide both pills despite stale reports", [top.hidden, bot.hidden], [true, true]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("pill segment cases failed:\n%s", s)
	}
}

// TestPillPendingCountIgnoresObserverDelivery reproduces the 9/11 count
// mechanism deterministically: eleven pending controls stay above the view,
// while delayed observer reports alternate the visibility of two of them.
// Reports are invalidations, never an alternate pending-item inventory.
func TestPillPendingCountIgnoresObserverDelivery(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; pill pending-count cases not executed")
	}
	script := pillSegPrelude + `
items = Array.from({ length: 11 }, function (_, i) { return item("pending-" + i, -1000 + i * 50, ""); });
` + string(data) + `
for (var i = 0; i < 40; i++) {
  fire(items.slice(0, 2).map(function (el) { return { isIntersecting: i % 2 === 0, target: el }; }));
  check("static pending count after report " + i, shown(pill(0)), [["open", "js.pill.open", "11"]]);
}
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("pill pending-count cases failed:\n%s", s)
	}
}

// TestPillReadsFarItemsByTheirTopic pins that the pill never reads the rect
// of an item in a topic wholly above or below the view: such an item takes
// its side from its topic's rect, read once per update, and one under a
// closed fold, or in a topic with no box, counts nowhere; an item in a
// topic crossing the view, or in no topic, reads its own. The cases
// execute under node when it is installed.
func TestPillReadsFarItemsByTheirTopic(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; pill topic cases not executed")
	}
	script := pillSegPrelude + `
Object.defineProperty(E.prototype, "parentElement", { get: function () { return this.parent; } });
E.prototype.closest = function (sel) { var tag = sel === "section.topic" ? "section" : null, c = tag ? "topic" : sel.slice(1);
  for (var e = this; e; e = e.parent) if (e.cls[c] && (!tag || e.tag === tag)) return e; return null; };
var reads = { item: [], topic: 0 };
function topic(top, h, kids) { var t = new E("section"); t.className = "topic"; t.top = top; t.h = h;
  t.getBoundingClientRect = function () { reads.topic++; return { top: this.top, bottom: this.top + this.h, width: this.h ? 100 : 0, height: this.h }; };
  kids.forEach(function (k) { t.appendChild(k); }); return t; }
var rawRect = E.prototype.getBoundingClientRect;
E.prototype.getBoundingClientRect = function () { if (this.attrs.id) reads.item.push(this.attrs.id); return rawRect.call(this); };
// Above: u1, u2 in one topic and u3 under a closed fold of it; v1 in a topic
// crossing the view's top; partially visible w1 in no topic; d1, d2 below; h1 in a
// topic with no box (not rendered).
var fold = new E("details"); fold.tagName = "DETAILS"; fold.open = false;
var u1 = item("u1", -900, ""), u2 = item("u2", -800, "pick"), u3 = item("u3", -700, ""); fold.appendChild(u3);
var v1 = item("v1", -50, ""), w1 = item("w1", -20, ""), d1 = item("d1", 1200, ""), d2 = item("d2", 1300, ""), h1 = item("h1", 2000, "");
topic(-1000, 600, [u1, u2, fold]); topic(-100, 400, [v1]); topic(1100, 500, [d1, d2]); topic(2000, 0, [h1]);
items.length = 0; items.push(u1, u2, u3, v1, w1, d1, d2, h1);
` + string(data) + `
reads = { item: [], topic: 0 };
fire([]);
var top = pill(0), bot = pill(1);
check("top segments (partially visible w1 excluded)", shown(top), [["open", "js.pill.open", "2"], ["ans", "js.pill.answered", "1"]]);
check("bottom segments", shown(bot), [["open", "js.pill.open", "2"]]);
check("only items in no topic or a topic crossing the view read their own rect", reads.item, ["v1", "w1"]);
check("each topic is read once per update", reads.topic, 4);
check("visibility watches topics once, not their far controls", [observed.length, observed.filter(function (el) { return items.indexOf(el) >= 0; }).map(function (el) { return el.attrs.id; })], [5, ["w1"]]);
fire(items.map(function (el) { return { isIntersecting: true, target: el }; }));
check("stale item visibility cannot remove a far topic's pending items", shown(top), [["open", "js.pill.open", "2"], ["ans", "js.pill.answered", "1"]]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("pill topic cases failed:\n%s", s)
	}
}
