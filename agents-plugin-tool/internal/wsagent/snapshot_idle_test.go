package wsagent

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

type idleSnapshotSlot struct{ snapshot wslive.Snapshot }

func (s *idleSnapshotSlot) Publish(snapshot wslive.Snapshot) { s.snapshot = snapshot }
func (*idleSnapshotSlot) RingAppended()                      {}
func (*idleSnapshotSlot) Close()                             {}

func TestHostedSnapshotDedupAndDiskHeartbeat(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	w := newLiveWriter(dir, "idle", "pi", func() time.Time { return now }, io.Discard)
	slot := &idleSnapshotSlot{}
	w.slot = slot
	w.writeSnapshot()
	read := func() wslive.Snapshot {
		t.Helper()
		s, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	first := read()
	now = now.Add(time.Second)
	w.writeSnapshot()
	if got := read(); got.UpdatedAt != first.UpdatedAt || slot.snapshot.UpdatedAt != now.UnixMilli() {
		t.Fatalf("unchanged disk=%d slot=%d", got.UpdatedAt, slot.snapshot.UpdatedAt)
	}
	w.setAdapter("claude")
	if got := read(); got.Adapter != "claude" || got.UpdatedAt != now.UnixMilli() {
		t.Fatalf("changed snapshot not persisted: %+v", got)
	}
	for range 4 {
		now = now.Add(wslive.HeartbeatInterval)
		w.heartbeatSnapshot()
		got := read()
		if got.UpdatedAt != now.UnixMilli() || got.Stale(now) || got.Stale(now.Add(wslive.HeartbeatInterval)) {
			t.Fatalf("legacy disk heartbeat is stale: %+v", got)
		}
	}
	if !read().Stale(now.Add((wslive.StaleAfterBeats + 1) * wslive.HeartbeatInterval)) {
		t.Fatal("stopped writer did not become stale")
	}
}

func TestSnapshotDedupRequiresSuccessfulWrite(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(blocker, "session")
	now := time.Unix(1000, 0)
	w := newLiveWriter(dir, "idle", "pi", func() time.Time { return now }, io.Discard)
	w.slot = &idleSnapshotSlot{}
	w.writeSnapshot() // a file blocks the parent directory
	if len(w.lastSnapshot) != 0 {
		t.Fatal("failed write was remembered")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	w.writeSnapshot()
	if _, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir)); err != nil {
		t.Fatal(err)
	}
	w.slot = nil
	now = now.Add(time.Second)
	w.writeSnapshot()
	s, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
	if err != nil || s.UpdatedAt != now.Add(-time.Second).UnixMilli() {
		t.Fatalf("file-only writer did not deduplicate: %+v, %v", s, err)
	}
	w.heartbeatSnapshot()
	s, err = wslive.ReadSnapshot(wslive.SnapshotPath(dir))
	if err != nil || s.UpdatedAt != now.UnixMilli() || s.Stale(now) {
		t.Fatalf("file-only heartbeat missing: %+v, %v", s, err)
	}
}
