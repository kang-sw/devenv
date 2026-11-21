package wsdashboard

import (
	"encoding/json"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// pageFake is a session source that also serves page content.
type pageFake struct {
	*fakeSource
	mu    sync.Mutex
	pages map[string]PageContent
	// boards are the whiteboards ReadClosedTopics renders from.
	boards map[string]*wswhiteboard.State
	// pageReads counts ReadPage calls per reply-id; contactReads counts
	// ReadContacts calls.
	pageReads    map[string]int
	contactReads int
	// outside are the files outside the root each board links
	// (ReadOutsideLinks).
	outside map[string][]string
}

func newPageFake() *pageFake {
	return &pageFake{fakeSource: newFakeSource(), pages: map[string]PageContent{}, boards: map[string]*wswhiteboard.State{}, pageReads: map[string]int{}}
}

func (p *pageFake) ReadPage(reg Registration) (PageContent, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pageReads[reg.ReplyID]++
	page, ok := p.pages[reg.ReplyID]
	return page, ok, nil
}

// ReadContacts re-reads a page's contacts, as a ContactSource.
func (p *pageFake) ReadContacts(reg Registration) ([]Contact, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.contactReads++
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
	// Replace as the ws-mcp writer does (with its Windows retry): the
	// daemon may be reading the previous snapshot, which on Windows makes a
	// plain rename over it fail.
	if err := wsstate.AtomicReplaceFile(path+".tmp", path); err != nil {
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
	client := testClient()
	if runtime.GOOS == "windows" {
		// These content checks include cold and dev-asset page renders, which
		// can exceed the health client's 500 ms budget on Windows. Keep a
		// finite render budget without changing the health/control tests.
		client.Timeout = 5 * time.Second
	}
	resp, err := client.Get(url)
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
	// Session insertion precedes its OnChange hook. Wait for the completed
	// refresh too, or a startup wake can invalidate the first shared render
	// after both sessions are already visible.
	waitFor(t, "the fixture sessions' initial refresh to finish", func() bool {
		_, live := f.d.Index().Session("livesession")
		_, board := f.d.Index().Session("boardsession")
		return f.d.Index().Loaded() && live && board
	})
	return f
}
