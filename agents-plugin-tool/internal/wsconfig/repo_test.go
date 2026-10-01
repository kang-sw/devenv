package wsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestSessionScopeAboveRepo verifies a session override wins over the committed
// repo value for the same key (session > repo), completing the precedence
// ordering the ticket phase named (repo vs each other scope).
func TestSessionScopeAboveRepo(t *testing.T) {
	sess := newFakeSessionStore()
	r, opts := newRepoTestResolver(t, sess)
	const key = "ticket-assignee-aware"
	const sessionKey = "test-session-key"

	writeRepoConfig(t, opts.RepoRoot, map[string]string{key: "repo-value"})
	if err := r.Set(key, "session-value", SetOptions{ExplicitScope: ScopeSession, SessionKey: sessionKey}); err != nil {
		t.Fatalf("set session: %v", err)
	}

	rv, err := r.Get(sessionKey, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "session-value" || rv.Scope != ScopeSession {
		t.Fatalf("expected session-value/session, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestRepoScopeMalformedFileErrors verifies loadRepoConfig surfaces a parse
// error (rather than silently treating a corrupt committed file as empty), so a
// broken .ws-workflow/config.json fails loud instead of dropping every repo
// override. This exercises repo.go's parse-error failure path.
func TestRepoScopeMalformedFileErrors(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".ws-workflow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := NewResolver(Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		RepoRoot:   root,
	}, nil, nil, nil)

	if _, err := r.Get("", "any.key"); err == nil {
		t.Fatalf("expected a parse error for malformed repo config, got nil")
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

// TestRepoScopeFunction is a 261001-feat-config-repo-scope-and-tune-weight-
// guidance Phase 1 unit test for RepoScope, the function config.list reads to
// publish the repo-scope file path/existence/shape (wsconfig.View.RepoScope).
// It covers the three observable states: no anchor (empty Options), an
// anchor with no committed file yet, and an anchor with the file present.
func TestRepoScopeFunction(t *testing.T) {
	// No RepoRoot: nothing anchors the repo scope, so Path stays empty, but the
	// shape is still reported (a lead can draft a file without a session root).
	empty := RepoScope(Options{})
	if empty.Path != "" || empty.Exists {
		t.Fatalf("RepoScope(empty opts) = %+v, want Path=\"\" Exists=false", empty)
	}
	if empty.Shape != RepoOverridesShape {
		t.Fatalf("RepoScope(empty opts).Shape = %q, want %q", empty.Shape, RepoOverridesShape)
	}

	// RepoRoot anchored, no committed file written yet: the path resolves, but
	// Exists is false.
	root := t.TempDir()
	wantPath := filepath.Join(root, ".ws-workflow", "config.json")
	absent := RepoScope(Options{RepoRoot: root})
	if absent.Path != wantPath {
		t.Fatalf("RepoScope(anchored, absent file).Path = %q, want %q", absent.Path, wantPath)
	}
	if absent.Exists {
		t.Fatalf("RepoScope reported Exists=true before any file was written")
	}

	// Committed file now present: Exists flips true; the path is unchanged.
	writeRepoConfig(t, root, map[string]string{"ticket-assignee-aware": "on"})
	present := RepoScope(Options{RepoRoot: root})
	if present.Path != wantPath {
		t.Fatalf("RepoScope(anchored, present file).Path = %q, want %q", present.Path, wantPath)
	}
	if !present.Exists {
		t.Fatalf("RepoScope reported Exists=false for a file that was just written")
	}
}

// TestRepoScopedReflectsGlobalOnlyExclusion is a Phase 1 unit test for
// RepoScoped, the per-key predicate config.list's scoped view and tuning
// catalog both call to flag whether the committed repo scope can supply a
// key: every resolver-backed key except a global-only one (Resolver.Get skips
// the repo overlay entirely for GlobalOnly items).
func TestRepoScopedReflectsGlobalOnlyExclusion(t *testing.T) {
	if !RepoScoped(ItemTicketAssigneeAware) {
		t.Fatalf("an ordinary resolver-backed key must be repo-scoped: %q", ItemTicketAssigneeAware)
	}
	if !RepoScoped(ItemWorktreePool) {
		t.Fatalf("worktree_pool must be repo-scoped: %q", ItemWorktreePool)
	}
	if RepoScoped(ItemWorkflowPreferSubagent) {
		t.Fatalf("a global-only key must not be repo-scoped: %q", ItemWorkflowPreferSubagent)
	}
	if RepoScoped(ItemBootstrapAlarm) {
		t.Fatalf("a global-only key must not be repo-scoped: %q", ItemBootstrapAlarm)
	}
}

// TestResolverCheckSurfacesMalformedRepoFile is a Phase 1 unit test for
// Resolver.Check, the probe a caller whose reads cannot carry an error (a
// lookup closure, see buildOverrideLookup) runs once up front so a malformed
// committed repo file fails loud instead of silently resolving every key to
// "". The error must name the file path, matching loadRepoConfig's own
// path-naming (repo.go).
func TestResolverCheckSurfacesMalformedRepoFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".ws-workflow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wantPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(wantPath, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := NewResolver(Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		RepoRoot:   root,
	}, nil, nil, nil)

	err := r.Check()
	if err == nil {
		t.Fatalf("Check() did not surface the malformed committed repo file")
	}
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("Check() error does not name the file path %q: %v", wantPath, err)
	}
}

// TestResolverCheckPassesForWellFormedScopes verifies Check's complement: with
// every scope well-formed (including an absent repo file and a present one),
// it returns nil rather than a false positive that would block every reader
// that calls it up front.
func TestResolverCheckPassesForWellFormedScopes(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	if err := r.Check(); err != nil {
		t.Fatalf("Check() with no committed file returned an error: %v", err)
	}
	writeRepoConfig(t, opts.RepoRoot, map[string]string{"ticket-assignee-aware": "on"})
	if err := r.Check(); err != nil {
		t.Fatalf("Check() with a well-formed committed file returned an error: %v", err)
	}
}

// TestLoadRepoConfigErrorNamesPath is a Phase 1 unit test pinning the
// repo.go change that gave loadRepoConfig's parse-error wrap the file path
// (previously "parse repo ws config: %w", now "parse repo ws config %s: %w"),
// so a lead or a test reading the error text can act on the path without
// re-deriving it.
func TestLoadRepoConfigErrorNamesPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".ws-workflow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wantPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(wantPath, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := NewResolver(Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
		RepoRoot:   root,
	}, nil, nil, nil)

	_, err := r.Get("", "any.key")
	if err == nil {
		t.Fatalf("expected a parse error for the malformed repo config")
	}
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("Get() error does not name the file path %q: %v", wantPath, err)
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

// TestResolverCheckSurfacesEachFileScope verifies Resolver.Check reports a
// malformed project or global file too, not only the repo file.
func TestResolverCheckSurfacesEachFileScope(t *testing.T) {
	for _, scope := range []string{"project", "global"} {
		t.Run(scope, func(t *testing.T) {
			opts := Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir(), RepoRoot: t.TempDir()}
			var path string
			var err error
			if scope == "project" {
				path, err = Path(opts)
			} else {
				path, err = GlobalPath(opts)
			}
			if err != nil {
				t.Fatalf("resolve %s path: %v", scope, err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			r := NewResolver(opts, nil, nil, nil)
			if err := r.Check(); err == nil {
				t.Fatalf("Check() = nil with a malformed %s file", scope)
			}
		})
	}
}
