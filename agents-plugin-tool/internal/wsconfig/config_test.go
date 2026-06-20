package wsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAgentsTierInfersBackend(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	cfg, err := SetAgentsTier(Options{CacheHome: cache}, "small", "", "claude-sonnet-4")
	if err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	if got := cfg.Agents.Tiers["small"].Backend; got != "claude" {
		t.Fatalf("backend = %q", got)
	}
	backend, model, err := ResolveAgent(Options{CacheHome: cache}, "small", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "claude" || model != "claude-sonnet-4" {
		t.Fatalf("resolved backend/model = %q/%q", backend, model)
	}
}

func TestResolveAgentExplicitModelWinsAndInfersBackend(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "medium", "codex", "gpt-5.2"); err != nil {
		t.Fatal(err)
	}
	backend, model, err := ResolveAgent(Options{CacheHome: cache}, "medium", "", "claude-sonnet-4")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "claude" || model != "claude-sonnet-4" {
		t.Fatalf("resolved backend/model = %q/%q", backend, model)
	}
}

func TestResolveAgentDefaultCoreModel(t *testing.T) {
	backend, model, err := ResolveAgent(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "sonnet", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("resolved backend/model = %q/%q", backend, model)
	}
}

func TestResolveAgentDefaultTierModels(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	tests := []struct {
		tier  string
		model string
	}{
		{tier: "small", model: "gpt-5.4-mini"},
		{tier: "medium", model: "gpt-5.5"},
		{tier: "large", model: "gpt-5.5"},
		{tier: "xlarge", model: "gpt-5.5"},
	}
	for _, tc := range tests {
		t.Run(tc.tier, func(t *testing.T) {
			backend, model, err := ResolveAgent(Options{CacheHome: cache}, tc.tier, "", "")
			if err != nil {
				t.Fatalf("ResolveAgent returned error: %v", err)
			}
			if backend != "codex" || model != tc.model {
				t.Fatalf("resolved backend/model = %q/%q", backend, model)
			}
		})
	}
}

// TestResolveAgentLegacyTierSynonyms verifies that legacy tier names
// (light/core/deep) still resolve unchanged via normalizedTier synonym support.
func TestResolveAgentLegacyTierSynonyms(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	tests := []struct {
		tier  string
		model string
	}{
		{tier: "light", model: "gpt-5.4-mini"}, // light → small
		{tier: "core", model: "gpt-5.5"},        // core → medium
		{tier: "deep", model: "gpt-5.5"},        // deep → large
	}
	for _, tc := range tests {
		t.Run(tc.tier, func(t *testing.T) {
			backend, model, err := ResolveAgent(Options{CacheHome: cache}, tc.tier, "", "")
			if err != nil {
				t.Fatalf("ResolveAgent(%q) returned error: %v", tc.tier, err)
			}
			if backend != "codex" || model != tc.model {
				t.Fatalf("resolved backend/model = %q/%q", backend, model)
			}
		})
	}
}

func TestResolveAgentModelAliasUsesHarness(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "", "", "medium", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "claude" || model != "sonnet" {
		t.Fatalf("resolved claude alias = %q/%q", backend, model)
	}

	backend, model, err = ResolveAgentForHarness(Options{CacheHome: cache}, "", "", "medium", "codex")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("resolved codex alias = %q/%q", backend, model)
	}
}

func TestResolveAgentConcreteModelWinsOverHarnessAlias(t *testing.T) {
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "large", "", "gpt-5.5", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("resolved concrete model = %q/%q", backend, model)
	}
}

func TestResolveAgentLegacyTierUsesHarness(t *testing.T) {
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "large", "", "", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "claude" || model != "opus" {
		t.Fatalf("resolved legacy tier = %q/%q", backend, model)
	}
}

func TestSetAgentsTierDoesNotOverwriteOtherBackendAliasMappings(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "medium", "", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}

	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "medium", "codex", "", "")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("explicit codex alias was overwritten = %q/%q", backend, model)
	}

	backend, model, err = ResolveAgent(Options{CacheHome: cache}, "medium", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "claude" || model != "claude-sonnet-4-6" {
		t.Fatalf("default alias mapping = %q/%q", backend, model)
	}
}

func TestSetAgentsTierForHarnessTargetsHarnessAlias(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	cfg, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "codex", "gpt-5.4", "claude")
	if err != nil {
		t.Fatalf("SetAgentsTierForHarness returned error: %v", err)
	}
	if mapping := cfg.Agents.ModelAliases["medium"]["claude"]; mapping.Backend != "codex" || mapping.Model != "gpt-5.4" {
		t.Fatalf("claude alias mapping = %#v", mapping)
	}
	if mapping := cfg.Agents.ModelAliases["medium"]["default"]; mapping.Backend != "codex" || mapping.Model != "gpt-5.5" {
		t.Fatalf("default alias mapping was overwritten = %#v", mapping)
	}

	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "medium", "", "", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.4" {
		t.Fatalf("claude harness resolution = %q/%q", backend, model)
	}
}

func TestSetAgentsTierForHarnessStoresEffortWithoutModelChange(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	cfg, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "", "", "codex", "medium")
	if err != nil {
		t.Fatalf("SetAgentsTierForHarness returned error: %v", err)
	}
	mapping := cfg.Agents.ModelAliases["medium"]["codex"]
	if mapping.Backend != "codex" || mapping.Model != "gpt-5.5" || mapping.Effort != "medium" {
		t.Fatalf("codex medium alias mapping = %#v", mapping)
	}
	if legacy := cfg.Agents.Tiers["medium"]; legacy.Effort != "" {
		t.Fatalf("default tier effort was overwritten = %#v", legacy)
	}

	backend, model, effort, err := ResolveAgentForHarnessConfig(Options{CacheHome: cache}, "medium", "", "", "codex")
	if err != nil {
		t.Fatalf("ResolveAgentForHarnessConfig returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" || effort != "medium" {
		t.Fatalf("resolved backend/model/effort = %q/%q/%q", backend, model, effort)
	}
}

func TestSetAgentsTierForHarnessClearsNoneEffort(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "large", "", "", "codex", "high"); err != nil {
		t.Fatalf("SetAgentsTierForHarness high returned error: %v", err)
	}
	cfg, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "large", "", "", "codex", "none")
	if err != nil {
		t.Fatalf("SetAgentsTierForHarness none returned error: %v", err)
	}
	if mapping := cfg.Agents.ModelAliases["large"]["codex"]; mapping.Effort != "" {
		t.Fatalf("effort was not cleared = %#v", mapping)
	}
}

func TestSetAgentsTierForHarnessClearsEffortWhenOmittedFromMappingUpdate(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "", "", "codex", "medium"); err != nil {
		t.Fatalf("SetAgentsTierForHarness effort returned error: %v", err)
	}
	cfg, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "codex", "gpt-5.4", "codex")
	if err != nil {
		t.Fatalf("SetAgentsTierForHarness model returned error: %v", err)
	}
	if mapping := cfg.Agents.ModelAliases["medium"]["codex"]; mapping.Backend != "codex" || mapping.Model != "gpt-5.4" || mapping.Effort != "" {
		t.Fatalf("effort was not cleared = %#v", mapping)
	}
}

func TestSetAgentsTierForHarnessClearsEffortWhenOnlyTierProvided(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "", "", "codex", "medium"); err != nil {
		t.Fatalf("SetAgentsTierForHarness effort returned error: %v", err)
	}
	cfg, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "", "", "codex")
	if err != nil {
		t.Fatalf("SetAgentsTierForHarness tier-only returned error: %v", err)
	}
	if mapping := cfg.Agents.ModelAliases["medium"]["codex"]; mapping.Backend != "codex" || mapping.Model != "gpt-5.5" || mapping.Effort != "" {
		t.Fatalf("tier-only update did not preserve model while clearing effort = %#v", mapping)
	}
}

func TestSetAgentsTierForHarnessRejectsInvalidEffort(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "", "", "codex", "max"); err == nil {
		t.Fatal("SetAgentsTierForHarness accepted invalid effort")
	}
}

func TestSetAgentsTierForHarnessRejectsUnknownHarness(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTierForHarness(Options{CacheHome: cache}, "medium", "codex", "gpt-5.4", "unknown"); err == nil {
		t.Fatal("SetAgentsTierForHarness accepted unknown harness")
	}
}

func TestResolveAgentExplicitBackendDoesNotBorrowCrossBackendModel(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	path, err := Path(Options{CacheHome: cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Config uses capability key "medium" directly.
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "agents": {
    "tiers": {
      "medium": {
        "backend": "claude",
        "model": "claude-sonnet-4-6"
      }
    },
    "model_aliases": {
      "medium": {
        "default": {
          "backend": "claude",
          "model": "claude-sonnet-4-6"
        }
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "medium", "codex", "", "")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "" {
		t.Fatalf("explicit codex should not borrow claude model = %q/%q", backend, model)
	}
}

func TestShowReturnsPathAndDefaultWithoutCreatingFile(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	view, err := Show(Options{CacheHome: cache})
	if err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	wantPath := filepath.Join(cache, "config.json")
	if view.Path != wantPath {
		t.Fatalf("path = %q, want %q", view.Path, wantPath)
	}
	if view.Config.SchemaVersion != schemaVersion {
		t.Fatalf("schema_version = %d", view.Config.SchemaVersion)
	}
	if len(view.Config.Agents.Tiers) != 4 {
		t.Fatalf("default tiers = %#v", view.Config.Agents.Tiers)
	}
	if small := view.Config.Agents.Tiers["small"]; small.Backend != "codex" || small.Model != "gpt-5.4-mini" {
		t.Fatalf("default small tier = %#v", small)
	}
	if medium := view.Config.Agents.Tiers["medium"]; medium.Backend != "codex" || medium.Model != "gpt-5.5" {
		t.Fatalf("default medium tier = %#v", medium)
	}
	if large := view.Config.Agents.Tiers["large"]; large.Backend != "codex" || large.Model != "gpt-5.5" {
		t.Fatalf("default large tier = %#v", large)
	}
	if xlarge := view.Config.Agents.Tiers["xlarge"]; xlarge.Backend != "codex" || xlarge.Model != "gpt-5.5" {
		t.Fatalf("default xlarge tier = %#v", xlarge)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("Show created config file or stat failed: %v", err)
	}
}

func TestShowReturnsConfiguredMapping(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "small", "", "claude-sonnet-4"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	view, err := Show(Options{CacheHome: cache})
	if err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	mapping := view.Config.Agents.Tiers["small"]
	if mapping.Backend != "claude" || mapping.Model != "claude-sonnet-4" {
		t.Fatalf("small mapping = %#v", mapping)
	}
}

// TestLoadReadCompatLegacyKeys is the load-bearing read-compat test: a config.json
// written with legacy keys (light/core/deep) must resolve custom models through
// the capability vocabulary after load-time key migration.
func TestLoadReadCompatLegacyKeys(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	path, err := Path(Options{CacheHome: cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a persisted config.json written before the capability vocabulary
	// migration: keys are light/core/deep, custom models differ from defaults.
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "agents": {
    "tiers": {
      "light": {
        "backend": "gemini",
        "model": "gemini-3-1-pro"
      }
    },
    "model_aliases": {
      "light": {
        "claude": {
          "backend": "claude",
          "model": "X-custom-light"
        }
      },
      "core": {
        "default": {
          "backend": "codex",
          "model": "gpt-custom-core"
        }
      },
      "deep": {
        "default": {
          "backend": "codex",
          "model": "gpt-custom-deep"
        }
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{CacheHome: cache})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Legacy light → capability small: tiers map should be keyed by "small".
	if small := cfg.Agents.Tiers["small"]; small.Backend != "gemini" || small.Model != "gemini-3-1-pro" {
		t.Fatalf("small tier from legacy light = %#v", small)
	}
	if _, ok := cfg.Agents.Tiers["light"]; ok {
		t.Fatal("legacy light tier key must be removed after migration")
	}

	// medium and large defaults must be backfilled.
	if medium := cfg.Agents.Tiers["medium"]; medium.Backend != "codex" || medium.Model != "gpt-5.5" {
		t.Fatalf("medium tier (backfilled) = %#v", medium)
	}
	if large := cfg.Agents.Tiers["large"]; large.Backend != "codex" || large.Model != "gpt-5.5" {
		t.Fatalf("large tier (backfilled) = %#v", large)
	}

	// Alias custom models must resolve through capability keys.
	// light → small: claude harness → X-custom-light (NOT the built-in haiku default).
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "small", "", "", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness small/claude: %v", err)
	}
	if backend != "claude" || model != "X-custom-light" {
		t.Fatalf("small/claude resolved = %q/%q, want claude/X-custom-light", backend, model)
	}

	// Querying by legacy "light" synonym must also resolve.
	backend, model, err = ResolveAgentForHarness(Options{CacheHome: cache}, "light", "", "", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness light/claude: %v", err)
	}
	if backend != "claude" || model != "X-custom-light" {
		t.Fatalf("light/claude synonym resolved = %q/%q, want claude/X-custom-light", backend, model)
	}

	// core → medium: default harness → gpt-custom-core.
	backend, model, err = ResolveAgent(Options{CacheHome: cache}, "medium", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent medium: %v", err)
	}
	if backend != "codex" || model != "gpt-custom-core" {
		t.Fatalf("medium resolved = %q/%q, want codex/gpt-custom-core", backend, model)
	}

	// deep → large: default harness → gpt-custom-deep.
	backend, model, err = ResolveAgent(Options{CacheHome: cache}, "large", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent large: %v", err)
	}
	if backend != "codex" || model != "gpt-custom-deep" {
		t.Fatalf("large resolved = %q/%q, want codex/gpt-custom-deep", backend, model)
	}
}

func TestLoadBackfillsMissingDefaultTiers(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	path, err := Path(Options{CacheHome: cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Uses capability key "small" directly (post-migration config).
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "agents": {
    "tiers": {
      "small": {
        "backend": "gemini",
        "model": "gemini-3-1-pro"
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{CacheHome: cache})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if small := cfg.Agents.Tiers["small"]; small.Backend != "gemini" || small.Model != "gemini-3-1-pro" {
		t.Fatalf("small mapping was overwritten: %#v", small)
	}
	if medium := cfg.Agents.Tiers["medium"]; medium.Backend != "codex" || medium.Model != "gpt-5.5" {
		t.Fatalf("medium mapping = %#v", medium)
	}
	if large := cfg.Agents.Tiers["large"]; large.Backend != "codex" || large.Model != "gpt-5.5" {
		t.Fatalf("large mapping = %#v", large)
	}
}

// TestSetAgentsTierRejectsInvalidTier verifies the new capability error message.
func TestSetAgentsTierRejectsInvalidTier(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	_, err := SetAgentsTier(Options{CacheHome: cache}, "bogus", "", "gpt-5.5")
	if err == nil {
		t.Fatal("SetAgentsTier accepted invalid tier")
	}
	if got := err.Error(); got != "tier must be small, medium, large, or xlarge" {
		t.Fatalf("unexpected error message: %q", got)
	}
}

// TestSetAgentsTierLegacySynonymsAccepted verifies that legacy tier names
// (light/core/deep) are still accepted by SetAgentsTier as synonyms.
func TestSetAgentsTierLegacySynonymsAccepted(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	// light → small
	cfg, err := SetAgentsTier(Options{CacheHome: cache}, "light", "", "claude-custom-light")
	if err != nil {
		t.Fatalf("SetAgentsTier light returned error: %v", err)
	}
	if m := cfg.Agents.Tiers["small"]; m.Model != "claude-custom-light" {
		t.Fatalf("light synonym did not write to small: %#v", m)
	}
}

// TestModelAliasCapabilityVocabulary verifies that ModelAlias returns capability
// keys for alias names and "" for concrete model names (haiku/sonnet/opus must NOT
// be treated as alias redirects).
func TestModelAliasCapabilityVocabulary(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"small", "small"},
		{"light", "small"},
		{"medium", "medium"},
		{"core", "medium"},
		{"large", "large"},
		{"deep", "large"},
		{"xlarge", "xlarge"},
		// Concrete model names must NOT be alias-redirected (regression guard).
		{"haiku", ""},
		{"sonnet", ""},
		{"opus", ""},
		{"gpt-5.5", ""},
		{"claude-sonnet-4", ""},
		{"", ""},
		{"bogus", ""},
	}
	for _, tc := range cases {
		if got := ModelAlias(tc.input); got != tc.want {
			t.Errorf("ModelAlias(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestXlargeIndependentOfLarge verifies that xlarge and large are independently
// configurable: setting one does not change the other, and unset xlarge defaults
// to the same model as large.
func TestXlargeIndependentOfLarge(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")

	// Unset xlarge defaults to the large default.
	backendL, modelL, err := ResolveAgent(Options{CacheHome: cache}, "large", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent large: %v", err)
	}
	backendX, modelX, err := ResolveAgent(Options{CacheHome: cache}, "xlarge", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent xlarge: %v", err)
	}
	if backendL != backendX || modelL != modelX {
		t.Fatalf("unset xlarge should default to large: large=%q/%q xlarge=%q/%q", backendL, modelL, backendX, modelX)
	}

	// Configure large with a custom model.
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "large", "", "gpt-custom-large"); err != nil {
		t.Fatalf("SetAgentsTier large: %v", err)
	}

	// xlarge must NOT pick up the large custom model.
	backendX, modelX, err = ResolveAgent(Options{CacheHome: cache}, "xlarge", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent xlarge after large change: %v", err)
	}
	if modelX == "gpt-custom-large" {
		t.Fatal("setting large must not change xlarge resolved model")
	}

	// Now configure xlarge independently.
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "xlarge", "", "gpt-custom-xlarge"); err != nil {
		t.Fatalf("SetAgentsTier xlarge: %v", err)
	}
	backendX, modelX, err = ResolveAgent(Options{CacheHome: cache}, "xlarge", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent xlarge after xlarge set: %v", err)
	}
	if backendX != "codex" || modelX != "gpt-custom-xlarge" {
		t.Fatalf("xlarge after set = %q/%q, want codex/gpt-custom-xlarge", backendX, modelX)
	}

	// large must still return its custom model after xlarge was set.
	backendL, modelL, err = ResolveAgent(Options{CacheHome: cache}, "large", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent large after xlarge set: %v", err)
	}
	if backendL != "codex" || modelL != "gpt-custom-large" {
		t.Fatalf("large after xlarge set = %q/%q, want codex/gpt-custom-large", backendL, modelL)
	}
}
