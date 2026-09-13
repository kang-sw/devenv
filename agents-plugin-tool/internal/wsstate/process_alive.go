package wsstate

// ProcessAlive reports whether pid is a live (not-yet-exited) process on
// this machine, using this package's own cross-platform processAlive
// primitive (process_alive_unix.go / process_alive_windows.go), already
// exercised by orchestrator_lock.go and CI's windows-smoke build. Exported
// for callers outside this package that need the same same-machine
// liveness check (e.g. internal/mcp's mailbox presence liveness) rather
// than hand-rolling a second, platform-specific implementation.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return processAlive(pid)
}
