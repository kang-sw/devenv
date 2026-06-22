//go:build windows

package wsagent

import "golang.org/x/sys/windows"

// openErrorMeansAlive reports whether an OpenProcess error indicates the
// process exists but cannot be opened at the requested access level. This
// mirrors the Unix EPERM→alive mapping: ERROR_ACCESS_DENIED means the kernel
// process object exists (the process is alive) but the caller lacks the
// required rights. All other open errors (e.g. ERROR_INVALID_PARAMETER = no
// such PID, fully reaped) mean the process is not alive.
func openErrorMeansAlive(err error) bool {
	if err == nil {
		return false
	}
	errno, ok := err.(windows.Errno)
	return ok && errno == windows.ERROR_ACCESS_DENIED
}

// processAlive reports whether pid is a live (not-yet-exited) process.
//
// A plain OpenProcess success is insufficient on Windows: the kernel process
// object survives termination until every handle to it closes, so OpenProcess
// keeps succeeding for an exited-but-unreaped process (the zombie/cached-handle
// case the recovery path must not mistake for a live worker). We therefore probe
// the object's signaled state with a zero-timeout wait: WAIT_TIMEOUT means the
// process is still running, WAIT_OBJECT_0 means the object is signaled (the
// process has exited). This gives the recovery path an accurate liveness signal
// after a cancel/terminate, matching the Unix path's intent.
//
// If OpenProcess returns ERROR_ACCESS_DENIED, the process exists but cannot be
// opened at the requested rights — treat as alive (mirrors Unix EPERM).
func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if openErrorMeansAlive(err) {
			return true, nil
		}
		// No such pid / fully reaped -> not alive.
		return false, nil
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, err
	}
	return state == uint32(windows.WAIT_TIMEOUT), nil
}
