package wsdashboard

import "testing"

// TestBoardKeepsATopicByItsHash pins the board swap's compare (dashboard.js
// morphBoard, topicSeen): a topic section whose incoming data-th equals the
// one it last arrived with keeps its node, whatever its HTML, and a changed
// data-th replaces it; a section without data-th compares its HTML as
// before. The record is the incoming hash, never the live DOM.
func TestBoardKeepsATopicByItsHash(t *testing.T) {
	src := dashboardFuncs(t, "openKey", "restoreOpenOne", "restoreOpen", "forgetOpen", "topicKey", "topicSeen", "morphBoard", "frameChain", "frameGraftable", "frameGrafts", "settleMoves", "frameGraft")
	runNode(t, "board topic hash", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function El(tag, cls, attrs, html) {
  this.nodeType = 1; this.tagName = tag; this.cls = cls || []; this.attrs = attrs || {}; this.outerHTML = html || ""; this.children = [];
  var self = this; this.classList = { contains: function (x) { return self.cls.indexOf(x) >= 0; } };
}
El.prototype.hasAttribute = function (k) { return k in this.attrs; };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.matches = function () { return false; };
El.prototype.querySelectorAll = function () { return []; };
function q(sel, el) { return Array.prototype.slice.call(el.querySelectorAll(sel)); }
function frag(nodes) { return { childNodes: nodes }; }
function isPeer() { return false; }
var window = { WsCards: { weave: function (w) { return { pins: [], board: w }; }, settle: function (el, p) { el.children = p.board; }, pinList: function () { return null; } } };
var openState = {}, boardSeen = {};
`+src+`
function topic(name, th, html) { var a = { "data-topic": name }; if (th) a["data-th"] = th; return new El("SECTION", ["topic"], a, html); }
var board = new El("DIV"), a = topic("A", "h1", "A served"), b = topic("B", null, "B1");
board.children = [a, b];
board.children.forEach(function (n) { boardSeen["t:" + topicKey(n)] = topicSeen(n); });
// The live section's HTML drifts after the swap (times, frame heights).
a.outerHTML = "A as the page changed it";
morphBoard(board, [topic("A", "h1", "A served again, other bytes"), topic("B", null, "B1")]);
check("an unchanged hash keeps the node", [board.children[0] === a, board.children[1] === b], [true, true]);
morphBoard(board, [topic("A", "h2", "A served again, other bytes"), topic("B", null, "B2")]);
check("a changed hash, or changed HTML without one, replaces the node", [board.children[0] === a, board.children[1] === b], [false, false]);
var a2 = board.children[0];
morphBoard(board, [topic("A", "h2", "x"), topic("B", null, "B2")]);
check("the record is the last incoming hash", board.children[0] === a2, true);
process.stdout.write(bad.join("\n"));
`)
}
