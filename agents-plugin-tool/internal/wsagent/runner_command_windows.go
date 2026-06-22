//go:build windows

package wsagent

import (
	"os/exec"
	"syscall"
)

func configureRunnerCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	// Mirror the Unix cmd.Cancel shape: when the synchronous runner's context
	// times out, Go kills only the root process unless we set cmd.Cancel.
	// Reuse the existing PID-scoped Toolhelp32 subtree kill so the whole child
	// tree is reaped — matching the Unix SIGKILL-to-pgid intent. Image-name
	// termination is forbidden (live-host safety; see Hard Constraint).
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cancelAsyncProcessTree(cmd.Process.Pid)
	}
}
