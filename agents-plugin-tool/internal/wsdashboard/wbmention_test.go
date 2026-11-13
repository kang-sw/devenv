package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestBoardMentionFromSelection pins how a board selection becomes a mention
// (dashboard.js board mentions): hostMention reads this page's selection and
// makes a mention only when both ends lie in one topic card, outside a form
// field; mentionOf keeps the item ID only when both ends lie in the same item
// card (by the card's element id, which a settled row carries too) and makes
// nothing of a blank selection; validMentions keeps the
// well-formed mentions a cancel or a draft returns. The cases execute under
// node when it is installed.
func TestBoardMentionFromSelection(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	var src strings.Builder
	for _, name := range []string{"mentionOf", "selEl", "entryAt", "itemIdAt", "endRect", "selectionMentions", "hostMention", "frameMention", "validMentions"} {
		// A one-line function first, so the multi-line pattern never runs on
		// into the next function.
		block := regexp.MustCompile(`\n  function ` + name + `\([^)]*\) \{[^\n]*\}\n`).FindString(js)
		if block == "" {
			block = regexp.MustCompile(`(?s)\n  function ` + name + `\([^)]*\) \{\n.*?\n  \}\n`).FindString(js)
		}
		if block == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src.WriteString(block)
	}
	src.WriteString(boardBoxSrc(t, js))
	runNode(t, "board mention", `
function El(tag, cls, attrs) { this.tagName = tag.toUpperCase(); this.nodeType = 1; this.className = cls || ""; this.attrs = attrs || {}; this.parentNode = null; this.children = []; }
El.prototype.add = function (c) { c.parentNode = this; this.children.push(c); return c; };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.contains = function (n) { for (var e = n; e; e = e.parentNode) if (e === this) return true; return false; };
// closest understands the selector lists the script uses: tag.class[attr].
El.prototype.matches1 = function (sel) {
  var m = /^([a-z][a-z0-9]*|)((?:\.[a-z-]+)*)((?:\[[a-z-]+\])*)$/.exec(sel.trim());
  if (!m) throw new Error("selector " + sel);
  var self = this;
  if (m[1] && self.tagName !== m[1].toUpperCase()) return false;
  var ok = m[2].split(".").filter(Boolean).every(function (c) { return (" " + self.className + " ").indexOf(" " + c + " ") >= 0; });
  return ok && (m[3].match(/[a-z-]+/g) || []).every(function (a) { return a in self.attrs; });
};
El.prototype.closest = function (sel) {
  for (var e = this; e && e.nodeType === 1; e = e.parentNode) if (sel.split(",").some(function (s) { return e.matches1(s); })) return e;
  return null;
};
El.prototype.querySelectorAll = function (sel) {
  var out = [];
  function visit(el) { (el.children || []).forEach(function (c) { if (c.nodeType !== 1) return; if (sel.split(",").some(function (s) { return c.matches1(s); })) out.push(c); visit(c); }); }
  visit(this); return out;
};
El.prototype.getClientRects = function () {
  for (var n = this; n; n = n.parentNode) {
    if (n.hidden) return [];
    if (n.parentNode && n.parentNode.tagName === "DETAILS" && n.parentNode.open === false && n.tagName !== "SUMMARY") return [];
  }
  return [{}];
};
function q(sel, el) { return (el || page).querySelectorAll(sel); }
function Text(parent, value) { this.nodeType = 3; this.parentNode = parent; this.nodeValue = value || ""; if (parent) parent.children.push(this); }
var page = new El("main"), board = page.add(new El("div", "region")), other = page.add(new El("div", "composer"));
var t1 = board.add(new El("section", "topic", { "data-topic": "ship-gate" })), t2 = board.add(new El("section", "topic", { "data-topic": "plan" }));
var recap = t1.add(new El("div", "trecap")), c5 = t1.add(new El("div", "card policy", { id: "item-5", "data-id": "05" })), c6 = t1.add(new El("div", "srow", { id: "item-6" }));
var c5title = c5.add(new El("b")), c5opt = c5.add(new El("button", "opt")), note = c5.add(new El("textarea", "note"));
var tRecap = new Text(recap), tTitle = new Text(c5title), tOpt = new Text(c5opt), tRow = new Text(c6), tPlan = new Text(t2), tOther = new Text(other);
var doc = { activeElement: null }, sel = null, bad = [];
var window = { getSelection: function () { return sel; } };
function region(name) { return name === "board" ? board : null; }
function selection(a, f, text) {
  return { anchorNode: a, focusNode: f, isCollapsed: !text, rangeCount: 1, toString: function () { return text; },
    getRangeAt: function () { return { startContainer: a, endContainer: f, getClientRects: function () { return [{ right: 10, bottom: 20 }, { right: 30, bottom: 40 }]; } }; } };
}
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`+src.String()+`
check("blank text makes nothing", [mentionOf("t", "1", "1", "  \n "), mentionOf("", "", "", "x")], [null, null]);
check("one item keeps its id", mentionOf("t", "05", "05", "  a b  "), { topic: "t", text: "a b", id: "05" });
check("two items keep the topic only", mentionOf("t", "05", "06", "x"), { topic: "t", text: "x" });
check("an end outside every item keeps the topic only", mentionOf("t", "", "05", "x"), { topic: "t", text: "x" });

sel = selection(tTitle, tOpt, "Ship it?\nyes");
check("within one item card", hostMention(), { m: { topic: "ship-gate", text: "Ship it?\nyes", id: "5" }, x: 30, y: 40 });
sel = selection(tRow, tRow, "settled");
check("within a settled row", hostMention().m, { topic: "ship-gate", text: "settled", id: "6" });
sel = selection(tOpt, tRow, "yes ... row");
check("across two item cards", hostMention().m, { topic: "ship-gate", text: "yes ... row" });
sel = selection(tRecap, tTitle, "recap Ship");
check("from the recap into an item", hostMention().m, { topic: "ship-gate", text: "recap Ship" });
sel = selection(tRow, tPlan, "row plan");
check("across two topic cards", hostMention(), null);
sel = selection(tOther, tOther, "composer text");
check("outside the board", hostMention(), null);
sel = selection(tTitle, tTitle, "   ");
check("a blank selection", hostMention(), null);
sel = selection(tTitle, tTitle, "");
check("a collapsed selection", hostMention(), null);
sel = selection(tTitle, tTitle, "typed"); doc.activeElement = note;
check("while a board note is focused", hostMention(), null);

check("validMentions", validMentions([null, { topic: "t", text: "x" }, { topic: "", text: "x" }, { topic: "t", text: "" }, { topic: "t", id: 7, text: "y", extra: 1 }, "s"]),
  [{ topic: "t", text: "x" }, { topic: "t", id: "7", text: "y" }]);
check("validMentions of no list", validMentions(undefined), []);
check("entry locators survive draft/cancel restoration", validMentions([
  { topic: "t", entry: "e7", text: "old version" }, { topic: "t", id: 3, entry: "e8", text: "system" },
  { topic: "t", entry: "p7", text: "not a version" }, { topic: "t", entry: "e0", text: "zero" },
  { topic: "t", entry: "e01", text: "noncanonical" }, { topic: "t", id: "oops", entry: "e9", text: "bad provenance" }
]), [{ topic: "t", text: "old version", entry: "e7" }, { topic: "t", id: "3", text: "system", entry: "e8" }]);

// Realistic text-node ranges exercise clipping rather than using a selection's
// unpartitioned string for every entry. Backwards selections keep DOM order.
function texts(root) { var out = []; function visit(n) { if (n.nodeType === 3) out.push(n); else (n.children || []).forEach(visit); } visit(root); return out; }
function Range(a, start, b, end) { this.startContainer = a; this.startOffset = start || 0; this.endContainer = b; this.endOffset = end || 0; this.commonAncestorContainer = page; }
Range.prototype.setStart = function (n, k) { this.startContainer = n; this.startOffset = k; };
Range.prototype.setEnd = function (n, k) { this.endContainer = n; this.endOffset = k; };
Range.prototype.intersectsNode = function (n) {
  var all = texts(page), ns = texts(n), a = all.indexOf(this.startContainer), b = all.indexOf(this.endContainer);
  return ns.some(function (x) { var i = all.indexOf(x); return i >= a && i <= b && (i !== a || this.startOffset < x.nodeValue.length) && (i !== b || this.endOffset > 0); }, this);
};
Range.prototype.toString = function () {
  var all = texts(page), a = all.indexOf(this.startContainer), b = all.indexOf(this.endContainer), self = this;
  return all.slice(a, b + 1).map(function (n) { return n.nodeValue.slice(n === self.startContainer ? self.startOffset : 0, n === self.endContainer ? self.endOffset : n.nodeValue.length); }).join("");
};
Range.prototype.getClientRects = function () { return [{ right: 30, bottom: 40 }]; };
doc.createRange = function () { return new Range(); };
doc.createTreeWalker = function (root) { var all = texts(root), i = 0; return { nextNode: function () { return all[i++] || null; } }; };
function nativeSelection(a, start, b, end, backwards) {
  var r = new Range(a, start, b, end);
  return { anchorNode: backwards ? b : a, focusNode: backwards ? a : b, isCollapsed: false, rangeCount: 1, toString: function () { return r.toString(); }, getRangeAt: function () { return r; } };
}
var stream = t1.add(new El("div", "topic-stream"));
var oldPanel = stream.add(new El("details", "entry", { "data-entry": "e7", "data-panel": "p1" }));
var oldText = new Text(oldPanel.add(new El("summary")), "older panel recap");
var log = stream.add(new El("div", "entry", { "data-entry": "e8", "data-entry-item": "3" }));
var logText = new Text(log, "system mentions item 99");
var latest = stream.add(new El("details", "entry", { "data-entry": "e9", "data-panel": "p1", id: "p1" }));
var latestText = new Text(latest.add(new El("summary")), "latest version");
doc.activeElement = null;
sel = nativeSelection(oldText, 0, oldText, oldText.nodeValue.length);
check("historical panel has its version locator", hostMention().m, { topic: "ship-gate", text: "older panel recap", entry: "e7" });
sel = nativeSelection(logText, 0, logText, logText.nodeValue.length);
check("system entry keeps trusted item provenance, not authored item 99", hostMention().m, { topic: "ship-gate", text: "system mentions item 99", id: "3", entry: "e8" });
sel = nativeSelection(oldText, 6, latestText, 6, true);
check("spanning entries split their own selected text in DOM order", hostMention().ms, [
  { topic: "ship-gate", text: "panel recap", entry: "e7" },
  { topic: "ship-gate", text: "system mentions item 99", id: "3", entry: "e8" },
  { topic: "ship-gate", text: "latest", entry: "e9" }
]);
var foreignLog = t2.add(new El("div", "entry", { "data-entry": "e10" })), foreignText = new Text(foreignLog, "other topic");
sel = nativeSelection(latestText, 7, foreignText, 5);
check("entry spans across topics retain each topic", hostMention().ms, [
  { topic: "ship-gate", text: "version", entry: "e9" }, { topic: "plan", text: "other", entry: "e10" }
]);

var collapsed = t1.add(new El("details", "system-run")); collapsed.open = false;
var summaryText = new Text(collapsed.add(new El("summary")), "system events 1");
var invisible = collapsed.add(new El("div", "entry", { "data-entry": "e20" }));
new Text(invisible, "HIDDEN OWNER ANSWER MUST NOT LEAK");
var script = t1.add(new El("script")); new Text(script, "SCRIPT MUST NOT LEAK");
var controlText = new Text(t1.add(new El("button")), "Load older");
sel = nativeSelection(summaryText, 0, controlText, controlText.nodeValue.length);
check("collapsed entries and script text between visible nodes never leak", hostMention().m, { topic: "ship-gate", text: "system events 1\nLoad older" });

var pending = null, mpend = null;
function placeMention(p) { pending = p; mpend = p; }
function hideMention() { pending = null; mpend = null; }
var frame = oldPanel.add(new El("iframe", "doc-frame", { "data-entry": "e7", "data-topic": "ship-gate" }));
frameMention(frame, "selected from old HTML", 4, 5);
check("frame identity comes from host-owned attributes", pending.m, { topic: "ship-gate", text: "selected from old HTML", entry: "e7" });
check("frame coordinates stay local", [pending.x, pending.y], [4, 5]);
var bodyFrame = t1.add(new El("iframe", "doc-frame", { "data-topic": "ship-gate" }));
frameMention(bodyFrame, "current content", 0, 0);
check("current-content frame remains an old-shaped topic mention", pending.m, { topic: "ship-gate", text: "current content" });
process.stdout.write(bad.join("\n"));
`)
}
