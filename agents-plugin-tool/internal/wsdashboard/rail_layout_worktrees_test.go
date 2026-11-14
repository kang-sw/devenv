package wsdashboard

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func worktreePath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func worktreeNativePaths(paths ...string) []string {
	for i := range paths {
		paths[i] = worktreePath(paths[i])
	}
	return paths
}

// Keep layoutTestView's historical identities intact for its existing callers.
func worktreeTestView() RailView {
	v := layoutTestView()
	v.Workspaces[1].Worktrees = append(v.Workspaces[1].Worktrees, RailWorktree{Path: "/w/empty"})
	for i := range v.Workspaces {
		w := &v.Workspaces[i]
		w.Path = worktreePath(w.Path)
		for j := range w.Worktrees {
			w.Worktrees[j].Path = worktreePath(w.Worktrees[j].Path)
		}
	}
	return v
}

func worktreePaths(v RailView) []string {
	for _, w := range v.Workspaces {
		if w.Path == worktreePath("/w/repo") {
			var paths []string
			for _, wt := range w.Worktrees {
				paths = append(paths, wt.Path)
			}
			return paths
		}
	}
	return nil
}

func TestRailLayoutWorktreeMove(t *testing.T) {
	v := worktreeTestView()
	f := railFactsOf(v)
	d := emptyLayout()
	apply := func(path, before, after string) {
		t.Helper()
		if reason := applyRailLayoutOp(d, railLayoutRequest{Op: "wtmove", Workspace: worktreePath("/w/repo"), Path: worktreePath(path), Before: worktreePath(before), After: worktreePath(after)}, f, true); reason != "" {
			t.Fatal(reason)
		}
	}
	apply("/w/empty", "/w/feat", "")
	if got := d.Workspaces[worktreePath("/w/repo")].WorktreeOrder; !slices.Equal(got, worktreeNativePaths("/w/repo", "/w/empty", "/w/feat")) {
		t.Fatalf("first move lost fallback relationships: %v", got)
	}
	apply("/w/repo", "", "/w/feat")
	apply("/w/feat", "/w/empty", "")
	orderRailView(&v, d, f)
	if got := worktreePaths(v); !slices.Equal(got, worktreeNativePaths("/w/feat", "/w/empty", "/w/repo")) {
		t.Fatal(got)
	}
	if len(v.Workspaces[1].Worktrees[2].Sessions) != 4 || !v.Workspaces[1].Worktrees[2].Root || v.Workspaces[0].Path != worktreePath("/w/plain") {
		t.Fatal("moving groups changed their contents or workspace order")
	}
}

func TestRailLayoutWorktreeRetention(t *testing.T) {
	v := worktreeTestView()
	f := railFactsOf(v)
	d := emptyLayout()
	// X is absent; a newly discovered empty group is unlisted. A saved
	// root may be anywhere, and discovery never mutates the saved list.
	w := &railLayoutWS{WorktreeOrder: worktreeNativePaths("/w/repo", "/w/X", "/w/feat")}
	d.Workspaces[worktreePath("/w/repo")] = w
	before, _ := d.encode()
	orderRailView(&v, d, f)
	if got := worktreePaths(v); !slices.Equal(got, worktreeNativePaths("/w/repo", "/w/feat", "/w/empty")) {
		t.Fatal(got)
	}
	if after, _ := d.encode(); !bytes.Equal(before, after) {
		t.Fatal("render mutated saved state")
	}
	// A visible no-op must not move the absent slot between its peers.
	if reason := applyRailLayoutOp(d, railLayoutRequest{Op: "wtmove", Workspace: worktreePath("/w/repo"), Path: worktreePath("/w/repo"), Before: worktreePath("/w/feat")}, f, true); reason != "" {
		t.Fatal(reason)
	}
	if after, _ := d.encode(); !bytes.Equal(before, after) {
		t.Fatal("visible no-op changed retained slots")
	}
	if reason := applyRailLayoutOp(d, railLayoutRequest{Op: "wtmove", Workspace: worktreePath("/w/repo"), Path: worktreePath("/w/empty"), Before: worktreePath("/w/feat")}, f, true); reason != "" {
		t.Fatal(reason)
	}
	want := worktreeNativePaths("/w/repo", "/w/X", "/w/empty", "/w/feat")
	if !slices.Equal(w.WorktreeOrder, want) {
		t.Fatalf("full-sequence splice = %v, want %v", w.WorktreeOrder, want)
	}
	// Reappearing X occupies its retained slot, not a new tail slot.
	v = worktreeTestView()
	v.Workspaces[1].Worktrees = append(v.Workspaces[1].Worktrees, RailWorktree{Path: worktreePath("/w/X")})
	orderRailView(&v, d, railFactsOf(v))
	if !slices.Equal(worktreePaths(v), want) {
		t.Fatal(worktreePaths(v))
	}
}

func TestRailLayoutWorktreeRefusals(t *testing.T) {
	f := railFactsOf(worktreeTestView())
	for _, c := range []struct {
		name, workspace, path, before, after, reason string
	}{
		{"missing workspace", "", "/w/repo", "/w/feat", "", layoutInvalid},
		{"missing path", "/w/repo", "", "/w/feat", "", layoutInvalid},
		{"missing reference", "/w/repo", "/w/repo", "", "", layoutInvalid},
		{"both references", "/w/repo", "/w/repo", "/w/feat", "/w/empty", layoutInvalid},
		{"unclean", "/w/repo", "/w/repo/../repo", "/w/feat", "", layoutInvalid},
		{"nul", "/w/repo", "/w/\x00", "/w/feat", "", layoutInvalid},
		{"unknown workspace", "/w/missing", "/w/repo", "/w/feat", "", layoutUnknown},
		{"plain folder", "/w/plain", "/w/plain", "/w/feat", "", layoutUnknown},
		{"unknown source", "/w/repo", "/w/missing", "/w/feat", "", layoutUnknown},
		{"unknown reference", "/w/repo", "/w/repo", "/w/missing", "", layoutUnknown},
		{"other workspace source", "/w/repo", "/w/plain", "/w/feat", "", layoutUnknown},
		{"other workspace reference", "/w/repo", "/w/repo", "/w/zed", "", layoutUnknown},
		{"self", "/w/repo", "/w/repo", "/w/repo", "", layoutSelf},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := openRailLayoutStore(t.TempDir())
			d, gen := s.current()
			before, _ := d.encode()
			path := worktreePath(c.path)
			if c.name == "unclean" {
				// Convert separators without cleaning away the invalid component.
				path = filepath.FromSlash(c.path)
			}
			a, err := s.apply(railLayoutRequest{Op: "wtmove", Workspace: worktreePath(c.workspace), Path: path, Before: worktreePath(c.before), After: worktreePath(c.after)}, f, true)
			if err != nil || a.Reason != c.reason || a.Changed || a.Gen != gen {
				t.Fatalf("answer %+v, %v, want %s without mutation", a, err, c.reason)
			}
			d, _ = s.current()
			if after, _ := d.encode(); !bytes.Equal(before, after) {
				t.Fatal("refusal mutated store")
			}
		})
	}
}

func TestRailLayoutWorktreeStoreCompatibility(t *testing.T) {
	dir := t.TempDir()
	fixture, err := json.Marshal(map[string]any{
		"schema_version": 1, "future": map[string]bool{"ok": true},
		"workspaces": map[string]any{worktreePath("/w/repo"): map[string]any{
			"worktree_order": []string{worktreePath("/w/repo"), worktreePath("/w/gone"), worktreePath("/w/feat"), worktreePath("/w/feat"), filepath.FromSlash("/w/a/../empty")}, "future_ws": 7,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeLayoutFile(t, dir, string(fixture))
	s := openRailLayoutStore(dir)
	d, _ := s.current()
	want := worktreeNativePaths("/w/repo", "/w/gone", "/w/feat", "/w/empty")
	if !slices.Equal(d.Workspaces[worktreePath("/w/repo")].WorktreeOrder, want) || railLayoutEmpty(d) {
		t.Fatal("group-only state normalized incorrectly or treated as empty")
	}
	f := railFactsOf(worktreeTestView())
	ops := []railLayoutRequest{
		{Op: "move", RID: "a2", Before: "a1"},
		{Op: "attach", RID: "a2", Parent: "a1"},
		{Op: "detach", RID: "a2"},
		{Op: "star", RID: "a1"},
		{Op: "unstar", RID: "a1"},
		{Op: "wsmove", Path: worktreePath("/w/repo"), Before: worktreePath("/w/plain")},
		{Op: "import", Order: []string{"a1"}},
	}
	for _, op := range ops {
		a, err := s.apply(op, f, true)
		if err != nil || (a.Reason != "" && a.Reason != layoutNotApplied) {
			t.Fatalf("%s: %+v %v", op.Op, a, err)
		}
		d, _ = openRailLayoutStore(dir).current()
		if !slices.Equal(d.Workspaces[worktreePath("/w/repo")].WorktreeOrder, want) {
			t.Fatalf("%s lost group order: %+v", op.Op, d)
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(readLayoutFile(t, dir), &raw); err != nil || raw["future"] == nil {
		t.Fatal("unknown root field lost", err)
	}
	if !bytes.Contains(raw["workspaces"], []byte(`"future_ws": 7`)) {
		t.Fatal("unknown workspace field lost")
	}
	// Session pruning and even temporary workspace disappearance must not
	// delete group-only state or permit a legacy import over it.
	d = emptyLayout()
	d.Workspaces[worktreePath("/w/repo")] = &railLayoutWS{WorktreeOrder: want}
	pruneRailLayout(d, railFacts{})
	if d.Workspaces[worktreePath("/w/repo")] == nil || railLayoutEmpty(d) || layoutImport(d, f, railLayoutRequest{}) != layoutNotApplied {
		t.Fatal("pruning lost group-only workspace/import gate")
	}
	// An old schema-1 fixture without this optional field remains valid.
	oldDir := t.TempDir()
	fixture, err = json.Marshal(map[string]any{"schema_version": 1, "workspace_order": []string{}, "workspaces": map[string]any{
		worktreePath("/w/repo"): map[string]any{"order": []string{"a2", "a1"}, "children": map[string][]string{}, "stars": []string{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	writeLayoutFile(t, oldDir, string(fixture))
	old, err := readRailLayout(filepath.Join(oldDir, railLayoutFileName))
	if err != nil || old.Workspaces[worktreePath("/w/repo")].WorktreeOrder != nil {
		t.Fatal(old, err)
	}
	v := worktreeTestView()
	orderRailView(&v, old, railFactsOf(v))
	if got := railIDs(v.Workspaces[1].Worktrees[0].Sessions); !slices.Equal(got, []string{"a2", "a1", "a3", "a4"}) {
		t.Fatalf("old fixture renders %v, want saved peers then unlisted ordinary rows", got)
	}
}
