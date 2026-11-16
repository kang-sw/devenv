package wsdashboard

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestGitWatcherMetadataSafetyRefreshAndNoConsumers(t *testing.T) {
	fake := newFakeGit()
	g, _ := newTestWatcher(t, fake)
	var clock atomic.Int64
	clock.Store(time.Unix(1700000000, 0).UnixNano())
	g.now = func() time.Time { return time.Unix(0, clock.Load()) }
	g.gap = 0
	var version atomic.Int64
	var probes atomic.Int64
	g.probe = func(string) (string, error) {
		probes.Add(1)
		if version.Load() == 0 {
			return "a", nil
		}
		return "b", nil
	}
	if probes.Load() != 0 {
		t.Fatal("unwatched probe")
	}
	a := g.watch("/repo")
	expectRead(t, fake, "/repo")
	b := g.watch("/repo/")
	tick := nextWait(t, fake, 3*time.Minute)
	clock.Add(int64(3 * time.Minute))
	tick.ch <- g.now()
	tick = nextWait(t, fake, 2*time.Minute)
	if fake.readCount("/repo") != 1 {
		t.Fatal("stable metadata launched git")
	}
	clock.Add(int64(2 * time.Minute))
	tick.ch <- g.now()
	expectRead(t, fake, "/repo")
	_ = nextWait(t, fake, 3*time.Minute)
	if g.badge("/repo").Ahead != 2 {
		t.Fatal("safety read did not update worktree-only changes")
	}
	// Explicit refresh is authoritative even when the metadata is unchanged.
	if !g.refresh("/repo") {
		t.Fatal("refresh refused")
	}
	expectRead(t, fake, "/repo")
	tick = nextWait(t, fake, 3*time.Minute)
	version.Store(1)
	clock.Add(int64(3 * time.Minute))
	tick.ch <- g.now()
	expectRead(t, fake, "/repo")
	_ = nextWait(t, fake, 3*time.Minute)
	a()
	b()
	waitFor(t, "no git consumers", func() bool { return watched(g) == 0 })
	before := probes.Load()
	if g.refresh("/repo") || probes.Load() != before {
		t.Fatal("hidden/unwatched root performed discovery")
	}
	resume := g.watch("/repo")
	expectRead(t, fake, "/repo")
	resume()
	t.Log("shared-root controller: no-consumer startup 0 reads; unchanged 3m check 0 commands; 5m/explicit/changed/resumed reads retained")
}

func TestRailMetadataSelectiveSafetyAndVisibleResumption(t *testing.T) {
	fake := &railFakeRead{reads: map[string]int{}}
	timers := newFakeGit()
	stop := make(chan struct{})
	defer close(stop)
	l := newRailLister(fake.read, nil, stop)
	var clock atomic.Int64
	clock.Store(time.Unix(1700000000, 0).UnixNano())
	l.now = func() time.Time { return time.Unix(0, clock.Load()) }
	l.after = timers.after
	var changed atomic.Bool
	l.probe = func(root string) (string, error) {
		if root == filepath.Clean("/a") && changed.Load() {
			return "changed", nil
		}
		return "stable", nil
	}
	wanted := map[string][]Registration{"/a": {{ReplyID: "one"}, {ReplyID: "two"}}, "/b": nil}
	l.lists(wanted)
	if fake.count("/a") != 0 {
		t.Fatal("no-consumer startup ran worktree list")
	}
	release := l.watchFresh()
	tick := nextWait(t, timers, 3*time.Minute)
	if fake.count("/a") != 1 || fake.count("/b") != 1 {
		t.Fatal("startup was not per repository")
	}
	changed.Store(true)
	clock.Add(int64(3 * time.Minute))
	tick.ch <- l.now()
	tick = nextWait(t, timers, 2*time.Minute)
	if fake.count("/a") != 2 || fake.count("/b") != 1 {
		t.Fatal("changed metadata reread unrelated repositories")
	}
	clock.Add(int64(2 * time.Minute))
	tick.ch <- l.now()
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/a") != 2 || fake.count("/b") != 2 {
		t.Fatal("safety deadline postponed by another root")
	}
	if !l.refresh() {
		t.Fatal("explicit rail refresh refused")
	}
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/a") != 3 || fake.count("/b") != 3 {
		t.Fatal("explicit refresh did not bypass metadata")
	}
	release()
	waitFor(t, "rail hidden", func() bool { l.mu.Lock(); defer l.mu.Unlock(); return !l.running })
	release = l.watchFresh()
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/a") != 4 || fake.count("/b") != 4 {
		t.Fatal("visible resumption reused stale display hints")
	}
	release()
}
