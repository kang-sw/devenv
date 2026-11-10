package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// mkRepoRoot makes dir a repository's root worktree as the daemon detects
// one: a folder holding a .git directory.
func mkRepoRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// mkLinkedWorktree makes dir a linked worktree of root as git lays one out:
// a .git file naming <root>/.git/worktrees/<name>.
func mkLinkedWorktree(t *testing.T, root, dir string) string {
	t.Helper()
	gitdir := filepath.Join(root, ".git", "worktrees", filepath.Base(dir))
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mkDir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func storedPaths(s *workspaceStore) []string {
	var out []string
	for _, w := range s.workspaces() {
		out = append(out, w.Path)
	}
	return out
}

// TestLocateWorkspace pins where a folder's workspace is: the repository's
// root worktree for a root, a subfolder of one, and a linked worktree or a
// folder inside one; the folder itself when no repository holds it, or
// when its .git is a submodule's.
func TestLocateWorkspace(t *testing.T) {
	base := t.TempDir()
	root := mkRepoRoot(t, filepath.Join(base, "repo"))
	sub := mkDir(t, filepath.Join(root, "pkg", "deep"))
	linked := mkLinkedWorktree(t, root, filepath.Join(base, "repo-wt"))
	linkedSub := mkDir(t, filepath.Join(linked, "pkg"))
	plain := mkDir(t, filepath.Join(base, "plain", "notes"))
	module := mkDir(t, filepath.Join(root, "vendor", "mod"))
	if err := os.WriteFile(filepath.Join(module, ".git"), []byte("gitdir: ../../.git/modules/mod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moduleSub := mkDir(t, filepath.Join(module, "src"))

	for _, c := range []struct {
		name, dir string
		want      workspaceLocation
	}{
		{"a root worktree", root, workspaceLocation{Root: root, Repo: true}},
		{"a subfolder of a repository", sub, workspaceLocation{Root: root, Repo: true}},
		{"a linked worktree", linked, workspaceLocation{Root: root, Repo: true, Worktree: linked}},
		{"a subfolder of a linked worktree", linkedSub, workspaceLocation{Root: root, Repo: true, Worktree: linked}},
		{"a plain folder", plain, workspaceLocation{Root: plain}},
		{"a submodule", module, workspaceLocation{Root: module}},
		{"a subfolder of a submodule", moduleSub, workspaceLocation{Root: moduleSub}},
	} {
		if got := locateWorkspace(c.dir + string(filepath.Separator)); got != c.want {
			t.Errorf("%s: locateWorkspace = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestWorkspaceStoreAddResults pins an add's three results and that a
// preview predicts each without changing the list: a new root is added
// (a subfolder adds its repository's root); a linked worktree of a stored
// root, and a stored root itself, change nothing; a stored folder that is a
// linked worktree merges under its root.
func TestWorkspaceStoreAddResults(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "config", "dashboard", "workspaces.json")
	root := mkRepoRoot(t, filepath.Join(base, "repo"))
	sub := mkDir(t, filepath.Join(root, "pkg"))
	linked := mkLinkedWorktree(t, root, filepath.Join(base, "repo-wt"))
	plain := mkDir(t, filepath.Join(base, "plain"))
	s := openWorkspaceStore(file)

	add := func(dir string, want workspaceAdd) {
		t.Helper()
		before := storedPaths(s)
		preview, err := s.add(dir, true)
		if err != nil || preview != want {
			t.Errorf("preview of %s = %+v, %v; want %+v", dir, preview, err, want)
		}
		if got := storedPaths(s); !reflect.DeepEqual(got, before) {
			t.Errorf("preview of %s changed the list: %v -> %v", dir, before, got)
		}
		if _, err := os.Stat(file); len(before) == 0 && err == nil {
			t.Errorf("preview of %s wrote the file", dir)
		}
		got, err := s.add(dir, false)
		if err != nil || got != want {
			t.Errorf("add of %s = %+v, %v; want %+v", dir, got, err, want)
		}
	}
	add(sub, workspaceAdd{Result: workspaceAdded, Path: sub, Root: root, Repo: true})
	add(root, workspaceAdd{Result: workspaceKnown, Path: root, Root: root, Repo: true})
	add(linked, workspaceAdd{Result: workspaceKnown, Path: linked, Root: root, Repo: true, Worktree: linked})
	add(plain, workspaceAdd{Result: workspaceAdded, Path: plain, Root: plain})
	add(plain, workspaceAdd{Result: workspaceKnown, Path: plain, Root: plain})
	if got, want := storedPaths(s), []string{root, plain}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored = %v, want %v", got, want)
	}

	// A stored folder that became a linked worktree of an unstored root
	// takes that root's place; one whose root is stored only leaves.
	other := mkRepoRoot(t, filepath.Join(base, "other"))
	late := mkDir(t, filepath.Join(base, "late"))
	add(late, workspaceAdd{Result: workspaceAdded, Path: late, Root: late})
	mkLinkedWorktree(t, other, late)
	add(late, workspaceAdd{Result: workspaceMerged, Path: late, Root: other, Repo: true, Worktree: late})
	if got, want := storedPaths(s), []string{root, plain, other}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored after the merge = %v, want %v", got, want)
	}
	mkLinkedWorktree(t, root, plain)
	add(filepath.Join(plain), workspaceAdd{Result: workspaceMerged, Path: plain, Root: root, Repo: true, Worktree: plain})
	if got, want := storedPaths(s), []string{root, other}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored after the second merge = %v, want %v", got, want)
	}
}

// TestWorkspaceStoreRoundTripAndForget: the list survives a reopen in the
// order it was added, in an owner-only file under the config home; forget
// removes one entry and leaves its folder alone; a malformed file refuses
// a change rather than being written over.
func TestWorkspaceStoreRoundTripAndForget(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "config", "dashboard", "workspaces.json")
	root := mkRepoRoot(t, filepath.Join(base, "repo"))
	plain := mkDir(t, filepath.Join(base, "plain"))
	keep := filepath.Join(plain, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if openWorkspaceStore("") != nil {
		t.Fatal("a store opened without a file")
	}
	s := openWorkspaceStore(file)
	if got := s.workspaces(); len(got) != 0 {
		t.Fatalf("a new store lists %v", got)
	}
	for _, dir := range []string{plain, root} {
		if _, err := s.add(dir, false); err != nil {
			t.Fatal(err)
		}
	}
	want := []storedWorkspace{{Path: plain}, {Path: root, Repo: true}}
	if got := openWorkspaceStore(file).workspaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reopened = %+v, want %+v", got, want)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var doc workspacesDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != workspacesSchemaVersion || len(doc.Workspaces) != 2 || doc.Workspaces[0].Path != plain {
		t.Fatalf("file = %s (%v)", raw, err)
	}
	if info, err := os.Stat(file); err != nil || (info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("file mode = %v, %v; want owner-only", info.Mode(), err)
	}

	if ok, err := s.forget(filepath.Join(base, "never-added")); ok || err != nil {
		t.Errorf("forget of an unknown path = %v, %v", ok, err)
	}
	if ok, err := s.forget(plain + string(filepath.Separator)); !ok || err != nil {
		t.Fatalf("forget = %v, %v", ok, err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("forget touched the folder: %v", err)
	}
	if got := openWorkspaceStore(file).workspaces(); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("after forget = %+v, want %+v", got, want[1:])
	}

	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.add(plain, false); err == nil {
		t.Error("an add over a malformed file succeeded")
	}
	if raw, _ := os.ReadFile(file); string(raw) != "{not json" {
		t.Errorf("a malformed file was written over: %s", raw)
	}
}

// TestWorkspaceStoreSweep: the reread cadence's look rewrites a stored
// workspace that became a linked worktree to its root, follows a folder
// that became a repository, picks up a list another process wrote, and
// reports a change only when the rail would show one.
func TestWorkspaceStoreSweep(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "workspaces.json")
	root := mkRepoRoot(t, filepath.Join(base, "repo"))
	folder := mkDir(t, filepath.Join(base, "folder"))
	later := mkDir(t, filepath.Join(base, "later"))
	s := openWorkspaceStore(file)
	for _, dir := range []string{folder, later} {
		if _, err := s.add(dir, false); err != nil {
			t.Fatal(err)
		}
	}
	if s.sweep() {
		t.Error("a sweep with nothing new reported a change")
	}
	mkLinkedWorktree(t, root, folder)
	mkRepoRoot(t, later)
	if !s.sweep() {
		t.Fatal("a sweep after the folders changed reported none")
	}
	want := []storedWorkspace{{Path: root, Repo: true}, {Path: later, Repo: true}}
	if got := s.workspaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("after the sweep = %+v, want %+v", got, want)
	}
	if got := openWorkspaceStore(file).workspaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the sweep's rewrite was not saved: %+v", got)
	}

	elsewhere := openWorkspaceStore(file)
	if ok, err := elsewhere.forget(later); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if !s.sweep() || len(s.workspaces()) != 1 {
		t.Errorf("a sweep missed another writer's change: %+v", s.workspaces())
	}
	var none *workspaceStore
	if none.sweep() || none.workspaces() != nil {
		t.Error("a nil store swept or listed something")
	}
}
