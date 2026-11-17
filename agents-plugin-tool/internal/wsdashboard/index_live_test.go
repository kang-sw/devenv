package wsdashboard

import (
	"sync"
	"testing"
	"time"
)

// rereadClock holds a cooldown without wall-clock sleeps. Refresh must be able
// to use the same lane while the scheduled worker waits here.
type rereadClock struct {
	mu      sync.Mutex
	at      time.Time
	waiting chan time.Duration
	proceed chan struct{}
}

func newRereadClock() *rereadClock {
	return &rereadClock{at: testNow, waiting: make(chan time.Duration, 1), proceed: make(chan struct{}, 1)}
}
func (c *rereadClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}
func (c *rereadClock) wait(d time.Duration) {
	c.waiting <- d
	<-c.proceed
}
func (c *rereadClock) advance() {
	c.mu.Lock()
	c.at = c.at.Add(time.Second)
	c.mu.Unlock()
	c.proceed <- struct{}{}
}
func (c *rereadClock) await(t *testing.T) {
	t.Helper()
	select {
	case d := <-c.waiting:
		if d != time.Second {
			t.Fatalf("cooldown = %v, want 1s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no trailing cooldown")
	}
}

type liveCountingSource struct {
	*fakeSource
	live   int
	stores LiveStores
}

func (s *liveCountingSource) ReadLiveSession(reg Registration, previous SessionData, stores LiveStores) (SessionData, bool, error) {
	s.fakeSource.mu.Lock()
	defer s.fakeSource.mu.Unlock()
	s.live++
	s.stores = stores
	return s.data[reg.ReplyID], true, nil
}
func (s *liveCountingSource) liveCount() (int, LiveStores) {
	s.fakeSource.mu.Lock()
	defer s.fakeSource.mu.Unlock()
	return s.live, s.stores
}

func TestIndexLiveBurstHasOneTrailingReadAcrossIdleWorkers(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	register(t, dir, "alpha", project, testNow)
	src := &liveCountingSource{fakeSource: newFakeSource()}
	clock := newRereadClock()
	idx := NewIndex(IndexOptions{Dir: dir, Source: src, Now: clock.now})
	idx.scheduledNow, idx.scheduledWait = clock.now, clock.wait
	idx.Refresh()
	idx.RereadLive("alpha")
	idx.Settle() // worker exits; its cooldown must survive
	for range 100 {
		idx.RereadLive("alpha", LiveRing)
		idx.RereadLive("alpha", LiveInbox)
	}
	clock.await(t)
	if n, _ := src.liveCount(); n != 1 {
		t.Fatalf("burst performed %d live reads before cooldown, want 1", n)
	}
	src.set("alpha", SessionData{Fingerprint: "last write"})
	clock.advance()
	idx.Settle()
	if n, stores := src.liveCount(); n != 2 || stores != LiveRing|LiveInbox {
		t.Fatalf("live reads = %d, stores = %b; want 2 and ring+inbox", n, stores)
	}
	if n := src.readCounts()["alpha"]; n != 1 {
		t.Fatalf("runner writes performed %d full reads, want startup only", n)
	}
	if s, _ := idx.Session("alpha"); s.Rev != 2 {
		t.Fatalf("trailing write was not applied: %+v", s)
	}
}

func TestIndexFullRequestWinsAndRefreshBypassesScheduledCooldown(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	register(t, dir, "alpha", project, testNow)
	register(t, dir, "bravo", project, testNow)
	src := &liveCountingSource{fakeSource: newFakeSource()}
	clock := newRereadClock()
	idx := NewIndex(IndexOptions{Dir: dir, Source: src, Now: clock.now})
	idx.scheduledNow, idx.scheduledWait = clock.now, clock.wait
	idx.Refresh()
	idx.RereadLive("alpha")
	idx.Settle()
	idx.RereadLive("alpha", LiveRing)
	clock.await(t)
	// A ws-mcp nudge and a board poke must not be downgraded by runner writes.
	if !idx.Nudge("alpha", project) {
		t.Fatal("nudge refused")
	}
	idx.Reread("alpha")
	for range 100 {
		idx.RereadLive("alpha", LiveInbox)
	}
	returns(t, "full Refresh during cooldown", idx.Refresh)
	if counts := src.readCounts(); counts["alpha"] != 2 || counts["bravo"] != 2 {
		t.Fatalf("Refresh was throttled: %v", counts)
	}
	clock.advance()
	idx.Settle()
	if n, _ := src.liveCount(); n != 1 {
		t.Fatalf("full requests were downgraded: %d live reads", n)
	}
	if n := src.readCounts()["alpha"]; n != 3 {
		t.Fatalf("full reads = %d, want startup, Refresh and trailing full read", n)
	}
}

func TestIndexColdRunnerWriteInitializesIdentity(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	register(t, dir, "alpha", project, testNow)
	src := &liveCountingSource{fakeSource: newFakeSource()}
	idx := NewIndex(IndexOptions{Dir: dir, Source: src})
	idx.RereadLive("alpha", LiveRing)
	idx.Settle()
	if n, _ := src.liveCount(); n != 0 || src.readCounts()["alpha"] != 1 {
		t.Fatal("cold runner write must establish its lead identity with one full read")
	}
	if _, ok := idx.Session("alpha"); !ok {
		t.Fatal("cold session was not indexed")
	}
}
