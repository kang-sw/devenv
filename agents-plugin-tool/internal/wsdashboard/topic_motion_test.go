package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestBoardTopicMotion pins the board swap's topic motion (dashboard.js
// topicMotion, growTopic, and their swap wiring): a topic the board did not
// hold grows from 0, a changed topic grows from its old rendered height,
// topics reconciled through the closed pages and non-topic nodes stay
// still; a deferred (editing) swap and reduced motion read no heights, so
// nothing animates; the animation fills nothing, so no fixed height stays
// behind. The cases execute under node when it is installed.
func TestBoardTopicMotion(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	key := regexp.MustCompile(`(?s)\n  function topicKey\(n\) \{.*?\n  \}\n`).FindString(js)
	motion := regexp.MustCompile(`(?s)\n  function topicMotion\(before, ins\) \{.*?\n  \}\n`).FindString(js)
	grow := regexp.MustCompile(`(?s)\n  function growTopic\(n, from\) \{.*?\n  \}\n`).FindString(js)
	before := regexp.MustCompile(`\n    var before = board [^\n]*\n`).FindString(js)
	if key == "" || motion == "" || grow == "" || before == "" {
		t.Fatal("dashboard.js holds no topicKey / topicMotion / growTopic / swap before-heights line")
	}
	if !strings.Contains(js, `swap("board", d.hash, d.html, true);`) {
		t.Error("the deferred board swap does not land still")
	}
	if !strings.Contains(js, `if (before) topicMotion(before, ins).forEach(function (m) { growTopic(m.n, m.from); });`) {
		t.Error("swap does not animate the inserted topics after preparing them")
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
N.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
N.prototype.getBoundingClientRect = function () { return { height: this.h }; };
N.prototype.animate = function (k, o) { this.anims.push([k, o]); };
function getComputedStyle() { return { marginBottom: "16px" }; }
`+key+motion+grow+`
var fresh = new N("SECTION", "New", ["topic"]), changed = new N("SECTION", "Old", ["topic"]), paged = new N("SECTION", "Paged", ["topic", "closed"]), bar = new N("DIV", undefined, ["reply"]);
var heights = { "t:Old": { h: 120, paged: false }, "t:Paged": { h: 40, paged: true }, "t:Gone": { h: 30, paged: false } };
check("new from 0, changed from its old height, paged and non-topic still",
  topicMotion(heights, [bar, fresh, changed, paged]).map(function (m) { return [m.n.getAttribute("data-topic"), m.from]; }),
  [["New", 0], ["Old", 120]]);

fresh.h = 80; growTopic(fresh, 0);
check("a new topic grows from 0 with fade and margin", fresh.anims,
  [[[{ height: "0px", opacity: 0, marginBottom: "0px" }, { height: "80px", opacity: 1, marginBottom: "16px" }], { duration: 200, easing: "ease-out" }]]);
changed.h = 60; growTopic(changed, 120);
check("a changed topic interpolates from its old height, filling nothing", changed.anims,
  [[[{ height: "120px" }, { height: "60px" }], { duration: 200, easing: "ease-out" }]]);
var same = new N("SECTION", "Same", ["topic"]); same.h = 50; growTopic(same, 50.4);
check("an unchanged height does not animate", same.anims, []);

function decide(board, still, rm) {
  var el = {};
  function reducedMotion() { return rm; }
  function topicHeights() { return "heights"; }
`+before+`
  return before;
}
check("a live board swap reads heights", decide(true, undefined, false), "heights");
check("a deferred swap is still", decide(true, true, false), null);
check("reduced motion is still", decide(true, undefined, true), null);
check("another region reads none", decide(false, undefined, false), null);
process.stdout.write(bad.join("\n"));
`)
}
