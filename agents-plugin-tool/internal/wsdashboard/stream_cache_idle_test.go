package wsdashboard

import (
	"testing"
	"time"
)

// A non-board session wake (e.g. a persisted live snapshot) deliberately reads
// the page source again even when file stamps match. A quiet tick does not.
// This distinguishes wake-driven folder opens from a broken refresh stamp.
func TestRefreshCacheNonBoardWake(t *testing.T) {
	stats := new(streamStats)
	b := newBroker(stats)
	c := newPageCache(b, stats)
	src := pageSource{key: pageKey{replyID: "session"}}
	reads := 0
	src.render = func(*liveRender) pageRender {
		reads++
		return pageRender{ok: true, regions: regionSet{}, deps: []string{"session"}, inputs: refreshInputs{ids: []string{"session"}}}
	}
	stamps := 0
	src.stamp = func(refreshInputs) (string, bool) { stamps++; return "unchanged-board-files", true }
	p := c.acquire(src.key)
	defer c.release(src.key, p)
	c.get(p, src, true)
	c.get(p, src, true) // first known stamp
	before := reads
	for i := 0; i < 10; i++ {
		c.get(p, src, true)
	}
	if reads != before || stamps != 1 || stats.Shared.Load() != 10 {
		t.Fatalf("memoized idle tick: reads=%d stamps=%d shared=%d", reads-before, stamps, stats.Shared.Load())
	}
	// Expiry revalidates the stamp, but unchanged files still read no page.
	p.checked = time.Now().Add(-pageStampInterval)
	c.get(p, src, true)
	if reads != before || stamps != 2 || stats.RefreshSkips.Load() != 1 {
		t.Fatalf("expired idle tick: reads=%d stamps=%d skips=%d", reads-before, stamps, stats.RefreshSkips.Load())
	}
	for i := 0; i < 10; i++ {
		b.wake("session")
		c.get(p, src, false)
	}
	if reads-before != 10 {
		t.Fatalf("non-board wakes: reads=%d, want 10", reads-before)
	}
	t.Logf("10 memoized quiet ticks -> 0 page reads (%d shared); expired stamp -> %d refresh skip; 10 session wakes -> %d full page reads", stats.Shared.Load(), stats.RefreshSkips.Load(), reads-before)
}
