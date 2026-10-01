package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestConfigListSurfacesRepoScope verifies config.list resolves and reports a
// key held only in the committed repo file (<root>/.ws-workflow/config.json),
// tagging it with the "repo" scope in the resolved-overrides view. This is the
// end-to-end path the assignee-flag consumer relies on: a session-bound caller
// anchors the repo scope at its worktree root and reads the committed value
// deterministically.
func TestConfigListSurfacesRepoScope(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// Committed repo-scope config with a project-wide flag.
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showJSON := toolText(t, byID["1"])

	var view struct {
		ResolvedOverrides []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Scope string `json:"scope"`
		} `json:"resolved_overrides"`
	}
	if err := json.Unmarshal([]byte(showJSON), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, showJSON)
	}

	var found bool
	for _, item := range view.ResolvedOverrides {
		if item.Key == "ticket-assignee-aware" {
			found = true
			if item.Value != "on" || item.Scope != "repo" {
				t.Fatalf("repo-scope key mismatch: got value=%q scope=%q, want on/repo", item.Value, item.Scope)
			}
		}
	}
	if !found {
		t.Fatalf("config.list did not surface committed repo-scope key: %s", showJSON)
	}
}

// TestConfigListKeylessSkipsRepoScope verifies a keyless config.list caller (the
// former config.show contract) resolves without a repo anchor and does not error
// even though no session root is available to locate a committed file.
func TestConfigListKeylessSkipsRepoScope(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// A committed file exists, but a keyless caller has no anchor to reach it.
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showJSON := toolText(t, byID["1"])

	var view struct {
		ResolvedOverrides []struct {
			Key   string `json:"key"`
			Scope string `json:"scope"`
		} `json:"resolved_overrides"`
	}
	if err := json.Unmarshal([]byte(showJSON), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, showJSON)
	}
	for _, item := range view.ResolvedOverrides {
		if item.Key == "ticket-assignee-aware" && item.Scope == "repo" {
			t.Fatalf("keyless config.list resolved a repo-scope value with no anchor: %s", showJSON)
		}
	}
}

// --- Phase 1 Verification (i)-(ix): 261001-feat-config-repo-scope-and-tune-weight-guidance ---
//
// The tests below exercise sessionConfigOptions/sessionResolver (the single
// session-anchored resolver constructor) at the readers Phase 1 switched to
// it: tickets.sage_gate/create_empty/move, the worktree-pool readers,
// config.tune's echo, playbook.read's prompt-override lookup, and
// config.list's own catalog+view surface.

// TestRepoScopeSageGateRequiresDesignReview is Phase 1 Verification item (i): a
// committed sage_review_design: auto in a temp repo's
// .ws-workflow/config.json makes tickets.sage_gate require design review
// under a session rooted there, with no project or global override.
// sage_review (completeness) is left at its builtin "auto" default, which
// always requires completeness regardless of repo scope, so the assertion
// pins the full "design, completeness" combined-mode response: if the repo
// scope had not fed cfg.Design, design would resolve to the builtin "off"
// (skipped) and the response would list only "completeness".
func TestRepoScopeSageGateRequiresDesignReview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	stem := "260101-feat-repo-scope-design"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Repo scope design\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 920101, root, nil))

	resp := callToolWithKey(t, server, 920102, key, "tickets.sage_gate", map[string]any{
		"stem":    stem,
		"landing": "ready",
	})
	for _, want := range []string{"action: run", "reviewers: design, completeness", "mode: combined"} {
		if !strings.Contains(resp, want) {
			t.Fatalf("sage_gate did not apply the committed repo sage_review_design=auto, missing %q:\n%s", want, resp)
		}
	}
}

// TestRepoScopeAppliesAtTicketMutatorsAndWorktreePool is Phase 1 Verification
// item (ii): the same repo-scope application proven at tickets.sage_gate
// (item i) also reaches tickets.create_empty's and tickets.move's posture
// echo, and a worktree-pool knob (worktree_pool) reaches worktree.acquire —
// each under a session rooted at the temp repo, with no project or global
// override.
func TestRepoScopeAppliesAtTicketMutatorsAndWorktreePool(t *testing.T) {
	t.Run("create_empty", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920201, root, nil))

		// initial_state "todo" stamps the design posture at creation time (idea/
		// landing skips the gate entirely, see sage_gate's idea branch), so the
		// repo-anchored resolver must apply before the first frontmatter write.
		resp := callToolWithKey(t, server, 920202, key, "tickets.create_empty", map[string]any{
			"stem":          "epic-repo-scope-create",
			"initial_state": "todo",
		})
		// The exact posture fragment: a bare "required" also appears in the
		// skipped-posture tip an epic gets without the repo value.
		if !strings.Contains(resp, "sage review posture: design required.") || strings.Contains(resp, "design skipped") {
			t.Fatalf("tickets.create_empty did not apply the committed repo sage_review_design=auto: %s", resp)
		}
	})

	t.Run("move", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920203, root, nil))

		// idea/ landing never stamps a posture (sage_gate skips it entirely), so
		// the move to todo/ is the first posture resolution and must read the
		// repo scope.
		createResp := callToolWithKey(t, server, 920204, key, "tickets.create_empty", map[string]any{
			"stem":          "epic-repo-scope-move",
			"initial_state": "idea",
		})
		if !strings.Contains(createResp, "Created ai-docs/tickets/idea/") {
			t.Fatalf("tickets.create_empty response missing created path: %s", createResp)
		}
		matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "idea", "*-epic-repo-scope-move.md"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("glob created ticket: matches=%v err=%v", matches, err)
		}
		datedStem := strings.TrimSuffix(filepath.Base(matches[0]), ".md")

		moveResp := callToolWithKey(t, server, 920205, key, "tickets.move", map[string]any{
			"stem": datedStem,
			"to":   "todo",
		})
		if !strings.Contains(moveResp, "sage review posture: design required.") || strings.Contains(moveResp, "design skipped") {
			t.Fatalf("tickets.move did not apply the committed repo sage_review_design=auto: %s", moveResp)
		}
	})

	t.Run("worktree_pool", func(t *testing.T) {
		root, base := worktreeFixture(t)
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"worktree_pool":"$(GitRoot)/.repo-scope-pool"}}`+"\n")
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920206, root, nil))

		resp := callToolWithKey(t, server, 920207, key, "worktree.acquire", map[string]any{
			"base":          base,
			"target_branch": "impl/test/repo-scope-pool",
			"format":        "json",
		})
		var acq worktreeAcquireResult
		if err := json.Unmarshal([]byte(resp), &acq); err != nil {
			t.Fatalf("unmarshal worktree.acquire response: %v\n%s", err, resp)
		}
		wantPrefix := filepath.Join(canonicalRootForTest(t, root), ".repo-scope-pool")
		if !strings.HasPrefix(acq.Path, wantPrefix) {
			t.Fatalf("worktree.acquire path = %q, want it under the committed repo-scope pool %q", acq.Path, wantPrefix)
		}
	})
}

// TestRepoScopeProjectValueBeatsRepoValue is Phase 1 Verification item (iii):
// a project-scope sage_review_design value still beats the committed repo
// value for the same key. Completeness (sage_review) stays at its builtin
// "auto" default throughout, so the reviewer list distinguishes the two
// cases unambiguously: if the repo value won, "design" would appear in the
// reviewer list; with the project value correctly winning, only
// "completeness" does.
func TestRepoScopeProjectValueBeatsRepoValue(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	stem := "260101-feat-repo-scope-precedence"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Repo scope precedence\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 920301, root, nil))

	tuneResp := callToolWithKey(t, server, 920302, key, "config.tune", map[string]any{
		"key":   wsconfig.ItemSageReviewDesign,
		"value": "off",
		"scope": "project",
	})
	if !strings.Contains(tuneResp, "sage_review_design: off [scope:project]") {
		t.Fatalf("project-scope config.tune response: %s", tuneResp)
	}

	resp := callToolWithKey(t, server, 920303, key, "tickets.sage_gate", map[string]any{
		"stem":    stem,
		"landing": "ready",
	})
	if strings.Contains(resp, "reviewers: design") {
		t.Fatalf("project-scope sage_review_design=off should beat the committed repo auto value, but design still ran:\n%s", resp)
	}
	if !strings.Contains(resp, "reviewers: completeness") || !strings.Contains(resp, "mode: standalone") {
		t.Fatalf("expected a completeness-only standalone gate response:\n%s", resp)
	}
}

// TestRepoScopeKeylessPlaybookReadDropsRepoScope is Phase 1 Verification item
// (iv): a keyless call drops the repo scope without error. playbook.read's
// session_key argument is genuinely optional (unlike the ticket mutators and
// worktree-pool readers, which require a session_key just to resolve a
// root), so it is the reader that can exercise a true keyless call into the
// new session-anchored prompt-override resolver (buildOverrideLookup,
// Decision 10).
func TestRepoScopeKeylessPlaybookReadDropsRepoScope(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	const marker = "REPO-SCOPE-KEYLESS-MARKER"
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"prompt.UserPreferenceSection.all":"`+marker+`"}}`+"\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	resp := callToolOnce(t, server, 920401, "playbook.read", map[string]any{"name": "lead-workflow-manual"})
	if toolIsError(t, resp) {
		t.Fatalf("keyless playbook.read must not error: %s", resp)
	}
	if strings.Contains(toolText(t, resp), marker) {
		t.Fatalf("keyless playbook.read must not read the committed repo prompt override:\n%s", toolText(t, resp))
	}
}

// TestConfigListReportsRepoScopeFields is Phase 1 Verification item (v):
// config.list's JSON response reports the committed repo-scope file's path
// and overrides shape, the per-knob repo_scope flag for a repo-capable knob
// (sage_review_design), a non-repo-capable scalar knob (agents.tier, not
// resolver-backed) and a global-only knob (bootstrap_alarm), a repo-capable
// prompt.* knob, and the repo_scope flag on the two repo-capable
// scoped-view items outside the tuning catalog (worktree_pool,
// ticket-assignee-aware).
func TestConfigListReportsRepoScopeFields(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 920501, root, nil))

	resp := callToolWithKey(t, server, 920502, key, "config.list", map[string]any{"format": "json"})

	var view struct {
		RepoScope *struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
			Shape  string `json:"shape"`
		} `json:"repo_scope"`
		ResolvedOverrides []struct {
			Key       string `json:"key"`
			RepoScope bool   `json:"repo_scope"`
		} `json:"resolved_overrides"`
		Knobs []struct {
			ID        string `json:"id"`
			RepoScope bool   `json:"repo_scope"`
		} `json:"knobs"`
	}
	if err := json.Unmarshal([]byte(resp), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, resp)
	}

	if view.RepoScope == nil {
		t.Fatalf("config.list missing top-level repo_scope: %s", resp)
	}
	wantPath := filepath.Join(canonicalRootForTest(t, root), ".ws-workflow", "config.json")
	if view.RepoScope.Path != wantPath || !view.RepoScope.Exists {
		t.Fatalf("config.list repo_scope = %+v, want path=%q exists=true", view.RepoScope, wantPath)
	}
	if view.RepoScope.Shape != wsconfig.RepoOverridesShape {
		t.Fatalf("config.list repo_scope.shape = %q, want %q", view.RepoScope.Shape, wsconfig.RepoOverridesShape)
	}

	knobRepoScope := map[string]bool{}
	knobSeen := map[string]bool{}
	for _, k := range view.Knobs {
		knobRepoScope[k.ID] = k.RepoScope
		knobSeen[k.ID] = true
	}
	if !knobSeen["sage_review_design"] || !knobRepoScope["sage_review_design"] {
		t.Fatalf("sage_review_design knob repo_scope = %v (seen=%v), want true", knobRepoScope["sage_review_design"], knobSeen["sage_review_design"])
	}
	if !knobSeen["agents.tier"] || knobRepoScope["agents.tier"] {
		t.Fatalf("agents.tier knob repo_scope = %v (seen=%v), want false (not resolver-backed)", knobRepoScope["agents.tier"], knobSeen["agents.tier"])
	}
	if !knobSeen["bootstrap_alarm"] || knobRepoScope["bootstrap_alarm"] {
		t.Fatalf("bootstrap_alarm knob repo_scope = %v (seen=%v), want false (global-only)", knobRepoScope["bootstrap_alarm"], knobSeen["bootstrap_alarm"])
	}
	if !knobSeen["prompt.UserPreferenceSection"] || !knobRepoScope["prompt.UserPreferenceSection"] {
		t.Fatalf("prompt.UserPreferenceSection knob repo_scope = %v (seen=%v), want true", knobRepoScope["prompt.UserPreferenceSection"], knobSeen["prompt.UserPreferenceSection"])
	}

	overrideRepoScope := map[string]bool{}
	overrideSeen := map[string]bool{}
	for _, item := range view.ResolvedOverrides {
		overrideRepoScope[item.Key] = item.RepoScope
		overrideSeen[item.Key] = true
	}
	if !overrideSeen["worktree_pool"] || !overrideRepoScope["worktree_pool"] {
		t.Fatalf("worktree_pool resolved_overrides repo_scope = %v (seen=%v), want true", overrideRepoScope["worktree_pool"], overrideSeen["worktree_pool"])
	}
	if !overrideSeen["ticket-assignee-aware"] || !overrideRepoScope["ticket-assignee-aware"] {
		t.Fatalf("ticket-assignee-aware resolved_overrides repo_scope = %v (seen=%v), want true", overrideRepoScope["ticket-assignee-aware"], overrideSeen["ticket-assignee-aware"])
	}
}

// TestRepoScopePromptOverridePlaybookRender is Phase 1 Verification item
// (vi): a committed prompt.<pointId>.<harness> override in the temp repo
// appears in a playbook rendered under a session rooted there.
// UserPreferenceSection/all is a real override point (scanOverridePoints),
// and the "all" harness bucket is the automatic fallback
// applyOverrideMarkers tries after the harness-exact lookup misses, so this
// exercises the repo-anchored prompt-override resolver (buildOverrideLookup,
// Decision 10) without needing a harness-specific key.
func TestRepoScopePromptOverridePlaybookRender(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	const marker = "REPO-SCOPE-PROMPT-OVERRIDE-MARKER"
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"prompt.UserPreferenceSection.all":"`+marker+`"}}`+"\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 920601, root, nil))

	resp := callToolWithKey(t, server, 920602, key, "playbook.read", map[string]any{"name": "lead-workflow-manual"})
	if !strings.Contains(resp, marker) {
		t.Fatalf("rendered playbook missing the committed repo-scope prompt override:\n%s", resp)
	}
}

// TestRepoScopeMalformedFileFailsLoud is Phase 1 Verification item (vii): a
// malformed committed repo file makes tickets.sage_gate, tickets.move, and a
// worktree-pool reader return an error naming the file, never a silently
// dropped-to-skipped posture or an empty pool.
func TestRepoScopeMalformedFileFailsLoud(t *testing.T) {
	const malformed = "{ not json"

	t.Run("tickets.sage_gate", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		stem := "260101-feat-repo-scope-malformed-gate"
		mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
			"---\ntitle: Repo scope malformed\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
		mustWrite(t, root, ".ws-workflow/config.json", malformed)
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920701, root, nil))
		wantPath := filepath.Join(canonicalRootForTest(t, root), ".ws-workflow", "config.json")

		resp := callToolOnce(t, server, 920702, "tickets.sage_gate", map[string]any{
			"session_key": key, "stem": stem, "landing": "ready",
		})
		if !toolIsError(t, resp) {
			t.Fatalf("tickets.sage_gate must error on a malformed committed repo file, got: %s", resp)
		}
		text := toolText(t, resp)
		if !strings.Contains(text, wantPath) {
			t.Fatalf("tickets.sage_gate error does not name the file path %q: %s", wantPath, text)
		}
		if strings.Contains(text, "skipped") {
			t.Fatalf("tickets.sage_gate must not resolve to a skipped posture on a config load error: %s", text)
		}
	})

	t.Run("tickets.move", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		stem := "260101-feat-repo-scope-malformed-move"
		ticketRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
		mustWrite(t, root, ticketRel,
			"---\ntitle: Repo scope malformed move\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
		mustWrite(t, root, ".ws-workflow/config.json", malformed)
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920703, root, nil))
		wantPath := filepath.Join(canonicalRootForTest(t, root), ".ws-workflow", "config.json")

		resp := callToolOnce(t, server, 920704, "tickets.move", map[string]any{
			"session_key": key, "stem": stem, "to": "ready",
		})
		if !toolIsError(t, resp) {
			t.Fatalf("tickets.move must error on a malformed committed repo file, got: %s", resp)
		}
		text := toolText(t, resp)
		if !strings.Contains(text, wantPath) {
			t.Fatalf("tickets.move error does not name the file path %q: %s", wantPath, text)
		}
		if _, err := os.Stat(filepath.Join(root, ticketRel)); err != nil {
			t.Fatalf("refused move must leave the ticket in todo/: %v", err)
		}
	})

	t.Run("worktree.list", func(t *testing.T) {
		root, _ := worktreeFixture(t)
		mustWrite(t, root, ".ws-workflow/config.json", malformed)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920705, root, nil))
		wantPath := filepath.Join(canonicalRootForTest(t, root), ".ws-workflow", "config.json")

		resp := callToolOnce(t, server, 920706, "worktree.list", map[string]any{"session_key": key})
		if !toolIsError(t, resp) {
			t.Fatalf("worktree.list must error on a malformed committed repo file, got: %s", resp)
		}
		text := toolText(t, resp)
		if !strings.Contains(text, wantPath) {
			t.Fatalf("worktree.list error does not name the file path %q: %s", wantPath, text)
		}
		if strings.Contains(text, "worktrees:") {
			t.Fatalf("worktree.list must not report a pool listing (even an empty one) on a config load error: %s", text)
		}
	})
}

// TestConfigTuneEchoesRepoScopeAfterResetAndShadowedSet is Phase 1
// Verification item (viii): after a project-scope reset of
// sage_review_design with a committed repo auto, config.tune's reset echo
// reports auto at scope repo (reading through the session-anchored
// resolver); and a global-scope set under the same committed repo value
// gets the "shadowed: effective ..." line, since repo beats global in the
// precedence chain.
func TestConfigTuneEchoesRepoScopeAfterResetAndShadowedSet(t *testing.T) {
	t.Run("reset_reports_repo_value", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920801, root, nil))

		setResp := callToolWithKey(t, server, 920802, key, "config.tune", map[string]any{
			"key":   wsconfig.ItemSageReviewDesign,
			"value": "ask",
			"scope": "project",
		})
		if !strings.Contains(setResp, "sage_review_design: ask [scope:project]") {
			t.Fatalf("project-scope set response: %s", setResp)
		}

		resetResp := callToolWithKey(t, server, 920803, key, "config.tune", map[string]any{
			"key":   wsconfig.ItemSageReviewDesign,
			"scope": "project",
			"reset": true,
		})
		if !strings.Contains(resetResp, "sage_review_design: auto [scope:repo]") {
			t.Fatalf("reset echo did not report the committed repo value: %s", resetResp)
		}
	})

	t.Run("global_set_shadowed_by_repo", func(t *testing.T) {
		useLeadProfile(t)
		root := t.TempDir()
		mustWrite(t, root, ".ws-workflow/config.json",
			`{"schema_version":1,"overrides":{"sage_review_design":"auto"}}`+"\n")
		initGit(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

		server := NewServer(root, "test")
		key, _ := parseLoginResponse(t, callLogin(t, server, 920804, root, nil))

		resp := callToolWithKey(t, server, 920805, key, "config.tune", map[string]any{
			"key":   wsconfig.ItemSageReviewDesign,
			"value": "ask",
			"scope": "global",
		})
		if !strings.Contains(resp, "sage_review_design: ask [scope:global]") {
			t.Fatalf("global-scope set response missing the write echo: %s", resp)
		}
		if !strings.Contains(resp, "shadowed: effective sage_review_design: auto [scope:repo]") {
			t.Fatalf("global-scope set under a committed repo value missing the shadowed line: %s", resp)
		}
	})
}

// TestConfigListWeightLeverDescriptionsStateValues is Phase 1 Verification
// item (ix): each of the five rewritten weight-lever descriptions
// (sage_review_design, sage_review, review_phase, agents.tier,
// workflow.prefer_subagent) is present in config.list and non-trivially
// states its accepted values — not just a restated knob name. This does not
// pin full sentences: it checks short value-anchored fragments so the prose
// can still be edited. Pre-existing golden tests that check a knob
// description (for example TestConfigTuningCatalogProjectsPromptAndSchemaKnobs
// in prompt_override_test.go, which already asserts on substrings rather
// than full sentences) are unaffected by the rewrite and still pass under
// `go test ./internal/mcp`.
func TestConfigListWeightLeverDescriptionsStateValues(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 920901, root, nil))

	resp := callToolWithKey(t, server, 920902, key, "config.list", map[string]any{"format": "json"})
	var view struct {
		Knobs []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"knobs"`
	}
	if err := json.Unmarshal([]byte(resp), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, resp)
	}
	descriptions := map[string]string{}
	for _, k := range view.Knobs {
		descriptions[k.ID] = k.Description
	}

	cases := []struct {
		id       string
		contains []string
	}{
		{"sage_review_design", []string{"auto requires one design-reviewer pass", "ask asks the user whether to run it", "off skips it"}},
		{"sage_review", []string{"auto requires one completeness-reviewer pass", "ask asks the user whether to run it", "off skips it"}},
		{"review_phase", []string{"lite (builtin) runs", "full runs the risk-keyed allocation", "off runs none"}},
		{"agents.tier", []string{"small, medium, large, xlarge"}},
		{"workflow.prefer_subagent", []string{"on adds a standing line", "off (builtin) leaves"}},
	}
	for _, c := range cases {
		desc, ok := descriptions[c.id]
		if !ok {
			t.Fatalf("config.list missing knob %q", c.id)
		}
		for _, want := range c.contains {
			if !strings.Contains(desc, want) {
				t.Fatalf("%s description missing %q:\n%s", c.id, want, desc)
			}
		}
	}
}
