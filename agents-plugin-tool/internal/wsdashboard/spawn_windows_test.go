//go:build windows

package wsdashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// consoleProbeEnv selects the helper mode of the test binary
// ("daemon" or "child"); consoleProbeOutEnv names the file the child writes.
const (
	consoleProbeEnv    = "WSDASHBOARD_CONSOLE_PROBE"
	consoleProbeOutEnv = "WSDASHBOARD_CONSOLE_PROBE_OUT"
)

var (
	procGetConsoleWindow      = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")
)

// consoleState is this process's console window handle and the ids of the
// processes attached to its console (none when it has no console).
func consoleState() (hwnd uintptr, pids []uint32) {
	hwnd, _, _ = procGetConsoleWindow.Call()
	var ids [64]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return hwnd, ids[:min(int(n), len(ids))]
}

// TestDaemonChildrenShareItsHiddenConsole: a process startDetached starts
// owns a console with no window, and a console child it starts the plain way
// (as the daemon starts git, ssh, shells) attaches to that console instead
// of getting a new, visible one. The console is the daemon's own: the test
// process is not attached to it. The test binary plays both: the "daemon"
// mode starts the "child" mode, which records its console state.
func TestDaemonChildrenShareItsHiddenConsole(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	switch os.Getenv(consoleProbeEnv) {
	case "daemon":
		child := exec.Command(self, "-test.run=^TestDaemonChildrenShareItsHiddenConsole$")
		child.Env = append(os.Environ(), consoleProbeEnv+"=child")
		if err := child.Run(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "child":
		hwnd, pids := consoleState()
		_ = os.WriteFile(os.Getenv(consoleProbeOutEnv), []byte(fmt.Sprint(hwnd, pids)), 0o644)
		os.Exit(0)
	}

	out := filepath.Join(t.TempDir(), "console.txt")
	t.Setenv(consoleProbeEnv, "daemon")
	t.Setenv(consoleProbeOutEnv, out)
	cmd, err := startDetached(self, []string{"-test.run=^TestDaemonChildrenShareItsHiddenConsole$"}, t.TempDir())
	if err != nil {
		t.Fatalf("startDetached: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("daemon helper: %v", err)
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		<-done // let the killed helper go before TempDir cleanup
		t.Fatal("daemon helper did not exit within 60s")
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("child wrote no console state: %v", err)
	}
	hwndText, pidsText, ok := strings.Cut(strings.TrimSpace(string(raw)), " ")
	if !ok {
		t.Fatalf("console state %q", raw)
	}
	if hwndText != "0" {
		t.Fatalf("child has console window %s, want none", hwndText)
	}
	pids := strings.Fields(strings.Trim(pidsText, "[]"))
	if len(pids) < 2 {
		t.Fatalf("child's console has processes %v, want >= 2 (shared with the daemon)", pids)
	}
	if !slices.Contains(pids, strconv.Itoa(cmd.Process.Pid)) {
		t.Fatalf("child's console processes %v lack the daemon %d", pids, cmd.Process.Pid)
	}
	if slices.Contains(pids, strconv.Itoa(os.Getpid())) {
		t.Fatalf("child's console processes %v include the test process %d, want the daemon's own console", pids, os.Getpid())
	}
}
