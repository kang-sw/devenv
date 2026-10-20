package mcp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// profileServer is a lead session bound to a fresh git repo with isolated
// project and global config homes.
func profileServer(t *testing.T) (server *Server, root, key string) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	root = t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	server = NewServer(root, "test")
	key, root = parseLoginResponse(t, callLogin(t, server, 940000, root, nil))
	return server, root, key
}

func TestParseWorktreeProfile(t *testing.T) {
	valid := `{"description":"d","sparse":{"mode":"patterns","paths":["/*","!/_deps/"]},"preserve":["x64/","*/x64/"],"hooks":{"post_provision":[{"argv":["tool","setup"],"cwd":"tools"}],"post_acquire":[{"argv":["tool"],"cwd":null}],"pre_release":[{"argv":["tool","x"]}],"pre_remove":[{"argv":["tool","y"]}]}}`
	p, err := parseWorktreeProfile(json.RawMessage(valid))
	if err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if p.Sparse.Mode != "patterns" || len(p.Preserve) != 2 || *p.Hooks.PostProvision[0].Cwd != "tools" || p.Hooks.PostAcquire[0].Cwd != nil {
		t.Fatalf("decoded profile = %+v", p)
	}
	if _, err := parseWorktreeProfile(json.RawMessage(`{}`)); err != nil {
		t.Fatalf("empty profile (a full checkout) rejected: %v", err)
	}
	rejects := []struct{ value, want string }{
		{`[]`, "JSON object"},
		{`"x"`, "JSON object"},
		{`null`, "JSON object"},
		{`{"unknown":1}`, "unknown field"},
		{`{"sparse":{"mode":"tree","paths":["a"]}}`, "sparse.mode"},
		{`{"sparse":{"mode":"cone","paths":[]}}`, "sparse.paths"},
		{`{"sparse":{"mode":"cone"}}`, "sparse.paths"},
		{`{"sparse":{"mode":"cone","paths":[" "]}}`, "sparse.paths[0]"},
		{`{"preserve":[""]}`, "preserve[0]"},
		{`{"hooks":{"post_acquire":[{"argv":[]}]}}`, "hooks.post_acquire[0].argv"},
		{`{"hooks":{"pre_release":[{"argv":["tool",""]}]}}`, "hooks.pre_release[0].argv[1]"},
		{`{"hooks":{"pre_remove":[{"argv":["tool"],"cwd":"/abs"}]}}`, "hooks.pre_remove[0].cwd"},
		{`{"hooks":{"post_provision":[{"argv":["tool"],"cwd":"../out"}]}}`, "hooks.post_provision[0].cwd"},
		{`{"hooks":{"post_install":[]}}`, "unknown field"},
		{`{"description":"a"} {"description":"b"}`, "single JSON object"},
	}
	for _, tc := range rejects {
		if _, err := parseWorktreeProfile(json.RawMessage(tc.value)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("parseWorktreeProfile(%s) = %v, want error containing %q", tc.value, err, tc.want)
		}
	}
}

func TestValidateWorktreeProfileName(t *testing.T) {
	for _, good := range []string{"full", "housekeeping", "build-x64", "a1"} {
		if err := validateWorktreeProfileName(good); err != nil {
			t.Fatalf("name %q rejected: %v", good, err)
		}
	}
	for _, bad := range []string{"", "Build", "1abc", "-a", "a_b", "a.b", "a b"} {
		if err := validateWorktreeProfileName(bad); err == nil {
			t.Fatalf("name %q accepted", bad)
		}
	}
}

// TestWorktreeProfileProvisionHash pins the hash to sparse and
// hooks.post_provision only.
func TestWorktreeProfileProvisionHash(t *testing.T) {
	parse := func(raw string) worktreeProfile {
		t.Helper()
		p, err := parseWorktreeProfile(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		return p
	}
	base := parse(`{"description":"a","sparse":{"mode":"cone","paths":["src"]},"hooks":{"post_provision":[{"argv":["setup"]}]}}`)
	same := []string{
		`{"description":"b","sparse":{"mode":"cone","paths":["src"]},"hooks":{"post_provision":[{"argv":["setup"]}]}}`,
		`{"sparse":{"mode":"cone","paths":["src"]},"preserve":["x64/"],"hooks":{"post_provision":[{"argv":["setup"]}],"post_acquire":[{"argv":["sync"]}],"pre_release":[{"argv":["r"]}],"pre_remove":[{"argv":["d"]}]}}`,
	}
	for _, raw := range same {
		if parse(raw).provisionHash() != base.provisionHash() {
			t.Fatalf("hash changed for a non-provisioning edit: %s", raw)
		}
	}
	differ := []string{
		`{"sparse":{"mode":"cone","paths":["src","docs"]},"hooks":{"post_provision":[{"argv":["setup"]}]}}`,
		`{"sparse":{"mode":"patterns","paths":["src"]},"hooks":{"post_provision":[{"argv":["setup"]}]}}`,
		`{"sparse":{"mode":"cone","paths":["src"]},"hooks":{"post_provision":[{"argv":["setup","--all"]}]}}`,
		`{"sparse":{"mode":"cone","paths":["src"]}}`,
		`{"hooks":{"post_provision":[{"argv":["setup"]}]}}`,
	}
	for _, raw := range differ {
		if parse(raw).provisionHash() == base.provisionHash() {
			t.Fatalf("hash unchanged for a provisioning edit: %s", raw)
		}
	}
}

// TestBuiltinWorktreeProfilesResolve: with no config the family resolves to
// the two builtins, valid under the profile schema.
func TestBuiltinWorktreeProfilesResolve(t *testing.T) {
	r := wsconfig.NewResolver(wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir(), RepoRoot: t.TempDir()}, nil, nil, nil)
	profiles, err := resolveWorktreeProfiles(&r)
	if err != nil {
		t.Fatalf("resolve builtins: %v", err)
	}
	if worktreeProfileNames(profiles) != "full, housekeeping" {
		t.Fatalf("builtin profiles = %s", worktreeProfileNames(profiles))
	}
	full, _ := findWorktreeProfile(profiles, "full")
	if full.Scope != wsconfig.ScopeBuiltin || full.Definition.Sparse != nil {
		t.Fatalf("full = %+v", full)
	}
	hk, _ := findWorktreeProfile(profiles, "housekeeping")
	if hk.Definition.Sparse == nil || hk.Definition.Sparse.Mode != "cone" || strings.Join(hk.Definition.Sparse.Paths, ",") != "ai-docs" {
		t.Fatalf("housekeeping = %+v", hk.Definition)
	}
}

// TestResolveWorktreeProfilesRejectsBadCommittedProfile: a committed profile
// that fails the schema or the name rule fails resolution naming the file and
// the key.
func TestResolveWorktreeProfilesRejectsBadCommittedProfile(t *testing.T) {
	for _, tc := range []struct{ name, body, key string }{
		{"schema", `{"structured_overrides":{"worktree.profile.build":{"sparse":{"mode":"tree","paths":["a"]}}}}`, "worktree.profile.build"},
		{"name", `{"structured_overrides":{"worktree.profile.Build":{}}}`, "worktree.profile.Build"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, root, ".ws-workflow/config.json", tc.body)
			r := wsconfig.NewResolver(wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir(), RepoRoot: root}, nil, nil, nil)
			_, err := resolveWorktreeProfiles(&r)
			file := filepath.Join(root, ".ws-workflow", "config.json")
			if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("resolve = %v, want error naming %s and %s", err, file, tc.key)
			}
		})
	}
}

func gitStatusPorcelain(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

// TestConfigTuneWorktreeProfileRepoWrite drives the family through config.tune:
// a repo write creates the committed file unstaged and names it, a project
// write shadows it, reset removes it, and invalid writes are refused before
// touching any file.
func TestConfigTuneWorktreeProfileRepoWrite(t *testing.T) {
	server, root, key := profileServer(t)
	repoFile := filepath.Join(root, ".ws-workflow", "config.json")
	profile := map[string]any{
		"description": "build checkout",
		"sparse":      map[string]any{"mode": "patterns", "paths": []any{"/*", "!/_deps/"}},
		"preserve":    []any{"x64/"},
		"hooks":       map[string]any{"post_provision": []any{map[string]any{"argv": []any{"tool", "setup"}}}},
	}

	rejects := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"key": "worktree.profile.Build", "value": profile, "scope": "repo"}, "must match"},
		{map[string]any{"key": "worktree.profile.", "value": profile, "scope": "repo"}, "must match"},
		{map[string]any{"key": "worktree.profile.build", "value": profile, "scope": "session"}, "never session-scoped"},
		{map[string]any{"key": "worktree.profile.build", "value": map[string]any{"sparse": map[string]any{"mode": "tree", "paths": []any{"a"}}}, "scope": "repo"}, "sparse.mode"},
		{map[string]any{"key": "worktree.profile.build", "value": map[string]any{"bogus": true}, "scope": "repo"}, "unknown field"},
		{map[string]any{"key": "worktree.profile.build", "value": "{}", "scope": "repo"}, "must be a profile object"},
		{map[string]any{"key": "worktree.profile.build", "scope": "repo"}, "value is required"},
	}
	for i, tc := range rejects {
		resp := keyedCall(t, server, 940100+i, key, "config.tune", tc.args)
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), tc.want) {
			t.Fatalf("config.tune(%v) = %s, want error containing %q", tc.args, resp, tc.want)
		}
	}
	if _, err := os.Stat(repoFile); !os.IsNotExist(err) {
		t.Fatalf("a refused write touched %s: %v", repoFile, err)
	}

	resp := keyedCall(t, server, 940200, key, "config.tune", map[string]any{"key": "worktree.profile.build", "value": profile, "scope": "repo"})
	if toolIsError(t, resp) {
		t.Fatalf("repo profile write failed: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "written: "+repoFile) || !strings.Contains(text, "git.commit") {
		t.Fatalf("repo write result must name the file and the git.commit obligation: %s", text)
	}
	if status := gitStatusPorcelain(t, root); !strings.Contains(status, "?? .ws-workflow/config.json") {
		t.Fatalf("repo write must leave the file unstaged; status:\n%s", status)
	}
	members := map[string]map[string]json.RawMessage{}
	raw, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("read repo file: %v", err)
	}
	if err := json.Unmarshal(raw, &members); err != nil {
		t.Fatalf("repo file: %v\n%s", err, raw)
	}
	stored, err := parseWorktreeProfile(members["structured_overrides"]["worktree.profile.build"])
	if err != nil || stored.Sparse.Mode != "patterns" || stored.Hooks.PostProvision[0].Argv[1] != "setup" {
		t.Fatalf("stored profile = %+v, %v\n%s", stored, err, raw)
	}

	got := configGetJSON(t, server, 940210, map[string]any{"key": "worktree.profile.build", "session_key": key})
	if got.Scope != "repo" {
		t.Fatalf("config.get scope = %q, want repo", got.Scope)
	}

	// A project write wins over repo; a second repo write reports it shadowed.
	resp = keyedCall(t, server, 940220, key, "config.tune", map[string]any{"key": "worktree.profile.build", "value": map[string]any{"description": "local"}, "scope": "project"})
	if toolIsError(t, resp) {
		t.Fatalf("project profile write failed: %s", resp)
	}
	resp = keyedCall(t, server, 940221, key, "config.tune", map[string]any{"key": "worktree.profile.build", "value": profile, "scope": "repo"})
	if !strings.Contains(toolText(t, resp), "shadowed: effective worktree.profile.build [scope:project]") {
		t.Fatalf("shadowed repo write must say so: %s", toolText(t, resp))
	}

	// Reset removes the profile from the selected scope only.
	resp = keyedCall(t, server, 940230, key, "config.tune", map[string]any{"key": "worktree.profile.build", "reset": true, "scope": "project"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "effective: worktree.profile.build [scope:repo]") {
		t.Fatalf("project reset = %s", resp)
	}
	resp = keyedCall(t, server, 940231, key, "config.tune", map[string]any{"key": "worktree.profile.build", "reset": true, "scope": "repo"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "worktree.profile.build is not defined in any scope") {
		t.Fatalf("repo reset = %s", resp)
	}
	if got := configGetJSON(t, server, 940232, map[string]any{"key": "worktree.profile.build", "session_key": key}); got.Scope != configGetUnsetScope {
		t.Fatalf("config.get after reset = %+v, want unset", got)
	}

	// A builtin is overridable by name and falls back on reset.
	resp = keyedCall(t, server, 940240, key, "config.tune", map[string]any{"key": "worktree.profile.full", "value": map[string]any{"description": "mine"}, "scope": "global"})
	if toolIsError(t, resp) {
		t.Fatalf("global override of a builtin failed: %s", resp)
	}
	if got := configGetJSON(t, server, 940241, map[string]any{"key": "worktree.profile.full", "session_key": key}); got.Scope != "global" {
		t.Fatalf("builtin override scope = %q, want global", got.Scope)
	}
	resp = keyedCall(t, server, 940242, key, "config.tune", map[string]any{"key": "worktree.profile.full", "reset": true, "scope": "global"})
	if !strings.Contains(toolText(t, resp), "effective: worktree.profile.full [scope:builtin]") {
		t.Fatalf("builtin reset fallback = %s", toolText(t, resp))
	}
}

// TestConfigTuneScalarRepoWrite: every repo-scoped scalar key writes the
// committed file through config.tune; a global-only key and agents.tier
// refuse it.
func TestConfigTuneScalarRepoWrite(t *testing.T) {
	server, root, key := profileServer(t)
	repoFile := filepath.Join(root, ".ws-workflow", "config.json")

	resp := keyedCall(t, server, 940300, key, "config.tune", map[string]any{"key": "review_phase", "value": "full", "scope": "repo"})
	if toolIsError(t, resp) {
		t.Fatalf("repo write failed: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "review_phase: full [scope:repo]") || !strings.Contains(text, "written: "+repoFile) {
		t.Fatalf("repo write result = %s", text)
	}
	raw, err := os.ReadFile(repoFile)
	if err != nil || !strings.Contains(string(raw), `"review_phase": "full"`) || strings.Contains(string(raw), "schema_version") {
		t.Fatalf("repo file = %s, %v", raw, err)
	}
	if status := gitStatusPorcelain(t, root); strings.Contains(status, "A  .ws-workflow") {
		t.Fatalf("config.tune staged the repo file:\n%s", status)
	}

	resp = keyedCall(t, server, 940301, key, "config.tune", map[string]any{"key": "review_phase", "reset": true, "scope": "repo"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "review_phase: lite [scope:builtin]") {
		t.Fatalf("repo reset = %s", resp)
	}

	for i, args := range []map[string]any{
		{"key": "workflow.prefer_subagent", "value": "on", "scope": "repo"},
		{"key": "agents.tier", "value": map[string]any{"tier": "medium", "model": "m"}, "scope": "repo"},
	} {
		resp := keyedCall(t, server, 940310+i, key, "config.tune", args)
		if !toolIsError(t, resp) {
			t.Fatalf("config.tune(%v) accepted a repo write: %s", args, resp)
		}
	}
	// A keyless repo write has no worktree to anchor the file.
	resp = callToolOnce(t, server, 940320, "config.tune", map[string]any{"key": "review_phase", "value": "full", "scope": "repo"})
	if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "session_key") {
		t.Fatalf("keyless repo write = %s, want refusal naming session_key", resp)
	}
}

// TestConfigListWorktreeProfileKnob: the catalog lists the family with its
// value_schema and the resolved profiles with provenance; a bad committed
// profile surfaces as the knob's error instead of failing config.list.
func TestConfigListWorktreeProfileKnob(t *testing.T) {
	server, root, key := profileServer(t)
	mustWrite(t, root, ".ws-workflow/config.json", `{"structured_overrides":{"worktree.profile.build":{"description":"b","sparse":{"mode":"cone","paths":["src"]}}}}`)

	knob := func(id int) tuningKnob {
		t.Helper()
		resp := keyedCall(t, server, id, key, "config.list", map[string]any{"format": "json"})
		if toolIsError(t, resp) {
			t.Fatalf("config.list failed: %s", resp)
		}
		var view struct {
			Knobs []tuningKnob `json:"knobs"`
		}
		if err := json.Unmarshal([]byte(toolText(t, resp)), &view); err != nil {
			t.Fatalf("config.list json: %v", err)
		}
		for _, k := range view.Knobs {
			if k.ID == "worktree.profile.<name>" {
				return k
			}
		}
		t.Fatalf("config.list has no worktree.profile.<name> knob")
		return tuningKnob{}
	}

	k := knob(940400)
	schema, _ := json.Marshal(k.ValueSchema)
	if !strings.Contains(string(schema), `"post_provision"`) || !strings.Contains(string(schema), `"patterns"`) || k.RepoMember != "structured_overrides" || !k.RepoScope {
		t.Fatalf("knob = %+v schema %s", k, schema)
	}
	current, _ := json.Marshal(k.Current)
	var profiles []resolvedWorktreeProfile
	if err := json.Unmarshal(current, &profiles); err != nil {
		t.Fatalf("current: %v\n%s", err, current)
	}
	build, found := findWorktreeProfile(profiles, "build")
	if !found || build.Scope != wsconfig.ScopeRepo || build.Source != filepath.Join(root, ".ws-workflow", "config.json") {
		t.Fatalf("build profile = %+v (found %v)", build, found)
	}
	if hk, _ := findWorktreeProfile(profiles, "housekeeping"); hk.Scope != wsconfig.ScopeBuiltin {
		t.Fatalf("housekeeping = %+v", hk)
	}

	mustWrite(t, root, ".ws-workflow/config.json", `{"structured_overrides":{"worktree.profile.build":{"sparse":{"mode":"tree","paths":["src"]}}}}`)
	k = knob(940401)
	if !strings.Contains(k.Error, "worktree.profile.build") || !strings.Contains(k.Error, ".ws-workflow") || k.Current != nil {
		t.Fatalf("bad profile knob = %+v", k)
	}
}

