package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The push action: a press on the ahead count (↑N) of the git badge or the
// status band, confirmed in the page's popover, asks the daemon to push the
// session root's current branch to its configured upstream, and nothing
// else: never a force-like option, never another branch or remote. The
// daemon re-reads the branch first and refuses a detached HEAD, a branch
// without an upstream, and one with nothing to push.

const (
	// gitPushTimeout bounds one push, network included.
	gitPushTimeout = 60 * time.Second
	// gitPushWaitDelay bounds the wait for git's output pipes once the push
	// is killed, which a helper git started (ssh) may still hold open.
	gitPushWaitDelay = 5 * time.Second
)

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
	ctx, cancel := context.WithTimeout(context.Background(), gitPushTimeout)
	defer cancel()
	run := &gitBadgeRunner{}
	st, ok := wsgit.BranchSync(ctx, run, root)
	if !ok {
		if run.err == nil {
			return pushRan, fmt.Errorf("HEAD names no commit")
		}
		return pushRan, fmt.Errorf("%s", gitErrText(run.err))
	}
	switch {
	case st.Head == "":
		return pushDetached, nil
	case st.Upstream == "":
		return pushNoUpstream, nil
	case st.Ahead == 0:
		return pushNothing, nil
	}
	remote, merge, ok := upstreamOf(ctx, run, root, st.Head)
	if !ok {
		return pushNoUpstream, nil
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, gitPushArgs(remote, st.Head, merge)...)...)
	// A credential prompt fails instead of waiting on a terminal the daemon
	// does not have; the C locale keeps git's messages comparable.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	cmd.WaitDelay = gitPushWaitDelay
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return pushRan, fmt.Errorf("timed out after %s", gitPushTimeout)
		}
		return pushRan, fmt.Errorf("%s", gitPushErrText(string(out), err))
	}
	return pushRan, nil
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

// gitPushErrText is a failed push's message for the popover: the first line
// of git's output that says what went wrong, skipping the destination line,
// hints, and blank lines, with its runs of spaces folded, bounded.
func gitPushErrText(out string, err error) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || strings.HasPrefix(line, "To ") || strings.HasPrefix(line, "hint:") {
			continue
		}
		return gitErrText(fmt.Errorf("%s", line))
	}
	return gitErrText(err)
}

// beginPush claims root's one push slot; false while another push of it
// runs. The release returned with true frees it.
func (d *Daemon) beginPush(root string) (release func(), ok bool) {
	key := gitKey(root)
	d.pushMu.Lock()
	defer d.pushMu.Unlock()
	if d.pushing[key] {
		return nil, false
	}
	if d.pushing == nil {
		d.pushing = map[string]bool{}
	}
	d.pushing[key] = true
	return func() {
		d.pushMu.Lock()
		delete(d.pushing, key)
		d.pushMu.Unlock()
	}, true
}

// handleGitPush serves POST /s/<reply-id>/git/push: a push of the session
// root's current branch to its upstream. It needs the daemon's own Origin,
// as the refresh route does, and answers once the push ended: 204 when it
// succeeded (the badge's re-read then lands through the stream), 409 with
// the reason when the daemon refused it or another push of the root runs,
// and 502 with git's first meaningful line when it failed. Every answer's
// text is for the popover.
func (d *Daemon) handleGitPush(w http.ResponseWriter, r *http.Request, replyID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	lang, _ := requestLang(r)
	release, ok := d.beginPush(s.Project)
	if !ok {
		http.Error(w, wsi18n.T(lang, "git.push.refused.busy"), http.StatusConflict)
		return
	}
	refusal, err := pushGit(s.Project)
	release()
	// Whatever the outcome, the badge reads the root again: a refusal or a
	// failure says the badge's figures are out of date as much as a success
	// changes them.
	if d.git != nil {
		d.git.refresh(s.Project)
	}
	switch {
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadGateway)
	case refusal == pushDetached:
		http.Error(w, wsi18n.T(lang, "git.push.refused.detached"), http.StatusConflict)
	case refusal == pushNoUpstream:
		http.Error(w, wsi18n.T(lang, "git.push.refused.noupstream"), http.StatusConflict)
	case refusal == pushNothing:
		http.Error(w, wsi18n.T(lang, "git.push.refused.nothing"), http.StatusConflict)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
