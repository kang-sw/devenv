package wsagent

import (
	"github.com/kang-sw/devenv/internal/wsstate"
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
