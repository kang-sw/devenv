package wsdashboard

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
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
	page := serve(f.d, local, "GET", "/s/livesession?frame=1", "").Body.String()
	at, cardsAt, mountAt := strings.Index(page, kind), strings.Index(page, "window.WsCards = {"), strings.Index(page, "window.WsTerm.mount(")
	if at < 0 || cardsAt < 0 || mountAt < 0 || !(cardsAt < at && at < mountAt) {
		t.Errorf("a local viewer's page lacks the terminal kind between the card framework and the page script (kind %d, framework %d, mount %d)", at, cardsAt, mountAt)
	}
	if !strings.Contains(page, `<symbol id="l-square-terminal" viewBox="0 0 24 24">`) {
		t.Error("the session page lacks the terminal card's icons")
	}
	if page := serve(f.d, lan, "GET", "/s/livesession?frame=1", "").Body.String(); strings.Contains(page, kind) || !strings.Contains(page, "window.WsCards = {") {
		t.Error("a LAN viewer's page carries the terminal kind")
	}
	plain := startPageFixture(t, withHolders(newFakeHolders()))
	plainLocal := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(plain.d.Endpoint().Port)}
	if page := serve(plain.d, plainLocal, "GET", "/s/livesession?frame=1", "").Body.String(); strings.Contains(page, kind) {
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

// The toolbar is the single new-terminal entry: no composer slot, fixed
// corner fallback, or copy-answers clearance survives the move.
func TestTerminalButtonLivesInBoardToolbar(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{})
	snap.Usage = &wslive.Usage{Context: &wslive.UsageContext{Tokens: i64p(1), Window: 10}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now})
	regions, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderSessionPage(view, regions, true, "xterm.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, `id="tslot"`) || !strings.Contains(page, `<div class="cfoot">`) {
		t.Error("the composer must keep its status line without a terminal slot")
	}
	if page, err = renderSessionPage(view, regions, true, ""); err != nil || strings.Contains(page, `id="tslot"`) || !strings.Contains(page, `<div class="cfoot">`) {
		t.Errorf("a page that offers no terminals has a terminal slot (err %v)", err)
	}
	js := mustAsset(t, "terminal.js")
	for _, want := range []string{
		`var slot = doc.getElementById("board-term-slot");`,
		`if (!slot) return;`,
		`ui.btn.setAttribute("aria-label", wsT("js.term.new"));`,
		`slot.appendChild(pop);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("terminal.js lacks %q", want)
		}
	}
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		".cfoot > .r-sline { flex: 1 1 auto; min-width: 0; }",
		".cfoot:has(> .r-sline:empty) { display: none; }",
		".tpop { position: absolute; top: calc(100% + 8px); left: 0;",
		"@container board-toolbar (max-width: 540px)",
		".board-action:not(.board-overview) .board-label, .board-action kbd { display: none; }",
		".r-board:not(:has(> .topic, > .bcard)) > .empty",
		".dgrid.live > .bcol > .r-board:not(:has(> .topic, > .bcard)):has(> .empty) { position: absolute; inset: 0 4px 0 2px; display: flex; flex-direction: column; }",
		".dgrid.live > .bcol > .r-board:not(:has(> .topic, > .bcard)) > .empty { flex: 1 1 0; }",
		".board-toolbar-face { container: board-toolbar / inline-size;", 
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	for _, gone := range []string{"tslot", "tfix", "margin-right: 44px"} {
		if strings.Contains(js, gone) || strings.Contains(css, gone) {
			t.Errorf("terminal slot leftover %q", gone)
		}
	}
	if strings.Contains(js, "bhead") || strings.Contains(css, ".bhead") {
		t.Error("the board column still has a header row for the terminal button")
	}
}

// TestBoardCardsJoinTheTopicRail pins the board cards' place in the topic
// rail (wswhiteboard rail.js's data-rail hook): every card is a rail item
// whose header is its bar and whose header icon and name make its capsule,
// and the card's header sticks at the rail's line with the stuck shadow, as
// a topic's bar does, except in a maximized card's sheet. The card clips
// rather than hides its overflow, so it is no scroll container for the
// sticky header.
func TestBoardCardsJoinTheTopicRail(t *testing.T) {
	js := mustAsset(t, "cards.js")
	for _, want := range []string{
		`el.setAttribute("data-rail", c.id);`,
		`head.setAttribute("data-rail-bar", "");`,
		`c.icon.setAttribute("data-rail-icon", "");`,
		`c.name.setAttribute("data-rail-name", "");`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("cards.js lacks %q", want)
		}
	}
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		"overflow: hidden; overflow: clip; display: flex; flex-direction: column; }",
		".wb-rail .bcard:not([data-max]) .bc-hd { position: sticky; top: var(--wb-rail-top, 0px); z-index: 1; background: var(--wb-card);",
		".wb-rail .bcard:not([data-max]) .bc-hd.wb-stuck { box-shadow: var(--wb-shadow), 0 -6px 0 6px var(--wb-bg); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}
