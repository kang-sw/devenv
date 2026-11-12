package wsdashboard

import (
	"regexp"
	"testing"
)

// TestBoardFrameGraftStaysPut pins when a changed topic keeps its old
// section and body frame (dashboard.js frameGrafts, settleMoves): only when
// settling the board with the old section standing for the new one moves
// nothing the plain settle would not move, since moving a frame reloads it.
// settleMoves replays cards.js settle, so random boards check that the two
// agree on what moves. The cases execute under node when it is installed.
func TestBoardFrameGraftStaysPut(t *testing.T) {
	cards, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	board, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := regexp.MustCompile(`(?s)\n  function settle\(el, want, fixed\) \{.*?\n  \}\n`).FindString(string(cards))
	if src == "" {
		t.Fatal("cards.js holds no settle")
	}
	for _, name := range []string{"settleMoves", "frameGrafts"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(string(board))
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	runNode(t, "board frame graft", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`+src+`
// A node is a name; a card "C..." is fixed.
function N(name) { this.name = name; this.parentNode = null; this.nodeType = 1; }
N.prototype.hasAttribute = function (k) { return k === "data-card" && this.name.charAt(0) === "C"; };
Object.defineProperty(N.prototype, "nextSibling", { get: function () { var k = this.parentNode.kids; return k[k.indexOf(this) + 1] || null; } });
function El() { this.kids = []; this.moves = []; }
El.prototype = {
  get firstChild() { return this.kids[0] || null; },
  get childNodes() { return this.kids.slice(); },
  removeChild: function (n) { this.kids.splice(this.kids.indexOf(n), 1); n.parentNode = null; },
  insertBefore: function (n, ref) {
    if (n.parentNode === this) { this.kids.splice(this.kids.indexOf(n), 1); this.moves.push(n.name); }
    this.kids.splice(ref ? this.kids.indexOf(ref) : this.kids.length, 0, n);
    n.parentNode = this;
  }
};
function isCard(n) { return n.hasAttribute("data-card"); }
function boardOf(list) { var el = new El(); list.forEach(function (n) { el.insertBefore(n, null); }); el.moves = []; return el; }
function names(set) { return Array.from(set).map(function (n) { return n.name; }).sort(); }

// settleMoves agrees with settle on random boards: kept, stale and new
// nodes, cards among them.
var seed = 7;
function rnd(n) { seed = (seed * 1103515245 + 12345) % 2147483648; return seed % n; }
for (var round = 0; round < 500; round++) {
  var have = [], want = [];
  for (var i = 0, k = 2 + rnd(7); i < k; i++) have.push(new N((rnd(4) ? "T" : "C") + i));
  have.forEach(function (n) { if (isCard(n) || rnd(5)) want.push(n); });
  for (var j = 0, m = rnd(3); j < m; j++) want.splice(rnd(want.length + 1), 0, new N("Tnew" + j));
  // Shuffle the topics among themselves; cards keep their order.
  for (var s = 0; s < want.length; s++) {
    var x = rnd(want.length);
    if (!isCard(want[s]) && !isCard(want[x])) { var tmp = want[s]; want[s] = want[x]; want[x] = tmp; }
  }
  var el = boardOf(have), got = names(settleMoves(el, want));
  settle(el, want, isCard);
  check("settleMoves round " + round + " (" + have.map(function (n) { return n.name; }) + " -> " + want.map(function (n) { return n.name; }) + ")", got, el.moves.slice().sort());
}

// frameGrafts: g.n is the changed topic's new section, g.old its old one.
function graft(name) { return { old: new N(name), n: new N(name + "'") }; }
var a = new N("A"), w = new N("W"), b = graft("B");
check("a changed topic that stays at the head keeps its old section",
  frameGrafts(boardOf([b.old, a, w]), [b.n, a, w], [b]).length, 1);
check("a changed topic that stays in the middle keeps it",
  frameGrafts(boardOf([a, b.old, w]), [a, b.n, w], [b]).length, 1);
check("one that rises to the head takes its new section",
  frameGrafts(boardOf([a, w, b.old]), [b.n, a, w], [b]).length, 0);
check("one that sinks takes its new section, so the kept topics above it stay",
  frameGrafts(boardOf([b.old, a, w]), [a, w, b.n], [b]).length, 0);
var c = graft("C2");
var both = frameGrafts(boardOf([b.old, a, c.old, w]), [b.n, a, w, c.n], [b, c]);
check("of two changed topics only the one that stays keeps its old section",
  both.map(function (g) { return g.old.name; }), ["B"]);
var card = new N("Cx");
check("a changed topic beside a card stays when the card does",
  frameGrafts(boardOf([b.old, card, a]), [b.n, card, a], [b]).length, 1);
check("no grafts, none kept", frameGrafts(boardOf([a, w]), [a, w], []), []);
process.stdout.write(bad.join("\n"));
`)
}
