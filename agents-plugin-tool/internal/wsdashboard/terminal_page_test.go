package wsdashboard

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestTerminalCardScriptRidesOnlyAPageThatOffersTerminals pins where the
// terminal card kind (terminal.js) goes: into a session page that offers
// terminals, after the card framework and before the page script that
// mounts it, and into no other page, so a LAN viewer's page carries no
// terminal code at all.
func TestTerminalCardScriptRidesOnlyAPageThatOffersTerminals(t *testing.T) {
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)})
	f := startPageFixture(t, lanMode, withHolders(manager))
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}
	lan := viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}
	const kind = "window.WsTerm = {"
	page := serve(f.d, local, "GET", "/s/livesession", "").Body.String()
	at, cardsAt, mountAt := strings.Index(page, kind), strings.Index(page, "window.WsCards = {"), strings.Index(page, "window.WsTerm.mount(")
	if at < 0 || cardsAt < 0 || mountAt < 0 || !(cardsAt < at && at < mountAt) {
		t.Errorf("a local viewer's page lacks the terminal kind between the card framework and the page script (kind %d, framework %d, mount %d)", at, cardsAt, mountAt)
	}
	if !strings.Contains(page, `<symbol id="l-square-terminal" viewBox="0 0 24 24">`) || !strings.Contains(page, `<symbol id="l-plus" viewBox="0 0 24 24">`) {
		t.Error("the session page lacks the terminal card's icons")
	}
	if page := serve(f.d, lan, "GET", "/s/livesession", "").Body.String(); strings.Contains(page, kind) || !strings.Contains(page, "window.WsCards = {") {
		t.Error("a LAN viewer's page carries the terminal kind")
	}
	plain := startPageFixture(t, withHolders(newFakeHolders()))
	plainLocal := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(plain.d.Endpoint().Port)}
	if page := serve(plain.d, plainLocal, "GET", "/s/livesession", "").Body.String(); strings.Contains(page, kind) {
		t.Error("a daemon that runs no terminals serves the terminal kind")
	}
}

// TestTerminalCardScriptMatchesTheRoutes pins what terminal.js takes from
// the daemon's side: the base height is the routes' default row count, the
// popover maps each folder refusal code the create route answers, and the
// card connects from the record after the last one it took.
func TestTerminalCardScriptMatchesTheRoutes(t *testing.T) {
	js := mustAsset(t, "terminal.js")
	if m := regexp.MustCompile(`\bROWS = (\d+)`).FindStringSubmatch(js); m == nil || m[1] != strconv.Itoa(terminalDefaultRows) {
		t.Errorf("terminal.js ROWS = %v, want the routes' default %d rows", m, terminalDefaultRows)
	}
	for _, code := range []string{terminalFolderRelative, terminalFolderMissing, terminalFolderNotDir} {
		if !regexp.MustCompile(`\b` + code + `: "js\.term\.err\.[a-z]+"`).MatchString(js) {
			t.Errorf("terminal.js maps no text to the folder refusal %q", code)
		}
	}
	if !strings.Contains(js, `"/ws?from=" + (s.last ? s.last + 1 : 0)`) {
		t.Error("terminal.js does not connect from the record after the last it took (0 for a new card)")
	}
	if files := mustAsset(t, "files.js"); !strings.Contains(files, `window.WsTerm.create({ folder: "path", path: dirAbs }`) {
		t.Error("the file explorer's folder menu does not open a terminal in the folder")
	}
}

// TestBoardHeaderRowKeepsTheRailBelowIt pins the board column's header row
// (terminal.js .bhead) and the topic rail sharing the column's top: the row
// is a fixed-height sticky row, the column sets the rail's inset to that
// height only while it holds the row, and the rail (wswhiteboard rail.js,
// page.css) sticks its top capsules and the current bar below the inset.
func TestBoardHeaderRowKeepsTheRailBelowIt(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	row := regexp.MustCompile(`(?m)^\.bhead \{[^}]*\}`).FindString(css)
	if !strings.Contains(row, "position: sticky; top: 0;") || !strings.Contains(row, "height: 28px;") {
		t.Errorf("the board header row is not a 28px sticky row: %q", row)
	}
	if !strings.Contains(css, ".bcol:has(> .bhead) { --wb-rail-inset: 28px; }") {
		t.Error("the board column does not set the rail's inset to the header row's height while it holds the row")
	}
	wb, err := wswhiteboard.PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wb.CSS), ".wb-rail-wrap.up { top: var(--wb-rail-inset, 0px);") {
		t.Error("the rail's top capsules do not stick below the host's inset")
	}
	for _, want := range []string{`getPropertyValue("--wb-rail-inset")`, `root.getBoundingClientRect().top + padTop + inset`, `host.style.setProperty("--wb-rail-top", inset + `, `host.style.setProperty("--wb-rail-h", inset + `} {
		if !strings.Contains(string(wb.FormScript), want) {
			t.Errorf("the rail script lacks %q", want)
		}
	}
}
