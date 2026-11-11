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
	view := buildRailView(sessions, repos, nil, now)
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
	}, map[string]railRepo{"/r": {Read: false, Entries: []railWorktreeEntry{{Path: "/r-other"}}}}, nil, now)
	if len(view.Workspaces) != 1 || len(view.Workspaces[0].Worktrees) != 2 {
		t.Fatalf("view = %+v, want the two session roots", view)
	}
	// A session in a folder of the root worktree sits under it; a linked
	// worktree inside the repository folder is its own row.
	sub := buildRailView([]railInput{
		{Session: railSession("a", "/r/sub/dir", now, time.Time{}), Repo: "/r"},
		{Session: railSession("b", "/r/.ws-worktrees/w", now, time.Time{}), Repo: "/r", Linked: true},
	}, nil, nil, now)
	if wts := sub.Workspaces[0].Worktrees; len(wts) != 2 || wts[0].Path != "/r" || len(wts[0].Sessions) != 1 || wts[0].Sessions[0].ReplyID != "a" || wts[1].Path != "/r/.ws-worktrees/w" {
		t.Errorf("worktrees = %+v, want a under the root and b on its own", wts)
	}
	if !view.Workspaces[0].Worktrees[0].Root {
		t.Errorf("first worktree = %+v, want the root", view.Workspaces[0].Worktrees[0])
	}
}

// TestRailViewShowsStoredWorkspaces pins how the stored workspaces join the
// grouping: a stored one shows without a session, a repository with the
// worktrees its list names (a pooled one still hidden); a stored and a
// session's repository of the same root are one bar; a session in no
// repository joins the longest stored plain folder at or above its root,
// while one in a repository stays under its repository even below a stored
// folder, and a stored repository takes no plain session.
func TestRailViewShowsStoredWorkspaces(t *testing.T) {
	now := time.Now()
	stored := []storedWorkspace{
		{Path: "/w/idle-repo", Repo: true},
		{Path: "/w/busy-repo", Repo: true},
		{Path: "/w/projects"},
		{Path: "/w/projects/notes"},
		{Path: "/w/empty-folder"},
	}
	sessions := []railInput{
		{Session: railSession("inrepo", "/w/busy-repo", now, time.Time{}), Repo: "/w/busy-repo"},
		{Session: railSession("nested", "/w/projects/app", now, time.Time{}), Repo: "/w/projects/app"},
		{Session: railSession("loose", "/w/projects/scratch/a", now, time.Time{})},
		{Session: railSession("deep", "/w/projects/notes/2026", now, time.Time{})},
		{Session: railSession("exact", "/w/projects/notes", now.Add(-time.Hour), time.Time{})},
		{Session: railSession("stray", "/w/elsewhere", now, time.Time{})},
		{Session: railSession("underrepo", "/w/idle-repo/docs", now, time.Time{})},
	}
	repos := map[string]railRepo{"/w/idle-repo": {Read: true, Pools: []string{"/w/.ws-worktrees/idle-repo"}, Entries: []railWorktreeEntry{
		{Path: "/w/idle-repo", Branch: "main"},
		{Path: "/w/idle-wt", Branch: "feat/idle"},
		{Path: "/w/.ws-worktrees/idle-repo/spare", Detached: true},
	}}}
	view := buildRailView(sessions, repos, stored, now)
	byPath := map[string]RailWorkspace{}
	var paths []string
	for _, ws := range view.Workspaces {
		byPath[ws.Path] = ws
		paths = append(paths, ws.Path)
	}
	want := []string{"/w/busy-repo", "/w/elsewhere", "/w/empty-folder", "/w/idle-repo", "/w/idle-repo/docs", "/w/projects", "/w/projects/app", "/w/projects/notes"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Fatalf("workspaces = %v, want %v", paths, want)
	}
	for path, wantStored := range map[string]bool{"/w/busy-repo": true, "/w/idle-repo": true, "/w/projects": true, "/w/projects/notes": true, "/w/empty-folder": true,
		"/w/elsewhere": false, "/w/projects/app": false, "/w/idle-repo/docs": false} {
		if byPath[path].Stored != wantStored {
			t.Errorf("%s: stored = %v, want %v", path, byPath[path].Stored, wantStored)
		}
	}
	idle := byPath["/w/idle-repo"]
	if !idle.Repo || idle.Live != 0 || len(idle.Worktrees) != 2 || !idle.Worktrees[0].Root || idle.Worktrees[0].Branch != "main" ||
		idle.Worktrees[1].Path != "/w/idle-wt" || idle.Worktrees[1].Branch != "feat/idle" || !idle.Worktrees[1].Empty() {
		t.Errorf("stored repository without a session = %+v, want its root and its unpooled worktree, both empty", idle)
	}
	if busy := byPath["/w/busy-repo"]; !busy.Repo || busy.Live != 1 || len(busy.Worktrees) != 1 {
		t.Errorf("stored repository with a session = %+v, want one bar holding it", busy)
	}
	if app := byPath["/w/projects/app"]; !app.Repo || app.Live != 1 {
		t.Errorf("repository under a stored folder = %+v, want its own workspace", app)
	}
	if got := railIDs(byPath["/w/projects"].Sessions); strings.Join(got, " ") != "loose" {
		t.Errorf("stored folder's sessions = %v, want loose only", got)
	}
	if got := railIDs(byPath["/w/projects/notes"].Sessions); strings.Join(got, " ") != "deep exact" {
		t.Errorf("nested stored folder's sessions = %v, want deep then exact", got)
	}
	if empty := byPath["/w/empty-folder"]; empty.Repo || len(empty.Sessions) != 0 || len(empty.Worktrees) != 0 {
		t.Errorf("stored folder without a session = %+v, want an empty bar", empty)
	}
	if docs := byPath["/w/idle-repo/docs"]; docs.Repo || len(docs.Sessions) != 1 {
		t.Errorf("plain session under a stored repository = %+v, want a workspace of its own", docs)
	}
	if view.Live != 7 {
		t.Errorf("live = %d, want 7", view.Live)
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
	}, map[string]railRepo{"/r": {Read: true, Entries: []railWorktreeEntry{{Path: "/r"}, {Path: "/r2"}}}}, nil, now)
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

// TestRailRowFailure pins the error state: a row takes the typed failure
// its runner reports in the snapshot's host state while the adapter is
// present; a stale snapshot, or one without host state, carries none.
func TestRailRowFailure(t *testing.T) {
	now := time.Now()
	ms := now.UnixMilli()
	fail := &wslive.Notice{Kind: wslive.NoticeLogin, Text: "please run /login"}
	snap := &wslive.Snapshot{Adapter: "claude", UpdatedAt: ms, HeartbeatMs: 30000, Host: &wslive.Host{Failure: fail}}
	hosted := railSession("h", "/p", now, time.Time{})
	hosted.Hosted = true
	if row := railRow(railInput{Session: hosted, Snapshot: snap}, true, now); row.Failure == nil || *row.Failure != *fail {
		t.Errorf("a failed session's row = %+v, want its failure", row.Failure)
	}
	stale := *snap
	stale.UpdatedAt = ms - time.Hour.Milliseconds()
	if row := railRow(railInput{Session: hosted, Snapshot: &stale}, true, now); row.Failure != nil {
		t.Errorf("a stale snapshot = %+v, want no failure", row.Failure)
	}
	bare := *snap
	bare.Host = &wslive.Host{BackgroundTasks: 1}
	if row := railRow(railInput{Session: hosted, Snapshot: &bare}, true, now); row.Failure != nil {
		t.Errorf("host state without a failure = %+v, want none", row.Failure)
	}
}

// TestRailRowApproval pins the awaiting-approval state: a live hosted
// session whose lead waits on a permission carries the tool and its
// arguments, and does not count as working; a session the daemon does not
// host (its wait is answered in its terminal), a stale snapshot, an ended
// row, and a lead in any other phase carry none; the view and each
// workspace count the waiting rows.
func TestRailRowApproval(t *testing.T) {
	now := time.Now()
	ms := now.UnixMilli()
	wait := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission, Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "git push", StartedAt: ms}}}
	snap := &wslive.Snapshot{Adapter: "claude", UpdatedAt: ms, HeartbeatMs: 30000, Lead: wait}
	hosted := railSession("h", "/p", now, time.Time{})
	hosted.Hosted = true
	cli := railSession("c", "/p", now, time.Time{})

	row := railRow(railInput{Session: hosted, Snapshot: snap}, true, now)
	if !row.Hosted || row.Working || row.Approval == nil || *row.Approval != (ApprovalWait{Tool: "Bash", Args: "git push"}) {
		t.Errorf("hosted waiting row = %+v (approval %+v), want the Bash wait", row, row.Approval)
	}
	if row := railRow(railInput{Session: hosted}, true, now); !row.Hosted || row.Approval != nil {
		t.Errorf("hosted row without a snapshot = %+v, want the hosted mark and no wait", row)
	}
	if row := railRow(railInput{Session: cli, Snapshot: snap}, true, now); row.Hosted || row.Approval != nil {
		t.Errorf("a session the daemon does not host = %+v, want no wait", row)
	}
	stale := *snap
	stale.UpdatedAt = ms - time.Hour.Milliseconds()
	if row := railRow(railInput{Session: hosted, Snapshot: &stale}, true, now); row.Approval != nil {
		t.Errorf("a stale snapshot = %+v, want no wait", row.Approval)
	}
	if row := railRow(railInput{Session: hosted, Snapshot: snap}, false, now); row.Approval != nil {
		t.Errorf("an ended row = %+v, want no wait", row.Approval)
	}
	busy := *snap
	busy.Lead = wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool}}
	if row := railRow(railInput{Session: hosted, Snapshot: &busy}, true, now); row.Approval != nil || !row.Working {
		t.Errorf("a working lead = %+v, want no wait", row)
	}
	bare := *snap
	bare.Lead = wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission}}
	if row := railRow(railInput{Session: hosted, Snapshot: &bare}, true, now); row.Approval == nil || *row.Approval != (ApprovalWait{}) {
		t.Errorf("a wait without its detail = %+v, want an empty wait", row.Approval)
	}

	other := railSession("o", "/q", now, time.Time{})
	other.Hosted = true
	view := buildRailView([]railInput{
		{Session: hosted, Snapshot: snap},
		{Session: cli, Snapshot: snap},
		{Session: other, Snapshot: snap},
		{Session: railSession("idle", "/q", now, time.Time{})},
	}, nil, nil, now)
	if view.Live != 4 || view.CountApproval != 2 {
		t.Errorf("view live %d approvals %d, want 4 and 2", view.Live, view.CountApproval)
	}
	for _, ws := range view.Workspaces {
		if ws.Approvals != 1 {
			t.Errorf("workspace %s approvals = %d, want 1", ws.Path, ws.Approvals)
		}
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
	// A page moving to another session closes its stream and opens the
	// next: within the interval of the last full read nothing is reread,
	// and the interval runs on from that read.
	release = l.watch()
	time.Sleep(50 * time.Millisecond)
	if fake.count("/a") != 3 {
		t.Errorf("a re-watch within the interval read /a again (%d reads)", fake.count("/a"))
	}
	ticks <- time.Now()
	waitFor(t, "the interval's full read after the re-watch", func() bool { return fake.count("/a") == 4 })
	release()
}

// TestRailListerSweepsOnFullReadsAndRereadsOneRepository pins the lister's
// two hooks for the stored workspaces: the sweep runs once before each full
// read and never on a single repository's read, and its change wakes the
// rail; a reread reads the one repository at once, the others keeping their
// lists, and with no viewer drops that repository's list so the next render
// reads it.
func TestRailListerSweepsOnFullReadsAndRereadsOneRepository(t *testing.T) {
	fake := &railFakeRead{reads: map[string]int{}}
	stop := make(chan struct{})
	defer close(stop)
	var mu sync.Mutex
	sweeps, changed := 0, 0
	l := newRailLister(fake.read, func() { mu.Lock(); changed++; mu.Unlock() }, stop)
	l.sweep = func() bool { mu.Lock(); sweeps++; mu.Unlock(); return true }
	count := func(n *int) int { mu.Lock(); defer mu.Unlock(); return *n }
	ticks := make(chan time.Time)
	l.after = func(time.Duration) <-chan time.Time { return ticks }
	wanted := map[string][]Registration{"/a": nil, "/b": nil}

	l.lists(wanted)
	release := l.watch()
	waitFor(t, "the first full read", func() bool { return fake.count("/a") == 1 && fake.count("/b") == 1 })
	waitFor(t, "both lists to land", func() bool { return len(l.lists(wanted)) == 2 })
	if count(&sweeps) != 1 {
		t.Fatalf("sweeps after the first full read = %d, want 1", count(&sweeps))
	}

	l.reread("/a/")
	l.reread("/not-wanted")
	waitFor(t, "the reread", func() bool { return fake.count("/a") == 2 })
	time.Sleep(50 * time.Millisecond)
	if fake.count("/b") != 1 || fake.count("/not-wanted") != 0 || count(&sweeps) != 1 {
		t.Errorf("a reread read /b %d times and an unwanted repository %d times, and swept %d times; want 1, 0, 1", fake.count("/b"), fake.count("/not-wanted"), count(&sweeps))
	}
	ticks <- time.Now()
	waitFor(t, "the interval's sweep and full read", func() bool { return count(&sweeps) == 2 && fake.count("/b") == 2 })
	if count(&changed) < 2 {
		t.Errorf("changed called %d times, want one per sweep that reported a change", count(&changed))
	}

	release()
	waitFor(t, "the goroutine to end", func() bool { l.mu.Lock(); defer l.mu.Unlock(); return !l.running })
	l.reread("/a")
	if got := l.lists(wanted); len(got) != 1 {
		t.Errorf("lists after a reread with no viewer = %v, want /b's only", got)
	}
	var none *railLister
	none.reread("/a")
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

// railSection is the rendered rail's section for the workspace at path.
func railSection(t *testing.T, html, path string) string {
	t.Helper()
	_, rest, ok := strings.Cut(html, `<section class="rws" data-ws="`+path+`">`)
	if !ok {
		t.Fatalf("the rail has no section for %s:\n%s", path, html)
	}
	section, _, _ := strings.Cut(rest, `</section>`)
	return section
}

// TestRailMarkupAddButtonsAndForget pins the rail's + buttons and forget:
// the header's add-workspace + and a repository bar's add-worktree + only on
// a daemon that keeps stored workspaces, never on a plain folder's bar or a
// worktree row; forget on a stored bar only, a plain folder's too; and a
// bar whose collapse toggle holds no other button, so the markup nests
// validly and each action is its own keyboard stop.
func TestRailMarkupAddButtonsAndForget(t *testing.T) {
	row := RailRow{ReplyID: "railrow", Harness: "claude", Live: true}
	view := RailView{CanAdd: true, Workspaces: []RailWorkspace{
		{Name: "stored-repo", Path: "/w/stored-repo", Repo: true, Stored: true, Worktrees: []RailWorktree{
			{Path: "/w/stored-repo", Root: true, Folder: "stored-repo", Branch: "main", Sessions: []RailRow{row}},
			{Path: "/w/stored-wt", Folder: "stored-wt", Branch: "feat/x"},
		}},
		{Name: "derived-repo", Path: "/w/derived-repo", Repo: true, Worktrees: []RailWorktree{{Path: "/w/derived-repo", Root: true, Branch: "main"}}},
		{Name: "stored-folder", Path: "/w/stored-folder", Stored: true},
		{Name: "derived-folder", Path: "/w/derived-folder", Sessions: []RailRow{row}},
	}}
	region, err := renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	head, _, _ := strings.Cut(html, `<div class="rlist"`)
	if !strings.Contains(head, `<button type="button" class="radd" data-add="ws"`) || !strings.Contains(head, `aria-haspopup="dialog"`) {
		t.Errorf("the rail header lacks the add-workspace +:\n%s", head)
	}
	for path, want := range map[string]struct{ add, forget bool }{
		"/w/stored-repo":    {true, true},
		"/w/derived-repo":   {true, false},
		"/w/stored-folder":  {false, true},
		"/w/derived-folder": {false, false},
	} {
		section := railSection(t, html, path)
		bar, body, _ := strings.Cut(section, `<div class="wsbody">`)
		if got := strings.Contains(bar, `class="radd" data-add="wt"`); got != want.add {
			t.Errorf("%s: add-worktree + on the bar = %v, want %v", path, got, want.add)
		}
		if got := strings.Contains(bar, `class="rforget"`); got != want.forget {
			t.Errorf("%s: forget on the bar = %v, want %v", path, got, want.forget)
		}
		if strings.Contains(body, `radd`) || strings.Contains(body, `rforget`) {
			t.Errorf("%s: a + or forget below the bar (a worktree row or a session):\n%s", path, body)
		}
		if strings.Contains(bar, `<button type="button" class="wsbar"`) || !strings.HasPrefix(bar, `<div class="wsbar">`) {
			t.Errorf("%s: the bar is not a container: %s", path, bar)
		}
		_, tog, ok := strings.Cut(bar, `<button type="button" class="wstog" aria-expanded="true" title="`+path+`">`)
		if !ok {
			t.Fatalf("%s: the bar lacks its collapse toggle: %s", path, bar)
		}
		tog, _, _ = strings.Cut(tog, `</button>`)
		if strings.Contains(tog, `<button`) {
			t.Errorf("%s: a button nests in the collapse toggle: %s", path, tog)
		}
	}
	if body := railSection(t, html, "/w/stored-folder"); !strings.Contains(body, `<div class="wsbody"></div>`) {
		t.Errorf("a stored folder without sessions = %s, want a bar with an empty body", body)
	}
	if !strings.Contains(railSection(t, html, "/w/stored-repo"), `title="Add a worktree to stored-repo"`) {
		t.Error("the add-worktree + does not name its workspace")
	}

	view.CanAdd = false
	view.Workspaces = view.Workspaces[1:2]
	region, err = renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(region.HTML), "radd") {
		t.Errorf("a daemon without stored workspaces shows a +:\n%s", region.HTML)
	}
}

// TestRailMarkupHostedSessions pins what the rail shows of hosted sessions.
// On a daemon that hosts them, whether or not it keeps stored workspaces,
// every worktree row ends in the new-session button, naming the folder a
// session would start in; a repository's bar and a plain folder's bar have
// none, since a lead session starts only at a Git worktree root. A hosted
// row carries the hosted mark. A row waiting on an approval has a still
// ring in place of the working one, the approve chip, the tool and its
// arguments as its second line, and the attributes the page's notification
// reads; its bar and the header count it. A row whose session stopped on a
// typed failure has a still error ring titled with the failure's kind.
func TestRailMarkupHostedSessions(t *testing.T) {
	wait := RailRow{ReplyID: "waits", Harness: "claude", Live: true, Hosted: true, Message: "last reply", Approval: &ApprovalWait{Tool: "mcp__ws__tracker_update", Args: "git push origin develop"}}
	plain := RailRow{ReplyID: "plain", Harness: "pi", Live: true, Working: true, Message: "hi"}
	failed := RailRow{ReplyID: "failed", Harness: "claude", Live: true, Hosted: true, Failure: &wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "limit reached"}}
	view := RailView{CanSpawn: true, Live: 3, CountApproval: 1, Workspaces: []RailWorkspace{
		{Name: "repo", Path: "/w/repo", Repo: true, Live: 3, Approvals: 1, Worktrees: []RailWorktree{
			{Path: "/w/repo", Root: true, Folder: "repo", Branch: "main", HasChanges: true, Changes: 2, Sessions: []RailRow{wait, plain, failed}},
			{Path: "/w/repo-wt", Folder: "repo-wt", Detached: true},
		}},
		{Name: "folder", Path: "/w/folder"},
	}}
	region, err := renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	const chip = `<span class="chip perm" title="sessions awaiting approval">`
	if head, _, _ := strings.Cut(html, `<div class="rlist"`); !strings.Contains(head, chip) {
		t.Errorf("the rail header lacks the approvals count:\n%s", head)
	}
	bar, body, _ := strings.Cut(railSection(t, html, "/w/repo"), `<div class="wsbody">`)
	if strings.Contains(bar, `class="rnew"`) || !strings.Contains(bar, chip) {
		t.Errorf("a repository bar = %s, want the approvals count and no new-session button", bar)
	}
	for _, want := range []string{
		`<button type="button" class="rnew" data-root="/w/repo" data-name="repo" data-branch="main" title="Start a session in main"`,
		`<button type="button" class="rnew" data-root="/w/repo-wt" data-name="repo" data-branch="detached HEAD" title="Start a session in detached HEAD"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the worktree rows lack %s:\n%s", want, body)
		}
	}
	if n := strings.Count(body, `class="rnew"`); n != 2 {
		t.Errorf("new-session buttons under the repository = %d, want one per worktree row", n)
	}
	if folder := railSection(t, html, "/w/folder"); strings.Contains(folder, `class="rnew"`) || strings.Contains(folder, chip) {
		t.Errorf("a plain folder = %s, want no new-session button and no count", folder)
	}
	rowOf := func(id string) string {
		_, rest, ok := strings.Cut(body, `data-rid="`+id+`"`)
		if !ok {
			t.Fatalf("no row for %s:\n%s", id, body)
		}
		row, _, _ := strings.Cut(rest, `</a></div>`)
		return row
	}
	waits := rowOf("waits")
	for _, want := range []string{
		` data-ap="1" data-ap-tool="mcp__ws__tracker_update" data-ap-args="git push origin develop">`,
		`<span class="host sm hb perm" title="awaiting approval">claude<svg class="ring" aria-hidden="true"><rect class="rtrk"/></svg></span>`,
		`<span class="hosted" title="session the dashboard runs">`,
		`<span class="chip perm" title="awaiting approval: tracker_update">`,
		`<span class="s2 ap"><b class="who">tracker_update</b>git push origin develop</span>`,
	} {
		if !strings.Contains(waits, want) {
			t.Errorf("the waiting row lacks %s:\n%s", want, waits)
		}
	}
	if strings.Contains(waits, "rrun") || strings.Contains(waits, "last reply") {
		t.Errorf("the waiting row shows a working ring or its last message:\n%s", waits)
	}
	other := rowOf("plain")
	for _, not := range []string{"data-ap", `class="hosted"`, "chip perm", "hb perm"} {
		if strings.Contains(other, not) {
			t.Errorf("a session the daemon does not host shows %q:\n%s", not, other)
		}
	}
	if !strings.Contains(other, "rrun") || !strings.Contains(other, `<span class="s2">hi</span>`) {
		t.Errorf("the working row lost its ring or message:\n%s", other)
	}
	if f := rowOf("failed"); !strings.Contains(f, `<span class="host sm hb err" title="usage limit reached">claude<svg class="ring" aria-hidden="true"><rect class="rtrk"/></svg></span>`) || strings.Contains(f, "rrun") {
		t.Errorf("the failed row lacks its error ring:\n%s", f)
	}
	if strings.Contains(waits, "hb err") || strings.Contains(other, "hb err") {
		t.Error("a row with no failure shows the error ring")
	}

	view.CanSpawn = false
	if region, err = renderRailRegion(view, "en"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(region.HTML), "rnew") {
		t.Errorf("a daemon that hosts no sessions shows a new-session button:\n%s", region.HTML)
	}
}
