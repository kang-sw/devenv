//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package wsagent

import "testing"

func TestParseUnixProcessTable(t *testing.T) {
	processes := parseUnixProcessTable("  10     1    10\n  11    10    11\nbad row\n  12    11    11\n")
	if len(processes) != 3 {
		t.Fatalf("len(processes) = %d, want 3: %+v", len(processes), processes)
	}
	if processes[0] != (unixProcess{PID: 10, PPID: 1, PGID: 10}) ||
		processes[1] != (unixProcess{PID: 11, PPID: 10, PGID: 11}) ||
		processes[2] != (unixProcess{PID: 12, PPID: 11, PGID: 11}) {
		t.Fatalf("processes mismatch: %+v", processes)
	}
}
