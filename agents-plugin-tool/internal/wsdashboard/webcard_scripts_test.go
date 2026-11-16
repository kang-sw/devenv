package wsdashboard

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestWebCardScripts runs the node cases of the web card's injected script
// (webinject.js: protocol origins, selector, pick mode, console ring), its
// wrapper relay (webframe.js), and the card kind on the session page
// (webcard.js: the host's message filter, picks and console blocks, the
// re-registration, the localhost link predicate) against a fake DOM.
func TestWebCardScripts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/webinject.test.cjs, testdata/webframe.test.cjs, and testdata/webcard.test.cjs locally")
	}
	for _, file := range []string{"testdata/webinject.test.cjs", "testdata/webframe.test.cjs", "testdata/webcard.test.cjs"} {
		out, err := exec.Command(node, file).CombinedOutput()
		if err != nil {
			t.Errorf("%s failed: %v\n%s", file, err, out)
		}
	}
}

func TestWebCardPopoverFitsPositionedSlot(t *testing.T) {
	js := mustAsset(t, "webcard.js")
	place := regexp.MustCompile(`(?s)\n  function placePop\(\) \{.*?\n  \}\n`).FindString(js)
	if place == "" {
		t.Fatal("missing placePop")
	}
	runNode(t, "web popover placement", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": " + JSON.stringify(got)); }
var window = { innerWidth: 1400, innerHeight: 900 };
var slot = { left: 500, top: 100 }, button = { left: 500, bottom: 130 };
var column = { left: 300, right: 700, top: 50, bottom: 800 };
var ui = {
  btn: { getBoundingClientRect: function () { return button; } },
  col: { getClientRects: function () { return [1]; }, getBoundingClientRect: function () { return column; } },
  pop: { style: {}, getBoundingClientRect: function () {
    check("measure from slot origin", [this.style.left, this.style.top, this.style.maxHeight], ["0px", "0px", ""]);
    return { left: slot.left, top: slot.top, width: Math.min(340, parseFloat(this.style.maxWidth)), height: 100 };
  } }
};
`+place+`
function position() {
  placePop();
  return [slot.left + parseFloat(ui.pop.style.left), slot.top + parseFloat(ui.pop.style.top), ui.pop.style.maxWidth, ui.pop.style.maxHeight];
}
check("nonzero slot origin and column right clamp", position(), [352, 134, "384px", "658px"]);
check("local offsets do not double the slot origin", [ui.pop.style.left, ui.pop.style.top], ["-148px", "34px"]);
column.right = 550;
check("narrow column limits measured width", position(), [308, 134, "234px", "658px"]);
button = { left: 280, bottom: 770 };
check("column left and available height", position(), [308, 720, "234px", "72px"]);
button.bottom = 20;
check("column top bound", position(), [308, 58, "234px", "734px"]);
ui.col = null; window.innerWidth = 420; window.innerHeight = 600;
slot = { left: 200, top: 100 }; button = { left: 200, bottom: 130 };
check("viewport fallback and nonzero origin", position(), [72, 134, "404px", "458px"]);
button = { left: -20, bottom: 580 };
check("viewport left and bottom bounds", position(), [8, 520, "404px", "72px"]);
process.stdout.write(bad.join("\n"));
`)
}

// TestWebCardScriptRidesOnlyALocalViewersPage pins where the web card kind
// (webcard.js) goes: into the session page of a browser on this machine,
// after the card framework and before the page script that sets it up and
// mounts it, with the icons it draws; a LAN viewer's page carries no web
// card code at all.
func TestWebCardScriptRidesOnlyALocalViewersPage(t *testing.T) {
	f := startPageFixture(t, lanMode)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port, localCookie: true}
	lan := viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}
	const kind = "window.WsWeb = {"
	page := frameDependencies(t, serve(f.d, local, "GET", "/s/livesession?frame=1", "").Body.String())
	cardsAt, at := strings.Index(page, "window.WsCards = {"), strings.Index(page, kind)
	scriptAt := strings.Index(page, `<script src="/s/dashboard-`)
	script := mustAsset(t, "dashboard.js")
	setupAt, mountAt := strings.Index(script, "window.WsWeb.setup("), strings.Index(script, "window.WsWeb.mount(")
	if cardsAt < 0 || at < 0 || scriptAt < 0 || setupAt < 0 || mountAt < 0 || !(cardsAt < at && at < scriptAt && setupAt < mountAt) {
		t.Errorf("a local viewer's page lacks the web card kind between the card framework and the page script (framework %d, kind %d, setup %d, mount %d)", cardsAt, at, setupAt, mountAt)
	}
	for _, icon := range []string{"globe", "square-mouse-pointer", "bug"} {
		if !strings.Contains(page, `<symbol id="l-`+icon+`" viewBox="0 0 24 24">`) {
			t.Errorf("the session page lacks the web card's %s icon", icon)
		}
	}
	if page := frameDependencies(t, serve(f.d, lan, "GET", "/s/livesession?frame=1", "").Body.String()); strings.Contains(page, kind) || !strings.Contains(page, "window.WsCards = {") {
		t.Error("a LAN viewer's page carries the web card kind")
	}
}
