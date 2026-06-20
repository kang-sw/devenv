//go:build windows

package execjob

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// DEFERRED (Phase 3): OpenProcess can report an exited-but-unreaped process
	// as alive (zombie/cached-handle issue). Left intentionally unguarded here;
	// the Phase 1 verification boundary is Linux-only. See ticket
	// 260620-chore-pre-shipping-windows-surface-verification.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}

// cancelProcess terminates the whole process subtree rooted at pid.
//
// This mirrors the Unix path's intent (syscall.Kill(-pgid, SIGKILL) on the
// spawned process group). Windows has no negative-PID group-signal primitive,
// so we enumerate the live process table via a Toolhelp32 snapshot, walk the
// PPID parent->child links rooted at pid, and TerminateProcess each PID
// individually. The kill is strictly PID-scoped (no image-name termination),
// so it cannot reach unrelated host processes. The contract stays best-effort:
// the first error is returned while remaining PIDs are still attempted.
func cancelProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	processes, listErr := processTree(pid)
	processIDs := map[int]bool{pid: true}
	for _, p := range processes {
		processIDs[p.pid] = true
	}
	var first error
	for p := range processIDs {
		if p <= 0 || p == os.Getpid() {
			continue
		}
		if err := terminateProcess(p); err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	return listErr
}

type procInfo struct{ pid, ppid int }

// processTree returns all descendants of root by walking parent->child links in
// a Toolhelp32 process snapshot (the Windows analogue of the Unix ps table read).
func processTree(root int) ([]procInfo, error) {
	all, err := snapshotProcesses()
	if err != nil {
		return nil, err
	}
	children := map[int][]procInfo{}
	for _, p := range all {
		children[p.ppid] = append(children[p.ppid], p)
	}
	var res []procInfo
	seen := map[int]bool{root: true}
	var walk func(int)
	walk = func(pp int) {
		for _, c := range children[pp] {
			if seen[c.pid] {
				continue
			}
			seen[c.pid] = true
			res = append(res, c)
			walk(c.pid)
		}
	}
	walk(root)
	return res, nil
}

func snapshotProcesses() ([]procInfo, error) {
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
	var res []procInfo
	for {
		res = append(res, procInfo{pid: int(entry.ProcessID), ppid: int(entry.ParentProcessID)})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return res, err
		}
	}
	return res, nil
}

// terminateProcess opens a single process by PID and terminates it. Strictly
// PID-scoped; an already-exited process (invalid pid) is not an error.
func terminateProcess(pid int) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}
