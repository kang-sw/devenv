package wsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeRepoConfig writes a committed repo-scope config file
// (<root>/.ws-workflow/config.json) carrying the given overrides, mirroring the
// hand-edited/checked-in file the tools read at resolution time.
func writeRepoConfig(t *testing.T, root string, overrides map[string]string) {
	t.Helper()
	dir := filepath.Join(root, ".ws-workflow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir repo config dir: %v", err)
	}
	raw, err := json.MarshalIndent(Config{SchemaVersion: schemaVersion, Overrides: overrides}, "", "  ")
	if err != nil {
		t.Fatalf("marshal repo config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write repo config: %v", err)
	}
}

// newRepoTestResolver builds a Resolver whose project/global scopes are isolated
// temp dirs and whose repo scope is anchored at a temp worktree root, returning
// both the resolver and the opts so a test can seed each scope independently.
func newRepoTestResolver(t *testing.T, sess *fakeSessionStore) (Resolver, Options) {
	t.Helper()
	opts := Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		RepoRoot:   t.TempDir(),
	}
	var sr SessionReader
	var sw SessionWriter
	if sess != nil {
		sr = sess
		sw = sess
	}
	return NewResolver(opts, nil, sr, sw), opts
}

// TestRepoScopeBelowProject verifies the committed repo value loses to a
// machine-local project override for the same key (session > project > repo).
func TestRepoScopeBelowProject(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	const key = "ticket-assignee-aware"

	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "repo-value"})
	if err := r.Set(key, "project-value", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatalf("set project: %v", err)
	}

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "project-value" || rv.Scope != ScopeProject {
		t.Fatalf("expected project-value/project, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopeAboveGlobal verifies the committed repo value wins over a global
// value for the same key (repo > global), so a committed project baseline
// overrides a cross-project user default.
func TestRepoScopeAboveGlobal(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	const key = "ticket-assignee-aware"

	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "repo-value"})
	if err := r.Set(key, "global-value", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set global: %v", err)
	}

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "repo-value" || rv.Scope != ScopeRepo {
		t.Fatalf("expected repo-value/repo, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopeResolvesWhenOnlyScope verifies a key set only in the committed
// repo file resolves to the repo scope (repo > builtin when nothing else holds).
func TestRepoScopeResolvesWhenOnlyScope(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	const key = "ticket-assignee-aware"

	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "on"})

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "on" || rv.Scope != ScopeRepo {
		t.Fatalf("expected on/repo, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopeMissingFileIsNoOverride verifies that a RepoRoot whose
// .ws-workflow/config.json does not exist contributes nothing: the key falls
// through to the builtin floor, and resolution never errors on the absent file.
func TestRepoScopeMissingFileIsNoOverride(t *testing.T) {
	// RepoRoot is a real temp dir but no .ws-workflow/config.json is written.
	r := NewResolver(Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		RepoRoot:   t.TempDir(),
	}, map[string]string{"some.key": "builtin-floor"}, nil, nil)

	rv, err := r.Get("", "some.key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "builtin-floor" || rv.Scope != ScopeBuiltin {
		t.Fatalf("expected builtin-floor/builtin, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopeEmptyRepoRootDropsOut verifies that an empty RepoRoot (no anchor,
// e.g. a keyless config.list caller) makes the repo scope absent entirely: a key
// present only in the global scope still resolves, and no repo file is read.
func TestRepoScopeEmptyRepoRootDropsOut(t *testing.T) {
	r := NewResolver(Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		// RepoRoot intentionally empty.
	}, nil, nil, nil)
	const key = "ticket-assignee-aware"

	if err := r.Set(key, "global-value", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set global: %v", err)
	}

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "global-value" || rv.Scope != ScopeGlobal {
		t.Fatalf("expected global-value/global, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopePerWorktreeIsolation verifies repo-root discovery under nested
// worktrees: each worktree checks out its own committed .ws-workflow/config.json,
// so a resolver anchored at worktree A reads A's value even when a sibling
// worktree B (created under a nested path) holds a different committed value.
// The resolver reads only the file under its own RepoRoot — no walk-up that
// could leak a parent/sibling worktree's value.
func TestRepoScopePerWorktreeIsolation(t *testing.T) {
	const key = "ticket-assignee-aware"

	base := t.TempDir()
	worktreeA := filepath.Join(base, "wt-a")
	// worktreeB is nested one level below worktreeA on disk, standing in for a
	// linked worktree materialized inside another checkout's directory tree.
	worktreeB := filepath.Join(worktreeA, "nested", "wt-b")
	if err := os.MkdirAll(worktreeB, 0o755); err != nil {
		t.Fatalf("mkdir nested worktree: %v", err)
	}
	writeRepoConfig(t, worktreeA, map[string]string{key: "value-A"})
	writeRepoConfig(t, worktreeB, map[string]string{key: "value-B"})

	for _, tc := range []struct {
		name string
		root string
		want string
	}{
		{name: "worktree A", root: worktreeA, want: "value-A"},
		{name: "nested worktree B", root: worktreeB, want: "value-B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(Options{
				CacheHome:  t.TempDir(),
				ConfigHome: t.TempDir(),
				RepoRoot:   tc.root,
			}, nil, nil, nil)
			rv, err := r.Get("", key)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if rv.Value != tc.want || rv.Scope != ScopeRepo {
				t.Fatalf("expected %s/repo, got %q/%q", tc.want, rv.Value, rv.Scope)
			}
		})
	}
}

// TestScopedShowSurfacesRepoScope verifies ScopedShow enumerates a key held only
// in the committed repo file and reports its resolved value tagged with the repo
// scope — the surface config.list projects to callers.
func TestScopedShowSurfacesRepoScope(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	const key = "ticket-assignee-aware"
	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "on"})

	view, err := ScopedShow(&r, opts, "")
	if err != nil {
		t.Fatalf("scoped show: %v", err)
	}
	var found *ScopedItem
	for i := range view.ResolvedOverrides {
		if view.ResolvedOverrides[i].Key == key {
			found = &view.ResolvedOverrides[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("repo-scope key %q absent from resolved overrides: %+v", key, view.ResolvedOverrides)
	}
	if found.Value != "on" || found.Scope != ScopeRepo {
		t.Fatalf("expected on/repo, got %q/%q", found.Value, found.Scope)
	}
}

// TestGlobalOnlyItemBypassesRepoScope verifies a global-only item ignores the
// committed repo overlay: even with a repo value present, resolution walks only
// global → builtin.
func TestGlobalOnlyItemBypassesRepoScope(t *testing.T) {
	const key = "global.only.repo.probe"
	RegisterGlobalOnly(key)
	t.Cleanup(func() {
		delete(scopeRegistry, key)
		delete(globalOnlyRegistry, key)
	})

	r, opts := newRepoTestResolver(t, nil)
	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "repo-value"})

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Scope == ScopeRepo {
		t.Fatalf("global-only item resolved from repo scope; want it to bypass repo (got %q/%q)", rv.Value, rv.Scope)
	}
	if rv.Value == "repo-value" {
		t.Fatalf("global-only item picked up repo value %q; repo overlay must not apply", rv.Value)
	}
}

// TestRepoPath verifies the pure path join and the empty-root sentinel.
func TestRepoPath(t *testing.T) {
	if _, ok := RepoPath(Options{}); ok {
		t.Fatalf("empty RepoRoot should report ok=false")
	}
	got, ok := RepoPath(Options{RepoRoot: "/repo"})
	if !ok {
		t.Fatalf("non-empty RepoRoot should report ok=true")
	}
	want := filepath.Join("/repo", ".ws-workflow", "config.json")
	if got != want {
		t.Fatalf("RepoPath = %q, want %q", got, want)
	}
}
