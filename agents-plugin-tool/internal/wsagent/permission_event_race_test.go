package wsagent

import (
	"runtime"
	"testing"
)

// A live mode observation must snapshot the record before passing it by
// value: even though permission fields are loop-owned, the copy also reads
// Sends, which Send updates concurrently under mu.
func TestPermissionLiveModeEventsDuringRecordUpdates(t *testing.T) {
	h := permissionHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, initLine("permission-event-race", "auto"), stateLine(HarnessRunning))
	h.sync(id, holder)
	c := h.runner.conv(id)

	stop, started, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		first := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Isolate Send's in-memory mutation and lock, without its disk
			// latency serializing away contention with the event handler.
			c.mu.Lock()
			c.sends++
			c.rec.Sends = c.sends
			c.mu.Unlock()
			if first {
				close(started)
				first = false
			}
			runtime.Gosched()
		}
	}()
	defer func() {
		close(stop)
		wait(t, "the record updater to finish", done)
	}()
	wait(t, "the concurrent record update", started)
	for i := range 64 {
		mode := "plan"
		if i%2 != 0 {
			mode = "auto"
		}
		if !c.do(func() { c.handle(Event{PermissionMode: mode}) }) {
			t.Fatal("conversation ended while observing permission modes")
		}
	}
	if rec := c.record(); rec.Sends <= 1 || rec.PermissionMode != "auto" {
		t.Fatalf("record after live mode observations and updates: sends=%d, mode=%q", rec.Sends, rec.PermissionMode)
	}
	if st := h.state(id); st.ActiveMode != "auto" || st.Status != StatusRunning {
		t.Fatalf("state after live mode observations and updates: %+v", st)
	}
}
