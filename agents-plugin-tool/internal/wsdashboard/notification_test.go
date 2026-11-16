package wsdashboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectNotificationRegistrationValidationAndRepair(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	src := newFakeSource()
	src.set("alpha", SessionData{Fingerprint: "first"})
	register(t, dir, "alpha", project, now)
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	wake := &countingMailWake{}
	d := &Daemon{dir: dir, index: idx, endpoint: Endpoint{Port: 1234}, stop: make(chan struct{}), mailWake: wake}
	before := src.readCounts()["alpha"]
	for _, body := range []NudgeBody{
		{ReplyID: "unknown", Project: project},
		{ReplyID: "alpha", Project: t.TempDir()},
	} {
		if port, handled := d.NotifySessionChange(dir, body); port != 0 || !handled {
			t.Fatalf("rejected notification = %d, %v", port, handled)
		}
	}
	idx.Settle()
	if counts := src.readCounts(); counts["alpha"] != before || counts["unknown"] != 0 {
		t.Fatalf("rejected notification read source: %v", counts)
	}
	if _, nudges := wake.counts(); nudges != 2 {
		t.Fatalf("mail nudges = %d, want 2", nudges)
	}
	if err := os.Remove(filepath.Join(SessionDir(dir, "alpha"), registrationFileName)); err != nil {
		t.Fatal(err)
	}
	body := NudgeBody{ReplyID: "alpha", Project: project}
	if port, handled := d.NotifySessionChange(dir, body); port != 0 || !handled {
		t.Fatalf("removed registration = %d, %v", port, handled)
	}
	idx.Settle()
	if _, ok := idx.Session("alpha"); ok {
		t.Fatal("removed registration retained in index")
	}
	if got := src.readCounts()["alpha"]; got != before {
		t.Fatalf("removed registration read source: %d, want %d", got, before)
	}
	// A replacement registration is accepted and reconstructed by the lane.
	register(t, dir, "alpha", project, now)
	if port, handled := d.NotifySessionChange(dir, body); port != 1234 || !handled {
		t.Fatalf("replacement registration = %d, %v", port, handled)
	}
	idx.Settle()
	if _, ok := idx.Session("alpha"); !ok {
		t.Fatal("replacement registration not indexed")
	}
}

func TestDirectNotificationSubscriberAndRestart(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	src := newFakeSource()
	src.set("alpha", SessionData{Fingerprint: "before"})
	register(t, dir, "alpha", project, now)
	b := newBroker(&streamStats{})
	idx := NewIndex(IndexOptions{Dir: dir, Source: src, Now: func() time.Time { return now }, OnChange: func(id string) { b.wake(id) }})
	idx.Refresh()
	sub, cancel := b.subscribe()
	defer cancel()
	wake := &countingMailWake{}
	d := &Daemon{dir: dir, index: idx, endpoint: Endpoint{Port: 1234}, stop: make(chan struct{}), mailWake: wake}
	body := NudgeBody{ReplyID: "alpha", Project: project}
	if _, ok := d.NotifySessionChange(t.TempDir(), body); ok {
		t.Fatal("foreign cache handled")
	}
	src.set("alpha", SessionData{Fingerprint: "after"})
	for i := 0; i < 20; i++ {
		if port, ok := d.NotifySessionChange(dir, body); !ok || port != 1234 {
			t.Fatalf("notification = %d, %v", port, ok)
		}
	}
	idx.Settle()
	select {
	case <-sub.ch:
	default:
		t.Fatal("subscriber not woken")
	}
	select {
	case <-sub.ch:
		t.Fatal("subscriber wake did not coalesce")
	default:
	}
	session, ok := idx.Session("alpha")
	if !ok || session.Rev < 2 {
		t.Fatalf("session = %+v", session)
	}
	close(d.stop)
	if _, ok := d.NotifySessionChange(dir, body); ok {
		t.Fatal("stopped daemon handled notification")
	}
	replacement, _ := newTestIndex(t, dir, src, &now)
	replacement.Refresh()
	if _, ok := replacement.Session("alpha"); !ok {
		t.Fatal("replacement did not reconstruct session")
	}
}
