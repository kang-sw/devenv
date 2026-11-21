//go:build windows

package wsdashboard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startDetached starts exe in its own process group with its own hidden
// console. It first asks to break away from the caller's job object,
// so a host that kills its job on exit does not take the daemon with it; a
// job that forbids breakaway refuses that start, and the daemon then starts
// inside the job (it ends with the host, and the next ws-mcp starts another).
//
// CREATE_NO_WINDOW, not DETACHED_PROCESS: a console-less parent makes Windows
// allocate a new, visible console for every console-subsystem child, so a
// DETACHED_PROCESS daemon flashes a window for each git/ssh/shell it runs.
// With a hidden console of its own, children and grandchildren inherit it and
// open no window, and the console is still not the host terminal's.
func startDetached(exe string, args []string, dir string) (*exec.Cmd, error) {
	return startWindowsDetached(exe, args, dir, false)
}

func startDetachedStrict(exe string, args []string, dir string, stdin *os.File) (*exec.Cmd, error) {
	// Electron retries only after the numeric breakaway-denied marker below.
	// Keep the same Ensure prestart ordering; only process creation mode changes.
	jobLifetime := os.Getenv("WS_MCP_DAEMON_JOB_LIFETIME") == "1"
	return startElectronDetachedWith(jobLifetime, func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		if stdin != nil {
			cmd.Stdin = stdin
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		return cmd, cmd.Start()
	}, func(message string) { fmt.Fprintln(os.Stderr, message) })
}

func startElectronDetachedWith(jobLifetime bool, start func(uint32) (*exec.Cmd, error), log func(string)) (*exec.Cmd, error) {
	flags := uint32(windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP)
	if jobLifetime {
		log("daemon spawn mode=job-lifetime (daemon may end when the containing job closes)")
	} else {
		flags |= windows.CREATE_BREAKAWAY_FROM_JOB
		log("daemon spawn mode=breakaway")
	}
	cmd, err := start(flags)
	if !jobLifetime && errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// Numeric, locale-independent signal: do not retry arbitrary spawn errors.
		log("WS_DAEMON_BREAKAWAY_DENIED=5")
	}
	return cmd, err
}

func startWindowsDetached(exe string, args []string, dir string, strict bool) (*exec.Cmd, error) {
	return startWindowsDetachedWith(strict, func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		return cmd, cmd.Start()
	})
}

func startWindowsDetachedWith(strict bool, start func(uint32) (*exec.Cmd, error)) (*exec.Cmd, error) {
	base := uint32(windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP)
	if cmd, err := start(base | windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return cmd, nil
	} else if strict {
		return nil, err // Never retry inside Electron's job.
	}
	return start(base)
}
