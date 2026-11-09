package wsdashboard

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// pushRepos makes a bare origin and a clone of it whose main is ahead by
// ahead commits, and returns both. The daemon's own git runs see no global
// or system config.
func pushRepos(t *testing.T, work string, ahead int) (origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	seed := filepath.Join(base, "seed")
	gitRun(t, base, "init", "-q", "--bare", "-b", "main", origin)
	gitRun(t, base, "clone", "-q", origin, seed)
	gitCommit(t, seed, "a")
	gitRun(t, seed, "push", "-q", "origin", "main")
	gitRun(t, base, "clone", "-q", origin, work)
	for i := range ahead {
		gitCommit(t, work, "local"+string(rune('0'+i)))
	}
	return origin
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("rev-parse %s in %s: %v", rev, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// TestPushGitPushesTheBranchToItsUpstream: a branch two ahead lands on the
// upstream, and the handle's read then shows it in sync.
func TestPushGitPushesTheBranchToItsUpstream(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	origin := pushRepos(t, work, 2)
	if b := readGitBadge(work); b.Ahead != 2 {
		t.Fatalf("badge before the push = %+v", b)
	}
	refusal, err := pushGit(work)
	if err != nil || refusal != pushRan {
		t.Fatalf("pushGit = %v, %v", refusal, err)
	}
	if b := readGitBadge(work); b.State != gitOK || b.Ahead != 0 || b.Behind != 0 || b.Upstream != "origin/main" {
		t.Fatalf("badge after the push = %+v", b)
	}
	if got, want := revParse(t, origin, "main"), revParse(t, work, "HEAD"); got != want {
		t.Fatalf("origin main = %s, want %s", got, want)
	}
}

// TestPushGitRefuses: a detached HEAD, a branch without an upstream, and a
// branch with nothing to push are refused before git push runs; a root that
// is not a repository fails with git's reason.
func TestPushGitRefuses(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	if refusal, err := pushGit(work); err != nil || refusal != pushNothing {
		t.Errorf("in sync: pushGit = %v, %v; want pushNothing", refusal, err)
	}
	gitRun(t, work, "checkout", "-q", "-b", "topic")
	gitCommit(t, work, "topic")
	if refusal, err := pushGit(work); err != nil || refusal != pushNoUpstream {
		t.Errorf("no upstream: pushGit = %v, %v; want pushNoUpstream", refusal, err)
	}
	gitRun(t, work, "checkout", "-q", "--detach")
	if refusal, err := pushGit(work); err != nil || refusal != pushDetached {
		t.Errorf("detached: pushGit = %v, %v; want pushDetached", refusal, err)
	}
	if _, err := pushGit(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("non-repository: err = %v", err)
	}
}

// treeUpstream points root's main at a local upstream whose ref names a tree:
// git resolves the upstream's name, but `rev-list HEAD...@{upstream}` fails,
// and with remote "." no fetch repairs the ref.
func treeUpstream(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "update-ref", "refs/remotes/origin/main", revParse(t, root, "HEAD^{tree}"))
	gitRun(t, root, "config", "branch.main.remote", ".")
	gitRun(t, root, "config", "branch.main.merge", "refs/remotes/origin/main")
}

// TestGitActionsSurfaceAFailedCount: a failed ahead/behind count leaves both
// counts 0; push and pull fail with git's reason rather than reading it as
// nothing to push or nothing to pull.
func TestGitActionsSurfaceAFailedCount(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 1)
	treeUpstream(t, work)
	if refusal, err := pushGit(work); refusal != pushRan || err == nil || !strings.Contains(err.Error(), "symmetric difference") {
		t.Errorf("push: pushGit = %v, %v; want git's count failure", refusal, err)
	}
	if outcome, err := pullGit(work); outcome != pullRan || err == nil || !strings.Contains(err.Error(), "symmetric difference") {
		t.Errorf("pull: pullGit = %v, %v; want git's count failure", outcome, err)
	}
}

// deadlineReads records how far off each branch read's deadline was.
type deadlineReads struct {
	inner wsgit.Runner
	left  []time.Duration
}

func (r *deadlineReads) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if d, ok := ctx.Deadline(); ok {
		r.left = append(r.left, time.Until(d))
	} else {
		r.left = append(r.left, -1)
	}
	return r.inner.RunGit(ctx, root, args...)
}

// TestGitActionsRunUnderTheActionTimeout: push's and pull's branch reads run
// under a context bounded by gitActionTimeout, not the badge's read bound.
func TestGitActionsRunUnderTheActionTimeout(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	var seen *deadlineReads
	old := actionReads
	var bound time.Duration
	actionReads = func(run *gitBadgeRunner) wsgit.Runner {
		bound = run.timeout
		seen = &deadlineReads{inner: run}
		return seen
	}
	t.Cleanup(func() { actionReads = old })
	for name, action := range map[string]func() error{
		"push": func() error { _, err := pushGit(work); return err },
		"pull": func() error { _, err := pullGit(work); return err },
	} {
		seen = nil
		if err := action(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if seen == nil || len(seen.left) == 0 {
			t.Fatalf("%s ran no branch read", name)
		}
		if bound != gitActionTimeout {
			t.Fatalf("%s: runner names a %v bound, want %v", name, bound, gitActionTimeout)
		}
		for _, left := range seen.left {
			if left <= gitReadTimeout || left > gitActionTimeout {
				t.Fatalf("%s: a read had %v left, want just under %v", name, left, gitActionTimeout)
			}
		}
	}
}

// TestPushGitSurfacesARejection: a push the upstream rejects (it moved on)
// fails with git's one rejection line, no hint and no newline, bounded.
func TestPushGitSurfacesARejection(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	origin := pushRepos(t, work, 1)
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", origin, other)
	gitCommit(t, other, "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")

	before := revParse(t, origin, "main")
	refusal, err := pushGit(work)
	if err == nil || refusal != pushRan {
		t.Fatalf("pushGit = %v, %v; want a failure", refusal, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "[rejected]") || strings.Contains(msg, "\n") || strings.Contains(msg, "hint:") || len([]rune(msg)) > gitErrMax+1 {
		t.Fatalf("failure message = %q", msg)
	}
	if after := revParse(t, origin, "main"); after != before {
		t.Fatalf("origin main moved from %s to %s", before, after)
	}
}

// TestGitPushArgsCarryNoForce pins the command line: the branch's own ref
// onto the upstream ref, after "--", with no option at all, so nothing
// force-like can ride along, whatever the names.
func TestGitPushArgsCarryNoForce(t *testing.T) {
	for _, c := range []struct{ remote, branch, merge string }{
		{"origin", "main", "refs/heads/main"},
		{"up", "topic/x", "refs/heads/feature"},
		{"origin", "f", "+refs/heads/f"},
	} {
		args := gitPushArgs(c.remote, c.branch, c.merge)
		if len(args) != 4 || args[0] != "push" || args[1] != "--" || args[2] != c.remote {
			t.Fatalf("gitPushArgs(%v) = %q", c, args)
		}
		for _, a := range args[2:] {
			if strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+") {
				t.Errorf("gitPushArgs(%v) carries %q", c, a)
			}
		}
		for _, a := range args {
			if strings.Contains(a, "force") || a == "-f" || strings.Contains(a, "mirror") || strings.Contains(a, "delete") {
				t.Errorf("gitPushArgs(%v) carries %q", c, a)
			}
		}
		if want := "refs/heads/" + c.branch + ":" + c.merge; args[3] != want {
			t.Errorf("refspec = %q, want %q", args[3], want)
		}
	}
}

func TestGitPushErrText(t *testing.T) {
	out := "To /tmp/origin.git\n ! [rejected]        main -> main (fetch first)\nerror: failed to push some refs to '/tmp/origin.git'\nhint: Updates were rejected\n"
	if got, want := gitActionErrText(out, errors.New("exit status 1")), "! [rejected] main -> main (fetch first)"; got != want {
		t.Errorf("rejection = %q, want %q", got, want)
	}
	hook := "remote: checking the push\nremote: policy says no\nTo /tmp/origin.git\n ! [remote rejected] main -> main (pre-receive hook declined)\nerror: failed to push some refs to '/tmp/origin.git'\n"
	if got, want := gitActionErrText(hook, nil), "! [remote rejected] main -> main (pre-receive hook declined)"; got != want {
		t.Errorf("hook rejection = %q, want %q", got, want)
	}
	if got, want := gitActionErrText("remote: hello\nerror: src refspec x does not match any\n", nil), "error: src refspec x does not match any"; got != want {
		t.Errorf("error line = %q, want %q", got, want)
	}
	if got := gitActionErrText("fatal: could not read Username for 'https://example.com': terminal prompts disabled\n", nil); !strings.HasPrefix(got, "fatal: could not read Username") {
		t.Errorf("credential failure = %q", got)
	}
	if got := gitActionErrText("\nhint: x\n", errors.New("exit status 128")); got != "exit status 128" {
		t.Errorf("no meaningful line = %q", got)
	}
	if got := gitActionErrText(strings.Repeat("x", 3*gitErrMax), nil); len([]rune(got)) != gitErrMax+1 {
		t.Errorf("long line = %d runes", len([]rune(got)))
	}
}

// TestGitBadgeRunnerNamesItsTimeout: a command cut by its context's bound
// says that bound: the read's by default, the action's when it runs under
// one.
func TestGitBadgeRunnerNamesItsTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		run  *gitBadgeRunner
		want string
	}{
		{&gitBadgeRunner{}, "timed out after " + gitReadTimeout.String()},
		{&gitBadgeRunner{timeout: gitActionTimeout}, "timed out after " + gitActionTimeout.String()},
	} {
		if _, err := c.run.RunGit(ctx, t.TempDir(), "status"); err == nil || err.Error() != c.want {
			t.Errorf("timeout %v: err = %v, want %q", c.run.timeout, err, c.want)
		}
	}
}

// TestBeginGitActionHoldsOnePerRoot: a second action of a root waits for
// the first to end; another root is free.
func TestBeginGitActionHoldsOnePerRoot(t *testing.T) {
	d := &Daemon{}
	release, ok := d.beginGitAction("/repo")
	if !ok {
		t.Fatal("first push refused")
	}
	if _, ok := d.beginGitAction("/repo/"); ok {
		t.Fatal("a second push of the same root was let through")
	}
	other, ok := d.beginGitAction("/other")
	if !ok {
		t.Fatal("a push of another root was refused")
	}
	other()
	release()
	if again, ok := d.beginGitAction("/repo"); !ok {
		t.Fatal("a push after the first ended was refused")
	} else {
		again()
	}
}

// TestGitPushRoute pins the route end to end on real repositories: it needs
// the daemon's own Origin and POST, refuses with the reason when there is
// nothing to push, and a push of a branch two ahead lands on the upstream and
// re-reads the handle, whose next swap shows the branch in sync.
func TestGitPushRoute(t *testing.T) {
	f := startPageFixture(t, func(o *DaemonOptions) { o.noGitBadge = false })
	f.d.git.gap = 0
	work := f.project // empty, so the clone lands in it
	origin := pushRepos(t, work, 2)
	route := f.base + "/s/livesession/git/push"

	for _, o := range []string{"", "null", "http://evil.example"} {
		if code, header, _ := postJSON(t, route, o, ""); code != http.StatusForbidden {
			t.Errorf("push with Origin %q = %d, want 403", o, code)
		} else {
			assertNoCORS(t, header)
		}
	}
	if code, _ := get(t, route); code != http.StatusMethodNotAllowed {
		t.Errorf("GET push = %d, want 405", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/git/push", f.base, ""); code != http.StatusNotFound {
		t.Errorf("push of an unknown session = %d, want 404", code)
	}
	if got := revParse(t, origin, "main"); got == revParse(t, work, "HEAD") {
		t.Fatal("a refused request pushed")
	}

	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `data-git="push"`) {
		t.Fatal("the session page lacks the git sheet's push button")
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/livesession/events?h=" + heldOf(page) + ",mode." + newRegion(modeLive).Hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	gitEvent := func(what string, match func(string) bool) {
		t.Helper()
		for {
			ev, ok := nextRegion(t, events, 5*time.Second)
			if !ok {
				t.Fatalf("no git region %s", what)
			}
			if ev.Name == regionGit && match(ev.HTML) {
				return
			}
		}
	}
	gitEvent("two ahead", func(h string) bool { return strings.Contains(h, `<b class="up">↑2</b>`) })

	// The push runs git in the handler, which outlasts testClient's 500 ms
	// on a loaded Windows runner (seen on CI).
	pushClient := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	if code, _, raw := postJSONWith(t, pushClient, route, f.base, ""); code != http.StatusNoContent {
		t.Fatalf("push = %d %s, want 204", code, raw)
	}
	if got, want := revParse(t, origin, "main"), revParse(t, work, "HEAD"); got != want {
		t.Fatalf("origin main = %s, want %s", got, want)
	}
	gitEvent("in sync", func(h string) bool { return strings.Contains(h, `class="gh ok"`) && !strings.Contains(h, "gh-ab") })

	code, _, raw := postJSONWith(t, pushClient, route, f.base, "")
	if code != http.StatusConflict || !strings.Contains(raw, "nothing to push") {
		t.Fatalf("push with nothing ahead = %d %q, want 409 nothing to push", code, raw)
	}
}

// TestGitActionCommandBoundsRemoteCommands: push and fetch run with no
// terminal prompt and ssh in batch mode over the repository's
// core.sshCommand, but leave credential helpers interactive (a GUI sign-in
// still works); a local merge gets none of it; every action keeps the C
// locale.
func TestGitActionCommandBoundsRemoteCommands(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GCM_INTERACTIVE", "always")
	t.Setenv("GIT_ASKPASS", "/x/askpass")
	t.Setenv("SSH_ASKPASS", "/x/askpass")
	gitRun(t, work, "config", "core.sshCommand", "ssh -i /k/id")
	ctx := context.Background()
	envHas := func(env []string, kv string) bool {
		for _, e := range env {
			if e == kv {
				return true
			}
		}
		return false
	}
	for _, args := range [][]string{{"fetch"}, gitPushArgs("origin", "main", "refs/heads/main")} {
		cmd := gitActionCommand(ctx, work, args...)
		if strings.Contains(strings.Join(cmd.Args, " "), "credential.interactive") {
			t.Fatalf("%s args = %q, want credential helpers left interactive", args[0], cmd.Args)
		}
		for _, kv := range []string{
			"LC_ALL=C",
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND=ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5",
		} {
			if !envHas(cmd.Env, kv) {
				t.Fatalf("%s env lacks %q", args[0], kv)
			}
		}
		for _, kv := range []string{"GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS="} {
			if envHas(cmd.Env, kv) {
				t.Fatalf("%s env sets %q, want credential prompting left to the user's config", args[0], kv)
			}
		}
		if cmd.WaitDelay == 0 {
			t.Fatalf("%s is not bounded (WaitDelay 0)", args[0])
		}
	}
	merge := gitActionCommand(ctx, work, "merge", "--ff-only", "--quiet", "@{upstream}")
	if !envHas(merge.Env, "LC_ALL=C") || envHas(merge.Env, "GIT_TERMINAL_PROMPT=0") ||
		envHas(merge.Env, "GIT_SSH_COMMAND=ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5") {
		t.Fatalf("merge env: want LC_ALL=C and no remote env")
	}
}
