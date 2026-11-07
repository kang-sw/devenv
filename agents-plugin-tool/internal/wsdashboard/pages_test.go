package wsdashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
}

func newPageFake() *pageFake {
	return &pageFake{fakeSource: newFakeSource(), pages: map[string]PageContent{}}
}

func (p *pageFake) ReadPage(reg Registration) (PageContent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	page, ok := p.pages[reg.ReplyID]
	return page, ok
}

// setPage stores page content and a matching session fingerprint, so the
// index sees a change when the page changes.
func (p *pageFake) setPage(id string, page PageContent) {
	p.mu.Lock()
	p.pages[id] = page
	p.mu.Unlock()
	raw, _ := json.Marshal(page)
	p.set(id, SessionData{HasBoard: page.HasBoard, Fingerprint: string(raw)})
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

// boardRegions renders a small board through the whiteboard renderer.
func boardRegions(t *testing.T, now string) wswhiteboard.Regions {
	t.Helper()
	view := wswhiteboard.BuildView(&wswhiteboard.State{Seq: 1, Now: now}, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull})
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
		`data-region="tree"`, `data-region="conversation"`, `data-region="board"`, `data-region="panels"`,
		`id="composer"`, `id="gear"`, "live board text",
		`id="splitter"`, `data-input="enter"`, `data-input="mod"`,
		`<span class="src wb">whiteboard</span>`, `<span class="src">terminal</span>`,
		"streamed &lt;partial&gt; reply",
		// The mail whose history entry exists expands to its body; the
		// other shows its first line only.
		`mail second line`, `<div class="mailline sent nobody">`,
		// The peer is served here, so its name links to its page.
		`href="/s/boardsession"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("session page lacks %q", want)
		}
	}
	for _, leak := range []string{"hist-1.json", "hist-gone.json", f.project + "/", "file:"} {
		if strings.Contains(body, leak) {
			t.Errorf("session page carries %q", leak)
		}
	}
}

// TestSessionPagesCarryScriptHooks pins the elements and attributes the page
// script and stylesheet read: the board column that scrolls the board and
// panels together, the tab bar and its dots, the layout menu, the composer,
// the live indicator's label, and the banner.
func TestSessionPagesCarryScriptHooks(t *testing.T) {
	f := startPageFixture(t)
	code, live := get(t, f.base+"/s/livesession")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d: %s", code, live)
	}
	for _, want := range []string{
		`<div id="bcol" class="bcol"><div id="r-board" class="region r-board" data-region="board"`,
		`id="tabbar"`, `data-tab="convo"`, `data-tab="board"`, `<span class="tdot"></span>`,
		`id="gmenu"`, `data-layout="auto"`, `data-layout="split"`, `data-layout="tabs"`,
		`id="ctext"`, `id="cnote"`, `id="csend"`, `aria-label="send"`, `class="sse-label"`, `id="dash-banner"`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live session page lacks %q", want)
		}
	}
	// The composer sends on a key or the thin icon-only send button; the
	// copy button and the lead strip are gone, and the splitter sits between the tree and the
	// conversation.
	for _, gone := range []string{`id="ccopy"`, `data-region="lead"`} {
		if strings.Contains(live, gone) {
			t.Errorf("live session page still carries %q", gone)
		}
	}
	if i, j, k := strings.Index(live, `data-region="tree"`), strings.Index(live, `id="splitter"`), strings.Index(live, `data-region="conversation"`); i < 0 || j < i || k < j {
		t.Errorf("splitter is not between the tree and the conversation regions")
	}
	// The board column holds the panels region too.
	if i, j := strings.Index(live, `id="bcol"`), strings.Index(live, `data-region="panels"`); i < 0 || j < i {
		t.Errorf("panels region is not inside the board column")
	}
	_, board := get(t, f.base+"/s/boardsession")
	if !strings.Contains(board, `<div id="bcol" class="bcol"><div id="r-board"`) {
		t.Errorf("board-first page lacks the board column")
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
	for _, absent := range []string{`id="gear"`, `id="composer"`, `id="splitter"`, `data-region="conversation"`, `id="tabbar"`} {
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
	a, err := renderSessionRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := renderSessionRegions(view)
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
	set, err := renderSessionRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	// The conversation says it in words; the tree's one-line lead row shows
	// the approval phase with the tool and its arguments. No lead region
	// exists to say it a third time.
	conv := string(set[regionConversation].HTML)
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
// subagent's caption sits on its own row, a thinking agent has no live row
// repeating its pill, and a lone call reads "1 call".
func TestTreeRowsAreDense(t *testing.T) {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{
		{Running: &wslive.RunningView{ToolName: "mcp__plugin_ws_ws__tickets_create_empty", ArgsPreview: "title x", StartedAt: now.UnixMilli()}},
		{ToolNames: []string{"Read"}, CallCount: 1, EndedAt: now.UnixMilli()},
	}}}
	agent := wslive.Agent{ID: "0123456789abcdef0", Status: wslive.StatusRunning, StartedAt: now.UnixMilli(),
		Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: now.UnixMilli(), Caption: "ws:effort-high"}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead, agent), Now: now})
	set, err := renderSessionRegions(view)
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
	// The thinking agent's live list stays empty.
	if !strings.Contains(tree, `<ol class="brows live"></ol>`) {
		t.Errorf("thinking agent has a live row: %s", tree)
	}
}
