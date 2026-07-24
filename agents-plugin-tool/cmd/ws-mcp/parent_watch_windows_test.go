//go:build windows

package main

import (
	"os/exec"
	"testing"
	"time"
)

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

// TestWatchProcessExit_NeverOpenablePID asserts that calling watchProcessExit
// on a PID that has already exited (and been reaped) returns without invoking
// onExit. There is a small, accepted PID-reuse race in principle (the OS could
// theoretically reassign the PID before OpenProcess runs), but in practice a
// freshly-reaped PID is not immediately reused within a test's lifetime.
func TestWatchProcessExit_NeverOpenablePID(t *testing.T) {
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
