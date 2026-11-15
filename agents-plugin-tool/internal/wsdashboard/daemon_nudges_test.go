package wsdashboard

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

type daemonNudgeFixture struct{ start, stop func() }

func (f daemonNudgeFixture) Start() { f.start() }
func (f daemonNudgeFixture) Close() { f.stop() }

func TestDaemonLeadNudgesFollowSingleInstanceLockWithoutPages(t *testing.T) {
	dir := t.TempDir()
	var starts, stops atomic.Int32
	checkHeld := func() {
		lock := flock.New(filepath.Join(dir, lockFileName))
		ok, err := lock.TryLock()
		if err != nil {
			t.Error(err)
		}
		if ok {
			_ = lock.Unlock()
			t.Error("nudge lifecycle outside single-instance lock")
		}
	}
	service := daemonNudgeFixture{start: func() { checkHeld(); starts.Add(1) }, stop: func() { checkHeld(); stops.Add(1) }}
	d, err := StartDaemon(DaemonOptions{Dir: dir, Port: 0, Version: "test", LeadNudges: service})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.index != nil || d.source != nil || starts.Load() != 1 {
		t.Fatal("scheduler depends on pages/index")
	}
	_, err = StartDaemon(DaemonOptions{Dir: dir, Port: 0, Version: "test", LeadNudges: service, LockWait: 20 * time.Millisecond})
	if !errors.Is(err, ErrLocked) || starts.Load() != 1 {
		t.Fatal("losing daemon started scheduler", err)
	}
	d.Close()
	d.Close()
	if stops.Load() != 1 {
		t.Fatal("scheduler did not stop once")
	}
	lock := flock.New(filepath.Join(dir, lockFileName))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		t.Fatal("daemon did not release lock", err)
	}
	_ = lock.Unlock()
}
