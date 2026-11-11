package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerItemLinkOpensTheItemOnItsStorePage pins the tracker page's
// item link (tracker.js applyRoute, the url a tracker event hook gets):
// #/<store>/<item> enters the store page, the hash becomes the store page's
// own route, and the item opens once the store's view holds it, so a link
// followed before the views load still opens it. An item the view does not
// hold opens nothing, and an unknown store is the store route's toast. The
// cases execute under node when it is installed.
func TestTrackerItemLinkOpensTheItemOnItsStorePage(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n  function routeStore\(\) \{.*?\n  function leavePage\(\)`).FindString(string(data))
	if block == "" {
		t.Fatal("tracker.js holds no routeStore ... leavePage block")
	}
	block = strings.TrimSuffix(block, "function leavePage()")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; item link cases not executed")
	}
	script := `
var location = { pathname: "/", search: "", hash: "" };
var history = { replaceState: function (s, t, u) { location.hash = u.slice(u.indexOf("#") < 0 ? u.length : u.indexOf("#")); } };
var views = {}, opened = [], toasts = [], bad = [];
var AD = { stores: function () { return ["team", "home"]; }, view: function (id) { return views[id] || null; } };
var ui = { page: null, store: null, open: null, item: null, win: false, routeItem: null };
function syncNow() {}
function bringUp() {}
function render() {}
function esc(s) { return s; }
function wsT(k, a) { return k + ":" + a; }
function toast(m) { toasts.push(m); }
function openItemWindow(s, i) { opened.push(s + "/" + i); return true; }
` + block + `
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }

// followed before the views load: the page enters, the item opens on the view's arrival
location.hash = "#/team/maya.pc%231";
applyRoute();
check("hash", location.hash, "#/team");
check("page", ui.page, "team");
check("nothing open yet", opened, []);
openRouteItem();
check("still waiting", opened, []);
views.team = { items: { "maya.pc#1": {} } };
openRouteItem();
check("opened", opened, ["maya.pc#1"].map(function (i) { return "team/" + i; }));
openRouteItem();
check("opened once", opened.length, 1);

// followed with the view loaded: opens at once, on the other store's page too
views.home = { items: { "x#2": {} } };
opened = [];
location.hash = "#/home/x%232";
applyRoute();
check("loaded: page", ui.page, "home");
check("loaded: opened", opened, ["home/x#2"]);

// an item the view does not hold opens nothing and is dropped
opened = [];
location.hash = "#/team/gone%231";
applyRoute();
check("unknown item", opened, []);
check("unknown item dropped", ui.routeItem, null);

// an unknown store is left to the store route's toast
location.hash = "#/nope/x";
applyRoute();
check("unknown store toast", toasts.length, 1);
check("unknown store page", ui.page, null);
process.stdout.write(bad.join("\n"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("item link cases failed:\n%s", s)
	}
}
