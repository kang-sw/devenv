package wsdashboard

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func TestInboxLiveRenderRefreshesHeldDrainedAndMixedFallback(t *testing.T) {
	f := startPageFixture(t)
	src := f.d.sessionPageSource("livesession", "en", "")
	page := f.d.cache.acquire(src.key)
	defer f.d.cache.release(src.key, page)
	first, _ := f.d.cache.get(page, src, false)
	initial := first.reads.(*sessionReads)
	readCount := f.src.readCount("livesession")
	entries := f.entryCounts()
	inbox := sessionInbox(f.dir, "livesession")
	if _, _, err := wsinbox.Send(inbox, wsinbox.Entry{Parts: []string{"fresh held text"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	session, _ := f.d.index.Session("livesession")
	f.d.inboxChanged(session)
	f.d.index.Settle()
	liveBefore := f.d.stats.LiveRenders.Load()
	held, _ := f.d.cache.get(page, src, false)
	reads := held.reads.(*sessionReads)
	if f.d.stats.LiveRenders.Load() != liveBefore+1 || f.src.readCount("livesession") != readCount {
		t.Fatal("inbox-only change re-read the page stores")
	}
	if len(reads.held) != 1 || len(reads.drained) != 0 || len(initial.held) != 0 || !strings.Contains(string(held.regions[regionConvLive].HTML), "fresh held text") {
		t.Fatal("inbox live render reused stale held reads or mutated previous reads")
	}
	got := f.entryCounts()
	if got[0] != entries[0] || got[1] <= entries[1] || got[2] <= entries[2] {
		t.Fatalf("inbox-only entry reads = %v from %v", got, entries)
	}
	if _, err := wsinbox.Drain(inbox); err != nil {
		t.Fatal(err)
	}
	f.d.inboxChanged(session)
	f.d.index.Settle()
	drained, _ := f.d.cache.get(page, src, false)
	reads = drained.reads.(*sessionReads)
	if len(reads.held) != 0 || len(reads.drained) != 1 || len(held.reads.(*sessionReads).held) != 1 {
		t.Fatal("drain did not refresh both immutable inbox reads")
	}
	canonical := src.render(nil)
	if !reflect.DeepEqual(drained.regions, canonical.regions) {
		t.Fatal("inbox live output differs from a full read")
	}
	// An inbox token plus any full store change must never reuse page reads.
	f.d.inboxChanged(session)
	f.change(t, "livesession", f.project, "mixed store text", 12)
	liveBefore = f.d.stats.LiveRenders.Load()
	readCount = f.src.readCount("livesession")
	mixed, _ := f.d.cache.get(page, src, false)
	if f.d.stats.LiveRenders.Load() != liveBefore || f.src.readCount("livesession") <= readCount || !strings.Contains(string(mixed.regions[regionBoard].HTML), "mixed store text") {
		t.Fatal("mixed store/inbox wake did not take the full fallback")
	}
}
