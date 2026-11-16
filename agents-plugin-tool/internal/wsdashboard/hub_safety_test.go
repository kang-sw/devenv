package wsdashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestHubClockReadsOnlyDueEvidenceAndRechecksLegacyHeartbeats(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	src := newFakeSource()
	reg := register(t, dir, "alpha", project, now)
	src.set("alpha", SessionData{Fingerprint: "same", SnapshotUpdatedAt: now, SnapshotStaleAt: now.Add(90*time.Second + time.Millisecond)})
	idx, changes := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	*changes = nil
	for range 2 {
		now = now.Add(30 * time.Second)
		idx.RefreshClock()
		idx.Settle()
	}
	if n := src.readCounts()["alpha"]; n != 1 {
		t.Fatalf("healthy clock reread source %d times", n)
	}
	// A legacy writer heartbeats on disk but supplies no daemon event. The
	// cached snapshot deadline requests a targeted authoritative read rather
	// than falsely declaring that writer stale for the full safety interval.
	now = now.Add(30*time.Second + time.Millisecond)
	reg.HeartbeatAt = stampFor(now)
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	src.set("alpha", SessionData{Fingerprint: "same", SnapshotUpdatedAt: now, SnapshotStaleAt: now.Add(90*time.Second + time.Millisecond)})
	idx.RefreshClock()
	idx.Settle()
	s, _ := idx.Session("alpha")
	if !s.Live || s.Rev != 1 || len(*changes) != 0 || src.readCounts()["alpha"] != 2 {
		t.Fatalf("legacy evidence falsely expired: %+v changes=%v reads=%v", s, *changes, src.readCounts())
	}
	// A real crash is observed at the same 30-second clock cadence. Keep
	// verifying stale legacy evidence so a non-notifying resume is prompt.
	now = now.Add(90*time.Second + time.Millisecond)
	idx.RefreshClock()
	idx.Settle()
	s, _ = idx.Session("alpha")
	if s.Rev != 2 || len(*changes) != 1 {
		t.Fatalf("snapshot expiry lost: %+v %v", s, *changes)
	}
	reads := src.readCounts()["alpha"]
	idx.RefreshClock()
	idx.Settle()
	if src.readCounts()["alpha"] != reads+1 {
		t.Fatal("stale legacy recovery backstop was lost")
	}
	reg.HeartbeatAt = stampFor(now)
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	src.set("alpha", SessionData{Fingerprint: "same", SnapshotUpdatedAt: now, SnapshotStaleAt: now.Add(90*time.Second + time.Millisecond)})
	idx.RefreshClock()
	idx.Settle()
	s, _ = idx.Session("alpha")
	if !s.Live || s.Rev != 3 || len(*changes) != 2 {
		t.Fatalf("legacy recovery after stale state lost: %+v %v", s, *changes)
	}
	now = now.Add(LiveWindow + time.Second)
	idx.RefreshClock()
	idx.Settle()
	s, _ = idx.Session("alpha")
	if s.Live || s.Rev != 4 {
		t.Fatalf("registration expiry lost: %+v", s)
	}
}

func TestHubSafetyScanRecoversLostNotificationsAndRestart(t *testing.T) {
	if DefaultPollInterval != 5*time.Minute || indexClockInterval != 30*time.Second {
		t.Fatal("safety or liveness policy changed")
	}
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	src := newFakeSource()
	for i := range 68 {
		id := fmt.Sprintf("session%d", i)
		register(t, dir, id, project, now)
		src.set(id, SessionData{Fingerprint: "before", SnapshotUpdatedAt: now, SnapshotStaleAt: now.Add(90*time.Second + time.Millisecond)})
	}
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	// Hosted snapshot heartbeats hand their new deadline over in memory.
	for range 9 {
		now = now.Add(indexClockInterval)
		for i := range 68 {
			id := fmt.Sprintf("session%d", i)
			idx.SetSnapshotUpdatedAt(id, now, now.Add(90*time.Second+time.Millisecond))
		}
		idx.RefreshClock()
		idx.Settle()
	}
	for id, n := range src.readCounts() {
		if n != 1 {
			t.Fatalf("idle clock reread %s: %d", id, n)
		}
	}
	// A foreign write/lost notification is discovered by the next full scan,
	// whose production interval is five minutes, not the per-stream clock.
	src.set("session0", SessionData{Fingerprint: "external edit"})
	idx.Refresh()
	for id, n := range src.readCounts() {
		if n != 2 {
			t.Fatalf("safety read count %s: %d", id, n)
		}
	}
	s, _ := idx.Session("session0")
	if s.Rev != 2 {
		t.Fatalf("lost notification not recovered: %+v", s)
	}
	replacement, _ := newTestIndex(t, dir, src, &now)
	replacement.Refresh()
	if restored, ok := replacement.Session("session0"); !ok || restored.Rev != 1 {
		t.Fatal("restart failed to reconstruct source state")
	}
	t.Log("comparable 68-session five-minute steady state: source reads 680 -> 68 after warmup; due liveness and real notifications remain targeted")
}

func TestHubSnapshotDeadlineMatchesStaleAndMovesWithHostedHeartbeat(t *testing.T) {
	for _, beat := range []int64{0, -1, 400, 30000} {
		snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: testNow.UnixMilli(), HeartbeatMs: beat}
		deadline := snapshotStaleAt(&snap)
		if snap.Stale(deadline.Add(-time.Millisecond)) || !snap.Stale(deadline) {
			t.Fatalf("deadline disagrees with reader for heartbeat %d", beat)
		}
	}
	if !snapshotStaleAt(nil).IsZero() {
		t.Fatal("absent cache produced a deadline")
	}
}
