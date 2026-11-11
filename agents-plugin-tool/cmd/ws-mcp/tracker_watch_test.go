package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// TestTrackerWatchBumpsTheDaemonRevision: with the daemon's store watch
// wired to Daemon.TrackerChanged, a write by another process (another Host
// over the same root, as a lead session's or a support agent's ws-mcp)
// bumps the tracker revision the page polls, within a few watch intervals.
func TestTrackerWatchBumpsTheDaemonRevision(t *testing.T) {
	dt, root := newTestTracker(t)
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: t.TempDir(), Port: 0, Version: "1.0.0", LockWait: 200 * time.Millisecond, Tracker: dt})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	rev := func() int64 {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/tracker/stores", nil)
		req.Host = "127.0.0.1"
		rec := httptest.NewRecorder()
		d.Handler().ServeHTTP(rec, req)
		var body struct {
			Rev int64 `json:"rev"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
			t.Fatalf("GET /tracker/stores: %d %s", rec.Code, rec.Body.String())
		}
		return body.Rev
	}
	previous := trackerWatchInterval
	trackerWatchInterval = 200 * time.Millisecond
	t.Cleanup(func() { trackerWatchInterval = previous })
	stop := startTrackerWatch(dt.h, nil, d.TrackerChanged)
	defer stop()
	before := rev()

	other, err := wstrackerhost.New(wstrackerhost.Options{Root: root, Runner: offlineRunner{inner: wsgit.ExecRunner{}}})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": "from another process"})
	if _, err := other.Dispatch(context.Background(), "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for rev() == before {
		if time.Now().After(deadline) {
			t.Fatalf("the tracker revision did not move within %v of another process's write", 4*time.Second)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestStartTrackerWatchWithoutAHost: a host that could not be created runs
// no watch, and stop is safe.
func TestStartTrackerWatchWithoutAHost(t *testing.T) {
	stop := startTrackerWatch(nil, context.Canceled, func() { t.Fatal("no watch runs without a host") })
	stop()
}

// TestLateFuncBindsAfterConstruction: a call before set does nothing; after
// set it runs the bound function.
func TestLateFuncBindsAfterConstruction(t *testing.T) {
	var l lateFunc
	l.call()
	n := 0
	l.set(func() { n++ })
	l.call()
	l.call()
	if n != 2 {
		t.Fatalf("bound function ran %d times, want 2", n)
	}
}

// TestStartTrackerHooksWithoutAHost: a host that could not be created runs
// no hooks, and nudge and stop are safe.
func TestStartTrackerHooksWithoutAHost(t *testing.T) {
	nudge, stop := startTrackerHooks(nil, context.Canceled, t.TempDir(), 7000, nil)
	nudge()
	stop()
}
