package wsdashboard

import (
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
	l.probe = func(string) (string, error) { return "unchanged", nil }
	l.lists(map[string][]Registration{"/repo": nil})
	a := l.watch()
	defer a()
	_ = nextWait(t, timers, 3*time.Minute)
	b := l.watchFresh()
	_ = nextWait(t, timers, 3*time.Minute)
	b()
	b = l.watchFresh()
	defer b()
	_ = nextWait(t, timers, 3*time.Minute)
	if fake.count("/repo") != 3 {
		t.Fatal("shared rail resumption suppressed a complete read")
	}
}
