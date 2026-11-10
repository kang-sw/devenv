package wsdoc

import (
	"os/exec"

	"github.com/kang-sw/devenv/internal/wsexec"
)

// gitCommand is the one way wsdoc runs git: `git -C root args...` on env,
// the environment of the caller the board is read for (nil: this process's
// own). A server the dashboard daemon hosts passes its connection's
// environment so git and its PATH, HOME, and GIT_* are the caller's, not the
// daemon's. wsdoc execs git itself rather than taking a wsgit runner: the
// {#260720-wsdoc-commit-boundary} rule keeps wsdoc from importing wsgit.
func gitCommand(env []string, root string, args ...string) *exec.Cmd {
	return wsexec.Command(env, "git", append([]string{"-C", root}, args...)...)
}
