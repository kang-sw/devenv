// Page render tests cover live regions and script hooks.

package wsdashboard

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSessionPageRendersLiveRegions(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/s/livesession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d: %s", code, body)
	}
	for _, want := range []string{
		`data-region="tree"`, `data-region="entries"`, `data-region="convlive"`, `data-region="board"`, `data-region="panels"`,
		`id="composer"`, "live board text",
		// A topic body sits in its sandboxed frame; there is no Now slot.
		`<div class="doc-frame" title="Plan" data-doc="topic:Plan" data-doc-key="topic:Plan"`,
		`<span class="src wb">whiteboard</span>`, `<span class="src">terminal</span>`,
		"streamed &lt;partial&gt; reply",
		// The mail whose history entry exists expands to its body; the
		// other shows its first line only.
		`mail second line`, `<div class="mailline sent nobody" data-entry=`,
		// The peer is served here, so its name links to its page.
		`href="/s/boardsession"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("session page lacks %q", want)
		}
	}
	assertSettingsOwner(t, body, false)
	for _, leak := range []string{"hist-1.json", "hist-gone.json", f.project + "/", `id="wb-now"`} {
		if strings.Contains(body, leak) {
			t.Errorf("session page carries %q", leak)
		}
	}
	// A file: URL, as a word: the inlined string table says "profile:".
	if regexp.MustCompile(`\bfile:`).MatchString(body) {
		t.Errorf("session page carries a file: URL")
	}
}

// TestSessionHeadLeadsWithTheProjectAndLightsTheHarnessBadge pins the top
// bar's head: the project's folder name first, then the reply-id, then the
// harness badge, which carries the process indicator; a page without a
// harness keeps the bare stream dot alone.
func TestSessionHeadLeadsWithTheProjectAndLightsTheHarnessBadge(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession")
	name := filepath.Base(f.project)
	for _, head := range []string{
		`<span class="tph tph-id"><span class="tph-c"><span class="meta tph-r1">` + name + `</span><span class="meta sname"`,
		`>livesession</button>`,
		`<span class="sseq"> · #3</span></span></span></span><span class="tph tph-hs"><span class="hb" data-h="claude" aria-hidden="true"></span><span class="host sse" title="harness process running"><span class="dot"></span>claude</span></span><span class="tph tph-chips">`,
	} {
		if !strings.Contains(page, head) {
			t.Errorf("session head lacks %q", head)
		}
	}
	if !strings.Contains(page, `<span class="sse" id="sse" title="live updates"><span class="dot"></span></span>`) {
		t.Errorf("session page lacks the bare stream dot")
	}
	if strings.Contains(page, "sse-label") {
		t.Errorf("session page still carries the stream indicator's text label")
	}

	reg := register(t, f.dir, "noharness", f.project, time.Now())
	reg.Harness = ""
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.src.setPage("noharness", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "plain")})
	f.d.Index().Refresh()
	_, plain := get(t, f.base+"/s/noharness")
	// The session rail lists every session with its own harness badge; the
	// head is what comes before it.
	plain, _, _ = strings.Cut(plain, `id="rail"`)
	if strings.Contains(plain, `class="host`) || !strings.Contains(plain, `id="sse"`) {
		t.Errorf("a page without a harness should keep the bare dot and no badge")
	}
}

// TestSessionPagesCarryScriptHooks pins the elements and attributes the page
// script and stylesheet read: the board column, the drawer that holds the
// panels, the tab bar and its dots, the layout menu, the composer and its
// interrupt badge, subagent badge, and background-task badge, its mention layer behind the message
// box and the file-mention module loaded ahead of the page script, the lead's
// phase and running subagent counts on the convlive region, the stream
// indicators, and the banner.
func TestSessionPagesCarryScriptHooks(t *testing.T) {
	f := startPageFixture(t)
	code, live := get(t, f.base+"/s/livesession?frame=1")
	live = frameDependencies(t, live)
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d: %s", code, live)
	}
	for _, want := range []string{
		`<div id="bcol" class="bcol"><div id="r-board" class="region r-board" data-region="board"`,
		`id="tabbar"`, `data-tab="convo"`, `data-tab="board"`, `<span class="tdot"></span>`,
		`id="ctext"`, `id="csend"`, `aria-label="send"`, `id="sse"`, `id="dash-banner"`,
		`<div id="composer" class="composer"><div class="cbadges"><button type="button" class="ibadge" id="ibadge"`, `<span class="ilabel">Interrupt</span>`,
		`</button><span class="sbwrap"><button type="button" class="sbadge" id="sbadge" title="running subagents: the lead&#39;s own, and (&#43;) those they started" aria-haspopup="dialog" aria-expanded="false" aria-controls="agpop" hidden><span class="spin"></span><svg class="i" aria-hidden="true"><use href="#i-agents"/></svg><span class="slabel"></span><span class="sword">running</span></button><span class="bgbadge" id="bgbadge" title="background tasks the harness runs: shells, monitors, and background subagents" hidden><svg class="i" aria-hidden="true"><use href="#i-clock"/></svg><span class="blabel"></span></span><span class="bgbadge" id="monbadge" title="running monitors" hidden><svg class="i" aria-hidden="true"><use href="#i-clock"/></svg><span class="blabel"></span></span></span></div>`,
		`<div class="conv clive" data-phase="replying" data-sub="0" data-subdeep="0" data-bg="0" data-monitors="0">`,
		`<div class="cfield"><div class="chl" id="chl" aria-hidden="true"></div><textarea id="ctext"`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live session page lacks %q", want)
		}
	}
	if m, d := strings.Index(live, "window.wsMention = {"), strings.Index(live, `<script src="/s/dashboard-`); m < 0 || d < m {
		t.Errorf("live session page does not load the mention module ahead of the page script (module at %d, page script at %d)", m, d)
	}
	// The composer sends on a key or the thin icon-only send button; the
	// copy button and the lead strip are gone, and the agents tree lives in
	// the popover the running-agent badge opens, after the badges and inside
	// the composer, with no splitter over the conversation.
	for _, gone := range []string{`id="ccopy"`, `data-region="lead"`} {
		if strings.Contains(live, gone) {
			t.Errorf("live session page still carries %q", gone)
		}
	}
	if i, j, k := strings.Index(live, `<div class="agpop" id="agpop" role="dialog" aria-label="Agents" hidden><div id="r-tree" class="region r-tree" data-region="tree"`), strings.Index(live, `<div id="composer" class="composer">`), strings.Index(live, `id="sbadge"`); i < 0 || j < i || k < j {
		t.Errorf("the agents tree is not in the island above the composer badge")
	}
	for _, want := range []string{
		`<div id="agcompact" class="agview agcompact" role="button" tabindex="0" aria-label="Open full agent tree" title="Open full agent tree" aria-controls="agpop" aria-expanded="false">`,
		`<div class="hud-tree-scroll"><div class="hud-canvas"><svg class="hud-lines" aria-hidden="true"></svg><div class="hud-roots"></div></div></div>`,
		`<div id="hud-tooltip" class="hud-tooltip" role="tooltip" hidden></div>`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live session page lacks HUD hook %q", want)
		}
	}
	if strings.Contains(live, `id="splitter"`) || !strings.Contains(live, `<div id="convo" class="convo"><div id="r-entries"`) {
		t.Errorf("the conversation wrapper does not hold the entries region without a splitter")
	}
	_, board := get(t, f.base+"/s/boardsession?frame=1")
	board = frameDependencies(t, board)
	if !strings.Contains(board, `<div id="bcol" class="bcol"><div id="r-board"`) {
		t.Errorf("board-first page lacks the board column")
	}
	assertSettingsOwner(t, live, false)
	assertSettingsOwner(t, board, false)
	// Both pages carry the drawer: the panels region sits in its body, out
	// of the board column.
	for name, page := range map[string]string{"live": live, "board-first": board} {
		for _, want := range []string{
			`<div class="drawer" id="drawer">`, `id="drclip"`, `id="drsheet"`, `id="drhandle"`, `class="dr-wait"`, `class="tdot"`,
			`<div class="dr-body" id="drbody"><div id="r-panels" class="region r-panels" data-region="panels"`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s page lacks drawer hook %q", name, want)
			}
		}
		if i, j := strings.Index(page, `data-region="panels"`), strings.Index(page, `id="bcol"`); i < 0 || j < 0 || i > j {
			t.Errorf("%s page: the panels region is not in the drawer ahead of the board column", name)
		}
	}
	// Both pages run the image viewer once, outside any region, and style it.
	for name, page := range map[string]string{"live": live, "board-first": board} {
		if n := strings.Count(page, "wbviewerclose"); n != 1 {
			t.Errorf("%s page carries the image viewer %d times, want once", name, n)
		}
		for _, want := range []string{`.wbv-stage`, `.thumbs img, .bub img { cursor: zoom-in; }`, `id="i-zoom-in"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s page lacks viewer hook %q", name, want)
			}
		}
	}
	// Both pages run the open-question pill and the topic rail beside the
	// answer form.
	for name, page := range map[string]string{"live": live, "board-first": board} {
		if !strings.Contains(page, "data-wb-title") {
			t.Errorf("%s page lacks the open-question pill script", name)
		}
		if !strings.Contains(page, "window.wbRail") {
			t.Errorf("%s page lacks the topic rail script", name)
		}
		if n := strings.Count(page, "if (d._wbDeps) return;"); n != 1 {
			t.Errorf("%s page carries dependency popover script %d times, want once", name, n)
		}
	}
}

// dashboardPageScript pins the intended script mode: session frames load the
// assembled script by URL, while the index frame embeds that script inline.
func dashboardPageScript(t *testing.T, base, page string, external bool) (int, string) {
	t.Helper()
	want, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`<script src="(/s/`+regexp.QuoteMeta(a.data.ScriptFile)+`)"></script>`).FindAllStringSubmatchIndex(page, -1)
	inline := "<script>" + string(want) + "</script>"
	if !external {
		if len(matches) != 0 || strings.Count(page, inline) != 1 {
			t.Fatal("index must inline the assembled dashboard script exactly once, without an external reference")
		}
		return strings.Index(page, inline), string(want)
	}
	if len(matches) != 1 {
		t.Fatalf("session frame has %d content-addressed dashboard scripts, want one", len(matches))
	}
	m := matches[0]
	code, script := get(t, base+page[m[2]:m[3]])
	if code != http.StatusOK {
		t.Fatalf("dashboard script GET = %d", code)
	}
	if script != string(want) || strings.Contains(page, "The dashboard page script, shared") {
		t.Error("session frame must reference the assembled dashboard script, not inline it")
	}
	return m[0], script
}
