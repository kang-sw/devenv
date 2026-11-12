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
	for _, name := range []string{"mentionOf", "selEl", "itemIdAt", "endRect", "hostMention", "validMentions"} {
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
	runNode(t, "board mention", `
function El(tag, cls, attrs) { this.tagName = tag.toUpperCase(); this.nodeType = 1; this.className = cls || ""; this.attrs = attrs || {}; this.parentNode = null; this.children = []; }
El.prototype.add = function (c) { c.parentNode = this; this.children.push(c); return c; };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.contains = function (n) { for (var e = n; e; e = e.parentNode) if (e === this) return true; return false; };
// closest understands the selector lists the script uses: tag.class[attr].
El.prototype.matches1 = function (sel) {
  var m = /^([a-z]*)((?:\.[a-z-]+)*)((?:\[[a-z-]+\])*)$/.exec(sel.trim());
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
function Text(parent) { this.nodeType = 3; this.parentNode = parent; }
var board = new El("div", "region"), other = new El("div", "composer");
var t1 = board.add(new El("section", "topic", { "data-topic": "ship-gate" })), t2 = board.add(new El("section", "topic", { "data-topic": "plan" }));
var recap = t1.add(new El("div", "trecap")), c5 = t1.add(new El("div", "card policy", { id: "item-5", "data-id": "05" })), c6 = t1.add(new El("div", "srow", { id: "item-6" }));
var c5title = c5.add(new El("b")), c5opt = c5.add(new El("button", "opt")), note = c5.add(new El("textarea", "note"));
var tRecap = new Text(recap), tTitle = new Text(c5title), tOpt = new Text(c5opt), tRow = new Text(c6), tPlan = new Text(t2), tOther = new Text(other);
var doc = { activeElement: null }, sel = null, bad = [];
var window = { getSelection: function () { return sel; } };
function region(name) { return name === "board" ? board : null; }
function selection(a, f, text) {
  return { anchorNode: a, focusNode: f, isCollapsed: !text, rangeCount: 1, toString: function () { return text; },
    getRangeAt: function () { return { getClientRects: function () { return [{ right: 10, bottom: 20 }, { right: 30, bottom: 40 }]; } }; } };
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
process.stdout.write(bad.join("\n"));
`)
}
