package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// railJSHarness is a minimal DOM (children, attributes, classes, one-class
// querySelector, insertBefore, removeChild, nextSibling) and an in-memory
// store for running rail.js under node.
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
El.prototype.removeChild = function (c) { this.children.splice(this.children.indexOf(c), 1); c.parentNode = null; return c; };
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
d.save("a", { text: "", mentions: [], answers: {}, chips: [], bmentions: [], refs: [], lost: [], open: {}, tab: "convo" });
check("an empty draft clears", [d.load("a"), st.load("ws.dashboard.drafts")], [null, null]);
d.save("a", { bmentions: [{ topic: "t", text: "x" }] });
check("board mentions alone are a draft", !!d.load("a"), true);
d.clear("a");
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

// The collapsed workspaces persist; the stars and the order are the
// daemon's, and a row's star is the one it rendered.
var ss = memStore(), box = new El("rsessions");
["r1", "r2", "r3"].forEach(function (id) { box.appendChild(row(id)); });
check("collapsed workspaces persist", [R.collapsed(ss).toggle("/w/repo"), R.collapsed(ss).has("/w/repo")], [true, true]);
function star(r, on) { r.querySelector(".star").setAttribute("aria-pressed", on ? "true" : "false"); return r; }
check("a row's star is the rendered one", [R.starred(star(row("a"), true)), R.starred(row("b")), R.starred(new El("rs")), R.starred(null)], [true, false, false, false]);

// drop: a live row moves within its group on its side of the stars, a
// workspace among the workspaces; never into another group, onto an ended
// row, onto itself, or across the stars.
function group(ids, ended, starred) {
  var b = new El("rsessions");
  ids.forEach(function (id) { b.appendChild(star(row(id), (starred || []).indexOf(id) >= 0)); });
  if (ended) {
    var f = new El("rended"), inner = new El("rsessions");
    ended.forEach(function (id) { var r = row(id); r.classList.add("ended"); inner.appendChild(r); });
    f.appendChild(inner); b.appendChild(f);
    b.inner = inner;
  }
  return b;
}
function ids(b) { return b.children.map(function (c) { return c.getAttribute("data-rid") || c.getAttribute("data-ws") || c.className; }); }
var ga = group(["a1", "a2", "a3"], ["e1", "e2"]), gb = group(["b1", "b2"]);
check("drop moves a row within its group", R.drop(ga.children[2], ga.children[0], false), true);
check("the row moved", ids(ga), ["a3", "a1", "a2", "rended"]);
check("a drop into another group is refused", [R.drop(ga.children[0], gb.children[0], false), ids(ga), ids(gb)], [false, ["a3", "a1", "a2", "rended"], ["b1", "b2"]]);
check("an ended row neither drags nor takes a drop", [R.drop(ga.inner.children[0], ga.inner.children[1], true), R.drop(ga.children[0], ga.inner.children[0], false)], [false, false]);
check("a drop on itself is refused", R.drop(ga.children[0], ga.children[0], true), false);
var ga3 = group(["a1", "a2", "a4", "a3"], null, ["a1", "a2"]);
check("a drop across the stars is refused", [R.drop(ga3.children[3], ga3.children[0], false), ids(ga3)], [false, ["a1", "a2", "a4", "a3"]]);
check("a drop inside the starred part", [R.drop(ga3.children[1], ga3.children[0], false), ids(ga3)], [true, ["a2", "a1", "a4", "a3"]]);
function wsList(paths) { var l = new El("rlist"); paths.forEach(function (p) { l.appendChild(new El("rws", { "data-ws": p })); }); return l; }
var wl = wsList(["/a", "/b", "/c"]);
wl.children[0].appendChild(group(["w1"], null, ["w1"]));
check("a workspace drops among the workspaces, stars inside or not", [R.drop(wl.children[2], wl.children[0], false), ids(wl)], [true, ["/c", "/a", "/b"]]);
check("a workspace does not drop on a row", R.drop(wl.children[0], (function () { var r = row("x"); wl.appendChild(r); return r; })(), false), false);

// The retired local layout: imported once into an empty server layout,
// cleared by an answer that applied it or found the layout taken, kept for
// a retry; a non-empty server layout clears it without an import.
var ls = memStore();
check("nothing kept, nothing to import", [R.legacyImport(ls, true), R.legacyImport(ls, false)], [null, null]);
ls.save("ws.dashboard.railorder", JSON.stringify(["a2", "a1", 3]));
ls.save("ws.dashboard.stars", JSON.stringify(["a1"]));
check("an empty layout imports every kept key", R.legacyImport(ls, true), { op: "import", order: ["a2", "a1"], stars: ["a1"], wsorder: [] });
check("the keys stay until the answer", [ls.load("ws.dashboard.railorder") !== null, ls.load("ws.dashboard.stars") !== null], [true, true]);
R.legacyDone(ls, { applied: false, reason: "retry" });
R.legacyDone(ls, {});
check("a retry or no answer keeps the keys", ls.load("ws.dashboard.stars"), JSON.stringify(["a1"]));
R.legacyDone(ls, { applied: true, gen: 3 });
check("an applied import clears the keys", [ls.load("ws.dashboard.railorder"), ls.load("ws.dashboard.stars")], [null, null]);
ls.save("ws.dashboard.wsorder", "{junk");
check("a junk key imports as nothing", R.legacyImport(ls, true), { op: "import", order: [], stars: [], wsorder: [] });
R.legacyDone(ls, { applied: false, reason: "not_applied" });
check("an import not applied clears the keys", ls.load("ws.dashboard.wsorder"), null);
ls.save("ws.dashboard.wsorder", JSON.stringify(["/a"]));
check("a non-empty layout clears the keys without an import", [R.legacyImport(ls, false), ls.load("ws.dashboard.wsorder")], [null, null]);

// layoutRefusal: every layout reason has its string; anything else the
// generic one.
check("each reason's string", ["unknown", "ended", "self", "depth", "workspace", "group", "level", "stars", "child", "toplevel", "invalid", "retry", "store"].map(R.layoutRefusal),
  ["unknown", "ended", "self", "depth", "workspace", "group", "level", "stars", "child", "toplevel", "invalid", "retry", "store"].map(function (c) { return "js.rail.layout." + c; }));
check("an unknown reason's string", [R.layoutRefusal("shuffle"), R.layoutRefusal(undefined), R.layoutRefusal("toString")], ["js.rail.layout.invalid", "js.rail.layout.invalid", "js.rail.layout.invalid"]);
// staleRail: a render below the generation a change answered is stale.
var rhtml = '<div class="rlist" id="rlist" data-max="64" data-lgen="1731500000000005">';
check("stale renders", [R.staleRail(rhtml, 1731500000000006), R.staleRail(rhtml, 1731500000000005), R.staleRail(rhtml, 0), R.staleRail("<div></div>", 9)], [true, false, false, false]);
// The press and its swaps: a rail swap offered during a press is held (the
// latest replaces an earlier one) and handed back only once the press ends;
// one whose hash turned current is forgotten; with no press none is held.
var pressing = false, gate = R.swapGate(function () { return pressing; });
check("no press: a swap lands at once", [gate.offer({ hash: "h0" }), gate.release()], [false, null]);
pressing = true;
check("a swap during the press is held", [gate.offer({ hash: "h1" }), gate.offer({ hash: "h2" })], [true, true]);
check("nothing is released while the press lasts", gate.release(), null);
pressing = false;
check("the press's end releases the latest held swap, once", [gate.release(), gate.release()], [{ hash: "h2" }, null]);
pressing = true;
gate.offer({ hash: "h3" });
gate.forget();
pressing = false;
check("a held swap whose hash turned current is forgotten", gate.release(), null);
// A node a swap replaced anyway is found again in the new render: a row by
// its reply-id, a workspace section by its path; one the render lacks is
// gone.
var oldL = wsList(["/a", "/b"]), oldG = group(["r1", "r2"]);
oldL.children[1].appendChild(oldG);
var rkey = R.itemKey(oldG.children[1]), wkey = R.itemKey(oldL.children[0]);
check("a row's key is its reply-id, a section's its path", [rkey, wkey], [{ cls: "rs", attr: "data-rid", val: "r2" }, { cls: "rws", attr: "data-ws", val: "/a" }]);
var newL = wsList(["/a", "/b"]), newG = group(["r2", "r1"]);
newL.children[1].appendChild(newG);
check("the replaced row is found again", R.refind(newL, rkey) === newG.children[0], true);
check("the replaced section is found again", R.refind(newL, wkey) === newL.children[0], true);
check("a row the new render lacks is gone", R.refind(newL, R.itemKey(row("r9"))), null);
// A refused drop says why: over another workspace, another worktree of its
// own, or its own list's other side of the stars; over its own group, a
// header, its ended fold, itself, or outside the list the drop takes the
// nearest place instead. A workspace section is never refused.
function worktree(rids, ended) {
  var w = new El("rwt"), hd = new El("wthd"), g = group(rids, ended);
  w.appendChild(hd); w.appendChild(g);
  w.g = g; w.hd = hd;
  return w;
}
function inside(el) { return el.appendChild(new El("s1")); }
var RL = new El("rlist"), W1 = new El("rws", { "data-ws": "/r" }), W2 = new El("rws", { "data-ws": "/f" });
var bar1 = W1.appendChild(new El("wsbar")), body1 = W1.appendChild(new El("wsbody"));
var T1 = body1.appendChild(worktree(["p1", "p2", "p3"], ["px"])), T2 = body1.appendChild(worktree(["q1"]));
W2.appendChild(new El("wsbar"));
var F = W2.appendChild(new El("wsbody")).appendChild(group(["f1"]));
RL.appendChild(W1); RL.appendChild(W2);
star(T1.g.children[0], true);
var p2 = T1.g.children[1];
check("over its own group", [R.dropRefusal(p2, T1.g.children[2]), R.dropRefusal(p2, inside(T1.g.children[2])), R.dropRefusal(p2, T1.g)], ["", "", ""]);
check("over its own header, bar, ended fold, or itself", [R.dropRefusal(p2, T1.hd), R.dropRefusal(p2, bar1), R.dropRefusal(p2, T1.g.inner.children[0]), R.dropRefusal(p2, inside(p2))], ["", "", "", ""]);
check("over another worktree", [R.dropRefusal(p2, T2.g.children[0]), R.dropRefusal(p2, T2.hd)], ["group", "group"]);
check("over another workspace", [R.dropRefusal(p2, F.children[0]), R.dropRefusal(p2, W2), R.dropRefusal(F.children[0], p2)], ["workspace", "workspace", "workspace"]);
check("over the other side of the stars", [R.dropRefusal(p2, T1.g.children[0]), R.dropRefusal(T1.g.children[0], inside(T1.g.children[2]))], ["stars", "stars"]);
check("outside the list", [R.dropRefusal(p2, RL), R.dropRefusal(p2, null)], ["", ""]);
check("a workspace section is never refused", R.dropRefusal(W1, F.children[0]), "");

// The tree. Workspace /r: worktree /r lists s (starred), m (a parent:
// k1 of /r and k2 of /r/wt under it), o (an orphan: it shows top-level),
// a, and the ended e; worktree /r/wt lists w1. Folder workspace /f lists
// f1.
function kid(rid, g) { var r = new El("rs", { "data-rid": rid, "data-g": g }); r.appendChild(new El("rl")); return r; }
function kidsBox(parent) { return new El("rkids", { "data-parent": parent }); }
var TL = new El("rlist"), TW = TL.appendChild(new El("rws", { "data-ws": "/r" })), TB = TW.appendChild(new El("wsbody"));
var TT = TB.appendChild(new El("rwt", { "data-g": "/r" })), TG = TT.appendChild(new El("rsessions"));
var TU = TB.appendChild(new El("rwt", { "data-g": "/r/wt" })), TH = TU.appendChild(new El("rsessions"));
var ts = TG.appendChild(star(row("s"), true)), tm = TG.appendChild(row("m")), tkids = TG.appendChild(kidsBox("m"));
var tk1 = tkids.appendChild(kid("k1", "/r")), tk2 = tkids.appendChild(kid("k2", "/r/wt"));
var to = TG.appendChild(row("o")), ta = TG.appendChild(row("a")), tfold = TG.appendChild(new El("rended")), te = tfold.appendChild(new El("rsessions")).appendChild(row("e"));
te.classList.add("ended"); tm.setAttribute("data-kids", "1");
var tw1 = TH.appendChild(row("w1"));
var TF = TL.appendChild(new El("rws", { "data-ws": "/f" })), tf1 = TF.appendChild(new El("wsbody")).appendChild(new El("rsessions")).appendChild(row("f1"));
var F = R.rowFacts;
check("a parent's facts", F(tm), { rid: "m", ws: "/r", group: "/r", live: true, parent: "", kids: true, star: false, first: "k1" });
check("a child's facts: its own group", F(tk2), { rid: "k2", ws: "/r", group: "/r/wt", live: true, parent: "m", kids: false, star: false, first: "" });
check("a starred row, an ended row, a folder workspace's row", [F(ts).star, F(te).live, F(tf1).group], [true, false, "/f"]);
tm.classList.add("kcol");
check("a collapsed parent shows no first child", F(tm).first, "");
check("the line after a collapsed parent is after its unit", R.dropZone(F(ta), F(tm), 0.9), { zone: "after", ref: "m", op: { op: "move", rid: "a", after: "m" } });
tm.classList.remove("kcol");
// a valid parent: its middle half attaches, its quarters reorder; the line
// after a parent whose children show is before its first child
check("a valid parent's middle attaches", R.dropZone(F(ta), F(tm), 0.5), { zone: "child", ref: "m", op: { op: "attach", rid: "a", parent: "m" } });
check("a valid parent's top quarter is before it", R.dropZone(F(ta), F(tm), 0.2), { zone: "before", ref: "m", op: { op: "move", rid: "a", before: "m" } });
check("a valid parent's bottom quarter is before its first child", R.dropZone(F(ta), F(tm), 0.8), { zone: "before", ref: "k1", op: { op: "attach", rid: "a", parent: "m", before: "k1" } });
check("an orphan is a valid parent", R.dropZone(F(ta), F(to), 0.5), { zone: "child", ref: "o", op: { op: "attach", rid: "a", parent: "o" } });
check("an attach crosses the stars, a reorder does not", [R.dropZone(F(ta), F(ts), 0.5).zone, R.dropZone(F(ta), F(ts), 0.1)], ["child", { refusal: "stars" }]);
check("an attach crosses the worktrees, a reorder does not", [R.dropZone(F(ta), F(tw1), 0.5).op, R.dropZone(F(ta), F(tw1), 0.9)], [{ op: "attach", rid: "a", parent: "w1" }, { refusal: "group" }]);
// a parent with children dragged: no middle zone, never onto its own children
check("a dragged parent finds no child zone", [R.dropZone(F(tm), F(ta), 0.4), R.dropZone(F(tm), F(ta), 0.6).zone], [{ zone: "before", ref: "a", op: { op: "move", rid: "m", before: "a" } }, "after"]);
check("a parent moves past an expanded parent as a unit", R.dropZone(F(tm), { rid: "other", ws: "/r", group: "/r", live: true, parent: "", kids: true, star: false, first: "otherKid" }, 0.6), { zone: "after", ref: "other", op: { op: "move", rid: "m", after: "other" } });
check("a dragged parent's children are no target, nor itself", [R.dropZone(F(tm), F(tk1), 0.5), R.dropZone(F(tm), F(tm), 0.5)], [null, null]);
check("a dragged parent among another's children is too deep", R.dropZone(F(tm), { rid: "x", ws: "/r", group: "/r", live: true, parent: "o", kids: false, star: false, first: "" }, 0.5), { refusal: "depth" });
// cross-workspace, ended targets
check("another workspace's row refuses", [R.dropZone(F(ta), F(tf1), 0.5), R.dropZone(F(tf1), F(ta), 0.1)], [{ refusal: "workspace" }, { refusal: "workspace" }]);
check("an ended row refuses", [R.dropZone(F(ta), F(te), 0.5), R.dropZone(F(tk1), F(te), 0.1)], [{ refusal: "ended" }, { refusal: "ended" }]);
// a child row has no middle zone: its halves are lines among its siblings
check("a child row's halves attach next to it", [R.dropZone(F(ta), F(tk1), 0.5), R.dropZone(F(ta), F(tk2), 0.3)],
  [{ zone: "after", ref: "k1", op: { op: "attach", rid: "a", parent: "m", after: "k1" } }, { zone: "before", ref: "k2", op: { op: "attach", rid: "a", parent: "m", before: "k2" } }]);
check("a child among its siblings moves", R.dropZone(F(tk2), F(tk1), 0.2), { zone: "before", ref: "k1", op: { op: "move", rid: "k2", before: "k1" } });
check("a dragged parent over another group's row refuses", R.dropZone(F(tm), F(tw1), 0.5), { refusal: "group" });
// the top-level line detaches a child only in its own group
check("a detach line in another group refuses", [R.dropZone(F(tk2), F(ta), 0.1), R.dropZone(F(tk2), F(ts), 0.9)], [{ refusal: "group" }, { refusal: "group" }]);
check("a detach line in its own group", [R.dropZone(F(tk2), F(tw1), 0.9), R.dropZone(F(tk1), F(ts), 0.1)],
  [{ zone: "after", ref: "w1", op: { op: "detach", rid: "k2", after: "w1" } }, { zone: "before", ref: "s", op: { op: "detach", rid: "k1", before: "s" } }]);
// dropNoop: a line beside the row, or an attach to its own parent while last
check("noop drops", [R.dropNoop(to, R.dropZone(F(to), F(ta), 0.1)), R.dropNoop(ta, R.dropZone(F(ta), F(to), 0.9)), R.dropNoop(tk2, R.dropZone(F(tk2), F(tm), 0.5)), R.dropNoop(tk2, R.dropZone(F(tk2), F(tk1), 0.9))], [true, true, true, true]);
check("drops that move", [R.dropNoop(ta, R.dropZone(F(ta), F(ts), 0.5)), R.dropNoop(tk1, R.dropZone(F(tk1), F(tm), 0.5)), R.dropNoop(tk1, R.dropZone(F(tk1), F(tk2), 0.9)), R.dropNoop(ta, null)], [false, false, false, true]);
// homeRows: where a drop over no row puts a row's top-level peers;
// dropRefusal reads a child's own group, not its parent's
check("home lists", [R.homeRows(ta) === TG, R.homeRows(tk1) === TG, R.homeRows(tk2) === TH, R.homeRows(tf1) === tf1.parentNode], [true, true, true, true]);
check("a child's own group", [R.dropRefusal(tk2, TH), R.dropRefusal(tk2, TG), R.dropRefusal(tk1, TG), R.dropRefusal(tk1, TH)], ["", "group", "", "group"]);
// Worktree groups move as complete sibling subtrees. Keys are qualified
// by workspace even if two workspaces happen to expose the same path.
var wtkey = R.itemKey(TU);
check("a group key is workspace-qualified", wtkey, { cls: "rwt", attr: "data-g", val: "/r/wt", ws: "/r" });
check("a group is found after a swap", R.refind(TL, wtkey) === TU, true);
check("a group never refinds in a different workspace", R.refind(TL, { cls: "rwt", attr: "data-g", val: "/r/wt", ws: "/f" }), null);
check("groups move as whole sibling units", [R.drop(TU, TT, false), TB.children[0] === TU, TU.children[0] === TH], [true, true, true]);
check("group drop refusals", [R.dropRefusal(TU, tf1), R.dropRefusal(TU, TT), R.drop(TU, TF, false)], ["workspace", "", false]);
R.drop(TU, TT, true);
// moveRow: the drop shown at once, and its undo
var make = function () { return new El("rkids"); };
var u1 = R.moveRow(ta, to, "child", make);
check("an attach makes the children's box", [ids(TG), ids(TG.children[4]), TG.children[4].getAttribute("data-parent"), ta.getAttribute("data-g")], [["s", "m", "rkids", "o", "rkids", "rended"], ["a"], "o", "/r"]);
u1();
check("its undo", [ids(TG), ta.getAttribute("data-g")], [["s", "m", "rkids", "o", "a", "rended"], null]);
var u2 = R.moveRow(tk2, tw1, "after", make);
check("a detach to its own group", [ids(TH), ids(tkids), tk2.getAttribute("data-g")], [["w1", "k2"], ["k1"], "/r/wt"]);
var u3 = R.moveRow(tk1, ta, "before", make);
check("the last child's leave drops the box", [ids(TG), tkids.parentNode], [["s", "m", "o", "k1", "a", "rended"], null]);
u3(); u2();
check("undone in turn", [ids(TG), ids(tkids), ids(TH)], [["s", "m", "rkids", "o", "a", "rended"], ["k1", "k2"], ["w1"]]);
var u4 = R.moveRow(tm, ta, "after", make);
check("a parent moves with its children", ids(TG), ["s", "o", "a", "m", "rkids", "rended"]);
u4();
R.moveRow(ta, tm, "after", make);
R.moveRow(tk2, tk1, "before", make);
check("after a parent is after its children; a child among its siblings", [ids(TG), ids(tkids)], [["s", "m", "rkids", "a", "o", "rended"], ["k2", "k1"]]);
check("collapsed parents persist", [R.kidsCollapsed(ss).toggle("m"), R.kidsCollapsed(ss).has("m"), ss.load("ws.dashboard.railkids")], [true, true, '["m"]']);

// The keyboard path (keyMove) and the context menu's "move under"
// (underRows). Workspace /k: worktree /k lists s (starred), m (a parent:
// c1 of /k, c2 of /k/wt, and c3 of /k/v under it), b, and the ended e;
// worktree /k/wt lists x (starred) and y; worktree /k/v lists no row.
// Folder workspace /g lists g1.
var KL = new El("rlist"), KW = KL.appendChild(new El("rws", { "data-ws": "/k" })), KB = KW.appendChild(new El("wsbody"));
var KG = KB.appendChild(new El("rwt", { "data-g": "/k" })).appendChild(new El("rsessions"));
var KH = KB.appendChild(new El("rwt", { "data-g": "/k/wt" })).appendChild(new El("rsessions"));
KB.appendChild(new El("rwt", { "data-g": "/k/v" }));
var ks = KG.appendChild(star(row("s"), true)), km = KG.appendChild(row("m")), kbox = KG.appendChild(kidsBox("m"));
var kc1 = kbox.appendChild(kid("c1", "/k")), kc2 = kbox.appendChild(kid("c2", "/k/wt")), kc3 = kbox.appendChild(kid("c3", "/k/v"));
var kb = KG.appendChild(row("b")), ke = KG.appendChild(new El("rended")).appendChild(new El("rsessions")).appendChild(row("e"));
ke.classList.add("ended"); km.setAttribute("data-kids", "1");
var kx = KH.appendChild(star(row("x"), true)), ky = KH.appendChild(row("y"));
var KF = KL.appendChild(new El("rws", { "data-ws": "/g" })), kg1 = KF.appendChild(new El("wsbody")).appendChild(new El("rsessions")).appendChild(row("g1"));
var K = R.keyMove;
// up and down: past the next live row of its level, by dropZone's rules
check("up before a parent, not into it", K(kb, "up"), { zone: "before", ref: "m", op: { op: "move", rid: "b", before: "m" } });
check("down past a row, after its unit", K(km, "down"), { zone: "after", ref: "b", op: { op: "move", rid: "m", after: "b" } });
check("up across the stars refuses", K(km, "up"), { refusal: "stars" });
check("nothing past the ends (the ended fold is no row to pass)", [K(ks, "up"), K(kb, "down"), K(kg1, "up"), K(kc1, "up"), K(kc3, "down")], [null, null, null, null, null]);
check("among the children", [K(kc2, "up"), K(kc1, "down")],
  [{ zone: "before", ref: "c1", op: { op: "move", rid: "c2", before: "c1" } }, { zone: "after", ref: "c2", op: { op: "move", rid: "c1", after: "c2" } }]);
check("an ended row moves no way", ["up", "down", "in", "out"].map(function (k) { return K(ke, k); }), [{ refusal: "ended" }, { refusal: "ended" }, { refusal: "ended" }, { refusal: "ended" }]);
// in: the previous top-level row takes it as its last child
check("in attaches to the previous row", K(kb, "in"), { zone: "child", ref: "m", op: { op: "attach", rid: "b", parent: "m" } });
check("in crosses the stars", K(ky, "in"), { zone: "child", ref: "x", op: { op: "attach", rid: "y", parent: "x" } });
check("in: a parent or a child goes no deeper", [K(km, "in"), K(kc2, "in")], [{ refusal: "depth" }, { refusal: "depth" }]);
check("in: no previous row", [K(ks, "in"), K(kx, "in")], [null, null]);
// out: next to its old parent in the parent's group, else the head of its
// own group's unstarred rows, else after its last, else unshown
check("out next to the parent", K(kc1, "out"), { zone: "after", ref: "m", op: { op: "detach", rid: "c1" } });
check("out to its own group's head, past the stars", K(kc2, "out"), { zone: "before", ref: "y", op: { op: "detach", rid: "c2" } });
star(ky, true);
check("out after an all-starred group", K(kc2, "out"), { zone: "after", ref: "y", op: { op: "detach", rid: "c2" } });
star(ky, false);
check("out to a group with no row is not shown", K(kc3, "out"), { zone: "", ref: "", op: { op: "detach", rid: "c3" } });
check("out: a top-level row has no parent", [K(kb, "out"), K(km, "out")], [{ refusal: "toplevel" }, { refusal: "toplevel" }]);
check("an unknown key", K(kb, "home"), null);
// underRows: every valid parent of the workspace in the rail's order, its
// own parent aside; none for an ended row or a parent
function under(r) { var u = R.underRows(r); return { rows: u.rows.map(function (x) { return x.getAttribute("data-rid"); }), refusal: u.refusal }; }
check("a top-level row's parents", under(kb), { rows: ["s", "m", "x", "y"], refusal: "" });
check("a child's parents, its own aside", under(kc2), { rows: ["s", "b", "x", "y"], refusal: "" });
check("no parent for a parent or an ended row", [under(km), under(ke)], [{ rows: [], refusal: "depth" }, { rows: [], refusal: "ended" }]);
check("another workspace's rows are never parents", under(kg1), { rows: [], refusal: "" });

// markCurrent marks only the viewed row.
var rows = box.children.slice(0, 3);
R.markCurrent(rows, "r2");
check("the viewed row is current", rows.map(function (r) { return [r.classList.contains("cur"), r.querySelector(".rl").getAttribute("aria-current")]; }), [[false, null], [true, "page"], [false, null]]);
R.markCurrent(rows, "r1");
check("another view moves the mark", rows.map(function (r) { return r.classList.contains("cur"); }), [true, false, false]);

// popPlace: below the anchor, right edges aligned, kept inside the
// viewport; above it when only that fits; pinned to the bottom margin when
// neither does.
var a = { top: 100, bottom: 118, left: 300, right: 318 };
check("below, right-aligned", R.popPlace(a, 200, 150, 1000, 800), { top: 122, left: 118 });
check("kept 8px from the left", R.popPlace({ top: 10, bottom: 28, left: 40, right: 58 }, 330, 150, 1000, 800), { top: 32, left: 8 });
check("kept 8px from the right", R.popPlace({ top: 10, bottom: 28, left: 990, right: 1008 }, 330, 150, 1000, 800), { top: 32, left: 662 });
check("above when only that fits", R.popPlace({ top: 700, bottom: 718, left: 300, right: 318 }, 200, 300, 1000, 800), { top: 396, left: 118 });
check("bottom margin when neither fits", R.popPlace({ top: 200, bottom: 218, left: 300, right: 318 }, 200, 700, 1000, 800), { top: 92, left: 118 });

// matchBranches filters the free branches by a case-insensitive substring.
check("an empty query matches all", R.matchBranches(["main", "feat/A", "fix/b"], " "), ["main", "feat/A", "fix/b"]);
check("a query filters ignoring case", R.matchBranches(["main", "feat/A", "fix/b", 3], "F"), ["feat/A", "fix/b"]);
check("no list matches nothing", R.matchBranches(null, "x"), []);

// worktreeBody: a new branch carries its base; an existing one none; the
// folder is the custom one when custom, else the default the form shows.
check("new branch, default folder", R.worktreeBody("/w/r", { mode: "new", name: " feat/x ", base: "main", pick: "free", custom: false, folder: "/x", shown: "/w/.wt/r/261112-1333" }),
  { workspace: "/w/r", branch: "feat/x", "new": true, base: "main", folder: "/w/.wt/r/261112-1333" });
check("new branch from the detached HEAD", R.worktreeBody("/w/r", { mode: "new", name: "feat/y", base: "" }).base, "");
check("existing branch, custom folder", R.worktreeBody("/w/r", { mode: "old", name: "ignored", base: "main", pick: "free", custom: true, folder: " /w/wt ", shown: "/w/.wt/r/261112-1333" }),
  { workspace: "/w/r", branch: "free", "new": false, base: "", folder: "/w/wt" });

// deleteReady: Delete stays off until the typed name is the folder exactly,
// for a refused check, and for a check that answered an earlier open; the
// submit's own refusals (force, changed) only ask for the name again.
var clean = { path: "/w/feat-x", folder: "feat-x", branch: "feat", dirty: 0, unreached: 0, force: false, token: "t1" };
var dirty = { path: "/w/feat-x", folder: "feat-x", dirty: 2, force: true, token: "t2" };
check("off until the name matches", ["", "f", "feat-", "feat-x"].map(function (s) { return R.deleteReady({ key: 3, check: clean }, s, 3); }), [false, false, false, true]);
check("the name must match exactly", ["Feat-x", " feat-x", "feat-x ", "feat-xx", "/w/feat-x"].map(function (s) { return R.deleteReady({ key: 3, check: clean }, s, 3); }), [false, false, false, false, false]);
check("a refused check stays off", ["unknown", "primary", "pooled", "missing", "locked", "busy"].map(function (r) {
  return R.deleteReady({ key: 3, check: { folder: "feat-x", refusal: r, note: "n" } }, "feat-x", 3);
}), [false, false, false, false, false, false]);
check("a stale check stays off", [R.deleteReady({ key: 2, check: clean }, "feat-x", 3), R.deleteReady(null, "feat-x", 3), R.deleteReady({ key: 3, check: null }, "feat-x", 3)], [false, false, false]);
check("a forced check needs its token", [R.deleteReady({ key: 1, check: dirty }, "feat-x", 1), R.deleteReady({ key: 1, check: { folder: "feat-x", force: true } }, "feat-x", 1)], [true, false]);
check("the submit's own refusals ask for the name again", ["force", "changed"].map(function (r) {
  var c = { folder: "feat-x", force: true, token: "t3", refusal: r, note: "n" };
  return [R.deleteReady({ key: 4, check: c }, "", 4), R.deleteReady({ key: 4, check: c }, "feat-x", 4)];
}), [[false, true], [false, true]]);
check("no folder never matches", R.deleteReady({ key: 1, check: { folder: "" } }, "", 1), false);
// deleteBody: force and the token only when the check needs force.
check("a clean delete sends no force", R.deleteBody("/w/r", "/w/feat-x", clean), { workspace: "/w/r", path: "/w/feat-x", force: false, token: "" });
check("a forced delete sends the token", R.deleteBody("/w/r", "/w/feat-x", dirty), { workspace: "/w/r", path: "/w/feat-x", force: true, token: "t2" });

// leadBody: only the picked options go out, and a permission mode only to
// Claude.
check("defaults send the harness alone", R.leadBody("/w/r", { harness: "claude", model: "", effort: "", mode: "" }), { root: "/w/r", harness: "claude", plugin_isolation: "default" });
check("picked options", R.leadBody("/w/r", { harness: "claude", model: "opus", effort: "high", mode: "plan" }),
  { root: "/w/r", harness: "claude", plugin_isolation: "default", model: "opus", effort: "high", permission_mode: "plan" });
check("pi takes no permission mode", R.leadBody("/w/r", { harness: "pi", model: "m", effort: "off", mode: "auto" }), { root: "/w/r", harness: "pi", plugin_isolation: "default", model: "m", effort: "off" });

check("isolation is always sent", [R.leadBody("/w/r", { harness: "pi", isolation: "isolated" }).plugin_isolation, R.leadBody("/w/r", { harness: "pi" }).plugin_isolation], ["isolated", "default"]);

// reviveRoute: a known move of a named session, its id escaped.
check("takeover route", R.reviveRoute("takeover", "a b"), "/agents/a%20b/takeover");
check("reopen route", R.reviveRoute("reopen", "s1"), "/agents/s1/reopen");
check("unknown move or no session", [R.reviveRoute("close", "s1"), R.reviveRoute("toString", "s1"), R.reviveRoute("takeover", "")], ["", "", ""]);

// markRevive: only the posting session's entry is busy; an empty id clears.
var rv1 = row("r1"), rv2 = row("r2"), rv3 = row("r3");
rv1.appendChild(new El("rrev")); rv2.appendChild(new El("rrev"));
R.markRevive([rv1, rv2, rv3], "r2");
check("posting entry busy", [rv1.querySelector(".rrev").className, rv2.querySelector(".rrev").className, rv2.querySelector(".rrev").getAttribute("aria-busy")], ["rrev", "rrev busy", "true"]);
R.markRevive([rv1, rv2, rv3], "");
check("cleared", [rv2.querySelector(".rrev").className, rv2.querySelector(".rrev").getAttribute("aria-busy")], ["rrev", null]);

// handedNotice: only a handed-off state, with its resume command when any.
check("handed off with a command", R.handedNotice({ status: "handed_off", resume_command: " cd /w && claude --resume x " }), { command: "cd /w && claude --resume x" });
check("handed off without one", R.handedNotice({ status: "handed_off" }), { command: "" });
check("other states", [R.handedNotice({ status: "closed", resume_command: "x" }), R.handedNotice({ status: "running" }), R.handedNotice(null)], [null, null, null]);

// approvalText: the readable part of a tool input.
check("a shell command", R.approvalText({ command: "git push origin develop", description: "push" }), "git push origin develop");
check("an edit", R.approvalText({ file_path: "/w/a.go", old_string: "a\nb", new_string: "c" }), "/w/a.go\n- a\n- b\n+ c");
check("a write", R.approvalText({ file_path: "/w/a.txt", content: "hello" }), "/w/a.txt\nhello");
check("a read names the file", R.approvalText({ file_path: "/w/a.txt" }), "/w/a.txt");
check("a fetch", R.approvalText({ url: "https://example.com", prompt: "x" }), "https://example.com");
check("another input is indented JSON", R.approvalText({ pattern: "x", n: 2 }), '{\n  "pattern": "x",\n  "n": 2\n}');
check("no input", [R.approvalText(null), R.approvalText({}), R.approvalText(undefined)], ["", "", ""]);
check("a long input is capped", R.approvalText({ command: new Array(5000).join("x") }).length, 4001);

if (bad.length) console.log(bad.join("\n"));
`

// TestRailScript runs rail.js under node: per-session drafts with their
// cap, the stripping of lost image markers, the leave guard, the persisted
// collapsed workspaces, a row's rendered star, the drag's drops, the
// retired local layout's import and clearing, the layout refusals' strings
// and the stale-render check, the press's swap hold, re-find, and drop
// refusals, the tree's drop zones, no-op drops, shown drops and their undo,
// and collapsed parents, the keyboard path's moves and the context menu's
// "move under" parents, the viewed row's mark, the add popovers' placement, branch
// filter, and add-worktree request, the worktree delete modal's Delete gate
// and submit, the spawn request, the revive route
// and entry mark, the hand-off notice, and an approval card's input
// preview.
func TestRailScript(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "rail.js", railJSHarness+string(data)+railJSCases)
}

// railDraftPrelude stubs what dashboard.js's per-session drafts block reads
// from the rest of the page script: the composer (text, mentions, images,
// slash command selection), the board answers, the chips, the board mentions, the open details, the
// tab, the notice, and a virtual clock.
const railDraftPrelude = `
var now = 0, timers = [], tid = 0;
function setTimeout(fn, ms) { var id = ++tid; timers.push({ id: id, at: now + (ms || 0), fn: fn }); return id; }
function clearTimeout(id) { timers = timers.filter(function (t) { return t.id !== id; }); }
function advance(ms) { now += ms; var due = timers.filter(function (t) { return t.at <= now; }); timers = timers.filter(function (t) { return t.at > now; }); due.forEach(function (t) { t.fn(); }); }
var listeners = {}, winListeners = {};
var doc = { hidden: false, addEventListener: function (t, fn) { listeners[t] = fn; } };
window.addEventListener = function (t, fn) { winListeners[t] = fn; };
var WsRail = window.WsRail, rid = "sess1", live = true, railStore = memStore();
var shell = false, framed = false;
var grid = new El("grid", { "data-tab": "convo" });
var ctext = { value: "", selectionStart: 0, selectionEnd: 0, setSelectionRange: function (a, b) { this.selectionStart = a; this.selectionEnd = b; } };
var mentions = [], mentionWas = "", imgs = [], imgSeq = 0, chips = {}, cchips = {}, items = {}, openState = {}, boardAnswers = {}, notes = [], bmentions = [], wpicks = [], wconsoles = [];
var ccmd = {}, cmdSel = null;
function setCommand(name, args) { cmdSel = { name: name, args: typeof args === "string" ? args : null }; }
function validMentions(l) { return Array.isArray(l) ? l.filter(function (m) { return m && m.topic && m.text; }) : []; }
function validPicks(l) { return Array.isArray(l) ? l.filter(function (p) { return p && p.url && p.selector; }) : []; }
function validConsoles(l) { return Array.isArray(l) ? l.filter(function (c) { return c && c.url && c.lines && c.lines.length; }) : []; }
function renderChips() {}
var MARKER = "\\[Image #([1-9][0-9]{0,5})\\]";
function markerNums(text) { var out = {}, re = new RegExp(MARKER, "g"), m; while ((m = re.exec(text))) out[+m[1]] = true; return out; }
function liveMention(m, v) { return v.slice(m.start, m.end) === m.text; }
function region(name) { return name === "board" ? {} : null; }
function readAnswers() { return JSON.parse(JSON.stringify(boardAnswers)); }
function writeAnswers(b, a) { boardAnswers = a; }
function chipList() { return Object.keys(chips).map(function (k) { return chips[k]; }); }
function putChip(c) { chips[c.id] = c; }
function setItem(id, c) { items[id] = c; }
function syncChips() {}
function restoreOpen() {}
function attachSrc(ref) { return "/s/sess1/attachment/" + ref; }
function grow() {} function paintMentions() {} function renderImgs() {}
function setTab(t) { grid.setAttribute("data-tab", t); }
function noteWarn(s) { notes.push(s); }
function wsT(k, n) { return k + ":" + n; }
`

// TestRailDraftRestore runs dashboard.js's per-session drafts block under
// node: an edit saves the draft after the debounce; a reload restores the
// composer text, mentions, selection, the reference images, the board
// answers, the chips, the board mentions, the open details, and the board tab; a pasted image is
// not kept, its marker is stripped, and the restore says so; the leave
// guard asks only while a pasted image is held; a send clears the draft.
func TestRailDraftRestore(t *testing.T) {
	rail, err := dashboardFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	block := regexp.MustCompile(`(?s)\n  // --- per-session drafts -.*?\n\n  // --- iframe shell`).FindString(src)
	if block == "" {
		t.Fatal("dashboard.js holds no per-session drafts block ending before the iframe shell block")
	}
	block = strings.TrimSuffix(block, "// --- iframe shell")
	if !regexp.MustCompile(`(?s)\n  function sendMessage\(\) \{.*?post\("message", body, function \(ok, res\) \{\n[^\n]*\n      if \(!ok\) \{[^\n]*return; \}\n.*?\n      clearDraft\(\);\n    \}\);\n  \}\n`).MatchString(src) {
		t.Error("a successful send no longer clears the draft")
	}
	runNode(t, "draft restore", railJSHarness+string(rail)+railDraftPrelude+boardBoxSrc(t, src)+block+`
// type, pick, open, and switch tab; the debounce saves it
ctext.value = "hi @bob see [Image #1] and [Image #2] end";
ctext.selectionStart = ctext.selectionEnd = 6;
mentions = [{ start: 3, end: 7, text: "@bob", extra: 1 }];
imgs = [{ n: 1, name: "a.png", ref: "00aa-1.png" }, { n: 2, name: "pasted.png", data: "xx" }];
boardAnswers = { "3": { answer: "yes" }, "topic:2": { note: "keep it" } };
chips = { "3": { id: "3", answer: "yes" } };
bmentions = [{ topic: "plan", id: "3", entry: "e7", text: "picked text" }];
wpicks = [{ url: "http://localhost:5173/", selector: "#a", text: "A" }]; wconsoles = [{ url: "http://localhost:5173/", lines: ["e1"] }];
openState = { "rended:/w": true, "gone": false };
grid.setAttribute("data-tab", "board");
listeners.input();
check("nothing saved before the debounce", railStore.load("ws.dashboard.draft.sess1"), null);
advance(400);
var saved = JSON.parse(railStore.load("ws.dashboard.draft.sess1"));
check("the saved draft", [saved.text, saved.mentions, saved.sel, saved.refs, saved.lost, saved.open, saved.tab],
  [ctext.value, [{ start: 3, end: 7, text: "@bob" }], [6, 6], [{ ref: "00aa-1.png", name: "a.png", n: 1 }], [2], { "rended:/w": true }, "board"]);
check("the saved board mentions", saved.bmentions, [{ topic: "plan", id: "3", entry: "e7", text: "picked text" }]);
check("the saved web picks", [saved.wpicks, saved.wconsoles], [[{ url: "http://localhost:5173/", selector: "#a", text: "A" }], [{ url: "http://localhost:5173/", lines: ["e1"] }]]);
check("the guard asks while a pasted image is held", (function () { var e = { preventDefault: function () {} }; winListeners.beforeunload(e); return e.returnValue; })(), "");

// a reload of the page: empty composer and state, then the restore
ctext.value = ""; mentions = []; imgs = []; chips = {}; items = {}; openState = {}; boardAnswers = {}; bmentions = []; wpicks = []; wconsoles = []; grid.setAttribute("data-tab", "convo");
restoreDraft();
check("the board mentions come back", bmentions, [{ topic: "plan", id: "3", entry: "e7", text: "picked text" }]);
check("the web picks come back", [wpicks, wconsoles], [[{ url: "http://localhost:5173/", selector: "#a", text: "A" }], [{ url: "http://localhost:5173/", lines: ["e1"] }]]);
check("text without the pasted image's marker", ctext.value, "hi @bob see [Image #1] and end");
check("mentions, selection, the reference image", [mentions, [ctext.selectionStart, ctext.selectionEnd], imgs.map(function (p) { return [p.n, p.ref, p.url]; })],
  [[{ start: 3, end: 7, text: "@bob" }], [6, 6], [[1, "00aa-1.png", "/s/sess1/attachment/00aa-1.png"]]]);
check("answers, chips, open details, tab", [boardAnswers, Object.keys(chips), items["3"].answer, openState, grid.getAttribute("data-tab")],
  [{ "3": { answer: "yes" }, "topic:2": { note: "keep it" } }, ["3"], "yes", { "rended:/w": true }, "board"]);
check("the restore says a pasted image was not kept", notes, ["js.draft.imgslost:1"]);
check("no guard once only reference images are held", (function () { var e = { preventDefault: function () {} }; return winListeners.beforeunload(e); })(), undefined);
check("the restore itself saved nothing", timers.length, 0);

// a typed composer is not overwritten by a later restore
ctext.value = "typed"; restoreDraft();
check("a non-empty composer keeps its text", ctext.value, "typed");

// a slash command selection alone is a draft, kept with its arguments
ctext.value = ""; imgs = []; chips = {}; boardAnswers = {}; openState = {}; bmentions = []; wpicks = []; wconsoles = []; grid.setAttribute("data-tab", "convo");
cmdSel = { name: "compact", args: "keep the plan" };
listeners.input(); advance(400);
check("the saved command", JSON.parse(railStore.load("ws.dashboard.draft.sess1")).cmd, { name: "compact", args: "keep the plan" });
cmdSel = null; restoreDraft();
check("the command comes back as the selection", cmdSel, { name: "compact", args: "keep the plan" });
cmdSel = null;

// a board note's images: a pasted one's marker leaves the item and topic
// notes and the chip on reload, and a reference image a note alone names
// comes back
ctext.value = ""; imgs = [{ n: 1, name: "a.png", ref: "00aa-1.png" }, { n: 2, name: "pasted.png", data: "xx" }, { n: 3, name: "p2.png", data: "yy" }];
chips = { "4": { id: "4", answer: "", note: "see [Image #1] and [Image #2]" } };
boardAnswers = { "4": { note: "see [Image #1] and [Image #2]", noteOpen: true }, "t:plan": { note: "[Image #3] look", noteOpen: true } };
notes = []; listeners.input(); advance(400);
ctext.value = ""; imgs = []; chips = {}; items = {}; boardAnswers = {};
restoreDraft();
check("lost markers leave the notes and the chip", [boardAnswers["4"].note, boardAnswers["t:plan"].note, chips["4"].note], ["see [Image #1] and ", "look", "see [Image #1] and "]);
check("a note's reference image comes back", imgs.map(function (p) { return [p.n, p.ref]; }), [[1, "00aa-1.png"]]);
check("the restore counts every lost image", notes, ["js.draft.imgslost:2"]);
ctext.value = ""; imgs = []; chips = {}; items = {}; boardAnswers = {};

// a send clears the draft; what is left is saved again as the next draft
ctext.value = ""; imgs = []; chips = {}; boardAnswers = {}; openState = {}; bmentions = []; wpicks = []; wconsoles = []; grid.setAttribute("data-tab", "convo");
clearDraft();
check("cleared at once", railStore.load("ws.dashboard.draft.sess1"), null);
advance(400);
check("an empty page saves no draft", [railStore.load("ws.dashboard.draft.sess1"), railStore.load("ws.dashboard.drafts")], [null, null]);
if (bad.length) console.log(bad.join("\n"));
`)
}
