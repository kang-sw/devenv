package wsdashboard

import (
	"regexp"
	"testing"
)

// TestBoardClosingTopicFolds pins the board morph's fold on close
// (dashboard.js morphBoard, forgetOpen, restoreOpen): a topic that closes
// while the page shows it arrives folded as the server rendered it, even
// where the user opened its body or settled rows before; what the user
// opens after that stays open across later swaps, changed or unchanged; a
// reopened topic and a topic first seen closed keep the remembered state.
// The cases execute under node when it is installed.
func TestBoardClosingTopicFolds(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	var src string
	for _, name := range []string{"openKey", "restoreOpenOne", "restoreOpen", "forgetOpen", "topicKey", "morphBoard", "frameChain", "frameGraftable", "frameGrafts", "settleMoves", "frameGraft"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	runNode(t, "board closing topic fold", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function El(tag, cls, attrs, kids, html) {
  this.nodeType = 1; this.tagName = tag; this.cls = cls || []; this.attrs = attrs || {}; this.kids = kids || []; this.outerHTML = html || "";
  this.open = "open" in this.attrs; this.children = [];
  var self = this; this.classList = { contains: function (x) { return self.cls.indexOf(x) >= 0; } };
}
El.prototype.hasAttribute = function (k) { return k in this.attrs; };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.matches = function () { return false; };
El.prototype.querySelectorAll = function (sel) {
  var out = [];
  (function walk(n) { n.kids.forEach(function (c) {
    if (c.tagName === "DETAILS" && (sel === "details" || (sel === "details[data-key]" && c.hasAttribute("data-key")))) out.push(c);
    walk(c);
  }); })(this);
  return out;
};
function q(sel, el) { return Array.prototype.slice.call(el.querySelectorAll(sel)); }
function frag(nodes) { return { childNodes: nodes }; }
function isPeer() { return false; }
var window = { WsCards: { weave: function (w) { return { pins: [], board: w }; }, settle: function (el, p) { el.children = p.board; }, pinList: function () { return null; } } };
var openState = {}, boardSeen = {};
`+src+`
// topic builds a topic section whose body and one settled row the server
// renders folded; html stands for the section's markup version.
function topic(name, closed, html) {
  var body = new El("DETAILS", ["tdoc"], { "data-key": "doc:" + name });
  var row = new El("DETAILS", ["sx"], { "data-key": "item:" + name + "-1" });
  var s = new El("SECTION", closed ? ["topic", "closed"] : ["topic"], { "data-topic": name }, [body, row], html);
  s.body = body; s.row = row;
  return s;
}
function swap(board, nodes) { morphBoard(board, nodes).forEach(restoreOpen); }
function toggle(d, open) { d.open = open; openState[openKey(d)] = open; }
function shown(board, name) {
  var s = board.children.filter(function (n) { return n.getAttribute("data-topic") === name; })[0];
  return [s.body.open, s.row.open];
}

var board = new El("DIV"), a = topic("A", false, "A1");
board.children = [a]; boardSeen = { "t:A": "A1" };
toggle(a.body, true); toggle(a.row, true);
swap(board, [topic("A", false, "A2")]);
check("an open topic's update keeps what the user opened", shown(board, "A"), [true, true]);

swap(board, [topic("A", true, "A3")]);
check("a topic that closes on the page folds its body and settled rows", shown(board, "A"), [false, false]);

toggle(board.children[0].body, true);
swap(board, [topic("A", true, "A4")]);
check("a body opened after the close stays open across a changed swap", shown(board, "A"), [true, false]);
var kept = board.children[0];
swap(board, [topic("A", true, "A4")]);
check("an unchanged closed topic keeps its node and its open body", [board.children[0] === kept, shown(board, "A")], [true, [true, false]]);

swap(board, [topic("A", false, "A5")]);
check("a reopened topic keeps the remembered state", shown(board, "A"), [true, false]);

openState["k:doc:B"] = true;
swap(board, [topic("A", false, "A5"), topic("B", true, "B1")]);
check("a topic first seen closed restores the remembered state", shown(board, "B"), [true, false]);
process.stdout.write(bad.join("\n"));
`)
}
