package wsdashboard

import "testing"

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
	src.stamp = func(refreshInputs) (string, bool) { return "unchanged-board-files", true }
	p := c.acquire(src.key)
	defer c.release(src.key, p)
	c.get(p, src, true)
	c.get(p, src, true) // first known stamp
	before := reads
	for i := 0; i < 10; i++ {
		c.get(p, src, true)
	}
	if reads != before || stats.RefreshSkips.Load() != 10 {
		t.Fatalf("idle tick: reads=%d skips=%d", reads-before, stats.RefreshSkips.Load())
	}
	for i := 0; i < 10; i++ {
		b.wake("session")
		c.get(p, src, false)
	}
	if reads-before != 10 {
		t.Fatalf("non-board wakes: reads=%d, want 10", reads-before)
	}
	t.Logf("unchanged stamp: 10 quiet ticks -> 0 page reads (%d skips); 10 session wakes -> %d full page reads", stats.RefreshSkips.Load(), reads-before)
}
