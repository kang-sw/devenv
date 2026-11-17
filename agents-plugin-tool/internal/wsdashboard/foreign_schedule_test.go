package wsdashboard

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswake"
)

// TestForeignScheduleSharesOneMailboxLook: idle mailbox stamps read once per
// key per look; a queue change hint or a nudge renews every key's look at
// once, the safety interval renews it with no hint, and a failed read is
// never kept.
func TestForeignScheduleSharesOneMailboxLook(t *testing.T) {
	hub := &wswake.Hub{}
	s := newForeignSchedule(time.Minute, hub)
	defer s.close()
	now := time.Now()
	s.now = func() time.Time { return now }
	reads, value := 0, "a"
	read := func() (string, bool) { reads++; return value, true }
	stamp := func(key string) string {
		got, ok := s.mailStamp(key, read)
		if !ok {
			t.Fatal("mail stamp failed")
		}
		return got
	}
	for range 10 {
		stamp("one")
		stamp("two")
		now = now.Add(5 * time.Second)
	}
	if reads != 2 {
		t.Fatalf("20 idle stamps over two keys read %d times, want two", reads)
	}
	value = "b"
	if stamp("one") != "a" || reads != 2 {
		t.Fatal("an unreported change was read before the look renewed")
	}
	hub.StoreChanged("queue.json")
	if stamp("one") != "b" || stamp("two") != "b" || reads != 4 {
		t.Fatalf("a queue hint renewed reads=%d, want 4", reads)
	}
	s.mailChanged()
	stamp("one")
	if reads != 5 {
		t.Fatalf("a nudge renewed reads=%d, want 5", reads)
	}
	now = now.Add(time.Minute)
	stamp("one")
	if reads != 6 {
		t.Fatalf("the safety interval renewed reads=%d, want 6", reads)
	}
	fail := func() (string, bool) { reads++; return "", false }
	for range 2 {
		if _, ok := s.mailStamp("three", fail); ok {
			t.Fatal("a failed read stamped")
		}
	}
	if reads != 8 {
		t.Fatalf("failed reads retried %d times, want two", reads-6)
	}
	var none *foreignSchedule
	none.mailStamp("one", read)
	none.mailStamp("one", read)
	if reads != 10 || !none.filesSince().IsZero() {
		t.Fatal("a nil schedule must read every time and ask for the one-second window")
	}
}

// TestForeignScheduleFileLook: the file look stays put within the interval
// and renews after it or on a clock rollback.
func TestForeignScheduleFileLook(t *testing.T) {
	s := newForeignSchedule(time.Minute, nil)
	now := time.Now()
	s.now = func() time.Time { return now }
	first := s.filesSince()
	now = now.Add(59 * time.Second)
	if s.filesSince() != first {
		t.Fatal("the file look renewed within its interval")
	}
	now = now.Add(time.Second)
	second := s.filesSince()
	if !second.Equal(now) {
		t.Fatal("the file look did not renew after its interval")
	}
	now = now.Add(-time.Second)
	if !s.filesSince().Equal(now) {
		t.Fatal("a clock rollback did not renew the file look")
	}
}

// TestForeignLookSharesWatchedStampsAcrossPages: refresh stamps of several
// pages over the same watched ticket directory stat it once per look, an
// in-place edit or an atomic replacement shows at the next look, and a
// render's own one-second look counts for the refresh stamps too.
func TestForeignLookSharesWatchedStampsAcrossPages(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	if err := os.MkdirAll(ready, 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(ready, "a.md")
	if err := os.WriteFile(ticket, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(image, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	stats, lists := 0, 0
	cache := watchedStampCache{
		now:     func() time.Time { return now },
		stat:    func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) },
		readDir: func(path string) ([]os.DirEntry, error) { lists++; return os.ReadDir(path) },
	}
	watch := []string{image, ready, ticket}
	stamp := func(since time.Time) string {
		h := sha256.New()
		cache.writeWatched(h, watch, since)
		return sumHex(h)
	}
	since := now
	first := stamp(since)
	// Three pages ticking every 15 s for 2.5 minutes.
	for range 10 {
		now = now.Add(sseRefreshInterval)
		for range 3 {
			if stamp(since) != first {
				t.Fatal("an unchanged look moved the stamp")
			}
		}
	}
	if stats != 1 || lists != 1 {
		t.Fatalf("30 idle refresh stamps: stats=%d listings=%d, want one each", stats, lists)
	}
	t.Logf("MEASURE 30 idle refresh stamps over 3 pages: stats 30 -> %d, listings 30 -> %d", stats, lists)

	// An in-place edit of a listed ticket.
	if err := os.WriteFile(ticket, []byte("# A edited in place\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpTime(t, ticket)
	if stamp(since) != first {
		t.Fatal("an in-place edit was seen before the next look")
	}
	now = now.Add(foreignSafetyInterval)
	since = now
	edited := stamp(since)
	if edited == first || stats != 2 || lists != 2 {
		t.Fatalf("the next look: changed=%t stats=%d listings=%d", edited != first, stats, lists)
	}

	// An atomic replacement of a watched file outside a listed directory.
	tmp := image + ".tmp"
	if err := os.WriteFile(tmp, []byte("png, replaced"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, image); err != nil {
		t.Fatal(err)
	}
	if stamp(since) != edited {
		t.Fatal("a replacement was seen before the next look")
	}
	// A render's one-second look (a zero since) refreshes the shared slots,
	// and the refresh stamps take its newer look.
	now = now.Add(watchedStampInterval)
	h := sha256.New()
	cache.writeWatched(h, watch, time.Time{})
	if replaced := stamp(since); replaced == edited || stats != 3 {
		t.Fatalf("a render's look was not shared: changed=%t stats=%d", replaced != edited, stats)
	}
}

// TestRailLayoutGenerationSinceStatsOncePerLook: the rail's refresh stamp
// stats the layout file once per foreign look; another process's write
// shows at the next look.
func TestRailLayoutGenerationSinceStatsOncePerLook(t *testing.T) {
	dir := t.TempDir()
	s := openRailLayoutStore(dir)
	since := time.Now()
	first := s.generationSince(since)
	if err := os.WriteFile(filepath.Join(dir, railLayoutFileName), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s.generationSince(since) != first {
		t.Fatal("another process's write was seen within the look")
	}
	if s.generationSince(time.Now().Add(time.Second)) == first {
		t.Fatal("the next look missed another process's write")
	}
}

// TestRefreshTickDefersForeignInputsToTheSafetyLook: with the production
// foreign look, idle ticks of several streams read no page and no contacts;
// a no-wake ticket edit shows only once the foreign look renews; a mailbox
// queue hint renews the contacts at the next tick; and a liveness change
// that follows the clock still shows on a tick with no renewed look.
func TestRefreshTickDefersForeignInputsToTheSafetyLook(t *testing.T) {
	hub := &wswake.Hub{}
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.refreshInterval = 300 * time.Millisecond
		o.foreignInterval = time.Hour
		o.MailWake = hub
	})
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
	board := openStream(t, f.base+"/s/boardsession/events")
	openStream(t, f.base+"/s/boardsession/events")
	live := openStream(t, f.base+"/s/livesession/events")
	openStream(t, f.base+"/s/livesession/events")
	waitRegionHolding(t, board, regionBoard, "one ticket")
	quiet(t, live)
	f.waitSkipping(t)

	idle := func(what string) {
		t.Helper()
		boardReads, liveReads := f.src.readCount("boardsession"), f.src.readCount("livesession")
		f.src.mu.Lock()
		contacts := f.src.contactReads
		f.src.mu.Unlock()
		time.Sleep(5 * 300 * time.Millisecond)
		f.src.mu.Lock()
		contacts = f.src.contactReads - contacts
		f.src.mu.Unlock()
		if b, l := f.src.readCount("boardsession")-boardReads, f.src.readCount("livesession")-liveReads; b != 0 || l != 0 || contacts != 0 {
			t.Fatalf("%s: idle ticks read board=%d live=%d contacts=%d, want none", what, b, l, contacts)
		}
	}
	idle("unchanged state")

	// A no-wake in-place ticket edit waits for the foreign look.
	if err := os.WriteFile(ticket, []byte("# A edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpTime(t, ticket)
	f.src.putPage("boardsession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "edited ticket"), Watch: watch})
	idle("an unreported ticket edit")
	f.d.foreign.mu.Lock()
	f.d.foreign.filesAt = f.d.foreign.filesAt.Add(-time.Hour)
	f.d.foreign.mu.Unlock()
	waitRegionHolding(t, board, regionBoard, "edited ticket")

	// A contact going live with no wake waits for the mailbox look; an
	// in-daemon queue change renews it at the next tick.
	page := f.src.page("livesession")
	page.Contacts = append([]Contact(nil), page.Contacts...)
	page.Contacts[0].PresenceLive = !page.Contacts[0].PresenceLive
	f.src.putPage("livesession", page)
	idle("an unreported contact change")
	reads := f.src.readCount("livesession")
	hub.StoreChanged("queue.json")
	waitFor(t, "the tick after a queue hint to re-read the page", func() bool { return f.src.readCount("livesession") > reads })

	// Liveness that follows the clock needs no renewed look.
	f.waitSkipping(t)
	writeSnapshot(t, f.dir, "boardsession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 400})
	f.d.broker.wake("boardsession")
	if ev, ok := nextRegion(t, board, 3*time.Second); !ok || ev.Name != regionMode || ev.HTML != modeLive {
		t.Fatalf("after the snapshot got %+v (%v), want the live mode", ev, ok)
	}
	waitRegionHolding(t, board, regionMode, modeBoard)
}
