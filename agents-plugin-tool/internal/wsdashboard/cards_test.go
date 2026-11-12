package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// cardScripts are the page scripts that draw vendored Lucide icons.
var cardScripts = []string{"assets/cards.js", "assets/preview.js", "assets/files.js", "assets/terminal.js", "assets/dashboard.js"}

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
	for _, m := range regexp.MustCompile(`(?:licon\(|luIcon\(|ibtn\("[a-z -]*", |iconBtn\(|icon: )"([a-z0-9-]+)"`).FindAllStringSubmatch(js, -1) {
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
	settle := regexp.MustCompile(`(?s)\n  function settle\(el, want, fixed, skip\) \{.*?\n  \}\n`).FindString(js)
	if order == "" || settle == "" {
		t.Fatal("cards.js holds no order / settle")
	}
	board, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(board), "var plan = window.WsCards.weave(want), woven = plan.board;") || !strings.Contains(string(board), "window.WsCards.settle(el, plan);") {
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
// (descendant combinators over tag, .class, [attr], [attr="v"], and
// comma lists), a document with a body, and boot, which loads cards.js
// into a fresh page over a storage map, as a reload does.
const cardsDOM = `
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
  insertBefore: function (n, ref) {
    if (n.parentNode) n.parentNode.removeChild(n);
    this.kids.splice(ref ? this.kids.indexOf(ref) : this.kids.length, 0, n); n.parentNode = this; return n;
  },
  appendChild: function (n) { return this.insertBefore(n, null); },
  removeChild: function (n) { this.kids.splice(this.kids.indexOf(n), 1); n.parentNode = null; return n; },
  set innerHTML(v) { this.kids.forEach(function (k) { k.parentNode = null; }); this.kids = []; this.html = v; },
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
  for (var n = el.parentNode; rest.length && n && n.nodeType === 1; n = n.parentNode) if (one(n, rest[rest.length - 1])) rest = rest.slice(0, -1);
  return !rest.length;
}
function Doc() {
  var d = this;
  this.listeners = {};
  this.documentElement = new El("html", d);
  this.body = this.documentElement.appendChild(new El("body", d));
}
Doc.prototype = {
  createElement: function (t) { return new El(t, this); },
  addEventListener: function (t, fn) { (this.listeners[t] = this.listeners[t] || []).push(fn); }
};
var page = null;
function wsT(k) { return k; }
// mk builds an element: tag.class.class with attributes and children.
function mk(spec, attrs, kids) {
  var p = spec.split("."), e = new El(p[0], page.doc);
  if (p.length > 1) e.className = p.slice(1).join(" ");
  Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
  (kids || []).forEach(function (k) { e.appendChild(k); });
  return e;
}
// topic builds a topic section as the board serves it: open with its bar
// time, or closed; fresh adds the update mark.
function topic(name, at, closed, fresh) {
  var bar = mk("div.tbar", {}, [mk("span.tlead", {}, [mk("span.tname")])].concat(closed ? [] : [mk("time.tago", { "data-at": at })]).concat(fresh ? [mk("span.chip.fresh")] : []).concat([mk("button.btn.mini.tnote-toggle")]));
  var t = mk("section.topic" + (closed ? ".closed" : ""), { "data-topic": name }, [bar]);
  t.label = name + (closed ? "/c" : "") + (fresh ? "*" : "");
  return t;
}
function more(next) { var b = mk("button.btn.closed-more", { "data-closed-next": next }); b.label = "more" + next; return b; }
function reply() { var r = mk("div.reply"); r.label = "reply"; return r; }
function label(n) { return n.label || (n.hasAttribute("data-card") ? n.card || "card" : n.hasAttribute("data-pins") ? "PINS" : n.tagName); }
function names(el) { return el.children.map(label); }
// boot loads cards.js into a fresh page whose board region holds served,
// over store (a reload keeps it), with the pinned column shown when wide.
function boot(store, served, o) {
  o = o || {};
  page = { doc: null, store: store, layouts: [], closedCalls: [], adopted: [] };
  page.doc = new Doc();
  var dbody = page.doc.body.appendChild(new El("div", page.doc));
  page.board = dbody.appendChild(new El("div", page.doc));
  page.board.id = "r-board";
  served().forEach(function (n) { page.board.appendChild(n); });
  page.col = dbody.appendChild(new El("aside", page.doc));
  var win = { getSelection: function () { return null; }, innerWidth: 2000 };
  new Function("document", "window", "wsT", "getComputedStyle", "requestAnimationFrame", CARDS)(page.doc, win, wsT, function () { return { overflowY: "visible" }; }, function () {});
  page.W = win.WsCards;
  page.W.register("k", { open: function (c) { c.title({ name: c.data.name }); c.el.card = c.data.name; } });
  page.W.mount(page.board, {
    key: "cards", load: function (k) { return store[k] || null; }, save: function (k, v) { if (v === null) delete store[k]; else store[k] = v; },
    column: page.col, shows: function (n) { return !!o.wide && n > 0; },
    layout: function (on) { page.layouts.push(on); },
    closed: function (from, done) { page.closedCalls.push(from); done(o.closed ? o.closed(from) : null); },
    adopted: function (n) { page.adopted.push(n.getAttribute("data-topic")); }
  });
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
function clickPin(t) { var b = pinBtn(t); (t.closest("aside") || page.board).fire("click", { target: b }); }
function pinsOf() { return names(page.W.pinList()); }
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
// closed route page by page, and one the route ends without loses its pin.
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
check("every topic bar has a pin button before its note pen, unpressed",
  [A, B, C].map(function (t) { var b = pinBtn(t); return [!!b, b && b.nextSibling && b.nextSibling.classList.contains("tnote-toggle"), b && b.getAttribute("aria-pressed")]; }),
  [[true, true, "false"], [true, true, "false"], [true, true, "false"]]);
check("an empty pin list heads the board", names(page.board), ["PINS", "a", "b", "c", "x/c", "more31", "reply"]);

// A pinned topic stands among pinned cards in pin order.
now = 1100; var ca = page.W.open("k", { name: "Ca" }); ca.pin(true);
now = 1200; clickPin(B);
now = 1300; var cb = page.W.open("k", { name: "Cb" }); cb.pin(true);
check("a pinned topic stands among the pinned cards, in pin order", pinsOf(), ["Ca", "b", "Cb"]);
check("its pin button is pressed and the section marked", [pinBtn(B).getAttribute("aria-pressed"), B.hasAttribute("data-pinned")], ["true", true]);
check("the board keeps the rest in its order", names(page.board), ["PINS", "a", "c", "x/c", "more31", "reply"]);
cb.pin(false);
check("an unpinned card enters the flow at its open time", names(page.board), ["PINS", "Cb", "a", "c", "x/c", "more31", "reply"]);
cb.close();

// An unpinned hand move is undone by a later bump above it.
now = 2000;
page.W.move(C, { pin: false, after: null, before: A });
check("a topic moved by hand stands where it was dropped", names(page.board), ["PINS", "c", "a", "x/c", "more31", "reply"]);
var A2 = topic("a", 2500);
swap([A2, C, topic("x", 0, true), more(31), reply()]);
check("a topic updated later rises above it", names(page.board), ["PINS", "a", "c", "x/c", "more31", "reply"]);
var stash = JSON.parse(store.cards);
check("the move is kept with its bar time", stash.topics.filter(function (r) { return r.n === "c"; }).map(function (r) { return [r.a > 2000 - 1, r.b]; }), [[true, 100]]);

// An updated pinned topic keeps its index and gains the update mark.
now = 2600; clickPin(A2);
check("pins before the update", pinsOf(), ["Ca", "b", "a"]);
var B2 = topic("b", 2700, false, true);
swap([B2, A2, C, topic("x", 0, true), more(31), reply()]);
check("an updated pinned topic keeps its index, with the board's update mark", pinsOf(), ["Ca", "b*", "a"]);
check("its new bar carries the pressed pin", pinBtn(B2).getAttribute("aria-pressed"), "true");

// A pinned topic updated through the frame graft keeps its pin pressed.
var oldBar = A2.querySelector(".tbar"), A3 = topic("a", 2800, false, true);
swap([B2, A3, C, topic("x", 0, true), more(31), reply()], function (p) {
  // The morph keeps the old section and gives it the new bar's content.
  A2.removeChild(oldBar); A2.insertBefore(A3.querySelector(".tbar"), A2.firstChild);
  p.pins[p.pins.indexOf(A3)] = A2;
});
check("a pinned topic updated through the graft keeps its section and index", [pinsOf(), page.W.pinList().children[2] === A2], [["Ca", "b*", "a"], true]);
check("and its pin button pressed", pinBtn(A2) && pinBtn(A2).getAttribute("aria-pressed"), "true");

// A closed pinned topic stays pinned; past the closed cap it stays too.
var B3 = topic("b", 0, true);
swap([A2, C, B3, topic("x", 0, true), more(31), reply()]);
check("a pinned topic that closes stays in the pin list, closed", pinsOf(), ["Ca", "b/c", "a"]);
swap([A2, C, topic("x", 0, true), more(31), reply()]);
check("one the board no longer serves keeps its last node there", [pinsOf(), page.W.pinList().children[1] === B3], [["Ca", "b/c", "a"], true]);

// The pinned column: where the page shows one, the pin list moves there.
var saved = store.cards;
boot(store, function () { return [topic("a", 2800), topic("c", 100), topic("x", 0, true), more(31), reply()]; }, {
  wide: true,
  closed: function (from) {
    if (from === 31) { var p = mk("div.closed-page", {}, [topic("y", 0, true), more(61)]); return p; }
    return mk("div.closed-page", {}, [topic("z", 0, true), topic("b", 0, true)]);
  }
});
check("after a reload the board's missing pinned topic is fetched page by page", [page.closedCalls, page.adopted], [[31, 61], ["b"]]);
check("pins and their order survive the reload, in the pinned column", [pinsOf(), page.W.pinList().parentNode === page.col, page.layouts], [["Ca", "b/c", "a"], true, [true]]);
check("the board column holds the rest, no pin list, the hand-moved topic where it stood",
  names(page.board), ["c", "x/c", "more31", "reply"]);
check("the pinned topic in the column has its pin pressed", pinBtn(page.W.pinList().children[2]).getAttribute("aria-pressed"), "true");
// Unpinning the last pins closes the column; an open topic goes back by
// recency, a closed one to the head of the closed section.
page.W.list("k")[0].close();
clickPin(page.W.pinList().children[0]);
clickPin(page.W.pinList().children[0]);
check("the last unpin hides the column and puts the pin list atop the board", [page.layouts, names(page.board)], [[true, false], ["PINS", "a", "c", "b/c", "x/c", "more31", "reply"]]);

// A pin whose topic is gone is dropped.
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), topic("c", 100), more(31), reply()]; }, { closed: function () { return mk("div.closed-page", {}, [topic("q", 0, true)]); } });
check("a pinned topic the closed route ends without loses its pin", [page.closedCalls, JSON.parse(store.cards).topics.map(function (r) { return r.n; }).sort()], [[31], ["a", "c"]]);
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), reply()]; });
check("with no closed control a missing pinned topic is gone at once", [page.closedCalls, JSON.parse(store.cards).topics.map(function (r) { return r.n; })], [[], ["a"]]);
// A failed fetch keeps the pin.
store = { cards: saved };
boot(store, function () { return [topic("a", 2800), more(31), reply()]; }, { closed: function () { return null; } });
check("a failed fetch keeps the pin", JSON.parse(store.cards).topics.map(function (r) { return r.n; }).sort(), ["a", "b"]);
// A record from before topics joined is the cards' list.
store = { cards: JSON.stringify([{ id: "c1", kind: "k", pinned: 5, at: 1, data: { name: "Old" } }]) };
boot(store, function () { return [topic("a", 2800), reply()]; });
check("an old record restores its cards", pinsOf(), ["Old"]);
process.stdout.write(bad.join("\n"));
`)
}
