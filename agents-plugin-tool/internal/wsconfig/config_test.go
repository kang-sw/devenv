package wsconfig

import (
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
