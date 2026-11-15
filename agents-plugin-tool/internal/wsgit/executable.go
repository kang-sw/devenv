package wsgit

import "github.com/kang-sw/devenv/internal/wsexec"

// Executable resolves process Git through wsexec's shared successful-entry
// cache. A failed resolution returns bare "git" so the command resolves it
// itself as before; failures are not cached. See wsexec.GitPath for reuse
// eligibility and the unchanged-key stale-success window.
func Executable() string {
	path, err := wsexec.GitPath(nil)
	if err != nil {
		return "git"
	}
	return path
}

// LookPathEnv resolves name against the PATH (and, on Windows, PATHEXT) of
// environ rather than this process's; see wsexec.LookPath. A name environ's
// PATH does not hold is an error, never this process's copy of it.
func LookPathEnv(name string, environ []string) (string, error) {
	if name == "git" {
		return wsexec.GitPath(environ)
	}
	return wsexec.LookPath(name, environ)
}
