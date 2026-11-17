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
			writes, fail := 0, false
			w.writeSnapshotFile = func(path string, raw []byte, mode os.FileMode) error {
				writes++
				if fail {
					return fmt.Errorf("injected write failure")
				}
				return writeFileAtomic(path, raw, mode)
			}
			// Replay the previous policy as a control: file-only writes had
			// no deduplication, just like the forced-heartbeat path here.
			for range 21 {
				if hosted {
					w.writeSnapshot()
				} else {
					w.heartbeatSnapshot()
				}
				now = now.Add(time.Millisecond)
			}
			before := writes
			wantBefore := 21
			if hosted {
				wantBefore = 1
			}
			if before != wantBefore {
				t.Fatalf("control atomic writes=%d, want %d", before, wantBefore)
			}
			writes, w.lastSnapshot = 0, nil
			for range 21 {
				w.writeSnapshot()
				now = now.Add(time.Millisecond)
			}
			if writes != 1 {
				t.Fatalf("21 identical burst projections: atomic writes=%d, want 1", writes)
			}
			t.Logf("21 identical projections: control=%d atomic writes, after=%d (hosted=%v)", before, writes, hosted)
			w.heartbeatSnapshot()
			if writes != 2 {
				t.Fatalf("heartbeat writes=%d, want 2", writes)
			}
			fail = true
			w.setAdapter("claude")
			if writes != 3 {
				t.Fatal("changed own state was delayed")
			}
			fail = false
			w.writeSnapshot()
			w.writeSnapshot()
			if writes != 4 {
				t.Fatalf("failed change must retry once then deduplicate: writes=%d", writes)
			}
			snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(w.dir))
			if err != nil || snap.Adapter != "claude" || snap.UpdatedAt != now.UnixMilli() {
				t.Fatalf("own write not immediately visible: %+v, %v", snap, err)
			}
		})
	}
}
