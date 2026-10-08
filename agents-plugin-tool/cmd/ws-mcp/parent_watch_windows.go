//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// startParentDeathWatch arms a Windows-only goroutine that calls onExit with
// the parent PID when this process's direct parent dies. On Windows the
// launcher blocks in subprocess.call as the parent of ws-mcp, and a host that
// stops its child terminates only that launcher, so without the watch this
// process is orphaned: serve would keep a stale state.sqlite lock (ticket
// 260724 hypothesis A) and mailbox wait would linger until its --timeout. The
// caller decides the response (serve self-terminates; mailbox wait cancels and
// clears its marker). If the parent PID is unknown or its handle cannot be
// opened (already gone), the watch is simply not armed — never a spurious
// onExit.
func startParentDeathWatch(onExit func(ppid int)) {
	ppid := os.Getppid()
	if ppid <= 0 {
		return
	}
	go watchProcessExit(ppid, func() { onExit(ppid) })
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
	// PID-reuse identity guard: only self-terminate when we witness the process
	// we opened transition from alive to dead. A zero-timeout wait that reports
	// WAIT_OBJECT_0 means the handle is *already* signaled at open time, so we did
	// not open a live parent — we opened an already-exited object (a lingering
	// process object, or a PID the OS reaped and recycled). Firing here would be a
	// spurious self-terminate, so disarm instead. startParentDeathWatch arms while
	// the parent is alive, so a healthy parent is never already-signaled at open.
	if ev, werr := windows.WaitForSingleObject(h, 0); werr == nil && ev == windows.WAIT_OBJECT_0 {
		return
	}
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
