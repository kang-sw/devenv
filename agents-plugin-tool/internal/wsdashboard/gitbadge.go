package wsdashboard

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wslive"
)

// The session page's git handle: the drawer handle left of the side-panel
// handle shows the session root's branch (abbreviated, see GitHandle), its
// standing against its upstream, and whether its working tree is dirty, which
// the daemon reads itself, apart from the session's own ws-mcp figures (the
// snapshot's repo, which the status band's folder comes from). The handle
// opens the git drawer (gitdrawer.go); the badge type below is what the
// handle draws.
//
// The daemon reads a root only while some page of a session under it holds
// its SSE stream open (gitWatcher): once when the first such stream opens,
// then checks Git metadata each gitReadInterval until the last one closes.
// Unchanged metadata suppresses commands, never beyond five minutes; explicit
// refresh and resumption bypass the hint. It also reads at once when a
// page asks (POST /s/<reply-id>/git, which the drawer sends as it opens). One
// goroutine per watched root runs every read, so pages sharing a root share
// one read and a burst of refresh requests never runs git concurrently for
// one root. No read ever fetches, so behind reflects the last fetch.
//
// The badge follows the snapshot: when a session's live snapshot brings repo
// figures that differ from the ones it brought before and from the badge's
// last read, the stream that rendered it asks for a read (follow), so the
// two agree once the badge's read lands. The interval stays the fallback for
// a root whose session is gone.
//
// One exception reads git for roots no stream watches: the session rail's
// worktree lists (railworktrees.go) run git worktree list --porcelain for
// the repositories of the sessions the rail shows, while some page shows
// the rail. That command reads only the repository's worktree records,
// never a working tree, and never fetches; the rail's change counts come
// from the sessions' own snapshots.

const (
	// gitReadInterval is the period between a watched root's reads.
	gitReadInterval = 3 * time.Minute
	// gitRefreshGap is the least time between a read and the next one a
	// refresh request asks for; requests inside it wait for its end.
	gitRefreshGap = 2 * time.Second
	// gitReadTimeout bounds one read's git commands together.
	gitReadTimeout = 10 * time.Second
	// gitErrMax bounds a failed read's message, which the handle's title
	// shows.
	gitErrMax = 240
)

// The git badge's states.
const (
	gitPending = "pending" // watched, not read yet
	gitOK      = "ok"
	gitNoRepo  = "norepo"
	gitFailed  = "failed"
)

// GitBadge is the git handle's read of one session root.
type GitBadge struct {
	// State is gitPending, gitOK, gitNoRepo, or gitFailed.
	State string
	// Root is the session root the badge reads.
	Root string
	// TopLevel is the git worktree root, when known.
	TopLevel string
	// Repo holds the branch fields of an ok read; Folder stays empty.
	wslive.Repo
	// Ticket is the title of the ticket an impl or goal branch's stem
	// names, empty when none resolves.
	Ticket string
	// Err is a failed read's git message.
	Err string
	// At is when the read finished (epoch ms; 0 before the first). It is in
	// the badge's markup, but a read that differs from the last one only by
	// At wakes no page (sameBadge), so a page may show an older At.
	At int64
}

// readGitBadge reads root's branch standing and working-tree counts.
func readGitBadge(root string) GitBadge {
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	return readGitBadgeWith(ctx, &gitBadgeRunner{}, root)
}

// One porcelain read covers both refs and working-tree state. HEAD/index
// metadata alone cannot prove freshness: the watcher retains a forced complete
// safety read for in-place edits/untracked contents and explicit refresh.
func readGitBadgeWith(ctx context.Context, run wsgit.Runner, root string) GitBadge {
	badge := GitBadge{State: gitOK, Root: root}
	out, err := run.RunGit(ctx, root, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "--ahead-behind", "--untracked-files=normal")
	if err != nil {
		badge.State = gitFailed
		switch {
		case strings.Contains(err.Error(), "not a git repository"):
			badge.State = gitNoRepo
		default:
			badge.Err = gitErrText(err)
		}
		return badge
	}
	if top, err := run.RunGit(ctx, root, "rev-parse", "--show-toplevel"); err == nil {
		badge.TopLevel = strings.TrimSuffix(string(top), "\n")
	}
	st := wsgit.ParseStatus(out)
	if st.Branch.OID == "(initial)" || st.Branch.OID == "" {
		badge.State, badge.Err = gitFailed, "HEAD names no commit"
		return badge
	}
	badge.Branch, badge.Upstream, badge.Ahead, badge.Behind = st.Branch.Head, st.Branch.Upstream, st.Branch.Ahead, st.Branch.Behind
	if st.Branch.Head == "(detached)" {
		short, err := run.RunGit(ctx, root, "rev-parse", "--short", "HEAD")
		if err != nil {
			badge.State, badge.Err = gitFailed, gitErrText(err)
			return badge
		}
		badge.Branch, badge.Detached = strings.TrimSpace(string(short)), true
	} else if h := branchHandle(st.Branch.Head, false); h.Impl || h.Goal {
		badge.Ticket = ticketTitles.title(root, h.Text, time.Now())
	}
	for _, file := range st.ChangedFiles {
		x, y := file.IndexStatus, file.WorktreeStatus
		switch {
		case file.Status == "?":
			badge.Untracked++
		case file.Status == "!":
		case x == "U" || y == "U" || file.Status == "AA" || file.Status == "DD":
			badge.Modified++
		default:
			if x != "." && x != "" {
				badge.Staged++
			}
			if y != "." && y != "" {
				badge.Modified++
			}
		}
	}
	return badge
}

// gitBadgeRunner runs git as wsgit.ExecRunner does, but in the C locale, so
// a non-repository is told apart by git's message whatever the user's
// language, and it keeps the last failure for the badge's title. Its output
// is stdout alone, which the callers parse: a warning git writes to stderr
// on success (autocrlf's "LF will be replaced by CRLF", an unreadable
// directory) never reaches a parse, and a failure's message is stderr's.
type gitBadgeRunner struct {
	err error
	// timeout is the bound of the context the runner is given, which a
	// timed-out command's message names; zero is gitReadTimeout.
	timeout time.Duration
}

// badgeGitCommand is git in root under the C locale, run by the path
// wsgit.Executable resolved, so a badge read does not search PATH on every
// spawn. Its timeout ends git's whole process tree (gitbatch.Bound), so a
// timed-out read returns at its bound on Windows too.
func badgeGitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	// Local browsing can ask git about an untrusted outside repository.
	// Never execute its configured filesystem monitor during a read.
	cmd := exec.CommandContext(ctx, wsgit.Executable(), append([]string{"-C", root, "-c", "core.fsmonitor=false"}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	gitbatch.Bound(cmd)
	return cmd
}

func (r *gitBadgeRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := badgeGitCommand(ctx, root, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			timeout := r.timeout
			if timeout == 0 {
				timeout = gitReadTimeout
			}
			err = fmt.Errorf("timed out after %s", timeout)
		} else if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = fmt.Errorf("%s", msg)
		} else if msg := strings.TrimSpace(string(out)); msg != "" {
			err = fmt.Errorf("%s", msg)
		}
		r.err = err
		return nil, err
	}
	return out, nil
}

// gitErrText is a failure's message for a title: its last line, bounded.
func gitErrText(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := strings.TrimSpace(err.Error())
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = strings.TrimSpace(msg[i+1:])
	}
	if r := []rune(msg); len(r) > gitErrMax {
		msg = string(r[:gitErrMax]) + "…"
	}
	return msg
}

// gitWatcher reads the roots open pages watch (see the file comment).
type gitWatcher struct {
	// read reads one root; it runs on the root's own goroutine, never under
	// mu.
	read func(root string) GitBadge
	// interval is the period between reads; gap the least time between a
	// read and the next a refresh asks for.
	interval, gap time.Duration
	// after is time.After, injected so a test drives the clock.
	after func(time.Duration) <-chan time.Time
	now   func() time.Time
	probe func(string) (string, error)
	// changed is told the root after each read.
	changed func(root string)
	// stop ends every root's goroutine.
	stop <-chan struct{}

	mu    sync.Mutex
	roots map[string]*gitRoot
}

// gitRoot is one watched root.
type gitRoot struct {
	// viewers counts the streams watching it; at zero its goroutine ends.
	viewers int
	// wake carries a refresh request, or the last viewer's leaving; it holds
	// one pending signal, so requests that arrive together fold into one.
	wake chan struct{}
	// badge is the last read; nil before the first.
	badge *GitBadge
	// seen is the repo figures each session's snapshot last brought (by
	// reply-id), in comparable form (followRepo).
	seen map[string]wslive.Repo
	gate gitReadGate // owned by this root's worker, not the viewer goroutines
}

func newGitWatcher(read func(string) GitBadge, changed func(string), stop <-chan struct{}) *gitWatcher {
	return &gitWatcher{
		read:     read,
		interval: gitReadInterval,
		gap:      gitRefreshGap,
		after:    time.After,
		now:      time.Now,
		probe:    gitMetadataStamp,
		changed:  changed,
		stop:     stop,
		roots:    map[string]*gitRoot{},
	}
}

// gitKey is the map key of a root.
func gitKey(root string) string { return filepath.Clean(root) }

// watch adds a viewer of root and returns its release. The first viewer
// starts the root's goroutine, which reads at once.
func (g *gitWatcher) watch(root string) (release func()) { return g.watchWithRefresh(root, false) }

// A visible resumption refreshes even when another tab kept this root alive.
func (g *gitWatcher) watchFresh(root string) (release func()) { return g.watchWithRefresh(root, true) }

func (g *gitWatcher) watchWithRefresh(root string, fresh bool) (release func()) {
	key := gitKey(root)
	g.mu.Lock()
	r := g.roots[key]
	if r == nil {
		r = &gitRoot{wake: make(chan struct{}, 1)}
		r.wake <- struct{}{}
		g.roots[key] = r
		go g.run(key, r)
	} else if fresh {
		signal(r.wake)
	}
	r.viewers++
	g.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			r.viewers--
			if r.viewers == 0 {
				signal(r.wake)
			}
			g.mu.Unlock()
		})
	}
}

// refresh asks for an immediate read of root; false when no page watches
// it, which reads nothing.
func (g *gitWatcher) refresh(root string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.roots[gitKey(root)]
	if r == nil || r.viewers == 0 {
		return false
	}
	signal(r.wake)
	return true
}

// follow tells the watcher the repo figures replyID's live snapshot holds
// for root. It asks for a read of root, as refresh does, when they differ
// both from what that snapshot held before and from the last read; an
// unchanged snapshot asks for nothing, so a badge and a band that keep
// disagreeing never make the root read in a loop. A root no page watches, a
// root whose first read is still to land, and a snapshot without a branch
// ask for nothing.
func (g *gitWatcher) follow(root, replyID string, repo wslive.Repo) {
	if g == nil || root == "" || repo.Branch == "" {
		return
	}
	repo = followRepo(repo)
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.roots[gitKey(root)]
	if r == nil || r.viewers == 0 {
		return
	}
	if seen, ok := r.seen[replyID]; ok && seen == repo {
		return
	}
	if r.seen == nil {
		r.seen = map[string]wslive.Repo{}
	}
	r.seen[replyID] = repo
	if r.badge == nil || (r.badge.State == gitOK && followRepo(r.badge.Repo) == repo) {
		return
	}
	signal(r.wake)
}

// followRepo is r in the form follow compares: as a snapshot keeps it, and
// without the folder, which the badge never reads.
func followRepo(r wslive.Repo) wslive.Repo {
	r = wslive.CleanRepo(r)
	r.Folder = ""
	return r
}

// badge is root's badge: its last read, or pending before the first; nil
// for an empty root.
func (g *gitWatcher) badge(root string) *GitBadge {
	if g == nil || root == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if r := g.roots[gitKey(root)]; r != nil && r.badge != nil {
		b := *r.badge
		return &b
	}
	return &GitBadge{State: gitPending, Root: root}
}

// run is a watched root's goroutine: it reads on each wake or interval
// while the root has a viewer, and ends, dropping the root, once it has
// none.
func (g *gitWatcher) run(key string, r *gitRoot) {
	var last time.Time
	// tick is the interval wait since the last read; nil before the first,
	// which the first viewer's wake starts.
	var tick <-chan time.Time
	for {
		woken := false
		select {
		case <-g.stop:
			return
		case <-r.wake:
			woken = true
		case <-tick:
		}
		if !g.viewed(key, r) {
			return
		}
		if wait := g.gap - g.now().Sub(last); woken && !last.IsZero() && wait > 0 {
			// A refresh inside the gap after a read waits for its end;
			// requests that arrive meanwhile fold into that one read.
			select {
			case <-g.stop:
				return
			case <-g.after(wait):
			}
			select {
			case <-r.wake:
			default:
			}
			if !g.viewed(key, r) {
				return
			}
		}
		stamp, probeErr := g.probe(key)
		if probeErr != nil {
			stamp = ""
		}
		if !r.gate.needed(stamp, g.now(), woken) {
			tick = g.after(r.gate.delay(g.now(), g.interval))
			continue
		}
		badge := g.read(key)
		last = g.now()
		r.gate.complete(stamp, last, badge.State == gitOK)
		badge.At = last.UnixMilli()
		tick = g.after(r.gate.delay(last, g.interval))
		g.mu.Lock()
		same := r.badge != nil && sameBadge(*r.badge, badge)
		r.badge = &badge
		g.mu.Unlock()
		// A read that found what the last one did wakes no page: every
		// session page on the root would re-render for nothing.
		if !same {
			g.changed(key)
		}
	}
}

// sameBadge reports whether two reads found the same standing: every field
// but At, which each read stamps.
func sameBadge(a, b GitBadge) bool {
	a.At, b.At = 0, 0
	return a == b
}

// viewed reports whether r still has a viewer and the daemon runs, dropping
// r from the watched roots when it has no viewer.
func (g *gitWatcher) viewed(key string, r *gitRoot) bool {
	select {
	case <-g.stop:
		return false
	default:
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.viewers > 0 {
		return true
	}
	delete(g.roots, key)
	return false
}

// signal sends on a one-slot channel without blocking.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// handleGitRefresh serves POST /s/<reply-id>/git: an immediate re-read of
// the session root's git handle. It needs the daemon's own Origin, as the
// other browser writes do, and answers 204 once the read is asked for (it
// lands on the page through the stream), 409 when no page watches the root.
func (d *Daemon) handleGitRefresh(w http.ResponseWriter, r *http.Request, replyID string) {
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
	if !ok || s.Project == "" || d.git == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if !d.git.refresh(s.Project) {
		http.Error(w, "no open page watches this session's git status", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
