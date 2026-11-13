package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

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

// drag order: a group's live rows follow the stored order, a session new
// since the last drag on top, stars still first with the order inside each
// part; the ended fold's rows keep the server's order; stale ids are pruned
// on the next save; a drop never leaves its group or its side of the stars.
function group(ids, ended) {
  var b = new El("rsessions");
  ids.forEach(function (id) { b.appendChild(row(id)); });
  if (ended) {
    var f = new El("rended"), inner = new El("rsessions");
    ended.forEach(function (id) { var r = row(id); r.classList.add("ended"); inner.appendChild(r); });
    f.appendChild(inner); b.appendChild(f);
    b.inner = inner;
  }
  return b;
}
function ids(b) { return b.children.map(function (c) { return c.getAttribute("data-rid") || c.getAttribute("data-ws") || c.className; }); }
var os = memStore(), so = R.sessionOrder(os), none = R.stars(memStore());
var ga = group(["a1", "a2", "a3"], ["e1", "e2"]), gb = group(["b1", "b2"]);
R.pin(ga, none, so.list());
check("no stored order keeps the server's", ids(ga), ["a1", "a2", "a3", "rended"]);
check("drop moves a row within its group", R.drop(ga.children[2], ga.children[0], false), true);
check("the row moved", ids(ga), ["a3", "a1", "a2", "rended"]);
check("a drop into another group is refused", [R.drop(ga.children[0], gb.children[0], false), ids(ga), ids(gb)], [false, ["a3", "a1", "a2", "rended"], ["b1", "b2"]]);
check("an ended row neither drags nor takes a drop", [R.drop(ga.inner.children[0], ga.inner.children[1], true), R.drop(ga.children[0], ga.inner.children[0], false)], [false, false]);
check("a drop on itself is refused", R.drop(ga.children[0], ga.children[0], true), false);
so.place(["a3", "a1", "a2"], ["a1", "a2", "a3", "b1", "b2", "e1", "e2"]);
check("the group's order is stored", JSON.parse(os.load("ws.dashboard.railorder")), ["a3", "a1", "a2"]);
so.place(["b2", "b1"], ["a1", "a2", "a3", "b1", "b2"]);
check("another group's order joins it", JSON.parse(os.load("ws.dashboard.railorder")), ["a3", "a1", "a2", "b2", "b1"]);
// A swap renders the server's order again; the stored order comes back.
var ga2 = group(["a4", "a1", "a2", "a3"], ["e2", "e1"]);
R.pin(ga2, none, so.list());
R.pin(ga2.inner, none, null);
check("after a swap: the new session on top, then the stored order", ids(ga2), ["a4", "a3", "a1", "a2", "rended"]);
check("the ended rows keep the server's order", ids(ga2.inner), ["e2", "e1"]);
var st2 = memStore(), sr = R.stars(st2);
sr.toggle("a2"); sr.toggle("a1");
var ga3 = group(["a4", "a1", "a2", "a3"]);
R.pin(ga3, sr, so.list());
check("stars first, the order inside each part", ids(ga3), ["a1", "a2", "a4", "a3"]);
check("a drop across the stars is refused", [R.drop(ga3.children[3], ga3.children[0], false), ids(ga3)], [false, ["a1", "a2", "a4", "a3"]]);
check("a drop inside the starred part", [R.drop(ga3.children[1], ga3.children[0], false), ids(ga3)], [true, ["a2", "a1", "a4", "a3"]]);
so.place(["a4", "a1"], ["a1", "a4", "b1"]);
check("stale ids are pruned on the next save", JSON.parse(os.load("ws.dashboard.railorder")), ["b1", "a4", "a1"]);
check("a stale id in the stored order is ignored", (function () { var g = group(["a1", "a4"]); R.pin(g, none, ["gone", "a4", "a1"]); return ids(g); })(), ["a4", "a1"]);
// Storage that throws: no order, nothing kept, the server's order.
var boom = { load: function () { throw new Error("denied"); }, save: function () { throw new Error("denied"); } }, bo = R.sessionOrder(boom);
check("a throwing store reads as no order", bo.list(), []);
check("a throwing store keeps nothing and does not throw", bo.place(["a2", "a1"], ["a1", "a2"]), ["a2", "a1"]);
var gt = group(["a1", "a2"]);
R.pin(gt, none, bo.list());
check("a throwing store leaves the server's order", ids(gt), ["a1", "a2"]);
// Workspaces: the stored ones first in their order, the rest after in the
// server's order; a section and a row never trade places.
function wsList(paths) { var l = new El("rlist"); paths.forEach(function (p) { l.appendChild(new El("rws", { "data-ws": p })); }); return l; }
var wo = R.wsOrder(os), wl = wsList(["/a", "/b", "/c"]);
R.arrange(wl, wo.list());
check("no workspace order keeps the server's", ids(wl), ["/a", "/b", "/c"]);
check("a workspace drops among the workspaces", [R.drop(wl.children[2], wl.children[0], false), ids(wl)], [true, ["/c", "/a", "/b"]]);
check("a workspace does not drop on a row", R.drop(wl.children[0], (function () { var r = row("x"); wl.appendChild(r); return r; })(), false), false);
wl.children.pop().parentNode = null;
wo.place(ids(wl), ids(wl));
var wl2 = wsList(["/a", "/b", "/c", "/d", "/0"]);
R.arrange(wl2, wo.list());
check("unknown workspaces go after the ordered ones, in server order", ids(wl2), ["/c", "/a", "/b", "/d", "/0"]);
check("the workspace order is its own", [JSON.parse(os.load("ws.dashboard.wsorder")), JSON.parse(os.load("ws.dashboard.railorder"))[0]], [["/c", "/a", "/b"], "b1"]);
// settle leaves rows already in order where they stand.
var gs = group(["s1", "s2"]), moved = 0, ib = gs.insertBefore;
gs.insertBefore = function (c, r) { moved++; return ib.call(this, c, r); };
R.pin(gs, none, null);
check("rows already in order are not moved", moved, 0);

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
// cap, the stripping of lost image markers, the leave guard, starred-row
// pinning and its undo, the persisted stars and collapsed workspaces, the
// drag order of the sessions and workspaces (its drops, storage, and
// pruning), the viewed row's mark, the add popovers' placement, branch
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
var grid = new El("grid", { "data-tab": "convo" });
var ctext = { value: "", selectionStart: 0, selectionEnd: 0, setSelectionRange: function (a, b) { this.selectionStart = a; this.selectionEnd = b; } };
var mentions = [], mentionWas = "", imgs = [], imgSeq = 0, chips = {}, cchips = {}, items = {}, openState = {}, boardAnswers = {}, notes = [], bmentions = [];
var ccmd = {}, cmdSel = null;
function setCommand(name, args) { cmdSel = { name: name, args: typeof args === "string" ? args : null }; }
function validMentions(l) { return Array.isArray(l) ? l.filter(function (m) { return m && m.topic && m.text; }) : []; }
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
	block := regexp.MustCompile(`(?s)\n  // --- per-session drafts -.*?\n\n  if \(live\) \{`).FindString(src)
	if block == "" {
		t.Fatal("dashboard.js holds no per-session drafts block ending before if (live)")
	}
	block = strings.TrimSuffix(block, "if (live) {")
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
openState = { "rended:/w": true, "gone": false };
grid.setAttribute("data-tab", "board");
listeners.input();
check("nothing saved before the debounce", railStore.load("ws.dashboard.draft.sess1"), null);
advance(400);
var saved = JSON.parse(railStore.load("ws.dashboard.draft.sess1"));
check("the saved draft", [saved.text, saved.mentions, saved.sel, saved.refs, saved.lost, saved.open, saved.tab],
  [ctext.value, [{ start: 3, end: 7, text: "@bob" }], [6, 6], [{ ref: "00aa-1.png", name: "a.png", n: 1 }], [2], { "rended:/w": true }, "board"]);
check("the saved board mentions", saved.bmentions, [{ topic: "plan", id: "3", entry: "e7", text: "picked text" }]);
check("the guard asks while a pasted image is held", (function () { var e = { preventDefault: function () {} }; winListeners.beforeunload(e); return e.returnValue; })(), "");

// a reload of the page: empty composer and state, then the restore
ctext.value = ""; mentions = []; imgs = []; chips = {}; items = {}; openState = {}; boardAnswers = {}; bmentions = []; grid.setAttribute("data-tab", "convo");
restoreDraft();
check("the board mentions come back", bmentions, [{ topic: "plan", id: "3", entry: "e7", text: "picked text" }]);
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
ctext.value = ""; imgs = []; chips = {}; boardAnswers = {}; openState = {}; bmentions = []; grid.setAttribute("data-tab", "convo");
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
ctext.value = ""; imgs = []; chips = {}; boardAnswers = {}; openState = {}; bmentions = []; grid.setAttribute("data-tab", "convo");
clearDraft();
check("cleared at once", railStore.load("ws.dashboard.draft.sess1"), null);
advance(400);
check("an empty page saves no draft", [railStore.load("ws.dashboard.draft.sess1"), railStore.load("ws.dashboard.drafts")], [null, null]);
if (bad.length) console.log(bad.join("\n"));
`)
}
