package wsdashboard

import "testing"

// TestBoardKeepsUnchangedEntryFrames pins keyed frame reuse (dashboard.js
// morphBoard, frameKeep, frameKept): inside a changed topic, an incoming
// entry placeholder (entry:eN) with the old frame's data-doc-key and
// data-srcdoc takes that old frame, the same element, moved (moveBefore)
// rather than detached, so it never reloads; one whose data-srcdoc changed
// stays a placeholder for the mount pass; the topic-body frame (topic:)
// still takes frameGraft's double buffer; duplicate keys pair off once
// each; and without moveBefore nothing is kept. The cases execute under node
// when it is installed.
func TestBoardKeepsUnchangedEntryFrames(t *testing.T) {
	src := dashboardFuncs(t, "openKey", "restoreOpenOne", "restoreOpen", "forgetOpen", "topicKey", "topicSeen", "morphBoard", "frameKeep", "frameKept", "frameChain", "frameGraftable", "frameGrafts", "settleMoves", "frameGraft", "frameNext", "mountFrame")
	runNode(t, "keyed frame reuse", frameDOM+`
function isFrame(n, kind) { return n.classList.contains("doc-frame") && (!kind || n.tagName === kind); }
function entry(n) { return /^entry:/.test(n.getAttribute("data-doc-key") || ""); }
SEL[".doc-frame:not(.doc-frame-next)"] = function (n) { return isFrame(n) && !n.classList.contains("doc-frame-next"); };
SEL[".doc-frame"] = function (n) { return isFrame(n); };
SEL['iframe.doc-frame[data-doc-key^="entry:"]:not(.doc-frame-next)'] = function (n) { return isFrame(n, "IFRAME") && entry(n) && !n.classList.contains("doc-frame-next"); };
SEL['div.doc-frame[data-doc-key^="entry:"]'] = function (n) { return isFrame(n, "DIV") && entry(n); };
// Detaching a connected frame drops its document: putting it back reloads
// it. moveBefore between connected nodes keeps it.
function frames(n) { return (n.tagName === "IFRAME" ? [n] : []).concat(walk(n, []).filter(function (c) { return c.tagName === "IFRAME"; })); }
var rawRemove = El.prototype.removeChild, rawInsert = El.prototype.insertBefore;
El.prototype.removeChild = function (c) { if (c.isConnected) frames(c).forEach(function (f) { f.dropped = true; }); return rawRemove.call(this, c); };
El.prototype.insertBefore = function (c, ref) {
  if (c.isConnected) frames(c).forEach(function (f) { f.dropped = true; });
  rawInsert.call(this, c, ref);
  if (c.isConnected) frames(c).forEach(function (f) { if (f.dropped) f.reloads = (f.reloads || 0) + 1; f.dropped = false; });
  return c;
};
El.prototype.moveBefore = function (c, ref) {
  if (!c.isConnected || !this.isConnected) throw new Error("moveBefore needs connected nodes");
  rawRemove.call(c.parentNode, c);
  rawInsert.call(this, c, ref);
};
var body = new El("body");
doc.appendChild(body);
doc.body = body;
var openState = {}, boardSeen = {}, frameRevealMs = 1500;
function frameShow() {}
function frameReveal() {}
var window = { WsCards: {
  weave: function (w) { return { pins: [], board: w }; },
  pinList: function () { return null; },
  settle: function (el, p) {
    el.childNodes.slice().forEach(function (n) { if (p.board.indexOf(n) < 0) el.removeChild(n); });
    p.board.forEach(function (n) { if (n.parentNode === el && el.moveBefore) el.moveBefore(n, null); else el.appendChild(n); });
  }
} };
function frag(nodes) { return { childNodes: nodes }; }
function boardNodes(input) { return frag(input).childNodes; }
`+src+`
function mounted(key, text) { return mountFrame(new El("div", { "class": "doc-frame", "data-doc": key, "data-doc-key": key, "data-sandbox": SANDBOX, "data-srcdoc": text })); }
function holder(key, text) { return new El("div", { "class": "doc-frame", "data-doc": key, "data-doc-key": key, "data-sandbox": SANDBOX, "data-srcdoc": text }); }
function run(f) { return new El("article", {}, [new El("details", { open: "" }, [f])]); }
function topic(th, bodyFrame, runs) {
  return new El("section", { "class": "topic", "data-topic": "A", "data-th": th }, [new El("div", { "class": "doc-body framed" }, [bodyFrame]), new El("details", { "class": "topic-stream", open: "" }, runs.map(run))]);
}
function setup() {
  var board = new El("div", { id: "r-board" });
  body.appendChild(board);
  var o = { body: mounted("topic:A", "B1"), e1: mounted("entry:e1", "S1"), e2: mounted("entry:e2", "S2"), d1: mounted("entry:e9", "D"), d2: mounted("entry:e9", "D") };
  o.sec = topic("h1", o.body, [o.e1, o.e2, o.d1, o.d2]);
  board.appendChild(o.sec);
  boardSeen = { "t:A": "h1" };
  o.board = board;
  return o;
}
function inBoard(o, n) { for (; n; n = n.parentNode) if (n === o.board) return true; return false; }

var o = setup();
var p1 = holder("entry:e1", "S1"), p2 = holder("entry:e2", "S2 changed"), p3 = holder("entry:e3", "S3"), pd1 = holder("entry:e9", "D"), pd2 = holder("entry:e9", "D"), pd3 = holder("entry:e9", "D");
morphBoard(o.board, [topic("h2", holder("topic:A", "B2"), [p1, p2, p3, pd1, pd2, pd3])]);
check("the changed topic keeps its section (frameGraft)", o.board.childNodes[0] === o.sec);
var bf = walk(o.sec, []).filter(function (n) { return n.classList.contains("doc-frame") && /^topic:/.test(n.getAttribute("data-doc-key")); });
check("the topic-body frame takes the double buffer: old kept, new mounted beside it, pending", bf.length === 2 && bf[0] === o.body && bf[1].tagName === "IFRAME" && bf[1].getAttribute("srcdoc") === "B2" && bf[1].wbNext === true);
check("an unchanged entry frame is the same element, on the board, never reloaded", inBoard(o, o.e1) && !o.e1.reloads && !p1.parentNode);
check("it stands where its placeholder stood", o.e1.parentNode && o.e1.parentNode.parentNode && o.e1.parentNode.parentNode.parentNode && o.e1.parentNode.parentNode.parentNode.classList.contains("topic-stream") && o.e1.parentNode.childNodes.length === 1);
check("a changed data-srcdoc is replaced: the placeholder stays for the mount pass", !inBoard(o, o.e2) && inBoard(o, p2) && p2.tagName === "DIV");
check("a new entry stays a placeholder", inBoard(o, p3));
check("duplicate keys pair off once each", inBoard(o, o.d1) && inBoard(o, o.d2) && !pd1.parentNode && !pd2.parentNode && inBoard(o, pd3) && !o.d1.reloads && !o.d2.reloads);
check("the holder is gone", body.childNodes.length === 1);

// Without moveBefore nothing is kept.
var mb = El.prototype.moveBefore;
El.prototype.moveBefore = undefined;
body.childNodes.slice().forEach(function (n) { body.removeChild(n); });
o = setup();
var q1 = holder("entry:e1", "S1");
morphBoard(o.board, [topic("h2", holder("topic:A", "B2"), [q1])]);
check("without moveBefore the placeholder stays", inBoard(o, q1) && !inBoard(o, o.e1));
El.prototype.moveBefore = mb;
if (bad.length) console.log(bad.join("\n"));
`)
}
