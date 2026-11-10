package wsdashboard

import "testing"

// railJSHarness is a minimal DOM (children, attributes, classes, one-class
// querySelector, insertBefore, nextSibling) and an in-memory store for
// running rail.js under node.
const railJSHarness = `
var window = {};
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function El(cls, attrs) {
  var self = this;
  this.className = cls || ""; this.children = []; this.parentNode = null; this.attrs = attrs || {};
  this.classList = {
    add: function (c) { if (!self.classList.contains(c)) self.className = (self.className + " " + c).trim(); },
    remove: function (c) { self.className = self.className.split(" ").filter(function (x) { return x && x !== c; }).join(" "); },
    contains: function (c) { return (" " + self.className + " ").indexOf(" " + c + " ") >= 0; }
  };
}
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
El.prototype.appendChild = function (c) { return this.insertBefore(c, null); };
El.prototype.insertBefore = function (c, ref) {
  if (c.parentNode) { var p = c.parentNode.children; p.splice(p.indexOf(c), 1); }
  var i = ref ? this.children.indexOf(ref) : -1;
  if (i < 0) this.children.push(c); else this.children.splice(i, 0, c);
  c.parentNode = this; return c;
};
Object.defineProperty(El.prototype, "nextSibling", { get: function () {
  if (!this.parentNode) return null;
  var s = this.parentNode.children; return s[s.indexOf(this) + 1] || null;
} });
El.prototype.querySelector = function (sel) {
  var c = sel.slice(1);
  for (var i = 0; i < this.children.length; i++) {
    var k = this.children[i];
    if (k.classList.contains(c)) return k;
    var deep = k.querySelector(sel); if (deep) return deep;
  }
  return null;
};
function memStore() {
  var m = {};
  return { m: m, load: function (k) { return k in m ? m[k] : null; }, save: function (k, v) { if (v === null) delete m[k]; else m[k] = v; } };
}
function row(rid) { var r = new El("rs", { "data-rid": rid }); r.appendChild(new El("star", { "aria-pressed": "false" })); r.appendChild(new El("rl")); return r; }
`

const railJSCases = `
var R = window.WsRail;

// drafts: save, load, clear; an empty draft clears; the cap drops the oldest.
var st = memStore(), d = R.drafts(st, 3);
d.save("a", { text: "hello", mentions: [], sel: [5, 5] });
check("a saved draft loads", d.load("a"), { text: "hello", mentions: [], sel: [5, 5] });
d.save("a", { text: "", mentions: [], answers: {}, chips: [], refs: [], lost: [], open: {}, tab: "convo" });
check("an empty draft clears", [d.load("a"), st.load("ws.dashboard.drafts")], [null, null]);
d.save("b", { answers: { "topic:1": { note: "n" } } });
d.save("c", { open: { "rended:/r": true } });
d.save("e", { tab: "board" });
d.save("f", { chips: [{ id: "x" }] });
check("the cap drops the oldest", [d.load("b"), !!d.load("c"), !!d.load("f"), JSON.parse(st.load("ws.dashboard.drafts"))], [null, true, true, ["c", "e", "f"]]);
d.save("c", { text: "again" });
d.save("g", { lost: [2] });
check("a re-saved draft is the newest", JSON.parse(st.load("ws.dashboard.drafts")), ["f", "c", "g"]);
d.clear("c");
check("clear removes the draft and its index entry", [d.load("c"), JSON.parse(st.load("ws.dashboard.drafts"))], [null, ["f", "g"]]);
st.save("ws.dashboard.draft.bad", "{not json");
check("a corrupt draft loads as none", d.load("bad"), null);

// stripMarkers drops the lost images' markers and moves the mentions.
var s = R.stripMarkers("see [Image #1] and [Image #2] @x here", [{ start: 30, end: 32, text: "@x" }], [1]);
check("a lost marker is stripped", s, { text: "see and [Image #2] @x here", mentions: [{ start: 19, end: 21, text: "@x" }] });
check("no lost marker keeps the text", R.stripMarkers("[Image #3] y", [], [1]).text, "[Image #3] y");
check("a mention cut by a removal is dropped", R.stripMarkers("[Image #1] z", [{ start: 2, end: 6, text: "x" }], [1]).mentions, []);

// leaveGuard asks only while pasted images are held.
var held = false, guard = R.leaveGuard(function () { return held; }), ev = { prevented: false, preventDefault: function () { this.prevented = true; } };
guard(ev);
check("no guard without held images", ev.prevented, false);
held = true; guard(ev);
check("the guard asks while images are held", [ev.prevented, ev.returnValue], [true, ""]);

// stars and pin: starred rows first, each part in server order; unstarring
// returns a row to its place; the ended fold stays last.
var ss = memStore(), stars = R.stars(ss), box = new El("rsessions");
["r1", "r2", "r3"].forEach(function (id) { box.appendChild(row(id)); });
var fold = new El("rended"); box.appendChild(fold);
function order() { return box.children.map(function (c) { return c.getAttribute("data-rid") || "fold"; }); }
check("toggle stars", stars.toggle("r3"), true);
R.pin(box, stars);
check("a starred row is pinned first", order(), ["r3", "r1", "r2", "fold"]);
check("its star is pressed", [box.children[0].querySelector(".star").getAttribute("aria-pressed"), box.children[1].querySelector(".star").getAttribute("aria-pressed")], ["true", "false"]);
stars.toggle("r2"); R.pin(box, stars);
check("two starred rows keep server order", order(), ["r2", "r3", "r1", "fold"]);
stars.toggle("r3"); stars.toggle("r2"); R.pin(box, stars);
check("unstarred rows return to their places", order(), ["r1", "r2", "r3", "fold"]);
check("no star left stores nothing", ss.load("ws.dashboard.stars"), null);
stars.toggle("r1");
check("stars persist", R.stars(ss).has("r1"), true);
check("collapsed workspaces persist", [R.collapsed(ss).toggle("/w/repo"), R.collapsed(ss).has("/w/repo")], [true, true]);

// markCurrent marks only the viewed row.
var rows = box.children.slice(0, 3);
R.markCurrent(rows, "r2");
check("the viewed row is current", rows.map(function (r) { return [r.classList.contains("cur"), r.querySelector(".rl").getAttribute("aria-current")]; }), [[false, null], [true, "page"], [false, null]]);
R.markCurrent(rows, "r1");
check("another view moves the mark", rows.map(function (r) { return r.classList.contains("cur"); }), [true, false, false]);

if (bad.length) console.log(bad.join("\n"));
`

// TestRailScript runs rail.js under node: per-session drafts with their
// cap, the stripping of lost image markers, the leave guard, starred-row
// pinning and its undo, the persisted stars and collapsed workspaces, and
// the viewed row's mark.
func TestRailScript(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "rail.js", railJSHarness+string(data)+railJSCases)
}
