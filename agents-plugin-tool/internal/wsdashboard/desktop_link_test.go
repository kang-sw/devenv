package wsdashboard

import (
	"os"
	"strings"
	"testing"
)

// desktopUA is the Werkschaft desktop app's user agent shape: WKWebView's
// macOS default followed by its product token.
const desktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Werkschaft/0.1.0"

// TestDesktopPathClickOpensMenuAndModifiedClickPreviews pins the host page's
// file link click in the Werkschaft desktop app, whose webview has no tabs:
// a plain click opens the link menu, a pointer Cmd/Ctrl+click previews what
// the card can show, and every click on a file link stays on the page
// (prevented), never the webview's in-place navigation. A file the card
// cannot show and a keyboard activation keep the menu. A url link and any
// browser keep their behavior: a plain click previews, a modified click is
// the browser's new tab.
func TestDesktopPathClickOpensMenuAndModifiedClickPreviews(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	from, to := "  // linkOpens reports whether this page can open", "  // An image outside the session root (img[data-out])"
	i, j := strings.Index(js, from), strings.Index(js, to)
	if i < 0 || j < i {
		t.Fatalf("dashboard.js lacks the span from %q to %q", from, to)
	}
	runNode(t, "desktop path click", outsideDOM+`
var doc = document, rid = "rid1", cardBoard = {}, lmenu = null, lmenuFrom = null;
var opened = [], tabs = [];
var window = {
  innerWidth: 800, innerHeight: 600,
  WsPreview: { takes: function (p) { return !/\.pdf$/.test(p); }, open: function (p, o) { opened.push(o ? [p, o] : [p]); } },
  open: function (url) { tabs.push(url); return null; }
};
function post() {} function toast() {} function copyText() {}
function closeLinkMenu() { lmenu = null; lmenuFrom = null; }
`+js[i:j]+`
function link(attrs, href) { var a = new El("a"); Object.keys(attrs).forEach(function (k) { a.attrs[k] = attrs[k]; }); if (href) { a.attrs.href = href; a.href = "http://127.0.0.1:1" + href; } return a; }
function fire(a, detail, mods) {
  mods = mods || {};
  var e = { type: "click", target: a, button: 0, detail: detail, clientX: 5, clientY: 6, metaKey: !!mods.meta, ctrlKey: !!mods.ctrl, shiftKey: !!mods.shift, altKey: false,
    prevented: false, preventDefault: function () { this.prevented = true; } };
  listeners.click.forEach(function (f) { f(e); });
  return e;
}
function menu() { return lmenu ? lmenu.children.map(function (b) { return b.textContent; }) : null; }
function reset() { opened = []; tabs = []; lmenu = null; }
function run(a, detail, mods) { reset(); var e = fire(a, detail, mods); return [opened, menu(), e.prevented]; }
var inside = link({ "data-wl": "file", "data-rel": "docs/a.md", "data-abs": "/root/docs/a.md" }, "/s/rid1/file?path=docs%2Fa.md");
var pdf = link({ "data-wl": "file", "data-rel": "docs/p.pdf", "data-abs": "/root/docs/p.pdf" }, "/s/rid1/file?path=docs%2Fp.pdf");
var away = link({ "data-wl": "file", "data-abs": "/away/site/page.html" });
var url = link({ "data-wl": "url" }, "/x");
url.attrs.href = "https://example.com/"; url.href = "https://example.com/";
root.attrs["data-sysopen"] = "";
var insideMenu = ["js.menu.preview", "js.menu.newtab", "js.menu.sysopen", "js.menu.copyrel", "js.menu.copyabs"];

// A browser: unchanged.
check("browser plain click previews", run(inside, 1), [[["docs/a.md"]], null, true]);
check("browser Cmd+click is the new tab", run(inside, 1, { meta: true }), [[], null, false]);

// The desktop app.
window.navigator = { userAgent: "` + desktopUA + `" };
check("desktop plain click opens the menu", run(inside, 1), [[], insideMenu, true]);
check("desktop Cmd+click previews", run(inside, 1, { meta: true }), [[["docs/a.md"]], null, true]);
check("desktop Ctrl+click previews", run(inside, 1, { ctrl: true }), [[["docs/a.md"]], null, true]);
check("desktop Shift+click stays on the page", run(inside, 1, { shift: true }), [[], insideMenu, true]);
check("desktop keyboard activation opens the menu", run(inside, 0, { meta: true }), [[], insideMenu, true]);
check("desktop Cmd+click on a file the card cannot show opens the menu", run(pdf, 1, { meta: true })[2], true);
check("desktop Cmd+click on a file the card cannot show lists no preview", run(pdf, 1, { meta: true })[1].indexOf("js.menu.preview"), -1);
check("desktop plain click on an outside file opens the menu", run(away, 1), [[], ["js.menu.preview", "js.menu.newtab", "js.menu.copyabs"], true]);
check("desktop Cmd+click on an outside file previews it", run(away, 1, { meta: true }), [[["/away/site/page.html", { out: true }]], null, true]);
check("desktop Cmd+click on a url link stays native", run(url, 1, { meta: true }), [[], null, false]);
check("no preview without a card board", (function () { cardBoard = null; var r = run(inside, 1, { meta: true }); cardBoard = {}; return [r[0], r[2]]; })(), [[], true]);
process.stdout.write(bad.join("\n"));
`)
}

// TestDocFrameDesktopModifiedClickGoesToHost pins the topic-body frame's
// side: in the desktop app a modified click on a marked file link is
// prevented and handed to the host with mod set for a pointer Cmd/Ctrl+click
// (the frame's <base target="_blank"> would otherwise ask the webview for a
// new window); a browser keeps a modified click on a link with an href as its
// new tab, and a url link stays native in either.
func TestDocFrameDesktopModifiedClickGoesToHost(t *testing.T) {
	data, err := os.ReadFile("../wswhiteboard/assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	from, to := "  var desktop = ", "  d.addEventListener(\"keydown\""
	i, j := strings.Index(js, from), strings.Index(js, to)
	if i < 0 || j < i {
		t.Fatalf("doc-frame.js lacks the span from %q to %q", from, to)
	}
	runNode(t, "doc frame desktop click", `
var bad = [];
function check(name, got, want) { var g = JSON.stringify(got), w = JSON.stringify(want); if (g !== w) bad.push(name + ": got " + g + ", want " + w); }
function frame(ua) {
  var navigator = { userAgent: ua }, handlers = [], posted = [];
  var d = { addEventListener: function (k, f) { if (k === "click") handlers.push(f); } };
  function post(m) { posted.push(m); }
  function wbSvgTarget() { return null; } function wbFlattenSvg() { return null; }
`+js[i:j]+`
  return function (attrs, mods) {
    var a = { attrs: attrs, getAttribute: function (k) { return k in this.attrs ? this.attrs[k] : null; }, hasAttribute: function (k) { return k in this.attrs; },
      closest: function (s) { return s === "a" ? this : null; }, getBoundingClientRect: function () { return { left: 1, bottom: 2 }; } };
    var e = { type: "click", target: a, button: 0, detail: 1, clientX: 5, clientY: 6, metaKey: !!mods.meta, ctrlKey: !!mods.ctrl, shiftKey: !!mods.shift, altKey: false,
      prevented: false, preventDefault: function () { this.prevented = true; } };
    posted.length = 0;
    handlers.forEach(function (f) { f(e); });
    return [e.prevented, posted.map(function (m) { return [m.wbDoc, m.kind, m.mod]; })];
  };
}
var file = { "data-wl": "file", href: "/s/rid1/file?path=docs%2Fa.md", "data-rel": "docs/a.md" };
var url = { "data-wl": "url", href: "https://example.com/" };
var browser = frame("Mozilla/5.0 Safari"), desktop = frame("` + desktopUA + `");
check("browser plain click", browser(file, {}), [true, [["link", "file", false]]]);
check("browser Cmd+click is the new tab", browser(file, { meta: true }), [false, []]);
check("desktop plain click", desktop(file, {}), [true, [["link", "file", false]]]);
check("desktop Cmd+click", desktop(file, { meta: true }), [true, [["link", "file", true]]]);
check("desktop Ctrl+click", desktop(file, { ctrl: true }), [true, [["link", "file", true]]]);
check("desktop Shift+click", desktop(file, { shift: true }), [true, [["link", "file", false]]]);
check("desktop Cmd+click on a url link stays native", desktop(url, { meta: true }), [false, []]);
process.stdout.write(bad.join("\n"));
`)
}
