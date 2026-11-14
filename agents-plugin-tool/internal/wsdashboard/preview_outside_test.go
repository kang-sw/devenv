package wsdashboard

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"
)

// outsideDOM is a minimal document for running page script under node:
// elements that keep their attributes, children, and listeners, and a root
// whose attributes the test sets.
const outsideDOM = `
var bad = [];
function check(name, got, want) {
  var g = JSON.stringify(got), w = JSON.stringify(want);
  if (g !== w) bad.push(name + ": got " + g + ", want " + w);
}
function El(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.attrs = {}; this.on = {}; this.style = {}; this.className = ""; this.offsetWidth = 10; this.offsetHeight = 10; }
El.prototype.appendChild = function (c) { this.children.push(c); c.parentNode = this; return c; };
El.prototype.removeChild = function (c) { this.children.splice(this.children.indexOf(c), 1); };
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.hasAttribute = function (k) { return k in this.attrs; };
El.prototype.addEventListener = function (k, f) { this.on[k] = f; };
El.prototype.focus = function () {};
El.prototype.closest = function (sel) { return sel === "a[data-wl]" && this.tagName === "A" ? this : null; };
El.prototype.getBoundingClientRect = function () { return { left: 1, top: 2, bottom: 3 }; };
Object.defineProperty(El.prototype, "firstChild", { get: function () { return this.children[0] || null; } });
Object.defineProperty(El.prototype, "textContent", {
  get: function () { return this.text != null ? this.text : this.children.map(function (c) { return c.textContent; }).join(""); },
  set: function (v) { this.children = []; this.text = v === "" ? null : v; }
});
// find returns the first descendant with the tag.
function find(el, tag) {
  for (var i = 0; i < el.children.length; i++) {
    if (el.children[i].tagName === tag) return el.children[i];
    var hit = find(el.children[i], tag);
    if (hit) return hit;
  }
  return null;
}
var root = new El("html"), listeners = {};
root.attrs["data-rid"] = "rid1";
var document = {
  documentElement: root, body: new El("body"),
  createElement: function (tag) { return new El(tag); },
  addEventListener: function (k, f) { (listeners[k] = listeners[k] || []).push(f); }
};
function wsT(k) { return k; }
`

// TestOutsidePreviewCardRemintsOnEveryLoad pins the preview card's side of
// an outside-root link (preview.js), executed under node when it is
// installed: an outside card's record is the absolute path and the out
// mark, never a capability URL; every load, the first and a restored
// card's, posts the path to the outside-link route and draws its answer,
// so each load shows a freshly minted capability; a refusal shows the "no
// longer available" note, where a file missing under the root keeps its
// own; a file inside the root still asks the preview route by GET; and the
// card's open-in-default-app action names the file by its folder and name.
func TestOutsidePreviewCardRemintsOnEveryLoad(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/preview.js")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := json.Marshal(string(data))
	runNode(t, "outside preview card", outsideDOM+`
var calls = [], answers = [], posts = [];
function fetch(url, o) {
  calls.push({ url: url, method: (o && o.method) || "GET", body: (o && o.body) || "" });
  var a = answers.shift() || { status: 500 };
  return Promise.resolve({ ok: !a.status, status: a.status || 200, json: function () { return Promise.resolve(a); } });
}
var kinds = {}, list = [], placements = [];
function Card(kind, data) {
  this.kind = kind; this.data = data; this.pinned = false; this.body = new El("div"); this.saves = 0;
  this.body.clientHeight = 100;
}
Card.prototype.title = function (t) { this.head = t; };
Card.prototype.actions = function (a) { this.acts = a; };
["fold", "show", "close"].forEach(function (k) { Card.prototype[k] = function () {}; });
Card.prototype.bump = function (origin) { placements.push(["bump", origin]); };
Card.prototype.save = function () { this.saves++; };
var window = { WsCards: {
  register: function (name, kind) { kinds[name] = kind; },
  list: function (kind) { return list.filter(function (c) { return c.kind === kind; }); },
  open: function (kind, data, at, origin) { placements.push(["open", origin]); var c = new Card(kind, data); list.push(c); kinds[kind].open(c); return c; }
} };
eval(`+string(src)+`);
window.WsPreview.setup({ post: function (route, body, done) { posts.push({ route: route, body: body }); done(true, {}); }, toast: function () {} });
function settle() { return new Promise(function (r) { setTimeout(r, 0); }); }
(async function () {
  var abs = "/away/site/page.html", mint = { url: "/s/rid1/fs/outside", method: "POST", body: JSON.stringify({ path: abs }) };
  root.attrs["data-sysopen"] = "";

  // The first load posts the path and frames the answer's capability URL.
  answers.push({ kind: "frame", abs: abs, frame: "/s/rid1/fs/ocap/CAP1/page.html", view: "/s/rid1/fs/oview/VIEW1/page.html", ttl: 900 });
  var origin = { topic: "exact node" };
  var card = window.WsPreview.open(abs, { out: true, origin: origin });
  await settle();
  check("new preview carries exact node only as transient placement", [placements.length, placements[0][0], placements[0][1] === origin, "origin" in card.data], [1, "open", true, false]);
  check("the first load", calls, [mint]);
  check("the card's record", card.data, { path: abs, out: true });
  var fr = find(card.body, "IFRAME");
  check("the frame", fr && [fr.src, fr.attrs.sandbox], ["/s/rid1/fs/ocap/CAP1/page.html", "allow-scripts"]);
  check("the card's title", [card.head.name, card.head.sub], ["page.html", abs]);

  // A restored card, and a reopened one, post again: no capability is kept.
  answers.push({ kind: "frame", frame: "/s/rid1/fs/ocap/CAP2/page.html" });
  kinds.preview.open(card);
  await settle();
  answers.push({ kind: "frame", frame: "/s/rid1/fs/ocap/CAP3/page.html" });
  window.WsPreview.open(abs, { out: true, origin: origin });
  await settle();
  check("same-path ordinary reuse bumps with exact origin", [placements.length, placements[1][0], placements[1][1] === origin], [2, "bump", true]);
  check("every load posts", calls, [mint, mint, mint]);
  check("the reloaded frame", find(card.body, "IFRAME").src, "/s/rid1/fs/ocap/CAP3/page.html");
  check("one slot", list.length, 1);
  check("no capability in the record", JSON.stringify(card.data).indexOf("CAP"), -1);

  // The card's own action opens the file by its folder and its name.
  card.acts[0].run(card);
  check("open in the default app", posts, [{ route: "fs/act", body: { path: "page.html", action: "open", root: "/away/site/" } }]);

  // The reload action is the last header button: a click asks the route
  // again and redraws the frame with the new capability.
  var rl = card.acts[card.acts.length - 1];
  check("the reload action", [rl.icon, rl.title, card.acts.length], ["refresh-cw", "js.preview.reload", 2]);
  calls = [];
  answers.push({ kind: "frame", frame: "/s/rid1/fs/ocap/CAP5/page.html" });
  rl.run(card);
  await settle();
  check("a reload asks again", calls, [mint]);
  check("a reload redraws the frame", find(card.body, "IFRAME").src, "/s/rid1/fs/ocap/CAP5/page.html");

  // An image and markdown draw from the same answer shape.
  calls = [];
  answers.push({ kind: "image", src: "/s/rid1/fs/ocap/CAP4/shot.png" });
  window.WsPreview.open("C:\\away\\shot.png", { out: true });
  await settle();
  check("an image's load", calls, [{ url: "/s/rid1/fs/outside", method: "POST", body: JSON.stringify({ path: "C:\\away\\shot.png" }) }]);
  check("the image", find(card.body, "IMG").src, "/s/rid1/fs/ocap/CAP4/shot.png");
  posts = [];
  card.acts[0].run(card);
  check("open a Windows path", posts[0].body, { path: "shot.png", action: "open", root: "C:\\away\\" });
  answers.push({ kind: "markdown", html: "<h1>Title</h1>" });
  window.WsPreview.open("/away/notes.md", { out: true });
  await settle();
  check("markdown", find(card.body, "ARTICLE").innerHTML, "<h1>Title</h1>");

  // A refusal says the file is no longer available; any other failure and
  // a file missing under the root keep their own notes.
  answers.push({ status: 404 });
  window.WsPreview.open(abs, { out: true });
  await settle();
  check("a refused mint", [card.body.textContent, find(card.body, "IFRAME"), find(card.body, "BUTTON")], ["js.preview.gone", null, null]);
  answers.push({ status: 403 });
  kinds.preview.open(card);
  await settle();
  check("a failed mint", card.body.textContent, "js.preview.failed");
  calls = [];
  answers.push({ status: 404 });
  window.WsPreview.open("docs/a.md");
  await settle();
  check("a file inside the root asks the preview route", calls, [{ url: "/s/rid1/fs/preview?path=docs%2Fa.md", method: "GET", body: "" }]);
  check("a file missing under the root", [card.body.textContent, card.data], ["js.preview.missing", { path: "docs/a.md" }]);

  // A pinned card of a path inside the root is not the outside card of
  // the same text.
  card.pinned = true;
  var before = placements.length;
  answers.push({ kind: "text", text: "pinned" });
  var matched = window.WsPreview.open("docs/a.md", { origin: origin, line: 12 });
  await settle();
  check("matching pinned reuse never repositions or persists origin", [matched === card, card.pinned, placements.length === before, card.data], [true, true, true, { path: "docs/a.md", line: 12 }]);
  answers.push({ kind: "text", text: "x" });
  var other = window.WsPreview.open("docs/a.md", { out: true });
  await settle();
  check("an outside card is its own", [other !== card, other.data, list.length], [true, { path: "docs/a.md", out: true }, 2]);
  process.stdout.write(bad.join("\n"));
})();
`)
}

// TestOutsideLinkClickPath pins the page's side of an outside-root link
// (dashboard.js), executed under node when it is installed. On a page the
// server marked data-sysopen, a pointer click on a link to a file outside
// the root, an image link included, opens the preview card on its absolute
// path; its menu (the right click, the keyboard activation, a topic-body
// frame's posted link) offers the preview and keeps the items it had, and
// for an HTML file offers a new tab, which posts the outside-link route and
// sends the tab only to the page URL the answer names under this session's
// oview route, closing it on a refusal. Any other page (a LAN viewer)
// keeps the copy-path menu and asks the route nothing. A link inside the
// root opens as before.
func TestOutsideLinkClickPath(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	cut := func(from, to string) string {
		i := strings.Index(js, from)
		j := strings.Index(js, to)
		if i < 0 || j < i {
			t.Fatalf("dashboard.js lacks the span from %q to %q", from, to)
		}
		return js[i:j]
	}
	path := cut("  // linkOpens reports whether this page can open", "  // An image outside the session root (img[data-out])")
	frame := cut("  function str(v) {", "  // relayHref is the URL the host fetches")
	for _, want := range []string{"function outsideOpens(a) {", "function openOutsideTab(abs) {", "function openLinkMenu(a, x, y) {", `doc.addEventListener("click"`, `doc.addEventListener("contextmenu"`} {
		if !strings.Contains(path, want) {
			t.Fatalf("the click path lacks %q", want)
		}
	}
	// The mint is a POST from a click: the click path holds no other
	// request, and no capability URL is written anywhere in it.
	if strings.Contains(path, "fetch(") || strings.Count(path, `post("fs/outside"`) != 1 || strings.Contains(path, "/fs/ocap/") {
		t.Errorf("the click path asks the outside-link route other than by its one post")
	}
	runNode(t, "outside link click path", outsideDOM+`
var doc = document, rid = "rid1", cardBoard = {}, lmenu = null, lmenuFrom = null;
var opened = [], posts = [], toasts = [], copied = [], tabs = [], answer = null;
function Tab(url) { var t = this; this.url = url; this.opener = {}; this.closed = false; this.location = { replace: function (u) { t.url = u; } }; }
Tab.prototype.close = function () { this.closed = true; };
var window = {
  innerWidth: 800, innerHeight: 600,
  WsPreview: { takes: function (p) { return !/\.pdf$/.test(p); }, open: function (p, o) { opened.push(o ? [p, o] : [p]); } },
  open: function (url, target, features) { var t = new Tab(url); t.features = features || ""; tabs.push(t); return t; }
};
function post(route, body, done) { posts.push([route, body]); done(!!answer, answer || "not found"); }
function toast(s) { toasts.push(s); }
function copyText(s) { copied.push(s); }
function closeLinkMenu() { lmenu = null; lmenuFrom = null; }
`+path+frame+`
function link(attrs, href) { var a = new El("a"); Object.keys(attrs).forEach(function (k) { a.attrs[k] = attrs[k]; }); if (href) { a.attrs.href = href; a.href = "http://127.0.0.1:1" + href; } return a; }
function fire(type, a, detail) {
  var e = { type: type, target: a, button: 0, detail: detail, clientX: 5, clientY: 6, prevented: false, preventDefault: function () { this.prevented = true; } };
  listeners[type].forEach(function (f) { f(e); });
  return e;
}
function menu() { var m = lmenu; return m ? m.children.map(function (b) { return b.textContent; }) : null; }
function pick(label) { var b = lmenu.children.filter(function (b) { return b.textContent === label; })[0]; b.on.click({ stopPropagation: function () {} }); }
function reset() { opened = []; posts = []; toasts = []; copied = []; tabs = []; lmenu = null; }
var html = link({ "data-wl": "file", "data-abs": "/away/site/page.html" });
var notes = link({ "data-wl": "file", "data-abs": "/away/notes.md" });
var pdf = link({ "data-wl": "file", "data-abs": "/away/paper.pdf" });
var shot = link({ "data-wl": "file", "data-abs": "/away/shot.png", "data-img": "", "data-out": "" }, "/s/rid1/image?path=%2Faway%2Fshot.png");
var inside = link({ "data-wl": "file", "data-rel": "docs/a.md", "data-abs": "/root/docs/a.md" }, "/s/rid1/file?path=docs%2Fa.md");

// A browser on the daemon's machine: the click opens the card.
root.attrs["data-sysopen"] = "";
[[html, "/away/site/page.html"], [notes, "/away/notes.md"], [shot, "/away/shot.png"]].forEach(function (c) {
  reset();
  var e = fire("click", c[0], 1);
  check("a click on " + c[1], [opened, menu(), e.prevented, posts], [[[c[1], { out: true }]], null, true, []]);
});
reset();
fire("click", inside, 1);
check("a click on a link inside the root", [opened, menu()], [[["docs/a.md"]], null]);
reset();
fire("click", pdf, 1);
check("a click on a file no preview shows", [opened, menu()], [[], ["js.menu.copyabs"]]);

// Direct and menu paths carry the exact enclosing node, not its name.
var exactTopic = { name: "same-name-is-not-authority" }, closest = inside.closest;
inside.closest = function (s) { return s === "section.topic[data-topic]" ? exactTopic : closest.call(this, s); };
reset(); fire("click", inside, 1);
check("direct link passes exact origin", opened[0][1].origin === exactTopic, true);
reset(); fire("contextmenu", inside, 1); pick("js.menu.preview");
check("context preview passes exact origin", opened[0][1].origin === exactTopic, true);
inside.closest = closest;

// The menu: the keyboard activation and the right click.
reset();
fire("click", html, 0);
check("the keyboard menu of an HTML file", [opened, menu()], [[], ["js.menu.preview", "js.menu.newtab", "js.menu.copyabs"]]);
reset();
var ctx = fire("contextmenu", notes, 1);
check("the right-click menu of a markdown file", [menu(), ctx.prevented], [["js.menu.preview", "js.menu.copyabs"], true]);
pick("js.menu.preview");
check("the menu's preview", opened, [["/away/notes.md", { out: true }]]);
reset();
fire("contextmenu", shot, 1);
check("the menu of an image link keeps its items", menu(), ["js.menu.preview", "js.menu.newtab", "js.menu.native", "js.menu.copyabs"]);
pick("js.menu.newtab");
check("an image's new tab is the image route", [tabs.map(function (t) { return [t.url, t.features]; }), posts], [[["http://127.0.0.1:1/s/rid1/image?path=%2Faway%2Fshot.png", "noopener"]], []]);
reset();
check("the right click on a file no preview shows stays the browser's", [fire("contextmenu", pdf, 1).prevented, menu()], [false, null]);

// An HTML file's new tab: one post, and the tab goes only to the answer's
// page URL under this session's oview route, cut from this page.
reset();
answer = { kind: "frame", frame: "/s/rid1/fs/ocap/CAP/page.html", view: "/s/rid1/fs/oview/VIEW/page.html" };
fire("contextmenu", html, 1);
pick("js.menu.newtab");
check("the new tab's post", posts, [["fs/outside", { path: "/away/site/page.html" }]]);
check("the new tab", tabs.map(function (t) { return [t.url, t.opener, t.closed]; }), [["/s/rid1/fs/oview/VIEW/page.html", null, false]]);
check("no toast", toasts, []);
[null, { kind: "text", text: "x" }, { view: "https://evil.test/x" }, { view: "/s/other/fs/oview/V/page.html" }, { view: "/s/rid1/fs/ocap/CAP/page.html" }, { view: 7 }].forEach(function (v) {
  reset();
  answer = v;
  fire("contextmenu", html, 1);
  pick("js.menu.newtab");
  check("a new tab answered " + JSON.stringify(v), [tabs.map(function (t) { return [t.url, t.closed]; }), toasts], [[["", true]], ["js.preview.gone"]]);
});

// A topic-body frame's posted link takes the same menu, and what it names
// is only a path the daemon then checks.
reset();
openLinkMenu(frameLink({ kind: "file", href: "", abs: "/away/site/page.html" }), 1, 2);
check("a framed link's menu", menu(), ["js.menu.preview", "js.menu.newtab", "js.menu.copyabs"]);
reset();
openLinkMenu(frameLink({ kind: "file", href: "/s/rid1/file?path=docs%2Fa.md", rel: "docs/a.md", abs: "/root/docs/a.md" }), 1, 2);
check("a framed link inside the root", menu()[0], "js.menu.preview");
pick("js.menu.preview");
check("a framed link inside the root previews by its path under the root", opened, [["docs/a.md"]]);

// Any other page: the copy-path menu, and nothing asked of the route.
delete root.attrs["data-sysopen"];
[html, notes, shot].forEach(function (a) {
  reset();
  fire("click", a, 1);
  check("a LAN click on " + a.attrs["data-abs"], [opened, menu(), posts, tabs.length], [[], ["js.menu.copyabs"], [], 0]);
  reset();
  check("a LAN right click on " + a.attrs["data-abs"], [fire("contextmenu", a, 1).prevented, menu(), posts], [false, null, []]);
});
reset();
openLinkMenu(frameLink({ kind: "file", href: "", abs: "/away/site/page.html" }), 1, 2);
check("a LAN framed link's menu", [menu(), posts], [["js.menu.copyabs"], []]);
pick("js.menu.copyabs");
check("the copied path", copied, ["/away/site/page.html"]);

// A page with no card board keeps the menu too.
root.attrs["data-sysopen"] = "";
cardBoard = null;
reset();
fire("click", html, 1);
check("a click with no card board", [opened, menu()], [[], ["js.menu.copyabs"]]);
process.stdout.write(bad.join("\n"));
`)
}

// TestOutsideRemintPagePostsOnceAndGoesOnlyToItsOwnRoute pins the re-mint
// page's script, taken from the page the route serves for an expired tab
// and executed under node when it is installed: it posts the linked path
// to the mint route once, and replaces the tab with the answer's page URL
// under the session's oview route, keeping the path the tab showed; a
// refusal, an answer naming no such URL, and a second run within a few
// seconds (a fresh URL that failed again) leave the "no longer available"
// note and go nowhere.
func TestOutsideRemintPagePostsOnceAndGoesOnlyToItsOwnRoute(t *testing.T) {
	clock := &capClock{now: time.Unix(1_800_000_000, 0)}
	o := startOutsideFixture(t)
	o.d.fsNow = clock.read
	page := o.write(t, o.away, "page.html", "<h1>HTML-AWAY</h1>")
	o.write(t, o.away, "sub/next page.html", "<h1>NEXT-AWAY</h1>")
	a := o.linked(t, page)
	clock.advance(fsCapabilityTTL + time.Second)
	body := capFetch(o.pageFixture, o.local, under(a.View, "sub/next%20page.html"), capShape{"", "none", "navigate", "document"}).Body.String()

	script := regexp.MustCompile(`<script nonce="[0-9a-f]{32}">([\s\S]*?)</script>`).FindStringSubmatch(body)
	if script == nil || strings.Count(body, "<script") != 1 {
		t.Fatalf("the re-mint page holds no single nonce script:\n%s", body)
	}
	attrs := map[string]string{}
	for _, m := range regexp.MustCompile(` (data-[a-z]+)="([^"]*)"`).FindAllStringSubmatch(body, -1) {
		attrs[m[1]] = html.UnescapeString(m[2])
	}
	if attrs["data-abs"] != page || attrs["data-mint"] != "/s/livesession/fs/outside" {
		t.Fatalf("the re-mint page's attributes = %v", attrs)
	}
	in, _ := json.Marshal(attrs)
	src, _ := json.Marshal(script[1])
	runNode(t, "re-mint page", `
var bad = [];
function check(name, got, want) {
  var g = JSON.stringify(got), w = JSON.stringify(want);
  if (g !== w) bad.push(name + ": got " + g + ", want " + w);
}
var attrs = `+string(in)+`, note = { hidden: true }, store = {}, calls = [], replaced = [], answer = null, clock = 1e12;
var document = { body: { getAttribute: function (k) { return attrs[k]; } }, getElementById: function (id) { return id === "fgone" ? note : null; } };
var sessionStorage = { getItem: function (k) { return k in store ? store[k] : null; }, setItem: function (k, v) { store[k] = v; } };
var window = { fetch: true };
var location = { replace: function (u) { replaced.push(u); } };
Date.now = function () { return clock; };
function fetch(url, o) {
  calls.push([url, o.method, o.body]);
  return Promise.resolve({ ok: !answer.status, status: answer.status || 200, json: function () { return Promise.resolve(answer); } });
}
function run(v) {
  answer = v; calls = []; replaced = []; note.hidden = true;
  eval(`+string(src)+`);
  return new Promise(function (r) { setTimeout(r, 0); });
}
(async function () {
  var mint = [["/s/livesession/fs/outside", "POST", JSON.stringify({ path: attrs["data-abs"] })]];
  await run({ kind: "frame", view: "/s/livesession/fs/oview/NEW/page.html" });
  check("a fresh URL", [calls, replaced, note.hidden], [mint, ["/s/livesession/fs/oview/NEW/sub/next%20page.html"], true]);
  // The fresh URL landed here again at once: no second post, no loop.
  clock += 1000;
  await run({ kind: "frame", view: "/s/livesession/fs/oview/NEW/page.html" });
  check("a second run at once", [calls, replaced, note.hidden], [[], [], false]);
  var refusals = [{ status: 404 }, { status: 403 }, { kind: "text", text: "x" }, { view: 7 }, { view: "https://evil.test/s/livesession/fs/oview/N/x.html" },
    { view: "/s/other/fs/oview/N/page.html" }, { view: "/s/livesession/fs/ocap/N/page.html" }];
  for (var i = 0; i < refusals.length; i++) {
    clock += 6000;
    await run(refusals[i]);
    check("answered " + JSON.stringify(refusals[i]), [calls, replaced, note.hidden], [mint, [], false]);
  }
  process.stdout.write(bad.join("\n"));
})();
`)
}
