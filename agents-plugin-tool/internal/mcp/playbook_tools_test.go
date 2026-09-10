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
		"TargetPath":         "ai-docs/tickets/ready/260726-bug-demo.md",
		"ReviewCycle":        "2",
		"CommitRange":        "abc123..def456",
		"ReviewPaths":        "ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"DispositionNotes":   "Fix correctness finding C1; defer test fixture rename until Phase 3.",
		"VerificationHint":   "go test ./internal/mcp -run TestRenderPlaybookShippedImplementerRelayDeclaredContext",
		"ResultExpectations": "Report per-finding dispositions, fix commits, updated range, verification, and blockers.",
	}
}

func shippedImplementerElevatedContext() map[string]string {
	return map[string]string{
		"TargetPath":         "ai-docs/tickets/ready/260726-bug-demo.md",
		"ReviewCycle":        "3",
		"CommitRange":        "abc123..def456",
		"ReviewPaths":        "ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"DispositionNotes":   "C1 was reported [fixed] at cycle 2 and returned [unresolved: the guard still misses the empty range].",
		"PriorFixCommits":    "abc123 (cycle 1 fix), def456 (cycle 2 fix)",
		"PriorDispositions":  "cycle 1: C1 [fixed]; cycle 2: C1 [fixed], T2 [deferred: fixture rename waits for Phase 3].",
		"VerificationHint":   "go test ./internal/mcp -run TestRenderPlaybookShippedImplementerElevatedDeclaredContext",
		"ResultExpectations": "Report per-finding dispositions, the attempt record, fix commits, updated range, verification, and blockers.",
	}
}

func shippedReviewAdjudicatorContext() map[string]string {
	return map[string]string{
		"PlanPath":         "ai-docs/.plans/plan.md",
		"ReviewPaths":      "ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"DispositionNotes": "C1 [won't fix: conflicts with the local table-driven test pattern]; F2 [escalate: the fix needs a plan update].",
		"CommitRange":      "abc123..def456",
		"ReviewCycle":      "2",
		"target_kind":      "ticket",
		"ticket_path":      "ai-docs/tickets/ready/260726-bug-demo.md",
		"selected_phase":   "Phase 2: Adjudicator delegate",
		"inline_contract":  "",
	}
}

// initGitRepo creates a git repository in a temp dir and returns its path.
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
// playbook.read — golden harness rendering
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

// TestPlaybookPrintPiHarnessSelectsOverlay proves a `.pi.md` structural
// overlay is selected only under a detected `pi` harness — the only piece
// gating overlay selection is that s.currentHarness() can now return "pi"
// (wsrsrc.Load itself validates harness only as a bare filesystem stem, with
// no closed enum, so no wsrsrc code change is required for this phase).
func TestPlaybookPrintPiHarnessSelectsOverlay(t *testing.T) {
	const baseText = "Base playbook body."
	const overlayText = "Pi overlay playbook body."
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"overlay-pb/overlay-pb.md":    "---\nkind: print\ndelegates: false\n---\n# Overlay Playbook\n\n" + baseText + "\n",
		"overlay-pb/overlay-pb.pi.md": "---\nkind: print\ndelegates: false\n---\n# Overlay Playbook (Pi)\n\n" + overlayText + "\n",
	})

	piServer := newTestServerWithHarness(t, "pi")
	piBody, _, err := printPlaybook(piServer, rsrcRoot, "overlay-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook (pi): %v", err)
	}
	if !strings.Contains(piBody, overlayText) {
		t.Errorf("pi body %q: expected pi overlay text %q", piBody, overlayText)
	}
	if strings.Contains(piBody, baseText) {
		t.Errorf("pi body %q: base text must not appear when overlay is selected", piBody)
	}

	for _, harness := range []string{"", "codex"} {
		s := newTestServerWithHarness(t, harness)
		body, _, err := printPlaybook(s, rsrcRoot, "overlay-pb", nil, wsconfig.Options{}, "", nil)
		if err != nil {
			t.Fatalf("printPlaybook (%q): %v", harness, err)
		}
		if !strings.Contains(body, baseText) {
			t.Errorf("harness %q body %q: expected base text", harness, body)
		}
		if strings.Contains(body, overlayText) {
			t.Errorf("harness %q body %q: pi overlay must not be selected for a non-pi harness", harness, body)
		}
	}
}

// TestPlaybookPrintPiHarnessUsesNeutralTerminology guards the Non-goal that
// this phase does not author Pi-specific terminology: terminologyForHarness
// must still fall back to the host-neutral row for a detected "pi" harness,
// even though structural overlay selection now works for pi.
func TestPlaybookPrintPiHarnessUsesNeutralTerminology(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "pi")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutral := terminologyForHarness("")
	piTerm := terminologyForHarness("pi")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if piTerm[varName] != neutral[varName] {
			t.Errorf("terminologyForHarness(pi)[%s] = %q, want host-neutral %q", varName, piTerm[varName], neutral[varName])
		}
		if !strings.Contains(body, neutral[varName]) {
			t.Errorf("body %q: expected neutral %s %q for pi harness", body, varName, neutral[varName])
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
// playbook.read — delegation tip injection
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
// playbook.read — caller context substitution
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
// playbook.read — no-vars fast path
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

	path, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "delegate-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
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

// TestPlaybookPrintModelAliasPiHarnessWhenSet verifies that a pi-keyed
// agents.tier value (harness="pi") is picked up by a detected pi-harness
// session's playbook.render/print, proving the tier→model resolver's
// aliasResolutionKeys chain now tries "pi" before falling back to "default".
func TestPlaybookPrintModelAliasPiHarnessWhenSet(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "pi")

	cacheHome := t.TempDir()
	uniqueModel := "test-pi-model-xyz-7777"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheHome}, "medium", "pi", uniqueModel, "pi"); err != nil {
		t.Fatalf("SetAgentsTierForHarness: %v", err)
	}

	body, _, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, uniqueModel) {
		t.Errorf("body %q: expected pi-keyed model name %q", body, uniqueModel)
	}
}

// TestPlaybookPrintModelAliasPiHarnessFallsBackToDefault verifies that a
// detected pi-harness session with no pi-keyed agents.tier value falls
// through to the seeded default-tier model (Decision: "Default bucket
// semantics unchanged" — no new fallback special-case for pi).
func TestPlaybookPrintModelAliasPiHarnessFallsBackToDefault(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "pi")

	cacheHome := t.TempDir()

	body, _, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "gpt-5.6-terra") {
		t.Errorf("body %q: expected seeded default medium-tier model gpt-5.6-terra", body)
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
			if tc.harness == "claude" {
				for _, forbidden := range []string{
					"### Native delegate spawn",
					"spawn_agent.model",
					"spawn_agent.reasoning_effort",
					`fork_turns: "none"`,
				} {
					if strings.Contains(body, forbidden) {
						t.Errorf("Claude workflow manual leaked Codex native binding guidance %q:\n%s", forbidden, body)
					}
				}
			}
			if tc.harness == "codex" {
				for _, want := range []string{
					"### Native delegate spawn",
					"pass a returned `recommended-model` as\n`spawn_agent.model`",
					"a returned `recommended-reasoning-effort` as\n`spawn_agent.reasoning_effort`",
					"Omit either field when its binding line is\nabsent",
					`fork_turns: "none"`,
					"never use `effort` as a spawn parameter.",
					"report the rejected field and\nvalue and do not claim that binding was applied",
				} {
					if !strings.Contains(body, want) {
						t.Errorf("Codex workflow manual missing %q:\n%s", want, body)
					}
				}
				if got := strings.Count(body, "### Native delegate spawn"); got != 1 {
					t.Errorf("Codex workflow manual rendered delegated-binding section %d times, want 1:\n%s", got, body)
				}
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
	if !has("playbook.read") {
		t.Error("playbook.read missing from LeadToolNames")
	}
	if !has("playbook.render") {
		t.Error("playbook.render missing from LeadToolNames")
	}
}

func TestPlaybookToolsNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("playbook.read") {
		t.Error("playbook.read is incorrectly hidden in no-agent mode")
	}
	if noAgentHiddenTool("playbook.render") {
		t.Error("playbook.render is incorrectly hidden in no-agent mode")
	}
}

// TestConfigResolveAgentNotNoAgentHidden covers config.resolve_agent's
// no-agent applicability (260905 Phase 3): like config.list/config.tune, it
// is a generic tool with no configRegistry entry, so configKeyEntryForTool
// resolves not-found and noAgentHiddenTool's config.* branch falls through
// to "not hidden" — the tool stays visible in agentless/wsflow mode.
func TestConfigResolveAgentNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("config.resolve_agent") {
		t.Error("config.resolve_agent is incorrectly hidden in no-agent mode")
	}
}

func TestPlaybookToolsVisibleInToolsList(t *testing.T) {
	listed := map[string]bool{}
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		listed[name] = true
	}
	for _, want := range []string{"playbook.read", "playbook.render"} {
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
		for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "exec.", "Full ws", "full ws", "ws:override:", "ws:/override:"} {
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
		`ws/config.list(session_key: <lead key>)`,
		`"workflow.prefer_subagent"`,
		"`config.tune` with `key` set to `\"workflow.prefer_subagent\"`",
		"prompt.UserPreferenceSection",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("lead-tune render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"Call `config.workflow_prefer_subagent`",
		"prompt.DelegationSection",
		"DelegationSection",
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
		`wsflow/config.list(session_key: <lead key>)`,
		"wsflow workflow",
		`"workflow.prefer_subagent"`,
		"`config.tune` with `key` set to `\"workflow.prefer_subagent\"`",
		"prompt.UserPreferenceSection",
		"## On: tune model tier",
		"Map the request to the `agents.tier` catalog knob",
		"model tier (`agents.tier`)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wsflow lead-tune render missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
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
	wsflow := renderProductModePlaybookBody(input)
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
	full := renderProductModePlaybookBody(input)
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
Call {{.McpNamespace}}/tickets.query and {{.SkillNamespace}}:lead-discuss.
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
	for _, want := range []string{"wsflow/tickets.query", "wsflow:lead-discuss", "ferrule"} {
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
Call {{.McpNamespace}}/tickets.query and {{.SkillNamespace}}:lead-discuss.
`,
	})
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "namespace-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"ws/tickets.query", "ws:lead-discuss"} {
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

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
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
	for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "exec.", "showsflow", "knowsflow", "followsflow", "workflowsflow"} {
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

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
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
		"Plan path: `ai-docs/.plans/plan.md`",
		"Verification instructions: go test ./internal/mcp -run TestRenderPlaybookShippedImplementerDeclaredContext",
		"Binding result expectations: Report outcome, files changed, commits, verification, and blockers.",
		"Commit-range reporting requirement: Report <first-commit>..<last-commit> after committing logical checkpoints.",
		"When `## Relevant Ticket Contract` names a ticket path and phase heading, read",
		"that ticket file and treat the selected phase text as the task contract:",
		"When it instead holds a verbatim",
		"inline contract, there is no ticket to read; treat that contract as the task",
		"Where a plan step and the ticket disagree, the ticket wins; report the",
		"disagreement instead of following the step.",
		"Read the plan path above, all `[Must]` References listed in the plan, and, when `## Relevant Ticket Contract` names a ticket path, that ticket file",
		"otherwise stop and ask the caller to update the plan's `Escalations` section.",
		"suggestions that expand scope beyond the selected phase.",
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
		"The plan and its listed references are the task contract.",
		"Do not re-research design alternatives; the plan owns the decisions.",
		"Do not read ticket files directly unless the plan's `Escalations` section explicitly authorizes ticket-file reading.",
		"Read the plan path above and all `[Must]` References listed in the plan except ticket files.",
		"suggestions that expand scope beyond the plan.",
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

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", shippedImplementerRelayContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
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
		"Target path: `ai-docs/tickets/ready/260726-bug-demo.md`",
		"Review cycle: 2",
		"Current commit range: abc123..def456",
		"Non-clean review paths: ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"Lead disposition notes: Fix correctness finding C1; defer test fixture rename until Phase 3.",
		"Verification instructions: go test ./internal/mcp -run TestRenderPlaybookShippedImplementerRelayDeclaredContext",
		"Result expectations: Report per-finding dispositions, fix commits, updated range, verification, and blockers.",
		"Rely only on this prompt and named paths; do not depend on prior conversation.",
		"Read the target and every non-clean review path directly.",
		"Won't-fix is allowed only for style suggestions conflicting with local patterns, findings that require scope expansion beyond the selected phase, or findings disproven by specific evidence.",
		"Treat the target as the task contract: read it, and when it names phases, treat the selected phase text as the contract and later phases as out of scope.",
		"escalate for a target update if a required fix needs a deviation from the target.",
		"Won't-fix is not allowed for correctness, security, contract, regression, or required-test violations.",
		"records the relevant per-finding dispositions known at that checkpoint",
		"`[fixed]`",
		"`[won't fix: <reason>]`",
		"`[deferred: <reason>]`",
		// Both token enumerations anchored explicitly: Process step 4 first, then the
		// Output bullet. A single unanchored Contains would pass with only one site
		// updated — the exact drift that made the escalation token invisible before.
		"decide `[fixed]`, `[won't fix: <reason>]`, `[deferred: <reason>]`, or `[escalate: <reason>]`.",
		"- `[escalate: <reason>]` — needs a change to the target itself; the lead decides the scope question before the next review.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("implementer-relay render missing %q:\n%s", want, body)
		}
	}
	if got := strings.Count(body, "`[escalate: <reason>]`"); got != 2 {
		t.Fatalf("implementer-relay must enumerate `[escalate: <reason>]` at both sites (Process step 4 and the Output bullet), got %d occurrence(s):\n%s", got, body)
	}
	for _, forbidden := range []string{
		"BriefPath",
		"Brief path:",
		"No-plan sentinel",
		"Read the brief",
		"scope expansion beyond the brief",
		"findings, or disposition notes explicitly authorize ticket-file reading",
		"Do not read ticket files directly unless the plan's `Escalations` section explicitly authorizes ticket-file reading.",
		"scope expansion beyond the plan.",
		"escalate for a plan update if a required fix needs ticket material or a plan deviation.",
		"Keep fixes inside the scope defined by the plan, review findings, and disposition notes.",
		"needs a plan update or ticket material",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("implementer-relay render retained old brief contract %q:\n%s", forbidden, body)
		}
	}
	if strings.Contains(body, "Continuity tip") {
		t.Fatalf("implementer-relay render must not include delegation continuity tip:\n%s", body)
	}
}

// TestRenderPlaybookShippedImplementerElevatedDeclaredContext verifies the elevated
// implementer by dispatching it, not by reading its text: the declared `tier: large`
// must actually reach the caller through renderPlaybook, and `role: implementer` must
// actually mint a delegate-scoped child key. A declared tier no dispatch path honors is
// how the original review-relay cap was lost, so this test asserts the render contract
// rather than the frontmatter file.
//
// It also pins the three axes on which this delegate must differ in kind from
// `implementer-relay`, since a tier-only copy would be near-duplicate prose: prior-cycle
// inputs, root-cause posture, and an attempt record that survives a failed cycle.
func TestRenderPlaybookShippedImplementerElevatedDeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-elevated", shippedImplementerElevatedContext(), wsconfig.Options{CacheHome: cacheHome}, worktreeRoot, "", "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "large" {
		t.Fatalf("implementer-elevated recommended tier = %q, want large", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"Your ws session_key",
		// Every declared input substitutes.
		"Target path: `ai-docs/tickets/ready/260726-bug-demo.md`",
		"Review cycle: 3",
		"Current commit range: abc123..def456",
		"Non-clean review paths: ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"Lead disposition notes: C1 was reported [fixed] at cycle 2 and returned [unresolved: the guard still misses the empty range].",
		"Prior cycles' fix commits: abc123 (cycle 1 fix), def456 (cycle 2 fix)",
		"Prior cycles' per-finding dispositions: cycle 1: C1 [fixed]; cycle 2: C1 [fixed], T2 [deferred: fixture rename waits for Phase 3].",
		"Verification instructions: go test ./internal/mcp -run TestRenderPlaybookShippedImplementerElevatedDeclaredContext",
		"Result expectations: Report per-finding dispositions, the attempt record, fix commits, updated range, verification, and blockers.",
		// Axis 1 — inputs: the prior fix commits are read, not merely listed.
		"Read the target, every non-clean review path, and the prior fix commits' diffs directly.",
		// Axis 2 — posture: symptom-vs-cause, a different in-scope approach, escalation.
		"Name each relayed finding's root cause before editing; every finding here survived a prior fix or shares a root cause with one.",
		"Propose and apply a different in-scope approach when the prior attempt treated a symptom rather than the cause.",
		"Escalate for a target update when the cause-addressing fix falls outside the target; do not shrink the fix to fit the target instead.",
		"Decide per finding whether the prior attempt addressed the cause or a symptom, and name the cause this cycle targets.",
		// Axis 3 — output: the attempt record is written even when this cycle also fails.
		"Report the approach you attempted and its outcome for every relayed finding, including each finding this cycle failed to resolve.",
		"Attempt record — one line per relayed finding, written whatever the disposition:",
		"when this cycle's attempt also failed — what failed this time and the evidence that showed it",
		// Lead-side parity: the same four disposition tokens as implementer-relay.
		"decide `[fixed]`, `[won't fix: <reason>]`, `[deferred: <reason>]`, or `[escalate: <reason>]`.",
		"- `[escalate: <reason>]` — needs a change to the target itself; the lead decides the scope question before the next review.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("implementer-elevated render missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Fatalf("implementer-elevated render left an unsubstituted variable placeholder:\n%s", body)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Fatalf("implementer-elevated render must not include delegation continuity tip:\n%s", body)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found in registry", key)
	}
	if entry.scope != roleDelegate {
		t.Fatalf("implementer-elevated minted key scope = %q, want %q", entry.scope, roleDelegate)
	}
}

// fixCycleDispositionTokens is the four-token disposition vocabulary shared
// byte-identically by both fix-cycle delegates, whichever one served the relay.
func fixCycleDispositionTokens() []string {
	return []string{
		"`[fixed]`",
		"`[won't fix: <reason>]`",
		"`[deferred: <reason>]`",
		"`[escalate: <reason>]`",
	}
}

// notFixedDispositionToken is the Important-only self-reported non-resolution
// marker added in 260831's severity-graded relay budget. Unlike the four core
// tokens above, it deliberately does not join the shared vocabulary: it lives
// in `implementer-relay`'s enumeration sites only. `implementer-elevated` is
// reachable only at the Critical ceiling, and Critical never gets a "not
// fixed" disposition — a still-non-clean Critical carries forward into the
// next Critical-scoped round instead of settling as unresolved.
const notFixedDispositionToken = "`[not fixed: <reason>]`"

// extractDispositionEnumerations returns a fix-cycle delegate's two vocabulary
// enumeration sites: the Process-step sentence that lists every token, and the Output
// bullet block that defines each one.
//
// The Process step number is stripped so the relay's step 4 and the elevated
// delegate's step 6 compare equal — the guard is about the token set, not its position.
// Sites that merely *use* one token (the elevated delegate's Process step 4 names
// `[escalate: <reason>]` as an approach choice) are deliberately excluded: they are not
// enumerations, and counting them would make a whole-body token count disagree between
// the two files for a legitimate reason.
func extractDispositionEnumerations(t *testing.T, name, body string) (string, []string) {
	t.Helper()
	processLine := ""
	bullets := []string{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.Contains(trimmed, "decide `[fixed]`"):
			if processLine != "" {
				t.Fatalf("%s enumerates the disposition vocabulary in more than one Process step; the drift guard assumes exactly one:\n%s", name, body)
			}
			if idx := strings.Index(trimmed, "For every relayed"); idx >= 0 {
				trimmed = trimmed[idx:]
			}
			processLine = trimmed
		case strings.HasPrefix(trimmed, "- `["):
			bullets = append(bullets, trimmed)
		}
	}
	if processLine == "" {
		t.Fatalf("%s has no Process step enumerating the disposition vocabulary:\n%s", name, body)
	}
	if len(bullets) == 0 {
		t.Fatalf("%s has no Output bullet block enumerating the disposition vocabulary:\n%s", name, body)
	}
	return processLine, bullets
}

// renderedImplementerDispositionEnumerations renders both fix-cycle delegates
// with their shipped context and extracts each one's Process/Output disposition
// enumeration sites, for the two drift guards below to check independently.
func renderedImplementerDispositionEnumerations(t *testing.T) (relayProcess string, relayBullets []string, elevatedProcess string, elevatedBullets []string) {
	t.Helper()
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	render := func(name string, ctx map[string]string) string {
		t.Helper()
		path, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, name, ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
		if err != nil {
			t.Fatalf("%s renderPlaybook: %v", name, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s read rendered playbook: %v", name, err)
		}
		return string(data)
	}

	relayProcess, relayBullets = extractDispositionEnumerations(t, "implementer-relay", render("implementer-relay", shippedImplementerRelayContext()))
	elevatedProcess, elevatedBullets = extractDispositionEnumerations(t, "implementer-elevated", render("implementer-elevated", shippedImplementerElevatedContext()))
	return relayProcess, relayBullets, elevatedProcess, elevatedBullets
}

// TestRenderedImplementerDelegatesShareOneDispositionVocabulary extends Phase 2's
// two-site anti-drift count to all four sites the four-token core vocabulary lives
// at: `implementer-relay` Process 4 and Output, `implementer-elevated` Process 6 and
// Output. It covers only the four tokens both delegates share
// (`fixCycleDispositionTokens`); the fifth, Important-only `[not fixed: <reason>]`
// token is a deliberate asymmetry between the two files (260831 severity-graded
// relay) and is covered separately by
// TestImplementerRelayNotFixedMarkerPresentOnlyOnImportantPath below — this test
// must not re-flag that asymmetry as drift.
//
// The two delegates are near-duplicates by construction and either can serve a
// relay for a Critical/Important finding's four settled dispositions, so the lead
// parses one token set back from both for those. A token added, reworded, or
// dropped at one site and not the others would hand the lead contradictory
// vocabularies on the same relay path — the exact drift class that produced this
// ticket, and a class the per-file count assertion cannot see because it never
// compares the files.
func TestRenderedImplementerDelegatesShareOneDispositionVocabulary(t *testing.T) {
	relayProcess, relayBullets, elevatedProcess, elevatedBullets := renderedImplementerDispositionEnumerations(t)

	// implementer-elevated carries no [not fixed: <reason>] site, so its full
	// enumeration IS the four-core-token clause; implementer-relay's enumeration
	// leads with that same clause before its own Important-only continuation, so
	// elevatedProcess must be a byte-identical prefix of relayProcess.
	if !strings.HasPrefix(relayProcess, elevatedProcess) {
		t.Fatalf("implementer-relay Process enumeration does not lead with the four-core-token clause implementer-elevated defines:\n relay: %s\n elevated: %s", relayProcess, elevatedProcess)
	}
	if len(elevatedBullets) != len(fixCycleDispositionTokens()) {
		t.Fatalf("implementer-elevated Output enumerates %d disposition bullets, want %d — a core token was added or removed without updating fixCycleDispositionTokens:\n%s",
			len(elevatedBullets), len(fixCycleDispositionTokens()), strings.Join(elevatedBullets, "\n"))
	}
	if len(relayBullets) < len(fixCycleDispositionTokens()) {
		t.Fatalf("implementer-relay Output enumerates %d disposition bullets, want at least %d core bullets:\n%s",
			len(relayBullets), len(fixCycleDispositionTokens()), strings.Join(relayBullets, "\n"))
	}
	relayCoreBlock := strings.Join(relayBullets[:len(fixCycleDispositionTokens())], "\n")
	elevatedBlock := strings.Join(elevatedBullets, "\n")
	if relayCoreBlock != elevatedBlock {
		t.Fatalf("implementer-relay's leading %d Output bullets disagree with implementer-elevated's:\n relay leading bullets:\n%s\n elevated bullets:\n%s", len(fixCycleDispositionTokens()), relayCoreBlock, elevatedBlock)
	}

	// Each core token appears exactly once at each site, in both delegates. Catches
	// a token dropped from one site, or duplicated within one.
	for _, delegate := range []struct {
		name    string
		process string
		bullets string
	}{
		{name: "implementer-relay", process: relayProcess, bullets: relayCoreBlock},
		{name: "implementer-elevated", process: elevatedProcess, bullets: elevatedBlock},
	} {
		for _, token := range fixCycleDispositionTokens() {
			if got := strings.Count(delegate.process, token); got != 1 {
				t.Fatalf("%s Process enumeration names %s %d time(s), want 1:\n%s", delegate.name, token, got, delegate.process)
			}
			if got := strings.Count(delegate.bullets, token); got != 1 {
				t.Fatalf("%s Output enumeration defines %s %d time(s), want 1:\n%s", delegate.name, token, got, delegate.bullets)
			}
		}
	}
}

// TestImplementerRelayNotFixedMarkerPresentOnlyOnImportantPath guards the
// deliberate asymmetry TestRenderedImplementerDelegatesShareOneDispositionVocabulary
// does not cover: `[not fixed: <reason>]` is Important-only self-reported
// non-resolution, so it must appear exactly once at each `implementer-relay`
// enumeration site and be wholly absent from `implementer-elevated` — that
// delegate is reachable only at the Critical ceiling, and Critical never
// settles as "not fixed" (see 260831's Risk Signal / `implementer-relay.md`
// Process step 4 and Output). A future edit that "fixes" this back into
// symmetry with implementer-elevated is the regression this test exists to
// catch.
func TestImplementerRelayNotFixedMarkerPresentOnlyOnImportantPath(t *testing.T) {
	relayProcess, relayBullets, elevatedProcess, elevatedBullets := renderedImplementerDispositionEnumerations(t)

	if got := strings.Count(relayProcess, notFixedDispositionToken); got != 1 {
		t.Fatalf("implementer-relay Process enumeration names %s %d time(s), want exactly 1:\n%s", notFixedDispositionToken, got, relayProcess)
	}
	relayBlock := strings.Join(relayBullets, "\n")
	if got := strings.Count(relayBlock, notFixedDispositionToken); got != 1 {
		t.Fatalf("implementer-relay Output enumeration defines %s %d time(s), want exactly 1:\n%s", notFixedDispositionToken, got, relayBlock)
	}
	if len(relayBullets) != len(fixCycleDispositionTokens())+1 {
		t.Fatalf("implementer-relay Output enumerates %d disposition bullets, want %d (four core plus [not fixed: <reason>]):\n%s",
			len(relayBullets), len(fixCycleDispositionTokens())+1, relayBlock)
	}

	if strings.Contains(elevatedProcess, notFixedDispositionToken) {
		t.Fatalf("implementer-elevated Process enumeration must not name %s — Critical never gets a \"not fixed\" disposition:\n%s", notFixedDispositionToken, elevatedProcess)
	}
	elevatedBlock := strings.Join(elevatedBullets, "\n")
	if strings.Contains(elevatedBlock, notFixedDispositionToken) {
		t.Fatalf("implementer-elevated Output enumeration must not define %s — Critical never gets a \"not fixed\" disposition:\n%s", notFixedDispositionToken, elevatedBlock)
	}
}

// TestRenderPlaybookShippedReviewAdjudicatorDeclaredContext pins the adjudicator
// delegate's render contract: every declared input substitutes, a lead render mints
// the child session key (role: delegate), the frontmatter tier reaches the caller,
// and the aperture constraint plus the three verdict tokens survive into the body.
func TestRenderPlaybookShippedReviewAdjudicatorDeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "review-adjudicator", shippedReviewAdjudicatorContext(), wsconfig.Options{CacheHome: cacheHome}, worktreeRoot, "", "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "large" {
		t.Fatalf("review-adjudicator recommended tier = %q, want large", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"Your ws session_key",
		"Plan path: `ai-docs/.plans/plan.md`",
		"Review findings paths: ai-docs/.reviews/correctness.md, ai-docs/.reviews/test.md",
		"Implementer disposition record: C1 [won't fix: conflicts with the local table-driven test pattern]; F2 [escalate: the fix needs a plan update].",
		"Commit range under review: abc123..def456",
		"Review cycle: 2",
		"Authority kind: ticket",
		"Ticket path: `ai-docs/tickets/ready/260726-bug-demo.md`",
		"Selected phase: Phase 2: Adjudicator delegate",
		`Answer only "is the implementer's stated reason true"; never answer "is this code correct".`,
		"Do not re-review the diff for correctness: the reviewer's factual claims about the diff stand unless the implementer supplied specific disproving evidence.",
		"`[accept]`",
		"`[override: <reason>]`",
		"`[out-of-scope: <reason>]`",
		"Rely only on this prompt and the named paths; do not depend on prior conversation.",
		"An escalation defends nothing, so it takes `[override: <reason>]` when the plan already covers the required fix, and `[out-of-scope: <reason>]` when it does not.",
		"The specific-evidence row licenses the commit range only for locating the evidence the implementer named; forming your own opinion of the code from it is the re-review this role forbids.",
		// Row references are by defense name, not position: a future row insertion or
		// reorder must not silently re-point these rules at the wrong defense.
		"an escalation by the scope-expansion row.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review-adjudicator render missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Fatalf("review-adjudicator render left an unsubstituted variable placeholder:\n%s", body)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found in registry", key)
	}
	if entry.scope != roleDelegate {
		t.Fatalf("review-adjudicator minted key scope = %q, want %q", entry.scope, roleDelegate)
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
		"note": "see ws/tickets.query for details",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil)
	if err != nil {
		t.Fatalf("renderPlaybook code-reviewer with legacy context: %v", err)
	}
	codeReviewerData, err := os.ReadFile(codeReviewerPath)
	if err != nil {
		t.Fatalf("read code-reviewer render: %v", err)
	}
	codeReviewerBody := string(codeReviewerData)
	for _, want := range []string{"wsflow/", "## Render Context", "- note: see ws/tickets.query for details"} {
		if !strings.Contains(codeReviewerBody, want) {
			t.Fatalf("code-reviewer render missing %q:\n%s", want, codeReviewerBody)
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
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for code-reviewer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	ctx := shippedImplementerContext()
	ctx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	implCtx := shippedImplementerContext()
	implCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", implCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
		}
	}

	relayBriefCtx := shippedImplementerRelayContext()
	relayBriefCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", relayBriefCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer-relay")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer-relay BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
		}
	}

	relayCtx := shippedImplementerRelayContext()
	relayCtx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", relayCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer-relay")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer-relay renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	// The elevated relay mints two new input names; a typo in either reaches the
	// delegate as a silently empty prior-cycle record unless renderPlaybook rejects it.
	elevatedTypoCtx := shippedImplementerElevatedContext()
	delete(elevatedTypoCtx, "PriorFixCommits")
	elevatedTypoCtx["PriorFixCommit"] = "abc123"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-elevated", elevatedTypoCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted a misspelled PriorFixCommits for implementer-elevated")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "PriorFixCommit" {
			t.Fatalf("full ws implementer-elevated PriorFixCommit error = %T %v, want ErrUndeclaredVar PriorFixCommit", err, err)
		}
	}

	elevatedCtx := shippedImplementerElevatedContext()
	elevatedCtx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-elevated", elevatedCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer-elevated")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer-elevated renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
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
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
		t.Fatal("wsflow non-legacy renderPlaybook accepted undeclared context")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("wsflow non-legacy renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer-relay", map[string]string{
		"note": "implementer-relay is not a wsflow legacy freeform stem",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil); err == nil {
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
		if name != "playbook.read" && name != "playbook.render" {
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
		"name": "playbook.read",
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

// TestPlaybookPrintRetiredSpecStemsGone pins the retirement of the spec and
// mental-model playbooks: printPlaybook must fail to resolve each stem rather
// than serve a stale body left behind in the rsrc tree.
func TestPlaybookPrintRetiredSpecStemsGone(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	for _, name := range []string{
		"lead-update-spec", "lead-write-spec", "lead-forge-spec",
		"lead-forge-mental-model", "lead-backfill-docs",
		"mental-model-updater", "doc-gap-discovery",
	} {
		if _, _, err := printPlaybook(s, rsrcRoot, name, nil, wsconfig.Options{}, "", nil); err == nil {
			t.Errorf("printPlaybook(%q) still resolves a retired playbook", name)
		}
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
	// Verify the dead-path fix: self-reinvoke uses playbook.read, not ws:lead-workflow-manual.
	if !strings.Contains(body, `ws/playbook.read(name: "lead-workflow-manual")`) {
		t.Errorf("body %q: expected updated self-reinvoke instruction using playbook.read", body)
	}
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadTicket verifies lead-ticket resolves from the
// real rsrc tree, splices its task-list include, and keeps the two boundaries
// the collapse must not lose: the Open Decision Queue gates what gets
// written, and the ready-promotion path runs dependency closure and the sage
// gate before its single commit. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadTicket(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-ticket", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{
		"You are the lead managing the ticket inventory",
		"## Open Decision Queue",
		"Included Guidance: Open Decision Queue Task List",
		"tickets.create_empty",
		"tickets.sage_gate(stem, landing: \"ready\")",
		"tickets.sage_stamp",
		"Dependency closure over the whole batch first",
		"Stamps leave files uncommitted",
		"ticket-fact-populator",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-ticket text %q:\n%s", want, body)
		}
	}
	// The retired entry points must not be named by the surviving skill.
	for _, forbidden := range []string{
		"lead-write-ticket",
		"lead-proceed",
		"lead-implement",
		"[design-review:",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("body still names retired surface %q:\n%s", forbidden, body)
		}
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
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
		"`Result` records the completed phase's behavioral delta; `Edition` records only\nits follow-up pass's delta. For either, include deviations — diffed between the\nticket's selected phase text and what landed, not recalled from the implementer —\nverification evidence, unresolved findings, and deferred follow-ups; do not\nrestate unchanged plan content.",
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

// TestRetiredLeadPlaybooksNoLongerResolve pins the collapse: the routing
// entry playbook retired into lead-run and the implementation procedure was
// superseded by the ticket-worker playbook, so neither may resolve again. A
// reintroduction is a silent re-expansion of the lead surface, which is
// exactly what this change removes; the replacements are asserted here too so
// the retirement cannot pass while leaving no path.
func TestRetiredLeadPlaybooksNoLongerResolve(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	for _, retired := range []string{"lead-proceed", "lead-implement", "lead-write-ticket", "lead-verify-discussion"} {
		if _, _, err := printPlaybook(s, rsrcRoot, retired, nil, wsconfig.Options{}, "", nil); err == nil {
			t.Errorf("%s must no longer resolve as an rsrc playbook", retired)
		}
	}
	for _, replacement := range []string{"lead-run", "lead-ticket", "ticket-worker"} {
		if _, _, err := printPlaybook(s, rsrcRoot, replacement, nil, wsconfig.Options{}, "", nil); err != nil {
			t.Errorf("replacement playbook %s must resolve: %v", replacement, err)
		}
	}
}

// TestPlaybookPrintGoldenLeadShip verifies lead-ship resolves from the real
// rsrc tree, keeps the un-omittable release gate, and still carries the ship
// config schema no tool owns. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadShip(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-ship", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{
		"A release is low-reversibility",
		"## Release gate",
		"release-boundary: present",
		"review.marker(format: json)",
		"This gate never calls `review.stamp`",
		"### Ship Config Format",
		"## Version Strategy",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-ship text %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[design-review:") {
		t.Errorf("body %q: unresolved design-review marker shipped", body)
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

// TestPlaybookPrintGoldenLeadDiscuss verifies lead-discuss resolves from the
// real rsrc tree, keeps its conversation-only boundary, and routes capture and
// execution at the collapsed names. delegates:true (exploration spawn) — tip
// must appear, and the ExploreAgent variable must be substituted.
func TestPlaybookPrintGoldenLeadDiscuss(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-discuss", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{
		"you edit no source and write no document here",
		"ws:lead-ticket",
		"ws:lead-run",
		"### Binding Anchor",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-discuss text %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{{.ExploreAgent}}") {
		t.Errorf("body %q: ExploreAgent left unsubstituted", body)
	}
	// delegates:true (exploration spawn) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadReview verifies lead-review resolves from the
// real rsrc tree, keeps the single-writer ledger stamp rule, and still
// carries the review config schema no tool owns. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadReview(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-review", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{
		"reviewing work you did not write",
		"This step\n   is the ledger's only writer.",
		"Never\n   pass the marker entry's base",
		"### Review Config Template",
		"## Landing Lens",
		"ws:lead-run",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-review text %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[design-review:") {
		t.Errorf("body %q: unresolved design-review marker shipped", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestSkillAuthoringRelocatedOutOfRsrc replaces the former
// TestPlaybookPrintGoldenLeadSkillAuthoring golden. The authoring manual is no
// longer a shipped playbook: it moved to ai-docs/manuals/skill-authoring.md, which
// AGENTS.md binds as a mandatory pre-edit read. The relocation is only safe
// while the content actually survives at the new path, so this asserts the
// destination carries the doctrine text the old golden checked, and that the
// playbook name stays unresolvable so a reintroduction fails loudly.
func TestSkillAuthoringRelocatedOutOfRsrc(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	if _, _, err := printPlaybook(s, rsrcRoot, "lead-skill-authoring", nil, wsconfig.Options{}, "", nil); err == nil {
		t.Error("lead-skill-authoring must no longer resolve as an rsrc playbook")
	}

	relocated := filepath.Join("..", "..", "..", "ai-docs", "manuals", "skill-authoring.md")
	raw, err := os.ReadFile(relocated)
	if err != nil {
		t.Fatalf("relocated authoring manual missing: %v", err)
	}
	body := string(raw)
	// The manual carries manuals-tier `summary:` frontmatter, not the
	// rsrc playbook-serving frontmatter (`kind:`, `delegates:`, etc.) which
	// has no meaning outside rsrc and must not have come along with the move.
	// Only the frontmatter block is inspected: the body may legitimately
	// mention `kind: render` when describing playbook layouts.
	frontmatter := body
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---"); end >= 0 {
			frontmatter = body[:4+end]
		}
	}
	if !strings.Contains(frontmatter, "summary:") {
		t.Error("relocated manual is missing manuals-tier `summary:` frontmatter")
	}
	if strings.Contains(frontmatter, "kind:") {
		t.Error("relocated manual still carries rsrc playbook-serving frontmatter")
	}
}

// TestSkillsCallEnterTools verifies the routing call site survives the lead
// surface collapse: it moved off the retired lead-facing playbooks onto the
// worker playbook, which is now the only reader that routes an implementation.
// Tokens are chosen to be non-incidental: route.<mode> appears only from the
// inserted call, and target/policy are argument-level signals that cannot
// appear from surrounding prose alone.
func TestSkillsCallEnterTools(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	cases := []struct {
		playbook string
		wantAll  []string
		wantNone []string
	}{
		{
			playbook: "ticket-worker",
			wantAll:  []string{"route.resolve_implement", "target:", "policy:"},
			wantNone: []string{"## Routing Verdict"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.playbook, func(t *testing.T) {
			body, _, err := renderPlaybookBody(s, rsrcRoot, tc.playbook, nil, wsconfig.Options{}, "", "", "", nil)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%q): %v", tc.playbook, err)
			}
			for _, token := range tc.wantAll {
				if !strings.Contains(body, token) {
					t.Errorf("renderPlaybookBody(%q): rendered body does not contain %q", tc.playbook, token)
				}
			}
			for _, token := range tc.wantNone {
				if strings.Contains(body, token) {
					t.Errorf("renderPlaybookBody(%q): rendered body should not contain %q", tc.playbook, token)
				}
			}
		})
	}
}

// TestPhase3bSkillRepoint verifies the workflow-manual bootstrap stayed on the
// skill shims rather than reappearing as a playbook self-load. The original
// four repointed skills are gone or collapsed; the invariant now lives on the
// parallel-init shims, which call workflow_manual alongside playbook.read.
// A playbook body that self-loads the manual re-adds the recursion this
// repoint removed, so no rsrc body may carry that call.
func TestPhase3bSkillRepoint(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	skillsRoot := filepath.Join("..", "..", "..", "agents-plugin", "skills")
	wsflowSkillsRoot := filepath.Join("..", "..", "..", "agents-plugin-wsflow", "skills")

	for _, skill := range []string{"lead-discuss", "lead-run", "lead-ticket"} {
		t.Run(skill, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(skillsRoot, skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("read %s SKILL.md: %v", skill, err)
			}
			if !strings.Contains(string(raw), "workflow_manual") {
				t.Errorf("%s: SKILL.md must bootstrap through workflow_manual", skill)
			}
		})
	}

	// No shipped playbook body may self-load the workflow manual.
	entries, err := os.ReadDir(rsrcRoot)
	if err != nil {
		t.Fatalf("read rsrc root: %v", err)
	}
	for _, entry := range entries {
		// lead-workflow-manual is the manual itself: it states its own reload
		// invariant, so the call it names is documentation, not a self-load.
		if !entry.IsDir() || entry.Name() == "lead-workflow-manual" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(rsrcRoot, entry.Name(), entry.Name()+".md"))
		if err != nil {
			continue
		}
		if strings.Contains(string(body), `playbook.read(name: "lead-workflow-manual")`) {
			t.Errorf("%s: playbook body must not contain the removed playbook.read self-load call", entry.Name())
		}
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

// workerPlaybookContent is a worker-role playbook: the render-minted child key
// must carry the caller's lead scope, not a delegate scope.
const workerPlaybookContent = `---
kind: render
delegates: true
role: worker
tier: large
---
# Worker Playbook

Execute the ticket.
`

// TestRenderMintsLeadScopedChildKeyForWorkerPlaybook pins the worker render
// role: a `role: worker` playbook rendered by a lead caller mints a child key
// that is lead-scoped and bound to the caller's root, so the worker can call
// every lead-gated ws tool without minting a key of its own.
func TestRenderMintsLeadScopedChildKeyForWorkerPlaybook(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"worker-pb/worker-pb.md": workerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")
	mintRoot := "/work/tree-a"
	parentKey := "parent-key-fixture"

	body, _, err := renderPlaybookBody(s, root, "worker-pb", nil, wsconfig.Options{}, mintRoot, parentKey, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found in registry", key)
	}
	if entry.scope != roleLead {
		t.Errorf("minted key scope = %q, want %q (worker → lead)", entry.scope, roleLead)
	}
	if entry.root != mintRoot {
		t.Errorf("minted key root = %q, want caller root %q", entry.root, mintRoot)
	}
	if entry.parent != parentKey {
		t.Errorf("minted key parent = %q, want %q", entry.parent, parentKey)
	}
}

// TestPlaybookRenderGoldenTicketWorker resolves the shipped ticket-worker
// playbook from the real rsrc tree: the worker-stop-protocol include must be
// spliced in, the harness idiom vars must substitute, and a lead caller must
// receive a lead-scoped child key. This is the end-to-end counterpart of
// TestRenderMintsLeadScopedChildKeyForWorkerPlaybook, which uses a fixture.
func TestPlaybookRenderGoldenTicketWorker(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			for _, tc := range []struct{ name, tier string }{
				{"ticket-worker", "medium"},
				{"ticket-worker-elevated", "large"},
				{"ticket-worker-escalated", "xlarge"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rsrcRoot := filepath.Join("..", "..", "..", map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product], "rsrc")
					s := newTestServerWithHarness(t, "claude")
					mintRoot := "/work/tree-a"

					body, tier, err := renderPlaybookBody(s, rsrcRoot, tc.name, nil, wsconfig.Options{}, mintRoot, "", "", nil)
					if err != nil {
						t.Fatalf("renderPlaybookBody: %v", err)
					}
					if tier != tc.tier {
						t.Errorf("recommended tier = %q, want %q", tier, tc.tier)
					}
					// The shared protocol include must arrive with the playbook body.
					for _, want := range []string{"# Worker Protocol", "## Stop List", "status: [ok] | [escalate-to-lead]"} {
						if !strings.Contains(body, want) {
							t.Errorf("rendered body missing protocol text %q", want)
						}
					}
					// delegates:true → the harness continuity tip carries the continuation idiom.
					if !strings.Contains(body, "SendMessage(to: <agentId>)") {
						t.Errorf("rendered body missing claude continuation idiom")
					}
					// No unsubstituted template variables may survive rendering.
					if strings.Contains(body, "{{.") {
						t.Errorf("rendered body has unsubstituted variables:\n%s", body)
					}
					key := extractSplicedKey(t, body)
					entry, ok := s.sessions.lookup(key)
					if !ok {
						t.Fatalf("minted key %q not found in registry", key)
					}
					if entry.scope != roleLead || entry.root != mintRoot {
						t.Errorf("minted key = (scope %q, root %q), want (%q, %q)", entry.scope, entry.root, roleLead, mintRoot)
					}
				})
			}
		})
	}
}

// The lead interprets this policy as prose, so pin the rendered decision rows
// rather than duplicate the routing logic in a test-only Go implementation.
func TestPlaybookPrintLeadRunWorkerTierPolicy(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			packageDir := map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product]
			root := filepath.Join("..", "..", "..", packageDir, "rsrc")
			s := newTestServerWithHarness(t, "codex")
			body, _, err := printPlaybook(s, root, "lead-run", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			body = strings.Join(strings.Fields(body), " ")
			for _, want := range []string{
				product + `/tickets.query(ticket_stem: "<stem>")`,
				"do not read or summarize the ticket body",
				"`risk.correctness`, `risk.fit`, `risk.test`, and `risk.security_or_contract`",
				"| Ticket: any risk is `high` | `ticket-worker-elevated` | large |",
				"| Ticket: all risks are `low`, `moderate`, or `unknown` | `ticket-worker` | medium |",
				"| Ad hoc: routine | `ticket-worker` | medium |",
				"| Ad hoc: difficult | `ticket-worker-elevated` | large |",
				"moderate risk still keeps its existing independent-review breadth",
				"Spawn one worker at the tier the render recommends",
				"playbook <chosen worker playbook>; stop-e retries <0 or 1>",
				"| `ticket-worker` | `ticket-worker-elevated` | large |",
				"| `ticket-worker-elevated` | `ticket-worker-escalated` | xlarge |",
				"A second (e) goes to the user",
				"do not reset the retry count on resume or reclassify the original risks",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("rendered policy missing %q", want)
				}
			}
			if strings.Contains(body, "flagship class") || strings.Contains(body, "{{.") {
				t.Error("rendered policy retains a fixed flagship floor or template variable")
			}
		})
	}
}

func TestTicketWorkerVariantsDifferOnlyByTier(t *testing.T) {
	root := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	base, err := os.ReadFile(filepath.Join(root, "ticket-worker", "ticket-worker.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, tier := range map[string]string{"ticket-worker-elevated": "large", "ticket-worker-escalated": "xlarge"} {
		data, err := os.ReadFile(filepath.Join(root, name, name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Replace(string(data), "tier: "+tier+"\n", "tier: medium\n", 1); got != string(base) {
			t.Errorf("%s differs from the base beyond tier frontmatter", name)
		}
	}
}
