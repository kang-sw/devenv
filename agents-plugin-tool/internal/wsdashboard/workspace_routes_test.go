package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withWorkspaces runs a fixture daemon that keeps stored workspaces, in a
// file of the test's own.
func withWorkspaces(t *testing.T) func(*DaemonOptions) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "dashboard", "workspaces.json")
	return func(o *DaemonOptions) { o.WorkspacesFile = file }
}

// gitWorktreeSource lists a repository's worktrees with git itself, as the
// daemon's own source does.
type gitWorktreeSource struct{}

func (gitWorktreeSource) ReadWorktrees(Registration) (WorktreePool, error) {
	return WorktreePool{}, nil
}

func (gitWorktreeSource) ReadRepoWorktrees(repo string, _ []Registration) (RepoWorktrees, error) {
	out, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return RepoWorktrees{}, err
	}
	var list RepoWorktrees
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			list.Worktrees = append(list.Worktrees, RepoWorktree{Path: p})
		} else if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok && len(list.Worktrees) > 0 {
			list.Worktrees[len(list.Worktrees)-1].Branch = b
		}
	}
	return list, nil
}

// worktreeRepo makes a repository at <base>/repo with one commit on main
// and a second branch, free, that no worktree has checked out. The daemon's
// own git runs see no global or system config.
func worktreeRepo(t *testing.T) (base, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "repo")
	gitRun(t, base, "init", "-q", "-b", "main", root)
	gitCommit(t, root, "a")
	gitRun(t, root, "branch", "free")
	return base, root
}

func headBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

func pathBody(t *testing.T, key, path string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{key: path})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestWorkspaceRoutesNeedTheDaemonsOrigin pins the gate of every workspace
// route: a POST carrying the daemon's own Origin and nothing more, and no
// route at all on a daemon that keeps no stored workspaces.
func TestWorkspaceRoutesNeedTheDaemonsOrigin(t *testing.T) {
	f := startPageFixture(t, withWorkspaces(t))
	routes := []string{workspacePreviewRoute, workspaceAddRoute, workspaceForgetRoute, worktreeOptionsRoute, worktreeAddRoute, worktreeDeletePreviewRoute, worktreeDeleteRoute}
	for _, route := range routes {
		for _, o := range []string{"", "null", "http://evil.example"} {
			if code, header, _ := postJSON(t, f.base+route, o, `{"path":"`+f.project+`","workspace":"`+f.project+`"}`); code != http.StatusForbidden {
				t.Errorf("%s with Origin %q = %d, want 403", route, o, code)
			} else {
				assertNoCORS(t, header)
			}
		}
		if code, _ := get(t, f.base+route); code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want 405", route, code)
		}
		if code, _, _ := postJSON(t, f.base+route, f.base, `{not json`); code != http.StatusBadRequest {
			t.Errorf("%s with a malformed body = %d, want 400", route, code)
		}
	}
	if got := f.d.workspaces.workspaces(); len(got) != 0 {
		t.Fatalf("a refused request stored %v", got)
	}

	off := startPageFixture(t)
	for _, route := range routes {
		if code, _, _ := postJSON(t, off.base+route, off.base, `{"path":"`+off.project+`"}`); code != http.StatusNotFound {
			t.Errorf("%s without stored workspaces = %d, want 404", route, code)
		}
	}
}

// TestWorkspacePreviewAddAndForget drives the stored list through its
// routes: the preview says what an add would do, or why the path is
// refused, and changes nothing; an add of a folder inside a repository
// stores the repository's root, and the rail shows it with its worktrees at
// once; a second add and a linked worktree's change nothing; forget removes
// the entry and leaves the folder.
func TestWorkspacePreviewAddAndForget(t *testing.T) {
	base, root := worktreeRepo(t)
	f := startPageFixture(t, withWorkspaces(t), func(o *DaemonOptions) { o.Worktrees = gitWorktreeSource{} })
	linked := filepath.Join(base, "repo-linked")
	gitRun(t, root, "worktree", "add", "-q", "-b", "topic/linked", linked)
	sub := mkDir(t, filepath.Join(root, "pkg"))
	file := filepath.Join(base, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview := func(path string) workspacePreview {
		t.Helper()
		code, _, raw := postJSON(t, f.base+workspacePreviewRoute, f.base, pathBody(t, "path", path))
		var out workspacePreview
		if err := json.Unmarshal([]byte(raw), &out); code != http.StatusOK || err != nil {
			t.Fatalf("preview of %q = %d %s (%v)", path, code, raw, err)
		}
		return out
	}
	for _, c := range []struct {
		path    string
		refusal pathRefusal
	}{
		{"", pathEmpty},
		{"pkg/sub", pathRelative},
		{`\\fileserver\share`, pathShare},
		{filepath.Join(base, "nowhere"), pathMissing},
		{file, pathNotDir},
	} {
		if got := preview(c.path); got.OK || got.Refusal != string(c.refusal) || got.Note == "" {
			t.Errorf("preview of %q = %+v, want the refusal %q with a note", c.path, got, c.refusal)
		}
		if code, _, body := postJSON(t, f.base+workspaceAddRoute, f.base, pathBody(t, "path", c.path)); code != http.StatusConflict || strings.TrimSpace(body) == "" {
			t.Errorf("add of %q = %d %q, want 409 with the reason", c.path, code, body)
		}
	}
	subPreview := preview(`"` + sub + `/"`)
	if got := subPreview; !got.OK || got.Path != sub || got.Root != root || !got.Repo || got.Result != string(workspaceAdded) || !strings.Contains(got.Note, root) {
		t.Errorf("preview of a repository subfolder = %+v, want an add of the root %s, named in the note", got, root)
	}
	if got := preview(root); !got.OK || got.Root != root || got.Note != "" {
		t.Errorf("preview of a root = %+v, want a plain add with no note", got)
	}
	if got := f.d.workspaces.workspaces(); len(got) != 0 {
		t.Fatalf("a preview or a refused add stored %v", got)
	}

	// The add lands on an open page's rail without a refresh: the bar, then
	// the worktree list read for it.
	live := openStream(t, f.base+"/s/livesession/events")
	add := func(path string) workspaceAddAnswer {
		t.Helper()
		code, _, raw := postJSON(t, f.base+workspaceAddRoute, f.base, pathBody(t, "path", path))
		var out workspaceAddAnswer
		if err := json.Unmarshal([]byte(raw), &out); code != http.StatusOK || err != nil {
			t.Fatalf("add of %q = %d %s (%v)", path, code, raw, err)
		}
		return out
	}
	if got := add(sub); got.Result != string(workspaceAdded) || got.Root != root || !got.Repo || !strings.Contains(got.Message, root) || got.Message == subPreview.Note {
		t.Errorf("add of a subfolder = %+v, want the root %s added and named, in other words than the preview's %q", got, root, subPreview.Note)
	}
	waitRegionHolding(t, live, regionRail, `data-ws="`+root+`"`)
	waitRegionHolding(t, live, regionRail, "topic/linked")
	if got := add(root); got.Result != string(workspaceKnown) || got.Message == "" {
		t.Errorf("a second add = %+v, want nothing changed", got)
	}
	if got := add(linked); got.Result != string(workspaceKnown) || !strings.Contains(got.Message, linked) || !strings.Contains(got.Message, root) {
		t.Errorf("add of a linked worktree = %+v, want nothing changed, naming the worktree and its root", got)
	}
	if got := preview(linked); !got.OK || got.Result != string(workspaceKnown) || got.Note == "" {
		t.Errorf("preview of a linked worktree = %+v", got)
	}
	want := []storedWorkspace{{Path: root, Repo: true}}
	if got := f.d.workspaces.workspaces(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}

	for _, path := range []string{"", "relative", linked, filepath.Join(base, "nowhere")} {
		if code, _, _ := postJSON(t, f.base+workspaceForgetRoute, f.base, pathBody(t, "path", path)); code != http.StatusNotFound {
			t.Errorf("forget of %q = %d, want 404", path, code)
		}
	}
	if code, _, body := postJSON(t, f.base+workspaceForgetRoute, f.base, pathBody(t, "path", root)); code != http.StatusNoContent {
		t.Fatalf("forget = %d %s, want 204", code, body)
	}
	if got := f.d.workspaces.workspaces(); len(got) != 0 {
		t.Errorf("stored after forget = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); err != nil {
		t.Errorf("forget touched the folder: %v", err)
	}
	waitFor(t, "the rail to drop the forgotten workspace", func() bool {
		_, page := get(t, f.base+"/s/livesession")
		return !strings.Contains(page, `data-ws="`+root+`"`)
	})
}

// TestWorktreeAddRoute drives the add-worktree form's routes against a real
// repository: the options name the branches a worktree can take and the
// default folder; an add on a new and on an existing branch creates the
// worktree where it says, and an open page's rail shows it at once; the
// daemon refuses a request git should not see; and only a repository the
// rail shows can be asked for.
func TestWorktreeAddRoute(t *testing.T) {
	base, root := worktreeRepo(t)
	f := startPageFixture(t, withWorkspaces(t), func(o *DaemonOptions) { o.Worktrees = gitWorktreeSource{} })
	post := func(route string, body any) (int, string) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		code, _, out := postJSONWith(t, &http.Client{Timeout: 30 * time.Second}, f.base+route, f.base, string(raw))
		return code, out
	}

	// A repository the rail does not show is unknown to both routes.
	if code, _ := post(worktreeOptionsRoute, worktreeOptionsRequest{Workspace: root}); code != http.StatusNotFound {
		t.Errorf("options of an unlisted repository = %d, want 404", code)
	}
	if code, _ := post(worktreeAddRoute, worktreeAddRequest{Workspace: root, Branch: "feat/unlisted", New: true}); code != http.StatusNotFound {
		t.Errorf("add in an unlisted repository = %d, want 404", code)
	}
	if code, _ := post(worktreeAddRoute, worktreeAddRequest{Workspace: f.project, Branch: "x", New: true}); code != http.StatusNotFound {
		t.Errorf("add in a plain folder = %d, want 404", code)
	}
	if _, err := f.d.workspaces.add(root, false); err != nil {
		t.Fatal(err)
	}

	code, raw := post(worktreeOptionsRoute, worktreeOptionsRequest{Workspace: root})
	var opts worktreeOptions
	if err := json.Unmarshal([]byte(raw), &opts); code != http.StatusOK || err != nil {
		t.Fatalf("options = %d %s (%v)", code, raw, err)
	}
	pool := filepath.Join(base, worktreeAddDir, "repo")
	if opts.Current != "main" || strings.Join(opts.Bases, " ") != "main free" || strings.Join(opts.Free, " ") != "free" || filepath.Dir(opts.Folder) != pool {
		t.Errorf("options = %+v, want main current and first, free the only free branch, and a folder under %s", opts, pool)
	}

	live := openStream(t, f.base+"/s/livesession/events")
	waitRegionHolding(t, live, regionRail, `data-ws="`+root+`"`)
	add := func(req worktreeAddRequest) worktreeAddAnswer {
		t.Helper()
		req.Workspace = root
		code, raw := post(worktreeAddRoute, req)
		var out worktreeAddAnswer
		if err := json.Unmarshal([]byte(raw), &out); code != http.StatusOK || err != nil {
			t.Fatalf("add %+v = %d %s (%v)", req, code, raw, err)
		}
		return out
	}
	// A new branch starts from the root's current branch, in the default
	// folder the options showed, which the form sends back as is: the add
	// lands there even once the minute it names has passed.
	made := add(worktreeAddRequest{Branch: "feat/new", New: true, Folder: opts.Folder})
	if made.Path != opts.Folder || headBranch(t, made.Path) != "feat/new" || revParse(t, made.Path, "HEAD") != revParse(t, root, "main") || !strings.Contains(made.Message, "feat/new") {
		t.Errorf("new-branch add = %+v, want feat/new at main's commit in %s", made, opts.Folder)
	}
	waitRegionHolding(t, live, regionRail, "feat/new")
	// An existing branch, in a folder the user named: absent, and so made.
	custom := filepath.Join(base, "custom", "deep")
	if made := add(worktreeAddRequest{Branch: "free", Folder: `"` + custom + `/"`}); made.Path != custom || headBranch(t, custom) != "free" {
		t.Errorf("existing-branch add = %+v, want free in %s", made, custom)
	}
	waitRegionHolding(t, live, regionRail, "deep")
	// A new branch from a named base, in an existing empty folder.
	gitRun(t, root, "branch", "base2")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "main moved on")
	empty := mkDir(t, filepath.Join(base, "empty"))
	if made := add(worktreeAddRequest{Branch: "feat/based", New: true, Base: "base2", Folder: empty}); made.Path != empty || revParse(t, empty, "HEAD") != revParse(t, root, "base2") {
		t.Errorf("based add = %+v, want feat/based at base2's commit in %s", made, empty)
	}

	full := mkDir(t, filepath.Join(base, "full"))
	if err := os.WriteFile(filepath.Join(full, "kept"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "branch", "spare")
	for _, c := range []struct {
		name string
		req  worktreeAddRequest
	}{
		{"a folder that is not empty", worktreeAddRequest{Branch: "spare", Folder: full}},
		{"a folder that is a file", worktreeAddRequest{Branch: "spare", Folder: filepath.Join(full, "kept")}},
		{"a relative folder", worktreeAddRequest{Branch: "spare", Folder: "custom/rel"}},
		{"a network-share folder", worktreeAddRequest{Branch: "spare", Folder: `\\fileserver\share\wt`}},
		{"a new branch that exists", worktreeAddRequest{Branch: "free", New: true}},
		{"an existing branch that does not exist", worktreeAddRequest{Branch: "nope"}},
		{"a branch a worktree has checked out", worktreeAddRequest{Branch: "main"}},
		{"a branch another worktree has checked out", worktreeAddRequest{Branch: "feat/new"}},
		{"a base that does not exist", worktreeAddRequest{Branch: "feat/nobase", New: true, Base: "nope"}},
		{"an option as the base", worktreeAddRequest{Branch: "feat/optbase", New: true, Base: "--detach"}},
		{"no branch", worktreeAddRequest{New: true}},
		{"an option as the branch", worktreeAddRequest{Branch: "--detach", New: true}},
		{"an option as an existing branch", worktreeAddRequest{Branch: "--force"}},
		{"an invalid branch name", worktreeAddRequest{Branch: "a..b", New: true}},
		{"a branch name with a space", worktreeAddRequest{Branch: "a b", New: true}},
	} {
		c.req.Workspace = root
		if code, body := post(worktreeAddRoute, c.req); code != http.StatusConflict || strings.TrimSpace(body) == "" {
			t.Errorf("%s: add = %d %q, want 409 with the reason", c.name, code, body)
		}
	}
	if _, err := os.Stat(filepath.Join(full, "kept")); err != nil {
		t.Errorf("a refused add touched the folder: %v", err)
	}
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "worktree "); n != 4 {
		t.Errorf("the repository has %d worktrees, want the root and the three added:\n%s", n, out)
	}

	// One git action of a root at a time.
	release, ok := f.d.beginGitAction(root)
	if !ok {
		t.Fatal("the root's action slot is held")
	}
	if code, _ := post(worktreeAddRoute, worktreeAddRequest{Workspace: root, Branch: "spare"}); code != http.StatusConflict {
		t.Errorf("add while another action runs = %d, want 409", code)
	}
	release()
}

// TestAddWorktreeDefaultFolder: the default folder is
// <root>/../.ws-dashboard-worktrees/<root-dir-name>/<YYMMDD-HHMM>, and adds
// in the same minute take -2, -3.
func TestAddWorktreeDefaultFolder(t *testing.T) {
	now := time.Date(2026, 11, 10, 20, 47, 30, 0, time.Local)
	taken := map[string]bool{}
	exists := func(p string) bool { return taken[p] }
	want := filepath.Join(string(filepath.Separator)+"w", worktreeAddDir, "repo", "261110-2047")
	rootPath := filepath.Join(string(filepath.Separator)+"w", "repo") + string(filepath.Separator)
	if got := defaultWorktreeFolder(rootPath, now, exists); got != want {
		t.Errorf("default folder = %s, want %s", got, want)
	}
	taken[want] = true
	if got := defaultWorktreeFolder(rootPath, now, exists); got != want+"-2" {
		t.Errorf("default folder with the name taken = %s, want %s-2", got, want)
	}

	base, root := worktreeRepo(t)
	ctx := context.Background()
	var got []string
	for _, branch := range []string{"one", "two", "three"} {
		folder, refusal, err := addWorktree(ctx, root, worktreeRequest{Branch: branch, New: true}, now)
		if err != nil || refusal != "" {
			t.Fatalf("add %s = %q, %v", branch, refusal, err)
		}
		if headBranch(t, folder) != branch {
			t.Errorf("%s holds %s, want %s", folder, headBranch(t, folder), branch)
		}
		got = append(got, folder)
	}
	first := filepath.Join(base, worktreeAddDir, "repo", "261110-2047")
	if strings.Join(got, " ") != strings.Join([]string{first, first + "-2", first + "-3"}, " ") {
		t.Errorf("folders = %v, want %s and its -2 and -3", got, first)
	}
}

// TestSessionPageOffersTheRailAddButtons pins the session page's rail on a
// fixture daemon: the add-workspace + and a stored folder's forget once the
// daemon keeps stored workspaces, neither without.
func TestSessionPageOffersTheRailAddButtons(t *testing.T) {
	f := startPageFixture(t, withWorkspaces(t))
	folder := t.TempDir()
	if _, err := f.d.workspaces.add(folder, false); err != nil {
		t.Fatal(err)
	}
	f.d.broker.wake(railListRepoKey)
	var page string
	waitFor(t, "the stored folder on the rail", func() bool {
		_, page = get(t, f.base+"/s/livesession")
		return strings.Contains(page, `data-ws="`+folder+`"`)
	})
	if !strings.Contains(page, `class="radd" data-add="ws"`) {
		t.Error("the rail lacks the add-workspace +")
	}
	if section := railSection(t, page, folder); !strings.Contains(section, `class="rforget"`) || strings.Contains(section, `data-add="wt"`) {
		t.Errorf("a stored plain folder's bar = %s, want forget and no +", section)
	}

	off := startPageFixture(t)
	if _, page := get(t, off.base+"/s/livesession"); !strings.Contains(page, `id="rail"`) || strings.Contains(page, `class="radd"`) || strings.Contains(page, `class="rforget"`) {
		t.Error("a daemon without stored workspaces offers a + or forget on the rail")
	}
}
