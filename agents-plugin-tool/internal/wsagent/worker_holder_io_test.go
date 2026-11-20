package wsagent

import (
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
	"io"
	"os"
	"testing"
	"time"
)

// The same fixture runs in the pinned old tree through the downgrade script.
// Counters cover instrumented file APIs; fake holder transport adds no disk IO.
func TestWorkerHolderFileOperations(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	h.runner.Close()
	h.holders.restart()
	h.opts.heartbeat = time.Hour
	h.start()
	wait(t, "initial scan", h.runner.rejoined)
	before := wsstate.FileIOCounters()["file_io_total"]
	worker, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "medium", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	var holder string
	eventually(t, "held launch", func() bool {
		rec := h.runner.conv(worker.ID).record()
		holder = rec.HolderID
		return holder != "" && rec.HostPID > 0
	})
	h.holders.out(holder, `{"type":"agent_start"}`)
	eventually(t, "worker output applied", func() bool {
		c := h.runner.conv(worker.ID)
		var applied bool
		c.do(func() { applied = c.seq > 0 })
		return applied
	})
	h.quiesce(worker.ID)
	t.Logf("worker launch instrumented file operations: %d", wsstate.FileIOCounters()["file_io_total"]-before)
	h.runner.Close()
	h.holders.restart()
	before = wsstate.FileIOCounters()["file_io_total"]
	h.start()
	wait(t, "rejoin scan", h.runner.rejoined)
	eventually(t, "replay ends", func() bool {
		c := h.runner.conv(worker.ID)
		done := false
		c.do(func() { done = !c.replaying })
		return done
	})
	h.quiesce(worker.ID)
	t.Logf("worker rejoin instrumented file operations: %d", wsstate.FileIOCounters()["file_io_total"]-before)
	c := h.runner.conv(worker.ID)
	c.do(func() {
		before := wsstate.FileIOCounters()["file_io_total"]
		for i := 0; i < 100; i++ {
			_ = c.state()
			_ = c.canKick()
		}
		delta := wsstate.FileIOCounters()["file_io_total"] - before
		t.Logf("100 steady-state status/kick checks instrumented file operations: %d", delta)
		if delta != 0 {
			t.Errorf("steady state added file operations: %d", delta)
		}
	})
	if h.record(worker.ID).HolderID != holder {
		t.Fatal("measurement changed holder")
	}
}

// Owner (1862)=a accepts this inherited held-lead cursor cost for this change.
// Sequence zero reproduces the old direct-worker path with the same live writer.
func TestWorkerHolderInheritedCursorFileOperations(t *testing.T) {
	for _, held := range []bool{false, true} {
		name := "direct"
		if held {
			name = "held"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			now := time.Unix(1000, 0)
			r := NewRunner(Options{Dir: dir, Now: func() time.Time { return now }, Log: io.Discard})
			c := &conv{r: r, dir: dir, rec: Record{ReplyID: "probe", WSKey: "key", Root: dir}, turn: true}
			c.live = newLiveWriter(dir, "key", "claude", r.opts.Now, io.Discard)
			slot := &idleSnapshotSlot{}
			c.live.slot, c.live.wake = slot, slot.RingAppended
			c.live.writeSnapshotFile = func(string, []byte, os.FileMode) error { return nil }
			c.update(func(*Record) {})
			c.live.apply(wslive.Event{Kind: wslive.KindTurnStart})
			line := func(n uint64, final bool) {
				seq := uint64(0)
				if held {
					seq = n
				}
				c.handle(Event{Seq: seq, Live: &wslive.Event{Kind: wslive.KindReply, Text: "progress", Final: final}})
				c.handle(Event{Seq: seq, LineEnd: true})
			}
			probe := func(label string, f func()) {
				writes, ops := recordAtomicWrites(), wsstate.FileIOCounters()["file_io_total"]
				f()
				t.Logf("%s: record replacements=%d instrumented file ops=%d", label, recordAtomicWrites()-writes, wsstate.FileIOCounters()["file_io_total"]-ops)
			}
			probe("200 snapshot-only lines", func() {
				for i := uint64(1); i <= 200; i++ {
					line(i, false)
				}
			})
			probe("25 final ring lines", func() {
				before := recordAtomicWrites()
				for i := uint64(201); i <= 225; i++ {
					line(i, true)
				}
				want := int64(0)
				if held {
					want = 25
				}
				if got := recordAtomicWrites() - before; got != want {
					t.Errorf("ring line record replacements=%d, want %d", got, want)
				}
			})
			line(226, false)
			probe("dirty cursor heartbeat", func() {
				before := recordAtomicWrites()
				c.persistSeq()
				want := int64(0)
				if held {
					want = 1
				}
				if got := recordAtomicWrites() - before; got != want {
					t.Errorf("heartbeat record replacements=%d, want %d", got, want)
				}
			})
		})
	}
}
