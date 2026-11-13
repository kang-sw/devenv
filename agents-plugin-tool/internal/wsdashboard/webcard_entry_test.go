package wsdashboard

import (
	"os"
	"strings"
	"testing"
)

// TestWebCardEntryPoints pins where the page opens a web card: the link
// predicates (a localhost page link, an HTML file under the session root;
// only with the web card kind, a card board, and a data-sysopen page), the
// link menu entries, a pointer click on a localhost link, a board frame's
// link report only under user activation, and the preview card's action
// on an HTML file.
func TestWebCardEntryPoints(t *testing.T) {
	src, err := os.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	script := `
var bad = [];
function check(what, got, want) { if (got !== want) bad.push(what + ": got " + got + ", want " + want); }
var sysopen = true, cardBoard = {};
var root = { hasAttribute: function (n) { return n === "data-sysopen" && sysopen; } };
var window = { WsWeb: { localLink: function (h) { return /^http:\/\/localhost:5173\//.test(h); } } };
function link(attrs) {
  return { href: attrs.href || "", getAttribute: function (n) { return n in attrs ? attrs[n] : null; }, hasAttribute: function (n) { return n in attrs; } };
}
` + composerFunc(t, js, "webLink") + composerFunc(t, js, "webFile") + `
var dev = link({ "data-wl": "url", href: "http://localhost:5173/" });
check("a localhost page link", webLink(dev), true);
check("another site's link", webLink(link({ "data-wl": "url", href: "https://example.com/" })), false);
check("a file link is no page link", webLink(link({ "data-wl": "file", href: "http://localhost:5173/", "data-rel": "a.html" })), false);
check("an HTML file under the root", webFile(link({ "data-wl": "file", "data-rel": "site/index.HTML" })), true);
check("a file outside the root", webFile(link({ "data-wl": "file", "data-rel": "a.html", "data-out": "" })), false);
check("a file that is no HTML", webFile(link({ "data-wl": "file", "data-rel": "notes.md" })), false);
check("an outside file with no root path", webFile(link({ "data-wl": "file", "data-abs": "/tmp/a.html" })), false);
sysopen = false;
check("a page that is not data-sysopen", webLink(dev), false);
sysopen = true; cardBoard = null;
check("a page with no card board", webLink(dev), false);
cardBoard = {}; window.WsWeb = null;
check("a page with no web card kind", webLink(dev), false);
if (bad.length) console.log(bad.join("\n"));
`
	runNode(t, "web card link predicate", script)

	menu := composerFunc(t, js, "openLinkMenu")
	for _, want := range []string{
		`if (webLink(a)) items.push([wsT("js.web.menu.open"), function () { window.WsWeb.open({ url: url }); }]);`,
		`if (webFile(a)) items.push([wsT("js.web.menu.open"), function () { window.WsWeb.open({ path: rel }); }]);`,
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("the link menu lacks %s", want)
		}
	}
	for _, want := range []string{
		`if (e.detail !== 0 && webLink(a)) { window.WsWeb.open({ url: a.href }); return; }`,
		`if (!a || !(previews(a) || outsideOpens(a) || webLink(a))) return;`,
		`if (webLink(a) && act && act.isActive) { window.WsWeb.open({ url: a.href }); return; }`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard.js lacks the web card entry %s", want)
		}
	}
	preview, err := os.ReadFile("assets/preview.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(preview), `if (window.WsWeb && /^html?$/.test(ext(card.data.path))) a.push({ icon: "globe", title: wsT("js.web.menu.open"), run: openWeb });`) {
		t.Error("the preview card offers no web card on an HTML file")
	}
}
