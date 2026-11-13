package wsdashboard

import (
	"os/exec"
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

// TestWebCardScriptRidesOnlyALocalViewersPage pins where the web card kind
// (webcard.js) goes: into the session page of a browser on this machine,
// after the card framework and before the page script that sets it up and
// mounts it, with the icons it draws; a LAN viewer's page carries no web
// card code at all.
func TestWebCardScriptRidesOnlyALocalViewersPage(t *testing.T) {
	f := startPageFixture(t, lanMode)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}
	lan := viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}
	const kind = "window.WsWeb = {"
	page := serve(f.d, local, "GET", "/s/livesession", "").Body.String()
	cardsAt, at := strings.Index(page, "window.WsCards = {"), strings.Index(page, kind)
	setupAt, mountAt := strings.Index(page, "window.WsWeb.setup("), strings.Index(page, "window.WsWeb.mount(")
	if cardsAt < 0 || at < 0 || setupAt < 0 || mountAt < 0 || !(cardsAt < at && at < setupAt && setupAt < mountAt) {
		t.Errorf("a local viewer's page lacks the web card kind between the card framework and the page script (framework %d, kind %d, setup %d, mount %d)", cardsAt, at, setupAt, mountAt)
	}
	for _, icon := range []string{"globe", "square-mouse-pointer", "bug"} {
		if !strings.Contains(page, `<symbol id="l-`+icon+`" viewBox="0 0 24 24">`) {
			t.Errorf("the session page lacks the web card's %s icon", icon)
		}
	}
	if page := serve(f.d, lan, "GET", "/s/livesession", "").Body.String(); strings.Contains(page, kind) || !strings.Contains(page, "window.WsCards = {") {
		t.Error("a LAN viewer's page carries the web card kind")
	}
}
