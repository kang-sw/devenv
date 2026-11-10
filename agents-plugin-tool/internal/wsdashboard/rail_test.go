package wsdashboard

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// railSession is a listed session for buildRailView: live (heartbeat now)
// when ended is zero, else ended at ended.
func railSession(id, project string, registered time.Time, ended time.Time) Session {
	reg := Registration{ReplyID: id, Project: project, Harness: "claude", RegisteredAt: stampFor(registered), HeartbeatAt: stampFor(time.Now())}
	if !ended.IsZero() {
		reg.HeartbeatAt, reg.EndedAt = stampFor(ended), stampFor(ended)
	}
	return Session{Registration: reg}
}

func railIDs(rows []RailRow) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ReplyID)
	}
	return ids
}

func railWT(t *testing.T, ws RailWorkspace, path string) RailWorktree {
	t.Helper()
	for _, w := range ws.Worktrees {
		if w.Path == path {
			return w
		}
	}
	t.Fatalf("workspace %s has no worktree %s: %+v", ws.Path, path, ws.Worktrees)
	return RailWorktree{}
}

// TestRailViewGroupsSessionsByWorkspaceAndWorktree pins the grouping: a
// repository lists every worktree git lists (a sessionless one dimmed), a
// pooled worktree only while it holds a session, never a prunable one; a
// session in a subfolder sits under its worktree, one in a worktree no list
// names yet gets a worktree of its own; a plain folder lists its sessions
// directly; live rows go newest registration first, ended ones fold apart,
// and a session ended past the window is not listed.
func TestRailViewGroupsSessionsByWorkspaceAndWorktree(t *testing.T) {
	now := time.Now()
	repo := "/w/repo"
	pool := "/w/.ws-worktrees/repo"
	sessions := []railInput{
		{Session: railSession("oldroot", repo, now.Add(-2*time.Hour), time.Time{}), Repo: repo},
		{Session: railSession("newroot", repo+"/sub/dir", now.Add(-time.Minute), time.Time{}), Repo: repo},
		{Session: railSession("pooled", pool+"/busy", now.Add(-time.Hour), time.Time{}), Repo: repo},
		{Session: railSession("endedfeat", "/w/feature", now.Add(-5*time.Hour), now.Add(-3*time.Hour)), Repo: repo},
		{Session: railSession("oldended", "/w/feature", now.Add(-50*time.Hour), now.Add(-48*time.Hour)), Repo: repo},
		{Session: railSession("unlistedwt", "/w/fresh", now.Add(-time.Minute), time.Time{}), Repo: repo,
			Snapshot: &wslive.Snapshot{UpdatedAt: now.UnixMilli(), Repo: &wslive.Repo{Branch: "topic/fresh"}}},
		{Session: railSession("folder", "/w/plain", now.Add(-time.Minute), time.Time{})},
	}
	repos := map[string]railRepo{repo: {Read: true, Pools: []string{pool}, Entries: []railWorktreeEntry{
		{Path: repo, Branch: "develop"},
		{Path: "/w/feature", Branch: "feat/x"},
		{Path: "/w/idle", Detached: true},
		{Path: pool + "/busy", Branch: "impl/a"},
		{Path: pool + "/spare", Detached: true},
		{Path: "/w/gone", Branch: "old", Prunable: true},
	}}}
	view := buildRailView(sessions, repos, now)
	if len(view.Workspaces) != 2 {
		t.Fatalf("workspaces = %+v, want the repository and the folder", view.Workspaces)
	}
	if view.Live != 5 {
		t.Errorf("live = %d, want 5", view.Live)
	}
	plain, ws := view.Workspaces[0], view.Workspaces[1]
	if plain.Path != "/w/plain" || plain.Repo || len(plain.Worktrees) != 0 || len(plain.Sessions) != 1 {
		t.Errorf("plain folder = %+v", plain)
	}
	if ws.Name != "repo" || !ws.Repo || ws.Live != 4 {
		t.Errorf("repository = %+v", ws)
	}
	var paths []string
	for _, w := range ws.Worktrees {
		paths = append(paths, w.Path)
	}
	want := []string{repo, pool + "/busy", "/w/feature", "/w/fresh", "/w/idle"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("worktrees = %v, want %v (root first, the spare pooled and the prunable one hidden)", paths, want)
	}
	root := railWT(t, ws, repo)
	if !root.Root || root.Branch != "develop" || strings.Join(railIDs(root.Sessions), " ") != "newroot oldroot" {
		t.Errorf("root worktree = %+v, want develop with newroot then oldroot", root)
	}
	feat := railWT(t, ws, "/w/feature")
	if feat.Empty() || len(feat.Sessions) != 0 || strings.Join(railIDs(feat.Ended), " ") != "endedfeat" {
		t.Errorf("feature worktree = %+v, want endedfeat folded as ended only", feat)
	}
	if idle := railWT(t, ws, "/w/idle"); !idle.Empty() || !idle.Detached {
		t.Errorf("idle worktree = %+v, want an empty detached row", idle)
	}
	if fresh := railWT(t, ws, "/w/fresh"); fresh.Branch != "topic/fresh" || !fresh.HasChanges || fresh.Folder != "fresh" {
		t.Errorf("unlisted worktree = %+v, want its own row with the snapshot's branch", fresh)
	}
}

// TestRailViewWithoutAListShowsTheSessionRoots: before the worktree list is
// read (or when it failed) a repository shows only the worktrees its
// sessions are rooted in.
func TestRailViewWithoutAListShowsTheSessionRoots(t *testing.T) {
	now := time.Now()
	view := buildRailView([]railInput{
		{Session: railSession("a", "/r", now, time.Time{}), Repo: "/r"},
		{Session: railSession("b", "/r-wt", now, time.Time{}), Repo: "/r"},
	}, map[string]railRepo{"/r": {Read: false, Entries: []railWorktreeEntry{{Path: "/r-other"}}}}, now)
	if len(view.Workspaces) != 1 || len(view.Workspaces[0].Worktrees) != 2 {
		t.Fatalf("view = %+v, want the two session roots", view)
	}
	if !view.Workspaces[0].Worktrees[0].Root {
		t.Errorf("first worktree = %+v, want the root", view.Workspaces[0].Worktrees[0])
	}
}

// TestRailWorktreeChangesComeFromTheNewestSnapshot: a worktree's change
// count is the newest snapshot's of its sessions; none without a snapshot
// that carries repo fields.
func TestRailWorktreeChangesComeFromTheNewestSnapshot(t *testing.T) {
	now := time.Now()
	older := &wslive.Snapshot{UpdatedAt: now.Add(-time.Minute).UnixMilli(), Repo: &wslive.Repo{Modified: 9}}
	newer := &wslive.Snapshot{UpdatedAt: now.UnixMilli(), Repo: &wslive.Repo{Modified: 1, Staged: 2, Untracked: 3}}
	view := buildRailView([]railInput{
		{Session: railSession("a", "/r", now, time.Time{}), Repo: "/r", Snapshot: older},
		{Session: railSession("b", "/r", now, time.Time{}), Repo: "/r", Snapshot: newer},
		{Session: railSession("c", "/r2", now, time.Time{}), Repo: "/r", Snapshot: &wslive.Snapshot{UpdatedAt: now.UnixMilli()}},
	}, map[string]railRepo{"/r": {Read: true, Entries: []railWorktreeEntry{{Path: "/r"}, {Path: "/r2"}}}}, now)
	ws := view.Workspaces[0]
	if w := railWT(t, ws, "/r"); !w.HasChanges || w.Changes != 6 {
		t.Errorf("root = %+v, want 6 changes from the newer snapshot", w)
	}
	if w := railWT(t, ws, "/r2"); w.HasChanges {
		t.Errorf("r2 = %+v, want no count without repo fields", w)
	}
}

// TestRailRowStatus pins a row's status: working is a live adapter in a
// phase other than idle and permission; the robot counts running agents the
// lead spawned (no parent id), not their children; the last tool call is
// the newest running call's start or finished batch's end, a generating
// batch excluded; a stale snapshot keeps the last tool time and nothing
// else.
func TestRailRowStatus(t *testing.T) {
	now := time.Now()
	ms := now.UnixMilli()
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying, Batches: []wslive.BatchView{
		{CallCount: 2, StartedAt: ms - 9000, EndedAt: ms - 8000},
		{CallCount: 1, StartedAt: ms - 7000, Running: &wslive.RunningView{StartedAt: ms - 6000}},
		{CallCount: 0, StartedAt: ms - 1000, Generating: &wslive.GeneratingView{Kind: "text"}},
	}}}
	agents := []wslive.Agent{
		{ID: "a1", Status: wslive.StatusRunning},
		{ID: "a2", Status: wslive.StatusRunning},
		{ID: "a3", ParentID: "a1", Status: wslive.StatusRunning},
		{ID: "a4", Status: wslive.StatusDone},
	}
	fresh := &wslive.Snapshot{Adapter: "claude", UpdatedAt: ms, HeartbeatMs: 30000, Lead: lead, Agents: agents}
	row := railRow(railInput{Session: railSession("s", "/p", now, time.Time{}), Snapshot: fresh}, true, now)
	if !row.Working || row.Subagents != 2 || row.LastTool != ms-6000 {
		t.Errorf("fresh row = %+v, want working, 2 subagents, last tool at the running call", row)
	}
	for _, phase := range []string{"", wslive.PhaseIdle, wslive.PhasePermission} {
		if leadWorking(phase) {
			t.Errorf("phase %q counts as working", phase)
		}
	}
	stale := *fresh
	stale.UpdatedAt = ms - time.Hour.Milliseconds()
	row = railRow(railInput{Session: railSession("s", "/p", now, time.Time{}), Snapshot: &stale}, false, now)
	if row.Working || row.Subagents != 0 || row.LastTool != ms-6000 {
		t.Errorf("stale row = %+v, want no status but the last tool time", row)
	}
	if got := lastToolAt(wslive.Lead{Activity: wslive.Activity{Batches: []wslive.BatchView{{CallCount: 1, StartedAt: 5}}}}); got != 5 {
		t.Errorf("a batch with no end = %d, want its start", got)
	}
}

// TestRailRepoRoot pins how a session finds its repository: the registered
// common directory's parent when it is a .git folder, a plain folder for a
// bare or submodule common directory, and without one the session root's
// .git read from disk (a linked worktree's main repository, or the root).
func TestRailRepoRoot(t *testing.T) {
	if got := railRepoRoot(Registration{Project: "/x/wt", GitCommonDir: "/x/main/.git"}); got != "/x/main" {
		t.Errorf("common dir root = %q", got)
	}
	if got := railRepoRoot(Registration{Project: "/x/wt", GitCommonDir: "/x/bare.git"}); got != "" {
		t.Errorf("bare common dir = %q, want a plain folder", got)
	}
	plainRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(plainRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := railRepoRoot(Registration{Project: plainRepo}); got != plainRepo {
		t.Errorf("fallback repo = %q, want %q", got, plainRepo)
	}
	main := t.TempDir()
	gitdir := filepath.Join(main, ".git", "worktrees", "wt")
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := t.TempDir()
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := railRepoRoot(Registration{Project: linked}); got != main {
		t.Errorf("fallback linked worktree = %q, want %q", got, main)
	}
	if got := railRepoRoot(Registration{Project: t.TempDir()}); got != "" {
		t.Errorf("plain folder = %q, want empty", got)
	}
}

// TestRailMessagesReadARingOnlyWhenItGrew pins the message cache: the last
// user message or reply (mail and tool entries skipped), one line; a call at
// the same revision or with the same newest entry reads no entry file.
func TestRailMessagesReadARingOnlyWhenItGrew(t *testing.T) {
	dir := wslive.RingDir(t.TempDir())
	if err := wslive.AppendRing(dir, []wslive.RingEntry{
		{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "first\n  line   two"},
		{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:x", Title: "mail"},
	}); err != nil {
		t.Fatal(err)
	}
	c := newRailMessages()
	if got := c.last("s", 1, dir); got.Text != "first line two" || !got.User {
		t.Fatalf("last = %+v, want the user message on one line", got)
	}
	reads := c.reads
	c.last("s", 1, dir)
	c.last("s", 2, dir)
	if c.reads != reads {
		t.Errorf("an unchanged ring read %d entries", c.reads-reads)
	}
	if err := wslive.AppendRing(dir, []wslive.RingEntry{{Kind: wslive.RingReply, Text: strings.Repeat("y", 300)}}); err != nil {
		t.Fatal(err)
	}
	got := c.last("s", 3, dir)
	if got.User || len([]rune(got.Text)) != railMessageRunes || !strings.HasSuffix(got.Text, "…") {
		t.Errorf("last = %+v, want the reply cut to %d runes", got, railMessageRunes)
	}
	if c.reads != reads+1 {
		t.Errorf("the grown ring read %d entries, want 1", c.reads-reads)
	}
	if err := wslive.AppendRing(dir, []wslive.RingEntry{{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard,
		Text: "look at this\n\n" + wsinbox.ImageLine(1, "/secret/attach/a.png") + "\n[image: /old/b.png]"}}); err != nil {
		t.Fatal(err)
	}
	if got := c.last("s", 4, dir); got.Text != "look at this" {
		t.Errorf("last = %+v, want the user message without its image lines", got)
	}
}

// railFakeRead is a lister read counting the repositories read.
type railFakeRead struct {
	mu    sync.Mutex
	reads map[string]int
}

func (f *railFakeRead) read(repo string, _ []Registration) (RepoWorktrees, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads[repo]++
	return RepoWorktrees{Worktrees: []RepoWorktree{{Path: repo, Branch: "main"}, {Path: repo + "-wt", Branch: fmt.Sprintf("branch-read-%d", f.reads[repo])}}}, nil
}

func (f *railFakeRead) count(repo string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[repo]
}

// TestRailListerReadsOnlyWhilePagesShowTheRail pins the lister's cadence:
// no read without a viewer; the first viewer reads every wanted repository;
// a newly wanted one is read alone; the interval and a refresh read all;
// a refresh with no viewer is refused, and the goroutine ends with the last
// viewer.
func TestRailListerReadsOnlyWhilePagesShowTheRail(t *testing.T) {
	fake := &railFakeRead{reads: map[string]int{}}
	stop := make(chan struct{})
	defer close(stop)
	var changedMu sync.Mutex
	changed := 0
	l := newRailLister(fake.read, func() { changedMu.Lock(); changed++; changedMu.Unlock() }, stop)
	ticks := make(chan time.Time)
	l.after = func(time.Duration) <-chan time.Time { return ticks }

	regs := []Registration{{ReplyID: "s"}}
	if got := l.lists(map[string][]Registration{"/a": regs}); len(got) != 0 {
		t.Fatalf("lists before any read = %v", got)
	}
	time.Sleep(50 * time.Millisecond)
	if fake.count("/a") != 0 || l.refresh() {
		t.Fatal("the lister read or took a refresh with no viewer")
	}
	release := l.watch()
	waitFor(t, "the first viewer's read", func() bool { return fake.count("/a") == 1 })
	waitFor(t, "the list to land", func() bool { return len(l.lists(map[string][]Registration{"/a": regs})) == 1 })

	l.lists(map[string][]Registration{"/a": regs, "/b": regs})
	waitFor(t, "the new repository's read", func() bool { return fake.count("/b") == 1 })
	time.Sleep(50 * time.Millisecond)
	if fake.count("/a") != 1 {
		t.Errorf("a newly wanted repository re-read the others: /a read %d times", fake.count("/a"))
	}
	ticks <- time.Now()
	waitFor(t, "the interval's full read", func() bool { return fake.count("/a") == 2 && fake.count("/b") == 2 })
	if !l.refresh() {
		t.Fatal("refresh refused with a viewer")
	}
	waitFor(t, "the refresh's full read", func() bool { return fake.count("/a") == 3 && fake.count("/b") == 3 })
	changedMu.Lock()
	if changed < 3 {
		t.Errorf("changed called %d times, want one per changing read", changed)
	}
	changedMu.Unlock()

	release()
	waitFor(t, "the goroutine to end", func() bool { l.mu.Lock(); defer l.mu.Unlock(); return !l.running })
	if l.refresh() {
		t.Error("refresh taken after the last viewer left")
	}
}

// railFakeSource is a WorktreeSource that also lists repositories.
type railFakeSource struct {
	railFakeRead
}

func (f *railFakeSource) ReadWorktrees(Registration) (WorktreePool, error) {
	return WorktreePool{}, nil
}
func (f *railFakeSource) ReadRepoWorktrees(repo string, regs []Registration) (RepoWorktrees, error) {
	return f.read(repo, regs)
}

// TestSessionPagesShareOneRailRender pins the rail on the session pages:
// the page and its stream carry the rail with every listed session; a
// session's change re-renders the rail once for every open session page,
// and no session page that does not show the session; the worktree list
// is read once a stream opens and lands on the rail; the refresh route
// needs an open page.
func TestSessionPagesShareOneRailRender(t *testing.T) {
	src := &railFakeSource{railFakeRead{reads: map[string]int{}}}
	f := startPageFixture(t, func(o *DaemonOptions) { o.Worktrees = src })
	repo := t.TempDir()
	if err := WriteRegistration(f.dir, Registration{ReplyID: "reposession", Project: repo, Harness: "pi",
		RegisteredAt: stampFor(time.Now()), HeartbeatAt: stampFor(time.Now()), GitCommonDir: filepath.Join(repo, ".git")}); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	f.addSession(t, "lonersession", other, "alone")
	waitFor(t, "reposession to be indexed", func() bool { _, ok := f.d.Index().Session("reposession"); return ok })

	code, _, body := postJSON(t, f.base+railRoute, f.base, "")
	if code != http.StatusConflict {
		t.Errorf("refresh with no page = %d, want 409", code)
	}
	status, page := get(t, f.base+"/s/boardsession")
	if status != http.StatusOK {
		t.Fatalf("session page = %d", status)
	}
	for _, want := range []string{`id="rail"`, `data-rid="lonersession"`, `data-rid="reposession"`, `href="/s/livesession"`} {
		if !strings.Contains(page, want) {
			t.Errorf("session page lacks %s", want)
		}
	}

	live := openStream(t, f.base+"/s/livesession/events")
	board := openStream(t, f.base+"/s/boardsession/events")
	waitRegionHolding(t, live, regionRail, filepath.Base(repo)+"-wt")
	quiet(t, live)
	quiet(t, board)
	if src.count(repo) != 1 {
		t.Errorf("the repository was read %d times, want once for both streams", src.count(repo))
	}

	rails, liveReads := f.d.stats.RailRenders.Load(), f.src.readCount("livesession")
	f.change(t, "lonersession", other, "loner changed", 2)
	time.Sleep(settle)
	if got := f.d.stats.RailRenders.Load() - rails; got != 1 {
		t.Errorf("one session's change rendered the rail %d times, want 1", got)
	}
	if f.src.readCount("livesession") != liveReads {
		t.Error("an unrelated session's change re-read livesession's page")
	}

	code, _, body = postJSON(t, f.base+railRoute, f.base, "")
	if code != http.StatusNoContent {
		t.Fatalf("refresh = %d %s, want 204", code, body)
	}
	waitFor(t, "the refresh's read", func() bool { return src.count(repo) == 2 })
	waitRegionHolding(t, live, regionRail, "branch-read-2")
	if code, _, _ := postJSON(t, f.base+railRoute, "http://evil.example", ""); code != http.StatusForbidden {
		t.Errorf("cross-origin refresh = %d, want 403", code)
	}
}
