package wsagent

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is the self-exec spawn helper. It is a no-op unless
// GO_WANT_HELPER_PROCESS=1, matching the idiom in internal/execjob.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			switch args[i+1] {
			case "spawnparent":
				runSpawnParent(args[i+2])
			case "spawnchild":
				runSpawnChild(args[i+2])
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func runSpawnChild(pidFile string) {
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

func runSpawnParent(childPidFile string) {
	child := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "spawnchild", childPidFile)
	child.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	configureAsyncCommand(child) // same killable-group setup the async runner uses
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// TestCancelAsyncProcessTreeReapsChildTree proves cancelAsyncProcessTree
// terminates the whole spawned subtree (parent + child), not only the root PID,
// mirroring the Unix process-group kill intent. The kill is scoped to this
// test's own spawned tree (group/PID); it never touches unrelated host
// processes. Table-driven on runtime.GOOS only for the OS-specific liveness
// probe; the self-exec spawn idiom is identical across platforms.
func TestCancelAsyncProcessTreeReapsChildTree(t *testing.T) {
	childPidFile := t.TempDir() + "/child.pid"

	parent := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "spawnparent", childPidFile)
	parent.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	configureAsyncCommand(parent)
	if err := parent.Start(); err != nil {
		t.Fatalf("start parent: %v", err)
	}
	parentPID := parent.Process.Pid
	t.Cleanup(func() {
		_ = cancelAsyncProcessTree(parentPID)
		_, _ = parent.Process.Wait()
	})

	childPID := waitForChildPID(t, childPidFile)
	if !aliveForTest(childPID) {
		t.Fatalf("child %d not alive before cancel", childPID)
	}

	if err := cancelAsyncProcessTree(parentPID); err != nil {
		t.Fatalf("cancelAsyncProcessTree: %v", err)
	}
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
