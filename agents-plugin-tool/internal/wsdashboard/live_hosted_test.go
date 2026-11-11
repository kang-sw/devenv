package wsdashboard

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// entryCounts are the entry cache's ring listings, drained listings, and
// held reads so far.
func (f *pageFixture) entryCounts() [3]int {
	c := f.d.entryFiles
	c.mu.Lock()
	defer c.mu.Unlock()
	return [3]int{c.ringLists, c.drainedLists, c.heldReads}
}

// TestHostedSlotRendersLiveOnly pins the daemon side of a hosted session:
// a published snapshot is served in place of the file and renders with a
// live-only wake that reads no session store but the snapshot (a ring
// append adds the ring); a heartbeat publish wakes nothing; neither raises
// the index revision, though SnapshotUpdatedAt follows; closing the slot
// serves the file again with a full re-read.
func TestHostedSlotRendersLiveOnly(t *testing.T) {
	f := startPageFixture(t)
	if _, ok := f.d.OpenHostedSlot(filepath.Join(t.TempDir(), "livesession")); ok {
		t.Fatal("a slot opened for a directory the daemon does not serve")
	}
	live := openStream(t, f.base+"/s/livesession/events")
	quiet(t, live)
	slot, ok := f.d.OpenHostedSlot(SessionDir(f.dir, "livesession"))
	if !ok {
		t.Fatal("no slot for a served session")
	}
	before, _ := f.d.Index().Session("livesession")
	reads, entries := f.src.readCount("livesession"), f.entryCounts()

	at := time.Now().Add(time.Second).Truncate(time.Millisecond)
	snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: at.UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "hosted copy reply"}}}
	slot.Publish(snap)
	waitRegionHolding(t, live, regionConvLive, "hosted copy reply")
	quiet(t, live)
	if got := f.src.readCount("livesession"); got != reads {
		t.Errorf("a live-only render read the page source %d times, want 0", got-reads)
	}
	if got := f.entryCounts(); got != entries {
		t.Errorf("a live-only render made entry reads %v, want none (was %v)", got, entries)
	}
	after, _ := f.d.Index().Session("livesession")
	if after.Rev != before.Rev {
		t.Errorf("a publish raised the index revision %d -> %d", before.Rev, after.Rev)
	}
	if !after.SnapshotUpdatedAt.Equal(at) {
		t.Errorf("SnapshotUpdatedAt = %v, want %v", after.SnapshotUpdatedAt, at)
	}
	if file, err := wslive.ReadSnapshot(wslive.SnapshotPath(SessionDir(f.dir, "livesession"))); err != nil || file.Lead.Reply == nil || file.Lead.Reply.Text != "streamed <partial> reply" {
		t.Fatalf("snapshot file = %+v, %v; a publish must not write it", file.Lead.Reply, err)
	}

	// A heartbeat moves only updatedAt: no wake, the index follows.
	wakes := f.d.stats.WakeLive.Load()
	snap.UpdatedAt += 30000
	slot.Publish(snap)
	if got := f.d.stats.WakeLive.Load(); got != wakes {
		t.Errorf("a heartbeat publish woke %d times", got-wakes)
	}
	if s, _ := f.d.Index().Session("livesession"); s.SnapshotUpdatedAt.UnixMilli() != snap.UpdatedAt {
		t.Errorf("SnapshotUpdatedAt after a heartbeat = %v, want %d", s.SnapshotUpdatedAt, snap.UpdatedAt)
	}

	// A ring append re-reads the ring alone.
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "appended while hosted"}}); err != nil {
		t.Fatal(err)
	}
	entries = f.entryCounts()
	slot.RingAppended()
	waitRegionHolding(t, live, regionEntries, "appended while hosted")
	quiet(t, live)
	got := f.entryCounts()
	if got[0] <= entries[0] || got[1] != entries[1] || got[2] != entries[2] {
		t.Errorf("a ring append's render made entry reads %v from %v, want the ring listing alone", got, entries)
	}
	if got := f.src.readCount("livesession"); got != reads {
		t.Errorf("a ring render read the page source %d times, want 0", got-reads)
	}

	// Closed, the file is the snapshot again.
	slot.Close()
	waitRegionHolding(t, live, regionConvLive, "streamed &lt;partial&gt; reply")
	if got := f.src.readCount("livesession"); got == reads {
		t.Error("the render after the slot closed did not re-read the page")
	}
	// A closed slot publishes nothing.
	slot.Publish(snap)
	if s := f.d.snapshots.Snapshot(f.dir, "livesession"); s == nil || s.Lead.Reply == nil || s.Lead.Reply.Text != "streamed <partial> reply" {
		t.Fatalf("snapshot after close = %+v", s)
	}
}

// TestHostedSlotTakeover pins slot ownership: a slot opened later for the
// same session takes the copy over, and the earlier slot's publish and
// close no longer touch it.
func TestHostedSlotTakeover(t *testing.T) {
	f := startPageFixture(t)
	dir := SessionDir(f.dir, "livesession")
	first, _ := f.d.OpenHostedSlot(dir)
	second, _ := f.d.OpenHostedSlot(dir)
	snap := func(text string) wslive.Snapshot {
		return wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: text}}}
	}
	reply := func() string {
		s := f.d.snapshots.Snapshot(f.dir, "livesession")
		if s == nil || s.Lead.Reply == nil {
			return ""
		}
		return s.Lead.Reply.Text
	}
	second.Publish(snap("second"))
	first.Publish(snap("first"))
	if got := reply(); got != "second" {
		t.Fatalf("served reply = %q, want the newer slot's", got)
	}
	first.Close()
	if got := reply(); got != "second" {
		t.Fatalf("served reply after the older slot closed = %q, want the newer slot's", got)
	}
	second.Close()
	if got := reply(); got != "streamed <partial> reply" {
		t.Fatalf("served reply after the owner closed = %q, want the file's", got)
	}
}
