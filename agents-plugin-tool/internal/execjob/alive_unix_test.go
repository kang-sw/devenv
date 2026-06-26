//go:build (aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris) && !windows

package execjob

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// aliveForTest reports whether pid refers to a live (non-zombie) process.
// Tests cannot rely on processAlive alone: a group-killed child the test did
// not waitpid() becomes a zombie, and syscall.Kill(pid,0) still reports a
// zombie as alive. On Linux we read /proc state and treat Z (zombie) / X
// (dead) as not alive; elsewhere we fall back to the signal-0 probe.
func aliveForTest(pid int) bool {
	if pid <= 0 {
		return false
	}
	if raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		// /proc/<pid>/stat: "<pid> (comm) <state> ..."; state is the field
		// right after the parenthesized comm (which may contain spaces).
		s := string(raw)
		if idx := strings.LastIndex(s, ") "); idx >= 0 && idx+2 < len(s) {
			state := s[idx+2]
			return state != 'Z' && state != 'X' && state != 'x'
		}
	}
	return syscall.Kill(pid, 0) == nil
}
