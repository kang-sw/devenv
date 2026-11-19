package wsagent

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSnapshotBurstAtomicWriteCounts(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		t.Run(fmt.Sprintf("hosted=%v", hosted), func(t *testing.T) {
			now := time.Unix(1000, 0)
			w := newLiveWriter(t.TempDir(), "counted", "pi", func() time.Time { return now }, io.Discard)
			if hosted {
				w.slot = &idleSnapshotSlot{}
			}
			writes := 0
			w.writeSnapshotFile = func(path string, raw []byte, mode os.FileMode) error {
				writes++
				return writeFileAtomic(path, raw, mode)
			}
			w.writeSnapshot()
			w.apply(wslive.Event{Kind: wslive.KindTurnStart})
			for i := range 200 {
				now = now.Add(10 * time.Millisecond)
				w.apply(wslive.Event{Kind: wslive.KindReply, Text: fmt.Sprintf("delta %d", i)})
			}
			want := 202 // initial + turn start + each changed reply
			if hosted {
				want = 1
			}
			if writes != want {
				t.Fatalf("stream atomic writes=%d, want %d", writes, want)
			}
			if hosted && (w.slot.(*idleSnapshotSlot).snapshot.Lead.Reply == nil || w.slot.(*idleSnapshotSlot).snapshot.Lead.Reply.Text != "delta 199") {
				t.Fatal("disk throttle delayed RAM publication")
			}
			now = now.Add(wslive.HeartbeatInterval)
			w.heartbeatSnapshot()
			if writes != want+1 {
				t.Fatalf("changed heartbeat atomic writes=%d, want %d", writes, want+1)
			}
			w.apply(wslive.Event{Kind: wslive.KindTurnEnd})
			if writes != want+2 {
				t.Fatalf("turn end did not flush inside the disk interval: %d", writes)
			}
			w.setAdapter("")
			beforeClose := writes
			w.closeSlot()
			if writes != beforeClose+1 {
				t.Fatalf("close did not flush: %d -> %d", beforeClose, writes)
			}
			snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(w.dir))
			if err != nil || snap.Adapter != "" || snap.Lead.Phase != wslive.PhaseIdle {
				t.Fatalf("close projection = %+v, %v", snap, err)
			}
			t.Logf("200 streamed deltas: %d snapshot replacements (hosted=%v)", want, hosted)
		})
	}
}

func TestHostedSnapshotFailedFlushRetriesWithoutAdvancingClock(t *testing.T) {
	now := time.Unix(1000, 0)
	w := newLiveWriter(t.TempDir(), "retry", "pi", func() time.Time { return now }, io.Discard)
	w.slot = &idleSnapshotSlot{}
	w.writeSnapshot()
	last := w.lastDiskAt
	now = now.Add(wslive.HeartbeatInterval)
	writes, fail := 0, true
	w.writeSnapshotFile = func(path string, raw []byte, mode os.FileMode) error {
		writes++
		if fail {
			return fmt.Errorf("injected write failure")
		}
		return writeFileAtomic(path, raw, mode)
	}
	w.setAdapter("claude")
	if writes != 1 || w.lastDiskAt != last {
		t.Fatal("failed write advanced the disk clock")
	}
	fail = false
	w.writeSnapshot()
	w.writeSnapshot()
	if writes != 2 || w.lastDiskAt != now {
		t.Fatalf("failed change must retry once then deduplicate: writes=%d", writes)
	}
}

func TestFileOnlyRingWakeFollowsSnapshotWrite(t *testing.T) {
	w := newLiveWriter(t.TempDir(), "wake", "pi", time.Now, io.Discard)
	w.apply(wslive.Event{Kind: wslive.KindTurnStart})
	w.apply(wslive.Event{Kind: wslive.KindReply, Text: "streaming"})
	wakes := 0
	w.wake = func() {
		wakes++
		snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(w.dir))
		if err != nil || snap.Lead.Reply != nil {
			t.Fatalf("ring wake raced ahead of the finished snapshot: %+v, %v", snap, err)
		}
		ring, err := wslive.ReadRing(wslive.RingDir(w.dir))
		if err != nil || len(ring) != 1 || ring[0].Text != "finished" {
			t.Fatalf("ring wake preceded the append: %+v, %v", ring, err)
		}
	}
	w.apply(wslive.Event{Kind: wslive.KindReply, Text: "finished", Final: true})
	if wakes != 1 {
		t.Fatalf("ring wakes=%d, want 1", wakes)
	}
}
