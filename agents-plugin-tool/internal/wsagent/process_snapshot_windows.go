//go:build windows

package wsagent

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// snapshotWindowsProcesses enumerates the live process table via a Toolhelp32
// snapshot and returns each process's PID and parent PID. This is the Windows
// analogue of the Unix `ps -axo pid=,ppid=` table read used to reconstruct the
// process tree for a scoped, PID-only subtree kill.
func snapshotWindowsProcesses() ([]windowsProcess, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	var processes []windowsProcess
	for {
		processes = append(processes, windowsProcess{
			PID:  int(entry.ProcessID),
			PPID: int(entry.ParentProcessID),
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return processes, err
		}
	}
	return processes, nil
}

// terminateWindowsProcess opens a single process by PID and terminates it.
// It is strictly PID-scoped: no image-name matching, so it cannot reach
// unrelated host processes. A missing/already-exited process is not an error.
func terminateWindowsProcess(pid int) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			// Process already gone (invalid pid) — treat as reaped.
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}
