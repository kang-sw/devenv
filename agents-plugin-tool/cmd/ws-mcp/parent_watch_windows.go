//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/kang-sw/devenv/internal/mcp"
)

// startParentDeathWatch arms a Windows-only goroutine that self-terminates this
// serve process if its parent (the resident launcher) dies. On Windows the
// launcher blocks in subprocess.call as the parent of ws-mcp; a launcher
// force-kill would otherwise orphan this process and leave a stale state.sqlite
// lock that breaks the next connection (ticket 260724 hypothesis A). If the
// parent PID is unknown or its handle cannot be opened (already gone), the watch
// is simply not armed — never a spurious exit.
func startParentDeathWatch() {
	ppid := os.Getppid()
	if ppid <= 0 {
		return
	}
	go watchProcessExit(ppid, func() {
		mcp.RecordLifecycleEvent("process.parent_exited", map[string]any{
			"ppid":   ppid,
			"action": "self_terminate",
		})
		os.Exit(0)
	})
}

// watchProcessExit blocks until the process identified by pid exits, then calls
// onExit. Split out from startParentDeathWatch so tests can exercise the
// wait-then-callback core against a real short-lived process without invoking
// os.Exit. If the process handle cannot be opened, onExit is not called.
func watchProcessExit(pid int, onExit func()) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	// With INFINITE the wait resolves to either WAIT_OBJECT_0 (the process
	// actually exited) or WAIT_FAILED (a syscall-level error). Only fire onExit
	// on a real exit signal: on failure, leave the watch silently disarmed
	// rather than self-terminate a healthy server on an exotic syscall error.
	event, err := windows.WaitForSingleObject(h, windows.INFINITE)
	if err != nil || event != windows.WAIT_OBJECT_0 {
		return
	}
	onExit()
}
