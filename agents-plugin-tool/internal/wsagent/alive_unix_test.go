//go:build (aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris) && !windows

package wsagent

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// aliveForTest reports whether pid refers to a live (non-zombie) process. A
// group-killed child the test did not waitpid() becomes a zombie, which
// syscall.Kill(pid,0) still reports as alive; on Linux we read /proc state and
// treat Z (zombie) / X (dead) as not alive, falling back to the signal-0 probe.
func aliveForTest(pid int) bool {
	if pid <= 0 {
		return false
	}
	if raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		s := string(raw)
		if idx := strings.LastIndex(s, ") "); idx >= 0 && idx+2 < len(s) {
			state := s[idx+2]
			return state != 'Z' && state != 'X' && state != 'x'
		}
	}
	return syscall.Kill(pid, 0) == nil
}
