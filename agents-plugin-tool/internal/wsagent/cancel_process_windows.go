//go:build windows

package wsagent

import (
	"os"
)

// cancelAsyncProcessTree terminates the whole process subtree rooted at pid.
//
// This mirrors the Unix path's intent (syscall.Kill(-pgid, SIGKILL) on the
// spawned process group): the spawn side sets CREATE_NEW_PROCESS_GROUP, but
// Windows has no negative-PID group-signal primitive, so we enumerate the live
// process table via a Toolhelp32 snapshot, walk the PPID parent->child links
// rooted at pid, and TerminateProcess each PID individually. The kill is
// strictly PID-scoped (no image-name termination), so it cannot reach unrelated
// host processes such as a live claude.exe. The contract stays best-effort: the
// first error is returned for the caller's cleanup_needed signal while we still
// attempt to terminate the remaining PIDs.
func cancelAsyncProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	processes, listErr := windowsProcessTree(pid)
	processIDs := map[int]bool{pid: true}
	for _, p := range processes {
		processIDs[p.PID] = true
	}
	var firstErr error
	for processID := range processIDs {
		if processID <= 0 || processID == os.Getpid() {
			continue
		}
		if err := terminateWindowsProcess(processID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return listErr
}

type windowsProcess struct {
	PID  int
	PPID int
}

// windowsProcessTree returns all descendants of rootPID by walking the
// parent->child relationships in a Toolhelp32 process snapshot.
func windowsProcessTree(rootPID int) ([]windowsProcess, error) {
	all, err := snapshotWindowsProcesses()
	if err != nil {
		return nil, err
	}
	children := make(map[int][]windowsProcess)
	for _, p := range all {
		children[p.PPID] = append(children[p.PPID], p)
	}
	var result []windowsProcess
	seen := map[int]bool{rootPID: true}
	var walk func(int)
	walk = func(parent int) {
		for _, child := range children[parent] {
			if seen[child.PID] {
				continue
			}
			seen[child.PID] = true
			result = append(result, child)
			walk(child.PID)
		}
	}
	walk(rootPID)
	return result, nil
}
