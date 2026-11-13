package wsdashboard

import (
	"bufio"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// settle is longer than sseMinInterval plus a render: a stream a wake
// reached has rendered by then.
const settle = 700 * time.Millisecond

// openStreamWith opens a stream sending cookie (empty sends none).
func openStreamWith(t *testing.T, url, cookie string) <-chan sseEvent {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream %s = %d", url, resp.StatusCode)
	}
	events := make(chan sseEvent, 64)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	return events
}

// quiet consumes events until none arrives for settle and returns the
// region events seen.
func quiet(t *testing.T, events <-chan sseEvent) []regionEvent {
	t.Helper()
	var got []regionEvent
	for {
		ev, ok := nextRegion(t, events, settle)
		if !ok {
			return got
		}
		got = append(got, ev)
	}
}

// waitRegionHolding reads events until a region named name whose HTML holds
// want arrives.
func waitRegionHolding(t *testing.T, events <-chan sseEvent, name, want string) regionEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ev, ok := nextRegion(t, events, time.Until(deadline))
		if ok && ev.Name == name && strings.Contains(ev.HTML, want) {
			return ev
		}
	}
	t.Fatalf("no %s region holding %q", name, want)
	return regionEvent{}
}

// addSession registers id under project with a board saying text and waits
// for the index to hold it.
func (f *pageFixture) addSession(t *testing.T, id, project, text string) {
	t.Helper()
	register(t, f.dir, id, project, time.Now())
	f.src.setPage(id, PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, text)})
	f.d.Index().Refresh()
	waitFor(t, id+" to be indexed", func() bool { _, ok := f.d.Index().Session(id); return ok })
}

// change gives id's board new text and nudges the index, as a session's
// own write does.
func (f *pageFixture) change(t *testing.T, id, project, text string, seq int) {
	t.Helper()
	page := f.src.page(id)
	page.Seq, page.Board = seq, boardRegions(t, text)
	f.src.setPage(id, page)
	if !f.d.index.Nudge(id, project) {
		t.Fatalf("nudge %s refused", id)
	}
	f.d.index.Settle()
}

// TestWakeRendersOnlyThePagesThatShowTheSession pins the targeted wake: a
// session's change renders its own page, a page listing it as a peer, and
// a page holding it in its agent tree, and never a page that shows none of
// it.
func TestWakeRendersOnlyThePagesThatShowTheSession(t *testing.T) {
	f := startPageFixture(t)
	other := t.TempDir()
	f.addSession(t, "lonersession", other, "alone")
	f.addSession(t, "delegatesess", f.project, "delegate board")
	// livesession lists boardsession as a peer (fixture) and holds
	// delegatesess as a delegate.
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
		Agents: []wslive.Agent{{ID: "a1", Label: "worker", ReplyID: "delegatesess", Status: wslive.StatusRunning}}})

	live := openStream(t, f.base+"/s/livesession/events")
	loner := openStream(t, f.base+"/s/lonersession/events")
	board := openStream(t, f.base+"/s/boardsession/events")
	quiet(t, live)
	quiet(t, loner)
	quiet(t, board)

	step := func(what, id, project, text string, seq int, want map[string]bool) {
		t.Helper()
		before := map[string]int{}
		for _, page := range []string{"livesession", "lonersession", "boardsession"} {
			before[page] = f.src.readCount(page)
		}
		f.change(t, id, project, text, seq)
		time.Sleep(settle)
		for page, rendered := range want {
			if got := f.src.readCount(page) > before[page]; got != rendered {
				t.Errorf("%s: %s rendered = %v, want %v", what, page, got, rendered)
			}
		}
	}
	step("the peer's change", "boardsession", f.project, "board changed", 2,
		map[string]bool{"boardsession": true, "livesession": true, "lonersession": false})
	waitRegionHolding(t, board, regionBoard, "board changed")
	step("the delegate's change", "delegatesess", f.project, "delegate changed", 2,
		map[string]bool{"livesession": true, "lonersession": false, "boardsession": false})
	step("the loner's change", "lonersession", other, "loner changed", 2,
		map[string]bool{"lonersession": true, "livesession": false, "boardsession": false})
	waitRegionHolding(t, loner, regionBoard, "loner changed")
}

// TestUntargetedWakesReachEveryPage pins the wakes that name no session:
// every session's change re-renders the sessions index, a wake naming no
// token re-renders every page, and the tracker's wake reaches every stream
// without a render (its revision rides beside the regions).
func TestUntargetedWakesReachEveryPage(t *testing.T) {
	f := startPageFixture(t)
	other := t.TempDir()
	f.addSession(t, "lonersession", other, "alone")
	index := openStream(t, f.base+"/events")
	live := openStream(t, f.base+"/s/livesession/events")
	loner := openStream(t, f.base+"/s/lonersession/events")
	quiet(t, index)
	quiet(t, live)
	quiet(t, loner)

	indexRenders := f.d.stats.IndexRenders.Load()
	f.change(t, "lonersession", other, "loner changed", 2)
	waitFor(t, "the index to re-render", func() bool { return f.d.stats.IndexRenders.Load() > indexRenders })

	quiet(t, loner)
	liveReads, lonerReads := f.src.readCount("livesession"), f.src.readCount("lonersession")
	f.d.broker.wake("")
	waitFor(t, "every page to re-render", func() bool {
		return f.src.readCount("livesession") > liveReads && f.src.readCount("lonersession") > lonerReads
	})

	time.Sleep(settle)
	renders, signals := f.d.stats.SessionRenders.Load()+f.d.stats.IndexRenders.Load(), f.d.stats.Signals.Load()
	f.d.TrackerChanged()
	waitFor(t, "the tracker wake to reach every stream", func() bool { return f.d.stats.Signals.Load() >= signals+3 })
	time.Sleep(settle)
	if got := f.d.stats.SessionRenders.Load() + f.d.stats.IndexRenders.Load(); got != renders {
		t.Errorf("the tracker wake rendered %d pages, want none", got-renders)
	}
}

// TestGitWakeReachesThePagesUnderItsRoot: a git read wakes the pages of
// sessions under that root, and not a page under another root.
func TestGitWakeReachesThePagesUnderItsRoot(t *testing.T) {
	git := newFakeGit()
	f := startPageFixture(t, gitBadgeOn(git))
	f.d.git.gap = 0
	other := t.TempDir()
	f.addSession(t, "lonersession", other, "alone")
	live := openStream(t, f.base+"/s/livesession/events")
	loner := openStream(t, f.base+"/s/lonersession/events")
	expectRead(t, git, f.project)
	expectRead(t, git, other)
	quiet(t, live)
	quiet(t, loner)

	lonerReads := f.src.readCount("lonersession")
	if !f.d.git.refresh(f.project) {
		t.Fatal("refresh found no watched root")
	}
	expectRead(t, git, f.project)
	if ev, ok := nextRegion(t, live, 3*time.Second); !ok || ev.Name != regionGit {
		t.Fatalf("live page after its root's read got %+v (%v), want the git region", ev, ok)
	}
	time.Sleep(settle)
	if got := f.src.readCount("lonersession"); got != lonerReads {
		t.Errorf("a read of another root rendered the loner's page %d times", got-lonerReads)
	}
}

// TestStreamsOfOnePageShareOneRender pins the shared render: the streams of
// one session, language, and theme take one render per change; a stream
// with another theme gets the topic frames in its own theme; and a stream
// that filters its regions never strips them from the render it shares.
func TestStreamsOfOnePageShareOneRender(t *testing.T) {
	f := startPageFixture(t)
	// The filtered stream opens first, so the render it shares is its own.
	filtered := openStream(t, f.base+"/s/livesession/events?regions=entries,convlive")
	quiet(t, filtered)
	a := openStream(t, f.base+"/s/livesession/events")
	if got := quiet(t, a); !hasRegion(got, regionBoard) || !hasRegion(got, regionHead) {
		t.Fatalf("a full stream sharing a filtered stream's render got %v, want every region", regionNames(got))
	}
	b := openStream(t, f.base+"/s/livesession/events")
	quiet(t, b)
	dark := openStreamWith(t, f.base+"/s/livesession/events", themeCookie+"=dark")
	if got := quiet(t, dark); !hasRegion(got, regionBoard) {
		t.Fatalf("the dark stream got %v, want every region", regionNames(got))
	}

	// ThemeFrames pins a frame's document to the theme this way.
	darkFrame := `srcdoc="` + template.HTMLEscapeString(`<!DOCTYPE html><html data-theme="dark">`)
	renders := f.d.stats.SessionRenders.Load()
	f.change(t, "livesession", f.project, "shared change", 4)
	for _, s := range []<-chan sseEvent{a, b} {
		ev := waitRegionHolding(t, s, regionBoard, "shared change")
		if strings.Contains(ev.HTML, darkFrame) {
			t.Errorf("a default-theme stream got dark frames")
		}
	}
	ev := waitRegionHolding(t, dark, regionBoard, "shared change")
	if !strings.Contains(ev.HTML, darkFrame) {
		t.Errorf("the dark stream's frames are not dark:\n%.400s", ev.HTML)
	}
	for _, ev := range quiet(t, filtered) {
		if ev.Name != regionEntries && ev.Name != regionConvLive {
			t.Errorf("the filtered stream got region %s", ev.Name)
		}
	}
	time.Sleep(settle)
	// One render for the default theme, which three streams share, and one
	// for the dark one.
	if got := f.d.stats.SessionRenders.Load() - renders; got != 2 {
		t.Errorf("the change took %d renders, want 2", got)
	}
}

func hasRegion(events []regionEvent, name string) bool {
	for _, ev := range events {
		if ev.Name == name {
			return true
		}
	}
	return false
}

func regionNames(events []regionEvent) []string {
	var names []string
	for _, ev := range events {
		names = append(names, ev.Name)
	}
	return names
}

// withRefresh runs the fixture's streams on a short refresh tick.
func withRefresh(o *DaemonOptions) { o.refreshInterval = 300 * time.Millisecond }

// waitSkipping waits until the refresh tick has skipped at least twice
// more, so the page's refresh stamp is known and the tick skips.
func (f *pageFixture) waitSkipping(t *testing.T) {
	t.Helper()
	skips := f.d.stats.RefreshSkips.Load()
	waitFor(t, "the refresh tick to skip", func() bool { return f.d.stats.RefreshSkips.Load() >= skips+2 })
}

// TestRefreshTickSkipsUntilAKeyPartChanges pins the refresh skip: a tick
// over unchanged state reads nothing, and a tick re-renders after a new
// ticket file, after an in-place edit of a listed ticket file, and on a
// liveness change that follows the clock alone; none of them is a wake.
func TestRefreshTickSkipsUntilAKeyPartChanges(t *testing.T) {
	f := startPageFixture(t, withRefresh)
	ready := filepath.Join(t.TempDir(), "ready")
	if err := os.MkdirAll(ready, 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(ready, "a.md")
	if err := os.WriteFile(ticket, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	watch := []string{ready, ticket}
	f.src.setPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "one ticket"), Watch: watch})
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	events := openStream(t, f.base+"/s/boardsession/events")
	waitRegionHolding(t, events, regionBoard, "one ticket")

	f.waitSkipping(t)
	reads := f.src.readCount("boardsession")
	time.Sleep(4 * 300 * time.Millisecond)
	if got := f.src.readCount("boardsession"); got != reads {
		t.Fatalf("ticks over unchanged state read the page %d times, want none", got-reads)
	}

	// A new ticket file: its directory's time moves.
	if err := os.WriteFile(filepath.Join(ready, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpTime(t, ready)
	f.src.putPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "two tickets"), Watch: watch})
	waitRegionHolding(t, events, regionBoard, "two tickets")

	// An in-place edit of a listed ticket file: the directory stays.
	f.waitSkipping(t)
	if err := os.WriteFile(ticket, []byte("# A edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpTime(t, ticket)
	f.src.putPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "edited ticket"), Watch: watch})
	waitRegionHolding(t, events, regionBoard, "edited ticket")

	// Liveness alone: a snapshot that goes stale with the clock turns the
	// page back to board-first with no write and no wake.
	f.waitSkipping(t)
	writeSnapshot(t, f.dir, "boardsession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 400})
	f.d.broker.wake("boardsession")
	if ev, ok := nextRegion(t, events, 3*time.Second); !ok || ev.Name != regionMode || ev.HTML != modeLive {
		t.Fatalf("after the snapshot got %+v (%v), want the live mode", ev, ok)
	}
	waitRegionHolding(t, events, regionMode, modeBoard)
}

// TestRefreshTickRereadsContactLiveness: a page that shows contacts skips
// its ticks while their liveness holds, and re-renders when a contact's
// liveness changes with no wake.
func TestRefreshTickRereadsContactLiveness(t *testing.T) {
	f := startPageFixture(t, withRefresh)
	page := f.src.page("livesession")
	page.Contacts = append(page.Contacts, Contact{Address: "far@machine", LastExchange: time.Now()})
	f.src.setPage("livesession", page)
	f.d.index.Nudge("livesession", f.project)
	f.d.index.Settle()
	events := openStream(t, f.base+"/s/livesession/events")
	quiet(t, events)

	f.waitSkipping(t)
	reads := f.src.readCount("livesession")
	time.Sleep(4 * 300 * time.Millisecond)
	if got := f.src.readCount("livesession"); got != reads {
		t.Fatalf("ticks over unchanged contacts read the page %d times, want none", got-reads)
	}
	page.Contacts = append([]Contact(nil), page.Contacts...)
	page.Contacts[len(page.Contacts)-1].PresenceLive = true
	f.src.putPage("livesession", page)
	waitFor(t, "the tick to re-read the page", func() bool { return f.src.readCount("livesession") > reads })
	if ev, ok := nextRegion(t, events, 3*time.Second); !ok || ev.Name != regionTree {
		t.Fatalf("after the contact went live got %+v (%v), want the tree", ev, ok)
	}
}

// bumpTime moves path's modification time a second ahead, past any coarse
// file-system time resolution.
func bumpTime(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	at := info.ModTime().Add(time.Second)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// TestIndexReadsNoSessionPage: the sessions index shows a session's board
// counts from the index's own read (SessionData.CountPolicy) and reads no
// session page for them; a moved count still shows, and a session that
// leaves and rejoins shows its current count.
func TestIndexReadsNoSessionPage(t *testing.T) {
	f := startPageFixture(t)
	other := t.TempDir()
	f.addSession(t, "lonersession", other, "alone")
	index := openStream(t, f.base+"/events")
	quiet(t, index)

	liveReads, lonerReads := f.src.readCount("livesession"), f.src.readCount("lonersession")
	page := f.src.page("lonersession")
	page.Seq, page.Board = 2, boardRegions(t, "loner changed")
	page.Board.CountPolicy = 7
	f.src.setPage("lonersession", page)
	f.d.index.Nudge("lonersession", other)
	f.d.index.Settle()
	waitRegionHolding(t, index, regionSessions, `>7</span></span><span class="when">`)
	time.Sleep(settle)
	if got := f.src.readCount("lonersession"); got != lonerReads {
		t.Errorf("the index read the changed session's page %d times, want none", got-lonerReads)
	}
	if got := f.src.readCount("livesession"); got != liveReads {
		t.Errorf("the index read an unchanged session's page %d times", got-liveReads)
	}

	// Leave and rejoin with another count: the index shows the new one.
	f.src.mu.Lock()
	f.src.gone["lonersession"] = true
	f.src.mu.Unlock()
	f.d.index.Nudge("lonersession", other)
	f.d.index.Settle()
	waitFor(t, "lonersession to leave", func() bool { _, ok := f.d.Index().Session("lonersession"); return !ok })
	f.src.mu.Lock()
	delete(f.src.gone, "lonersession")
	f.src.mu.Unlock()
	page.Board.CountPolicy = 9
	f.src.setPage("lonersession", page)
	f.d.index.Nudge("lonersession", other)
	f.d.index.Settle()
	waitRegionHolding(t, index, regionSessions, `>9</span></span><span class="when">`)
}

// TestRefreshTickFollowsAWatchedProcess: a page that names a process
// (PageContent.WatchPIDs) skips its ticks while the process's liveness
// holds, and re-renders when it dies, with no wake and no file change.
func TestRefreshTickFollowsAWatchedProcess(t *testing.T) {
	var mu sync.Mutex
	alive := true
	prev := processAlive
	processAlive = func(pid int) bool {
		mu.Lock()
		defer mu.Unlock()
		return pid == 4242 && alive
	}
	t.Cleanup(func() { processAlive = prev })

	f := startPageFixture(t, withRefresh)
	f.src.setPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "waiting"), WatchPIDs: []int{4242}})
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	events := openStream(t, f.base+"/s/boardsession/events")
	waitRegionHolding(t, events, regionBoard, "waiting")

	f.waitSkipping(t)
	reads := f.src.readCount("boardsession")
	time.Sleep(4 * 300 * time.Millisecond)
	if got := f.src.readCount("boardsession"); got != reads {
		t.Fatalf("ticks over a live process read the page %d times, want none", got-reads)
	}
	f.src.putPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "wait ended"), WatchPIDs: []int{4242}})
	mu.Lock()
	alive = false
	mu.Unlock()
	waitRegionHolding(t, events, regionBoard, "wait ended")
}

// A hosted PID exit updates the head even if its registration never changes.
func TestHostedProcessExitRefreshesHead(t *testing.T) {
	var mu sync.Mutex
	alive := true
	prev := processAlive
	processAlive = func(pid int) bool {
		mu.Lock()
		defer mu.Unlock()
		return pid == 4242 && alive
	}
	t.Cleanup(func() { processAlive = prev })
	f := startPageFixture(t, withRefresh)
	reg := register(t, f.dir, "hostprocess", f.project, time.Now())
	reg.Hosted, reg.HostPID = true, 4242
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.src.setPage("hostprocess", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "host")})
	f.d.Index().Refresh()
	events := openStream(t, f.base+"/s/hostprocess/events")
	waitRegionHolding(t, events, regionHead, `title="harness process running"`)
	mu.Lock()
	alive = false
	mu.Unlock()
	waitRegionHolding(t, events, regionHead, `class="host sse stopped"`)
}

// TestWokenPageSkipsTheNextTick: a page a wake re-rendered takes its
// refresh stamp before that render, so the next tick over unchanged state
// skips it rather than rendering it again.
func TestWokenPageSkipsTheNextTick(t *testing.T) {
	f := startPageFixture(t, withRefresh)
	events := openStream(t, f.base+"/s/boardsession/events")
	quiet(t, events)
	f.waitSkipping(t)

	f.change(t, "boardsession", f.project, "woken", 2)
	waitRegionHolding(t, events, regionBoard, "woken")
	reads := f.src.readCount("boardsession")
	skips := f.d.stats.RefreshSkips.Load()
	waitFor(t, "a tick after the wake", func() bool { return f.d.stats.RefreshSkips.Load() > skips })
	if got := f.src.readCount("boardsession"); got != reads {
		t.Fatalf("the tick after a wake read the page %d times, want none", got-reads)
	}
}
