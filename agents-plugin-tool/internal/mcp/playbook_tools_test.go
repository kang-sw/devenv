package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/claudeagents"
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

	neutral := terminologyForHarness("", RuntimeNamespace())
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

	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Codex terms must NOT appear for any var (proves harness selection on all vars).
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
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

// TestPlaybookPrintPiHarnessUsesPiTerminology pins the exact native Pi idioms
// (the ws Pi extension's tools) that delegate playbooks receive; a Pi session
// without that extension gets the MCP-only row instead
// (playbook_pi_mcp_only_test.go). Structural overlays remain independently
// tested; terminology substitution must not rely on one.
func TestPlaybookPrintPiHarnessUsesPiTerminology(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "pi")

	body, _, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	want := map[string]string{
		"ExploreAgent":  "the `explore` researcher",
		"SpawnIdiom":    "ws-agent-spawn",
		"ContinueIdiom": "ws-agent-send to the same alias or agent id",
	}
	piTerm := terminologyForHarness("pi", RuntimeNamespace())
	for varName, term := range want {
		if piTerm[varName] != term {
			t.Errorf("terminologyForHarness(pi)[%s] = %q, want %q", varName, piTerm[varName], term)
		}
		if !strings.Contains(body, term) {
			t.Errorf("body %q: expected Pi %s %q", body, varName, term)
		}
	}
}

// TestPlaybookPrintPiDelegationFlowPlaybooks exercises the shared sources that
// dogfood relies on: their Pi render must need terminology substitution only,
// without a structural .pi.md overlay.
func TestPlaybookPrintPiDelegationFlowPlaybooks(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "pi")
	piTerm := terminologyForHarness("pi", RuntimeNamespace())

	cases := map[string][]string{
		"lead-discuss":  {"ExploreAgent", "ContinueIdiom"},
		"lead-run":      {"SpawnIdiom"},
		"ticket-worker": {"ExploreAgent", "SpawnIdiom", "ContinueIdiom"},
	}
	for name, variableNames := range cases {
		t.Run(name, func(t *testing.T) {
			body, _, err := printPlaybook(s, rsrcRoot, name, nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			for _, variableName := range variableNames {
				if !strings.Contains(body, piTerm[variableName]) {
					t.Errorf("body missing Pi %s %q:\n%s", variableName, piTerm[variableName], body)
				}
			}
		})
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

	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
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
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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

	path, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "delegate-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
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
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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
	if !strings.Contains(body, "gpt-6-luna") {
		t.Errorf("body %q: expected seeded default medium-tier model gpt-6-luna", body)
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
// small=medium=gpt-6-luna (the shipped Codex tier remap).
func TestPlaybookPrintGoldenLeadWorkflowManualScopedExplorationTierModels(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")

	cases := []struct {
		harness     string
		smallModel  string
		mediumModel string
	}{
		{"claude", "haiku", "sonnet"},
		{"codex", "gpt-6-luna", "gpt-6-luna"},
	}
	for _, tc := range cases {
		t.Run(tc.harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, tc.harness)
			body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			for _, token := range []string{terminologyForHarness(tc.harness, RuntimeNamespace())["ExploreAgent"], tc.smallModel, tc.mediumModel} {
				if !strings.Contains(body, token) {
					t.Errorf("body missing substituted token %q", token)
				}
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
		for _, want := range []string{"wsflow/", "wsflow:"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: wsflow playbook output missing %q:\n%s", label, want, body)
			}
		}
		if !strings.Contains(body, "ferrule") {
			t.Fatalf("%s: wsflow playbook output rewrote literal ws.ferrule tool name:\n%s", label, body)
		}
	}

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, configOpts, "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent off", body)
	if strings.Contains(body, "Use `wsflow:lead-delegate` for bounded investigation") {
		t.Fatalf("wsflow workflow manual must not append lead-prefer-subagent while preference is off:\n%s", body)
	}

	resolver := wsconfig.NewResolver(configOpts, builtinConfigDefaults(), nil, nil)
	if err := resolver.Set(wsconfig.ItemWorkflowPreferSubagent, "on", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("enable workflow.prefer_subagent: %v", err)
	}
	bodyOn, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, configOpts, "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook on: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent on", bodyOn)
	for _, want := range []string{
		"Use `wsflow:lead-delegate` for bounded investigation",
		"wsflow:lead-delegate",
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

	body, _, err := printPlaybook(s, rsrcRoot, "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-tune rendered empty")
	}
}

func TestPlaybookPrintWsflowLeadTuneOmitsFullWsOnlyCatalogKnobs(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune wsflow: %v", err)
	}
	if !strings.Contains(body, "wsflow/config.list") {
		t.Fatal("lead-tune render missing substituted namespace")
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
	wsflow := renderProductModePlaybookBody(input, NoAgentMode())
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
	full := renderProductModePlaybookBody(input, NoAgentMode())
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

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
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

	path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
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
	for _, value := range shippedImplementerContext() {
		if !strings.Contains(body, value) {
			t.Fatalf("implementer render missing substituted value %q", value)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Fatal("implementer render contains unresolved variables")
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
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
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
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for code-reviewer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	ctx := shippedImplementerContext()
	ctx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	implCtx := shippedImplementerContext()
	implCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "implementer", implCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
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
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("wsflow non-legacy renderPlaybook accepted undeclared context")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("wsflow non-legacy renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
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
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
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

	neutralTerm := terminologyForHarness("", RuntimeNamespace())
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
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
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

	neutralTerm := terminologyForHarness("", RuntimeNamespace())
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
	neutralTerm := terminologyForHarness("", RuntimeNamespace())
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
	for _, harness := range []string{"claude", "codex", "pi", ""} {
		if _, ok := playbookTerminologyTable[harness]; !ok {
			t.Errorf("terminology table missing harness entry %q", harness)
			continue
		}
		// The effective table: claude's SpawnIdiom is added at resolve time.
		tbl := terminologyForHarness(harness, RuntimeNamespace())
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
	claude := terminologyForHarness("claude", RuntimeNamespace())
	codex := terminologyForHarness("codex", RuntimeNamespace())
	neutral := terminologyForHarness("", RuntimeNamespace())
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
		"SmallTierReasoningEffort", "MediumTierReasoningEffort", "LargeTierReasoningEffort", "XLargeTierReasoningEffort",
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
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-check-blockers rendered empty")
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
// and points compaction recovery at the state-restoring lead-revive entry.
func TestPlaybookPrintGoldenLeadWorkflowManual(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-workflow-manual rendered empty")
	}
	if !strings.Contains(body, `ws:lead-revive`) {
		t.Errorf("body %q: expected compaction recovery instruction using lead-revive", body)
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
// real rsrc tree under both harness variants without naming a retired entry
// point. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadTicket(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, harness)

			body, _, err := printPlaybook(s, rsrcRoot, "lead-ticket", nil, wsconfig.Options{}, "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
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
		})
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
// rsrc tree with no unresolved marker. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadShip(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-ship", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.Contains(body, "[design-review:") {
		t.Errorf("body %q: unresolved design-review marker shipped", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

func TestPlaybookDocumentAuditProducts(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			packageDir := map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product]
			rsrcRoot := filepath.Join("..", "..", "..", packageDir, "rsrc")
			s := newTestServerWithHarness(t, "codex")
			opts := isolatedPlaybookConfigOptions(t)
			body, _, err := printPlaybook(s, rsrcRoot, "lead-audit-doc", nil, opts, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, product+"/playbook.render") {
				t.Error("lead render missing substituted namespace")
			}
			if _, _, err := printPlaybook(s, rsrcRoot, "lead-add-rule", nil, opts, "", nil); err == nil {
				t.Fatal("retired playbook still resolves")
			}
			// A downstream target needs only its document and the installed bundle.
			worktreeRoot := initGitRepo(t)
			target := filepath.Join(worktreeRoot, "AGENTS.md")
			if err := os.WriteFile(target, []byte("# Project\n\nUse the existing conventions.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			path, tier, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "fresh-read-doc-auditor", map[string]string{"TargetFiles": target}, opts, "", "", "", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if tier != "large" {
				t.Errorf("auditor tier = %q, want large", tier)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			audit := string(data)
			for _, want := range []string{target} {
				if !strings.Contains(audit, want) {
					t.Errorf("auditor missing %q", want)
				}
			}
			for _, forbidden := range []string{"{{.", "skill-authoring", "ai-docs/manuals/", "Continuity tip"} {
				if strings.Contains(audit, forbidden) {
					t.Errorf("auditor contains %q", forbidden)
				}
			}
		})
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
	for _, want := range []string{"ws:lead-delegate", "ws:lead-ticket", "ws:lead-run"} {
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
	for _, want := range []string{"ws:lead-run", "ws/git.commit"} {
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
	}{
		{
			playbook: "ticket-worker",
		},
	}

	for _, tc := range cases {
		t.Run(tc.playbook, func(t *testing.T) {
			body, _, err := renderPlaybookBody(s, rsrcRoot, tc.playbook, nil, wsconfig.Options{}, "", "", "", nil, "")
			if err != nil {
				t.Fatalf("renderPlaybookBody(%q): %v", tc.playbook, err)
			}
			if strings.TrimSpace(body) == "" {
				t.Errorf("renderPlaybookBody(%q) returned empty text", tc.playbook)
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
	skillsRoot := filepath.Join("..", "..", "..", "agents-plugin", "skills")
	wsflowSkillsRoot := filepath.Join("..", "..", "..", "agents-plugin-wsflow", "skills")

	for _, skill := range []string{"lead-discuss", "lead-run", "lead-ticket"} {
		t.Run(skill, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(skillsRoot, skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("read %s SKILL.md: %v", skill, err)
			}
			if len(raw) == 0 {
				t.Errorf("%s: SKILL.md is empty", skill)
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
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-bootstrap rendered empty")
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

	body, _, err := renderPlaybookBody(s, root, "worker-pb", nil, wsconfig.Options{}, mintRoot, parentKey, "", nil, "")
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
			} {
				t.Run(tc.name, func(t *testing.T) {
					rsrcRoot := filepath.Join("..", "..", "..", map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product], "rsrc")
					s := newTestServerWithHarness(t, "claude")
					mintRoot := "/work/tree-a"

					body, tier, err := renderPlaybookBody(s, rsrcRoot, tc.name, nil, wsconfig.Options{}, mintRoot, "", "", nil, "")
					if err != nil {
						t.Fatalf("renderPlaybookBody: %v", err)
					}
					if tier != tc.tier {
						t.Errorf("recommended tier = %q, want %q", tier, tc.tier)
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
			if strings.TrimSpace(body) == "" {
				t.Fatal("lead-run rendered empty")
			}
			if strings.Contains(body, "{{.") {
				t.Error("rendered policy retains a template variable")
			}
		})
	}
}

// Pin the real query boundary the lead consumes: compact text omits Route
// Facts, so the policy must request the existing JSON projection explicitly.
func TestLeadRunTicketQueryRiskProjection(t *testing.T) {
	root := initGitRepo(t)
	mustWrite(t, root, "ai-docs/tickets/ready/260910-feat-risk-fixture.md", `---
title: Risk fixture
---
## Route Facts

| fact | value | evidence |
|---|---|---|
| risk.correctness | high | fixture |
| risk.fit | low | fixture |
| risk.test | moderate | fixture |
| risk.security_or_contract | unknown | fixture |
`)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newTestServerWithHarness(t, "codex")
	key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	resp := callToolOnce(t, s, 1, "tickets.query", map[string]any{
		"session_key": key,
		"ticket_stem": "260910-feat-risk-fixture",
		"format":      "json",
	})
	var projection struct {
		Present bool              `json:"route_facts_present"`
		Facts   map[string]string `json:"route_facts"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &projection); err != nil {
		t.Fatal(err)
	}
	if !projection.Present {
		t.Fatal("point query did not expose Route Facts presence")
	}
	for fact, want := range map[string]string{
		"risk.correctness": "high", "risk.fit": "low",
		"risk.test": "moderate", "risk.security_or_contract": "unknown",
	} {
		if got := projection.Facts[fact]; got != want {
			t.Errorf("projected %s = %q, want %q", fact, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Claude effort-carrier SpawnIdiom
// ---------------------------------------------------------------------------

// spawnIdiomSites are the real playbooks that substitute {{.SpawnIdiom}}.
var spawnIdiomSites = []string{"lead-run", "ticket-worker", "ticket-worker-elevated", "explore", "delegate-sample"}

// wantClaudeSpawnIdiom is the hardcoded Claude SpawnIdiom per namespace, so a
// wrong claudeSpawnIdiom cannot agree with itself in a derived assertion.
func wantClaudeSpawnIdiom(namespace string) string {
	return `Agent({subagent_type: "` + namespace + `:effort-<resolved effort>", model: <resolved model>, ...})` +
		` (subagent_type "general-purpose" when the resolved effort is not exactly low, medium, high, xhigh, or max)`
}

// TestClaudeSpawnIdiomRendersAtEverySite pins the Claude SpawnIdiom text as
// rendered at each {{.SpawnIdiom}} site for both plugin namespaces: the value
// carries the effort-agent mapping rule and its general-purpose fallback, and
// the namespace comes from the runtime, never a hard-coded ws: prefix.
func TestClaudeSpawnIdiomRendersAtEverySite(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			want := wantClaudeSpawnIdiom(namespace)
			if got := terminologyForHarness("claude", RuntimeNamespace())["SpawnIdiom"]; got != want {
				t.Fatalf("claude SpawnIdiom = %q, want %q", got, want)
			}
			s := newTestServerWithHarness(t, "claude")
			for _, name := range spawnIdiomSites {
				body, _, err := printPlaybook(s, rsrcRoot, name, nil, isolatedPlaybookConfigOptions(t), "", nil)
				if err != nil {
					t.Fatalf("printPlaybook(%s): %v", name, err)
				}
				if !strings.Contains(body, want) {
					t.Errorf("%s: Claude render missing SpawnIdiom %q:\n%s", name, want, body)
				}
				if strings.Contains(body, "{{.") {
					t.Errorf("%s: unsubstituted placeholder remains", name)
				}
			}
		})
	}
}

// TestSpawnIdiomSitesListIsComplete keeps spawnIdiomSites equal to the real
// playbook directories that use {{.SpawnIdiom}}, so a new site cannot go
// unpinned. lead-workflow-manual uses it only through its Claude overlay,
// which TestLeadWorkflowManualClaudeSpawnBindingOverlay pins.
func TestSpawnIdiomSitesListIsComplete(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	found := map[string]bool{}
	err := filepath.WalkDir(rsrcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "{{.SpawnIdiom}}") {
			found[filepath.Base(filepath.Dir(path))] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk rsrc: %v", err)
	}
	delete(found, "lead-workflow-manual")
	want := map[string]bool{}
	for _, name := range spawnIdiomSites {
		want[name] = true
	}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("{{.SpawnIdiom}} sites = %v, spawnIdiomSites = %v", found, want)
	}
}

// TestClaudeSpawnIdiomLevelsMatchGeneratedAgents keeps the level list the rule
// names equal to the agents the generator ships.
func TestClaudeSpawnIdiomLevelsMatchGeneratedAgents(t *testing.T) {
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if strings.Join(claudeagents.Levels, ",") != strings.Join(want, ",") {
		t.Fatalf("claudeagents.Levels = %v, want %v (update the pinned SpawnIdiom with it)", claudeagents.Levels, want)
	}
}

// TestLeadWorkflowManualClaudeSpawnBindingOverlay pins the lead-side Claude
// overlay for both namespaces: it applies render bindings through the
// SpawnIdiom rule, keeps ExploreAgent on the built-in Explore agent, and uses
// its own heading rather than Codex wording.
func TestLeadWorkflowManualClaudeSpawnBindingOverlay(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			s := newTestServerWithHarness(t, "claude")
			body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			want := wantClaudeSpawnIdiom(namespace)
			if !strings.Contains(body, want) {
				t.Errorf("Claude workflow manual missing overlay %q:\n%s", want, body)
			}
			if terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"] != "the Explore agent" {
				t.Errorf("claude ExploreAgent changed: %q", terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"])
			}
			if !strings.Contains(body, terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"]) {
				t.Errorf("Claude workflow manual lost the built-in Explore agent wording:\n%s", body)
			}
		})
	}
}

// TestNonClaudeRendersOmitEffortAgents asserts the effort agents are a
// Claude-only surface: Codex, Pi, and host-neutral renders never mention them.
func TestNonClaudeRendersOmitEffortAgents(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	names := append([]string{"lead-workflow-manual"}, spawnIdiomSites...)
	for _, harness := range []string{"codex", "pi", ""} {
		s := newTestServerWithHarness(t, harness)
		for _, name := range names {
			body, _, err := printPlaybook(s, rsrcRoot, name, nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook(%s, %q): %v", name, harness, err)
			}
			for _, forbidden := range []string{":effort-", "effort-<resolved effort>"} {
				if strings.Contains(body, forbidden) {
					t.Errorf("harness %q %s render mentions Claude effort agents (%q)", harness, name, forbidden)
				}
			}
		}
	}
}

// mustOverrideLookup builds the session-anchored prompt-override lookup and
// fails the test when any config scope fails to load.
func mustOverrideLookup(t *testing.T, s *Server, sessionKey string) overrideLookupFn {
	t.Helper()
	lookup, err := buildOverrideLookup(s, sessionKey)
	if err != nil {
		t.Fatalf("buildOverrideLookup: %v", err)
	}
	return lookup
}
