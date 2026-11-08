package wsdashboard

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// fakeGit stands in for the git badge's read and clock: each read reports
// its root on reads and, when gate is set, waits for a value on it first;
// each wait the watcher starts is sent on waits for the test to fire.
type fakeGit struct {
	reads chan string
	gate  chan struct{}
	waits chan fakeWait

	mu    sync.Mutex
	count map[string]int
}

type fakeWait struct {
	d  time.Duration
	ch chan time.Time
}

func newFakeGit() *fakeGit {
	return &fakeGit{reads: make(chan string, 64), waits: make(chan fakeWait, 64), count: map[string]int{}}
}

func (f *fakeGit) read(root string) GitBadge {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	f.count[root]++
	n := f.count[root]
	f.mu.Unlock()
	f.reads <- root
	return GitBadge{State: gitOK, Root: root, Repo: wslive.Repo{Branch: "main", Upstream: "origin/main", Ahead: n}}
}

func (f *fakeGit) readCount(root string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count[root]
}

func (f *fakeGit) after(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	f.waits <- fakeWait{d: d, ch: ch}
	return ch
}

// newTestWatcher is a watcher on f's read and clock.
func newTestWatcher(t *testing.T, f *fakeGit) (*gitWatcher, chan string) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	changed := make(chan string, 64)
	g := newGitWatcher(f.read, func(root string) { changed <- root }, stop)
	g.after = f.after
	return g, changed
}

func expectRead(t *testing.T, f *fakeGit, root string) {
	t.Helper()
	select {
	case got := <-f.reads:
		if got != root {
			t.Fatalf("read %q, want %q", got, root)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no read of %q", root)
	}
}

func expectNoRead(t *testing.T, f *fakeGit) {
	t.Helper()
	select {
	case got := <-f.reads:
		t.Fatalf("unexpected read of %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// nextWait is the next wait the watcher starts that lasts d (d < 0: any
// wait shorter than the interval, which is the refresh gap's).
func nextWait(t *testing.T, f *fakeGit, d time.Duration) fakeWait {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case w := <-f.waits:
			if w.d == d || (d < 0 && w.d < gitReadInterval) {
				return w
			}
		case <-deadline:
			t.Fatalf("no wait of %v", d)
		}
	}
}

func watched(g *gitWatcher) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.roots)
}

// TestGitWatcherReadsWhileViewedOnly pins the watcher's life: the first
// viewer starts a root and reads it at once, each interval reads again, and
// the last viewer's leaving ends it, after which nothing is read and a
// refresh asks for nothing.
func TestGitWatcherReadsWhileViewedOnly(t *testing.T) {
	f := newFakeGit()
	g, changed := newTestWatcher(t, f)
	if b := g.badge("/repo"); b == nil || b.State != gitPending {
		t.Fatalf("badge before any viewer = %+v, want pending", b)
	}
	release := g.watch("/repo")
	expectRead(t, f, "/repo")
	if got := <-changed; got != "/repo" {
		t.Fatalf("changed %q", got)
	}
	if b := g.badge("/repo"); b.State != gitOK || b.Ahead != 1 || b.At == 0 {
		t.Fatalf("badge after the first read = %+v", b)
	}

	nextWait(t, f, gitReadInterval).ch <- time.Now()
	expectRead(t, f, "/repo")
	<-changed // the read is stored before the change is told
	if b := g.badge("/repo"); b.Ahead != 2 {
		t.Fatalf("badge after the interval read = %+v", b)
	}

	release()
	release() // idempotent
	waitFor(t, "the root to be dropped", func() bool { return watched(g) == 0 })
	expectNoRead(t, f)
	if g.refresh("/repo") {
		t.Fatal("refresh of an unwatched root reported a read")
	}
	if b := g.badge("/repo"); b.State != gitPending {
		t.Fatalf("badge after the last viewer left = %+v, want pending", b)
	}
	if f.readCount("/repo") != 2 {
		t.Fatalf("reads = %d, want 2", f.readCount("/repo"))
	}
}

// TestGitWatcherSharesARootBetweenViewers: pages of one root share its one
// goroutine and its reads; the root stops only when its last viewer leaves.
func TestGitWatcherSharesARootBetweenViewers(t *testing.T) {
	f := newFakeGit()
	g, _ := newTestWatcher(t, f)
	a := g.watch("/repo")
	expectRead(t, f, "/repo")
	b := g.watch("/repo/") // the same root, spelled differently
	other := g.watch("/other")
	expectRead(t, f, "/other")
	expectNoRead(t, f)
	if n := watched(g); n != 2 {
		t.Fatalf("watched roots = %d, want 2", n)
	}

	a()
	if !g.refresh("/repo") {
		t.Fatal("a root with a viewer left refused a refresh")
	}
	nextWait(t, f, -1).ch <- time.Now()
	expectRead(t, f, "/repo")

	b()
	other()
	waitFor(t, "both roots to be dropped", func() bool { return watched(g) == 0 })
	if f.readCount("/repo") != 2 || f.readCount("/other") != 1 {
		t.Fatalf("reads = %v", f.count)
	}
}

// TestGitWatcherFoldsRefreshRequests: requests that arrive while a read runs
// or inside the gap after it fold into one read at the gap's end, and no
// read of a root ever overlaps another.
func TestGitWatcherFoldsRefreshRequests(t *testing.T) {
	f := newFakeGit()
	f.gate = make(chan struct{})
	g, _ := newTestWatcher(t, f)
	release := g.watch("/repo")
	defer release()
	for range 5 {
		if !g.refresh("/repo") {
			t.Fatal("refresh refused")
		}
	}
	f.gate <- struct{}{} // the first read finishes
	expectRead(t, f, "/repo")
	for range 5 {
		g.refresh("/repo")
	}
	gap := nextWait(t, f, -1)
	if gap.d <= 0 || gap.d > gitRefreshGap {
		t.Fatalf("gap wait %v, want within (0, %v]", gap.d, gitRefreshGap)
	}
	gap.ch <- time.Now()
	f.gate <- struct{}{}
	expectRead(t, f, "/repo")
	// No request is left: the next wait is the interval's, and no read
	// waits at the gate.
	nextWait(t, f, gitReadInterval)
	select {
	case f.gate <- struct{}{}:
		t.Fatal("a third read started")
	case <-time.After(100 * time.Millisecond):
	}
	if n := f.readCount("/repo"); n != 2 {
		t.Fatalf("reads = %d, want 2", n)
	}
}

// TestGitWatcherEndsWithTheDaemon: a daemon stop ends a root's goroutine
// even while a viewer remains.
func TestGitWatcherEndsWithTheDaemon(t *testing.T) {
	f := newFakeGit()
	stop := make(chan struct{})
	g := newGitWatcher(f.read, func(string) {}, stop)
	g.after = f.after
	g.watch("/repo")
	expectRead(t, f, "/repo")
	nextWait(t, f, gitReadInterval)
	close(stop)
	g.refresh("/repo")
	expectNoRead(t, f)
}

// TestGitBadgeRendersEachState pins the badge's markup: the band's branch,
// sync, and change chips for a read repository, the detached form, and the
// dimmed forms with their reasons. The badge is a span whose branch chip is
// the refresh button, and its ahead count, where a push can go, the push
// popover's.
func TestGitBadgeRendersEachState(t *testing.T) {
	repo := func(r wslive.Repo) *GitBadge { return &GitBadge{State: gitOK, Root: "/p", Repo: r, At: 42} }
	cases := []struct {
		name  string
		badge *GitBadge
		want  []string
		not   []string
	}{
		{"clean, in sync", repo(wslive.Repo{Branch: "main", Upstream: "origin/main"}),
			[]string{`<span class="btn sb gitb ok"`, `data-at="42"`, `title="git status of /p: click to read it again now"`, `<button type="button" class="sb-br" title="branch: main"`, `<span>main</span>`, `class="sb-sync ok" title="in sync with origin/main as of the last fetch"`},
			[]string{"sb-diff", "gitpush", `<button type="button" class="btn`}},
		{"dirty", repo(wslive.Repo{Branch: "main", Upstream: "origin/main", Modified: 2, Staged: 1, Untracked: 3}),
			[]string{`class="sb-diff" title="working tree: 2 modified 1 staged 3 untracked"`, `<b class="mod">~2</b><b class="add">+1</b><b class="unt">?3</b>`},
			nil},
		{"ahead and behind", repo(wslive.Repo{Branch: "topic", Upstream: "origin/topic", Ahead: 2, Behind: 1}),
			[]string{`<button type="button" class="up gitpush" data-br="topic" data-up="origin/topic" data-n="2 commits to push" aria-haspopup="dialog" title="push to origin/topic…">↑2</button><span class="dn">↓1</span>`, "2 to push", "1 to pull"},
			[]string{"sb-sync ok", "sb-diff"}},
		{"no upstream", repo(wslive.Repo{Branch: "topic"}),
			[]string{`class="sb-sync none" title="no upstream branch is set"`, ">no upstream<"},
			[]string{"gitpush"}},
		{"detached", repo(wslive.Repo{Branch: "abc1234", Detached: true, Modified: 1}),
			[]string{`title="detached HEAD at abc1234"`, `<span>abc1234</span>`, `<b class="mod">~1</b>`},
			[]string{"sb-sync", "gitpush"}},
		{"not a repository", &GitBadge{State: gitNoRepo, Root: "/p", At: 42},
			[]string{`class="btn sb gitb norepo"`, `title="/p is not a git repository"`, "<span>no repo</span>"},
			[]string{"sb-sync", "sb-diff"}},
		{"read failure", &GitBadge{State: gitFailed, Root: "/p", Err: "timed out after 10s", At: 42},
			[]string{`class="btn sb gitb failed"`, `title="git status could not be read: timed out after 10s"`, "<span>git error</span>"},
			[]string{"sb-sync", "sb-diff"}},
		{"pending", &GitBadge{State: gitPending, Root: "/p"},
			[]string{`class="btn sb gitb pending"`, `title="reading the git status…"`, `data-at="0"`},
			[]string{"sb-sync"}},
	}
	for _, c := range cases {
		for _, adapter := range []bool{false, true} {
			regions, err := renderSessionRegions(SessionView{ReplyID: "x", Adapter: adapter, Git: c.badge}, nil)
			if err != nil {
				t.Fatal(err)
			}
			html := string(regions[regionGit].HTML)
			for _, w := range c.want {
				if !strings.Contains(html, w) {
					t.Errorf("%s (adapter %v): missing %q in %s", c.name, adapter, w, html)
				}
			}
			for _, n := range c.not {
				if strings.Contains(html, n) {
					t.Errorf("%s (adapter %v): unexpected %q in %s", c.name, adapter, n, html)
				}
			}
		}
	}
	regions, err := renderSessionRegions(SessionView{ReplyID: "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if html := regions[regionGit].HTML; html != "" {
		t.Errorf("no badge renders %q, want nothing", html)
	}
}

// TestGitBadgeSitsBetweenTheLiveMarkAndTheGear pins the badge's place in
// the session page's top bar.
func TestGitBadgeSitsBetweenTheLiveMarkAndTheGear(t *testing.T) {
	view := SessionView{ReplyID: "x", Git: &GitBadge{State: gitPending, Root: "/p"}}
	regions, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderSessionPage(view, regions, false)
	if err != nil {
		t.Fatal(err)
	}
	sse, git, gear := strings.Index(page, `id="sse"`), strings.Index(page, `id="r-git"`), strings.Index(page, `id="gear"`)
	if sse < 0 || git < sse || gear < git {
		t.Fatalf("top bar order: sse %d, git %d, gear %d", sse, git, gear)
	}
}

// TestReadGitBadge pins the daemon's read on real repositories: branch,
// upstream standing, and counts; a detached HEAD; a root that is not a
// repository; and a root git cannot enter, which fails with git's reason.
func TestReadGitBadge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	origin, work := filepath.Join(base, "origin"), filepath.Join(base, "work")
	gitRun(t, base, "init", "-q", "-b", "main", origin)
	gitCommit(t, origin, "a")
	gitRun(t, base, "clone", "-q", origin, work)
	gitCommit(t, work, "b")
	if err := os.WriteFile(filepath.Join(work, "a"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "new"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := readGitBadge(work)
	if b.State != gitOK || b.Branch != "main" || b.Detached || b.Upstream != "origin/main" || b.Ahead != 1 || b.Behind != 0 || b.Modified != 1 || b.Untracked != 1 || b.Staged != 0 {
		t.Fatalf("work tree badge = %+v", b)
	}
	gitRun(t, work, "checkout", "-q", "--detach")
	if b = readGitBadge(work); b.State != gitOK || !b.Detached || b.Branch == "" || b.Upstream != "" {
		t.Fatalf("detached badge = %+v", b)
	}
	if b = readGitBadge(t.TempDir()); b.State != gitNoRepo || b.Err != "" {
		t.Fatalf("non-repository badge = %+v", b)
	}
	if b = readGitBadge(filepath.Join(base, "missing")); b.State != gitFailed || b.Err == "" {
		t.Fatalf("missing root badge = %+v", b)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func gitCommit(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-q", "-m", name)
}

// gitBadgeOn runs a fixture daemon with the git badge on f's read.
func gitBadgeOn(f *fakeGit) func(*DaemonOptions) {
	return func(o *DaemonOptions) {
		o.noGitBadge = false
		o.gitRead = f.read
	}
}

// TestGitRefreshRouteAndStream pins the badge end to end: the page renders
// it pending, an open stream starts the root's read and sends the badge, the
// refresh route needs the daemon's own Origin and a watching page, and its
// read reaches the stream; the stream's close stops the reads.
func TestGitRefreshRouteAndStream(t *testing.T) {
	git := newFakeGit()
	f := startPageFixture(t, gitBadgeOn(git))
	f.d.git.gap = 0
	route := f.base + "/s/livesession/git"

	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `class="btn sb gitb pending"`) {
		t.Fatalf("page lacks the pending badge")
	}
	// No page watches the root yet: nothing is read.
	if code, _, raw := postJSON(t, route, f.base, ""); code != http.StatusConflict {
		t.Fatalf("refresh with no stream = %d %s, want 409", code, raw)
	}
	for _, origin := range []string{"", "null", "http://evil.example"} {
		if code, header, _ := postJSON(t, route, origin, ""); code != http.StatusForbidden {
			t.Errorf("refresh with Origin %q = %d, want 403", origin, code)
		} else {
			assertNoCORS(t, header)
		}
	}
	if code, _ := get(t, route); code != http.StatusMethodNotAllowed {
		t.Errorf("GET refresh = %d, want 405", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/git", f.base, ""); code != http.StatusNotFound {
		t.Errorf("refresh of an unknown session = %d, want 404", code)
	}
	if n := git.readCount(f.project); n != 0 {
		t.Fatalf("reads before any stream = %d", n)
	}

	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/livesession/events?h=" + heldOf(page) + ",mode." + newRegion(modeLive).Hash)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	gitEvent := func(want string) {
		t.Helper()
		for {
			ev, ok := nextRegion(t, events, 2*time.Second)
			if !ok {
				t.Fatalf("no git region carrying %q", want)
			}
			if ev.Name == regionGit && strings.Contains(ev.HTML, want) {
				return
			}
		}
	}
	gitEvent(`>↑1</button>`)

	if code, _, raw := postJSON(t, route, f.base, ""); code != http.StatusNoContent {
		t.Fatalf("refresh = %d %s, want 204", code, raw)
	}
	gitEvent(`>↑2</button>`)

	resp.Body.Close()
	waitFor(t, "the root to be dropped", func() bool { return watched(f.d.git) == 0 })
	if n := git.readCount(f.project); n != 2 {
		t.Fatalf("reads = %d, want 2", n)
	}
}

// TestGitBadgeFollowsTheSnapshot pins follow on the watcher: a snapshot whose
// repo figures differ from the badge's read asks for a read, the same
// snapshot again asks for nothing even while the two still disagree, one
// that agrees with the read asks for nothing, and neither does a snapshot
// without a branch or of a root no page watches.
func TestGitBadgeFollowsTheSnapshot(t *testing.T) {
	f := newFakeGit()
	g, _ := newTestWatcher(t, f)
	g.gap = 0
	repo := func(ahead int) wslive.Repo {
		return wslive.Repo{Folder: "repo", Branch: "main", Upstream: "origin/main", Ahead: ahead}
	}
	// Before the first read lands nothing is asked for: that read is fresh.
	f.gate = make(chan struct{})
	release := g.watch("/repo")
	defer release()
	g.follow("/repo", "s1", repo(1))
	f.gate <- struct{}{}
	expectRead(t, f, "/repo") // the first read: ahead 1
	f.gate = nil
	expectNoRead(t, f)

	g.follow("/repo", "s1", repo(17))
	expectRead(t, f, "/repo") // ahead 2
	g.follow("/repo", "s1", repo(17))
	expectNoRead(t, f)

	waitFor(t, "the read to be stored", func() bool { return g.badge("/repo").Ahead == 2 })
	g.follow("/repo", "s1", repo(2)) // changed, but agrees with the read
	expectNoRead(t, f)

	g.follow("/repo", "s1", wslive.Repo{Folder: "repo"})
	g.follow("/other", "s1", repo(5))
	expectNoRead(t, f)

	g.follow("/repo", "s1", wslive.Repo{Folder: "repo", Branch: "main", Upstream: "origin/main", Ahead: 2, Modified: 1})
	expectRead(t, f, "/repo")
}

// TestGitBadgeFollowsTheStreamedSnapshot: a session page's stream hands its
// snapshot's repo figures to follow, so a snapshot change reads the badge
// again, and a snapshot that changed elsewhere does not.
func TestGitBadgeFollowsTheStreamedSnapshot(t *testing.T) {
	git := newFakeGit()
	f := startPageFixture(t, gitBadgeOn(git))
	f.d.git.gap = 0
	snapshot := func(reply string, ahead int) {
		writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: reply}},
			Repo: &wslive.Repo{Folder: "p", Branch: "main", Upstream: "origin/main", Ahead: ahead}})
		f.d.broker.wake("livesession")
	}
	snapshot("first reply", 1)
	_, page := get(t, f.base+"/s/livesession")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/livesession/events?h=" + heldOf(page) + ",mode." + newRegion(modeLive).Hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 64)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	waitRegion := func(name, want string) {
		t.Helper()
		for {
			ev, ok := nextRegion(t, events, 5*time.Second)
			if !ok {
				t.Fatalf("no %s region carrying %q", name, want)
			}
			if ev.Name == name && strings.Contains(ev.HTML, want) {
				return
			}
		}
	}
	expectRead(t, git, f.project)
	waitRegion(regionGit, `↑1<`)

	snapshot("first reply", 17)
	expectRead(t, git, f.project)
	waitRegion(regionGit, `↑2<`)

	snapshot("second reply", 17)
	waitRegion("convlive", "second reply")
	expectNoRead(t, git)
}
