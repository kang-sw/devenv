package wsdashboard

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
)

const workspaceGitInitRoute = "/rail/workspaces/git-init"

// railPlainFolder resolves only a plain-folder workspace already shown by the
// rail, never a new path supplied by the browser. Git membership is checked
// again at submit time, including ancestors and symlink targets.
func (d *Daemon) railPlainFolder(path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	path = filepath.Clean(path)
	for _, ws := range d.workspaces.workspaces() {
		if !ws.Repo && samePath(ws.Path, path) {
			return ws.Path, true
		}
	}
	if d.index != nil {
		for _, s := range d.index.Sessions() {
			if !s.Unlisted && railRepoRoot(s.Registration) == "" && samePath(s.Registration.Project, path) {
				return s.Registration.Project, true
			}
		}
	}
	return "", false
}

func gitInitCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, wsgit.Executable(), args...)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	gitbatch.Bound(cmd)
	return cmd
}

func gitInitError(lang string, ctx context.Context, out []byte, err error) string {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return wsi18n.T(lang, "rail.gitinit.nogit")
	}
	if ctx.Err() != nil {
		return ctx.Err().Error()
	}
	return gitActionErrText(string(out), err)
}

// handleWorkspaceGitInit is an additive, local-only write with the same gate
// as worktree deletion. The only mutation is exactly `git init` in the listed
// folder: no branch options, staging, commits, or configuration writes.
func (d *Daemon) handleWorkspaceGitInit(w http.ResponseWriter, r *http.Request) {
	var req workspacePathRequest
	if !worktreeDeleteBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	path, ok := d.railPlainFolder(req.Path)
	if !ok {
		http.Error(w, wsi18n.T(lang, "js.rail.unknown"), http.StatusNotFound)
		return
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		http.Error(w, wsi18n.T(lang, "rail.ws.refuse.notdir", path), http.StatusConflict)
		return
	}
	release, ok := d.beginGitAction(canonical)
	if !ok {
		http.Error(w, wsi18n.T(lang, "git.busy"), http.StatusConflict)
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	out, err := gitInitCommand(ctx, canonical, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	if err == nil {
		http.Error(w, wsi18n.T(lang, "rail.gitinit.already"), http.StatusConflict)
		return
	}
	// Only Git's explicit non-repository diagnostic permits initialization.
	// Other failures (including unsafe ownership or an invalid gitdir) fail
	// closed, rather than creating a nested repository over them.
	var exit *exec.ExitError
	notRepo := errors.As(err, &exit) && exit.ExitCode() == 128 &&
		strings.HasPrefix(string(out), "fatal: not a git repository (or any")
	if !notRepo {
		http.Error(w, gitInitError(lang, ctx, out, err), http.StatusBadGateway)
		return
	}
	out, err = gitInitCommand(ctx, canonical, "init").CombinedOutput()
	if err != nil {
		http.Error(w, gitInitError(lang, ctx, out, err), http.StatusBadGateway)
		return
	}
	d.workspaces.sweep()
	d.rail.reread(path)
	d.broker.wake(railListRepoKey)
	writeJSON(w, http.StatusOK, workspaceAddAnswer{Root: path, Repo: true, Message: wsi18n.T(lang, "rail.gitinit.done", lastElem(path))})
}
