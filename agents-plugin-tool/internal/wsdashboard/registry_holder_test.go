package wsdashboard

import (
	"testing"
	"time"
)

func TestRegistrationHolderVerdicts(t *testing.T) {
	alive := func(pid int) bool { return pid == 100 }
	host := []struct {
		name string
		row  Registration
		want bool
	}{
		{"a live process", Registration{Hosted: true, HostPID: 100}, true},
		{"a process that is gone", Registration{Hosted: true, HostPID: 200}, false},
		{"a spawn under way", Registration{Hosted: true}, true},
		{"stopped", Registration{Hosted: true, HostStopped: true}, false},
		{"closed", Registration{Hosted: true, EndedAt: "2026-01-02T03:04:05Z"}, false},
		// A process outlives a stale mark: the row is asked of the process.
		{"a live process on an ended row", Registration{Hosted: true, HostPID: 100, EndedAt: "2026-01-02T03:04:05Z"}, true},
	}
	for _, tc := range host {
		if got := tc.row.HostMayRun(alive); got != tc.want {
			t.Errorf("HostMayRun(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
	harness := []struct {
		name string
		row  Registration
		want bool
	}{
		{"a live harness", Registration{HarnessPID: 100}, true},
		{"a harness that is gone", Registration{HarnessPID: 200}, false},
		{"no recorded harness, not ended", Registration{HeartbeatAt: "2020-01-02T03:04:05Z"}, true},
		{"no recorded harness, ended", Registration{EndedAt: "2026-01-02T03:04:05Z"}, false},
	}
	for _, tc := range harness {
		if got := tc.row.HarnessMayRun(alive); got != tc.want {
			t.Errorf("HarnessMayRun(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLockRegistrationSerializesItsHolders(t *testing.T) {
	dir := t.TempDir()
	const id = "abc123"
	if _, err := LockRegistration(dir, "../escape"); err == nil {
		t.Fatal("an invalid reply-id was locked")
	}
	unlock, err := LockRegistration(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		second, err := LockRegistration(dir, id)
		if err == nil {
			second()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("a second holder took the lock while the first held it (err %v)", err)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("the second holder after the release: %v", err)
		}
	case <-time.After(registrationLockTimeout + time.Second):
		t.Fatal("the second holder never took the released lock")
	}
	// The lock leaves a row write as it was: no row exists yet.
	if _, err := ReadRegistration(dir, id); err == nil {
		t.Fatal("taking the lock wrote a row")
	}
}
