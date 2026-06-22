//go:build windows

package wsstate

import "golang.org/x/sys/windows"

// processAlive reports whether pid is a live (not-yet-exited) process.
//
// OpenProcess alone is insufficient on Windows: the kernel process object
// survives termination until every handle closes, so it keeps succeeding for an
// exited-but-unreaped process. A zero-timeout wait disambiguates — WAIT_TIMEOUT
// means still running, WAIT_OBJECT_0 means the object is signaled (exited) — so
// the recovery path does not keep a dead worker marked active.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false
	}
	return state == uint32(windows.WAIT_TIMEOUT)
}
