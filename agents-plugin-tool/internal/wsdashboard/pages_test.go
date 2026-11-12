package wsdashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// pageFake is a session source that also serves page content.
type pageFake struct {
	*fakeSource
	mu    sync.Mutex
	pages map[string]PageContent
	// boards are the whiteboards ReadClosedTopics renders from.
	boards map[string]*wswhiteboard.State
	// pageReads counts ReadPage calls per reply-id.
	pageReads map[string]int
}

func newPageFake() *pageFake {
	return &pageFake{fakeSource: newFakeSource(), pages: map[string]PageContent{}, boards: map[string]*wswhiteboard.State{}, pageReads: map[string]int{}}
}

func (p *pageFake) ReadPage(reg Registration) (PageContent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pageReads[reg.ReplyID]++
	page, ok := p.pages[reg.ReplyID]
	return page, ok
}

// ReadContacts re-reads a page's contacts, as a ContactSource.
func (p *pageFake) ReadContacts(reg Registration) ([]Contact, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pages[reg.ReplyID].Contacts, true
}

// readCount is how often ReadPage read id.
func (p *pageFake) readCount(id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pageReads[id]
}

// page is id's stored page content.
func (p *pageFake) page(id string) PageContent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pages[id]
}

// putPage replaces page content without touching the session fingerprint,
// as a write no nudge reports and no fingerprint covers.
func (p *pageFake) putPage(id string, page PageContent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pages[id] = page
}

// setPage stores page content and a matching session fingerprint, so the
// index sees a change when the page changes.
func (p *pageFake) setPage(id string, page PageContent) {
	p.mu.Lock()
	p.pages[id] = page
	p.mu.Unlock()
	raw, _ := json.Marshal(page)
	p.set(id, SessionData{HasBoard: page.HasBoard, CountPolicy: page.Board.CountPolicy, Fingerprint: string(raw)})
}

func writeSnapshot(t *testing.T, dir, id string, snap *wslive.Snapshot) {
	t.Helper()
	snap.ReplyID = id
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	path := wslive.SnapshotPath(SessionDir(dir, id))
	if err := os.WriteFile(path+".tmp", raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

// boardRegions renders a small board through the whiteboard renderer: one
// topic holding body, which the dashboard shows in a topic-body frame.
func boardRegions(t *testing.T, body string) wswhiteboard.Regions {
	t.Helper()
	state := &wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "Plan", Body: body, Changed: 1}}}
	view := wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	return regions
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := testClient().Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

type pageFixture struct {
	dir     string
	project string
	src     *pageFake
	d       *Daemon
	base    string
}

func startPageFixture(t *testing.T, options ...func(*DaemonOptions)) *pageFixture {
	t.Helper()
	f := &pageFixture{dir: t.TempDir(), project: t.TempDir(), src: newPageFake()}
	now := time.Now()
	register(t, f.dir, "livesession", f.project, now)
	register(t, f.dir, "boardsession", f.project, now)
	f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 3, Board: boardRegions(t, "live board text"),
		Contacts:   []Contact{{Address: "id:boardsession", ReplyID: "boardsession", LastExchange: now}},
		MailBodies: map[string]string{"hist-1.json": "mail first line\nmail second line"}})
	f.src.setPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "only the board")})
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "streamed <partial> reply"}}})
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{
		{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "from the board"},
		{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "from the terminal"},
		{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:boardsession", Title: "mail first line", HistoryKey: "hist-1.json"},
		{Kind: wslive.RingMail, Direction: wslive.MailSent, Peer: "gone@machine", Title: "expired mail", HistoryKey: "hist-gone.json"},
	}); err != nil {
		t.Fatal(err)
	}
	// The git badge is off unless a test turns it on: its first read would
	// swap a region the tests that pin a stream's regions do not expect.
	options = append([]func(*DaemonOptions){func(o *DaemonOptions) { o.noGitBadge = true }}, options...)
	f.d = startIndexedDaemon(t, f.dir, f.src, time.Hour, options...)
	f.base = endpointURL(f.d.Endpoint(), "")
	// The daemon's first index refresh runs beside its start; a page asked
	// for before it lands is an unknown session.
	waitFor(t, "the fixture sessions to be indexed", func() bool {
		_, live := f.d.Index().Session("livesession")
		_, board := f.d.Index().Session("boardsession")
		return live && board
	})
	return f
}

func TestSessionPageRendersLiveRegions(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/s/livesession")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d: %s", code, body)
	}
	for _, want := range []string{
		`data-region="tree"`, `data-region="entries"`, `data-region="convlive"`, `data-region="board"`, `data-region="panels"`,
		`id="composer"`, `id="p-settings"`, "live board text",
		// A topic body sits in its sandboxed frame; there is no Now slot.
		`<iframe class="doc-frame" title="Plan" data-doc="Plan"`,
		`id="splitter"`, `data-input="enter"`, `data-input="mod"`,
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
// harness badge, which carries the stream indicator's dot and tooltip with
// no text label; a page without a harness keeps the bare dot alone.
func TestSessionHeadLeadsWithTheProjectAndLightsTheHarnessBadge(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession")
	name := filepath.Base(f.project)
	for _, head := range []string{
		`<span class="meta">` + name + `</span><span class="meta sname"`,
		`>livesession</button>`,
		`<span class="sseq"> · #3</span></span><span class="host sse" title="live updates"><span class="dot"></span>claude</span>`,
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
	code, live := get(t, f.base+"/s/livesession")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d: %s", code, live)
	}
	for _, want := range []string{
		`<div id="bcol" class="bcol"><div id="r-board" class="region r-board" data-region="board"`,
		`id="tabbar"`, `data-tab="convo"`, `data-tab="board"`, `<span class="tdot"></span>`,
		`class="gmenu gcard"`, `data-layout="auto"`, `data-layout="split"`, `data-layout="tabs"`,
		`id="ctext"`, `id="cnote"`, `id="csend"`, `aria-label="send"`, `id="sse"`, `id="dash-banner"`,
		`<div id="composer" class="composer"><div class="cbadges"><button type="button" class="ibadge" id="ibadge"`, `<span class="ilabel">Interrupt</span>`,
		`</button><span class="sbwrap"><span class="sbadge" id="sbadge" title="running subagents: the lead&#39;s own, and (&#43;) those they started" hidden><span class="spin"></span><svg class="i" aria-hidden="true"><use href="#i-agents"/></svg><span class="slabel"></span></span><span class="bgbadge" id="bgbadge" title="background tasks the harness runs: shells, monitors, and background subagents" hidden><svg class="i" aria-hidden="true"><use href="#i-clock"/></svg><span class="blabel"></span></span></span></div>`,
		`<div class="conv clive" data-phase="replying" data-sub="0" data-subdeep="0">`,
		`<div class="cfield"><div class="chl" id="chl" aria-hidden="true"></div><textarea id="ctext"`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live session page lacks %q", want)
		}
	}
	if m, d := strings.Index(live, "window.wsMention = {"), strings.Index(live, "--- composer file mentions"); m < 0 || d < m {
		t.Errorf("live session page does not load the mention module ahead of the page script (module at %d, page script at %d)", m, d)
	}
	// The composer sends on a key or the thin icon-only send button; the
	// copy button and the lead strip are gone, and the splitter sits between the tree and the
	// conversation.
	for _, gone := range []string{`id="ccopy"`, `data-region="lead"`} {
		if strings.Contains(live, gone) {
			t.Errorf("live session page still carries %q", gone)
		}
	}
	if i, j, k := strings.Index(live, `data-region="tree"`), strings.Index(live, `id="splitter"`), strings.Index(live, `<div id="convo" class="convo"><div id="r-entries"`); i < 0 || j < i || k < j {
		t.Errorf("splitter is not between the tree and the conversation wrapper holding the entries region")
	}
	_, board := get(t, f.base+"/s/boardsession")
	if !strings.Contains(board, `<div id="bcol" class="bcol"><div id="r-board"`) {
		t.Errorf("board-first page lacks the board column")
	}
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
	}
}

// TestPagesCarryTheTracker pins the tracker renderer on every page: its
// stylesheet after the dashboard's, its script after the string table and
// before the page script that mounts it, and the header slot its quick
// switcher goes into.
func TestPagesCarryTheTracker(t *testing.T) {
	f := startPageFixture(t)
	for name, path := range map[string]string{"live": "/s/livesession", "board-first": "/s/boardsession", "index": "/"} {
		code, page := get(t, f.base+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, code)
		}
		css, trackerCSS := strings.Index(page, ".dwrap {"), strings.Index(page, ".tk.tk-drawer {")
		if css < 0 || trackerCSS < css {
			t.Errorf("%s page: the tracker stylesheet is missing or ahead of the dashboard's", name)
		}
		i18n, tracker, script := strings.Index(page, "window.wsT = function"), strings.Index(page, "window.WsTracker = { mount: mount }"), strings.Index(page, "The dashboard page script, shared")
		if i18n < 0 || tracker < i18n || script < tracker {
			t.Errorf("%s page: the tracker script is missing or not between the string table and the page script", name)
		}
		// The whiteboard's own pages leave the tracker strings out; the
		// dashboard pages that mount the tracker must keep them.
		if !strings.Contains(page, `"js.tk.`) {
			t.Errorf("%s page: the string table lacks the tracker strings (js.tk.*)", name)
		}
		if strings.Count(page, `<span class="qsslot" id="qsslot"></span>`) != 1 {
			t.Errorf("%s page lacks the quick-switcher slot", name)
		}
		// The wiki in the Items panel: its stylesheet block, the renderer's
		// switch and card, the adapter's wiki methods, and the wiki strings.
		for _, hook := range []string{
			".tk .wk-sw {", ".tk .card > .cbody.wk-body {", ".tk a.wlink {", ".tk .wk-vs {",
			`attr('act', 'wk-mode')`, "function cardWiki(", "function wkRefused(", `'a[data-wiki][data-page]'`,
			`tkUrl(id, "wiki")`, `"/save"), req,`, `"js.tk.wk.tab"`,
		} {
			if !strings.Contains(page, hook) {
				t.Errorf("%s page lacks the wiki hook %q", name, hook)
			}
		}
		// In the header, at its end: before the gear menu on the index
		// page, and last on a session page, whose settings are a drawer card.
		tail := `id="qsslot"></span></div>`
		if name == "index" {
			tail = `id="qsslot"></span><span class="gearwrap">`
		}
		if i, j := strings.Index(page, `<div class="top">`), strings.Index(page, tail); i < 0 || j < i {
			t.Errorf("%s page: the quick-switcher slot is not at the header's end", name)
		}
	}
}

func TestBoardFirstPageHasNoLayoutControl(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/s/boardsession")
	if code != http.StatusOK {
		t.Fatalf("GET /s/boardsession = %d", code)
	}
	if !strings.Contains(body, "only the board") || !strings.Contains(body, `data-mode="board"`) {
		t.Errorf("board-first page lacks its board")
	}
	// The settings card is there for the theme and language items alone:
	// no layout or input choice, which only a live layout has.
	for _, present := range []string{`id="p-settings"`, `<option value="ko">`, `<option value="">`} {
		if !strings.Contains(body, present) {
			t.Errorf("board-first page lacks %q", present)
		}
	}
	for _, absent := range []string{`<button data-layout=`, `<button data-input=`, `id="composer"`, `id="splitter"`, `id="convo"`, `data-region="entries"`, `data-region="convlive"`, `id="tabbar"`, `id="ibadge"`} {
		if strings.Contains(body, absent) {
			t.Errorf("board-first page carries %q", absent)
		}
	}
}

func TestIndexPageListsSessions(t *testing.T) {
	f := startPageFixture(t)
	code, body := get(t, f.base+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{`href="/s/livesession"`, `href="/s/boardsession"`, filepath.Base(f.project), `class="phase replying"`, "no live state"} {
		if !strings.Contains(body, want) {
			t.Errorf("index lacks %q", want)
		}
	}
}

// TestPagesRefuseFraming pins the clickjacking guard: every daemon HTML page
// (the index, a session page, and the file view's rendered and not-viewable
// pages) refuses to be framed, since a framed page's posts pass the
// same-origin guards.
func TestPagesRefuseFraming(t *testing.T) {
	f := startPageFixture(t)
	writeFile(t, filepath.Join(f.project, "a.md"), "# A\n")
	writeFile(t, filepath.Join(f.project, "blob.bin"), "BIN\x00")
	for _, path := range []string{"/", "/s/livesession", "/s/boardsession", "/s/livesession/file?path=a.md", "/s/livesession/file?path=blob.bin"} {
		resp, err := testClient().Get(f.base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		h := resp.Header
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(h.Get("Content-Type"), "text/html") {
			t.Errorf("GET %s = %d %q", path, resp.StatusCode, h.Get("Content-Type"))
			continue
		}
		if h.Get("X-Frame-Options") != "DENY" || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("GET %s is frameable: X-Frame-Options %q, CSP %q", path, h.Get("X-Frame-Options"), h.Get("Content-Security-Policy"))
		}
	}
}

// TestSessionPageFramesOnlyItsOwnOrigin pins the session page's frame-src:
// a frame on it (a preview's sandboxed document) cannot navigate itself to
// another origin, where it would carry off its capability.
func TestSessionPageFramesOnlyItsOwnOrigin(t *testing.T) {
	f := startPageFixture(t)
	resp, err := testClient().Get(f.base + "/s/livesession")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if csp := resp.Header.Get("Content-Security-Policy"); resp.StatusCode != http.StatusOK || csp != "frame-src 'self'; frame-ancestors 'none'" {
		t.Errorf("session page = %d, policy %q", resp.StatusCode, csp)
	}
}

func TestUnknownPagesAreNotFound(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/s/nosuchsession", "/s/NOT-VALID", "/s/livesession/more", "/elsewhere", "/s/nosuchsession/events"} {
		if code, _ := get(t, f.base+path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
	// A registered session whose page cannot be read is not served.
	f.src.mu.Lock()
	delete(f.src.pages, "boardsession")
	f.src.mu.Unlock()
	if code, _ := get(t, f.base+"/s/boardsession"); code != http.StatusNotFound {
		t.Errorf("unreadable page = %d, want 404", code)
	}
	// The page has one URL, since it loads its assets by relative path.
	noFollow := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if resp, err := noFollow.Get(f.base + "/s/livesession/"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/s/livesession" {
			t.Errorf("trailing slash = %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// A stream answers GET only.
	for _, path := range []string{"/events", "/s/livesession/events"} {
		req, _ := http.NewRequest(http.MethodHead, f.base+path, nil)
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("HEAD %s = %d", path, resp.StatusCode)
		}
	}
	resp, err := testClient().Post(f.base+"/s/livesession", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST page = %d", resp.StatusCode)
	}
}

func TestMermaidAssetIsServedBesideThePages(t *testing.T) {
	f := startPageFixture(t)
	name, data := wswhiteboard.MermaidAsset()
	resp, err := testClient().Get(f.base + "/s/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(body) != len(data) || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("mermaid asset = %d, %d bytes, %q", resp.StatusCode, len(body), resp.Header.Get("Cache-Control"))
	}
	// The page names it by relative path.
	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `window.wbMermaidFile = "`+name+`"`) {
		t.Errorf("page does not name the mermaid file %s relatively", name)
	}
}

func TestPagesRefuseANonLoopbackHost(t *testing.T) {
	f := startPageFixture(t)
	for _, path := range []string{"/", "/events", "/s/livesession", "/s/livesession/events"} {
		req, _ := http.NewRequest(http.MethodGet, f.base+path, nil)
		req.Host = "evil.example"
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s with a foreign Host = %d", path, resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("GET %s sent a CORS header", path)
		}
	}
}

// sseEvent is one parsed SSE event.
type sseEvent struct {
	name string
	data string
}

func readEvents(t *testing.T, r *bufio.Reader, out chan<- sseEvent) {
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			close(out)
			return
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if ev.name != "" {
				out <- ev
			}
			ev = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			ev.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func nextRegion(t *testing.T, events <-chan sseEvent, timeout time.Duration) (regionEvent, bool) {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("stream closed")
		}
		// Every stream's first event names the assets identity, which
		// assets_id_test.go pins; region readers skip it.
		if ev.name == "assets" {
			return nextRegion(t, events, timeout)
		}
		if ev.name != "region" {
			return regionEvent{Name: "event:" + ev.name}, true
		}
		var re regionEvent
		if err := json.Unmarshal([]byte(ev.data), &re); err != nil {
			t.Fatal(err)
		}
		return re, true
	case <-time.After(timeout):
		return regionEvent{}, false
	}
}

var regionHashPattern = regexp.MustCompile(`data-region="([a-z]+)" data-h="([0-9a-f]+)"`)

// heldOf lists the region hashes a rendered page holds, as the page script
// sends them.
func heldOf(page string) string {
	var parts []string
	for _, m := range regionHashPattern.FindAllStringSubmatch(page, -1) {
		parts = append(parts, m[1]+"."+m[2])
	}
	return strings.Join(parts, ",")
}

func TestStreamSendsOnlyChangedRegions(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/boardsession")
	modeHash := newRegion(modeBoard).Hash
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/boardsession/events?h=" + heldOf(page) + ",mode." + modeHash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)

	// The page is current: nothing is sent.
	if ev, ok := nextRegion(t, events, 400*time.Millisecond); ok {
		t.Fatalf("a current page received %+v", ev)
	}

	// The board changes; a nudge sends exactly the board region.
	f.src.setPage("boardsession", PageContent{HasBoard: true, Seq: 2, Board: boardRegions(t, "the board changed")})
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	var got []string
	for {
		ev, ok := nextRegion(t, events, 800*time.Millisecond)
		if !ok {
			break
		}
		got = append(got, ev.Name)
		if ev.Name == regionBoard && !strings.Contains(ev.HTML, "the board changed") {
			t.Errorf("board event lacks the new board: %s", ev.HTML)
		}
	}
	// The head carries the board's sequence number, so it changes too.
	if strings.Join(got, ",") != "head,board" {
		t.Fatalf("regions sent = %v, want head,board", got)
	}

	// The session leaves the index: the stream says so and ends.
	if err := os.RemoveAll(SessionDir(f.dir, "boardsession")); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	if ev, ok := nextRegion(t, events, 2*time.Second); !ok || ev.Name != "event:gone" {
		t.Fatalf("after removal got %+v (%v)", ev, ok)
	}
}

func TestStreamSendsTheModeWhenTheAdapterAppears(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/boardsession")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/boardsession/events?h=" + heldOf(page) + ",mode." + newRegion(modeBoard).Hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)

	writeSnapshot(t, f.dir, "boardsession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000})
	f.d.broker.wake("boardsession")
	ev, ok := nextRegion(t, events, 2*time.Second)
	if !ok || ev.Name != regionMode || ev.HTML != modeLive {
		t.Fatalf("first event = %+v (%v), want the live mode", ev, ok)
	}
}

func TestRegionHashIgnoresRenderTime(t *testing.T) {
	// Times render as fixed UTC text; the page script formats them, so a
	// re-render of unchanged state hashes the same.
	view := SessionView{ReplyID: "x", Adapter: true, Lead: LeadView{Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: testNow.UnixMilli()}}}}
	a, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := renderSessionRegions(view, nil)
	if a[regionTree].Hash != b[regionTree].Hash {
		t.Fatal("tree hash changed between identical renders")
	}
	if !strings.Contains(string(a[regionTree].HTML), `data-f="since"`) {
		t.Errorf("tree has no client-formatted time: %s", a[regionTree].HTML)
	}
}

func TestRenderedPermissionWaitAndEndedSessions(t *testing.T) {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission,
		Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "rm -rf <build>", StartedAt: now.UnixMilli()}}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The conversation says it in words; the tree's one-line lead row shows
	// the approval phase with the tool and its arguments. No lead region
	// exists to say it a third time.
	conv := string(set[regionConvLive].HTML)
	if !strings.Contains(conv, "waiting for approval in the terminal") || !strings.Contains(conv, "rm -rf &lt;build&gt;") {
		t.Errorf("conversation region lacks the escaped permission wait: %s", conv)
	}
	tree := string(set[regionTree].HTML)
	for _, want := range []string{`class="phase permission"`, `<span class="tn" title="Bash">Bash</span>`, "rm -rf &lt;build&gt;"} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree region lacks %q in the permission phase: %s", want, tree)
		}
	}
	if _, ok := set["lead"]; ok {
		t.Error("a lead region is still rendered")
	}

	old := liveSession("oldend", "/a", now.Add(-48*time.Hour))
	old.EndedAt = stampFor(now.Add(-48 * time.Hour))
	idx, err := renderIndexRegions(buildIndexView([]indexInput{
		{Session: liveSession("livenow", "/a", now)},
		{Session: endedSession("recentend", "/a", now)},
		{Session: old},
	}, now, "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(idx[regionSessions].HTML)
	for _, want := range []string{`href="/s/livenow"`, `<div class="endedhead">ended</div>`, `class="sess ended" href="/s/recentend"`, "1 sessions ended more than a day ago"} {
		if !strings.Contains(html, want) {
			t.Errorf("index region lacks %q: %s", want, html)
		}
	}
	if strings.Contains(html, "oldend") {
		t.Errorf("a session ended two days ago is listed")
	}
}

// TestRenderedCompaction pins the compaction rows: while the lead compacts,
// the conversation's live region shows one running compaction line in place
// of the generic activity line and the tree's lead row shows the phase; a
// landed or failed compaction is a ring entry line naming its trigger, and a
// failed one shows the harness's error when the ring entry carries it.
func TestRenderedCompaction(t *testing.T) {
	now := testNow
	started := now.Add(-3 * time.Second).UnixMilli()
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseCompacting, PhaseStartedAt: started}, TurnStartedAt: started}
	ring := []wslive.RingRecord{
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, Trigger: wslive.TriggerAuto}, Name: "20261107T000000.000000001Z-a1.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, Trigger: wslive.TriggerManual}, Name: "20261107T000000.000000002Z-a2.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, CompactFailed: true}, Name: "20261107T000000.000000003Z-a3.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, CompactFailed: true, Text: "Conversation too long"}, Name: "20261107T000000.000000004Z-a4.json", At: now},
	}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Ring: ring, Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	conv := string(set[regionConvLive].HTML)
	if !strings.Contains(conv, `<div class="sysline compaction running">`) || !strings.Contains(conv, "compacting context…") || !strings.Contains(conv, `data-ms="`+strconv.FormatInt(started, 10)+`"`) {
		t.Errorf("live region lacks the running compaction line: %s", conv)
	}
	if strings.Contains(conv, `class="activity"`) {
		t.Errorf("live region also shows the generic activity line: %s", conv)
	}
	if tree := string(set[regionTree].HTML); !strings.Contains(tree, `<span class="phase compacting">compacting</span>`) {
		t.Errorf("tree's lead row lacks the compacting phase: %s", tree)
	}
	entries := string(set[regionEntries].HTML)
	for _, want := range []string{
		`<div class="sysline compaction" data-entry="20261107T000000.000000001Z-a1.json"><span class="mk">✓</span>context compacted (auto)`,
		`<div class="sysline compaction" data-entry="20261107T000000.000000002Z-a2.json"><span class="mk">✓</span>context compacted (manual)`,
		`<div class="sysline compaction fail" data-entry="20261107T000000.000000003Z-a3.json"><span class="mk">✗</span><span class="nt">context compaction failed</span><time`,
		`<div class="sysline compaction fail" data-entry="20261107T000000.000000004Z-a4.json"><span class="mk">✗</span><span class="nt">context compaction failed: Conversation too long</span><time`,
	} {
		if !strings.Contains(entries, want) {
			t.Errorf("entries region lacks %q: %s", want, entries)
		}
	}
}

func TestToolNameAndShortID(t *testing.T) {
	for in, want := range map[string]string{
		"mcp__plugin_ws_ws__tickets_create_empty": "tickets_create_empty",
		"mcp__srv__do": "do",
		"mcp__broken":  "mcp__broken",
		"Bash":         "Bash",
	} {
		if got := toolName(in); got != want {
			t.Errorf("toolName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := shortID("0123456789abcdef0"); got != "012345" {
		t.Errorf("shortID = %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID short = %q", got)
	}
}

// TestTreeRowsAreDense pins the compact tree: the lead's tool shows with its
// MCP prefix stripped (full name in the title) on the one summary row, a
// subagent's caption sits on its own row, a thinking agent with no calls yet
// shows its two placeholder slots (no live row repeating its pill), and a
// lone call reads "1 call".
func TestTreeRowsAreDense(t *testing.T) {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{
		{Running: &wslive.RunningView{ToolName: "mcp__plugin_ws_ws__tickets_create_empty", ArgsPreview: "title x", StartedAt: now.UnixMilli()}},
		{ToolNames: []string{"Read"}, CallCount: 1, EndedAt: now.UnixMilli()},
	}}}
	agent := wslive.Agent{ID: "0123456789abcdef0", Status: wslive.StatusRunning, StartedAt: now.UnixMilli(),
		Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: now.UnixMilli(), Caption: "ws:effort-high"}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead, agent), Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	tree := string(set[regionTree].HTML)
	for _, want := range []string{
		`<span class="tn" title="mcp__plugin_ws_ws__tickets_create_empty">tickets_create_empty</span>`,
		`<span class="acap" title="ws:effort-high">ws:effort-high</span>`,
		`>agent 012345</span>`, "1 call ",
	} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree lacks %q: %s", want, tree)
		}
	}
	if strings.Contains(tree, "1 calls") || strings.Contains(tree, "writing") {
		t.Errorf("tree carries a plural or a live row repeating the phase: %s", tree)
	}
	// The thinking agent's live list holds only its one placeholder.
	if ph := `<li class="brow ph"><span class="mk">-</span></li>`; !strings.Contains(tree, `<ol class="brows live bguide">`+ph+`</ol>`) {
		t.Errorf("thinking agent has a live row: %s", tree)
	}
}

// TestPageMarksASessionHandedToATerminal: the page of an ended terminal
// session carries the handed mark once its directory holds a lead record
// that was handed off, and not for an agent conversation's record.
func TestPageMarksASessionHandedToATerminal(t *testing.T) {
	f := startPageFixture(t)
	now := time.Now()
	reg := Registration{ReplyID: "handedsession", Project: f.project, Harness: "claude", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now), EndedAt: stampFor(now)}
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.src.setPage("handedsession", PageContent{})
	f.d.Index().Refresh()
	marked := func() bool {
		t.Helper()
		code, page := get(t, f.base+"/s/handedsession")
		if code != http.StatusOK {
			t.Fatalf("GET the session page = %d", code)
		}
		return strings.Contains(page, ` data-handed="1"`)
	}
	record := func(raw string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(SessionDir(f.dir, "handedsession"), AgentRecordFileName), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if marked() {
		t.Error("a session with no agent record carries the handed mark")
	}
	record(`{"handed_off_at":"2026-11-01T00:00:00Z"}`)
	if marked() {
		t.Error("an agent conversation's record marks the page")
	}
	record(`{"lead":true,"handed_off_at":"2026-11-01T00:00:00Z"}`)
	if !marked() {
		t.Error("a handed-off lead session's page lacks the handed mark")
	}
}
