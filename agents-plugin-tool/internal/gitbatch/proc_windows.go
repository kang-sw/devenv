//go:build windows

package gitbatch

import (
	"os/exec"

	"github.com/kang-sw/devenv/internal/execjob"
)

// configureKill kills git's whole process tree when the context ends.
// Windows has no process group to signal, and the default kill reaches only
// the process started: with Git for Windows that is the git.exe redirector
// (Git\cmd or Git\bin), and the real mingw64 git.exe it started, with any
// transport below it, keeps running and holding the pipes (and its working
// directory) after the redirector dies.
func configureKill(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return execjob.KillTree(cmd.Process.Pid)
	}
}
