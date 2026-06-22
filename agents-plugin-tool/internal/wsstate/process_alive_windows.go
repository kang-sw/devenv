//go:build windows

package wsstate

import "golang.org/x/sys/windows"

// openErrorMeansAlive reports whether an OpenProcess error indicates the
// process exists but cannot be opened at the requested access level. Mirrors
// the Unix EPERM→alive mapping: ERROR_ACCESS_DENIED means the process object
// exists; all other errors (e.g. ERROR_INVALID_PARAMETER = no such PID) mean
// the process is gone.
func openErrorMeansAlive(err error) bool {
	if err == nil {
		return false
	}
	errno, ok := err.(windows.Errno)
	return ok && errno == windows.ERROR_ACCESS_DENIED
}

// processAlive reports whether pid is a live (not-yet-exited) process.
//
// OpenProcess alone is insufficient on Windows: the kernel process object
// survives termination until every handle closes, so it keeps succeeding for an
// exited-but-unreaped process. A zero-timeout wait disambiguates — WAIT_TIMEOUT
// means still running, WAIT_OBJECT_0 means the object is signaled (exited) — so
// the recovery path does not keep a dead worker marked active.
//
// If OpenProcess returns ERROR_ACCESS_DENIED, the process exists but cannot be
// opened at the requested rights — treat as alive (mirrors Unix EPERM).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return openErrorMeansAlive(err)
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false
	}
	return state == uint32(windows.WAIT_TIMEOUT)
}
