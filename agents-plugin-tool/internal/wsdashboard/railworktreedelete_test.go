package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// poolSource lists a repository's worktrees with git, as gitWorktreeSource
// does, and answers pools as the repository's worktree pool roots.
type poolSource struct {
	gitWorktreeSource
	pools []string
}

func (s poolSource) ReadRepoWorktrees(repo string, regs []Registration) (RepoWorktrees, error) {
	list, err := s.gitWorktreeSource.ReadRepoWorktrees(repo, regs)
	list.Pools = s.pools
	return list, err
}

// deleteFixture is a fixture daemon that keeps stored workspaces, with a
// repository (worktreeRepo) stored as one; the rail's source answers the
// pool folders it was made with, under the fixture's base, as its pool roots.
type deleteFixture struct {
	*pageFixture
	// tmp is the folder the repository is made in (the fixture's base is
	// the daemon's URL).
	tmp, root string
}

func newDeleteFixture(t *testing.T, pools ...string) deleteFixture {
	t.Helper()
	base, root := worktreeRepo(t)
	var roots []string
	for _, p := range pools {
		roots = append(roots, filepath.Join(base, p))
	}
	f := startPageFixture(t, withWorkspaces(t), func(o *DaemonOptions) { o.Worktrees = poolSource{pools: roots} })
	if _, err := f.d.workspaces.add(root, false); err != nil {
		t.Fatal(err)
	}
	return deleteFixture{pageFixture: f, tmp: base, root: root}
}

// post sends a delete route's body from the daemon's own Origin.
func (f deleteFixture) post(t *testing.T, route string, req worktreeDeleteRequest) (int, string) {
	t.Helper()
	if req.Workspace == "" {
		req.Workspace = f.root
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	code, _, out := postJSONWith(t, &http.Client{Timeout: 30 * time.Second}, f.base+route, f.base, string(raw))
	return code, out
}

// check runs the preview of path and decodes its answer.
func (f deleteFixture) check(t *testing.T, path string) worktreeCheck {
	t.Helper()
	code, raw := f.post(t, worktreeDeletePreviewRoute, worktreeDeleteRequest{Path: path})
	var c worktreeCheck
	if err := json.Unmarshal([]byte(raw), &c); code != http.StatusOK || err != nil {
		t.Fatalf("preview of %s = %d %s (%v)", path, code, raw, err)
	}
	return c
}

// refused submits a delete of path that must be refused with refusal, and
// returns the fresh check the 409 carries.
func (f deleteFixture) refused(t *testing.T, req worktreeDeleteRequest, refusal string) worktreeCheck {
	t.Helper()
	code, raw := f.post(t, worktreeDeleteRoute, req)
	var c worktreeCheck
	if err := json.Unmarshal([]byte(raw), &c); code != http.StatusConflict || err != nil || c.Refusal != refusal || c.Note == "" {
		t.Fatalf("submit %+v = %d %s (%v), want 409 refused %q with a note", req, code, raw, err, refusal)
	}
	if !pathExists(req.Path) && refusal != deleteMissing && refusal != deleteUnknown {
		t.Fatalf("a refused submit removed %s", req.Path)
	}
	return c
}

// deleted submits a delete that must succeed and leave the folder gone.
func (f deleteFixture) deleted(t *testing.T, req worktreeDeleteRequest) worktreeDeleteAnswer {
	t.Helper()
	code, raw := f.post(t, worktreeDeleteRoute, req)
	var a worktreeDeleteAnswer
	if err := json.Unmarshal([]byte(raw), &a); code != http.StatusOK || err != nil || a.Message == "" {
		t.Fatalf("submit %+v = %d %s (%v), want 200 with a message", req, code, raw, err)
	}
	if pathExists(req.Path) {
		t.Fatalf("%s still exists after its delete", req.Path)
	}
	out, err := exec.Command("git", "-C", f.root, "worktree", "list", "--porcelain").Output()
	if err != nil || strings.Contains(string(out), "worktree "+req.Path+"\n") {
		t.Fatalf("git still lists %s after its delete (%v):\n%s", req.Path, err, out)
	}
	return a
}

func branchKept(t *testing.T, root, branch string) bool {
	t.Helper()
	return exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// TestWorktreeDeleteNeedsALocalViewer pins decision 5's gate: a viewer on
// another machine gets 404 from both routes even with the daemon's Origin,
// and a refused request deletes nothing. (The Origin and method gate is
// pinned with the other workspace routes.)
func TestWorktreeDeleteNeedsALocalViewer(t *testing.T) {
	f := newDeleteFixture(t)
	wt := filepath.Join(f.tmp, "lan")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "lan", wt)
	raw, err := json.Marshal(worktreeDeleteRequest{Workspace: f.root, Path: wt, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	for route, handle := range map[string]http.HandlerFunc{
		worktreeDeletePreviewRoute: f.d.handleWorktreeDeletePreview,
		worktreeDeleteRoute:        f.d.handleWorktreeDelete,
	} {
		r := httptest.NewRequest(http.MethodPost, f.base+route, strings.NewReader(string(raw)))
		r.RemoteAddr = "192.168.0.2:1234"
		r.Header.Set("Origin", f.base)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handle(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s from a LAN viewer = %d %s, want 404", route, w.Code, w.Body)
		}
	}
	if !pathExists(wt) {
		t.Fatal("a LAN viewer's request deleted the worktree")
	}
	// The same request from this machine reaches the route.
	if c := f.check(t, wt); c.Refusal != "" || c.Folder != "lan" || c.Branch != "lan" {
		t.Errorf("local preview = %+v, want a deletable check of lan", c)
	}
}

// TestWorktreeDeleteRefusals: the preview and the submit refuse the primary
// worktree, a path git does not list (and a workspace the rail does not
// show), a pooled worktree under each pool root (the rail's, the default
// one, the legacy in-tree one), a worktree whose folder is gone, a locked
// worktree, and a worktree holding a live session; force changes none of
// it. An ended session is listed only.
func TestWorktreeDeleteRefusals(t *testing.T) {
	f := newDeleteFixture(t, "custompool")
	custom := filepath.Join(f.tmp, "custompool")

	if code, _ := f.post(t, worktreeDeletePreviewRoute, worktreeDeleteRequest{Workspace: f.project, Path: f.project}); code != http.StatusNotFound {
		t.Errorf("preview in a plain folder = %d, want 404", code)
	}
	if code, _ := f.post(t, worktreeDeleteRoute, worktreeDeleteRequest{Workspace: filepath.Join(f.tmp, "nowhere"), Path: f.root}); code != http.StatusNotFound {
		t.Errorf("submit in an unknown workspace = %d, want 404", code)
	}

	if c := f.check(t, f.root); c.Refusal != deletePrimary || c.Note == "" {
		t.Errorf("primary preview = %+v, want refused as primary", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: f.root, Force: true}, deletePrimary)

	stray := mkDir(t, filepath.Join(f.tmp, "stray"))
	for _, p := range []string{stray, "relative/path", ""} {
		if c := f.check(t, p); c.Refusal != deleteUnknown {
			t.Errorf("preview of %q = %+v, want refused as unknown", p, c)
		}
	}
	f.refused(t, worktreeDeleteRequest{Path: stray, Force: true}, deleteUnknown)

	for name, path := range map[string]string{
		"rail pool":    filepath.Join(custom, "a"),
		"default pool": filepath.Join(f.tmp, ".ws-worktrees", "repo", "b"),
		"legacy pool":  filepath.Join(f.root, ".ws-worktrees", "c"),
	} {
		gitRun(t, f.root, "worktree", "add", "-q", "--detach", path)
		if c := f.check(t, path); c.Refusal != deletePooled || !strings.Contains(c.Note, "worktree.remove") {
			t.Errorf("%s preview = %+v, want refused as pooled, naming worktree.remove", name, c)
		}
		f.refused(t, worktreeDeleteRequest{Path: path, Force: true}, deletePooled)
	}

	gone := filepath.Join(f.tmp, "gone")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if c := f.check(t, gone); c.Refusal != deleteMissing {
		t.Errorf("missing preview = %+v, want refused as missing", c)
	}

	locked := filepath.Join(f.tmp, "locked")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "locked", locked)
	writeFile(t, filepath.Join(locked, "dirty.txt"), "x")
	gitRun(t, f.root, "worktree", "lock", "--reason", "on a USB disk", locked)
	c := f.check(t, locked)
	if c.Refusal != deleteLocked || !c.Locked || c.LockReason != "on a USB disk" || c.Dirty != 1 {
		t.Errorf("locked preview = %+v, want refused as locked with its reason and its one change", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: locked, Force: true, Token: c.Token}, deleteLocked)

	busy := filepath.Join(f.tmp, "busy")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "busy", busy)
	now := time.Now()
	// The index drops a session whose root folder is gone.
	register(t, f.dir, "busylive", mkDir(t, filepath.Join(busy, "sub")), now)
	ended := Registration{ReplyID: "busyended", Project: busy, Harness: "pi", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now), EndedAt: stampFor(now)}
	if err := WriteRegistration(f.dir, ended); err != nil {
		t.Fatal(err)
	}
	f.src.setPage("busylive", PageContent{})
	f.src.setPage("busyended", PageContent{})
	f.d.Index().Refresh()
	c = f.check(t, busy)
	if c.Refusal != deleteBusy || len(c.Holders) != 2 {
		t.Fatalf("busy preview = %+v, want refused as busy with both sessions", c)
	}
	if h := c.Holders[0]; h.Kind != "session" || h.ID != "busylive" || h.Harness != "claude" || !h.Live {
		t.Errorf("first holder = %+v, want the live session", h)
	}
	if h := c.Holders[1]; h.ID != "busyended" || h.Live {
		t.Errorf("second holder = %+v, want the ended session, listed only", h)
	}
	f.refused(t, worktreeDeleteRequest{Path: busy, Force: true, Token: c.Token}, deleteBusy)

	// Once the live session ends too, the worktree deletes.
	live := register(t, f.dir, "busylive", filepath.Join(busy, "sub"), now)
	live.EndedAt = stampFor(now)
	if err := WriteRegistration(f.dir, live); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	if c := f.check(t, busy); c.Refusal != "" || len(c.Holders) != 2 || c.Holders[0].Live || c.Holders[1].Live {
		t.Fatalf("preview with ended sessions = %+v, want deletable, both sessions listed", c)
	}
	f.deleted(t, worktreeDeleteRequest{Path: busy})
}

// TestWorktreeDeleteRefusesARunningTerminal: a terminal whose shell started
// in the worktree blocks its delete as a live session does, on real
// holders.
func TestWorktreeDeleteRefusesARunningTerminal(t *testing.T) {
	holders, python := termHoldersDir(t)
	base, root := worktreeRepo(t)
	manager := wsholder.New(wsholder.Options{Dir: holders, Python: python})
	tf := &termFixture{manager: manager, pageFixture: startPageFixture(t, withHolders(manager), withWorkspaces(t),
		func(o *DaemonOptions) { o.terminalShell = termTestShell; o.Worktrees = gitWorktreeSource{} })}
	if _, err := tf.d.workspaces.add(root, false); err != nil {
		t.Fatal(err)
	}
	f := deleteFixture{pageFixture: tf.pageFixture, tmp: base, root: root}
	wt := filepath.Join(base, "shell")
	gitRun(t, root, "worktree", "add", "-q", "-b", "shell", wt)
	body, err := json.Marshal(terminalCreate{Folder: "path", Path: mkDir(t, filepath.Join(wt, "pkg"))})
	if err != nil {
		t.Fatal(err)
	}
	term := tf.create(t, string(body))
	c := f.check(t, wt)
	if c.Refusal != deleteBusy || len(c.Holders) != 1 || c.Holders[0].Kind != "terminal" || c.Holders[0].ID != term.ID || c.Holders[0].Label != "livesession" || !c.Holders[0].Live {
		t.Fatalf("preview with a running terminal = %+v, want refused as busy by it", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: wt}, deleteBusy)
}

// TestWorktreeDeleteForce pins the force gate: a branch worktree whose
// commits no other branch reaches deletes without force and keeps its
// branch; changed or untracked files, a detached HEAD's unreached commits,
// and initialized submodules each need force, which the submit carries
// with the check's token; a state that changed after the check is refused
// again (needs force, or force on a stale token).
func TestWorktreeDeleteForce(t *testing.T) {
	f := newDeleteFixture(t)

	keep := filepath.Join(f.tmp, "keep")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "feat/keep", keep)
	gitCommit(t, keep, "only-here")
	c := f.check(t, keep)
	if c.Force || c.Dirty != 0 || c.Unreached != 0 || c.Branch != "feat/keep" || c.Detached || c.Refusal != "" {
		t.Errorf("branch worktree preview = %+v, want no force and the kept branch", c)
	}
	a := f.deleted(t, worktreeDeleteRequest{Path: keep})
	if a.Branch != "feat/keep" || !strings.Contains(a.Message, "feat/keep") || !branchKept(t, f.root, "feat/keep") {
		t.Errorf("branch worktree delete = %+v, want feat/keep kept and named", a)
	}

	dirty := filepath.Join(f.tmp, "dirty")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "dirty", dirty)
	writeFile(t, filepath.Join(dirty, "a"), "changed")
	writeFile(t, filepath.Join(dirty, "new.txt"), "untracked")
	c = f.check(t, dirty)
	if !c.Force || c.Dirty != 2 || strings.Join(c.DirtyPaths, " ") != "a new.txt" || c.Token == "" || c.Refusal != "" {
		t.Errorf("dirty preview = %+v, want force over its two changes", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: dirty}, deleteForce)
	f.refused(t, worktreeDeleteRequest{Path: dirty, Force: true, Token: "stale"}, deleteChanged)
	f.deleted(t, worktreeDeleteRequest{Path: dirty, Force: true, Token: c.Token})
	if !branchKept(t, f.root, "dirty") {
		t.Error("a forced delete lost the branch")
	}

	// Checked clean, then dirtied: the submit needs force again.
	late := filepath.Join(f.tmp, "late")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "late", late)
	if c := f.check(t, late); c.Force {
		t.Fatalf("clean preview = %+v, want no force", c)
	}
	writeFile(t, filepath.Join(late, "late.txt"), "x")
	if c := f.refused(t, worktreeDeleteRequest{Path: late}, deleteForce); !c.Force || c.Dirty != 1 {
		t.Errorf("the refused submit's check = %+v, want the new change and force", c)
	}
	// Checked dirty, then dirtied further: the force given on the first
	// check's token covers only what that check showed.
	before := f.check(t, late)
	writeFile(t, filepath.Join(late, "later.txt"), "y")
	fresh := f.refused(t, worktreeDeleteRequest{Path: late, Force: true, Token: before.Token}, deleteChanged)
	if fresh.Dirty != 2 || fresh.Token == before.Token {
		t.Errorf("the changed check = %+v, want both changes and a new token", fresh)
	}
	f.deleted(t, worktreeDeleteRequest{Path: late, Force: true, Token: fresh.Token})

	// A detached HEAD: commits no ref reaches need force; none, none.
	bare := filepath.Join(f.tmp, "bare")
	gitRun(t, f.root, "worktree", "add", "-q", "--detach", bare, "main")
	if c := f.check(t, bare); c.Force || !c.Detached || c.Unreached != 0 || c.Branch != "" {
		t.Errorf("detached preview without commits = %+v, want no force", c)
	}
	f.deleted(t, worktreeDeleteRequest{Path: bare})
	lost := filepath.Join(f.tmp, "lost")
	gitRun(t, f.root, "worktree", "add", "-q", "--detach", lost, "main")
	gitCommit(t, lost, "x1")
	gitCommit(t, lost, "x2")
	c = f.check(t, lost)
	if !c.Force || c.Unreached != 2 || c.Dirty != 0 {
		t.Errorf("detached preview with commits = %+v, want force over its two unreached commits", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: lost}, deleteForce)
	f.deleted(t, worktreeDeleteRequest{Path: lost, Force: true, Token: c.Token})

	// Initialized submodules: removed only with force (git itself refuses
	// them without --force).
	sub := filepath.Join(f.tmp, "subrepo")
	gitRun(t, f.tmp, "init", "-q", "-b", "main", sub)
	gitCommit(t, sub, "s")
	gitRun(t, f.root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "mod")
	gitRun(t, f.root, "commit", "-q", "-m", "add mod")
	withSub := filepath.Join(f.tmp, "withsub")
	gitRun(t, f.root, "worktree", "add", "-q", "-b", "withsub", withSub)
	if c := f.check(t, withSub); c.Submodules || c.Force {
		t.Errorf("preview with an uninitialized submodule = %+v, want neither submodules nor force", c)
	}
	gitRun(t, withSub, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
	c = f.check(t, withSub)
	if !c.Submodules || !c.Force || c.Dirty != 0 {
		t.Errorf("preview with an initialized submodule = %+v, want submodules and force", c)
	}
	f.refused(t, worktreeDeleteRequest{Path: withSub}, deleteForce)
	f.deleted(t, worktreeDeleteRequest{Path: withSub, Force: true, Token: c.Token})
}

// TestWorktreeHolders pins which sessions and terminals a worktree holds:
// a folder at or below it (also through a symlink), never a sibling that
// shares its name's prefix; a live session and a terminal that may still
// run block, the live ones first; an ended session is listed unless the
// index page leaves it out, and an exited terminal is listed.
func TestWorktreeHolders(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wt := mkDir(t, filepath.Join(base, "wt"))
	mkDir(t, filepath.Join(wt, "deep"))
	link := filepath.Join(base, "link")
	if err := os.Symlink(wt, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	now := time.Now()
	sess := func(id, project string, live, unlisted bool) Session {
		reg := Registration{ReplyID: id, Project: project, Harness: "claude", HeartbeatAt: stampFor(now)}
		if !live {
			reg.EndedAt = stampFor(now)
		}
		reg.Unlisted = unlisted
		return Session{Registration: reg, Label: "label " + id}
	}
	sessions := []Session{
		sess("ended", filepath.Join(wt, "deep"), false, false),
		sess("sibling", wt+"-other", true, false),
		sess("outside", base, true, false),
		sess("vialink", filepath.Join(link, "deep"), true, false),
		sess("hidden", wt, false, true),
		sess("worker", wt, true, true),
	}
	terms := []TerminalInfo{
		{ID: "t-exited", Session: "s1", Cwd: wt, State: "exited"},
		{ID: "t-lost", Session: "s2", Cwd: filepath.Join(wt, "deep"), State: "lost"},
		{ID: "t-out", Session: "s3", Cwd: base, State: "running"},
	}
	got := worktreeHolders(wt, sessions, terms, now)
	var line []string
	for _, h := range got {
		live := "listed"
		if h.Live {
			live = "live"
		}
		line = append(line, h.Kind+":"+h.ID+":"+live)
	}
	want := "session:vialink:live session:worker:live terminal:t-lost:live session:ended:listed terminal:t-exited:listed"
	if strings.Join(line, " ") != want {
		t.Errorf("holders = %v\nwant %s", line, want)
	}
	if got[0].Label != "label vialink" || got[2].Label != "s2" {
		t.Errorf("holders = %+v, want the session's label and the terminal's opening session", got)
	}
	if len(worktreeHolders(wt, nil, nil, now)) != 0 {
		t.Error("an empty worktree holds something")
	}
}

// TestRailRowsOfferDeleteOnLinkedWorktrees pins the deletable flag and its
// button: a linked worktree git lists, not the primary, not prunable, not
// under any pool root (the rail's, the default, the legacy in-tree one;
// pooled rows show while they hold a session) and not a session root the
// list does not name; the button shows only on such rows of a daemon that
// keeps stored workspaces.
func TestRailRowsOfferDeleteOnLinkedWorktrees(t *testing.T) {
	now := time.Now()
	repo := "/w/repo"
	in := func(id, project string) railInput {
		return railInput{Session: railSession(id, project, now.Add(-time.Minute), time.Time{}), Repo: repo}
	}
	sessions := []railInput{
		in("inpool", "/w/custom/a"),
		in("indefault", "/w/.ws-worktrees/repo/b"),
		in("inlegacy", "/w/repo/.ws-worktrees/c"),
		in("unlisted", "/w/fresh"),
	}
	repos := map[string]railRepo{filepath.FromSlash(repo): {Read: true, Pools: []string{"/w/custom"}, Entries: []railWorktreeEntry{
		{Path: repo, Branch: "main"},
		{Path: "/w/feature", Branch: "feat"},
		{Path: "/w/idle", Detached: true},
		{Path: "/w/custom/a", Detached: true},
		{Path: "/w/.ws-worktrees/repo/b", Detached: true},
		{Path: "/w/repo/.ws-worktrees/c", Detached: true},
		{Path: "/w/gone", Branch: "old", Prunable: true},
	}}}
	view := buildRailView(sessions, repos, nil, now)
	if len(view.Workspaces) != 1 {
		t.Fatalf("workspaces = %+v", view.Workspaces)
	}
	got := map[string]bool{}
	for _, w := range view.Workspaces[0].Worktrees {
		got[filepath.ToSlash(w.Path)] = w.Deletable
	}
	want := map[string]bool{
		repo: false, "/w/feature": true, "/w/idle": true,
		"/w/custom/a": false, "/w/.ws-worktrees/repo/b": false, "/w/repo/.ws-worktrees/c": false,
		"/w/fresh": false,
	}
	for p, d := range want {
		if g, ok := got[p]; !ok || g != d {
			t.Errorf("%s: deletable = %v (shown %v), want %v", p, g, ok, d)
		}
	}
	if _, ok := got["/w/gone"]; ok {
		t.Error("a prunable worktree shows")
	}
	unread := buildRailView(sessions[3:], map[string]railRepo{}, nil, now)
	for _, w := range unread.Workspaces[0].Worktrees {
		if w.Deletable {
			t.Errorf("%s is deletable before the list was read", w.Path)
		}
	}

	view.CanAdd = true
	region, err := renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	if n := strings.Count(html, `class="rdel"`); n != 2 {
		t.Errorf("delete buttons = %d, want 2:\n%s", n, html)
	}
	if !strings.Contains(html, `<button type="button" class="rdel" data-ws="`+filepath.FromSlash(repo)+`" data-path="`+filepath.FromSlash("/w/feature")+`" data-folder="feature" title="Delete the worktree feature" aria-label="Delete the worktree feature" aria-haspopup="dialog">`) {
		t.Errorf("the feature row's delete button is not as expected:\n%s", html)
	}
	view.CanAdd = false
	region, err = renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(region.HTML), "rdel") {
		t.Error("a daemon without stored workspaces offers a delete")
	}
}
