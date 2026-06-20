package execjob

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// runSpawnChild and runSpawnParent are dispatched by the shared
// TestHelperProcess switch in execjob_test.go (the "spawnchild" / "spawnparent"
// cases) under GO_WANT_HELPER_PROCESS=1 — that re-exec entry point lives in the
// sibling file, not here.
//
// runSpawnChild is the leaf helper: it records its own PID so the test can poll
// liveness, then blocks long enough that only the cancel path should reap it.
func runSpawnChild(pidFile string) {
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// runSpawnParent is the root helper: it spawns one child (which records its PID)
// and then blocks. Cancelling the parent must reap the whole subtree, not just
// the parent PID.
func runSpawnParent(childPidFile string) {
	child := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "spawnchild", childPidFile)
	child.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	configureCommand(child) // inherit the same killable-group setup the runner uses
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// TestCancelProcessReapsChildTree proves cancelProcess terminates the entire
// spawned subtree (parent + child), not only the root PID. The kill is scoped
// to this test's own spawned tree (group/PID), so it never touches unrelated
// host processes. Table-driven on runtime.GOOS only to gate the OS-specific
// liveness probe; the spawn idiom (self-exec) is identical across platforms.
func TestCancelProcessReapsChildTree(t *testing.T) {
	childPidFile := t.TempDir() + "/child.pid"

	parent := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "spawnparent", childPidFile)
	parent.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	configureCommand(parent)
	if err := parent.Start(); err != nil {
		t.Fatalf("start parent: %v", err)
	}
	parentPID := parent.Process.Pid
	t.Cleanup(func() {
		_ = cancelProcess(parentPID)
		_, _ = parent.Process.Wait()
	})

	childPID := waitForChildPID(t, childPidFile)
	if !aliveForTest(childPID) {
		t.Fatalf("child %d not alive before cancel", childPID)
	}

	if err := cancelProcess(parentPID); err != nil {
		t.Fatalf("cancelProcess: %v", err)
	}
	// Reap the parent so its slot is released; the cancel must have reaped the
	// child independently (it is not the parent's direct waitpid target here).
	_, _ = parent.Process.Wait()

	if !pollUntilDead(childPID, 5*time.Second) {
		t.Fatalf("child %d still alive after cancel on %s; subtree not reaped", childPID, runtime.GOOS)
	}
}

func waitForChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(raw))); perr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child pid file %s never populated", pidFile)
	return 0
}

func pollUntilDead(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !aliveForTest(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !aliveForTest(pid)
}
