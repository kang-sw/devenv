package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestBoardTopicMotion pins the board swap's motion (dashboard.js
// topicMotion, growTopic, slideBoard, and their swap wiring): a topic the
// board did not hold grows from 0, a changed topic grows from its old
// rendered height, topics reconciled through the closed pages and non-topic
// nodes stay still; every topic and card standing in the board slides from
// its old offset to its new place, but a maximized card, a large topic,
// and a topic holding a large panel (a large topic does not grow); a deferred
// (editing) swap and reduced motion read no spots, so nothing animates; the
// animations fill nothing, so no fixed height or transform stays behind;
// growTopic and slideBoard return what they start, and swap starts it all
// at the first frame (startMotion, TestBoardMotionStartsAtFirstFrame).
// The cases execute under node when it is installed.
func TestBoardTopicMotion(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	key := regexp.MustCompile(`(?s)\n  function topicKey\(n\) \{.*?\n  \}\n`).FindString(js)
	motion := regexp.MustCompile(`(?s)\n  function topicMotion\(before, ins\) \{.*?\n  \}\n`).FindString(js)
	grow := regexp.MustCompile(`(?s)\n  function growTopic\(n, from\) \{.*?\n  \}\n`).FindString(js)
	slide := regexp.MustCompile(`(?s)\n  function slideBoard\(el, before\) \{.*?\n  \}\n`).FindString(js)
	before := regexp.MustCompile(`\n    var before = board [^\n]*\n`).FindString(js)
	if key == "" || motion == "" || grow == "" || slide == "" || before == "" {
		t.Fatal("dashboard.js holds no topicKey / topicMotion / growTopic / slideBoard / swap before-spots line")
	}
	if !strings.Contains(js, `swap("board", d.hash, d.html, true);`) {
		t.Error("the deferred board swap does not land still")
	}
	if !strings.Contains(js, "        var moves = topicMotion(before, ins).map(function (m) { return growTopic(m.n, m.from); });\n        startMotion(moves.concat(slideBoard(el, before)));\n") {
		t.Error("swap does not grow the inserted topics after preparing them, then slide the board, and start both at the first frame")
	}
	runNode(t, "board topic motion", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function N(tag, topic, cls) {
  this.nodeType = 1; this.tagName = tag; this.attrs = {}; this.anims = [];
  var c = cls || []; this.classList = { contains: function (x) { return c.indexOf(x) >= 0; } };
  if (topic !== undefined) this.attrs["data-topic"] = topic;
}
N.prototype.hasAttribute = function (k) { return k in this.attrs; };
N.prototype.querySelector = function (sel) { return sel === "details[data-max]" && this.panelMax ? {} : null; };
N.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
N.prototype.getBoundingClientRect = function () { return { height: this.h, top: this.y || 0 }; };
N.prototype.animate = function (k, o) { this.anims.push([k, o]); return "anim:" + this.attrs["data-topic"]; };
function getComputedStyle() { return { marginBottom: "16px" }; }
`+key+motion+grow+slide+`
var fresh = new N("SECTION", "New", ["topic"]), changed = new N("SECTION", "Old", ["topic"]), paged = new N("SECTION", "Paged", ["topic", "closed"]), bar = new N("DIV", undefined, ["reply"]);
var heights = { "t:Old": { h: 120, paged: false }, "t:Paged": { h: 40, paged: true }, "t:Gone": { h: 30, paged: false } };
var large = new N("SECTION", "Large", ["topic"]); large.attrs["data-max"] = ""; heights["t:Large"] = { h: 90, paged: false };
check("new from 0, changed from its old height, paged, large, and non-topic still",
  topicMotion(heights, [bar, fresh, changed, paged, large]).map(function (m) { return [m.n.getAttribute("data-topic"), m.from]; }),
  [["New", 0], ["Old", 120]]);

fresh.h = 80; check("a growth is returned to start", growTopic(fresh, 0), "anim:New");
check("a new topic grows from 0 with fade and margin", fresh.anims,
  [[[{ height: "0px", opacity: 0, marginBottom: "0px" }, { height: "80px", opacity: 1, marginBottom: "16px" }], { duration: 200, easing: "ease-out" }]]);
changed.h = 60; growTopic(changed, 120);
check("a changed topic interpolates from its old height, filling nothing", changed.anims,
  [[[{ height: "120px" }, { height: "60px" }], { duration: 200, easing: "ease-out" }]]);
var same = new N("SECTION", "Same", ["topic"]); same.h = 50;
check("an unchanged height returns no growth", growTopic(same, 50.4), null);
check("an unchanged height does not animate", same.anims, []);

var board = { y: 100, children: [] };
board.getBoundingClientRect = function () { return { top: this.y }; };
function at(n, y) { n.y = y; board.children.push(n); return n; }
function card(id, max) { var c = new N("DIV", undefined, ["bcard"]); c.attrs["data-card"] = id; if (max) c.attrs["data-max"] = ""; return c; }
var up = at(new N("SECTION", "Up", ["topic"]), 100), down = at(new N("SECTION", "Down", ["topic"]), 180), crd = at(card("c1"), 260),
  maxed = at(card("c2", true), 300), born = at(new N("SECTION", "Born", ["topic"]), 340), unpaged = at(new N("SECTION", "Unpaged", ["topic", "closed"]), 400),
  stay = at(new N("SECTION", "Stay", ["topic"]), 420), other = at(new N("DIV", undefined, ["closed-more"]), 500),
  big = at(new N("SECTION", "Big", ["topic"]), 520), holder = at(new N("SECTION", "Holder", ["topic"]), 600);
big.attrs["data-max"] = ""; holder.panelMax = true;
var slides = slideBoard(board, { "t:Up": { h: 50, y: 230 }, "t:Down": { h: 70, y: 0 }, "c:c1": { y: 80 }, "c:c2": { y: 10 }, "t:Unpaged": { h: 20, y: 30, paged: true }, "t:Stay": { h: 20, y: 320.5 }, "t:Big": { h: 20, y: 0 }, "t:Holder": { h: 20, y: 0 } });
check("the slides are returned to start", slides, ["anim:Up", "anim:Down", "anim:undefined"]);
check("a topic slides from its old offset", up.anims, [[[{ transform: "translateY(230px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }]]);
check("a shifted topic slides back", down.anims, [[[{ transform: "translateY(-80px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }]]);
check("a card slides", crd.anims, [[[{ transform: "translateY(-80px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }]]);
check("a maximized card, a large topic, a topic holding a large panel, a new topic, one out of the closed pages, an unmoved one, and other nodes stay still",
  [maxed, big, holder, born, unpaged, stay, other].map(function (n) { return n.anims.length; }), [0, 0, 0, 0, 0, 0, 0]);

function decide(board, still, rm) {
  var el = {};
  function reducedMotion() { return rm; }
  function boardSpots() { return "heights"; }
`+before+`
  return before;
}
check("a live board swap reads spots", decide(true, undefined, false), "heights");
check("a deferred swap is still", decide(true, true, false), null);
check("reduced motion is still", decide(true, undefined, true), null);
check("another region reads none", decide(false, undefined, false), null);
process.stdout.write(bad.join("\n"));
`)
}
