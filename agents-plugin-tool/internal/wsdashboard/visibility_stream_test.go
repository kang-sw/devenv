package wsdashboard

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestVisibilityStreamWatches(t *testing.T) {
	f := startPageFixture(t, withTracker(newFakeTracker()))
	viewers := func() int { f.d.rail.mu.Lock(); defer f.d.rail.mu.Unlock(); return f.d.rail.viewers }
	waitViewers := func(n int) { t.Helper(); waitFor(t, "rail viewers", func() bool { return viewers() == n }) }
	send := func(c wsClient, frame string) {
		t.Helper()
		if err := websocket.Message.Send(c.conn, frame); err != nil {
			t.Fatal(err)
		}
	}

	hidden := openWS(t, f.base, "/rail/events?display=0", f.base, true)
	// The first event proves the connection is live despite having no viewer.
	<-hidden.events
	if viewers() != 0 {
		t.Fatal("hidden connection acquired rail")
	}
	f.d.TrackerChanged()
	deadline := time.After(5 * time.Second)
tracker:
	for {
		select {
		case ev, ok := <-hidden.events:
			if !ok {
				t.Fatal("hidden stream closed")
			}
			if ev.name == "tracker" && ev.data == `{"rev":1}` {
				break tracker
			}
		case <-deadline:
			t.Fatal("hidden stream missed tracker update")
		}
	}
	for _, frame := range []string{`{"event":"visibility"}`, `{"event":"visibility","visible":null}`, `{"event":"visibility","visible":"true"}`} {
		send(hidden, frame)
	}
	send(hidden, `{"event":"visibility","visible":true}`)
	waitViewers(1)
	other := openWS(t, f.base, "/rail/events?display=0", f.base, true)
	<-other.events
	send(hidden, `{"event":"visibility","visible":false}`)
	waitViewers(0)
	send(other, `{"event":"visibility","visible":true}`)
	waitViewers(1)
	send(hidden, `{"event":"visibility","visible":true}`)
	waitViewers(2)
	hidden.conn.Close()
	waitViewers(1)
	other.conn.Close()
	waitViewers(0)
}

func TestVisibilityLegacyAndRegionFilter(t *testing.T) {
	f := startPageFixture(t)
	viewers := func() int { f.d.rail.mu.Lock(); defer f.d.rail.mu.Unlock(); return f.d.rail.viewers }
	waitViewers := func(n int) { t.Helper(); waitFor(t, "rail viewers", func() bool { return viewers() == n }) }
	legacy := openWS(t, f.base, "/rail/events", f.base, true)
	waitViewers(1)
	if err := websocket.Message.Send(legacy.conn, `{"event":"visibility","visible":false}`); err != nil {
		t.Fatal(err)
	}
	// A second legacy stream gives the read loop time to consume the noise.
	resp, err := (&http.Client{Transport: localhostTestHTTPTransport()}).Get(f.base + "/rail/events?display=0")
	if err != nil {
		t.Fatal(err)
	}
	waitViewers(2)
	resp.Body.Close()
	waitViewers(1)
	legacy.conn.Close()
	waitViewers(0)
	filtered := openWS(t, f.base, "/s/livesession/events?regions=entries&display=1", f.base, true)
	<-filtered.events
	if viewers() != 0 {
		t.Fatal("entries-only stream acquired rail")
	}
	filtered.conn.Close()
}

func TestVisibilitySessionGitWatch(t *testing.T) {
	f := startPageFixture(t, func(o *DaemonOptions) { o.noGitBadge = false })
	waitRoots := func(n int) { t.Helper(); waitFor(t, "git roots", func() bool { return watched(f.d.git) == n }) }
	c := openWS(t, f.base, "/s/livesession/events?frame=1&regions=git&display=0", f.base, true)
	<-c.events
	if watched(f.d.git) != 0 {
		t.Fatal("hidden session acquired git")
	}
	if err := websocket.Message.Send(c.conn, `{"event":"visibility","visible":true}`); err != nil {
		t.Fatal(err)
	}
	waitRoots(1)
	if err := websocket.Message.Send(c.conn, `{"event":"visibility","visible":false}`); err != nil {
		t.Fatal(err)
	}
	waitRoots(0)
	c.conn.Close()
}

func TestDisplayWatchClosedGuard(t *testing.T) {
	active, acquisitions := 0, 0
	w := &displayWatch{acquire: func(bool) func() { active++; acquisitions++; return func() { active-- } }}
	w.set(true)
	w.set(true)
	w.close()
	w.set(true)
	w.close()
	if active != 0 || acquisitions != 1 {
		t.Fatalf("active=%d acquisitions=%d", active, acquisitions)
	}
}
