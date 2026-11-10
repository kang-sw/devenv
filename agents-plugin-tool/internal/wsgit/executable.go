package wsgit

import (
	"os"
	"os/exec"
	"sync"

	"github.com/kang-sw/devenv/internal/wsexec"
)

// lookPath resolves the git executable (a test seam).
var lookPath = exec.LookPath

// resolved is the git executable Executable resolved and the search path it
// was resolved under; path is empty until a resolution succeeds.
var resolved struct {
	sync.Mutex
	path, under string
}

// Executable is the git executable's path, resolved once per search path: a
// bare "git" makes every command search each PATH entry (and, on Windows,
// each PATHEXT extension), and a dashboard reload spawns several. A changed
// PATH or PATHEXT resolves again (a process-wide cache pinned the first git
// in fake-git PATH tests). A failed resolution is not kept: it returns
// "git", so the command resolves it itself as before and a later PATH fix
// still takes.
func Executable() string {
	under := os.Getenv("PATH") + "\x00" + os.Getenv("PATHEXT")
	resolved.Lock()
	defer resolved.Unlock()
	if resolved.path == "" || resolved.under != under {
		path, err := lookPath("git")
		if err != nil {
			resolved.path = ""
			return "git"
		}
		resolved.path, resolved.under = path, under
	}
	return resolved.path
}

// LookPathEnv resolves name against the PATH (and, on Windows, PATHEXT) of
// environ rather than this process's; see wsexec.LookPath.
func LookPathEnv(name string, environ []string) string {
	return wsexec.LookPath(name, environ)
}
