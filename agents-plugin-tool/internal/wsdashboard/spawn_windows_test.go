//go:build windows

package wsdashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// consoleState is this process's console window handle and the number of
// processes attached to its console (0 when it has none).
func consoleState() (hwnd uintptr, procs uintptr) {
	hwnd, _, _ = procGetConsoleWindow.Call()
	var ids [64]uint32
	procs, _, _ = procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return hwnd, procs
}

// TestDaemonChildrenShareItsHiddenConsole: a process startDetached starts
// owns a console with no window, and a console child it starts the plain way
// (as the daemon starts git, ssh, shells) attaches to that console instead
// of getting a new, visible one. The test binary plays both: the "daemon"
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
		hwnd, procs := consoleState()
		_ = os.WriteFile(os.Getenv(consoleProbeOutEnv), []byte(fmt.Sprintf("%d %d", hwnd, procs)), 0o644)
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
		t.Fatal("daemon helper did not exit within 60s")
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("child wrote no console state: %v", err)
	}
	var hwnd, procs uintptr
	if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d %d", &hwnd, &procs); err != nil {
		t.Fatalf("console state %q: %v", raw, err)
	}
	if hwnd != 0 {
		t.Fatalf("child has console window %#x, want none", hwnd)
	}
	if procs < 2 {
		t.Fatalf("child's console has %d processes, want >= 2 (shared with the daemon)", procs)
	}
}
