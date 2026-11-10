//go:build !windows

package wsholder

import "syscall"

// processAlive reports whether pid names a live process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// killHolder kills the holder pid with its process group. A holder leads
// its own group (startDetached), which a pipe child shares, so the child
// ends with it; a PTY child leads its own session and gets the hangup of
// its terminal closing.
func killHolder(pid int) error {
	if pid <= 1 {
		return syscall.EINVAL
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}
