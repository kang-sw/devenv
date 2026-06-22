//go:build windows

package wsagent

import "golang.org/x/sys/windows"

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
func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// Cannot open the process (no such pid / fully reaped) -> not alive.
		return false, nil
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, err
	}
	return state == uint32(windows.WAIT_TIMEOUT), nil
}
