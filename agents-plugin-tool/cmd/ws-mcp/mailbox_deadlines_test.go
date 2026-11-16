package main

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

type deadlineSweepFixture struct {
	stores  []wsmailbox.QueueStore
	calls   map[wsmailbox.QueueStore]int
	next    map[wsmailbox.QueueStore]time.Time
	failure map[wsmailbox.QueueStore]error
	scans   int
}

func (f *deadlineSweepFixture) Stores() ([]wsmailbox.QueueStore, error) {
	f.scans++
	return f.stores, nil
}
func (f *deadlineSweepFixture) Target(path string) (wsmailbox.QueueStore, bool) {
	for _, store := range f.stores {
		if store.Path == path {
			return store, true
		}
	}
	return wsmailbox.QueueStore{}, false
}
func (f *deadlineSweepFixture) Sweep(store wsmailbox.QueueStore, _ time.Time, _ time.Duration) (int, time.Time, error) {
	f.calls[store]++
	return 0, f.next[store], f.failure[store]
}

func deadlineMaintenance() (*mailboxMaintenance, *deadlineSweepFixture) {
	work := &deadlineSweepFixture{
		stores: []wsmailbox.QueueStore{{Path: "reply"}, {Path: "machine", Scope: wsmailbox.ScopeMachine}},
		calls:  map[wsmailbox.QueueStore]int{}, next: map[wsmailbox.QueueStore]time.Time{}, failure: map[wsmailbox.QueueStore]error{},
	}
	return &mailboxMaintenance{work: work, threshold: func() time.Duration { return 3 * time.Minute }, wake: func() {}, log: io.Discard}, work
}

func TestMailboxMaintenanceIdleAndChangedStoreReadCounts(t *testing.T) {
	maintenance, work := deadlineMaintenance()
	now := time.Now()
	configReads := 0
	maintenance.threshold = func() time.Duration { configReads++; return 3 * time.Minute }
	if next := maintenance.step(now, nil); !next.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("empty scheduler wake = %v", next)
	}
	for i := 1; i < 10; i++ {
		maintenance.step(now.Add(time.Duration(i)*30*time.Second), nil)
	}
	if work.scans != 1 || configReads != 1 {
		t.Fatalf("idle rediscovery/config reads: %d/%d", work.scans, configReads)
	}
	for store, reads := range work.calls {
		if reads != 1 {
			t.Fatalf("idle store %v read %d times", store, reads)
		}
	}
	t.Log("comparable five-minute idle window: 10 full sweeps -> 1, no intermediate store/config reads")
	maintenance.step(now.Add(4*time.Minute), []string{"reply", "reply", "foreign"})
	if work.calls[work.stores[0]] != 2 || work.calls[work.stores[1]] != 1 || work.scans != 1 {
		t.Fatalf("changed-store processing touched other stores: %v scans=%d", work.calls, work.scans)
	}
	maintenance.step(now.Add(5*time.Minute), nil)
	if work.scans != 2 || work.calls[work.stores[1]] != 2 {
		t.Fatal("lost/external changes were not recovered at safety boundary")
	}
}

func TestMailboxMaintenanceExactDeadlineRetryAndRemovedStore(t *testing.T) {
	maintenance, work := deadlineMaintenance()
	now := time.Now()
	soon, failing := work.stores[0], work.stores[1]
	work.next[soon] = now.Add(7 * time.Second)
	work.failure[failing] = errors.New("transient")
	if next := maintenance.step(now, nil); !next.Equal(work.next[soon]) {
		t.Fatalf("future deadline delayed by failure: %v", next)
	}
	work.next[soon] = time.Time{}
	maintenance.step(now.Add(7*time.Second), nil)
	if work.calls[soon] != 2 || work.calls[failing] != 1 {
		t.Fatalf("precise timer processed wrong stores: %v", work.calls)
	}
	delete(work.failure, failing)
	maintenance.step(now.Add(30*time.Second), nil)
	if work.calls[failing] != 2 {
		t.Fatal("failed delivery did not retry")
	}
	// A new process builds from discovery, not the old in-memory deadlines.
	restart := &mailboxMaintenance{work: work, threshold: maintenance.threshold, wake: maintenance.wake, log: io.Discard}
	restart.step(now.Add(time.Minute), nil)
	if work.scans != 2 {
		t.Fatal("restart skipped initial discovery")
	}
	work.stores = []wsmailbox.QueueStore{soon}
	maintenance.step(now.Add(5*time.Minute), nil)
	if _, retained := maintenance.due[failing]; retained {
		t.Fatal("removed store kept a deadline")
	}
}
