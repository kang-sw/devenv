package wsstate

// ProcessParent reports the parent PID and executable image name (for example
// "cmd.exe") of the process pid. ok is false when the process is not found,
// the lookup failed, or the platform has no support for it; callers treat that
// as "unknown" and keep the PID they were given. Only Windows implements it
// (process_parent_windows.go), where a harness commonly starts a stdio server
// through a shell wrapper whose own PID is not the harness's.
func ProcessParent(pid int) (ppid int, image string, ok bool) {
	if pid <= 0 {
		return 0, "", false
	}
	return processParent(pid)
}
