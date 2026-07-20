package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildTestRsrcTree creates a minimal rsrc tree for testing playbook tools.
// playbooks maps relative paths to file content.
// Returns the root path with a freshly generated manifest.json.
func buildTestRsrcTree(t *testing.T, playbooks map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range playbooks {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
	m, err := wsrsrc.GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := wsrsrc.WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

// newTestServerWithHarness creates a Server bound to a temp root with the given harness.
func newTestServerWithHarness(t *testing.T, harness string) *Server {
	t.Helper()
	s := NewServer(t.TempDir(), "test")
	if harness != "" {
		s.observeHarness("test", harness)
	}
	return s
}

func isolatedPlaybookConfigOptions(t *testing.T) wsconfig.Options {
	t.Helper()
	return wsconfig.Options{
		CacheHome:  filepath.Join(t.TempDir(), "cache"),
		ConfigHome: filepath.Join(t.TempDir(), "config"),
	}
}

func shippedImplementerContext() map[string]string {
	return map[string]string{
		"PlanPath":           "ai-docs/.plans/plan.md",
		"VerificationHint":   "go test ./internal/mcp -run TestRenderPlaybookShippedImplementerDeclaredContext",
		"ResultExpectations": "Report outcome, files changed, commits, verification, and blockers.",
		"CommitRangeHint":    "Report <first-commit>..<last-commit> after committing logical checkpoints.",
	}
}

func shippedImplementerRelayContext() map[string]string {
	return map[string]string{
		"PlanPath":           "ai-docs/.plans/plan.md",
		"ReviewCycle":        "2",
		"CommitRange":        "abc123..def456",
		"ReviewPaths":        "ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"DispositionNotes":   "Fix correctness finding C1; defer test fixture rename until Phase 3.",
		"VerificationHint":   "go test ./internal/mcp -run TestRenderPlaybookShippedImplementerRelayDeclaredContext",
		"ResultExpectations": "Report per-finding dispositions, fix commits, updated range, verification, and blockers.",
	}
}

func shippedPlanPopulatorContext() map[string]string {
	return map[string]string{
		"target_kind":     "ticket",
		"ticket_path":     "ai-docs/tickets/ready/260628-feat-demo.md",
		"selected_phase":  "Phase 2: Rework planner playbooks around ticket-to-plan",
		"inline_contract": "",
		"plan_path":       "ai-docs/.plans/2026-06/28-1200-demo.md",
	}
}

func shippedInlinePlanPopulatorContext() map[string]string {
	return map[string]string{
		"target_kind":     "inline",
		"ticket_path":     "",
		"selected_phase":  "",
		"inline_contract": "Change the bounded renderer path; preserve public behavior; verify focused planner and review tests.",
		"plan_path":       "ai-docs/.plans/2026-06/28-1200-inline.md",
	}
}

// initGitRepo creates a git repository in a temp dir and returns its path.
// Required for renderPlaybook tests since GeneratePaths calls gitIdentity.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", c[1:], err, out)
		}
	}
	return dir
}

// asPlaybookError reports whether err or any error in its chain matches type T.
func asPlaybookError[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}

// writeTestFile writes content to root/relPath, creating parent dirs.
func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// ---------------------------------------------------------------------------
// Fixture playbook content strings
// ---------------------------------------------------------------------------

const (
	// plainPlaybookContent: non-delegate, one custom variable.
	plainPlaybookContent = `---
kind: print
delegates: false
variables:
  - WorktreeID
---
# Plain Playbook

Worktree: {{.WorktreeID}}
`

	// delegatePlaybookContent: delegates:true with all terminology vars.
	// NOTE: kind:render is advisory metadata only — the loader does not restrict
	// by kind, so this fixture is valid for use with printPlaybook too. kind is
	// not a tool-routing gate; it is surfaced in PlaybookMeta for caller inspection.
	delegatePlaybookContent = `---
kind: render
delegates: true
variables:
  - ExploreAgent
  - SpawnIdiom
  - ContinueIdiom
---
# Delegate Playbook

Explore: {{.ExploreAgent}}
Spawn: {{.SpawnIdiom}}
Continue: {{.ContinueIdiom}}
`

	// modelAliasPlaybookContent: declares RoleModel, resolved from the playbook's tier.
	// tier: medium is used so the derivation path is exercised in tests.
	modelAliasPlaybookContent = `---
kind: print
delegates: false
tier: medium
variables:
  - RoleModel
---
# Model Alias Playbook

Model: {{.RoleModel}}
`

	// tierModelPlaybookContent: no declared variables — the four fixed-tier
	// model vars are unconditionally auto-injected (ImplicitVariableNames),
	// so they render without a frontmatter `variables:` declaration.
	tierModelPlaybookContent = `---
kind: print
delegates: false
---
# Tier Model Playbook

Small: {{.SmallTierModel}}
Medium: {{.MediumTierModel}}
Large: {{.LargeTierModel}}
XLarge: {{.XLargeTierModel}}
`

	// noVarsPlaybookContent: no variables, static content.
	noVarsPlaybookContent = `---
kind: print
delegates: false
---
# No-Vars Playbook

Static content only.
`
)

// ---------------------------------------------------------------------------
// playbook.print — golden harness rendering
// ---------------------------------------------------------------------------

func TestPlaybookPrintUnknownHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "") // no harness → host-neutral

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutral := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutral[varName]) {
			t.Errorf("body %q: expected neutral %s %q", body, varName, neutral[varName])
		}
	}
	// Placeholders must be substituted.
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
}

func TestPlaybookPrintClaudeHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Codex terms must NOT appear for any var (proves harness selection on all vars).
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if claudeTerm[varName] != codexTerm[varName] {
			if strings.Contains(body, codexTerm[varName]) {
				t.Errorf("body %q: codex %s term %q must not appear in claude render", body, varName, codexTerm[varName])
			}
		}
	}
}

func TestPlaybookPrintCodexHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
}

// ---------------------------------------------------------------------------
// playbook.print — delegation tip injection
// ---------------------------------------------------------------------------

func TestPlaybookPrintDelegatesTipPresent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
	// Tip must include the claude ContinueIdiom.
	claudeTerm := terminologyForHarness("claude")
	if !strings.Contains(body, claudeTerm["ContinueIdiom"]) {
		t.Errorf("body %q: expected tip to include claude ContinueIdiom %q", body, claudeTerm["ContinueIdiom"])
	}
}

func TestPlaybookPrintDelegatesTipAbsent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{"WorktreeID": "wt-123"}, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.print — caller context substitution
// ---------------------------------------------------------------------------

func TestPlaybookPrintCallerContextSubstituted(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	body, _, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{"WorktreeID": "wt-abc"}, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "wt-abc") {
		t.Errorf("body %q: expected caller context value 'wt-abc' substituted", body)
	}
	if strings.Contains(body, "{{.WorktreeID}}") {
		t.Errorf("body %q: placeholder should have been substituted", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.print — no-vars fast path
// ---------------------------------------------------------------------------

func TestPlaybookPrintNoVarsPlaybook(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"novars/novars.md": noVarsPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	body, _, err := printPlaybook(s, rsrcRoot, "novars", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Static content only") {
		t.Errorf("body %q: expected static content", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.render — writes tmp file, returns path
// ---------------------------------------------------------------------------

func TestPlaybookRenderWritesTmpFile(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	s := newTestServerWithHarness(t, "claude")

	path, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "delegate-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if path == "" {
		t.Fatal("renderPlaybook returned empty path")
	}

	// File must exist and contain the rendered content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	body := string(data)
	claudeTerm := terminologyForHarness("claude")
	if !strings.Contains(body, claudeTerm["ExploreAgent"]) {
		t.Errorf("file body %q: expected claude ExploreAgent %q", body, claudeTerm["ExploreAgent"])
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("file body %q: expected delegation tip", body)
	}
}

// ---------------------------------------------------------------------------
// Model alias — config-sourced resolution (no baked model names)
// ---------------------------------------------------------------------------

func TestPlaybookPrintModelAliasFromConfig(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	// Write a config with a unique, recognizable model name.
	cacheHome := t.TempDir()
	uniqueModel := "test-custom-model-xyz-9999"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheHome}, "core", "custom-backend", uniqueModel, ""); err != nil {
		t.Fatalf("SetAgentsTierForHarness: %v", err)
	}

	// Render using the custom config.
	body, _, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	// The model name must come from config, not be baked in.
	if !strings.Contains(body, uniqueModel) {
		t.Errorf("body %q: expected config-sourced model name %q", body, uniqueModel)
	}
}

// TestPlaybookPrintModelAliasVariesWithConfig verifies that changing the config
// changes the model in the output — proving config-sourced resolution.
func TestPlaybookPrintModelAliasVariesWithConfig(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	cacheA := t.TempDir()
	modelA := "model-variant-aaa"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheA}, "core", "", modelA, ""); err != nil {
		t.Fatalf("config A: %v", err)
	}

	cacheB := t.TempDir()
	modelB := "model-variant-bbb"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheB}, "core", "", modelB, ""); err != nil {
		t.Fatalf("config B: %v", err)
	}

	bodyA, _, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheA}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook A: %v", err)
	}
	bodyB, _, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheB}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook B: %v", err)
	}

	if !strings.Contains(bodyA, modelA) {
		t.Errorf("bodyA %q: expected model %q from config A", bodyA, modelA)
	}
	if !strings.Contains(bodyB, modelB) {
		t.Errorf("bodyB %q: expected model %q from config B", bodyB, modelB)
	}
	if bodyA == bodyB {
		t.Error("different configs produced identical output — model alias resolution not config-driven")
	}
}

// ---------------------------------------------------------------------------
// Fixed-tier model vars — config-sourced resolution (no baked model names)
// ---------------------------------------------------------------------------

// TestPlaybookPrintTierModelVarsFromConfig verifies the four fixed-tier vars
// resolve from config (not baked-in names) and are usable without a
// frontmatter `variables:` declaration (ImplicitVariableNames auto-inject).
func TestPlaybookPrintTierModelVarsFromConfig(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"tier-pb/tier-pb.md": tierModelPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	cacheHome := t.TempDir()
	tierModels := map[string]string{
		"small":  "test-small-model-1111",
		"medium": "test-medium-model-2222",
		"large":  "test-large-model-3333",
		"xlarge": "test-xlarge-model-4444",
	}
	for tier, model := range tierModels {
		if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheHome}, tier, "custom-backend", model, ""); err != nil {
			t.Fatalf("SetAgentsTierForHarness(%s): %v", tier, err)
		}
	}

	body, _, err := printPlaybook(s, rsrcRoot, "tier-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for tier, model := range tierModels {
		if !strings.Contains(body, model) {
			t.Errorf("body %q: expected config-sourced %s-tier model %q", body, tier, model)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
}

// TestPlaybookPrintTierModelVarsFallbackOnResolverError verifies that when
// ResolveAgentForHarnessConfig errors (here, via a malformed config.json),
// each fixed-tier var falls back to a stable "the <tier>-tier model" label
// instead of rendering empty — these vars sit mid-sentence in prose, so an
// empty slot would read as a rendering bug.
func TestPlaybookPrintTierModelVarsFallbackOnResolverError(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"tier-pb/tier-pb.md": tierModelPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	cacheHome := t.TempDir()
	// A malformed config.json forces wsconfig.Load (and therefore
	// ResolveAgentForHarnessConfig) into its error path.
	if err := os.WriteFile(filepath.Join(cacheHome, "config.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write malformed config.json: %v", err)
	}

	body, _, err := printPlaybook(s, rsrcRoot, "tier-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, tier := range []string{"small", "medium", "large", "xlarge"} {
		want := "the " + tier + "-tier model"
		if !strings.Contains(body, want) {
			t.Errorf("body %q: expected fallback label %q on resolver error", body, want)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
}

// TestPlaybookPrintGoldenLeadWorkflowManualScopedExplorationTierModels renders
// the real lead-workflow-manual under both claude and codex harness contexts
// with an isolated (default) config and verifies the Scoped Exploration
// sentence materializes the correct per-harness default small/medium models
// with no {{. placeholder remaining — the ticket's stated verification
// boundary. Default config: claude small=haiku medium=sonnet; codex
// small=gpt-5.6-luna medium=gpt-5.6-terra (post 9bfe7aa3 tier-default remap).
func TestPlaybookPrintGoldenLeadWorkflowManualScopedExplorationTierModels(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")

	cases := []struct {
		harness     string
		smallModel  string
		mediumModel string
	}{
		{"claude", "haiku", "sonnet"},
		{"codex", "gpt-5.6-luna", "gpt-5.6-terra"},
	}
	for _, tc := range cases {
		t.Run(tc.harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, tc.harness)
			body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			exploreAgent := terminologyForHarness(tc.harness)["ExploreAgent"]
			wantSentence := "dispatch\n" + exploreAgent + " as " + tc.smallModel +
				" by default; escalate to\n" + tc.mediumModel
			if !strings.Contains(body, wantSentence) {
				t.Errorf("body %q: expected Scoped Exploration sentence to contain %q", body, wantSentence)
			}
			if strings.Contains(body, "{{.") {
				t.Errorf("body %q: unsubstituted placeholder remains", body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Loud failure paths
// ---------------------------------------------------------------------------

func TestPlaybookPrintMissingManifest(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	// No manifest written.

	s := newTestServerWithHarness(t, "")
	_, _, err := printPlaybook(s, root, "pb", nil, wsconfig.Options{}, "", nil)
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
	var missing wsrsrc.ErrManifestMissing
	if !asPlaybookError(err, &missing) {
		t.Errorf("expected ErrManifestMissing, got %T: %v", err, err)
	}
}

func TestPlaybookPrintSchemaMismatch(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	writeTestFile(t, root, "manifest.json", `{"schema_version":999,"files":{"pb/pb.md":"deadbeef"}}`)

	s := newTestServerWithHarness(t, "")
	_, _, err := printPlaybook(s, root, "pb", nil, wsconfig.Options{}, "", nil)
	if err == nil {
		t.Fatal("expected error for schema mismatch, got nil")
	}
	var mismatch wsrsrc.ErrSchemaMismatch
	if !asPlaybookError(err, &mismatch) {
		t.Errorf("expected ErrSchemaMismatch, got %T: %v", err, err)
	}
}

func TestPlaybookPrintUndeclaredCallerVar(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	_, _, err := printPlaybook(s, rsrcRoot, "plain-pb",
		map[string]string{"WorktreeID": "wt", "Undeclared": "oops"},
		wsconfig.Options{}, "", nil)
	if err == nil {
		t.Fatal("expected ErrUndeclaredVar for undeclared caller var, got nil")
	}
	var undecl wsrsrc.ErrUndeclaredVar
	if !asPlaybookError(err, &undecl) {
		t.Errorf("expected ErrUndeclaredVar, got %T: %v", err, err)
	}
	if undecl.Name != "Undeclared" {
		t.Errorf("ErrUndeclaredVar.Name = %q, want Undeclared", undecl.Name)
	}
}

func TestPlaybookPrintUnprovidedVar(t *testing.T) {
	// WorktreeID is declared and used in body but neither caller nor tool provides it.
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	_, _, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{}, wsconfig.Options{}, "", nil)
	if err == nil {
		t.Fatal("expected ErrUnprovidedVar for missing required var, got nil")
	}
	var unprov wsrsrc.ErrUnprovidedVar
	if !asPlaybookError(err, &unprov) {
		t.Errorf("expected ErrUnprovidedVar, got %T: %v", err, err)
	}
	if unprov.Name != "WorktreeID" {
		t.Errorf("ErrUnprovidedVar.Name = %q, want WorktreeID", unprov.Name)
	}
}

func TestPlaybookPrintDanglingInclude(t *testing.T) {
	// Playbook declares includes: [dangling] but dangling.md is not in the tree.
	// The include resolution should fail and propagate the error through printPlaybook.
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		// Only the playbook file; dangling.md is intentionally absent so the
		// manifest will not list it, causing ErrFileMissing from resolveIncludes.
		"dangle-pb/dangle-pb.md": "---\nkind: print\ndelegates: false\nincludes:\n  - dangling\n---\nbody\n",
	})
	s := newTestServerWithHarness(t, "")

	_, _, err := printPlaybook(s, rsrcRoot, "dangle-pb", nil, wsconfig.Options{}, "", nil)
	if err == nil {
		t.Fatal("expected error for dangling include, got nil")
	}
	// The error message must contain the missing include stem name.
	if !strings.Contains(err.Error(), "dangling") {
		t.Errorf("error %q: expected include stem 'dangling' in message", err)
	}
	// The wrapped underlying error must be ErrFileMissing (the manifest does not
	// list dangling.md since it was never written to the tree).
	var fileMissing wsrsrc.ErrFileMissing
	if !asPlaybookError(err, &fileMissing) {
		t.Errorf("expected ErrFileMissing (via errors.As), got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// MCP dispatch: tool surface (no wsflow gate, no no-agent gate)
// ---------------------------------------------------------------------------

func TestPlaybookToolsInLeadToolNames(t *testing.T) {
	names := LeadToolNames()
	has := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has("playbook.print") {
		t.Error("playbook.print missing from LeadToolNames")
	}
	if !has("playbook.render") {
		t.Error("playbook.render missing from LeadToolNames")
	}
}

func TestPlaybookToolsNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("playbook.print") {
		t.Error("playbook.print is incorrectly hidden in no-agent mode")
	}
	if noAgentHiddenTool("playbook.render") {
		t.Error("playbook.render is incorrectly hidden in no-agent mode")
	}
}

func TestPlaybookToolsVisibleInToolsList(t *testing.T) {
	listed := map[string]bool{}
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		listed[name] = true
	}
	for _, want := range []string{"playbook.print", "playbook.render"} {
		if !listed[want] {
			t.Errorf("tool %q missing from tools() list", want)
		}
	}
}

func TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_SKILLS_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "skills"))
	s := newTestServerWithHarness(t, "codex")
	configOpts := isolatedPlaybookConfigOptions(t)

	assertCleanWsflowManual := func(label, body string) {
		t.Helper()
		for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "ws.mercenary.", "exec.", "Full ws", "full ws", "ws:override:", "ws:/override:"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s: wsflow playbook output contains forbidden %q:\n%s", label, forbidden, body)
			}
		}
		// Exclude HTML comment lines (e.g. <!-- ws:fresh-only:start -->): these
		// inert Markdown marker tokens are not namespace notation, so filtering
		// them avoids false positives on the workflow-manual fresh-only markers.
		bodyLinesWithoutComments := strings.Join(func() []string {
			var out []string
			for _, line := range strings.Split(body, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "<!--") {
					out = append(out, line)
				}
			}
			return out
		}(), "\n")
		if regexp.MustCompile(`\bws[/:]`).MatchString(bodyLinesWithoutComments) {
			t.Fatalf("%s: wsflow playbook output contains bare ws namespace notation:\n%s", label, body)
		}
		if strings.Contains(body, "{{.") {
			t.Fatalf("%s: wsflow playbook output contains unsubstituted placeholder:\n%s", label, body)
		}
		for _, want := range []string{"wsflow/", "wsflow:", "wsflow runtime"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: wsflow playbook output missing %q:\n%s", label, want, body)
			}
		}
		if !strings.Contains(body, "ferrule") {
			t.Fatalf("%s: wsflow playbook output rewrote literal ws.ferrule tool name:\n%s", label, body)
		}
	}

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, configOpts, "", buildOverrideLookup(s, ""))
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent off", body)
	if strings.Contains(body, `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`) {
		t.Fatalf("wsflow workflow manual must not append lead-prefer-subagent while preference is off:\n%s", body)
	}

	resolver := wsconfig.NewResolver(configOpts, builtinConfigDefaults(), nil, nil)
	if err := resolver.Set(wsconfig.ItemWorkflowPreferSubagent, "on", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("enable workflow.prefer_subagent: %v", err)
	}
	bodyOn, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, configOpts, "", buildOverrideLookup(s, ""))
	if err != nil {
		t.Fatalf("printPlaybook on: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent on", bodyOn)
	for _, want := range []string{
		`<playbook name="lead-prefer-subagent" title="Prefer Subagent">`,
		"Maximum-delegation posture for this session",
		"spawn_agent(fork_context:true, message:<prompt>)",
	} {
		if !strings.Contains(bodyOn, want) {
			t.Fatalf("wsflow workflow manual with prefer-subagent on missing %q:\n%s", want, bodyOn)
		}
	}
}

func TestPlaybookPrintLeadTuneUsesWorkflowPreferenceCatalogKnobs(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune: %v", err)
	}
	for _, want := range []string{
		`ws/config.tuning(session_key: <lead key>)`,
		`"workflow.prefer_subagent"`,
		"catalog-provided writer for `\"workflow.prefer_subagent\"`",
		`"workflow.prefer_mercenary"`,
		"catalog-provided writer for `\"workflow.prefer_mercenary\"`",
		"prompt.UserPreferenceSection",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("lead-tune render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"Call `config.workflow_prefer_subagent`",
		"Call `config.workflow_prefer_mercenary`",
		"prompt.DelegationSection",
		"DelegationSection",
		"delegation.prefer_mercenary",
		"ws.lead.prefer_mercenary",
		"session-scoped",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("lead-tune render contains stale guidance %q:\n%s", forbidden, body)
		}
	}
}

func TestPlaybookPrintWsflowLeadTuneOmitsFullWsOnlyCatalogKnobs(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune wsflow: %v", err)
	}
	for _, want := range []string{
		`wsflow/config.tuning(session_key: <lead key>)`,
		"wsflow workflow",
		`"workflow.prefer_subagent"`,
		"catalog-provided writer for `\"workflow.prefer_subagent\"`",
		"prompt.UserPreferenceSection",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wsflow lead-tune render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		`"workflow.prefer_mercenary"`,
		"config.workflow_prefer_mercenary",
		"delegation.prefer_mercenary",
		"agents.tier",
		"config.agents_tier",
		"ws.mercenary.",
		"Full ws",
		"full ws",
		"ws:override:",
		"ws:/override:",
		"{{.",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("wsflow lead-tune render contains forbidden %q:\n%s", forbidden, body)
		}
	}
	if regexp.MustCompile(`\bws[/:]`).MatchString(body) {
		t.Fatalf("wsflow lead-tune render contains bare ws namespace notation:\n%s", body)
	}
}

func TestProductModeBlockSelection(t *testing.T) {
	t.Setenv(envNamespace, "wsflow")
	input := strings.Join([]string{
		"shared text",
		fullOnlyStart,
		"full-only text",
		fullOnlyEnd,
		wsflowOnlyStart,
		"wsflow-only text",
		wsflowOnlyEnd,
	}, "\n")

	t.Setenv(envNoAgent, "1")
	wsflow := renderProductModePlaybookBody(input, false)
	for _, forbidden := range []string{"full-only text", fullOnlyStart, wsflowOnlyStart} {
		if strings.Contains(wsflow, forbidden) {
			t.Fatalf("wsflow render contains forbidden %q:\n%s", forbidden, wsflow)
		}
	}
	for _, want := range []string{"shared text", "wsflow-only text"} {
		if !strings.Contains(wsflow, want) {
			t.Fatalf("wsflow render missing %q:\n%s", want, wsflow)
		}
	}

	t.Setenv(envNoAgent, "")
	full := renderProductModePlaybookBody(input, true)
	if strings.Contains(full, "wsflow-only text") || strings.Contains(full, fullOnlyStart) || strings.Contains(full, wsflowOnlyStart) {
		t.Fatalf("full render kept wsflow-only text or marker comments:\n%s", full)
	}
	if !strings.Contains(full, "full-only text") {
		t.Fatalf("full render omitted full-only content:\n%s", full)
	}
}

func TestReservedNamespaceVarsDoNotRequireFrontmatter(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"namespace-pb/namespace-pb.md": `---
kind: print
delegates: false
---
Call {{.McpNamespace}}/tickets.find and {{.SkillNamespace}}:lead-discuss.
Actual tool: ws.ferrule.
`,
	})
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "namespace-pb", map[string]string{
		"McpNamespace":   "spoof",
		"SkillNamespace": "spoof",
	}, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"wsflow/tickets.find", "wsflow:lead-discuss", "ferrule"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "spoof") {
		t.Fatalf("caller context overrode reserved namespace vars:\n%s", body)
	}
}

func TestPlaybookReservedNamespaceVarsFullWs(t *testing.T) {
	t.Setenv(envNamespace, "")
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"namespace-pb/namespace-pb.md": `---
kind: print
delegates: false
---
Call {{.McpNamespace}}/tickets.find and {{.SkillNamespace}}:lead-discuss.
`,
	})
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "namespace-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"ws/tickets.find", "ws:lead-discuss"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, body)
		}
	}
}

func TestRenderPlaybookWsflowProductModeUsesShippedDelegate(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("implementer recommended tier = %q, want medium", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "Continuity tip") {
		t.Fatalf("rendered implementer output must not include delegation continuity tip:\n%s", body)
	}
	for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "Mercenary path", "ws.mercenary.", "exec.", "showsflow", "knowsflow", "followsflow", "workflowsflow"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("rendered wsflow delegate contains forbidden %q:\n%s", forbidden, body)
		}
	}
	if regexp.MustCompile(`\bws[/:]`).MatchString(body) {
		t.Fatalf("rendered wsflow delegate contains bare ws namespace notation:\n%s", body)
	}
}

func TestRenderPlaybookShippedImplementerDeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("implementer recommended tier = %q, want medium", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"Alias model for this role: gpt-5.6-terra.",
		"Plan path: `ai-docs/.plans/plan.md`",
		"Verification instructions: go test ./internal/mcp -run TestRenderPlaybookShippedImplementerDeclaredContext",
		"Binding result expectations: Report outcome, files changed, commits, verification, and blockers.",
		"Commit-range reporting requirement: Report <first-commit>..<last-commit> after committing logical checkpoints.",
		"The plan and its listed references are the task contract.",
		"Read the plan path above and all `[Must]` References listed in the plan except ticket files.",
		"Do not read ticket files directly unless the plan's `Escalations` section explicitly authorizes ticket-file reading.",
		"ask the caller to update the plan's `Escalations` section unless that section already authorizes ticket-file reading",
		"Satisfy `ResultExpectations`; it is binding output scope, not advisory text.",
		"Normal completion report:",
		"If `ResultExpectations` names an output file, also include its path plus a short completion summary.",
		"Always include final commit hash and commit range, or `none` with reason.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("implementer render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"BriefPath",
		"Brief path:",
		"No-plan sentinel",
		"brief or plan",
		"plan or brief",
		"Do not read ticket files directly, even when a ticket path appears",
		"caller explicitly authorizes ticket-file reading",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("implementer render retained old brief/implicit-ticket contract %q:\n%s", forbidden, body)
		}
	}
}

func TestRenderPlaybookShippedImplementerRelayDeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", shippedImplementerRelayContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("implementer-relay recommended tier = %q, want medium", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"Alias model for this role: gpt-5.6-terra.",
		"Plan path: `ai-docs/.plans/plan.md`",
		"Review cycle: 2",
		"Current commit range: abc123..def456",
		"Non-clean review paths: ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"Lead disposition notes: Fix correctness finding C1; defer test fixture rename until Phase 3.",
		"Verification instructions: go test ./internal/mcp -run TestRenderPlaybookShippedImplementerRelayDeclaredContext",
		"Result expectations: Report per-finding dispositions, fix commits, updated range, verification, and blockers.",
		"Rely only on this prompt and named paths; do not depend on prior conversation.",
		"Read the plan and every non-clean review path directly.",
		"Won't-fix is allowed only for style suggestions conflicting with local patterns, findings that require scope expansion beyond the plan, or findings disproven by specific evidence.",
		"Do not read ticket files directly unless the plan's `Escalations` section explicitly authorizes ticket-file reading.",
		"escalate for a plan update if a required fix needs ticket material or a plan deviation.",
		"Won't-fix is not allowed for correctness, security, contract, regression, or required-test violations.",
		"records the relevant per-finding dispositions known at that checkpoint",
		"`[fixed]`",
		"`[won't fix: <reason>]`",
		"`[deferred: <reason>]`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("implementer-relay render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"BriefPath",
		"Brief path:",
		"No-plan sentinel",
		"Read the brief",
		"scope expansion beyond the brief",
		"findings, or disposition notes explicitly authorize ticket-file reading",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("implementer-relay render retained old brief contract %q:\n%s", forbidden, body)
		}
	}
	if strings.Contains(body, "Continuity tip") {
		t.Fatalf("implementer-relay render must not include delegation continuity tip:\n%s", body)
	}
}

func TestRenderPlaybookWsflowLegacyPromptStemsAppendContext(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	codeReviewerPath, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "code-reviewer", map[string]string{
		"note": "see ws/specs.find for details",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook code-reviewer with legacy context: %v", err)
	}
	codeReviewerData, err := os.ReadFile(codeReviewerPath)
	if err != nil {
		t.Fatalf("read code-reviewer render: %v", err)
	}
	codeReviewerBody := string(codeReviewerData)
	for _, want := range []string{"wsflow/", "## Render Context", "- note: see ws/specs.find for details"} {
		if !strings.Contains(codeReviewerBody, want) {
			t.Fatalf("code-reviewer render missing %q:\n%s", want, codeReviewerBody)
		}
	}

	planContext := shippedPlanPopulatorContext()
	planContext["note"] = "legacy extra context"
	planPath, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "plan-populator-survey", planContext, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook plan-populator-survey with legacy context: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("plan-populator-survey tier = %q, want medium", tier)
	}
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan-populator-survey render: %v", err)
	}
	planBody := string(planData)
	for _, want := range []string{
		"## Render Context",
		"- note: legacy extra context",
		"- Target kind: `ticket`",
		"- Ticket path: `ai-docs/tickets/ready/260628-feat-demo.md`",
		"- Selected phase: `Phase 2: Rework planner playbooks around ticket-to-plan`",
		"- Plan path: `ai-docs/.plans/2026-06/28-1200-demo.md`",
		"## Relevant Ticket Contract",
		"## Out of Scope",
		"## Codebase Findings",
		"## Implementation Plan",
		"## Verification Plan",
		"## Escalations",
		"[escalate-to-research]",
		"Confidence: `<high|medium|low>`",
	} {
		if !strings.Contains(planBody, want) {
			t.Fatalf("plan-populator-survey render missing %q:\n%s", want, planBody)
		}
	}
	for _, forbidden := range []string{"- brief_path:", "brief path", "Brief path", "Mercenary path", "ws.mercenary.", "exec."} {
		if strings.Contains(planBody, forbidden) {
			t.Fatalf("plan-populator-survey wsflow render contains forbidden %q:\n%s", forbidden, planBody)
		}
	}

	researchContext := shippedPlanPopulatorContext()
	researchContext["note"] = "legacy extra context"
	researchPath, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "plan-populator-research", researchContext, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook plan-populator-research with legacy context: %v", err)
	}
	if tier != "large" {
		t.Fatalf("plan-populator-research tier = %q, want large", tier)
	}
	researchData, err := os.ReadFile(researchPath)
	if err != nil {
		t.Fatalf("read plan-populator-research render: %v", err)
	}
	researchBody := string(researchData)
	for _, want := range []string{
		"## Render Context",
		"- note: legacy extra context",
		"- Target kind: `ticket`",
		"- Ticket path: `ai-docs/tickets/ready/260628-feat-demo.md`",
		"- Selected phase: `Phase 2: Rework planner playbooks around ticket-to-plan`",
		"- Plan path: `ai-docs/.plans/2026-06/28-1200-demo.md`",
		"If `ai-docs/.plans/2026-06/28-1200-demo.md` already contains survey output, read it before replacing or",
		"## Relevant Ticket Contract",
		"## Out of Scope",
		"## Codebase Findings",
		"## Implementation Plan",
		"## Verification Plan",
		"## Escalations",
	} {
		if !strings.Contains(researchBody, want) {
			t.Fatalf("plan-populator-research render missing %q:\n%s", want, researchBody)
		}
	}
	for _, forbidden := range []string{"- brief_path:", "brief path", "Brief path"} {
		if strings.Contains(researchBody, forbidden) {
			t.Fatalf("plan-populator-research wsflow render contains forbidden %q:\n%s", forbidden, researchBody)
		}
	}
}

func TestRenderPlaybookFullWsPlannerContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	assertPlanner := func(name, wantTier string, wants []string) {
		t.Helper()
		path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, name, shippedPlanPopulatorContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
		if err != nil {
			t.Fatalf("renderPlaybook %s with declared planner context: %v", name, err)
		}
		if tier != wantTier {
			t.Fatalf("%s tier = %q, want %s", name, tier, wantTier)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s render: %v", name, err)
		}
		body := string(data)
		commonWants := []string{
			"- Ticket path: `ai-docs/tickets/ready/260628-feat-demo.md`",
			"- Selected phase: `Phase 2: Rework planner playbooks around ticket-to-plan`",
			"- Plan path: `ai-docs/.plans/2026-06/28-1200-demo.md`",
			"## Relevant Ticket Contract",
			"## Out of Scope",
			"## Codebase Findings",
			"## Implementation Plan",
			"## Verification Plan",
			"## Escalations",
		}
		for _, want := range append(commonWants, wants...) {
			if !strings.Contains(body, want) {
				t.Fatalf("%s full ws render missing %q:\n%s", name, want, body)
			}
		}
		for _, forbidden := range []string{"brief_path", "BriefPath", "brief path", "Brief path"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s full ws render retained brief dependency %q:\n%s", name, forbidden, body)
			}
		}
	}

	assertPlanner("plan-populator-survey", "medium", []string{
		"[ok]` or `[escalate-to-research]`",
		"Confidence: `<high|medium|low>`",
		"Escalation rationale when returning `[escalate-to-research]`",
	})
	assertPlanner("plan-populator-research", "large", []string{
		"[ok]` or `[escalate-to-lead]`",
		"Include `None` when no blocker remains. Otherwise include the blocker,",
	})

	for _, name := range []string{"plan-populator-survey", "plan-populator-research"} {
		path, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, name, shippedInlinePlanPopulatorContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil)
		if err != nil {
			t.Fatalf("renderPlaybook %s with inline authority: %v", name, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s inline render: %v", name, err)
		}
		body := string(data)
		for _, want := range []string{
			"- Target kind: `inline`",
			"- Inline contract: `Change the bounded renderer path; preserve public behavior; verify focused planner and review tests.`",
			"for `inline`, use `Change the bounded renderer path; preserve public behavior; verify focused planner and review tests.` and do not read a ticket",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s inline render missing %q:\n%s", name, want, body)
			}
		}
		for _, forbidden := range []string{"Read the ticket at ``"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s inline render requires fake ticket authority %q:\n%s", name, forbidden, body)
			}
		}
	}

	ctx := shippedPlanPopulatorContext()
	ctx["brief_path"] = "ai-docs/.plans/legacy-brief.md"
	for _, name := range []string{"plan-populator-survey", "plan-populator-research"} {
		if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, name, ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
			t.Fatalf("full ws renderPlaybook accepted brief_path for %s", name)
		} else {
			var undeclared wsrsrc.ErrUndeclaredVar
			if !errors.As(err, &undeclared) || undeclared.Name != "brief_path" {
				t.Fatalf("%s brief_path error = %T %v, want ErrUndeclaredVar brief_path", name, err, err)
			}
		}
	}
}

func TestRenderPlaybookFullWsStillRejectsUndeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "code-reviewer", map[string]string{
		"note": "ordinary full ws context remains template vars",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for code-reviewer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	ctx := shippedImplementerContext()
	ctx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	implCtx := shippedImplementerContext()
	implCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", implCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
		}
	}

	relayBriefCtx := shippedImplementerRelayContext()
	relayBriefCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", relayBriefCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer-relay")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer-relay BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
		}
	}

	relayCtx := shippedImplementerRelayContext()
	relayCtx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", relayCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer-relay")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer-relay renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}
}

func TestRenderPlaybookWsflowNonLegacyStemRejectsUndeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", map[string]string{
		"note": "wsflow non-legacy stems still require declared template vars",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("wsflow non-legacy renderPlaybook accepted undeclared context")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("wsflow non-legacy renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", map[string]string{
		"note": "implementer-relay is not a wsflow legacy freeform stem",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", false, "", nil); err == nil {
		t.Fatal("wsflow non-legacy renderPlaybook accepted undeclared implementer-relay context")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("wsflow implementer-relay renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}
}

func TestPlaybookToolsSchemaNameRequired(t *testing.T) {
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		if name != "playbook.print" && name != "playbook.render" {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		required, _ := schema["required"].([]string)
		found := false
		for _, r := range required {
			if r == "name" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q schema: 'name' not in required %v", name, required)
		}
	}
}

// ---------------------------------------------------------------------------
// MCP dispatch: end-to-end via callTool
// ---------------------------------------------------------------------------

func TestPlaybookPrintMCPDispatch(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"novars/novars.md": noVarsPlaybookContent,
	})
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	srv := NewServer(t.TempDir(), "test")
	// req.Params is the JSON for the tools/call params object:
	// {"name": "<tool-name>", "arguments": {...}}
	reqParams, _ := json.Marshal(map[string]any{
		"name": "playbook.print",
		"arguments": map[string]any{
			"name": "novars",
		},
	})
	req := request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  reqParams,
	}
	resp := srv.callTool(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("callTool error: %v", resp.Error.Message)
	}
	result, _ := resp.Result.(map[string]any)
	if result["isError"] == true {
		if content, ok := result["content"].([]map[string]string); ok && len(content) > 0 {
			t.Fatalf("callTool isError: %s", content[0]["text"])
		}
		t.Fatal("callTool returned isError")
	}
	content, _ := result["content"].([]map[string]string)
	if len(content) == 0 {
		t.Fatal("callTool returned no content")
	}
	if !strings.Contains(content[0]["text"], "Static content only") {
		t.Errorf("callTool result %q: expected playbook content", content[0]["text"])
	}
}

// ---------------------------------------------------------------------------
// Golden render: real agents-plugin/rsrc tree
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenDelegateSampleClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("golden body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Hardcoded expected strings to guard against a wrong terminology table
	// (both sides of a derived assertion would agree even if the table were wrong).
	if !strings.Contains(body, "the Explore agent") {
		t.Errorf("golden body %q: expected hardcoded claude ExploreAgent 'the Explore agent'", body)
	}
	if !strings.Contains(body, "SendMessage(to: <agentId>)") {
		t.Errorf("golden body %q: expected hardcoded claude ContinueIdiom 'SendMessage(to: <agentId>)'", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenDelegateSampleCodexHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "an explorer subagent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'an explorer subagent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its task id") {
		t.Errorf("golden body %q: expected hardcoded codex ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenDelegateSampleUnknownHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "") // host-neutral

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q", body, varName, neutralTerm[varName])
		}
	}
}

func TestPlaybookPrintGoldenSamplePlaybookNoDelegation(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "sample-playbook",
		map[string]string{"WorktreeID": "wt-golden"},
		wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "wt-golden") {
		t.Errorf("golden body %q: expected WorktreeID substituted", body)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: delegation tip must not appear for delegates:false", body)
	}
}

// ---------------------------------------------------------------------------
// Golden render: fallback explore playbook (real rsrc tree)
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenExploreClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("golden body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Hardcoded expected strings to guard against a wrong terminology table.
	if !strings.Contains(body, "the Explore agent") {
		t.Errorf("golden body %q: expected hardcoded claude ExploreAgent 'the Explore agent'", body)
	}
	if !strings.Contains(body, "SendMessage(to: <agentId>)") {
		t.Errorf("golden body %q: expected hardcoded claude ContinueIdiom 'SendMessage(to: <agentId>)'", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreCodexHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "an explorer subagent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'an explorer subagent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its task id") {
		t.Errorf("golden body %q: expected hardcoded codex ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreUnknownHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "") // host-neutral

	body, _, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q", body, varName, neutralTerm[varName])
		}
	}
	// Hardcoded expected strings.
	if !strings.Contains(body, "an exploration agent") {
		t.Errorf("golden body %q: expected hardcoded neutral ExploreAgent 'an exploration agent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its returned id") {
		t.Errorf("golden body %q: expected hardcoded neutral ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreJunkHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "junk-harness-xyz") // unrecognized → neutral

	body, _, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Unrecognized harness falls back to host-neutral table.
	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q for junk harness", body, varName, neutralTerm[varName])
		}
	}
	// Hardcoded literals for anti-tautology: guards against a wrong neutral table
	// producing a false-positive derived pass (same strings as Unknown harness test).
	if !strings.Contains(body, "an exploration agent") {
		t.Errorf("golden body %q: expected hardcoded neutral ExploreAgent 'an exploration agent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its returned id") {
		t.Errorf("golden body %q: expected hardcoded neutral ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

// ---------------------------------------------------------------------------
// Terminology table coverage assertions
// ---------------------------------------------------------------------------

func TestTerminologyTableCoverage(t *testing.T) {
	for _, harness := range []string{"claude", "codex", ""} {
		tbl, ok := playbookTerminologyTable[harness]
		if !ok {
			t.Errorf("terminology table missing harness entry %q", harness)
			continue
		}
		for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
			v, ok := tbl[varName]
			if !ok || v == "" {
				t.Errorf("terminology[%q][%q] = %q, want non-empty", harness, varName, v)
			}
		}
	}
}

// TestTermsDifferThreeWay asserts three-way distinctness (claude ≠ codex ≠ neutral)
// for each terminology variable, so a copy-paste collapse in the neutral table
// does not go undetected even when tautological golden tests pass.
func TestTermsDifferThreeWay(t *testing.T) {
	claude := terminologyForHarness("claude")
	codex := terminologyForHarness("codex")
	neutral := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if claude[varName] == codex[varName] {
			t.Errorf("claude == codex for %q: %q — update terminology table", varName, claude[varName])
		}
		if neutral[varName] == claude[varName] {
			t.Errorf("neutral == claude for %q: %q — update terminology table", varName, neutral[varName])
		}
		if neutral[varName] == codex[varName] {
			t.Errorf("neutral == codex for %q: %q — update terminology table", varName, neutral[varName])
		}
	}
}

func TestReservedToolVarNamesContainsRequiredNames(t *testing.T) {
	for _, name := range []string{
		"ExploreAgent", "SpawnIdiom", "ContinueIdiom", "RoleModel", "McpNamespace", "SkillNamespace",
		"SmallTierModel", "MediumTierModel", "LargeTierModel", "XLargeTierModel",
	} {
		if !reservedToolVarNames[name] {
			t.Errorf("reservedToolVarNames missing %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Golden print: migrated internal procedure playbooks (real rsrc tree)
// ---------------------------------------------------------------------------

// TestPlaybookPrintGoldenLeadCheckBlockers verifies lead-check-blockers
// resolves from the real rsrc tree and contains procedure body text.
func TestPlaybookPrintGoldenLeadCheckBlockers(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-check-blockers", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	// Verify non-trivial procedure text is present.
	if !strings.Contains(body, "user-blocking design questions") {
		t.Errorf("body %q: expected procedure text 'user-blocking design questions'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestSkillBodyGoldenLeadVerifyDiscussion verifies lead-verify-discussion
// resolves from the real skills tree as a static inlined SKILL.md body.
// lead-verify-discussion is no longer a playbook.print-backed rsrc playbook
// (its procedure body was inlined directly into SKILL.md), so this reads
// through wsrsrc.LoadSkillBody rather than printPlaybook. The former
// delegates:true continuity-tip and mercenary-path paragraphs were removed
// (see 864902a3): they were a poor fit for this checkpoint's conditional
// delegation, and their removal makes the source eligible for
// substitution-mirrored wsflow generation (no product-specific content).
func TestSkillBodyGoldenLeadVerifyDiscussion(t *testing.T) {
	skillsRoot := filepath.Join("..", "..", "..", "agents-plugin", "skills")

	body, err := wsrsrc.LoadSkillBody(skillsRoot, "lead-verify-discussion")
	if err != nil {
		t.Fatalf("LoadSkillBody: %v", err)
	}
	if !strings.Contains(body, "Re-objectify the discussion") {
		t.Errorf("body %q: expected procedure text 'Re-objectify the discussion'", body)
	}
	if strings.Contains(body, "mercenary") {
		t.Errorf("body %q: must not contain mercenary-path content (removed in 864902a3)", body)
	}
}

// TestPlaybookPrintGoldenLeadUpdateSpec verifies lead-update-spec resolves
// and contains the updated rsrc path reference.
func TestPlaybookPrintGoldenLeadUpdateSpec(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-update-spec", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "spec coverage at commit boundaries") {
		t.Errorf("body %q: expected doctrine text 'spec coverage at commit boundaries'", body)
	}
	// Verify the dead-path fix: updated rsrc path, not old SKILL.md path.
	if !strings.Contains(body, "agents-plugin/rsrc/lead-write-spec/lead-write-spec.md") {
		t.Errorf("body %q: expected updated rsrc path reference", body)
	}
	if strings.Contains(body, "agents-plugin/skills/lead-write-spec/SKILL.md") {
		t.Errorf("body %q: must not contain stale SKILL.md path reference", body)
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadWorkflowManual verifies lead-workflow-manual resolves
// and contains the updated self-reinvoke instruction.
func TestPlaybookPrintGoldenLeadWorkflowManual(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "WS Workflow Primitives") {
		t.Errorf("body %q: expected heading 'WS Workflow Primitives'", body)
	}
	// Verify the dead-path fix: self-reinvoke uses playbook.print, not ws:lead-workflow-manual.
	if !strings.Contains(body, `ws/playbook.print(name: "lead-workflow-manual")`) {
		t.Errorf("body %q: expected updated self-reinvoke instruction using playbook.print", body)
	}
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadWriteSpec verifies lead-write-spec resolves
// and is delegates:true (tip must appear).
func TestPlaybookPrintGoldenLeadWriteSpec(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-write-spec", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "behavioral drift resistance") {
		t.Errorf("body %q: expected doctrine text 'behavioral drift resistance'", body)
	}
	// delegates:true (conditional Explore accuracy check) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadWriteTicket verifies lead-write-ticket resolves
// and delegates:false (no tip).
func TestPlaybookPrintGoldenLeadWriteTicket(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-write-ticket", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "recoverability of intent") {
		t.Errorf("body %q: expected doctrine text 'recoverability of intent'", body)
	}
	if !strings.Contains(body, `tickets.create(session_key: <lead key>, stem: "<category>-<name>", initial_state: "<initial-status>")`) {
		t.Errorf("body missing tickets.create public schema call:\n%s", body)
	}
	for _, want := range []string{
		"If posture is `recommended`, ask the user",
		"If posture is `required`, run design review without asking",
		"add `sage-review-design: skipped`",
		"add or update `sage-review-design: completed`",
		"add or update `sage-review-design: blocked`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing sage review gate language %q:\n%s", want, body)
		}
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadImplement verifies lead-implement resolves
// and is delegates:true (tip must appear).
func TestPlaybookPrintGoldenLeadImplement(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-implement", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "execution attention") {
		t.Errorf("body %q: expected doctrine text 'execution attention'", body)
	}
	for _, want := range []string{
		"Gather `target`, `facts`, and explicit caller `policy` for `ws/enter.implement`",
		"For tickets, use the ticket description only; for inline targets, use the accepted caller contract, loaded context, focused source inspection, and command output.",
		"Treat the installed todo list as the ordered runbook",
		"Stop for unresolved binding decisions before source edits.",
		"If a plan artifact was created, commit it before Edit.",
		"Delegate dispatch",
		"Implementer spawn prompt",
		"Rendered implementer prompt: <prompt-path>",
		"contains the plan path, verification",
		"implementer-relay` gets **Review relay dispatch**",
		"choose the worker tier from dispatch metadata, but do not include `recommended-tier` in worker-facing task text",
		"Collect the normal completion report",
		"| `ticket` | `ticket_path`, `selected_phase`, empty `inline_contract`, `plan_path` |",
		"| `inline` | empty `ticket_path`/`selected_phase`, self-contained `inline_contract`, `plan_path` |",
		"Full scope | `reviewer` | `reviewer` (includes `code-reviewer`)",
		"Generated plan:",
		"Authority: Ticket path <ticket-path>",
		"Authority: Inline contract <accepted scope, constraints, non-goals, verification boundary>",
		"Direct edit with no generated plan:",
		"Reviewer prompt frame",
		"Review the supplied authority, plan contract, and diff together.",
		"without a plan artifact",
		"Review relay dispatch",
		"Render `implementer-relay` with declared inputs",
		"Rendered review relay prompt: <prompt-path>",
		"Mercenary path:",
		`ws/mercenary.result(name: "<name>", timeout_seconds: 600)`,
		"Set `policy.branch.merge_target` only when already on an implementation branch (`impl/*`, or legacy `implement/*`) or the user names it.",
		"Map a clear request to streamline implementation to `policy.low_ceremony_if_safe=yes`; labels such as `hotfix`, `tweak`, or `small fix` alone do not count.",
		"Low-ceremony preference affects branch selection only; it waives no other policy or gate.",
		"If no skip condition holds and the branch name matches `impl/*`, delete without asking",
		"the branch does not match `impl/*`, including legacy `implement/*`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("lead-implement full ws render missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Recommended tier: <recommended-tier>") {
		t.Fatalf("lead-implement full ws render still exposes recommended tier in worker-facing task text:\n%s", body)
	}
	for _, forbidden := range []string{
		"Brief template",
		"BriefPath",
		"Brief path:",
		"implementation brief",
		"lead-authored brief",
		"contains the brief path",
		"using the brief",
		"dispatch `mental-model-updater` only when workflow behavior",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("lead-implement full ws render retained old brief contract %q:\n%s", forbidden, body)
		}
	}
	for _, forbidden := range []string{
		"Review cycle <N>. Rely only on this prompt and named paths.",
		"Non-clean review paths: <paths>. Read each file directly.",
		"Commit fixes, run verification, and report commit hashes plus test results.",
		"Won't-fix allowed: style conflicts with codebase patterns; scope expansion beyond brief.",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("lead-implement full ws render still embeds old review relay prompt body %q:\n%s", forbidden, body)
		}
	}
	for _, forbidden := range []string{
		"If `Branch Action: create`",
		"If `Branch Action: rename`",
		"If `Branch Action: continue`",
		"If direct-edit:",
		"If delegated:",
		"If lead-only:",
		"If single:",
		"If partitioned:",
		"If `doc_mode` is `skipped`",
		"Acceptance:",
		"Implement or escalate Brief `## Contract Instructions`",
		"Satisfy Brief `## Integration Test Instructions`",
		"Test files: <paths, or None with reason>",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("lead-implement full ws render still contains unreachable-path prose %q:\n%s", forbidden, body)
		}
	}
	// delegates:true (spawns implementer/reviewer agents) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

func TestPlaybookPrintWsflowLeadImplementOmitsMercenaryCommands(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-implement", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, forbidden := range []string{
		"ws/mercenary.",
		"ws.mercenary.",
		`"workflow.prefer_mercenary"`,
		"Mercenary (when selected):",
		fullOnlyStart,
		fullOnlyEnd,
		mercenaryOnlyStart,
		mercenaryOnlyEnd,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("wsflow lead-implement render contains forbidden %q:\n%s", forbidden, body)
		}
	}
	for _, want := range []string{
		"Set `policy.branch.merge_target` only when already on an implementation branch (`impl/*`, or legacy `implement/*`) or the user names it.",
		"Map a clear request to streamline implementation to `policy.low_ceremony_if_safe=yes`; labels such as `hotfix`, `tweak`, or `small fix` alone do not count.",
		"Low-ceremony preference affects branch selection only; it waives no other policy or gate.",
		"If no skip condition holds and the branch name matches `impl/*`, delete without asking",
		"the branch does not match `impl/*`, including legacy `implement/*`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wsflow lead-implement render missing %q:\n%s", want, body)
		}
	}
}

func TestShippedExecutorWrapupResultIncludesBehavioralDelta(t *testing.T) {
	path := filepath.Join("..", "..", "..", "agents-plugin", "rsrc", "executor-wrapup.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read executor-wrapup: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"`Result` records the completed phase's behavioral delta; `Edition` records only\nits follow-up pass's delta. For either, include deviations, verification evidence,\nunresolved findings, and deferred follow-ups; do not restate unchanged plan or spec content.",
		"#### Edition (<short-hash>) - YYYY-MM-DD` under that phase's Result area.\n   Use the result commit supplied by the caller.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("executor-wrapup missing Result/Edition delta contract %q:\n%s", want, body)
		}
	}
}

// ---------------------------------------------------------------------------
// Golden print: Phase 3 entry-skill playbooks (real rsrc tree)
// ---------------------------------------------------------------------------

// TestPlaybookPrintGoldenLeadProceed verifies lead-proceed resolves from the
// real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadProceed(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-proceed", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "full-pipeline routing accuracy") {
		t.Errorf("body %q: expected doctrine text 'full-pipeline routing accuracy'", body)
	}
	for _, want := range []string{
		`enter.proceed`,
		"Follow `Next:` exactly",
		"Follow `Next:` from `enter.proceed` exactly",
		"scope_blocked=no-unfinished-phase",
		"scope_blocked=container-ticket",
		"scope_blocked=multiple-explicit-phases",
		"Accepted work spans multiple independently reviewable phases or needs pre-implementation contract/verification traceability beyond its eventual implementation commit and any relevant existing spec",
		"Accepted work is one bounded reviewable slice recoverable from its eventual implementation commit plus any relevant existing spec, regardless of file count or public surface",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q: expected lead-proceed handoff/verdict text %q", body, want)
		}
	}
	for _, old := range []string{
		"Use the first matching route block",
		"#### Implementation Dispatch",
		"| `has-ticket=yes`, `status=ready`, `freshness=current`, `scope-blocked=none` | `lead-implement` |",
		"### 3. Report Routing Verdict",
		"## Routing Verdict",
		"If `NEXT: lead-discuss`, continue through `ws:lead-discuss`.",
	} {
		if strings.Contains(body, old) {
			t.Errorf("body %q: old deterministic route matrix text still present: %q", body, old)
		}
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadShip verifies lead-ship resolves from the real
// rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadShip(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-ship", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "zero-surprise releases") {
		t.Errorf("body %q: expected doctrine text 'zero-surprise releases'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadAddRule verifies lead-add-rule resolves from the
// real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadAddRule(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-add-rule", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "classification accuracy at capture time") {
		t.Errorf("body %q: expected doctrine text 'classification accuracy at capture time'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadSprint verifies lead-sprint resolves from the
// real rsrc tree and is delegates:true (tip must appear).
func TestPlaybookPrintGoldenLeadSprint(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-sprint", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "session continuity across exploratory workflow turns") {
		t.Errorf("body %q: expected doctrine text 'session continuity across exploratory workflow turns'", body)
	}
	// delegates:true (native exploration-worker dispatch + ws.mercenary.register) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadDiscuss verifies lead-discuss resolves from the
// real rsrc tree and is delegates:true (reference-discovery spawn — tip must appear).
func TestPlaybookPrintGoldenLeadDiscuss(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-discuss", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "decision quality per conversation turn") {
		t.Errorf("body %q: expected doctrine text 'decision quality per conversation turn'", body)
	}
	// delegates:true (reference-discovery spawn in judge: needs-survey) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadReview verifies lead-review resolves from the
// real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadReview(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-review", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "maintainer decision quality with minimum friction") {
		t.Errorf("body %q: expected doctrine text 'maintainer decision quality with minimum friction'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadSalvage verifies lead-salvage resolves from the
// real rsrc tree and is delegates:true (native exploration-worker dispatch + ws.mercenary.register — tip must appear).
func TestPlaybookPrintGoldenLeadSalvage(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-salvage", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "evidence-preserving loss containment") {
		t.Errorf("body %q: expected doctrine text 'evidence-preserving loss containment'", body)
	}
	// delegates:true (native exploration-worker dispatch + named agent registration) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadSkillAuthoring verifies lead-skill-authoring resolves
// from the real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadSkillAuthoring(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-skill-authoring", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "executability under pressure") {
		t.Errorf("body %q: expected doctrine text 'executability under pressure'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadForgeSpec verifies lead-forge-spec resolves from the
// real rsrc tree and is delegates:true (native exploration-worker spawns — tip must appear).
func TestPlaybookPrintGoldenLeadForgeSpec(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-forge-spec", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "low-friction throughput per domain") {
		t.Errorf("body %q: expected doctrine text 'low-friction throughput per domain'", body)
	}
	// delegates:true (native exploration-worker spawns) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadForgeMentalModel verifies lead-forge-mental-model resolves
// from the real rsrc tree and is delegates:true (native exploration-worker spawns — tip must appear).
func TestPlaybookPrintGoldenLeadForgeMentalModel(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-forge-mental-model", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "confirmed operational knowledge per domain") {
		t.Errorf("body %q: expected doctrine text 'confirmed operational knowledge per domain'", body)
	}
	// delegates:true (native exploration-worker spawns) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestSkillsCallEnterTools verifies that the four skills modified in Phase 2
// contain the expected enter.* and agenda.set call tokens after rendering.
// Tokens are chosen to be non-incidental: enter.<mode> appears only from the
// inserted calls, and target/facts/policy or agenda.set are argument-level
// signals that cannot appear from surrounding prose alone.
func TestSkillsCallEnterTools(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	cases := []struct {
		skill    string
		wantAll  []string
		wantNone []string
	}{
		{
			skill:   "lead-implement",
			wantAll: []string{"enter.implement", "`target`", "`facts`", "`policy`"},
		},
		{
			skill:    "lead-proceed",
			wantAll:  []string{"enter.proceed", "Follow `Next:` from `enter.proceed` exactly", "Follow `Next:` exactly"},
			wantNone: []string{"### 3. Report Routing Verdict", "## Routing Verdict"},
		},
		{
			skill:   "lead-sprint",
			wantAll: []string{"enter.sprint"},
		},
		{
			skill:   "lead-salvage",
			wantAll: []string{"enter.salvage", "agenda.set"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.skill, func(t *testing.T) {
			body, _, err := printPlaybook(s, rsrcRoot, tc.skill, nil, wsconfig.Options{}, "", nil)
			if err != nil {
				t.Fatalf("printPlaybook(%q): %v", tc.skill, err)
			}
			for _, token := range tc.wantAll {
				if !strings.Contains(body, token) {
					t.Errorf("printPlaybook(%q): rendered body does not contain %q", tc.skill, token)
				}
			}
			for _, token := range tc.wantNone {
				if strings.Contains(body, token) {
					t.Errorf("printPlaybook(%q): rendered body should not contain %q", tc.skill, token)
				}
			}
		})
	}
}

// TestPhase3bSkillRepoint verifies that the four repointed lead skills call
// ws.workflow_manual and reference lead-revive instead of the removed
// playbook.print(name: "lead-workflow-manual") self-load pattern, and that the
// lead-revive skill exists while lead-load-workflow-manual does not.
func TestPhase3bSkillRepoint(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	skillsRoot := filepath.Join("..", "..", "..", "agents-plugin", "skills")
	wsflowSkillsRoot := filepath.Join("..", "..", "..", "agents-plugin-wsflow", "skills")
	s := newTestServerWithHarness(t, "claude")

	// Verify the four repointed skills call workflow_manual and lead-revive,
	// and no longer contain the removed playbook.print self-load call.
	repointed := []string{"lead-proceed", "lead-discuss", "lead-sprint", "lead-salvage"}
	for _, skill := range repointed {
		t.Run(skill, func(t *testing.T) {
			body, _, err := printPlaybook(s, rsrcRoot, skill, nil, wsconfig.Options{}, "", nil)
			if err != nil {
				t.Fatalf("printPlaybook(%q): %v", skill, err)
			}
			if !strings.Contains(body, "workflow_manual") {
				t.Errorf("%s: rendered body must contain 'workflow_manual'", skill)
			}
			if !strings.Contains(body, "lead-revive") {
				t.Errorf("%s: rendered body must contain 'lead-revive'", skill)
			}
			if strings.Contains(body, `playbook.print(name: "lead-workflow-manual")`) {
				t.Errorf("%s: rendered body must not contain removed playbook.print self-load call", skill)
			}
		})
	}

	// lead-revive SKILL.md must exist in agents-plugin/skills/.
	reviveSkillPath := filepath.Join(skillsRoot, "lead-revive", "SKILL.md")
	if _, err := os.Stat(reviveSkillPath); err != nil {
		t.Errorf("lead-revive SKILL.md missing from agents-plugin/skills/: %v", err)
	}

	// lead-revive SKILL.md must exist in agents-plugin-wsflow/skills/.
	wsflowReviveSkillPath := filepath.Join(wsflowSkillsRoot, "lead-revive", "SKILL.md")
	if _, err := os.Stat(wsflowReviveSkillPath); err != nil {
		t.Errorf("lead-revive SKILL.md missing from agents-plugin-wsflow/skills/: %v", err)
	}

	// lead-load-workflow-manual must NOT exist in agents-plugin/skills/.
	removedSkillPath := filepath.Join(skillsRoot, "lead-load-workflow-manual", "SKILL.md")
	if _, err := os.Stat(removedSkillPath); !os.IsNotExist(err) {
		t.Errorf("lead-load-workflow-manual SKILL.md should be removed but still present (stat: %v)", err)
	}
}

// TestPlaybookPrintGoldenLeadBootstrap verifies lead-bootstrap resolves from the
// real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadBootstrap(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-bootstrap", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "idempotent downstream migration") {
		t.Errorf("body %q: expected doctrine text 'idempotent downstream migration'", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}
