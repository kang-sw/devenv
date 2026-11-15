package wsdashboard

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLifecycleDerivationContract(t *testing.T) {
	fresh := stampFor(testNow)
	stale := stampFor(testNow.Add(-LiveWindow - time.Second))
	for _, tc := range []struct {
		name   string
		reg    Registration
		hidden bool
		want   SessionLifecycle
	}{
		{"running hosted", Registration{Hosted: true, HeartbeatAt: fresh, HostPID: 123}, false, SessionActiveOn},
		{"visible asleep", Registration{Hosted: true, HeartbeatAt: fresh, HostStopped: true}, false, SessionAsleep},
		{"archived", Registration{Hosted: true, HeartbeatAt: fresh, HostStopped: true}, true, SessionArchived},
		{"stale open archived", Registration{Hosted: true, HeartbeatAt: stale, HostStopped: true}, true, SessionArchived},
		{"ended archived", Registration{Hosted: true, HeartbeatAt: fresh, EndedAt: fresh}, true, SessionPendingDeletion},
		{"stale visible", Registration{Hosted: true, HeartbeatAt: stale}, false, SessionPendingDeletion},
		{"terminal retained sidecar", Registration{HeartbeatAt: fresh}, true, SessionActiveOn},
		{"stale terminal retained sidecar", Registration{HeartbeatAt: stale}, true, SessionPendingDeletion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveSessionLifecycle(tc.reg, tc.hidden, testNow); got != tc.want {
				t.Fatalf("lifecycle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSessionLifecycleIndexSidecarAndClockContract(t *testing.T) {
	dir, root := t.TempDir(), t.TempDir()
	now := testNow
	reg := register(t, dir, "hosted", root, now)
	reg.Hosted, reg.HostStopped = true, true
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionHidden(dir, reg.ReplyID, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(SessionDir(dir, reg.ReplyID), SessionVisibilityFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	first, ok := idx.Session(reg.ReplyID)
	if !ok || !first.Hidden || first.Lifecycle != SessionArchived || !first.Live {
		t.Fatalf("initial projection = %+v", first)
	}
	// Registration-only writers, including old fixed-shape writers, must not own visibility.
	reg.HostPID = 42
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	idx.Reread(reg.ReplyID)
	idx.Settle()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("registration rewrite changed sidecar bytes")
	}
	if hidden, err := ReadSessionHidden(dir, reg.ReplyID); err != nil || !hidden {
		t.Fatalf("restored hidden = %v, %v", hidden, err)
	}
	archived, _ := idx.Session(reg.ReplyID)
	if err := WriteSessionHidden(dir, reg.ReplyID, false); err != nil {
		t.Fatal(err)
	}
	idx.Reread(reg.ReplyID)
	idx.Settle()
	unhidden, _ := idx.Session(reg.ReplyID)
	if unhidden.Hidden || unhidden.Lifecycle != SessionAsleep || unhidden.Rev <= archived.Rev {
		t.Fatalf("sidecar-only revision = %+v, previous = %+v", unhidden, archived)
	}
	if err := WriteSessionHidden(dir, reg.ReplyID, true); err != nil {
		t.Fatal(err)
	}
	idx.Reread(reg.ReplyID)
	idx.Settle()
	archived, _ = idx.Session(reg.ReplyID)
	now = now.Add(LiveWindow + time.Second)
	aged, _ := idx.Session(reg.ReplyID)
	if aged.Live || aged.Lifecycle != SessionArchived {
		t.Fatalf("aged archive = %+v", aged)
	}
	if err := WriteSessionHidden(dir, reg.ReplyID, false); err != nil {
		t.Fatal(err)
	}
	idx.Reread(reg.ReplyID)
	idx.Settle()
	visible, _ := idx.Session(reg.ReplyID)
	if visible.Hidden || visible.Lifecycle != SessionPendingDeletion || visible.Rev <= archived.Rev {
		t.Fatalf("sidecar reread = %+v, previous = %+v", visible, archived)
	}
	now = testNow
	awake, _ := idx.Session(reg.ReplyID)
	if !awake.Live || awake.Lifecycle != SessionAsleep {
		t.Fatalf("clock-only projection = %+v", awake)
	}
	// A retained sidecar adds no prune exemption for an otherwise eligible row.
	reg.Hosted = false
	reg.HeartbeatAt = stampFor(testNow.Add(-RegistryRetention - time.Hour))
	reg.RegisteredAt = reg.HeartbeatAt
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionHidden(dir, reg.ReplyID, true); err != nil {
		t.Fatal(err)
	}
	if n := PruneRegistrations(dir, testNow, RegistryRetention); n != 1 {
		t.Fatalf("pruned = %d, want 1", n)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("eligible session sidecar survived prune: %v", err)
	}
}
