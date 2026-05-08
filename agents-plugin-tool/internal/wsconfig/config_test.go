package wsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAgentsTierInfersBackend(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	cfg, err := SetAgentsTier(Options{CacheHome: cache}, "light", "", "gemini-3-1-pro")
	if err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	if got := cfg.Agents.Tiers["light"].Backend; got != "gemini" {
		t.Fatalf("backend = %q", got)
	}
	backend, model, err := ResolveAgent(Options{CacheHome: cache}, "light", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "gemini" || model != "gemini-3-1-pro" {
		t.Fatalf("resolved backend/model = %q/%q", backend, model)
	}
}

func TestResolveAgentExplicitModelWinsAndInfersBackend(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "core", "codex", "gpt-5.2"); err != nil {
		t.Fatal(err)
	}
	backend, model, err := ResolveAgent(Options{CacheHome: cache}, "core", "", "claude-sonnet-4")
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
		{tier: "light", model: "gpt-5.4-mini"},
		{tier: "core", model: "gpt-5.5"},
		{tier: "deep", model: "gpt-5.5"},
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

func TestResolveAgentModelAliasUsesHarness(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: cache}, "", "", "core", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "claude" || model != "sonnet" {
		t.Fatalf("resolved claude alias = %q/%q", backend, model)
	}

	backend, model, err = ResolveAgentForHarness(Options{CacheHome: cache}, "", "", "core", "codex")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("resolved codex alias = %q/%q", backend, model)
	}
}

func TestResolveAgentConcreteModelWinsOverHarnessAlias(t *testing.T) {
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "deep", "", "gpt-5.5", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "codex" || model != "gpt-5.5" {
		t.Fatalf("resolved concrete model = %q/%q", backend, model)
	}
}

func TestResolveAgentLegacyTierUsesHarness(t *testing.T) {
	backend, model, err := ResolveAgentForHarness(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "deep", "", "", "claude")
	if err != nil {
		t.Fatalf("ResolveAgentForHarness returned error: %v", err)
	}
	if backend != "claude" || model != "opus" {
		t.Fatalf("resolved legacy tier = %q/%q", backend, model)
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
	if len(view.Config.Agents.Tiers) != 3 {
		t.Fatalf("default tiers = %#v", view.Config.Agents.Tiers)
	}
	if light := view.Config.Agents.Tiers["light"]; light.Backend != "codex" || light.Model != "gpt-5.4-mini" {
		t.Fatalf("default light tier = %#v", light)
	}
	if core := view.Config.Agents.Tiers["core"]; core.Backend != "codex" || core.Model != "gpt-5.5" {
		t.Fatalf("default core tier = %#v", core)
	}
	if deep := view.Config.Agents.Tiers["deep"]; deep.Backend != "codex" || deep.Model != "gpt-5.5" {
		t.Fatalf("default deep tier = %#v", deep)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("Show created config file or stat failed: %v", err)
	}
}

func TestShowReturnsConfiguredMapping(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := SetAgentsTier(Options{CacheHome: cache}, "light", "", "gemini-3-1-pro"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	view, err := Show(Options{CacheHome: cache})
	if err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	mapping := view.Config.Agents.Tiers["light"]
	if mapping.Backend != "gemini" || mapping.Model != "gemini-3-1-pro" {
		t.Fatalf("light mapping = %#v", mapping)
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
	if err := os.WriteFile(path, []byte(`{
  "schema_version": 1,
  "agents": {
    "tiers": {
      "light": {
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
	if light := cfg.Agents.Tiers["light"]; light.Backend != "gemini" || light.Model != "gemini-3-1-pro" {
		t.Fatalf("light mapping was overwritten: %#v", light)
	}
	if core := cfg.Agents.Tiers["core"]; core.Backend != "codex" || core.Model != "gpt-5.5" {
		t.Fatalf("core mapping = %#v", core)
	}
	if deep := cfg.Agents.Tiers["deep"]; deep.Backend != "codex" || deep.Model != "gpt-5.5" {
		t.Fatalf("deep mapping = %#v", deep)
	}
}
