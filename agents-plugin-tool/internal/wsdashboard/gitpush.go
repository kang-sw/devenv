package wsdashboard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The git drawer's actions, each a POST from its action panel that runs on
// the session root's current branch and nothing else, never a force-like
// option, never another branch:
//
//   - push sends the branch to its configured upstream; the daemon re-reads
//     the branch first and refuses a detached HEAD, a branch without an
//     upstream, and one with nothing to push.
//   - pull fast-forwards the branch to its upstream and never merges: it
//     refuses a detached HEAD, a branch without an upstream, and a working
//     tree with uncommitted changes to tracked files before it fetches, and a
//     branch that has diverged from its upstream once the fetch landed.
//   - fetch runs a plain `git fetch` (the branch's remote, else origin).
//
// One action of a root runs at a time.

const (
	// gitActionTimeout bounds one action, network included.
	gitActionTimeout = 60 * time.Second
)

// actionReads is the runner an action's branch reads go through, given the
// runner that records their failure; a test wraps it to see each read.
var actionReads = func(run *gitBadgeRunner) wsgit.Runner { return run }

// pushRefusal is why the daemon refused a push before git ran.
type pushRefusal int

const (
	pushRan        pushRefusal = iota // not refused: git push ran
	pushDetached                      // HEAD names no branch
	pushNoUpstream                    // the branch has no usable upstream
	pushNothing                       // the branch is not ahead of it
)

// pushGit pushes root's current branch to its upstream. A refusal comes
// with a nil error; err is a failure of the re-read or of the push, its
// message git's first meaningful line, bounded.
func pushGit(root string) (pushRefusal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	run := &gitBadgeRunner{timeout: gitActionTimeout}
	reads := actionReads(run)
	st, ok := wsgit.BranchSync(ctx, reads, root)
	if !ok {
		return pushRan, branchErr(run)
	}
	// A failed count leaves Ahead 0, which must not read as nothing to
	// push. With a branch and an upstream, the only failure BranchSync
	// passes over is the count's.
	switch {
	case st.Head == "":
		return pushDetached, nil
	case st.Upstream == "":
		return pushNoUpstream, nil
	case run.err != nil:
		return pushRan, branchErr(run)
	case st.Ahead == 0:
		return pushNothing, nil
	}
	remote, merge, ok := upstreamOf(ctx, reads, root, st.Head)
	if !ok {
		return pushNoUpstream, nil
	}
	return pushRan, runGitAction(ctx, root, gitPushArgs(remote, st.Head, merge)...)
}

// pullOutcome is how a pull ended short of a failure.
type pullOutcome int

const (
	pullRan        pullOutcome = iota // the branch was fast-forwarded
	pullCurrent                       // nothing to pull: not behind its upstream
	pullDetached                      // HEAD names no branch
	pullNoUpstream                    // the branch has no usable upstream
	pullDirty                         // tracked files have uncommitted changes
	pullDiverged                      // ahead of and behind its upstream
)

// pullGit fast-forwards root's current branch to its upstream: it refuses
// before the fetch when the branch cannot pull or the tree is dirty, fetches
// the upstream's remote (none for a local upstream), and merges only when the
// branch is behind and not ahead, with --ff-only so git refuses any other
// merge too. Untracked files do not make the tree dirty: git's own merge
// refuses one the fast-forward would overwrite.
func pullGit(root string) (pullOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	run := &gitBadgeRunner{timeout: gitActionTimeout}
	reads := actionReads(run)
	st, ok := wsgit.BranchSync(ctx, reads, root)
	if !ok {
		return pullRan, branchErr(run)
	}
	switch {
	case st.Head == "":
		return pullDetached, nil
	case st.Upstream == "":
		return pullNoUpstream, nil
	}
	remote, _, ok := upstreamOf(ctx, reads, root, st.Head)
	if !ok {
		return pullNoUpstream, nil
	}
	counts, ok := wsgit.WorkingTreeCounts(ctx, reads, root)
	if !ok {
		return pullRan, branchErr(run)
	}
	if counts.Modified > 0 || counts.Staged > 0 {
		return pullDirty, nil
	}
	if remote != "." {
		if err := runGitAction(ctx, root, "fetch", "--", remote); err != nil {
			return pullRan, err
		}
	}
	run.err = nil
	if st, ok = wsgit.BranchSync(ctx, reads, root); !ok {
		return pullRan, branchErr(run)
	}
	// The fetch may have pruned the upstream, or the count may have failed:
	// either leaves Behind 0, which must not read as nothing to pull. With
	// a branch and an upstream, the only failure BranchSync passes over is
	// the count's.
	switch {
	case st.Head == "":
		return pullDetached, nil
	case st.Upstream == "":
		return pullNoUpstream, nil
	case run.err != nil:
		return pullRan, branchErr(run)
	case st.Behind == 0:
		return pullCurrent, nil
	case st.Ahead > 0:
		return pullDiverged, nil
	}
	return pullRan, runGitAction(ctx, root, "merge", "--ff-only", "--quiet", "@{upstream}")
}

// fetchGit runs a plain fetch in root.
func fetchGit(root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	return runGitAction(ctx, root, "fetch")
}

// branchErr is a failed branch read's error.
func branchErr(run *gitBadgeRunner) error {
	if run.err == nil {
		return fmt.Errorf("HEAD names no commit")
	}
	return fmt.Errorf("%s", gitErrText(run.err))
}

// runGitAction runs one git command of an action in root. Its error is git's
// first meaningful line, bounded.
func runGitAction(ctx context.Context, root string, args ...string) error {
	cmd := gitActionCommand(ctx, root, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out after %s", gitActionTimeout)
		}
		return fmt.Errorf("%s", gitActionErrText(string(out), err))
	}
	return nil
}

// gitActionCommand is one action's git in root under the C locale, which
// keeps git's messages comparable, bounded by ctx (gitbatch.Bound). A remote
// command (push, fetch) also runs with no terminal prompt and ssh in batch
// mode (gitbatch.SSHCommand): the daemon's console is hidden
// (spawn_windows.go), so a passphrase or host-key prompt there would wait
// unseen until the timeout; it fails at once instead. Unlike the background
// callers it keeps credential helpers interactive (no gitbatch.RemoteArgs or
// RemoteEnv): the action is user-initiated, and a credential manager's GUI
// sign-in dialog is answerable where a console prompt is not.
func gitActionCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	remote := len(args) > 0 && (args[0] == "push" || args[0] == "fetch")
	cmd := gitActionGit(ctx, root, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if remote {
		cmd.Env = append(cmd.Env,
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND="+gitActionSSHCommand(ctx, root))
	}
	return cmd
}

// gitActionGit is git in root, bounded by ctx (gitbatch.Bound).
func gitActionGit(ctx context.Context, root string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	gitbatch.Bound(cmd)
	return cmd
}

// gitActionSSHCommand is gitbatch.SSHCommand over the process environment
// and root's core.sshCommand.
func gitActionSSHCommand(ctx context.Context, root string) string {
	return gitbatch.SSHCommand(os.Getenv, func() string {
		out, err := gitActionGit(ctx, root, "config", "--get", "core.sshCommand").Output()
		if err != nil {
			return ""
		}
		return string(out)
	})
}

// upstreamOf is branch's configured upstream: its remote and the remote
// ref it merges. ok is false when either is unset, or when the remote would
// read as an option.
func upstreamOf(ctx context.Context, run wsgit.Runner, root, branch string) (remote, merge string, ok bool) {
	get := func(key string) string {
		out, err := run.RunGit(ctx, root, "config", "--get", "branch."+branch+"."+key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	remote, merge = get("remote"), get("merge")
	if remote == "" || merge == "" || strings.HasPrefix(remote, "-") {
		return "", "", false
	}
	return remote, merge, true
}

// gitPushArgs is the push command line: branch's own ref onto the upstream's
// ref, on remote. It names the source ref rather than HEAD, so a checkout
// that lands between the re-read and the push cannot push another branch,
// and it carries no option: the refspec starts with "refs/", never "+", so
// git updates the remote only by fast-forward.
func gitPushArgs(remote, branch, merge string) []string {
	return []string{"push", "--", remote, "refs/heads/" + branch + ":" + merge}
}

// gitActionErrText is a failed action's message for the drawer: the line of
// git's output that says what went wrong, with its runs of spaces folded,
// bounded. A ref's rejection line ("! [rejected]", "! [remote rejected]")
// comes first, then an "error:" or "fatal:" line, then the first other line
// that is not the destination or source line, a hint, or blank, so a hook's
// "remote: ..." chatter never hides the reason.
func gitActionErrText(out string, err error) string {
	best, bestRank := "", 3
	for _, line := range strings.Split(out, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		rank := 2
		switch {
		case line == "" || strings.HasPrefix(line, "To ") || strings.HasPrefix(line, "From ") || strings.HasPrefix(line, "hint:"):
			continue
		case strings.HasPrefix(line, "! ["):
			rank = 0
		case strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "fatal:"):
			rank = 1
		}
		if rank < bestRank {
			best, bestRank = line, rank
		}
	}
	if best != "" {
		return gitErrText(fmt.Errorf("%s", best))
	}
	return gitErrText(err)
}

// beginGitAction claims root's one action slot; false while another action
// of it runs. The release returned with true frees it.
func (d *Daemon) beginGitAction(root string) (release func(), ok bool) {
	key := gitKey(root)
	d.gitActMu.Lock()
	defer d.gitActMu.Unlock()
	if d.gitActing[key] {
		return nil, false
	}
	if d.gitActing == nil {
		d.gitActing = map[string]bool{}
	}
	d.gitActing[key] = true
	return func() {
		d.gitActMu.Lock()
		delete(d.gitActing, key)
		d.gitActMu.Unlock()
	}, true
}

// gitActions are the action routes' names, as in /s/<reply-id>/git/<name>.
var gitActions = map[string]bool{"push": true, "pull": true, "fetch": true}

// handleGitAction serves POST /s/<reply-id>/git/<action>: push, pull, or
// fetch on the session root. It needs the daemon's own Origin, as the
// refresh route does, and answers once the action ended: 204 when it
// succeeded (the handle's re-read then lands through the stream), 200 with
// a note when a pull found nothing to pull, 409 with the reason when the
// daemon refused it or another action of the root runs, and 502 with git's
// first meaningful line when it failed. Every answer's text is for the
// drawer.
func (d *Daemon) handleGitAction(w http.ResponseWriter, r *http.Request, replyID, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) || !gitActions[action] {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	lang, _ := requestLang(r)
	release, ok := d.beginGitAction(s.Project)
	if !ok {
		http.Error(w, wsi18n.T(lang, "git.busy"), http.StatusConflict)
		return
	}
	code, text := gitActionAnswer(lang, action, s.Project)
	release()
	// Whatever the outcome, the handle reads the root again: a refusal or a
	// failure says its figures are out of date as much as a success changes
	// them.
	if d.git != nil {
		d.git.refresh(s.Project)
	}
	if code == http.StatusNoContent {
		w.WriteHeader(code)
		return
	}
	if code == http.StatusOK {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.WriteString(w, text)
		return
	}
	http.Error(w, text, code)
}

// gitActionAnswer runs action on root and is its answer's status and text.
func gitActionAnswer(lang, action, root string) (int, string) {
	switch action {
	case "push":
		refusal, err := pushGit(root)
		switch {
		case err != nil:
			return http.StatusBadGateway, err.Error()
		case refusal == pushDetached:
			return http.StatusConflict, wsi18n.T(lang, "git.push.refused.detached")
		case refusal == pushNoUpstream:
			return http.StatusConflict, wsi18n.T(lang, "git.push.refused.noupstream")
		case refusal == pushNothing:
			return http.StatusConflict, wsi18n.T(lang, "git.push.refused.nothing")
		}
	case "pull":
		outcome, err := pullGit(root)
		switch {
		case err != nil:
			return http.StatusBadGateway, err.Error()
		case outcome == pullCurrent:
			return http.StatusOK, wsi18n.T(lang, "git.pull.current")
		case outcome == pullDetached:
			return http.StatusConflict, wsi18n.T(lang, "git.pull.refused.detached")
		case outcome == pullNoUpstream:
			return http.StatusConflict, wsi18n.T(lang, "git.pull.refused.noupstream")
		case outcome == pullDirty:
			return http.StatusConflict, wsi18n.T(lang, "git.pull.refused.dirty")
		case outcome == pullDiverged:
			return http.StatusConflict, wsi18n.T(lang, "git.pull.refused.diverged")
		}
	case "fetch":
		if err := fetchGit(root); err != nil {
			return http.StatusBadGateway, err.Error()
		}
	}
	return http.StatusNoContent, ""
}
