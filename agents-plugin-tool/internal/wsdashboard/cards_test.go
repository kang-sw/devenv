package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// cardScripts are the page scripts that draw vendored Lucide icons.
var cardScripts = []string{"assets/cards.js", "assets/preview.js", "assets/files.js", "assets/terminal.js", "assets/webcard.js", "assets/dashboard.js"}

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
		raw, err := dashboardTestAsset(name)
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
	for _, m := range regexp.MustCompile(`(?:licon\(|luIcon\(|ibtn\("[a-z -]*", |iconBtn\(|icon: )"([a-z0-9-]+)"`).FindAllStringSubmatch(js, -1) {
		if !vendored[m[1]] {
			t.Errorf("a script draws the icon %q, which is not vendored", m[1])
		}
	}
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession?frame=1")
	page = frameDependencies(t, page)
	for _, want := range []string{`<symbol id="l-pin" viewBox="0 0 24 24">`, "window.WsCards = {", "window.WsPreview = {"} {
		if !strings.Contains(page, want) {
			t.Errorf("the session page lacks %q", want)
		}
	}
	if _, index := get(t, f.base+"/?frame=1"); strings.Contains(frameDependencies(t, index), "window.WsCards = {") {
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
	settle := regexp.MustCompile(`(?s)\n  function settle\(el, want, fixed, skip\) \{.*?\n  \}\n`).FindString(js)
	if order == "" || settle == "" {
		t.Fatal("cards.js holds no order / settle")
	}
	board, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	// Frozen viewed staging may reorder plan lists between the weave and the
	// read of the woven board order, so the two are separate statements.
	if !strings.Contains(string(board), "var plan = window.WsCards.weave(want);") || !strings.Contains(string(board), "var woven = plan.board;") || !strings.Contains(string(board), "window.WsCards.settle(el, plan);") {
		t.Error("the board morph does not weave the cards in and settle around them")
	}
	runNode(t, "board card placement", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`+order+`function rank(u) { return typeof u.rank === "number" ? u.rank : -1; }
`+settle+`
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

// One-time placement snapshots use a time plus an independent tie rank,
// never a topic identity. Negative time addresses the ordinary closed tail.
var tied = card("Ctie", 40); tied.rank = 0.5;
check("a snapshot can sit between equal-time topics", names(order(nodes("T60 T40 T40 T10"), [tied], at)), ["T60", "T40", "Ctie", "T40", "T10"]);
var tail = card("Ctail", -1); tail.rank = 0.5;
check("a snapshot can sit between closed topics", names(order(nodes("T40 Xone Xtwo Xreply"), [tail], at)), ["T40", "Xone", "Ctail", "Xtwo", "Xreply"]);

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
// Where the browser has Element.moveBefore, a kept node moves with it (its
// frames stay loaded) and a new node still enters with insertBefore, which
// moveBefore refuses for a node from outside the board; a moveBefore that
// throws falls back to insertBefore.
El.prototype.moveBefore = function (n, ref) {
  if (n.parentNode !== this) throw new Error("moveBefore: not a child");
  if (this.refuse) throw new Error("moveBefore: refused");
  this.kids.splice(this.kids.indexOf(n), 1); this.moved.push(n.name);
  this.kids.splice(ref ? this.kids.indexOf(ref) : this.kids.length, 0, n);
};
var t30 = new N("T30");
el = boardOf([t40, t10, a.el]); el.moved = [];
settle(el, [t10, t30, a.el, t40], isCard);
check("with moveBefore the board settles the same", names(el.kids), ["T10", "T30", "Ca", "T40"]);
check("kept topics move with moveBefore", el.moved, ["T10", "T40"]);
check("and the new one enters with insertBefore", el.moves, []);
el = boardOf([t40, t10]); el.moved = []; el.refuse = true;
settle(el, [t10, t40], isCard);
check("a refused moveBefore falls back to insertBefore", [names(el.kids), el.moved, el.moves], [["T10", "T40"], [], ["T10"]]);
delete El.prototype.moveBefore;
process.stdout.write(bad.join("\n"));
`)
}

func TestSharedCardActionSnapshots(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := json.Marshal(string(data))
	runNode(t, "shared card action snapshots", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, A, B, C;
function served() { A = topic("a", 300); B = topic("b", 200); C = topic("c", 200); return [A, B, C, topic("x", 0, true), topic("y", 0, true), reply()]; }
boot(store, served);
var c = page.W.open("k", {name: "preview"}, undefined, C);
var topicRaise = A.querySelector(".t-raise"), cardRaise = c.raiseBtn;
check("topic and generic raise icons use the upward top-line icon", [topicRaise.html.indexOf("#l-arrow-up-to-line") >= 0, cardRaise.html.indexOf("#l-arrow-up-to-line") >= 0], [true, true]);
check("topic and generic raise labels remain accessible", [topicRaise.getAttribute("aria-label"), cardRaise.getAttribute("aria-label")], ["js.card.raise", "js.card.raise"]);
check("new card immediately above a tied origin", names(page.board), ["TOOLBAR", "a", "b", "preview", "c", "x/c", "y/c", "reply"]);
c.bump(B);
check("reused slot moves above its new origin", names(page.board), ["TOOLBAR", "a", "preview", "b", "c", "x/c", "y/c", "reply"]);
c.bump(page.board.querySelector('section[data-topic="y"]'));
check("ordinary closed origin also has adjacency", names(page.board), ["TOOLBAR", "a", "b", "c", "x/c", "preview", "y/c", "reply"]);
boot(store, served);
check("snapshot survives reload", names(page.board), ["TOOLBAR", "a", "b", "c", "x/c", "preview", "y/c", "reply"]);
c = page.W.list()[0]; clickPin(B); c.bump(B);
check("pinned origin sends ordinary preview to ordinary head", names(page.board), ["TOOLBAR", "preview", "a", "c", "x/c", "y/c", "reply"]);
var detached = topic("a", 300); c.bump(detached);
check("detached origin falls back to head", names(page.board)[1], "preview");
var fold = A.querySelector(".t-fold");
page.board.fire("click", {target: fold});
check("topic whole fold is independent state", [A.getAttribute("data-size"), fold.getAttribute("aria-expanded")], ["fold", "false"]);
swap([topic("a", 400), B, C, reply()]);
check("topic update retains whole fold", page.board.querySelector('section[data-topic="a"]').getAttribute("data-size"), "fold");
boot(store, served);
check("fold-only topic state restores", A.getAttribute("data-size"), "fold");
c = page.W.list()[0];
var oldA = A;
swap([topic("a", 800), B, C, reply()]);
c.bump(oldA);
check("replaced same-name origin is not borrowed", names(page.board)[1], "preview");
c.bump(mk("div", {"data-topic": "b"}));
check("non-topic origin falls back safely", names(page.board)[1], "preview");
check("no origin identity persists", JSON.stringify(store).indexOf("origin"), -1);
var current = page.board.querySelector('section[data-topic="a"]');
page.W.decorate(); page.W.decorate();
page.board.fire("click", {target: current.querySelector(".t-fold")});
check("redecoration adds no duplicate fold handler", current.getAttribute("data-size"), "base");
check("topic fold is stored in f", JSON.parse(store.cards).topics.filter(function (r) { return r.n === "a"; }).map(function (r) { return !!r.f; }), []);
store = {};
function noTime() { return [topic("first", 300), topic("zero", 0), reply()]; }
boot(store, noTime);
c = page.W.open("k", {name: "zero-preview"}, undefined, page.board.querySelector('section[data-topic="zero"]'));
check("a zero-time origin is not a default open", [c.at, names(page.board)], [0, ["TOOLBAR", "first", "zero-preview", "zero", "reply"]]);
boot(store, noTime);
check("zero-time snapshot survives reload", names(page.board), ["TOOLBAR", "first", "zero-preview", "zero", "reply"]);
process.stdout.write(bad.join("\n"));
`)
}

func TestBoardTopicRevealUsesFoldOwner(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := json.Marshal(string(data))
	runNode(t, "topic navigation fold owner", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, A, B;
function withDisclosures(name, at) {
  var t = topic(name, at), body = t.appendChild(mk("details.tdoc")), versions = t.appendChild(mk("details.panel-versions"));
  body.open = true; versions.open = false;
  return t;
}
function served() { A = withDisclosures("a", 300); B = withDisclosures("b", 200); return [A, B]; }
function records() { return store.cards ? JSON.parse(store.cards).topics : []; }
function fold(t) { page.board.fire("click", {target: t.querySelector(".t-fold")}); }
boot(store, served, {wide: true});
page.W.unfoldTopic(A);
check("an unfolded no-op creates no topic record", records(), []);
fold(A); fold(B);
var oldA = A;
A = withDisclosures("a", 400);
swap([A, B]);
var saved = store.cards, outside = withDisclosures("a", 500);
page.doc.body.appendChild(outside);
[oldA, topic("a", 600), outside, mk("div", {"data-topic": "a"}), null].forEach(page.W.unfoldTopic);
check("detached, replaced, off-board and non-topic nodes cannot borrow the live record", [store.cards, A.getAttribute("data-size")], [saved, "fold"]);
var body = A.querySelector(".tdoc"), versions = A.querySelector(".panel-versions"), focused = body.appendChild(mk("input")), focusMoves = 0;
El.prototype.focus = function () { focusMoves++; this.ownerDocument.activeElement = this; };
page.doc.activeElement = focused;
page.W.unfoldTopic(focused);
check("reveal decorates through the fold owner", [A.getAttribute("data-size"), A.querySelector(".t-fold").getAttribute("aria-expanded")], ["base", "true"]);
check("reveal removes a fold-only record, not another topic's fold", records().map(function (r) { return [r.n, r.f]; }), [["b", true]]);
check("reveal preserves independent disclosures and focus", [body.open, versions.open, B.getAttribute("data-size"), page.doc.activeElement === focused, focusMoves], [true, false, "fold", true, 0]);
saved = store.cards;
page.W.unfoldTopic(A); page.W.decorate();
check("reveal is idempotent and later decoration does not refold", [store.cards, A.getAttribute("data-size")], [saved, "base"]);
boot(store, served, {wide: true});
check("reveal persists across reload without clearing unrelated folds", [A.getAttribute("data-size"), B.getAttribute("data-size")], ["base", "fold"]);
clickPin(A); fold(A);
var pinned = records().filter(function (r) { return r.n === "a"; })[0];
page.W.unfoldTopic(A.querySelector(".tdoc"));
var revealed = records().filter(function (r) { return r.n === "a"; })[0];
check("docked reveal preserves placement and its record", [page.W.isDocked(A), revealed.p, revealed.a, revealed.b, revealed.f], [true, pinned.p, pinned.a, pinned.b, false]);
check("pinned reveal still leaves inner disclosures alone", [A.querySelector(".tdoc").open, A.querySelector(".panel-versions").open], [true, false]);
boot(store, served, {wide: true});
check("docked unfolded state survives reload", [page.W.isDocked(A), A.getAttribute("data-size"), B.getAttribute("data-size")], [true, "base", "fold"]);
process.stdout.write(bad.join("\n"));
`)
}

// boardBoxSrc is dashboard.js's board-box helpers (boardBoxes, boardQ,
// inBoard, boardRead), for a node harness that runs a function reading the
// board's topics wherever they stand: the board region or the pin list.
func boardBoxSrc(t *testing.T, js string) string {
	t.Helper()
	src := regexp.MustCompile(`(?s)\n  function boardBoxes\(\) \{.*?\n  function boardRead\(read\) \{[^\n]*\n`).FindString(js)
	if src == "" {
		t.Fatal("dashboard.js holds no board-box helpers")
	}
	return src
}

// cardsDOM is a minimal DOM for running the whole of cards.js under node:
// elements with attributes, classes, children, and a selector subset
// (descendant and child combinators over tag, .class, [attr], [attr="v"],
// and comma lists), a document with a body, and boot, which loads
// gallery.js and cards.js (as CardsScript ships them) into a fresh page
// over a storage map, as a reload does.
var cardsDOM = galleryPrelude() + cardsDOMBase

// galleryPrelude is gallery.js's source as the harness's GALLERY.
func galleryPrelude() string {
	data, err := dashboardFS.ReadFile("assets/gallery.js")
	if err != nil {
		panic(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		panic(err)
	}
	return "var GALLERY = " + string(src) + ";\n"
}

const cardsDOMBase = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var now = 1000;
Date.now = function () { return now; };
function Cls(el) { this.el = el; }
Cls.prototype = {
  list: function () { return (this.el.attrs["class"] || "").split(/\s+/).filter(Boolean); },
  set: function (l) { this.el.attrs["class"] = l.join(" "); },
  contains: function (c) { return this.list().indexOf(c) >= 0; },
  add: function () { var l = this.list(); Array.prototype.forEach.call(arguments, function (c) { if (l.indexOf(c) < 0) l.push(c); }); this.set(l); },
  remove: function () { var a = Array.prototype.slice.call(arguments); this.set(this.list().filter(function (c) { return a.indexOf(c) < 0; })); },
  toggle: function (c, on) { if (on === undefined) on = !this.contains(c); if (on) this.add(c); else this.remove(c); return on; }
};
function El(tag, doc) {
  this.tagName = tag.toUpperCase(); this.nodeType = 1; this.attrs = {}; this.kids = []; this.parentNode = null; this.ownerDocument = doc;
  this.classList = new Cls(this); this.style = { props: {}, setProperty: function (k, v) { this.props[k] = v; }, removeProperty: function (k) { delete this.props[k]; } };
  this.listeners = {}; this.hidden = false; this.text = "";
}
El.prototype = {
  get className() { return this.attrs["class"] || ""; }, set className(v) { this.attrs["class"] = v; },
  get id() { return this.attrs.id || ""; }, set id(v) { this.attrs.id = v; },
  setAttribute: function (k, v) { this.attrs[k] = String(v); }, getAttribute: function (k) { return k in this.attrs ? this.attrs[k] : null; },
  hasAttribute: function (k) { return k in this.attrs; }, removeAttribute: function (k) { delete this.attrs[k]; },
  get childNodes() { return this.kids.slice(); }, get children() { return this.kids.slice(); },
  get firstChild() { return this.kids[0] || null; }, get firstElementChild() { return this.kids[0] || null; },
  get nextSibling() { var k = this.parentNode && this.parentNode.kids; return k ? k[k.indexOf(this) + 1] || null : null; },
  get nextElementSibling() { return this.nextSibling; },
  get previousElementSibling() { var k = this.parentNode && this.parentNode.kids; return k ? k[k.indexOf(this) - 1] || null : null; },
  get isConnected() { for (var n = this; n; n = n.parentNode) if (n === this.ownerDocument.documentElement) return true; return false; },
  contains: function (n) { for (; n; n = n.parentNode) if (n === this) return true; return false; },
  insertBefore: function (n, ref) {
    if (n.parentNode) n.parentNode.removeChild(n);
    this.kids.splice(ref ? this.kids.indexOf(ref) : this.kids.length, 0, n); n.parentNode = this; return n;
  },
  appendChild: function (n) { return this.insertBefore(n, null); },
  removeChild: function (n) { this.kids.splice(this.kids.indexOf(n), 1); n.parentNode = null; return n; },
  set innerHTML(v) { this.kids.forEach(function (k) { k.parentNode = null; }); this.kids = []; this.html = v; },
  get outerHTML() { return "<" + this.tagName + JSON.stringify(this.attrs) + ">" + this.kids.map(function (k) { return k.outerHTML; }).join("") + this.text; },
  get textContent() { return this.text + this.kids.map(function (k) { return k.textContent; }).join(""); },
  set textContent(v) { this.kids = []; this.text = String(v); },
  addEventListener: function (t, fn) { (this.listeners[t] = this.listeners[t] || []).push(fn); },
  fire: function (t, e) { (this.listeners[t] || []).forEach(function (fn) { fn(e); }); },
  focus: function () {}, getBoundingClientRect: function () { return { top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 }; }, getClientRects: function () { return []; },
  matches: function (sel) { var me = this; return sel.split(",").some(function (s) { return match(me, s.trim().split(/\s+/)); }); },
  closest: function (sel) { for (var n = this; n && n.nodeType === 1; n = n.parentNode) if (n.matches(sel)) return n; return null; },
  querySelectorAll: function (sel) { var out = []; (function walk(n) { n.kids.forEach(function (k) { if (k.matches(sel) && k.inside(n, sel)) out.push(k); walk(k); }); })(this); return out; },
  querySelector: function (sel) { return this.querySelectorAll(sel)[0] || null; },
  inside: function () { return true; }
};
function one(el, s) {
  var m = /^([a-z0-9]*)((?:\.[a-z0-9-]+|\[[a-z-]+(?:="[^"]*")?\])*)$/i.exec(s);
  if (!m) throw new Error("selector " + s);
  if (m[1] && el.tagName !== m[1].toUpperCase()) return false;
  var parts = m[2].match(/\.[a-z0-9-]+|\[[^\]]+\]/gi) || [];
  return parts.every(function (p) {
    if (p.charAt(0) === ".") return el.classList.contains(p.slice(1));
    var a = /^\[([a-z-]+)(?:="([^"]*)")?\]$/i.exec(p);
    return el.hasAttribute(a[1]) && (a[2] === undefined || el.getAttribute(a[1]) === a[2]);
  });
}
function match(el, chain) {
  if (!one(el, chain[chain.length - 1])) return false;
  var rest = chain.slice(0, -1);
  if (!rest.length) return true;
  if (rest[rest.length - 1] === ">") return !!el.parentNode && el.parentNode.nodeType === 1 && match(el.parentNode, rest.slice(0, -1));
  for (var n = el.parentNode; n && n.nodeType === 1; n = n.parentNode) if (match(n, rest)) return true;
  return false;
}
function Doc() {
  var d = this;
  this.listeners = {};
  this.documentElement = new El("html", d);
  this.body = this.documentElement.appendChild(new El("body", d));
}
Doc.prototype = {
  createElement: function (t) { return new El(t, this); },
  addEventListener: function (t, fn) { (this.listeners[t] = this.listeners[t] || []).push(fn); },
  getElementById: function (id) { return this.documentElement.querySelector('[id="' + id + '"]'); }
};
var page = null;
function wsT(k) { return k; }
function toolbarCounts() {
  var inline = page.W.toolbar().querySelector(".board-state-inline");
  function count(value) { return +inline.querySelector('[data-filter="' + value + '"] .board-state-count').textContent; }
  var closed = count("closed");
  return "open " + (count("all") - closed) + " · closed " + closed;
}
// mk builds an element: tag.class.class with attributes and children.
function mk(spec, attrs, kids) {
  var p = spec.split("."), e = new El(p[0], page.doc);
  if (p.length > 1) e.className = p.slice(1).join(" ");
  Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
  (kids || []).forEach(function (k) { e.appendChild(k); });
  return e;
}
// topic builds a topic section as the board serves it, its bar in its head:
// open with its bar time, or closed; fresh adds the update mark.
function topic(name, at, closed, fresh) {
  var bar = mk("div.tbar", {}, [mk("span.tlead", {}, [mk("span.tname")])].concat(closed ? [] : [mk("time.tago", { "data-at": at })]).concat(fresh ? [mk("span.chip.fresh")] : []).concat([mk("button.btn.mini.tnote-toggle")]));
  var t = mk("section.topic" + (closed ? ".closed" : ""), { "data-topic": name }, [mk("div.tophead", {}, [bar])]);
  t.label = name + (closed ? "/c" : "") + (fresh ? "*" : "");
  return t;
}
function more(next, remainder) { var b = mk("button.btn.closed-more", { "data-closed-next": next }); if (remainder !== undefined) b.setAttribute("data-closed-more", remainder); b.label = "more" + next; return b; }
function reply() { var r = mk("div.reply"); r.label = "reply"; return r; }
function label(n) { return n.label || (n.hasAttribute("data-card") ? n.card || "card" : n.classList.contains("board-toolbar") ? "TOOLBAR" : n.tagName); }
function names(el) { return el.children.map(label); }
// boot loads gallery.js and cards.js into a fresh page whose board region
// holds served, over store (a reload keeps it), wide (1800px) or narrow
// (1000px; page.width changes it before a relayout), and with split a grid
// for the split handle (its resolved columns: page.gridCols). page.layouts
// records each change of whether the gallery shows as a column beside
// the board, page.states every layout state the framework hands the page.
function boot(store, served, o) {
  o = o || {};
  page = { doc: null, store: store, layouts: [], states: [], closedCalls: [], adopted: [], gridCols: "none", width: o.wide ? 1800 : 1000, tabs: false };
  page.doc = new Doc();
  var dbody = page.dbody = page.doc.body.appendChild(new El("div", page.doc));
  page.board = dbody.appendChild(new El("div", page.doc));
  page.board.id = "r-board";
  served().forEach(function (n) { page.board.appendChild(n); });
  page.split = o.split ? dbody.appendChild(new El("main", page.doc)) : null;
  var win = page.win = { listeners: {}, getSelection: function () { return null; }, addEventListener: function (type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); }, innerWidth: 2000, innerHeight: 800 };
  new Function("document", "window", "wsT", "getComputedStyle", "requestAnimationFrame", "module", GALLERY + "\n;\n" + CARDS)(page.doc, win, wsT, function () { return { overflowY: "visible", gridTemplateColumns: page.gridCols }; }, function () {}, undefined);
  page.W = win.WsCards;
  page.G = win.WsGallery;
  page.W.register("k", { open: function (c) { c.title({ name: c.data.name }); c.el.card = c.data.name; } });
  (o.kinds || []).forEach(function (kind) { page.W.register(kind, { open: function () {} }); });
  var col = false;
  page.W.mount(page.board, {
    toolbar: o.toolbar, host: dbody,
    key: "cards", load: function (k) { return store[k] || null; }, save: function (k, v) { if (v === null) delete store[k]; else store[k] = v; },
    mode: function () { return { width: page.width, tabs: page.tabs }; }, split: page.split,
    layout: function (s) { page.states.push(s); var on = s.column && s.pages > 0; if (on !== col) { col = on; page.layouts.push(on); } },
    closed: function (from, done) { page.closedCalls.push(from); if (!o.pending) done(o.closed ? o.closed(from) : null); },
    adopted: function (n) { page.adopted.push(n.getAttribute("data-topic")); }
  });
  // The gallery's element is the wide view's column, its width handle on it.
  page.col = page.W.gallery();
  return page;
}
// swap is the board morph's weave and settle for the nodes the board wants.
function swap(nodes, graft) {
  var p = page.W.weave(nodes);
  if (graft) graft(p);
  page.W.settle(page.board, p);
  return p;
}
function pinBtn(t) { return t.querySelector(".tbar .tpin"); }
function clickPin(t) { var b = pinBtn(t); (page.W.gallery().contains(t) ? page.W.gallery() : page.board).fire("click", { target: b }); }
// pinsOf is the docked units in pin order; pagesOf the gallery's pages,
// each its panes' units; shownOf the page shown (its index, "+" or "home").
function pinsOf() { return page.W.docked().map(label); }
// The pages' DOM order never changes (a page switch moves nothing), so
// pagesOf reads them in the stored gallery's order.
function pagesOf() {
  var els = page.W.gallery().querySelectorAll(".gal-page"), g = JSON.parse(page.store.cards || "{}").gallery || { pages: [] };
  check("one slide per page, and +", els.length, g.pages.length + 1);
  return g.pages.map(function (pg) { var p = els.filter(function (e) { return e.getAttribute("data-page") === pg.id; })[0]; return p ? p.querySelectorAll(".gal-pane").map(function (pane) { return pane.children.map(label).join(""); }) : null; });
}
function galOf() { return JSON.parse(page.store.cards || "{}").gallery; }
function paneOf(n) { return n.parentNode && n.parentNode.classList.contains("gal-pane") ? n.parentNode : null; }
`

// TestBoardTopicPins pins whiteboard topics in the card framework's
// placement (cards.js), the whole script executed under node over a
// minimal DOM when node is installed: a topic's title bar carries a pin
// button; a pinned topic stands in the pin list among the pinned cards, in
// pin order, atop the board or, where the page shows it, in the pinned
// column; an unpinned topic moved by hand stands where it was dropped until
// a topic updated later rises above it, and its own update ends the move;
// an updated, closed, or no longer served pinned topic keeps its index (the
// update mark is the board's own); a topic updated through the frame graft
// keeps its pin button pressed; after a reload pins and moves come back
// from storage, a pinned topic the board lacks is fetched through the
// closed route page by page, and one the route ends without loses its pin,
// while a board with no show-more control (empty, not yet loaded) keeps
// every pin; a topic of a fetched closed page gets its pin button; a pin dropped while
// a pinned topic is still being fetched keeps that topic's order.
func TestBoardTopicPins(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "board topic pins", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, A, B, C, R;
function served1() { A = topic("a", 300); B = topic("b", 200); C = topic("c", 100); var X = topic("x", 0, true); R = reply(); return [A, B, C, X, more(31), R]; }
boot(store, served1);
check("every topic bar has a pin button, then the maximize and back buttons, before its note pen, unpressed",
  [A, B, C].map(function (t) { var b = pinBtn(t); return [!!b, b && b.parentNode.children.slice(b.parentNode.children.indexOf(b)).map(function (x) { return x.className; }), b && b.getAttribute("aria-pressed")]; }),
  [A, B, C].map(function () { return [true, ["ibtn tpin", "ibtn t-fold", "ibtn t-raise", "ibtn d-max", "ibtn d-min", "btn mini tnote-toggle"], "false"]; }));
check("the toolbar heads the board, which holds no pin list", names(page.board), ["TOOLBAR", "a", "b", "c", "x/c", "more31", "reply"]);
check("the gallery joins the page body, hidden with nothing docked", [page.W.gallery().parentNode === page.dbody, page.W.gallery().id, page.W.gallery().hidden], [true, "bgal", true]);

// A pinned topic docks among the docked cards, each on a page of its own.
now = 1100; var ca = page.W.open("k", { name: "Ca" }); ca.pin(true);
now = 1200; clickPin(B);
now = 1300; var cb = page.W.open("k", { name: "Cb" }); cb.pin(true);
check("a docked topic stands among the docked cards, in pin order", pinsOf(), ["Ca", "b", "Cb"]);
check("each docks on a new page before +", pagesOf(), [["Ca"], ["b"], ["Cb"]]);
check("its pin button is pressed and the section marked", [pinBtn(B).getAttribute("aria-pressed"), B.hasAttribute("data-docked")], ["true", true]);
check("the board keeps the rest in its order", names(page.board), ["TOOLBAR", "a", "c", "x/c", "more31", "reply"]);
cb.pin(false);
check("an undocked card enters the flow at its open time, its emptied page gone", [names(page.board), pagesOf()], [["TOOLBAR", "Cb", "a", "c", "x/c", "more31", "reply"], [["Ca"], ["b"]]]);
cb.close();

// An unpinned hand move is undone by a later bump above it.
now = 2000;
page.W.move(C, { pin: false, after: null, before: A });
check("a topic moved by hand stands where it was dropped", names(page.board), ["TOOLBAR", "c", "a", "x/c", "more31", "reply"]);
var A2 = topic("a", 2500);
swap([A2, C, topic("x", 0, true), more(31), reply()]);
check("a topic updated later rises above it", names(page.board), ["TOOLBAR", "a", "c", "x/c", "more31", "reply"]);
var stash = JSON.parse(store.cards);
check("the move is kept with its bar time", stash.topics.filter(function (r) { return r.n === "c"; }).map(function (r) { return [r.a > 2000 - 1, r.b]; }), [[true, 100]]);

// An updated docked topic keeps its pane and gains the update mark.
now = 2600; clickPin(A2);
check("pins before the update", pinsOf(), ["Ca", "b", "a"]);
var B2 = topic("b", 2700, false, true);
swap([B2, A2, C, topic("x", 0, true), more(31), reply()]);
check("an updated docked topic keeps its index, with the board's update mark", [pinsOf(), pagesOf()], [["Ca", "b*", "a"], [["Ca"], ["b*"], ["a"]]]);
check("its new bar carries the pressed pin", pinBtn(B2).getAttribute("aria-pressed"), "true");

// A docked topic updated through the frame graft keeps its pin pressed.
var oldHead = A2.querySelector(".tophead"), A3 = topic("a", 2800, false, true), paneA = paneOf(A2);
swap([B2, A3, C, topic("x", 0, true), more(31), reply()], function (p) {
  // The morph keeps the old section and gives it the new head's content.
  A2.removeChild(oldHead); A2.insertBefore(A3.querySelector(".tophead"), A2.firstChild);
  p.pins[p.pins.indexOf(A3)] = A2;
});
check("a docked topic updated through the graft keeps its section, pane and index", [pinsOf(), page.W.docked()[2] === A2, paneOf(A2) === paneA], [["Ca", "b*", "a"], true, true]);
check("and its pin button pressed", pinBtn(A2) && pinBtn(A2).getAttribute("aria-pressed"), "true");

// A closed docked topic stays docked; past the closed cap it stays too.
var B3 = topic("b", 0, true);
swap([A2, C, B3, topic("x", 0, true), more(31), reply()]);
check("a docked topic that closes stays in its pane, closed", pinsOf(), ["Ca", "b/c", "a"]);
swap([A2, C, topic("x", 0, true), more(31), reply()]);
check("one the board no longer serves keeps its last node there", [pinsOf(), page.W.docked()[1] === B3, !!paneOf(B3)], [["Ca", "b/c", "a"], true, true]);

// The wide view: the gallery shows as a column beside the board.
var saved = store.cards;
boot(store, function () { return [topic("a", 2800), topic("c", 100), topic("x", 0, true), more(31), reply()]; }, {
  wide: true,
  closed: function (from) {
    if (from === 31) { var p = mk("div.closed-page", {}, [topic("y", 0, true), more(61)]); return p; }
    return mk("div.closed-page", {}, [topic("z", 0, true), topic("b", 0, true)]);
  }
});
check("after a reload the board's missing docked topic is fetched page by page", [page.closedCalls, page.adopted], [[31, 61], ["b"]]);
check("pins, pages and their order survive the reload, the gallery a column", [pinsOf(), pagesOf(), page.layouts], [["Ca", "b/c", "a"], [["Ca"], ["b/c"], ["a"]], [true]]);
check("the board column holds the rest, the hand-moved topic where it stood",
  names(page.board), ["TOOLBAR", "c", "x/c", "more31", "reply"]);
check("the docked topic has its pin pressed", pinBtn(page.W.docked()[2]).getAttribute("aria-pressed"), "true");
// Undocking the last ones drops the column; an open topic goes back by
// recency, a closed one to the head of the closed section.
page.W.list("k")[0].close();
clickPin(page.W.docked()[0]);
clickPin(page.W.docked()[0]);
check("the last undock drops the column and its pages", [page.layouts, names(page.board), pagesOf(), galOf().pages], [[true, false], ["TOOLBAR", "a", "c", "b/c", "x/c", "more31", "reply"], [], []]);

// A docked topic whose topic is gone is dropped, its page with it.
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), topic("c", 100), more(31), reply()]; }, { closed: function () { return mk("div.closed-page", {}, [topic("q", 0, true)]); } });
check("a docked topic the closed route ends without leaves the record and the gallery",
  [page.closedCalls, JSON.parse(store.cards).topics.map(function (r) { return r.n; }).sort(), galOf().pages.map(function (p) { return page.G.leaves(p.root); }), pagesOf()],
  [[31], ["a", "c"], [["c:" + page.W.list("k")[0].id], ["t:a"]], [["Ca"], ["a"]]]);
// A board with no closed control (an empty board's placeholder, a board
// not yet loaded, or one serving every closed topic) cannot tell a topic
// past its cap from one that is gone: the pins stay.
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), reply()]; });
check("with no closed control a missing docked topic keeps its pin and its (empty) pane", [page.closedCalls, JSON.parse(store.cards).topics.map(function (r) { return r.n; }).sort(), pagesOf()], [[], ["a", "b"], [["Ca"], [""], ["a"]]]);
store = { cards: saved };
boot(store, function () { return []; });
check("an empty board keeps every pin", [page.closedCalls, JSON.parse(store.cards).topics.filter(function (r) { return r.p; }).map(function (r) { return r.n; }).sort()], [[], ["a", "b"]]);
// A failed fetch keeps the pin.
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), more(31), reply()]; }, { closed: function () { return null; } });
check("a failed fetch keeps the pin", JSON.parse(store.cards).topics.map(function (r) { return r.n; }).sort(), ["a", "b"]);
// A record from before topics joined is the cards' list.
store = { cards: JSON.stringify([{ id: "c1", kind: "k", pinned: 5, at: 1, data: { name: "Old" } }]) };
boot(store, function () { return [topic("a", 2800), reply()]; });
check("an old record restores its cards, docked", [pinsOf(), pagesOf()], [["Old"], [["Old"]]]);
// A topic the page adds outside a board update gets its pin button.
var Q = topic("q", 0, true);
page.board.insertBefore(mk("div.closed-page", {}, [Q]), page.board.children[page.board.children.length - 1]);
page.W.decorate();
check("a topic of a fetched closed page gets its pin button", !!pinBtn(Q), true);
// A unit docked while a docked topic is still being fetched keeps that
// topic's page and order.
store = { cards: JSON.stringify({ cards: [{ id: "c1", kind: "k", pinned: 2, at: 1, data: { name: "Old" } }], topics: [{ n: "b", p: 1, a: 0, b: 0 }], w: 0 }) };
boot(store, function () { return [topic("a", 2800), more(31), reply()]; }, { pending: true });
page.W.move(page.board.querySelector('section.topic[data-topic="a"]'), { pin: true });
check("a unit docked while a docked topic is fetched docks after it",
  [pinsOf(), JSON.parse(store.cards).topics.map(function (r) { return [r.n, r.p]; }).sort(), JSON.parse(store.cards).cards[0].pinned, galOf().pages.map(function (p) { return page.G.leaves(p.root); })],
  [["Old", "a"], [["a", 3], ["b", 1]], 2, [["t:b"], ["c:c1"], ["t:a"]]]);
process.stdout.write(bad.join("\n"));
`)
}

// TestBoardGallery pins the paged docking gallery's DOM side (cards.js over
// gallery.js), the whole script executed under node over the minimal DOM:
// the stored record migrates into the gallery (flat pins one per page in
// pin order, the bare cards list of old, a gallery reconciled against flat
// pins a previous build rewrote) and every save dual-writes the flat pin
// orders beside it; the pin and unpin buttons dock on a new page before +
// (shown) and undock (an emptied page goes); a preview card's transient
// state ends through its kind's pinned hook; a docked topic's body is
// wrapped in its pane's scroller and unwrapped back to stock markup; the
// rail's dots are buttons (the narrow view's ⌂, a pane count per page, +)
// and a drag docks on +, appends on a page's dot, undocks on ⌂, and a
// dragged dot reorders the pages; activate shows a node's page; and
// neither a page switch, a width crossing nor a board morph moves a
// docked unit's node.
func TestBoardGallery(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mustAsset(t, "preview.js"), "pinned: function (card) { card.preview = false; changed(card); }") {
		t.Error("preview.js no longer ends a preview's transient state through its pinned hook")
	}
	runNode(t, "board gallery", "var CARDS = "+string(src)+";\n"+cardsDOM+`
function rec() { return JSON.parse(page.store.cards); }
function leavesOf(g) { return g.pages.map(function (p) { return page.G.leaves(p.root); }); }
function dots() { return page.W.gallery().querySelectorAll(".gal-dot"); }
function dotTexts() { return dots().map(function (b) { return [b.getAttribute("data-pos"), b.textContent, b.classList.contains("active")]; }); }
function shown() { var a = page.W.gallery().querySelectorAll(".gal-page").filter(function (p) { return p.hasAttribute("data-active"); }); return a.map(function (p) { return p.getAttribute("data-page"); }); }
function pageIds() { return galOf().pages.map(function (p) { return p.id; }); }
function clickDot(pos) { var b = dots().filter(function (d) { return d.getAttribute("data-pos") === String(pos); })[0]; page.W.gallery().querySelector(".gal-dots").fire("click", { target: b }); }

// Migration: flat pins, one page each in pin order, the flat orders kept.
var store = { cards: JSON.stringify({ cards: [{ id: "c1", kind: "k", pinned: 3, at: 1, data: { name: "One" } }, { id: "c2", kind: "k", pinned: 0, at: 2, data: { name: "Two" } }], topics: [{ n: "a", p: 1, a: 0, b: 0 }, { n: "b", p: 2, a: 0, b: 0 }] }) };
boot(store, function () { return [topic("a", 300), topic("b", 200), topic("c", 100), reply()]; });
check("flat pins migrate one per page in pin order", leavesOf(galOf()), [["t:a"], ["t:b"], ["c:c1"]]);
check("the pages show them in that order", pagesOf(), [["a"], ["b"], ["One"]]);
check("the flat orders are dual-written as the gallery's", [rec().topics.map(function (r) { return [r.n, r.p]; }), rec().cards.map(function (c) { return [c.id, c.pinned]; })], [[["a", 1], ["b", 2]], [["c1", 3], ["c2", 0]]]);
// A previous build that rewrote the record: a cleared pin leaves its page,
// a new flat pin gets one of its own, a deleted item's leaf goes.
var g = galOf(); g.pages.push({ id: "gone", root: { card: "zz" } });
var r = rec(); r.gallery = g; r.topics = r.topics.filter(function (t) { return t.n !== "a"; }); r.cards[1].pinned = 9;
store = { cards: JSON.stringify(r) };
boot(store, function () { return [topic("a", 300), topic("b", 200), reply()]; });
check("reconcile: a cleared pin and a deleted item leave, a new flat pin is appended", leavesOf(galOf()), [["t:b"], ["c:c1"], ["c:c2"]]);
check("and the orders follow", [rec().topics.map(function (r) { return [r.n, r.p]; }), rec().cards.map(function (c) { return [c.id, c.pinned]; })], [[["b", 1]], [["c1", 2], ["c2", 3]]]);
// The bare cards list of old.
store = { cards: JSON.stringify([{ id: "o1", kind: "k", pinned: 2, at: 1, data: { name: "Old" } }, { id: "o2", kind: "k", pinned: 1, at: 1, data: { name: "Older" } }]) };
boot(store, function () { return [topic("a", 300), reply()]; });
check("a bare cards list migrates in pin order", [leavesOf(galOf()), rec().cards.map(function (c) { return c.pinned; })], [[["c:o2"], ["c:o1"]], [2, 1]]);

// The pin and unpin buttons.
store = {};
var A, B;
boot(store, function () { A = topic("a", 300); B = topic("b", 200); return [A, B, reply()]; }, { kinds: ["preview"] });
check("narrow, nothing docked: no rail, no gallery", [page.W.gallery().hidden, dots().length], [true, 2]);
now = 1100; var c = page.W.open("k", { name: "Ca" });
c.pinBtn.fire("click");
check("pin docks on a new page, shown", [pagesOf(), galOf().active === pageIds()[0], shown(), page.W.gallery().hidden], [[["Ca"]], true, [pageIds()[0]], false]);
check("the narrow rail: home, the page's pane count, +", dotTexts(), [["-1", "⌂", false], ["0", "1", true], ["1", "+", false]]);
check("dots are buttons", dots().map(function (b) { return [b.tagName, b.type]; }), [["BUTTON", "button"], ["BUTTON", "button"], ["BUTTON", "button"]]);
clickPin(A);
check("a topic's pin docks it on a page before +, shown", [pagesOf(), shown(), pinBtn(A).getAttribute("aria-pressed")], [[["Ca"], ["a"]], [pageIds()[1]], "true"]);
check("a docked topic's body stands in its pane's scroller", [A.children.map(function (n) { return n.className; }), paneOf(A).getAttribute("data-ref")], [["tophead", "gal-pbody"], "t:a"]);
clickDot(0);
check("a dot shows its page", [shown(), dotTexts()[1][2]], [[pageIds()[0]], true]);
clickDot(-1);
check("home shows the Main Board: every page off the view", [shown(), page.W.gallery().getAttribute("data-pos")], [[], "-1"]);
check("a page off the view stays mounted, translated, inert", page.W.gallery().querySelectorAll(".gal-page").map(function (p) { return [p.style.props["--gal-off"], p.inert, p.getAttribute("aria-hidden")]; }), [["1", true, "true"], ["2", true, "true"], ["3", true, "true"]]);
check("activate shows a docked node's page", [page.W.activate(A.querySelector(".tbar")), shown()], [true, [pageIds()[1]]]);
check("activate on the shown page switches nothing", page.W.activate(A), false);
check("narrow: a board node shows Page 0", [page.W.activate(B), shown()], [true, []]);
clickPin(A);
check("unpin undocks, its emptied page gone, its markup stock again", [pagesOf(), names(page.board), A.children.map(function (n) { return n.className; }), pinBtn(A).getAttribute("aria-pressed")], [[["Ca"]], ["TOOLBAR", "a", "b", "reply"], ["tophead"], "false"]);
c.pinBtn.fire("click");
check("the last unpin leaves no page and hides the gallery", [pagesOf(), galOf().pages, page.W.gallery().hidden, names(page.board)], [[], [], true, ["TOOLBAR", "Ca", "a", "b", "reply"]]);
// A preview's transient state ends when it docks (its kind's pinned hook).
var pv = null;
page.W.register("preview", { open: function (x) { x.title({ name: "pv" }); x.el.card = "pv"; }, pinned: function (x) { x.preview = false; } });
pv = page.W.open("preview", {}); pv.preview = true;
pv.pin(true);
check("docking a preview ends its transient state", [pv.preview, !!pv.pinned, pagesOf()], [false, true, [["pv"]]]);
pv.pin(false);

// Drag: the rail shows on a drag start, + docks, a page's dot appends,
// home undocks, a dragged dot reorders the pages.
store = {};
boot(store, function () { A = topic("a", 300); B = topic("b", 200); return [A, B, reply()]; });
now = 1200; c = page.W.open("k", { name: "Cd" });
var zero = { top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 };
function box(l, t, w, h) { return { left: l, top: t, right: l + w, bottom: t + h, width: w, height: h }; }
function geo(n) {
  if (n.classList.contains("gal-dot")) return box(10, 100 + 30 * +n.getAttribute("data-pos"), 20, 20);
  if (n === page.board) return box(100, 0, 600, 800);
  return null;
}
El.prototype.getClientRects = function () { return geo(this) ? [1] : []; };
El.prototype.getBoundingClientRect = function () { return geo(this) || zero; };
function each(type, e) { (page.doc.listeners[type] || []).forEach(function (fn) { fn(e); }); }
function dragTo(from, head, x, y) {
  from.fire("pointerdown", { target: head, isPrimary: true, button: 0, pointerId: 1, pointerType: "mouse", clientX: 300, clientY: 300 });
  each("pointermove", { pointerId: 1, pointerType: "mouse", buttons: 1, clientX: x, clientY: y, preventDefault: function () {} });
  var during = [page.W.dragging(), page.W.gallery().hidden, page.states[page.states.length - 1].rail];
  each("pointerup", { pointerId: 1, type: "pointerup" });
  return during;
}
check("a drag start shows the rail with nothing docked", dragTo(page.board, c.el.querySelector(".bc-hd"), 20, 110), [true, false, true]);
check("a drop on + docks on a new page", [pagesOf(), galOf().pages.length, page.W.dragging()], [[["Cd"]], 1, false]);
dragTo(page.board, A.querySelector(".tbar"), 20, 110);
check("a drop on a page's dot appends below what it holds", [pagesOf(), dotTexts()[1][1], page.G.isSplit(galOf().pages[0].root) && galOf().pages[0].root.axis], [[["Cd", "a"]], "2", "col"]);
check("the split renders as a column of two panes", page.W.gallery().querySelectorAll(".gal-split").map(function (s) { return s.className; }), ["gal-split col"]);
dragTo(page.W.gallery(), c.el.querySelector(".bc-hd"), 20, 70);
check("a drop on home undocks, the Main Board shown", [pagesOf(), names(page.board)[1], page.W.gallery().getAttribute("data-pos")], [[["a"]], "Cd", "-1"]);
c.pin(true);
check("two pages", pagesOf(), [["a"], ["Cd"]]);
var rail = page.W.gallery().querySelector(".gal-rail"), d0 = dots()[1];
rail.fire("pointerdown", { target: d0, button: 0, pointerId: 2, clientX: 20, clientY: 110 });
each("pointermove", { pointerId: 2, clientX: 20, clientY: 140, preventDefault: function () {} });
each("pointerup", { pointerId: 2, type: "pointerup" });
check("a page's dot dropped on another's moves its page there, shown", [pagesOf(), shown().length === 1 && shown()[0] === pageIds()[1]], [[["Cd"], ["a"]], true]);
El.prototype.getClientRects = function () { return []; };
El.prototype.getBoundingClientRect = function () { return zero; };

// No remount: a page switch, a width crossing and a board morph move no
// docked unit's node.
store = {};
var T;
boot(store, function () { T = topic("t", 300); A = topic("a", 200); return [T, A, reply()]; });
c = page.W.open("k", { name: "Cn" }); c.pin(true); clickPin(T);
var paneC = paneOf(c.el), paneT = paneOf(T), moved = [], ins = El.prototype.insertBefore;
El.prototype.insertBefore = function (n, ref) { if (n === c.el || n === T || (n.hasAttribute && (n.hasAttribute("data-page") || n.classList.contains("gal-pane")))) moved.push(label(n)); return ins.call(this, n, ref); };
clickDot(0); clickDot(1); clickDot(-1); clickDot(2); clickDot(0);
check("page switches move no node, only transforms", [moved, paneOf(c.el) === paneC, paneOf(T) === paneT], [[], true, true]);
page.width = 1800; page.W.relayout(); page.width = 1000; page.W.relayout();
check("a width crossing moves no node", [moved, page.layouts], [[], [true, false]]);
swap([T, A, reply()]);
var A2 = topic("a", 900);
swap([A2, T, reply()]);
check("board morphs move no docked node", [moved, paneOf(c.el) === paneC, paneOf(T) === paneT, page.W.docked().map(label)], [[], true, true, ["Cn", "t"]]);
var T2 = topic("t", 950);
swap([T2, A2, reply()]);
check("a docked topic updated anew lands in its own pane, wrapped", [paneOf(T2) === paneT, T2.children.map(function (n) { return n.className; }), T.parentNode], [true, ["tophead", "gal-pbody"], null]);
El.prototype.insertBefore = ins;
check("the record keeps the shown page", galOf().active, galOf().pages[0].id);
process.stdout.write(bad.join("\n"));
`)
}

// TestPinnedColumnWidth pins the board-width handle on the pinned column
// (cards.js buildCol, edgeHandle), the whole script executed under node
// over the minimal DOM when node is installed: the column fills the window
// right of the board, so it has one handle, on its left edge where it
// meets the board, and dragging it resizes the board's .dgrid cell live
// through --boardw; the width stops at BOARDW_MIN and where the column
// would drop below PINW_MIN, is saved with the pins (bw) on release, and
// comes back after a reload; a double click goes back to the stylesheet's
// width; a press of another button drags nothing; a stored w (the
// column's own width from before) is dropped.
func TestPinnedColumnWidth(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "pinned column width", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, colW = 600, cellW = 1000;
function served() { return [topic("a", 300), reply()]; }
function start() {
  boot(store, served, { wide: true });
  page.board.classList.add("dgrid");
  page.board.getBoundingClientRect = function () { return { top: 0, bottom: 0, left: 0, right: cellW, width: cellW, height: 0 }; };
  page.col.getBoundingClientRect = function () { return { top: 0, bottom: 0, left: 0, right: colW, width: colW, height: 0 }; };
}
start();
function boardw() { return page.doc.documentElement.style.props["--boardw"] || null; }
function savedW() { return store.cards ? JSON.parse(store.cards).bw : 0; }
function ev(x, button) { return { button: button === undefined ? 0 : button, clientX: x, pointerId: 1, preventDefault: function () {} }; }
function drag(e, by) { e.fire("pointerdown", ev(1000)); e.fire("pointermove", ev(1000 + by)); }
function release(e) { e.fire("pointerup", ev(1000)); }
var hs = page.col.querySelectorAll(".bp-edge");
check("the column has one handle, on its left edge", hs.map(function (e) { return e.getAttribute("data-edge"); }), ["left"]);
var e = hs[0];
drag(e, 100);
check("dragging right widens the board live", [boardw(), e.classList.contains("drag"), savedW()], ["1100px", true, 0]);
release(e);
check("the release saves the board's width with the pins", [savedW(), e.classList.contains("drag")], [1100, false]);
cellW = 1100; colW = 500;
drag(e, -60);
release(e);
check("dragging left narrows it", [boardw(), savedW()], ["1040px", 1040]);
cellW = 1040; colW = 560;
drag(e, -5000);
release(e);
check("it stops at BOARDW_MIN", [boardw(), savedW()], ["400px", 400]);
cellW = 400; colW = 1200;
drag(e, 5000);
release(e);
check("it stops where the column keeps PINW_MIN", [boardw(), savedW()], ["1320px", 1320]);
e.fire("dblclick", {});
check("a double click goes back to the stylesheet's width", [boardw(), savedW()], [null, 0]);
e.fire("pointerdown", ev(1000, 2));
e.fire("pointermove", ev(1100));
check("another button drags nothing", boardw(), null);
cellW = 1000; colW = 600;
drag(e, -160);
release(e);
start();
check("a reload brings the width back", boardw(), "840px");
store.cards = JSON.stringify({ cards: [], topics: [], w: 640 });
start();
check("a stored column width is dropped", [boardw(), JSON.parse(store.cards || "{}").w], [null, undefined]);
process.stdout.write(bad.join("\n"));
`)
}

// TestConversationSplitWidth pins the conversation/board split handle
// (cards.js splitHandle), the whole script executed under node over the
// minimal DOM when node is installed: the page's grid gets one separator,
// and dragging it sets the conversation's column live through --convow
// (with data-convow) from the grid's resolved tracks; the width stops at
// CONVW_MIN and where the board would drop below SPLITB_MIN, is saved with
// the pins (cw) on release, even with nothing else to keep, and comes back
// after a reload; a one-column grid drags nothing; a double click goes
// back to the stylesheet's split; a press of another button drags nothing.
func TestConversationSplitWidth(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "conversation split width", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {};
function served() { return [topic("a", 300), reply()]; }
function start() { boot(store, served, { split: true }); page.gridCols = "840px 1036px"; }
start();
function convow() { return page.doc.documentElement.style.props["--convow"] || null; }
function flagged() { return page.doc.documentElement.hasAttribute("data-convow"); }
function savedW() { return store.cards ? JSON.parse(store.cards).cw : 0; }
function ev(x, button) { return { button: button === undefined ? 0 : button, clientX: x, pointerId: 1, preventDefault: function () {} }; }
function drag(e, by) { e.fire("pointerdown", ev(1000)); e.fire("pointermove", ev(1000 + by)); }
function release(e) { e.fire("pointerup", ev(1000)); }
var hs = page.split.querySelectorAll(".dcv-edge");
check("the grid has one split handle, a separator", hs.map(function (e) { return e.getAttribute("role"); }), ["separator"]);
check("no split is set before a drag", [convow(), flagged(), savedW()], [null, false, 0]);
var e = hs[0];
drag(e, 100);
check("dragging right widens the conversation live", [convow(), flagged(), e.classList.contains("drag"), savedW()], ["940px", true, true, 0]);
release(e);
check("the release saves the width with the pins", [savedW(), e.classList.contains("drag")], [940, false]);
page.gridCols = "940px 936px";
drag(e, -5000);
release(e);
check("it stops at CONVW_MIN", [convow(), savedW()], ["320px", 320]);
page.gridCols = "320px 1556px";
drag(e, 5000);
release(e);
check("it stops where the board keeps SPLITB_MIN", [convow(), savedW()], ["1516px", 1516]);
page.gridCols = "1892px";
drag(e, -400);
release(e);
check("a one-column grid drags nothing", [convow(), e.classList.contains("drag")], ["1516px", false]);
e.fire("dblclick", {});
check("a double click goes back to the stylesheet's split", [convow(), flagged(), savedW()], [null, false, 0]);
page.gridCols = "840px 1036px";
e.fire("pointerdown", ev(1000, 2));
e.fire("pointermove", ev(1100));
check("another button drags nothing", convow(), null);
drag(e, -40);
release(e);
start();
check("a reload brings the width back", [convow(), flagged()], ["800px", true]);
process.stdout.write(bad.join("\n"));
`)
}

// TestColumnGrabsPush pins the two width handles together on a page with a
// split while the pinned column shows (cards.js freeze, edgeHandle,
// splitHandle), the whole script executed under node over the minimal DOM
// when node is installed: each handle sizes only the panel on its left and
// the pinned column takes the difference. The column's handle sets the
// board's own width (--boardw with data-boardw, saved as bb) and first
// holds the conversation at the width it shows; the split handle sets the
// conversation's and first holds the board's; either stops where the
// column would drop below PINW_MIN, the board at SPLITB_MIN; a double
// click resets only its own panel, holding the other; without the column
// the split handle leaves the board's width alone; a bw saved before the
// split (the whole cell) is dropped.
func TestColumnGrabsPush(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "column grabs push", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, colW = 500, A;
function served() { A = topic("a", 300); return [A, reply()]; }
function start(pin) {
  boot(store, served, { wide: true, split: true });
  page.col.getBoundingClientRect = function () { return { top: 0, bottom: 0, left: 0, right: colW, width: colW, height: 0 }; };
  if (pin) clickPin(A);
}
function prop(k) { return page.doc.documentElement.style.props[k] || null; }
function widths() { return [prop("--convow"), prop("--boardw"), page.doc.documentElement.hasAttribute("data-boardw")]; }
function saved() { var r = JSON.parse(store.cards || "{}"); return [r.cw, r.bb, r.bw]; }
function ev(x, button) { return { button: button === undefined ? 0 : button, clientX: x, pointerId: 1, preventDefault: function () {} }; }
function drag(e, by) { e.fire("pointerdown", ev(1000)); e.fire("pointermove", ev(1000 + by)); e.fire("pointerup", ev(1000)); }
start(true);
check("the column shows", page.layouts[page.layouts.length - 1], true);
var be = page.col.querySelector(".bp-edge"), ce = page.split.querySelector(".dcv-edge");
page.gridCols = "600px 840px";
drag(be, 100);
check("the column's handle sizes the board and holds the conversation", [widths(), saved()], [["600px", "940px", true], [600, 940, undefined]]);
page.gridCols = "600px 940px"; colW = 400;
drag(ce, 50);
check("the split handle sizes the conversation and keeps the board", [widths(), saved()], [["650px", "940px", true], [650, 940, undefined]]);
page.gridCols = "650px 940px"; colW = 350;
drag(ce, 5000);
check("the split handle stops where the column keeps PINW_MIN", widths(), ["720px", "940px", true]);
page.gridCols = "720px 940px"; colW = 280;
drag(be, 5000);
check("the column's handle stops there too", widths(), ["720px", "940px", true]);
drag(be, -5000);
check("the board stops at SPLITB_MIN", widths(), ["720px", "360px", true]);
ce.fire("dblclick", {});
check("a double click on the split handle resets the conversation alone", [widths(), saved()], [[null, "360px", true], [0, 360, undefined]]);
store = {};
start(true);
page.gridCols = "500px 700px";
be = page.col.querySelector(".bp-edge"); ce = page.split.querySelector(".dcv-edge");
be.fire("dblclick", {});
check("a double click on the column's handle resets the board and holds the conversation", widths(), ["500px", null, false]);
ce.fire("dblclick", {});
check("a double click on the split handle holds the board", widths(), [null, "700px", true]);
store = {};
start(false);
page.gridCols = "500px 700px";
drag(page.split.querySelector(".dcv-edge"), 40);
check("without the column the split handle leaves the board's width alone", widths(), ["540px", null, false]);
store = { cards: JSON.stringify({ cards: [], topics: [], bw: 1200, cw: 0 }) };
start(false);
check("a bw from before the split is dropped", [widths(), saved()], [[null, null, false], [undefined, undefined, undefined]]);
process.stdout.write(bad.join("\n"));
`)
}

func TestBoardModeWidthsSurviveOtherModeSaves(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := json.Marshal(string(data))
	runNode(t, "board mode widths", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, A;
function served() { A = topic("a", 300); return [A, reply()]; }
function width() { return page.doc.documentElement.style.props["--boardw"] || null; }
function saved() { var r = JSON.parse(store.cards || "{}"); return [r.bw, r.bb]; }
function drag(edge, by) {
  function ev(x) { return { button: 0, clientX: x, pointerId: 1, preventDefault: function () {} }; }
  edge.fire("pointerdown", ev(1000)); edge.fire("pointermove", ev(1000 + by)); edge.fire("pointerup", ev(1000 + by));
}
boot(store, served, {wide: true});
page.board.classList.add("dgrid");
page.board.getBoundingClientRect = function () { return {width: 1000}; };
page.col.getBoundingClientRect = function () { return {width: 600}; };
drag(page.col.querySelector(".bp-edge"), -160);
check("board-only save marks its width as post-migration", saved(), [840, 0]);
boot(store, served, {wide: true, split: true});
check("switching to live keeps the other mode's width without applying it", [width(), saved()], [null, [840, 0]]);
clickPin(A);
check("a pin in live mode does not erase the board-only width", saved(), [840, 0]);
page.gridCols = "600px 700px";
page.col.getBoundingClientRect = function () { return {width: 500}; };
drag(page.col.querySelector(".bp-edge"), 100);
check("live width saves independently", saved(), [840, 800]);
boot(store, served, {wide: true});
check("board reload restores its own width and retains live width", [width(), saved()], ["840px", [840, 800]]);
clickPin(A);
check("unpin in board mode retains both widths", saved(), [840, 800]);
boot(store, served, {wide: true, split: true});
check("live reload restores the live width", width(), "800px");
page.col.querySelector(".bp-edge").fire("dblclick", {});
page.split.querySelector(".dcv-edge").fire("dblclick", {});
// Resetting the split freezes the board, so reset it again after the split.
page.col.querySelector(".bp-edge").fire("dblclick", {});
check("clearing live widths cannot remove a record holding the other mode's width", saved(), [840, 0]);
boot(store, served, {wide: true});
check("board width survives the live reset", width(), "840px");
store = {cards: JSON.stringify({cards: [], topics: [], bw: 1200})};
boot(store, served, {split: true});
check("legacy pre-split width still migrates once", width(), null);
process.stdout.write(bad.join("\n"));
`)
}

// TestBoardPinnedClosedPagesCount pins the closed pages' show-more index
// under pins (dashboard.js morphBoard with cards.js, executed under node
// over the minimal DOM when node is installed): a topic pinned out of a
// fetched closed page leaves a slot there, and a pinned closed topic that a
// newly closed one pushes past the cap stays in the pin list with a slot
// at the pages' head, so the next page starts where the server's index
// does, neither skipping a topic nor showing one twice.
func TestBoardPinnedClosedPagesCount(t *testing.T) {
	cards, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	var src string
	for _, name := range []string{"openKey", "restoreOpenOne", "restoreOpen", "forgetOpen", "topicKey", "topicSeen", "morphBoard", "frameKeep", "frameKept", "frameChain", "frameGraftable", "frameGrafts", "settleMoves", "frameGraft"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	lit, err := json.Marshal(string(cards))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "pinned closed pages count", "var CARDS = "+string(lit)+";\n"+cardsDOM+`
var window = {}, doc = null, openState = {}, boardSeen = {};
function q(sel, el) { try { return el.querySelectorAll(sel); } catch (e) { return []; } }
function frag(nodes) { return { childNodes: nodes }; }
function boardNodes(input) { return frag(input).childNodes; }
function isPeer() { return false; }
function isPanel() { return false; }
`+src+`
function served(closed) { return [topic("A", 500)].concat(closed.map(function (n) { return topic(n, 0, true); })).concat([more(1, 5), reply()]); }
function start() {
  var first;
  boot({}, function () { first = served(["X"]); return first; });
  window.WsCards = page.W; doc = page.doc;
  first.forEach(function (n) { var k = topicKey(n); if (k !== null) boardSeen["t:" + k] = n.outerHTML; });
  // "show more" fetched the page from index 1: Y and Z, then more from 3.
  var btn = page.board.querySelector("button.closed-more");
  var p = mk("div.closed-page", {}, [topic("Y", 0, true), topic("Z", 0, true), more(3, 3)]);
  page.board.insertBefore(p, btn);
  page.board.removeChild(btn);
  return p;
}
function tail() { var b = page.board.querySelectorAll("button.closed-more"); return b.length ? b[b.length - 1].getAttribute("data-closed-next") : null; }

var p = start();
morphBoard(page.board, served(["X"]));
check("with nothing pinned the pages count on from the board's control", tail(), "3");
check("landed morph maintains remainder and total", [page.board.querySelector("button.closed-more").getAttribute("data-closed-more"), toolbarCounts()], ["3", "open 1 · closed 6"]);
clickPin(p.querySelector('section.topic[data-topic="Y"]'));
check("a topic pinned out of a page leaves a slot there", [pinsOf(), p.querySelectorAll(".closed-slot").map(function (s) { return s.getAttribute("data-slot"); })], [["Y/c"], ["Y"]]);
morphBoard(page.board, served(["X"]));
check("the slot counts as the topic did", tail(), "3");

p = start();
clickPin(page.board.querySelector('section.topic[data-topic="X"]'));
morphBoard(page.board, served(["W"]));
check("a pinned closed topic pushed past the cap stays pinned", pinsOf(), ["X/c"]);
check("and a slot at the pages' head counts it", [p.firstChild.getAttribute("data-slot"), tail()], ["X", "4"]);
morphBoard(page.board, served(["W"]));
check("a later swap counts it once", [p.querySelectorAll(".closed-slot").length, tail()], [1, "4"]);
check("pinned slots consume remainder without counting as shown topics", [page.board.querySelector("button.closed-more").getAttribute("data-closed-more"), toolbarCounts()], ["2", "open 1 · closed 6"]);
process.stdout.write(bad.join("\n"));
`)
}

// TestFilesToolbarSheet keeps the page-owned explorer separate from the tab
// through the shared sheet lifecycle, board swaps and card/preview reveal.
func TestFilesToolbarSheet(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "Files toolbar sheet", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
El.prototype.focus = function () { this.ownerDocument.activeElement = this; };
var notices = 0, A, owner = { el: null, shown: function () { notices++; }, hidden: function () { notices++; } };
boot({}, function () { A = topic("a", 300); return [A]; }, { toolbar: { files: owner } });
var grid = page.doc.body.appendChild(mk("div", { "data-tab": "board", "data-ctab": "convo" }));
var slot = grid.appendChild(mk("span.fsum")), root = slot.appendChild(mk("span.froot"));
var tab = grid.appendChild(mk("section.fpane", { id: "files" }));
var pane = owner.el = page.doc.body.appendChild(mk("section.fpane", { id: "toolbar-files" }, [mk("div.fhd"), mk("input.fq"), mk("div.ftree")]));
pane.hidden = true;
page.doc.querySelector = function (s) { return s === "#chead .fsum" ? slot : null; };
var filter = pane.querySelector(".fq"), tree = pane.querySelector(".ftree"), bar = page.W.toolbar(), btn = bar.querySelector(".board-files");
filter.value = "keep query"; tree.scrollTop = 73;
function toggle() { btn.fire("click", {}); }
function esc(consumed) {
  var e = { key: "Escape", defaultPrevented: !!consumed, preventDefault: function () { this.defaultPrevented = true; } };
  page.doc.listeners.keydown.forEach(function (f) { f(e); });
  return e.defaultPrevented;
}
function state() { return [pane.hasAttribute("data-max"), btn.getAttribute("aria-expanded"), !!bar.hasAttribute("data-files")]; }
toggle();
check("Files toggles its owned explorer into the shared sheet", [state(), pane.hidden], [[true, "true", true], false]);
check("opening focuses search without borrowing the tab or root label", [page.doc.activeElement === filter, root.parentNode === slot, tab.parentNode === grid, pane.parentNode === page.doc.body], [true, true, true, true]);
check("no tab switch", [grid.getAttribute("data-tab"), grid.getAttribute("data-ctab")], ["board", "convo"]);
swap([A, topic("b", 400)]);
check("a board swap does not mistake Files for a removed topic", state(), [true, "true", true]);
esc(true);
check("consumed Escape does not close the sheet", state(), [true, "true", true]);
toggle();
check("second click closes", state(), [false, "false", false]);
check("dismissal hides its owner and returns focus without rebuilding state", [root.parentNode === slot, pane.hidden, page.doc.activeElement === btn, pane.querySelector(".fq") === filter, filter.value, tree.scrollTop, notices], [true, true, true, true, "keep query", 73, 2]);
toggle(); esc(false);
check("unconsumed Escape dismisses and returns focus", [state(), page.doc.activeElement === btn], [[false, "false", false], true]);
page.board.fire("click", { target: A.querySelector(".d-max"), preventDefault: function () {} });
toggle();
check("Files replaces another maximized sheet", [pane.hasAttribute("data-max"), A.hasAttribute("data-max")], [true, false]);
page.board.fire("click", { target: A.querySelector(".d-max"), preventDefault: function () {} });
check("another sheet replaces Files without moving its label", [pane.hasAttribute("data-max"), A.hasAttribute("data-max"), root.parentNode === slot, pane.hidden], [false, true, true, true]);
toggle();
var c = page.W.open("k", { name: "preview" }); c.show();
check("preview reveal dismisses Files instead of hiding the preview behind it", state(), [false, "false", false]);
grid.setAttribute("data-ctab", "files"); toggle(); esc(false);
check("existing Files-tab access remains selected on dismissal", [grid.getAttribute("data-ctab"), tab.parentNode === grid, root.parentNode === slot], ["files", true, true]);
process.stdout.write(bad.join("\n"));
`)
}

// TestBoardSheet draws a topic and a topic's panel large through the card
// framework's maximize sheet (cards.js), executed under node when it is
// installed: a topic's maximize button marks the topic large over the
// shared scrim; a board update that brings the topic as a new section, or
// grafts the new bar into the old one, draws it large there; one thing is
// large at a time, a card, a topic, or a panel; the back button, Escape,
// and the scrim restore it; a panel's buttons never fold its details, a
// closed panel opens as it grows, its entry keeps its place meanwhile, and a
// large panel's head does not fold it; a large node the board stops serving
// takes the scrim with it.
func TestBoardSheet(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/cards.js")
	if err != nil {
		t.Fatal(err)
	}
	src, err := json.Marshal(string(data))
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "board sheet", "var CARDS = "+string(src)+";\n"+cardsDOM+`
var store = {}, A, B;
function panel(key) {
  var sum = mk("summary", {}, [mk("span.pmeta"), mk("button.ibtn.d-max"), mk("button.ibtn.d-min")]);
  return mk("article.wb-entry", {}, [mk("details.entry-panel", { "data-key": key }, [sum, mk("div.doc-body")])]);
}
function served() { A = topic("a", 300); B = topic("b", 200); B.appendChild(panel("entry:p1")); return [A, B, reply()]; }
boot(store, served);
function btn(t, cls) { return t.querySelector(".tbar ." + cls); }
function pbtn(t, cls) { return t.querySelector("details.entry-panel summary ." + cls); }
function ev(target) { return { target: target, prevented: false, preventDefault: function () { this.prevented = true; } }; }
function click(b) { var e = ev(b); (b.closest("aside") || page.board).fire("click", e); return e; }
function scrims() { return page.doc.body.children.filter(function (n) { return n.classList.contains("bc-scrim") && !n.hidden; }).length; }
function esc() { var e = { key: "Escape", defaultPrevented: false, preventDefault: function () { this.defaultPrevented = true; } }; page.doc.listeners.keydown.forEach(function (fn) { fn(e); }); }
function large() { return page.board.querySelectorAll("[data-max]").map(function (n) { return n.getAttribute("data-topic") || n.getAttribute("data-key") || "card"; }); }

click(btn(A, "d-max"));
check("a topic's maximize button draws it large over the scrim", [large(), scrims()], [["a"], 1]);
var A2 = topic("a", 400);
swap([A2, B, reply()]);
check("a board update that brings the topic anew draws the new section large", [large(), scrims(), A.hasAttribute("data-max")], [["a"], 1, false]);
var A3 = topic("a", 500), oldHead = A2.querySelector(".tophead");
swap([A3, B, reply()], function (p) {
  // The frame graft keeps the old section, taking the new one's attributes and head.
  A2.removeAttribute("data-max"); A2.removeChild(oldHead); A2.insertBefore(A3.querySelector(".tophead"), A2.firstChild);
  p.board[p.board.indexOf(A3)] = A2;
});
check("a grafted update keeps the old section large, with its buttons", [large(), !!btn(A2, "d-min"), scrims()], [["a"], true, 1]);
click(btn(A2, "d-min"));
check("the back button restores it and hides the scrim", [large(), scrims()], [[], 0]);

var d = B.querySelector("details.entry-panel"), e = click(pbtn(B, "d-max"));
check("a panel's maximize button opens and draws it large without folding it", [large(), d.open, e.prevented, scrims()], [["entry:p1"], true, true, 1]);
check("its entry keeps its place", d.parentNode.style.height, "0px");
check("its head does not fold it while large", click(d.querySelector("summary")).prevented, true);
click(btn(A2, "d-max"));
check("maximizing a topic restores the panel: one is large at a time", [large(), d.parentNode.style.height, scrims()], [["a"], "", 1]);
var c = page.W.open("k", { name: "K" });
c.maxBtn.fire("click");
check("maximizing a card restores the topic", [large(), scrims()], [["card"], 1]);
click(pbtn(B, "d-max"));
check("maximizing a panel restores the card", [large(), c.el.hasAttribute("data-max")], [["entry:p1"], false]);
esc();
check("Escape restores the panel", [large(), scrims()], [[], 0]);
check("a closed panel's head click folds it as before", click(d.querySelector("summary")).prevented, false);

click(btn(A2, "d-max"));
page.doc.body.children.filter(function (n) { return n.classList.contains("bc-scrim"); })[0].fire("click", ev(null));
check("the scrim restores the topic", [large(), scrims()], [[], 0]);
click(btn(A2, "d-max"));
swap([B, reply()]);
check("a large topic the board no longer serves takes the scrim with it", [large(), scrims()], [[], 0]);
process.stdout.write(bad.join("\n"));
`)
}

// TestFullscreenPanelWidth exercises the shared transient width state through
// actual cards, topics, entry panels, Files and live renderer replacements.
func TestFullscreenPanelWidth(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "fullscreen panel width", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var A, panel, owner = { el: null, shown: function () {}, hidden: function () {} }, store = {};
function served() {
  A = topic("A", 100);
  panel = mk("details.entry-panel", { "data-key": "entry:p1" }, [mk("summary", {}, [mk("button.d-max"), mk("button.d-min")]), mk("div", { "data-kind": "diff" })]);
  A.appendChild(mk("article.wb-entry", { "data-kind": "panel" }, [panel]));
  return [A];
}
boot(store, served, { wide: true, toolbar: { files: owner } });
El.prototype.focus = function () { this.ownerDocument.activeElement = this; };
var sized = 0;
["preview", "terminal", "web", "approval", "system", "other"].forEach(function (kind) {
  page.W.register(kind, { open: function () {}, sized: function () { sized++; } });
});
function resize(w) { page.win.innerWidth = w; (page.win.listeners.resize || []).forEach(function (fn) { fn(); }); }
function click(n) { page.board.fire("click", { target: n, preventDefault: function () {} }); }
function inset(n) { return n.style.props["--sheet-inset"]; }
function widthBtn(n) { return n.querySelector(".sheet-width"); }
function toggle(n) { widthBtn(n).fire("click", { preventDefault: function () {} }); }
function closeCard(c) { c.minBtn.fire("click"); }
function card(kind, semantic) { var c = page.W.open(kind, {}); if (semantic) c.el.setAttribute("data-preview-kind", semantic); return c; }
resize(2400);
// The preview renderer exposes text responses as code, and prose as markdown.
[["markdown", 1280], ["code", 1440], ["diff", 1760], ["frame", 1600], ["image", 1600], ["video", 1600], ["unknown", 1920]].forEach(function (item) {
  var c = card("preview", item[0]); c.body.appendChild(mk("div", { "data-kind": "diff" })); c.maxBtn.fire("click");
  check(item[0] + " uses owning renderer, not nested content", inset(c.el), ((2400 - item[1]) / 2) + "px");
  check("effective extension is offered", widthBtn(c.el).hidden, false);
  toggle(c.el);
  check("extension exceeds 1920 cap and exposes next action", [inset(c.el), widthBtn(c.el).getAttribute("aria-pressed"), widthBtn(c.el).title], ["16px", "true", "js.card.width.normal"]);
  toggle(c.el);
  check("same toggle restores width without leaving sheet", [inset(c.el), c.el.hasAttribute("data-max"), widthBtn(c.el).getAttribute("aria-pressed"), widthBtn(c.el).title], [((2400 - item[1]) / 2) + "px", true, "false", "js.card.width.extend"]);
  toggle(c.el); closeCard(c);
  check("minimize discards width and toggle", [inset(c.el), widthBtn(c.el)], [undefined, null]);
  c.maxBtn.fire("click"); check("reopen is normal", widthBtn(c.el).getAttribute("aria-pressed"), "false"); closeCard(c);
});
[["web", 1600], ["approval", 1024], ["system", 1024], ["other", 1920]].forEach(function (item) {
  var c = card(item[0]); c.maxBtn.fire("click"); check(item[0], inset(c.el), ((2400 - item[1]) / 2) + "px"); closeCard(c);
});
var term = card("terminal"), reads = 0, span = 812;
page.board.getBoundingClientRect = function () { reads++; return { width: span }; };
term.maxBtn.fire("click"); var initialReads = reads;
check("Terminal captures board column", inset(term.el), "794px");
span = 1111; resize(1000); resize(2400);
check("resize never remeasures owning column", [inset(term.el), reads], ["794px", initialReads]);
closeCard(term); term.pin(true);
page.col.querySelector(".gal-viewport").getBoundingClientRect = function () { return { width: 620 }; };
term.maxBtn.fire("click"); check("docked Terminal captures the gallery's viewport", inset(term.el), "890px"); closeCard(term);
term.pin(false); span = 0; term.maxBtn.fire("click"); check("Terminal fallback is 960", inset(term.el), "720px"); closeCard(term);
span = 2600; term.maxBtn.fire("click"); check("column snapshot also has 1920 cap", inset(term.el), "240px"); closeCard(term);
var convo = page.doc.body.appendChild(mk("div", { id: "convo" }));
convo.getBoundingClientRect = function () { return { left: 16, right: 1116, width: 1100 }; };
var bcol = page.doc.body.appendChild(mk("div", { id: "bcol" })), bcolRect = { left: 1130, right: 2384, width: 1254 };
bcol.getBoundingClientRect = function () { return bcolRect; };
function sides(n) { return [n.style.props["--sheet-left"], n.style.props["--sheet-right"]]; }
owner.el = page.doc.body.appendChild(mk("section.fpane", {}, [mk("input.fq")]));
var files = page.W.toolbar().querySelector(".board-files"); files.fire("click", {});
check("Files spans the main (board) column", [inset(owner.el), sides(owner.el)], ["573px", ["1130px", "16px"]]);
var filesWidthBtn = page.W.toolbar().querySelector(".sheet-width");
filesWidthBtn.fire("click", { preventDefault: function () {} }); resize(1000); resize(2400);
check("Files extend ignores the column", [inset(owner.el), sides(owner.el)], ["16px", [undefined, undefined]]);
filesWidthBtn.fire("click", { preventDefault: function () {} });
bcolRect = { left: 1200, right: 2384, width: 1184 }; resize(2400);
check("Files follows its column across resizes", [inset(owner.el), sides(owner.el)], ["608px", ["1200px", "16px"]]);
files.fire("click", {}); bcolRect = { left: 0, right: 0, width: 0 }; files.fire("click", {});
check("Files falls back to the conversation where the board is hidden", [inset(owner.el), sides(owner.el)], ["650px", ["16px", "1284px"]]);
files.fire("click", {}); convo.getBoundingClientRect = function () { return { width: 0 }; }; files.fire("click", {});
check("Files fallback is 1280 and resets on close", [inset(owner.el), sides(owner.el), page.W.toolbar().querySelector(".sheet-width").getAttribute("aria-pressed")], ["560px", [undefined, undefined], "false"]);
files.fire("click", {});
click(A.querySelector(".d-max")); check("topic outranks nested diff", inset(A), "480px"); toggle(A);
var A2 = topic("A", 200); swap([A2]);
check("live replacement keeps width state on the new owner", [inset(A), inset(A2), widthBtn(A2).getAttribute("aria-pressed")], [undefined, "16px", "true"]);
click(A2.querySelector(".d-min"));
swap(served()); click(panel.querySelector(".d-max"));
check("panel category outranks nested diff", inset(panel), "480px");
var child = panel.children[panel.children.length - 1]; toggle(panel); resize(640);
check("mobile hides ineffective extension", [widthBtn(panel).hidden, panel.hasAttribute("data-max")], [true, true]);
resize(1280); check("wide but clamped hides ineffective extension", widthBtn(panel).hidden, true);
resize(1900); check("resize keeps node/disclosure while exposing extension", [widthBtn(panel).hidden, panel.children[panel.children.length - 1] === child, panel.open], [false, true, true]);
click(panel.querySelector(".d-min"));
var p = card("preview", "unknown"); p.maxBtn.fire("click"); p.el.setAttribute("data-preview-kind", "markdown"); p.paint();
check("late preview renderer identity sizes an already open sheet", inset(p.el), "310px"); closeCard(p);
resize(2400);
var remembered = page.W.open("k", { name: "reload" }); remembered.maxBtn.fire("click"); toggle(remembered.el);
check("storage excludes sheet geometry", /normalWidth|extended|sheet-inset/.test(store.cards), false);
boot(store, served); var restored = page.W.list("k")[0]; restored.maxBtn.fire("click");
check("reload does not remember extension", widthBtn(restored.el).getAttribute("aria-pressed"), "false");
process.stdout.write(bad.join("\n"));
`)
}
