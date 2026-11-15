package wsdashboard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// transientSource deliberately keeps the valid data available while reporting
// an error, proving the index must heed the error rather than the bool/data.
type transientSource struct {
	err    error
	found  bool
	forgot int
}

func (s *transientSource) ReadSession(Registration) (SessionData, bool, error) {
	return SessionData{HasBoard: true, Fingerprint: "valid"}, s.found, s.err
}
func (s *transientSource) ForgetSession(string) { s.forgot++ }

func TestTransientReadIndexRetainsAndRetriesEndedSession(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	reg := register(t, dir, "transient", project, testNow)
	reg.EndedAt = stampFor(testNow)
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	src := &transientSource{found: true}
	changes := 0
	x := NewIndex(IndexOptions{Dir: dir, Source: src, Now: func() time.Time { return testNow }})
	// Install the daemon's removal/forget behavior without running a daemon.
	x.opts.OnChange = func(id string) {
		changes++
		if _, ok := x.Session(id); !ok {
			src.ForgetSession(id)
		}
	}
	x.Refresh()
	good, ok := x.Session(reg.ReplyID)
	if !ok {
		t.Fatal("valid session missing")
	}
	src.err = errors.New("busy board lock")
	x.Reread(reg.ReplyID)
	x.Settle()
	if got, ok := x.Session(reg.ReplyID); !ok || got != good || changes != 1 || src.forgot != 0 {
		t.Fatalf("transient changed valid state: %+v %v changes=%d forgot=%d", got, ok, changes, src.forgot)
	}
	if x.endedApplied(reg) {
		t.Fatal("ended entry would skip its scheduled retry")
	}
	src.err = nil
	x.Refresh() // the daemon's existing DefaultPollInterval backstop
	if !x.endedApplied(reg) {
		t.Fatal("successful retry did not settle ended entry")
	}
	src.found = false
	x.Reread(reg.ReplyID)
	x.Settle()
	if _, ok := x.Session(reg.ReplyID); ok || src.forgot != 1 {
		t.Fatal("genuine absence retained valid caches")
	}
}

func TestTransientReadInitialIndexFailureIsUnavailableUntilAbsent(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	reg := register(t, dir, "transient", project, testNow)
	src := &transientSource{found: true, err: errors.New("busy")}
	x := NewIndex(IndexOptions{Dir: dir, Source: src})
	x.Refresh()
	d := &Daemon{index: x}
	status := func() int {
		w := httptest.NewRecorder()
		d.handleSessionEvents(w, httptest.NewRequest(http.MethodGet, "/s/transient/events", nil), reg.ReplyID)
		return w.Code
	}
	if got := status(); got != http.StatusServiceUnavailable {
		t.Fatalf("initial transient status=%d", got)
	}
	if err := RemoveRegistration(dir, reg.ReplyID); err != nil {
		t.Fatal(err)
	}
	x.Refresh()
	if got := status(); got != http.StatusNotFound {
		t.Fatalf("confirmed absence status=%d", got)
	}
	if err := os.WriteFile(RegistrationPath(dir, reg.ReplyID), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := status(); got != http.StatusServiceUnavailable {
		t.Fatalf("unreadable registration status=%d", got)
	}
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	src.err = nil
	x.Refresh()
	if _, ok := x.Session(reg.ReplyID); !ok {
		t.Fatal("initial transient never recovered")
	}
}

func TestTransientReadRegistrationRetainedButMismatchRemoved(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	reg := register(t, dir, "transient", project, testNow)
	x := NewIndex(IndexOptions{Dir: dir, Source: &transientSource{found: true}})
	x.Refresh()
	path := RegistrationPath(dir, reg.ReplyID)
	for _, raw := range []string{"{", "{}"} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		x.Refresh() // unreadable rows disappear from the listing, not the index
		if _, ok := x.Session(reg.ReplyID); !ok {
			t.Fatal("unreadable registration removed session")
		}
	}
	// A present but unreadable registration is not a deletion either.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	x.Refresh()
	if _, ok := x.Session(reg.ReplyID); !ok {
		t.Fatal("registration I/O error removed session")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"reply_id":"different","project":"/other"}`), 0600); err != nil {
		t.Fatal(err)
	}
	x.Refresh()
	if _, ok := x.Session(reg.ReplyID); ok {
		t.Fatal("identity mismatch retained session")
	}
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	x.Refresh()
	if err := RemoveRegistration(dir, reg.ReplyID); err != nil {
		t.Fatal(err)
	}
	x.Refresh()
	if _, ok := x.Session(reg.ReplyID); ok {
		t.Fatal("removed registration retained session")
	}
}

func TestTransientReadCachePreservesLastGoodAndForcesRetry(t *testing.T) {
	b := newBroker(&streamStats{})
	c := newPageCache(b, &streamStats{})
	key := pageKey{replyID: "transient"}
	p := c.acquire(key)
	defer c.release(key, p)
	reads, fail, gone := 0, false, false
	src := pageSource{key: key, stamp: func(refreshInputs) (string, bool) { return "same", true }, render: func(*liveRender) pageRender {
		reads++
		if fail {
			return pageRender{err: errors.New("I/O"), deps: []string{key.replyID}}
		}
		if gone {
			return pageRender{deps: []string{key.replyID}}
		}
		return pageRender{ok: true, regions: regionSet{regionBoard: {HTML: "last good"}}, deps: []string{key.replyID}, reads: "valid"}
	}}
	good, _ := c.get(p, src, false)
	fail = true
	b.wake(key.replyID)
	got, _ := c.get(p, src, false)
	if got.err == nil || !got.ok || got.regions[regionBoard] != good.regions[regionBoard] || got.reads != "valid" {
		t.Fatal("lost last-good cache")
	}
	c.get(p, src, true)
	if reads != 3 {
		t.Fatal("unchanged stamp suppressed retry")
	}
	fail = false
	got, _ = c.get(p, src, true)
	if got.err != nil || !got.ok || reads != 4 {
		t.Fatal("recovery was not read")
	}
	gone = true
	b.wake(key.replyID)
	got, _ = c.get(p, src, false)
	if got.ok || got.regions != nil {
		t.Fatal("confirmed absence retained stale content")
	}
}

func TestTransientReadOpenStreamRecoversThenSendsGone(t *testing.T) {
	stats := &streamStats{}
	d := &Daemon{broker: newBroker(stats), stats: stats, stop: make(chan struct{}), refreshInterval: 20 * time.Millisecond, keepAlive: time.Hour}
	d.cache = newPageCache(d.broker, stats)
	var phase atomic.Int32
	failed := make(chan struct{}, 1)
	src := pageSource{key: pageKey{replyID: "transient"}, stamp: func(refreshInputs) (string, bool) { return "", false }, render: func(*liveRender) pageRender {
		result := pageRender{ok: true, deps: []string{"transient"}, regions: regionSet{regionBoard: {HTML: "last good", Hash: "one"}}}
		switch phase.Load() {
		case 1:
			select {
			case failed <- struct{}{}:
			default:
			}
			return pageRender{err: errors.New("busy folder"), deps: result.deps}
		case 2:
			result.regions = regionSet{regionBoard: {HTML: "recovered", Hash: "two"}}
		case 3:
			return pageRender{deps: result.deps}
		}
		return result
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { d.stream(w, r, src, nil, nil) }))
	t.Cleanup(server.Close)
	events := openStream(t, server.URL)
	waitRegionHolding(t, events, regionBoard, "last good")
	phase.Store(1)
	d.broker.wake("transient")
	select {
	case <-failed:
	case <-time.After(5 * time.Second):
		t.Fatal("stream never attempted transient render")
	}
	phase.Store(2)
	waitRegionHolding(t, events, regionBoard, "recovered")
	phase.Store(3)
	d.broker.wake("transient")
	select {
	case ev := <-events:
		if ev.name != "gone" {
			t.Fatalf("confirmed absence event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed absence never sent gone")
	}
}

func TestTransientReadStreamInitialStatuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result pageRender
		status int
	}{
		{"absent", pageRender{}, http.StatusNotFound},
		{"unreadable", pageRender{err: errors.New("busy")}, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &Daemon{broker: newBroker(&streamStats{}), stats: &streamStats{}}
			d.cache = newPageCache(d.broker, d.stats)
			w := httptest.NewRecorder()
			d.stream(w, httptest.NewRequest(http.MethodGet, "/events", nil), pageSource{render: func(*liveRender) pageRender { return tc.result }}, nil, nil)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
}
