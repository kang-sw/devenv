package wsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRawFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readJSONMembers(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	members := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &members); err != nil {
		t.Fatalf("parse %s: %v\n%s", path, err, raw)
	}
	return members
}

// TestListStructuredPrecedence pins project > repo > global > builtin per key,
// with the winning scope's whole value (no field merge) and its source file.
func TestListStructuredPrecedence(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	builtin := map[string]json.RawMessage{
		"fam.a":   json.RawMessage(`{"from":"builtin"}`),
		"fam.d":   json.RawMessage(`{"from":"builtin"}`),
		"other.x": json.RawMessage(`{"from":"builtin"}`),
	}
	for _, step := range []struct {
		scope Scope
		key   string
		value string
	}{
		{ScopeGlobal, "fam.a", `{"from":"global"}`},
		{ScopeGlobal, "fam.b", `{"from":"global"}`},
		{ScopeRepo, "fam.a", `{"from":"repo","extra":1}`},
		{ScopeRepo, "fam.c", `{"from":"repo"}`},
		{ScopeProject, "fam.c", `{"from":"project"}`},
	} {
		if err := r.SetStructured(step.key, json.RawMessage(step.value), step.scope); err != nil {
			t.Fatalf("SetStructured(%s, %s): %v", step.key, step.scope, err)
		}
	}

	got, err := r.ListStructured("fam.", builtin)
	if err != nil {
		t.Fatalf("ListStructured: %v", err)
	}
	repoPath, _ := RepoPath(opts)
	globalPath, _ := GlobalPath(opts)
	projectPath, _ := Path(opts)
	want := []struct {
		key, from string
		scope     Scope
		source    string
	}{
		{"fam.a", "repo", ScopeRepo, repoPath},
		{"fam.b", "global", ScopeGlobal, globalPath},
		{"fam.c", "project", ScopeProject, projectPath},
		{"fam.d", "builtin", ScopeBuiltin, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("ListStructured = %+v, want %d entries", got, len(want))
	}
	for i, w := range want {
		var value map[string]any
		if err := json.Unmarshal(got[i].Value, &value); err != nil {
			t.Fatalf("decode %s: %v", got[i].Key, err)
		}
		if got[i].Key != w.key || value["from"] != w.from || got[i].Scope != w.scope || got[i].Source != w.source {
			t.Fatalf("entry %d = %+v (value %v), want key %s from %s scope %s source %s", i, got[i], value, w.key, w.from, w.scope, w.source)
		}
	}
	// Whole-value override: the repo value replaces the global one, extra
	// field and all.
	var first map[string]any
	if err := json.Unmarshal(got[0].Value, &first); err != nil || first["extra"] != float64(1) {
		t.Fatalf("repo value not taken whole: %s", got[0].Value)
	}
}

// TestRepoStructuredWritePreservesOtherMembers verifies a repo write touches
// only overrides/structured_overrides: an unknown member survives, no
// schema_version or agents noise is added, and no lock file lands in the
// worktree.
func TestRepoStructuredWritePreservesOtherMembers(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	repoPath, _ := RepoPath(opts)
	writeRawFile(t, repoPath, `{"overrides":{"ticket-assignee-aware":"on"},"future_member":{"kept":true}}`+"\n")

	if err := r.SetStructured("fam.a", json.RawMessage(`{"x":1}`), ScopeRepo); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}
	if err := r.Set("review_phase", "full", SetOptions{ExplicitScope: ScopeRepo}); err != nil {
		t.Fatalf("Set repo: %v", err)
	}
	members := readJSONMembers(t, repoPath)
	for _, absent := range []string{"schema_version", "agents"} {
		if _, ok := members[absent]; ok {
			t.Fatalf("repo write added %q: %v", absent, members)
		}
	}
	var future map[string]bool
	if err := json.Unmarshal(members["future_member"], &future); err != nil || !future["kept"] {
		t.Fatalf("unknown member not preserved: %s", members["future_member"])
	}
	var overrides map[string]string
	if err := json.Unmarshal(members["overrides"], &overrides); err != nil || overrides["ticket-assignee-aware"] != "on" || overrides["review_phase"] != "full" {
		t.Fatalf("overrides = %s (%v)", members["overrides"], err)
	}
	if !strings.Contains(string(members["structured_overrides"]), `"fam.a"`) {
		t.Fatalf("structured_overrides = %s", members["structured_overrides"])
	}
	entries, err := os.ReadDir(filepath.Dir(repoPath))
	if err != nil {
		t.Fatalf("read repo config dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("repo write left %q beside the committed file", e.Name())
		}
	}

	rv, err := r.Get("", "review_phase")
	if err != nil || rv.Value != "full" || rv.Scope != ScopeRepo {
		t.Fatalf("Get after repo Set = %+v, %v", rv, err)
	}
	if err := r.Unset("review_phase", SetOptions{ExplicitScope: ScopeRepo}); err != nil {
		t.Fatalf("Unset repo: %v", err)
	}
	if rv, _ := r.Get("", "review_phase"); rv.Scope == ScopeRepo {
		t.Fatalf("repo Unset left the value: %+v", rv)
	}
}

// TestUnsetStructuredRemovesAndNoOps covers reset of a structured value and
// the absent-key / absent-file no-ops.
func TestUnsetStructuredRemovesAndNoOps(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	if err := r.UnsetStructured("fam.a", ScopeRepo); err != nil {
		t.Fatalf("unset on absent file: %v", err)
	}
	repoPath, _ := RepoPath(opts)
	if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
		t.Fatalf("unset on absent file created it: %v", err)
	}
	for _, scope := range []Scope{ScopeProject, ScopeRepo, ScopeGlobal} {
		if err := r.SetStructured("fam.a", json.RawMessage(`{"s":"`+string(scope)+`"}`), scope); err != nil {
			t.Fatalf("set %s: %v", scope, err)
		}
	}
	if err := r.UnsetStructured("fam.a", ScopeProject); err != nil {
		t.Fatalf("unset project: %v", err)
	}
	got, err := r.ListStructured("fam.", nil)
	if err != nil || len(got) != 1 || got[0].Scope != ScopeRepo {
		t.Fatalf("after project unset = %+v, %v; want repo winner", got, err)
	}
	if err := r.UnsetStructured("fam.missing", ScopeRepo); err != nil {
		t.Fatalf("unset absent key: %v", err)
	}
	if err := r.UnsetStructured("fam.a", ScopeRepo); err != nil {
		t.Fatalf("unset repo: %v", err)
	}
	if _, ok := readJSONMembers(t, repoPath)["structured_overrides"]; ok {
		t.Fatalf("empty structured_overrides member left in the repo file")
	}
	got, _ = r.ListStructured("fam.", nil)
	if len(got) != 1 || got[0].Scope != ScopeGlobal {
		t.Fatalf("after repo unset = %+v; want global winner", got)
	}
}

// TestStructuredRefusesSessionScope: structured values have no session scope.
func TestStructuredRefusesSessionScope(t *testing.T) {
	r, _ := newRepoTestResolver(t, newFakeSessionStore())
	if err := r.SetStructured("fam.a", json.RawMessage(`{}`), ScopeSession); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("SetStructured(session) = %v, want refusal", err)
	}
}

// TestRepoWriteNeedsRoot: without a worktree root the repo scope has no file
// to write.
func TestRepoWriteNeedsRoot(t *testing.T) {
	r := NewResolver(Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}, nil, nil, nil)
	if err := r.Set("review_phase", "full", SetOptions{ExplicitScope: ScopeRepo}); err == nil || !strings.Contains(err.Error(), "session_key") {
		t.Fatalf("Set(repo) without root = %v, want refusal naming session_key", err)
	}
	if err := r.SetStructured("fam.a", json.RawMessage(`{}`), ScopeRepo); err == nil {
		t.Fatalf("SetStructured(repo) without root succeeded")
	}
}

// TestRepoWriteRefusedForGlobalOnlyKey: a global-only key never reads the repo
// layer, so a repo write is refused.
func TestRepoWriteRefusedForGlobalOnlyKey(t *testing.T) {
	r, _ := newRepoTestResolver(t, nil)
	if err := r.Set(ItemWorkflowPreferSubagent, "on", SetOptions{ExplicitScope: ScopeRepo}); err == nil || !strings.Contains(err.Error(), "global-only") {
		t.Fatalf("repo write of a global-only key = %v, want global-only refusal", err)
	}
}

// TestListStructuredMalformedFileNamesPath: a malformed committed file fails
// the listing and names the file.
func TestListStructuredMalformedFileNamesPath(t *testing.T) {
	r, opts := newRepoTestResolver(t, nil)
	repoPath, _ := RepoPath(opts)
	writeRawFile(t, repoPath, `{"structured_overrides": {"fam.a": `)
	_, err := r.ListStructured("fam.", nil)
	if err == nil || !strings.Contains(err.Error(), repoPath) {
		t.Fatalf("ListStructured on malformed file = %v, want error naming %s", err, repoPath)
	}
}

// TestPreStructuredConfigShapeParsesStructuredFile pins forward
// compatibility: a reader built on the pre-change Config shape (a plain string
// overrides map and no structured member) still parses a file carrying a
// structured value, so a teammate on an older ws keeps every other knob.
func TestPreStructuredConfigShapeParsesStructuredFile(t *testing.T) {
	type preStructuredConfig struct {
		SchemaVersion int               `json:"schema_version"`
		Agents        AgentsConfig      `json:"agents"`
		Overrides     map[string]string `json:"overrides,omitempty"`
	}
	r, opts := newRepoTestResolver(t, nil)
	if err := r.Set("review_phase", "full", SetOptions{ExplicitScope: ScopeRepo}); err != nil {
		t.Fatalf("Set repo: %v", err)
	}
	if err := r.SetStructured("worktree.profile.build", json.RawMessage(`{"description":"d","sparse":{"mode":"patterns","paths":["/*","!/_deps/"]},"hooks":{"post_provision":[{"argv":["tool","setup"]}]}}`), ScopeRepo); err != nil {
		t.Fatalf("SetStructured repo: %v", err)
	}
	repoPath, _ := RepoPath(opts)
	raw, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var old preStructuredConfig
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("pre-change Config shape failed to parse a file carrying a profile: %v\n%s", err, raw)
	}
	if old.Overrides["review_phase"] != "full" {
		t.Fatalf("pre-change reader lost a string override: %+v", old.Overrides)
	}
}
