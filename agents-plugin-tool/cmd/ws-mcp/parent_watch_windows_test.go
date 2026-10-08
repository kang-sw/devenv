//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// Env keys that turn a re-executed test binary into the wait's helper parent.
const (
	waitParentHelperBinEnv = "WS_MCP_TEST_WAIT_PARENT_BIN"
	waitParentHelperKeyEnv = "WS_MCP_TEST_WAIT_PARENT_SESSION_KEY"
)

// TestMailboxWaitParentHelperProcess is not a test on its own: it skips unless
// TestMailboxWait_EndsWhenParentExits re-executes the test binary as the
// wait's direct parent. It starts `mailbox wait` and blocks so the caller can
// terminate it, which on Windows orphans the wait the way a host stopping the
// launcher does.
func TestMailboxWaitParentHelperProcess(t *testing.T) {
	bin := os.Getenv(waitParentHelperBinEnv)
	if bin == "" {
		t.Skip("helper parent for TestMailboxWait_EndsWhenParentExits")
	}
	child := exec.Command(bin, "mailbox", "wait", "--session-key", os.Getenv(waitParentHelperKeyEnv), "--timeout", "60s")
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// TestMailboxWait_EndsWhenParentExits pins the orphaned-wait fix: when a
// mailbox wait's direct parent is terminated, the wait ends promptly through
// its interrupted path, clears its own listening marker, and records a
// cancel_wait parent-exit lifecycle event — instead of lingering until its
// --timeout.
func TestMailboxWait_EndsWhenParentExits(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	t.Setenv("WS_CACHE_HOME", cacheHomeFromEnv(t, env))

	const sessionKey = "amber-tide-fox"
	markerPath, err := wsmailbox.ListeningMarkerPath(sessionKey)
	if err != nil {
		t.Fatalf("ListeningMarkerPath: %v", err)
	}
	lifecycleLog := filepath.Join(filepath.Dir(filepath.Dir(markerPath)), "crash", "mcp-lifecycle.log")

	helper := exec.Command(os.Args[0], "-test.run=^TestMailboxWaitParentHelperProcess$")
	helper.Env = append(env, waitParentHelperBinEnv+"="+bin, waitParentHelperKeyEnv+"="+sessionKey)
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper parent: %v", err)
	}
	defer func() {
		_ = helper.Process.Kill()
		_ = helper.Wait()
	}()

	var waitPID int
	deadline := time.Now().Add(10 * time.Second)
	for {
		marker, ok, rerr := wsmailbox.ReadListeningMarker(sessionKey)
		if rerr == nil && ok {
			waitPID = marker.PID
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("listening marker not written within 10s at %s (err=%v)", markerPath, rerr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if waitPID == helper.Process.Pid {
		t.Fatalf("marker PID %d is the helper parent, want the wait child", waitPID)
	}

	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(waitPID))
	if err != nil {
		t.Fatalf("open wait process %d: %v", waitPID, err)
	}
	defer windows.CloseHandle(h)
	// PID-scoped cleanup through the handle opened above: never leave a wait
	// behind if the assertions below fail.
	defer func() { _ = windows.TerminateProcess(h, 1) }()

	// Give the wait time to open its parent-watch handle before the parent
	// dies; a parent already gone at open leaves the watch disarmed by design.
	time.Sleep(500 * time.Millisecond)

	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper parent: %v", err)
	}
	_ = helper.Wait()

	if ev, werr := windows.WaitForSingleObject(h, 5000); werr != nil || ev != windows.WAIT_OBJECT_0 {
		t.Fatalf("mailbox wait still running 5s after its parent exited (event=%#x err=%v)", ev, werr)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		t.Fatalf("GetExitCodeProcess: %v", err)
	}
	if code != mailboxWaitExitInterrupted {
		t.Fatalf("mailbox wait exit code = %d, want the interrupted exit %d", code, mailboxWaitExitInterrupted)
	}
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("listening marker still present after the orphaned wait exited: err=%v", statErr)
	}
	raw, err := os.ReadFile(lifecycleLog)
	if err != nil {
		t.Fatalf("read lifecycle log: %v", err)
	}
	if !strings.Contains(string(raw), `"process.parent_exited"`) || !strings.Contains(string(raw), `"cancel_wait"`) {
		t.Fatalf("lifecycle log lacks the wait's parent-exit event:\n%s", raw)
	}
}

// TestWatchProcessExit_FiresOnRealExit starts a short-lived helper process,
// arms watchProcessExit against it, kills it, and asserts the callback fires
// within a bounded timeout.
func TestWatchProcessExit_FiresOnRealExit(t *testing.T) {
	// ping -n keeps a headless process alive ~1s per echo without needing a
	// console; unlike `timeout`, it survives the redirected stdin that go test
	// hands its children (timeout exits immediately under redirection, which
	// would leave nothing to kill and make Process.Kill return ACCESS_DENIED).
	helper := exec.Command("ping", "127.0.0.1", "-n", "20")
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = helper.Process.Kill()
	}()

	done := make(chan struct{})
	go watchProcessExit(helper.Process.Pid, func() {
		close(done)
	})

	// Give the watcher time to open the handle before the process dies.
	time.Sleep(200 * time.Millisecond)

	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchProcessExit did not fire onExit within timeout")
	}
}

// TestWatchProcessExit_DeadPIDNeverFires asserts that arming watchProcessExit on
// a PID that has already exited (and been reaped) returns without invoking
// onExit. This is deterministic: watchProcessExit polls the handle with a zero
// timeout at open and disarms when it is already signaled, so whether
// OpenProcess fails outright (object gone) or succeeds against a lingering /
// reaped-then-reused object (already signaled), onExit never fires. The former
// "accepted PID-reuse race" is closed by that at-open guard.
func TestWatchProcessExit_DeadPIDNeverFires(t *testing.T) {
	helper := exec.Command("ping", "127.0.0.1", "-n", "20")
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	pid := helper.Process.Pid
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_ = helper.Wait()

	fired := make(chan struct{})
	go watchProcessExit(pid, func() {
		close(fired)
	})

	select {
	case <-fired:
		t.Fatal("watchProcessExit fired onExit for an already-dead pid")
	case <-time.After(1 * time.Second):
	}
}
