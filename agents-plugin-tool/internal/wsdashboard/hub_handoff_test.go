package wsdashboard

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

type heldClockSource struct {
	data    SessionData
	entered chan struct{}
	release chan struct{}
}

func (s *heldClockSource) ReadSession(Registration) (SessionData, bool, error) {
	data := s.data
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	return data, true, nil
}

func TestHubInFlightReadCannotRollBackHostedClockHandoff(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	register(t, dir, "alpha", project, now)
	oldDeadline := now.Add(90*time.Second + time.Millisecond)
	source := &heldClockSource{data: SessionData{Fingerprint: "same", SnapshotUpdatedAt: now, SnapshotStaleAt: oldDeadline}}
	idx, _ := newTestIndex(t, dir, source, &now)
	idx.Refresh()
	source.entered, source.release = make(chan struct{}), make(chan struct{})
	idx.Reread("alpha")
	<-source.entered
	fresh := now.Add(time.Minute)
	deadline := fresh.Add(90*time.Second + time.Millisecond)
	idx.SetSnapshotUpdatedAt("alpha", fresh, deadline)
	close(source.release)
	idx.Settle()
	s, _ := idx.Session("alpha")
	idx.mu.Lock()
	gotDeadline := idx.sessions["alpha"].snapshotStaleAt
	idx.mu.Unlock()
	if !s.SnapshotUpdatedAt.Equal(fresh) || !gotDeadline.Equal(deadline) {
		t.Fatalf("older read rolled back handoff: at=%v expiry=%v", s.SnapshotUpdatedAt, gotDeadline)
	}
}

func TestHubFirstInsertionKeepsConcurrentHostedClockHandoff(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	register(t, dir, "alpha", project, now)
	source := &heldClockSource{data: SessionData{Fingerprint: "first", SnapshotUpdatedAt: now, SnapshotStaleAt: now.Add(90 * time.Second)}, entered: make(chan struct{}), release: make(chan struct{})}
	idx, _ := newTestIndex(t, dir, source, &now)
	done := make(chan struct{})
	go func() { idx.Refresh(); close(done) }()
	<-source.entered
	fresh := now.Add(time.Minute)
	deadline := fresh.Add(90*time.Second + time.Millisecond)
	idx.SetSnapshotUpdatedAt("alpha", fresh, deadline)
	close(source.release)
	<-done
	s, ok := idx.Session("alpha")
	idx.mu.Lock()
	gotDeadline := idx.sessions["alpha"].snapshotStaleAt
	idx.mu.Unlock()
	if !ok || !s.SnapshotUpdatedAt.Equal(fresh) || !gotDeadline.Equal(deadline) {
		t.Fatalf("first read lost handoff: %+v expiry=%v", s, gotDeadline)
	}
}

type publishOnHash struct {
	once    sync.Once
	publish func()
}

func (w *publishOnHash) Write(p []byte) (int, error) { w.once.Do(w.publish); return len(p), nil }

func TestHubHashAndClockUseOneSnapshotGeneration(t *testing.T) {
	dir := t.TempDir()
	cache := &SnapshotCache{}
	old := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: testNow.UnixMilli(), HeartbeatMs: 400}
	fresh := old
	fresh.UpdatedAt = testNow.Add(time.Minute).UnixMilli()
	fresh.HeartbeatMs = 30000
	cache.SetLive(dir, "alpha", old)
	hook := &publishOnHash{publish: func() { cache.SetLive(dir, "alpha", fresh) }}
	at, expiry := cache.HashLiveStoresClock(hook, dir, "alpha", testNow)
	if at.UnixMilli() != old.UpdatedAt || !expiry.Equal(snapshotStaleAt(&old)) {
		t.Fatalf("mixed generations: at=%v expiry=%v", at, expiry)
	}
	at, expiry = cache.HashLiveStoresClock(io.Discard, dir, "alpha", testNow)
	if at.UnixMilli() != fresh.UpdatedAt || !expiry.Equal(snapshotStaleAt(&fresh)) {
		t.Fatal("next read missed new generation")
	}
}
