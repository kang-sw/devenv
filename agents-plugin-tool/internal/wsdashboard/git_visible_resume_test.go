package wsdashboard

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestFreshGitWatchRefreshesAnAlreadySharedRoot(t *testing.T) {
	fake := newFakeGit()
	g, _ := newTestWatcher(t, fake)
	g.gap = 0
	g.probe = func(string) (string, error) { return "unchanged", nil }
	a := g.watch("/repo")
	defer a()
	expectRead(t, fake, "/repo")
	_ = nextWait(t, fake, gitReadInterval)
	b := g.watchFresh("/repo")
	expectRead(t, fake, "/repo")
	_ = nextWait(t, fake, gitReadInterval)
	b() // A remains visible while B hides and resumes.
	b = g.watchFresh("/repo")
	defer b()
	expectRead(t, fake, "/repo")
	if fake.readCount("/repo") != 3 {
		t.Fatal("shared visible resumption suppressed a complete read")
	}
}

func TestFreshRailWatchRefreshesWithAnotherViewer(t *testing.T) {
	fake := &railFakeRead{reads: map[string]int{}}
	timers := newFakeGit()
	stop := make(chan struct{})
	defer close(stop)
	l := newRailLister(fake.read, nil, stop)
	l.after = timers.after
	var clock atomic.Int64
	clock.Store(time.Unix(1700000000, 0).UnixNano())
	l.now = func() time.Time { return time.Unix(0, clock.Load()) }
	l.probe = func(string) (string, error) { return "unchanged", nil }
	l.lists(map[string][]Registration{"/repo": nil})
	a := l.watch()
	defer a()
	_ = nextWait(t, timers, 3*time.Minute)
	b := l.watchFresh()
	tick := nextWait(t, timers, gitRefreshGap)
	for range 10 {
		release := l.watchFresh()
		release()
	}
	if fake.count("/repo") != 1 {
		t.Fatal("resumption inside the minimum gap launched git")
	}
	clock.Add(int64(gitRefreshGap))
	tick.ch <- l.now()
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/repo") != 2 {
		t.Fatal("resumption burst did not coalesce into one forced read")
	}
	b()
	b = l.watchFresh()
	tick = nextWait(t, timers, gitRefreshGap)
	b()
	a()
	clock.Add(int64(gitRefreshGap))
	tick.ch <- l.now()
	waitFor(t, "hidden rail gap to stop", func() bool { l.mu.Lock(); defer l.mu.Unlock(); return !l.running })
	if fake.count("/repo") != 2 {
		t.Fatal("gap expiry read without consumers")
	}
	// The clock has passed the gap; a fresh viewer must force unchanged
	// metadata even after the previous worker exited without a read.
	b = l.watchFresh()
	defer b()
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/repo") != 3 {
		t.Fatal("visible rail resumption suppressed a complete read")
	}
}
