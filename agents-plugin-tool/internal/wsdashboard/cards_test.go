package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// cardScripts are the page scripts that draw vendored Lucide icons.
var cardScripts = []string{"assets/cards.js", "assets/preview.js", "assets/files.js", "assets/terminal.js"}

// TestLucideIconsAreVendoredWithTheirLicense pins the vendored icon set:
// the package's license file ships beside the icons, the sprite holds a
// symbol per icon, every icon a script asks for by name is vendored, and
// every vendored icon is drawn somewhere (only the icons used are here).
func TestLucideIconsAreVendoredWithTheirLicense(t *testing.T) {
	names, body, err := lucideIcons()
	if err != nil || len(names) == 0 {
		t.Fatalf("lucideIcons = %v, %v", names, err)
	}
	license, err := lucideLicense.ReadFile("assets/lucide/LICENSE")
	if err != nil || !strings.Contains(string(license), "ISC License") || !strings.Contains(string(license), "Lucide") {
		t.Fatalf("the vendored icons ship no Lucide ISC license: %v", err)
	}
	sprite, err := lucideSprite()
	if err != nil {
		t.Fatal(err)
	}
	var scripts strings.Builder
	for _, name := range cardScripts {
		raw, err := dashboardFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		scripts.Write(raw)
	}
	js := scripts.String()
	for _, name := range names {
		if !strings.Contains(string(sprite), `<symbol id="l-`+name+`" viewBox="0 0 24 24">`+body[name]+`</symbol>`) || body[name] == "" || strings.Contains(body[name], "<svg") {
			t.Errorf("the sprite lacks a clean symbol for %s", name)
		}
		if !strings.Contains(js, `"`+name+`"`) {
			t.Errorf("the vendored icon %s is drawn by no script", name)
		}
	}
	vendored := map[string]bool{}
	for _, name := range names {
		vendored[name] = true
	}
	for _, m := range regexp.MustCompile(`(?:licon\(|ibtn\("[a-z -]*", |iconBtn\(|icon: )"([a-z0-9-]+)"`).FindAllStringSubmatch(js, -1) {
		if !vendored[m[1]] {
			t.Errorf("a script draws the icon %q, which is not vendored", m[1])
		}
	}
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{`<symbol id="l-pin" viewBox="0 0 24 24">`, "window.WsCards = {", "window.WsPreview = {"} {
		if !strings.Contains(page, want) {
			t.Errorf("the session page lacks %q", want)
		}
	}
	if _, index := get(t, f.base+"/"); strings.Contains(index, "window.WsCards = {") {
		t.Error("the index page, which has no board, carries the card framework")
	}
}

// TestBoardCardPlacement pins the card framework's placement (cards.js
// order and settle), executed under node when it is installed: pinned
// cards first in pin order; an unpinned card below every open topic
// updated after it opened and above everything else (older topics, closed
// topics, the board's own controls); and a board update that settles
// around the cards without ever moving one, whatever the topics do.
func TestBoardCardPlacement(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	order := regexp.MustCompile(`(?s)\n  function order\(base, list, at\) \{.*?\n  \}\n`).FindString(js)
	settle := regexp.MustCompile(`(?s)\n  function settle\(el, want, fixed\) \{.*?\n  \}\n`).FindString(js)
	if order == "" || settle == "" {
		t.Fatal("cards.js holds no order / settle")
	}
	board, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(board), "window.WsCards.settle(el, window.WsCards.weave(want));") {
		t.Error("the board morph does not weave the cards in and settle around them")
	}
	runNode(t, "board card placement", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`+order+settle+`
// A node is a name; a topic "T<time>" is open and updated then, "X..." is
// closed or a control. A card "C..." carries its record.
function at(n) { return n.name.charAt(0) === "T" ? +n.name.slice(1) : null; }
function N(name) { this.name = name; this.parentNode = null; }
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
Object.defineProperty(N.prototype, "nextSibling", { get: function () { var k = this.parentNode.kids; return k[k.indexOf(this) + 1] || null; } });
function names(list) { return list.map(function (n) { return n.name; }); }
function card(name, opened, pinned) { return { el: new N(name), at: opened, pinned: pinned || 0 }; }
function nodes(s) { return s.split(" ").filter(Boolean).map(function (x) { return new N(x); }); }
function isCard(n) { return n.name.charAt(0) === "C"; }

var a = card("Ca", 50), b = card("Cb", 20), p1 = card("Cp1", 5, 200), p2 = card("Cp2", 90, 100);
check("pinned first in pin order, then each card below the topics newer than it",
  names(order(nodes("T60 T40 T30 T10 Xclosed Xmore Xreply"), [a, b, p1, p2], at)),
  ["Cp2", "Cp1", "T60", "Ca", "T40", "T30", "Cb", "T10", "Xclosed", "Xmore", "Xreply"]);
check("a card newer than every topic heads the unpinned flow",
  names(order(nodes("T40 T30"), [card("Cn", 99), p1], at)), ["Cp1", "Cn", "T40", "T30"]);
check("a card older than every open topic still stands above the closed ones",
  names(order(nodes("T40 T30 Xclosed Xreply"), [card("Co", 1)], at)), ["T40", "T30", "Co", "Xclosed", "Xreply"]);
check("an empty board holds its cards", names(order(nodes("Xempty"), [a, p1], at)), ["Cp1", "Ca", "Xempty"]);
check("a topic with no time is older than any card", names(order(nodes("T40 T0"), [card("Cz", 1)], at)), ["T40", "Cz", "T0"]);
check("two cards opened at the same place keep newest first",
  names(order(nodes("T60 T10"), [card("C1", 30), card("C2", 40)], at)), ["T60", "C2", "C1", "T10"]);

// settle: a board update around the cards.
function boardOf(list) { var el = new El(); list.forEach(function (n) { el.insertBefore(n, null); }); el.moves = []; return el; }
var t60 = new N("T60"), t40 = new N("T40"), t10 = new N("T10"), reply = new N("Xreply");
var el = boardOf([p1.el, a.el, t40, b.el, t10, reply]);
// T10 is updated (a new node T70) and rises above the card Ca; T40 is kept.
var t70 = new N("T70"), reply2 = new N("Xreply");
settle(el, order([t70, t40, reply2], [a, b, p1], at), isCard);
check("an updated topic rises above a card", names(el.kids), ["Cp1", "T70", "Ca", "T40", "Cb", "Xreply"]);
check("and nothing kept was moved", el.moves, []);
check("stale nodes are gone", [t10.parentNode, reply.parentNode], [null, null]);
// A kept topic that must pass a card moves; the card never does.
el = boardOf([a.el, t40, t10]);
settle(el, [t40, a.el, t10], isCard);
check("a kept topic wanted above a card", names(el.kids), ["T40", "Ca", "T10"]);
check("moves past it, the card staying put", el.moves, ["T40"]);
el = boardOf([t40, t10, a.el, b.el, t60]);
settle(el, [a.el, t60, b.el, t40, t10], isCard);
check("topics reorder around two fixed cards", names(el.kids), ["Ca", "T60", "Cb", "T40", "T10"]);
check("with no card moved", el.moves.filter(function (n) { return n.charAt(0) === "C"; }), []);
// A new card (not in the board yet) is inserted at its place.
var fresh = card("Cf", 45);
el = boardOf([t60, t40, t10]);
settle(el, order([t60, t40, t10], [fresh], at), isCard);
check("a new card enters at its place", names(el.kids), ["T60", "Cf", "T40", "T10"]);
process.stdout.write(bad.join("\n"));
`)
}
