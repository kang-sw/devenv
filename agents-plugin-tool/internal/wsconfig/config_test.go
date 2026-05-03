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

func TestResolveAgentDefaultCodex(t *testing.T) {
	backend, model, err := ResolveAgent(Options{CacheHome: filepath.Join(t.TempDir(), "cache")}, "sonnet", "", "")
	if err != nil {
		t.Fatalf("ResolveAgent returned error: %v", err)
	}
	if backend != "codex" || model != "" {
		t.Fatalf("resolved backend/model = %q/%q", backend, model)
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
	if len(view.Config.Agents.Tiers) != 0 {
		t.Fatalf("default tiers = %#v", view.Config.Agents.Tiers)
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
