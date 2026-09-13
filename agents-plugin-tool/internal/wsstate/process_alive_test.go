package wsstate

import (
	"os"
	"os/exec"
	"testing"
)

// TestProcessAliveDetectsLiveAndExitedProcess verifies ProcessAlive's actual
// syscall-liveness behavior (not just its pid<=0 guard) on whichever
// platform this test runs on: this test's own live PID is reported alive,
// and a real child process that has genuinely exited is reported dead.
//
// The child re-execs this same test binary with -test.run=^$ (matches no
// tests) rather than an external command like "true": that keeps the test
// portable across the unix and windows builds CI exercises for this
// package (windows-smoke runs `go test ./...`), since it never depends on
// a platform-specific shell or binary being on PATH.
func TestProcessAliveDetectsLiveAndExitedProcess(t *testing.T) {
	if ProcessAlive(0) {
		t.Fatalf("PID 0 must not be reported alive")
	}
	if ProcessAlive(-1) {
		t.Fatalf("a negative PID must not be reported alive")
	}
	if !ProcessAlive(os.Getpid()) {
		t.Fatalf("this test's own live PID was reported not alive")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run a short-lived self-reexec child process: %v", err)
	}
	// Harden against PID reuse: confirm the child truly reached a terminated
	// (Exited) state before asserting its PID reads dead, so a "not alive"
	// result cannot be an artifact of the OS having recycled the PID for an
	// unrelated live process between Run() and the probe.
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatalf("child process did not reach an Exited terminal state: state=%v", cmd.ProcessState)
	}
	exitedPID := cmd.Process.Pid
	if ProcessAlive(exitedPID) {
		t.Fatalf("a real, already-exited child process PID %d was reported alive", exitedPID)
	}
}
