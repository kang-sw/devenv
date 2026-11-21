// playbook_vars_test.go checks playbook caller, model, and namespace variables.
package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// playbook.read — caller context substitution
// ---------------------------------------------------------------------------

func TestPlaybookPrintCallerContextSubstituted(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "plain-pb", map[string]string{"WorktreeID": "wt-abc"}, wsconfig.Options{}, "", nil)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "novars", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Static content only") {
		t.Errorf("body %q: expected static content", body)
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
	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
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

	bodyA, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "model-pb", nil, wsconfig.Options{CacheHome: cacheA}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook A: %v", err)
	}
	bodyB, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "model-pb", nil, wsconfig.Options{CacheHome: cacheB}, "", nil)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "model-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "gpt-6.1-terra") {
		t.Errorf("body %q: expected seeded default medium-tier model gpt-6.1-terra", body)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "tier-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "tier-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", nil)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "namespace-pb", map[string]string{
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "namespace-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"ws/tickets.query", "ws:lead-discuss"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, body)
		}
	}
}
