package wsconfig

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// --- test doubles ---

// fakeSessionStore implements SessionReader and SessionWriter for tests.
// It stores overrides per session key in memory.
type fakeSessionStore struct {
	mu       sync.Mutex
	sessions map[string]map[string]string
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{sessions: map[string]map[string]string{}}
}

func (f *fakeSessionStore) GetOverride(sessionKey, itemKey string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.sessions[sessionKey]
	if !ok {
		return "", false
	}
	v, ok := m[itemKey]
	return v, ok
}

func (f *fakeSessionStore) SetOverride(sessionKey, itemKey, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[sessionKey]; !ok {
		f.sessions[sessionKey] = map[string]string{}
	}
	f.sessions[sessionKey][itemKey] = value
	return nil
}

// --- helpers ---

// newTestResolver creates a Resolver with an isolated temp-dir project scope,
// an isolated temp-dir global scope, and an optional fake session store.
func newTestResolver(t *testing.T, builtinDefaults map[string]string, sess *fakeSessionStore) (Resolver, Options) {
	t.Helper()
	opts := Options{
		CacheHome:  t.TempDir(),
		ConfigHome: t.TempDir(),
	}
	var sr SessionReader
	var sw SessionWriter
	if sess != nil {
		sr = sess
		sw = sess
	}
	return NewResolver(opts, builtinDefaults, sr, sw), opts
}

// --- tests ---

// TestScopePrecedenceSessionOverProject verifies that a session-scope value
// wins over a project-scope value for the same item key.
func TestScopePrecedenceSessionOverProject(t *testing.T) {
	sess := newFakeSessionStore()
	r, opts := newTestResolver(t, nil, sess)
	const key = "test.item"
	const sessionKey = "test-session-key"

	// Set project value.
	if err := r.Set(key, "project-value", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatalf("set project: %v", err)
	}
	// Set session value.
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
	_ = opts
}

// TestScopePrecedenceProjectOverGlobal verifies that a project-scope value wins
// over a global-scope value.
func TestScopePrecedenceProjectOverGlobal(t *testing.T) {
	r, _ := newTestResolver(t, nil, nil)
	const key = "test.item"

	if err := r.Set(key, "global-value", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set global: %v", err)
	}
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

// TestScopePrecedenceGlobalOverBuiltin verifies that a global-scope value wins
// over the builtin default.
func TestScopePrecedenceGlobalOverBuiltin(t *testing.T) {
	builtins := map[string]string{"test.item": "builtin-value"}
	r, _ := newTestResolver(t, builtins, nil)
	const key = "test.item"

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

// TestScopePrecedenceBuiltinFloor verifies that the builtin floor is returned
// when no other scope holds the key.
func TestScopePrecedenceBuiltinFloor(t *testing.T) {
	builtins := map[string]string{"test.item": "builtin-value"}
	r, _ := newTestResolver(t, builtins, nil)

	rv, err := r.Get("", "test.item")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "builtin-value" || rv.Scope != ScopeBuiltin {
		t.Fatalf("expected builtin-value/builtin, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestScopeReportingGetReportsResolvedScope verifies that Get reports the
// correct source scope in ResolvedValue.Scope.
func TestScopeReportingGetReportsResolvedScope(t *testing.T) {
	sess := newFakeSessionStore()
	builtins := map[string]string{"item": "b"}
	r, _ := newTestResolver(t, builtins, sess)

	// Only builtin available.
	rv, _ := r.Get("sk", "item")
	if rv.Scope != ScopeBuiltin {
		t.Errorf("expected builtin scope, got %s", rv.Scope)
	}

	// Add global.
	if err := r.Set("item", "g", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	rv, _ = r.Get("sk", "item")
	if rv.Scope != ScopeGlobal || rv.Value != "g" {
		t.Errorf("expected global scope, got %s/%s", rv.Scope, rv.Value)
	}

	// Add project.
	if err := r.Set("item", "p", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	rv, _ = r.Get("sk", "item")
	if rv.Scope != ScopeProject || rv.Value != "p" {
		t.Errorf("expected project scope, got %s/%s", rv.Scope, rv.Value)
	}

	// Add session.
	if err := r.Set("item", "s", SetOptions{ExplicitScope: ScopeSession, SessionKey: "sk"}); err != nil {
		t.Fatal(err)
	}
	rv, _ = r.Get("sk", "item")
	if rv.Scope != ScopeSession || rv.Value != "s" {
		t.Errorf("expected session scope, got %s/%s", rv.Scope, rv.Value)
	}
}

// TestExplicitScopeOverridesDeclaredDefault verifies that passing an explicit
// scope on Set overrides the item's declared default scope.
func TestExplicitScopeOverridesDeclaredDefault(t *testing.T) {
	// Register the item's declared default as project.
	RegisterDefaultScope("default-project-item", ScopeProject)
	defer delete(scopeRegistry, "default-project-item")

	r, opts := newTestResolver(t, nil, nil)
	const key = "default-project-item"

	// Use explicit global scope — should land in global, not project.
	if err := r.Set(key, "global-forced", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Verify project file does NOT contain the key.
	projectCfg, err := Load(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := projectCfg.Overrides[key]; ok {
		t.Error("value landed in project scope but explicit global was requested")
	}

	// Verify global file DOES contain the key.
	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	if globalCfg.Overrides == nil || globalCfg.Overrides[key] != "global-forced" {
		t.Errorf("global file does not contain value: %v", globalCfg.Overrides)
	}
}

// TestDefaultScopeFallbackToProject verifies that an item with no declared
// default scope writes to project.
func TestDefaultScopeFallbackToProject(t *testing.T) {
	const key = "unregistered-item-xyz"
	// Ensure it is not in the registry.
	delete(scopeRegistry, key)

	r, opts := newTestResolver(t, nil, nil)

	if err := r.Set(key, "val", SetOptions{}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if DefaultScope(key) != ScopeProject {
		t.Errorf("DefaultScope for unregistered key = %s, want project", DefaultScope(key))
	}

	projectCfg, err := Load(opts)
	if err != nil {
		t.Fatal(err)
	}
	if projectCfg.Overrides == nil || projectCfg.Overrides[key] != "val" {
		t.Errorf("project overrides = %v", projectCfg.Overrides)
	}
}

// TestGlobalPathUsesConfigHomeEnvVar verifies that $WS_CONFIG_HOME controls
// the global config file location.
func TestGlobalPathUsesConfigHomeEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WS_CONFIG_HOME", dir)

	// opts has no ConfigHome, so env var should be used.
	opts := Options{CacheHome: t.TempDir()}
	path, err := GlobalPath(opts)
	if err != nil {
		t.Fatalf("GlobalPath: %v", err)
	}
	wantPath := dir + "/config.json"
	if path != wantPath {
		t.Errorf("GlobalPath = %q, want %q", path, wantPath)
	}
}

// TestGlobalPathUsesConfigHomeOption verifies that Options.ConfigHome controls
// the global config file location and takes precedence over the env var.
func TestGlobalPathUsesConfigHomeOption(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WS_CONFIG_HOME", "/should-be-ignored")

	opts := Options{CacheHome: t.TempDir(), ConfigHome: dir}
	path, err := GlobalPath(opts)
	if err != nil {
		t.Fatalf("GlobalPath: %v", err)
	}
	wantPath := dir + "/config.json"
	if path != wantPath {
		t.Errorf("GlobalPath = %q, want %q", path, wantPath)
	}
}

// TestGlobalFileCreatedAtConfigHomePath verifies that setting a global-scoped
// value creates the config file under opts.ConfigHome.
func TestGlobalFileCreatedAtConfigHomePath(t *testing.T) {
	r, opts := newTestResolver(t, nil, nil)
	const key = "test.global"

	if err := r.Set(key, "gval", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set: %v", err)
	}

	globalPath, err := GlobalPath(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(globalPath); os.IsNotExist(err) {
		t.Errorf("global config file was not created at %s", globalPath)
	}

	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	if globalCfg.Overrides == nil || globalCfg.Overrides[key] != "gval" {
		t.Errorf("global overrides = %v", globalCfg.Overrides)
	}
}

// TestConcurrentWritersNoLostWrites verifies that concurrent goroutines
// incrementing a string-encoded counter all land in the file (serialized via
// file lock). Each goroutine sets a unique key; the test checks all N keys are
// present after all goroutines complete.
func TestConcurrentWritersNoLostWrites(t *testing.T) {
	const n = 20
	r, opts := newTestResolver(t, nil, nil)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("concurrent.key.%d", i)
			if err := r.Set(key, fmt.Sprintf("v%d", i), SetOptions{ExplicitScope: ScopeProject}); err != nil {
				t.Errorf("goroutine %d set: %v", i, err)
			}
		}()
	}
	wg.Wait()

	projectCfg, err := Load(opts)
	if err != nil {
		t.Fatalf("load after concurrent writes: %v", err)
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("concurrent.key.%d", i)
		want := fmt.Sprintf("v%d", i)
		if got, ok := projectCfg.Overrides[key]; !ok || got != want {
			t.Errorf("key %s: got %q ok=%v, want %q", key, got, ok, want)
		}
	}
}

// TestConcurrentGlobalWritersNoLostWrites verifies that concurrent writes to
// the global file are also serialized correctly.
func TestConcurrentGlobalWritersNoLostWrites(t *testing.T) {
	const n = 20
	r, opts := newTestResolver(t, nil, nil)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("global.key.%d", i)
			if err := r.Set(key, fmt.Sprintf("gv%d", i), SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
				t.Errorf("goroutine %d set global: %v", i, err)
			}
		}()
	}
	wg.Wait()

	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		t.Fatalf("load global after concurrent writes: %v", err)
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("global.key.%d", i)
		want := fmt.Sprintf("gv%d", i)
		if got, ok := globalCfg.Overrides[key]; !ok || got != want {
			t.Errorf("key %s: got %q ok=%v, want %q", key, got, ok, want)
		}
	}
}

// TestZeroMigrationProjectWinsOverGlobal verifies that a pre-existing project
// config value still wins after a global value is added (no data migration).
func TestZeroMigrationProjectWinsOverGlobal(t *testing.T) {
	r, _ := newTestResolver(t, nil, nil)
	const key = "existing.item"

	// Simulate a pre-existing project value.
	if err := r.Set(key, "pre-existing-project", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatalf("set project: %v", err)
	}
	// Now add a global value (as if a new global layer was introduced).
	if err := r.Set(key, "new-global", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatalf("set global: %v", err)
	}

	rv, err := r.Get("", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rv.Value != "pre-existing-project" || rv.Scope != ScopeProject {
		t.Fatalf("expected pre-existing-project/project, got %q/%q", rv.Value, rv.Scope)
	}
}

// TestScopedShowReportsResolvedScopes verifies that ScopedShow returns
// ResolvedOverrides with correct scope labels.
func TestScopedShowReportsResolvedScopes(t *testing.T) {
	sess := newFakeSessionStore()
	builtins := map[string]string{}
	r, opts := newTestResolver(t, builtins, sess)

	const sessionKey = "test-session-show"
	if err := r.Set("item.global", "gv", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set("item.project", "pv", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set("item.project", "pv-wins", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set("item.session", "sv", SetOptions{ExplicitScope: ScopeSession, SessionKey: sessionKey}); err != nil {
		t.Fatal(err)
	}
	// item.session is also in project — but session should win.
	if err := r.Set("item.session", "sv-project", SetOptions{ExplicitScope: ScopeProject}); err != nil {
		t.Fatal(err)
	}

	view, err := ScopedShow(&r, opts, sessionKey)
	if err != nil {
		t.Fatalf("ScopedShow: %v", err)
	}
	if len(view.ResolvedOverrides) == 0 {
		t.Fatal("expected non-empty ResolvedOverrides")
	}

	found := map[string]ScopedItem{}
	for _, item := range view.ResolvedOverrides {
		found[item.Key] = item
	}

	// item.global: no project override, so global scope.
	if got, ok := found["item.global"]; !ok || got.Scope != ScopeGlobal || got.Value != "gv" {
		t.Errorf("item.global: %+v", found["item.global"])
	}
	// item.project: project scope wins over global.
	if got, ok := found["item.project"]; !ok || got.Scope != ScopeProject || got.Value != "pv-wins" {
		t.Errorf("item.project: %+v", found["item.project"])
	}
	// item.session: session scope wins over project.
	if got, ok := found["item.session"]; !ok || got.Scope != ScopeSession || got.Value != "sv" {
		t.Errorf("item.session: %+v", found["item.session"])
	}
}

// TestCapabilityCheckHookIsCalledAndCanBlock verifies that the CapabilityCheck
// hook is invoked and can reject a set operation.
func TestCapabilityCheckHookIsCalledAndCanBlock(t *testing.T) {
	r, _ := newTestResolver(t, nil, nil)

	called := false
	blockErr := fmt.Errorf("permission denied by test")
	err := r.Set("blocked.item", "value", SetOptions{
		ExplicitScope: ScopeProject,
		CapabilityCheck: func(key string, targetScope Scope) error {
			called = true
			return blockErr
		},
	})
	if !called {
		t.Error("CapabilityCheck was not called")
	}
	if err == nil {
		t.Error("expected error from CapabilityCheck, got nil")
	}
}

// TestCapabilityCheckAllowsWrite verifies that when the hook returns nil the
// write proceeds normally.
func TestCapabilityCheckAllowsWrite(t *testing.T) {
	r, opts := newTestResolver(t, nil, nil)

	err := r.Set("allowed.item", "value", SetOptions{
		ExplicitScope: ScopeProject,
		CapabilityCheck: func(key string, targetScope Scope) error {
			return nil // allow
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectCfg, err := Load(opts)
	if err != nil {
		t.Fatal(err)
	}
	if projectCfg.Overrides == nil || projectCfg.Overrides["allowed.item"] != "value" {
		t.Errorf("overrides = %v", projectCfg.Overrides)
	}
}
