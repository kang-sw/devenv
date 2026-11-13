//go:build windows

package wsstate

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processParent looks pid up in a Toolhelp process snapshot.
func processParent(pid int) (int, string, bool) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, "", false
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(pid) {
			return int(entry.ParentProcessID), windows.UTF16ToString(entry.ExeFile[:]), true
		}
	}
	return 0, "", false
}
