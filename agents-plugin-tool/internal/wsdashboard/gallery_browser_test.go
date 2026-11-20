package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// galleryBoardState is the gallery acceptance board: 35 closed topics (the
// board serves 30, the closed route the rest), a "deep" topic whose
// answered policy (item 1) stands in its history far above its long log,
// and open topics with
// body and panel frames; "epsilon"'s last log line references item 1, so
// a click on it navigates into "deep" wherever it stands. closed closes
// "gamma" as a later board update would.
func galleryBoardState(t *testing.T, closed bool) *wswhiteboard.State {
	t.Helper()
	st := &wswhiteboard.State{}
	apply := func(text string) {
		t.Helper()
		if _, problems := wswhiteboard.Apply(st, text, wswhiteboard.Env{Now: time.Now()}); len(problems) != 0 {
			t.Fatalf("fixture Apply: %v", problems)
		}
	}
	for i := 0; i < 35; i++ {
		apply(fmt.Sprintf(`<wb:topic name="archive%03d"><wb:body>Archived body %d.</wb:body><wb:close result="Archived"/></wb:topic>`, i, i))
	}
	apply(`<wb:topic name="deep" label="Deep topic"><wb:body># Deep topic

Deep body frame.</wb:body><wb:add class="policy"><wb:title>Deep target policy</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	apply(`<wb:answer id="1" value="a">pick A</wb:answer>`)
	for i := 0; i < 60; i++ {
		apply(fmt.Sprintf(`<wb:topic name="deep"><wb:log>deep log %02d: a reasonably long line of text that gives the history real height</wb:log></wb:topic>`, i))
	}
	for _, name := range []string{"alpha", "beta", "gamma", "epsilon"} {
		apply(fmt.Sprintf(`<wb:topic name="%s" label="%s topic"><wb:body># %s body

Frame identity sentinel for %s.</wb:body><wb:panel recap="%s panel">Panel payload for %s.</wb:panel><wb:log>%s first log line.</wb:log></wb:topic>`, name, name, name, name, name, name, name))
	}
	apply(`<wb:topic name="epsilon"><wb:log>the deep policy {{1}} decides this</wb:log></wb:topic>`)
	if closed {
		apply(`<wb:topic name="gamma"><wb:close result="Done"/></wb:topic>`)
	}
	return st
}

// TestGalleryBrowser drives the paged docking gallery (cards.js over
// gallery.js) in Chromium against a disposable daemon: drag docking from
// the Main Board, the rail on drag start, + with no page, ⌂ unpinning, dot
// reordering, the pin buttons and keyboard, touch taps on the dots, page
// activation by reveal/navigate/update card, inactive pages and the narrow
// Main Board reported out of view with their frames and terminal still
// mounted, closed and missing docked topics, the layout modes at real
// viewports, the docked-topic deep link below its header, and the split
// tree (Phase 2): five-zone drops with their preview, pane moves within
// and across pages, the center swap, divider drags kept across a reload,
// a split collapsing when one pane unpins, the last pane's page going,
// and the docked frames never reloading on the way; and Phase 3: the 300ms
// spring-loaded rail dots during a drag (and a drop on + before then), the
// wheel bounce-snap gate on the hovered pane of a split page, at the
// narrow Main Board's top and rendered bottom (inert with no docked page),
// the damped micro-bounce at the first page's top and +'s bottom, the
// refractory lock and the idle spring-back, terminals and frames never
// charging it, touch scrolling never charging it, a docked explorer's
// own list scrolling natively before its end pulls, the narrow board
// drag kept on the board after the last page went while + showed, and
// the wheel handler's cost. The
// terminal capability is synthetic (no shell starts); the terminal list
// and socket are routed in the browser.
func TestGalleryBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	f := startPageFixture(t, func(o *DaemonOptions) { o.Holders = &stateFilterTerminalHost{newFakeHolders()} })
	// The project's files fill a docked explorer's list past its height, so
	// the wheel over it scrolls natively before the gate engages.
	for i := 0; i < 120; i++ {
		if err := os.WriteFile(filepath.Join(f.project, fmt.Sprintf("file%03d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}})
	update, ok := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{{Kind: "log", Topic: "beta", EntryID: "e1", Field: "text", After: "beta update card marker"}})
	if !ok {
		t.Fatal("effects carrier not built")
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{update}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	seq := 10
	publish := func(closed bool) {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range []string{"livesession", "boardsession"} {
			st := galleryBoardState(t, closed)
			board, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: id}))
			if err != nil {
				t.Error(err)
				return
			}
			// ReadClosedTopics may run concurrently: install an immutable snapshot.
			raw, _ := json.Marshal(st)
			snapshot := &wswhiteboard.State{}
			if err := json.Unmarshal(raw, snapshot); err != nil {
				t.Error(err)
				return
			}
			f.src.mu.Lock()
			f.src.boards[id] = snapshot
			f.src.mu.Unlock()
			seq++
			f.src.setPage(id, PageContent{HasBoard: true, Seq: seq, Board: board})
			f.d.index.Nudge(id, f.project)
		}
		f.d.index.Settle()
	}
	publish(false)
	f.d.Index().Refresh()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		publish(strings.HasSuffix(r.URL.Path, "/close-gamma"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	// WS_GALLERY_CAPTURES=<dir> also writes the live and mockup v4 captures
	// for the side-by-side review (ai-docs/assets/captures/261119-gallery-phase1
	// and, for the split pages, 261119-gallery-phase2).
	mockup, _ := filepath.Abs("../../../ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html")
	if _, err := os.Stat(mockup); err != nil {
		mockup = ""
	}
	cmd := exec.CommandContext(ctx, node, "testdata/gallery.browser.cjs", f.base, control.URL, mockup)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium gallery: %v", err)
	}
}
