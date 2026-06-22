//go:build windows

package execjob

// aliveForTest reports whether pid refers to a live process. Reuses the
// package's own Windows liveness probe; the zombie/exited-handle nuance is the
// deferred Phase 3 concern noted in process_windows.go, acceptable for the
// Phase 3 host run of the subtree-reap test.
func aliveForTest(pid int) bool {
	return processAlive(pid)
}
